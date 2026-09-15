package rpc

import (
	"errors"
	"testing"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
)

func TestDifficultyRoundTrip(t *testing.T) {
	t.Parallel()

	for _, d := range []settings.Difficulty{
		settings.DifficultyPeaceful, settings.DifficultyEasy,
		settings.DifficultyNormal, settings.DifficultyHard,
	} {
		p := difficultyToProto(d)
		if p == mcadminv1.Difficulty_DIFFICULTY_UNSPECIFIED {
			t.Errorf("%q が未指定に落ちている", d)
			continue
		}
		back, err := difficultyFromProto(p)
		if err != nil || back != d {
			t.Errorf("%q → %v → %q (%v)", d, p, back, err)
		}
	}
}

// 欄を送り忘れたクライアントが、利用者の意図と無関係に難易度を変えないこと。
func TestDifficultyUnspecifiedIsRejected(t *testing.T) {
	t.Parallel()

	if _, err := difficultyFromProto(mcadminv1.Difficulty_DIFFICULTY_UNSPECIFIED); !errors.Is(err, settings.ErrInvalid) {
		t.Fatalf("ErrInvalid を期待したが %v", err)
	}
	if got := difficultyToProto(settings.Difficulty("hardcore")); got != mcadminv1.Difficulty_DIFFICULTY_UNSPECIFIED {
		t.Errorf("未知の難易度が %v になった", got)
	}
}

func TestGameSettingsFromProto(t *testing.T) {
	t.Parallel()

	if _, err := gameSettingsFromProto(nil); !errors.Is(err, settings.ErrInvalid) {
		t.Errorf("nil は ErrInvalid のはず: %v", err)
	}

	s, err := gameSettingsFromProto(&mcadminv1.GameSettings{
		Difficulty: mcadminv1.Difficulty_DIFFICULTY_EASY, Motd: "hi",
		MaxPlayers: 4, ViewDistance: 8, SimulationDistance: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	back := gameSettingsToProto(s)
	if back.GetDifficulty() != mcadminv1.Difficulty_DIFFICULTY_EASY || back.GetMotd() != "hi" ||
		back.GetMaxPlayers() != 4 || back.GetViewDistance() != 8 || back.GetSimulationDistance() != 6 {
		t.Errorf("往復で値が変わった: %+v", back)
	}

	// 規則の検証はドメインに任せる。ここで素通りさせないこと。
	_, err = gameSettingsFromProto(&mcadminv1.GameSettings{
		Difficulty: mcadminv1.Difficulty_DIFFICULTY_EASY, Motd: "hi",
		MaxPlayers: 4, ViewDistance: 5, SimulationDistance: 9,
	})
	if !errors.Is(err, settings.ErrInvalid) {
		t.Errorf("規則外の組み合わせが通った: %v", err)
	}
}
