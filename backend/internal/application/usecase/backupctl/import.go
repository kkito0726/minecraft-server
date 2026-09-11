package backupctl

import (
	"context"
	"fmt"
	"io"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// importNote はファイル名の末尾に付ける印。持ち込んだものだと後で分かる。
const importNote = "imported"

// unknownVersion はアーカイブの版を読めなかったときのファイル名用の値。
const unknownVersion = "unknown"

// rewrapFactor は包み直しに要る一時的な容量の倍率。
//
// 包み直しは元のアーカイブを読みながら新しいアーカイブを書くので、
// 確定するまでの間だけ 2 つ分の場所が要る。
const rewrapFactor = 2

// ImportResult は取り込みの結果。
type ImportResult struct {
	ID        backup.ID
	Level     string
	Version   shared.WorldVersion
	SizeBytes int64
	// Rewrapped は data/ 配下へ包み直したかどうか。
	Rewrapped bool
}

/*
Import は外から持ち込まれたアーカイブを保管先に取り込む。

取り込むだけで、ワールドには一切触れない。差し替えるかどうかは
このあと復元の画面で、これまでどおりの関門（バージョンの確認と
名前の入力）を通して決める。取り込みを復元と切り離してあるのは、
「置くこと」と「使うこと」を同時に決めさせないため。

操作（Operation）にはしない。data/ を触らないので排他の枠を
消費する理由が無く、待たせると取り込み中に他の操作ができなくなる。
*/
func (u *UseCase) Import(
	ctx context.Context, src io.Reader, declaredSize int64,
) (ImportResult, error) {
	limit, err := u.importLimit(ctx, declaredSize)
	if err != nil {
		return ImportResult{}, err
	}

	staged, err := u.cfg.Store.Stage(ctx, src, limit)
	if err != nil {
		return ImportResult{}, err
	}
	// 確定していなければ一時ファイルを消す。中断がゴミを積むと
	// いずれディスクが埋まる。
	defer staged.Discard()

	plan, err := backup.PlanImport(staged.LevelDatEntries())
	if err != nil {
		return ImportResult{}, err
	}

	version := u.stagedVersion(ctx, staged, plan)
	id, err := backup.NewID(backup.BuildName(
		versionLabel(version), plan.Level, importNote, u.cfg.Clock.Now()))
	if err != nil {
		return ImportResult{}, err
	}

	stored, err := staged.Adopt(ctx, id, plan.Prefix)
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		ID:        stored.ID,
		Level:     plan.Level,
		Version:   version,
		SizeBytes: stored.SizeBytes,
		Rewrapped: plan.NeedsRewrap(),
	}, nil
}

/*
importLimit は受け取ってよい大きさを決める。

Pi ではディスクを埋めきるとサーバーごと止まる。送られてくる大きさが
分かっているなら、受け取る前に落とす。分からない場合も空き容量を
上限にして、埋めきる前に打ち切れるようにする。
*/
func (u *UseCase) importLimit(ctx context.Context, declaredSize int64) (int64, error) {
	available, err := u.cfg.Worlds.AvailableBytes(ctx)
	if err != nil {
		return 0, err
	}

	// 包み直しの可能性があるので、確定までに 2 つ分を見込む。
	budget := available / rewrapFactor
	if budget <= 0 {
		return 0, fmt.Errorf("%w（空き %d バイト）", port.ErrInsufficientSpace, available)
	}
	if declaredSize > 0 && declaredSize > budget {
		return 0, fmt.Errorf("%w（必要 %d バイト、空き %d バイト）",
			port.ErrInsufficientSpace, declaredSize*rewrapFactor, available)
	}
	return budget, nil
}

// stagedVersion はアーカイブ内の level.dat から版を読む。
//
// 読めなくても取り込みは止めない。版が分からないワールドでも、
// 復元の画面が「不明」として承諾を求めるので安全側に倒れる。
func (u *UseCase) stagedVersion(
	ctx context.Context, staged port.ArchiveImport, plan backup.ImportPlan,
) shared.WorldVersion {
	entry := levelDatEntryFor(staged.LevelDatEntries(), plan)
	if entry == "" || u.cfg.Levels == nil {
		return shared.UnreadableWorldVersion()
	}

	rc, err := staged.OpenEntry(ctx, entry)
	if err != nil {
		return shared.UnreadableWorldVersion()
	}
	defer func() { _ = rc.Close() }()

	return u.cfg.Levels.Read(ctx, rc)
}

// levelDatEntryFor は計画で選んだワールドの level.dat を選び直す。
func levelDatEntryFor(entries []string, plan backup.ImportPlan) string {
	for _, entry := range entries {
		if backup.IsLevelDatOf(entry, plan.Level) {
			return entry
		}
	}
	return ""
}

// versionLabel はファイル名に入れる版の文字列。
func versionLabel(v shared.WorldVersion) string {
	if !v.Readable() || v.Name() == "" {
		return unknownVersion
	}
	return v.Name()
}
