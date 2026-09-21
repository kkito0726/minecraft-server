package settings_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
)

func TestParseDifficulty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    settings.Difficulty
		wantErr bool
	}{
		{in: "peaceful", want: settings.DifficultyPeaceful},
		{in: "easy", want: settings.DifficultyEasy},
		{in: "normal", want: settings.DifficultyNormal},
		{in: "hard", want: settings.DifficultyHard},
		// .env を手で書くと大文字にされることがある。弾くと画面が開けなくなる。
		{in: "HARD", want: settings.DifficultyHard},
		{in: " Normal ", want: settings.DifficultyNormal},
		{in: "hardcore", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := settings.ParseDifficulty(tt.in)
			if tt.wantErr {
				if !errors.Is(err, settings.ErrInvalid) {
					t.Fatalf("ErrInvalid を期待したが %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewAcceptsValidSettings(t *testing.T) {
	t.Parallel()

	s, err := settings.New(settings.Params{
		Difficulty: settings.DifficultyHard, Mode: settings.GameModeCreative,
		MOTD: "§aようこそ", MaxPlayers: 10, ViewDistance: 8, SimulationDistance: 6,
	})
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if s.Difficulty() != settings.DifficultyHard || s.MOTD() != "§aようこそ" ||
		s.MaxPlayers() != 10 || s.ViewDistance() != 8 || s.SimulationDistance() != 6 {
		t.Errorf("値が保持されていない: %+v", s)
	}
}

// 範囲の端はちょうど受け付け、1 つ外れたら弾く。
func TestNewRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                      string
		players, view, simulation int
		wantErr                   bool
	}{
		{name: "最小の組み合わせ", players: 1, view: 3, simulation: 3},
		{name: "最大の組み合わせ", players: 100, view: 32, simulation: 32},
		{name: "人数が 0", players: 0, view: 7, simulation: 5, wantErr: true},
		{name: "人数が上限超え", players: 101, view: 7, simulation: 5, wantErr: true},
		{name: "描画距離が小さすぎる", players: 5, view: 2, simulation: 2, wantErr: true},
		{name: "描画距離が大きすぎる", players: 5, view: 33, simulation: 5, wantErr: true},
		{name: "シミュレーション距離が小さすぎる", players: 5, view: 7, simulation: 2, wantErr: true},
		// 見えない範囲まで処理しても CPU を使うだけ。Pi では許さない。
		{name: "シミュレーションが描画より遠い", players: 5, view: 6, simulation: 7, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := settings.New(settings.Params{
				Difficulty: settings.DifficultyNormal, Mode: settings.GameModeSurvival,
				MOTD: "motd", MaxPlayers: tt.players,
				ViewDistance: tt.view, SimulationDistance: tt.simulation,
			})
			if tt.wantErr && !errors.Is(err, settings.ErrInvalid) {
				t.Fatalf("ErrInvalid を期待したが %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
		})
	}
}

// MOTD は .env に書かれ、docker compose がシェルに近い規則で読む。
// 展開や退避の解釈が揺れる文字は値の段階で弾く。
func TestNewValidatesMOTD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		motd    string
		wantErr bool
	}{
		{name: "色コードつき", motd: "§aE2E のサーバー"},
		{name: "上限ちょうど", motd: strings.Repeat("あ", settings.MaxMOTDLength)},
		{name: "上限超え", motd: strings.Repeat("あ", settings.MaxMOTDLength+1), wantErr: true},
		{name: "空", motd: "", wantErr: true},
		{name: "空白だけ", motd: "   ", wantErr: true},
		{name: "改行", motd: "1 行目\n2 行目", wantErr: true},
		{name: "タブ", motd: "a\tb", wantErr: true},
		{name: "二重引用符", motd: `say "hi"`, wantErr: true},
		{name: "ドル記号（変数展開）", motd: "$HOME", wantErr: true},
		{name: "バッククォート（コマンド置換）", motd: "`id`", wantErr: true},
		{name: "バックスラッシュ", motd: `a\b`, wantErr: true},
		{name: "不正な UTF-8", motd: string([]byte{0xff, 0xfe}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := settings.New(settings.Params{
				Difficulty: settings.DifficultyNormal, Mode: settings.GameModeSurvival,
				MOTD: tt.motd, MaxPlayers: 5, ViewDistance: 7, SimulationDistance: 5,
			})
			if tt.wantErr && !errors.Is(err, settings.ErrInvalid) {
				t.Fatalf("ErrInvalid を期待したが %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
		})
	}
}

func TestNewRejectsUnknownDifficulty(t *testing.T) {
	t.Parallel()

	_, err := settings.New(settings.Params{
		Difficulty: settings.Difficulty("hardcore"), Mode: settings.GameModeSurvival,
		MOTD: "motd", MaxPlayers: 5, ViewDistance: 7, SimulationDistance: 5,
	})
	if !errors.Is(err, settings.ErrInvalid) {
		t.Fatalf("ErrInvalid を期待したが %v", err)
	}
}

// 既定値は compose.yaml の `${MC_...:-既定}` と一致していなければならない。
// 食い違うと、.env にキーが無いときの画面の表示が実際の動作と違う。
func TestDefaultsMatchCompose(t *testing.T) {
	t.Parallel()

	d := settings.Defaults()
	if d.Difficulty() != settings.DifficultyNormal {
		t.Errorf("difficulty = %q", d.Difficulty())
	}
	if d.MOTD() != "A Minecraft Server on Docker" {
		t.Errorf("motd = %q", d.MOTD())
	}
	if d.MaxPlayers() != 5 || d.ViewDistance() != 7 || d.SimulationDistance() != 5 {
		t.Errorf("数値の既定が compose.yaml と違う: %+v", d)
	}

	// 既定値そのものが規則を満たしていること。
	if _, err := settings.New(settings.Params{
		Difficulty: d.Difficulty(), Mode: d.Mode(), MOTD: d.MOTD(),
		MaxPlayers: d.MaxPlayers(), ViewDistance: d.ViewDistance(),
		SimulationDistance: d.SimulationDistance(), Hardcore: d.Hardcore(),
	}); err != nil {
		t.Errorf("既定値が規則を満たしていない: %v", err)
	}
}

func TestParseGameMode(t *testing.T) {
	t.Parallel()

	// 手で書かれた .env は大文字のことがある。弾くと設定画面が開けなくなる。
	for _, in := range []string{"creative", "CREATIVE", " Creative "} {
		got, err := settings.ParseGameMode(in)
		if err != nil {
			t.Fatalf("%q を読めない: %v", in, err)
		}
		if got != settings.GameModeCreative {
			t.Errorf("%q が %q", in, got)
		}
	}

	for _, in := range []string{"", "hardcore", "survivals"} {
		if _, err := settings.ParseGameMode(in); !errors.Is(err, settings.ErrInvalid) {
			t.Errorf("%q は ErrInvalid のはず: %v", in, err)
		}
	}
}

// ハードコアは検証しない。真偽どちらも有効で、画面からは変えられない。
func TestHardcoreIsCarriedThrough(t *testing.T) {
	t.Parallel()

	s, err := settings.New(settings.Params{
		Difficulty: settings.DifficultyNormal, Mode: settings.GameModeSurvival,
		MOTD: "motd", MaxPlayers: 5, ViewDistance: 7, SimulationDistance: 5,
		Hardcore: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Hardcore() {
		t.Error("Hardcore が落ちている")
	}
}
