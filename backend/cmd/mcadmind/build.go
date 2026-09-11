package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"connectrpc.com/connect"

	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/reconcile"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/config/dotenv"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/console/rcon"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/container/compose"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/filesystem/worldfs"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/leveldat"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
	adminhttp "github.com/kkito0726/minecraft-server/backend/internal/presentation/http"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/http/auth"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/rpc"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/upload"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/webui"
)

// 設定のキー。
const (
	keyAdminToken     = "ADMIN_TOKEN"
	keyAdminAddr      = "ADMIN_ADDR"
	keyAdminDockerBin = "ADMIN_DOCKER_BIN"
	keyAdminBackupDir = "ADMIN_BACKUP_DIR"
	keyAdminProject   = "ADMIN_COMPOSE_PROJECT"
)

// 既定値。
const (
	defaultAddr      = "0.0.0.0:8787"
	defaultDockerBin = "docker"
	defaultBackupDir = "backups"
	composeService   = "mc"
	dataDirName      = "data"
	lockFileName     = ".admin-console.lock"
)

// publicConfigKeys は API から返してよい .env のキー。
//
// 許可リスト方式にするのは、新しいキーが増えたときに既定で秘匿される
// ようにするため。RCON_PASSWORD と ADMIN_TOKEN は決して含めない。
var publicConfigKeys = []string{
	"MC_VERSION", "MC_TYPE", "MC_LEVEL", "MC_MOTD",
	"MC_DIFFICULTY", "MC_MAX_PLAYERS", "MC_MEMORY", "MC_MEM_LIMIT",
	"MC_VIEW_DISTANCE", "MC_SIMULATION_DISTANCE", "TZ",
}

// app は組み立て済みのアプリケーション。
type app struct {
	server     *adminhttp.Server
	reconciler *reconcile.Reconciler
	status     *serverctl.StatusUseCase
	logger     *slog.Logger
}

// build は依存を組み立てる。
//
// クリーンアーキテクチャの配線はここだけで行う。各層は互いの
// 具体的な実装を知らず、インターフェース越しにしか繋がらない。
func build(ctx context.Context, opts options, logger *slog.Logger) (*app, error) {
	if err := validateProjectDir(opts.projectDir); err != nil {
		return nil, err
	}
	if !webui.IsBuilt() {
		return nil, errors.New("フロントエンドがビルドされていません。make build-front を実行してください")
	}

	config := dotenv.NewAdapter(filepath.Join(opts.projectDir, ".env"))

	settings, err := loadSettings(ctx, opts.projectDir, config)
	if err != nil {
		return nil, err
	}

	deps, err := buildDeps(ctx, opts, settings, config, logger)
	if err != nil {
		return nil, err
	}

	server, err := buildServer(settings, deps, logger)
	if err != nil {
		return nil, err
	}

	return &app{
		server: server, reconciler: deps.reconciler,
		status: deps.status, logger: logger,
	}, nil
}

// deps は組み立てた依存の束。
type deps struct {
	status     *serverctl.StatusUseCase
	lifecycle  *serverctl.LifecycleUseCase
	worlds     *worldctl.UseCase
	backups    *backupctl.UseCase
	ops        *operations.Manager
	reconciler *reconcile.Reconciler
}

// buildDeps はインフラとユースケースを組み立てる。
// infra は外部と話す実装の束。
type infra struct {
	runtime *compose.Runner
	console *rcon.Client
	levels  *leveldat.Adapter
	worlds  *worldfs.Repository
	lock    *lockfile.Lock
	backups *backupfs.Store
}

// buildInfra はインフラ層を組み立てる。
func buildInfra(opts options, s settings) (infra, error) {
	dataDir := filepath.Join(opts.projectDir, dataDirName)

	runtime := compose.NewRunner(compose.Config{
		DockerBin:   s.dockerBin,
		ProjectDir:  opts.projectDir,
		ProjectName: s.composeProject,
		Service:     composeService,
	})
	levels := leveldat.NewAdapter(dataDir)

	worldRepo, err := worldfs.New(worldfs.Config{DataDir: dataDir, Versions: levels})
	if err != nil {
		return infra{}, err
	}

	backupStore, err := backupfs.New(backupfs.Config{
		ProjectDir: opts.projectDir,
		BackupDir:  s.backupDir,
	})
	if err != nil {
		return infra{}, err
	}

	return infra{
		runtime: runtime,
		console: rcon.NewClient(runtime),
		levels:  levels,
		worlds:  worldRepo,
		lock:    lockfile.NewLock(filepath.Join(dataDir, lockFileName)),
		backups: backupStore,
	}, nil
}

func buildDeps(
	ctx context.Context,
	opts options,
	s settings,
	config *dotenv.Adapter,
	logger *slog.Logger,
) (deps, error) {
	in, err := buildInfra(opts, s)
	if err != nil {
		return deps{}, err
	}
	// 操作の寿命はアプリの寿命に紐づける。HTTP リクエストの context を
	// 引き継ぐと、レスポンスを返した時点で復元やバックアップが死ぬ。
	ops, err := operations.NewManager(operations.Config{Lock: in.lock, BaseCtx: ctx})
	if err != nil {
		return deps{}, err
	}

	return buildUseCases(in, config, ops, logger)
}

// buildUseCases はユースケース層を組み立てる。
func buildUseCases(
	in infra,
	config *dotenv.Adapter,
	ops *operations.Manager,
	logger *slog.Logger,
) (deps, error) {
	status, err := serverctl.NewStatusUseCase(serverctl.StatusConfig{
		Runtime: in.runtime, Console: in.console, Config: config, Levels: in.levels,
	})
	if err != nil {
		return deps{}, err
	}

	lifecycle, err := serverctl.NewLifecycleUseCase(serverctl.LifecycleConfig{
		Runtime: in.runtime, Console: in.console, Operations: ops,
	})
	if err != nil {
		return deps{}, err
	}

	worlds, err := worldctl.New(worldctl.Config{
		Runtime: in.runtime, Console: in.console, Worlds: in.worlds,
		Config: config, Levels: in.levels, Operations: ops,
	})
	if err != nil {
		return deps{}, err
	}

	backups, err := backupctl.New(backupctl.Config{
		Runtime: in.runtime, Console: in.console, Store: in.backups,
		Worlds: in.worlds, Config: config, Levels: in.levels, Operations: ops,
	})
	if err != nil {
		return deps{}, err
	}

	reconciler, err := reconcile.New(reconcile.Config{
		Console: in.console,
		Health:  healthChecker{runtime: in.runtime},
		Lock:    in.lock,
		Logger:  logger,
	})
	if err != nil {
		return deps{}, err
	}

	return deps{
		status: status, lifecycle: lifecycle, worlds: worlds, backups: backups,
		ops: ops, reconciler: reconciler,
	}, nil
}

// settings は .env から読んだ管理コンソールの設定。
type settings struct {
	token     string
	addr      string
	dockerBin string
	// backupDir はアーカイブの保管先。相対パスはプロジェクトディレクトリ基準。
	backupDir string
	// composeProject は操作する compose のプロジェクト名。
	//
	// **プロジェクトディレクトリごとに変わる。** 固定にすると、別の
	// ディレクトリへ向けたつもりの mcadmind が同じコンテナを操作する。
	composeProject string
}

func loadSettings(ctx context.Context, projectDir string, config *dotenv.Adapter) (settings, error) {
	snapshot, err := config.Load(ctx)
	if err != nil {
		return settings{}, err
	}

	token, _ := snapshot.Get(keyAdminToken)
	if token == "" {
		return settings{}, fmt.Errorf(
			"%w: %s が設定されていません。生成: openssl rand -hex 32",
			errNotConfigured, keyAdminToken)
	}

	addr, _ := snapshot.Get(keyAdminAddr)
	if addr == "" {
		addr = defaultAddr
	}

	dockerBin, _ := snapshot.Get(keyAdminDockerBin)
	if dockerBin == "" {
		dockerBin = defaultDockerBin
	}
	// systemd 配下では PATH が通らないことがある。起動時に解決しておく。
	resolved, err := exec.LookPath(dockerBin)
	if err != nil {
		return settings{}, fmt.Errorf("docker が見つかりません (%s): %w", dockerBin, err)
	}

	project, _ := snapshot.Get(keyAdminProject)

	return settings{
		token: token, addr: addr, dockerBin: resolved,
		backupDir:      backupDirFrom(snapshot, projectDir),
		composeProject: composeProjectFrom(projectDir, project),
	}, nil
}

// backupDirFrom は保管先を解決する。
//
// 相対パスはプロジェクトディレクトリ基準で解決する。systemd 配下では
// 作業ディレクトリがリポジトリと一致しないため、プロセスの cwd を
// 基準にすると保管先が思わぬ場所になる。
func backupDirFrom(snapshot port.ConfigSnapshot, projectDir string) string {
	dir, _ := snapshot.Get(keyAdminBackupDir)
	if dir == "" {
		dir = defaultBackupDir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(projectDir, dir)
}

func buildServer(s settings, d deps, logger *slog.Logger) (*adminhttp.Server, error) {
	interceptor, err := auth.NewInterceptor(s.token)
	if err != nil {
		return nil, err
	}
	// 認証を外側に置く。トークンが通らなかったリクエストの中身まで
	// 記録に残す必要はないし、残せば攻撃者の入力をログに書くことになる。
	withAuth := connect.WithInterceptors(interceptor, rpc.NewLoggingInterceptor(logger))

	handlers := map[string]http.Handler{}
	serverPath, serverHandler := mcadminv1connect.NewServerServiceHandler(
		rpc.NewServerHandler(d.status, d.lifecycle, d.ops, publicConfigKeys), withAuth)
	handlers[serverPath] = serverHandler

	opPath, opHandler := mcadminv1connect.NewOperationServiceHandler(
		rpc.NewOperationHandler(d.ops), withAuth)
	handlers[opPath] = opHandler

	worldPath, worldHandler := mcadminv1connect.NewWorldServiceHandler(
		rpc.NewWorldHandler(d.worlds), withAuth)
	handlers[worldPath] = worldHandler

	backupPath, backupHandler := mcadminv1connect.NewBackupServiceHandler(
		rpc.NewBackupHandler(d.backups), withAuth)
	handlers[backupPath] = backupHandler

	assets, err := webui.Assets()
	if err != nil {
		return nil, fmt.Errorf("フロントエンドを読めません: %w", err)
	}

	// Connect の外側のルートは、ここで明示的に認証で包む。
	// 包み忘れるとトークン無しで叩ける口になるため、配線を 1 箇所に寄せる。
	routes := map[string]http.Handler{
		adminhttp.UploadBackupPath: interceptor.Middleware(
			upload.NewHandler(d.backups, logger)),
	}

	return adminhttp.New(adminhttp.Config{
		Addr: s.addr, Assets: assets, RPC: handlers, Routes: routes, Logger: logger,
	})
}

// healthChecker は reconcile.Health の実装。
type healthChecker struct {
	runtime *compose.Runner
}

func (h healthChecker) Healthy(ctx context.Context) (bool, error) {
	st, err := h.runtime.Status(ctx)
	if err != nil {
		return false, err
	}
	return st.Healthy, nil
}

// Run はアプリケーションを起動する。
func (a *app) Run(ctx context.Context) error {
	// 起動時の回復処理。save-off が残っている可能性に備えて
	// 無条件に save-on を送り、中断された操作を検出する。
	result, err := a.reconciler.RunOnce(ctx)
	if err != nil {
		a.logger.Warn("起動時の回復処理に失敗しました", "error", err)
	}
	a.status.SetInterrupted(result.InterruptedDetected)

	// healthy への遷移を監視して save-on を送り直す。
	go a.reconciler.Loop(ctx)

	return a.server.ListenAndServe(ctx)
}

// validateProjectDir は compose.yaml と .env の存在を確認する。
// systemd 配下では作業ディレクトリがリポジトリと一致しないため、
// 起動時に確かめておかないと後続の docker compose が分かりにくい形で失敗する。
func validateProjectDir(dir string) error {
	for _, name := range []string{"compose.yaml", ".env"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("%s が見つかりません (%s): %w", name, dir, err)
		}
	}
	return nil
}
