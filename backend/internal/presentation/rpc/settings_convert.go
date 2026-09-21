package rpc

import (
	"fmt"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
)

// difficultyToProto は難易度を転送形式にする。
func difficultyToProto(d settings.Difficulty) mcadminv1.Difficulty {
	switch d {
	case settings.DifficultyPeaceful:
		return mcadminv1.Difficulty_DIFFICULTY_PEACEFUL
	case settings.DifficultyEasy:
		return mcadminv1.Difficulty_DIFFICULTY_EASY
	case settings.DifficultyNormal:
		return mcadminv1.Difficulty_DIFFICULTY_NORMAL
	case settings.DifficultyHard:
		return mcadminv1.Difficulty_DIFFICULTY_HARD
	default:
		return mcadminv1.Difficulty_DIFFICULTY_UNSPECIFIED
	}
}

// difficultyFromProto は転送形式の難易度を読む。
//
// 未指定を normal として受け取らない。欄を送り忘れたクライアントが、
// 利用者の意図と無関係に難易度を書き換えてしまう。
func difficultyFromProto(d mcadminv1.Difficulty) (settings.Difficulty, error) {
	switch d {
	case mcadminv1.Difficulty_DIFFICULTY_PEACEFUL:
		return settings.DifficultyPeaceful, nil
	case mcadminv1.Difficulty_DIFFICULTY_EASY:
		return settings.DifficultyEasy, nil
	case mcadminv1.Difficulty_DIFFICULTY_NORMAL:
		return settings.DifficultyNormal, nil
	case mcadminv1.Difficulty_DIFFICULTY_HARD:
		return settings.DifficultyHard, nil
	default:
		return "", fmt.Errorf("%w: 難易度が指定されていません", settings.ErrInvalid)
	}
}

// gameModeToProto はゲームモードを転送形式にする。
func gameModeToProto(m settings.GameMode) mcadminv1.GameMode {
	switch m {
	case settings.GameModeSurvival:
		return mcadminv1.GameMode_GAME_MODE_SURVIVAL
	case settings.GameModeCreative:
		return mcadminv1.GameMode_GAME_MODE_CREATIVE
	case settings.GameModeAdventure:
		return mcadminv1.GameMode_GAME_MODE_ADVENTURE
	case settings.GameModeSpectator:
		return mcadminv1.GameMode_GAME_MODE_SPECTATOR
	default:
		return mcadminv1.GameMode_GAME_MODE_UNSPECIFIED
	}
}

// gameModeFromProto は転送形式のゲームモードを読む。
//
// 難易度と同じく、未指定を survival として受け取らない。欄を送り忘れた
// クライアントが、利用者の意図と無関係にモードを書き換えてしまう。
func gameModeFromProto(m mcadminv1.GameMode) (settings.GameMode, error) {
	switch m {
	case mcadminv1.GameMode_GAME_MODE_SURVIVAL:
		return settings.GameModeSurvival, nil
	case mcadminv1.GameMode_GAME_MODE_CREATIVE:
		return settings.GameModeCreative, nil
	case mcadminv1.GameMode_GAME_MODE_ADVENTURE:
		return settings.GameModeAdventure, nil
	case mcadminv1.GameMode_GAME_MODE_SPECTATOR:
		return settings.GameModeSpectator, nil
	default:
		return "", fmt.Errorf("%w: ゲームモードが指定されていません", settings.ErrInvalid)
	}
}

func gameSettingsToProto(s settings.GameSettings) *mcadminv1.GameSettings {
	return &mcadminv1.GameSettings{
		Difficulty:         difficultyToProto(s.Difficulty()),
		Mode:               gameModeToProto(s.Mode()),
		Motd:               s.MOTD(),
		MaxPlayers:         int32(s.MaxPlayers()),
		ViewDistance:       int32(s.ViewDistance()),
		SimulationDistance: int32(s.SimulationDistance()),
		Hardcore:           s.Hardcore(),
	}
}

// gameSettingsFromProto は転送形式の設定を検証してドメインの型にする。
// 規則はドメインが持つ。ここは形の変換だけを受け持つ。
func gameSettingsFromProto(p *mcadminv1.GameSettings) (settings.GameSettings, error) {
	if p == nil {
		return settings.GameSettings{}, fmt.Errorf("%w: 設定が指定されていません", settings.ErrInvalid)
	}
	difficulty, err := difficultyFromProto(p.GetDifficulty())
	if err != nil {
		return settings.GameSettings{}, err
	}
	mode, err := gameModeFromProto(p.GetMode())
	if err != nil {
		return settings.GameSettings{}, err
	}
	// hardcore は受け取らない。画面から変えられない値なので、送られてきても
	// 無視する。書き込み側（write）もこのキーには触らない。
	return settings.New(settings.Params{
		Difficulty:         difficulty,
		Mode:               mode,
		MOTD:               p.GetMotd(),
		MaxPlayers:         int(p.GetMaxPlayers()),
		ViewDistance:       int(p.GetViewDistance()),
		SimulationDistance: int(p.GetSimulationDistance()),
	})
}
