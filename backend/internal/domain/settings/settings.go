// Package settings はゲーム設定のドメインモデル。
//
// 管理画面から書き換えてよい設定だけを型にしている。バージョン・サーバー種別・
// メモリはここに無い。前者は片道のアップグレードを、後者は Pi が固まる状態を
// 画面のひと押しで起こせてしまうため、あえて扱わない。
//
// この層は外部を知らない。.env のキー名も持たない。値の規則だけを表す。
package settings

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalid はゲーム設定として使えない値であることを表す。
var ErrInvalid = errors.New("ゲーム設定が不正です")

// Difficulty は難易度。
type Difficulty string

// 難易度の値。server.properties の difficulty に書かれる文字列と一致させる。
const (
	DifficultyPeaceful Difficulty = "peaceful"
	DifficultyEasy     Difficulty = "easy"
	DifficultyNormal   Difficulty = "normal"
	DifficultyHard     Difficulty = "hard"
)

var difficulties = []Difficulty{
	DifficultyPeaceful,
	DifficultyEasy,
	DifficultyNormal,
	DifficultyHard,
}

// ParseDifficulty は難易度を読む。
//
// 大文字小文字は区別しない。.env を手で書くと NORMAL のように大文字に
// されることがあり、それを「不正」として弾くと画面が開けなくなる。
func ParseDifficulty(s string) (Difficulty, error) {
	d := Difficulty(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range difficulties {
		if d == known {
			return d, nil
		}
	}
	return "", fmt.Errorf(
		"%w: 難易度 %q は peaceful / easy / normal / hard のいずれかです", ErrInvalid, s)
}

// 値の範囲。
//
// 距離の範囲は server.properties が受け付ける 3〜32 に合わせる。範囲外を
// 書くとサーバー側で黙って丸められ、画面の表示と実際の値が食い違う。
const (
	MinMaxPlayers = 1
	MaxMaxPlayers = 100
	MinDistance   = 3
	MaxDistance   = 32
	MaxMOTDLength = 100
)

// forbiddenMOTDChars は MOTD に含めてはいけない文字。
//
// .env は docker compose がシェルに近い規則で読む。二重引用符の中でも $ は
// 変数展開され、` や \ の扱いは実装ごとに揺れる。書き込み側で退避しても、
// 読む側の compose が同じ規則で戻す保証が無い。値の段階で弾く方が確実。
const forbiddenMOTDChars = "\"$`\\"

// GameSettings は管理画面から書き換えてよいゲーム設定。
//
// 値オブジェクトであり、生成できた時点で規則を満たしていることが保証される。
type GameSettings struct {
	difficulty         Difficulty
	motd               string
	maxPlayers         int
	viewDistance       int
	simulationDistance int
}

// Defaults は compose.yaml の既定値と同じ設定を返す。
//
// .env にキーが無いとき、compose は `${MC_DIFFICULTY:-normal}` のように
// 既定値で補う。画面の表示もそれに合わせないと、実際に動いている値と食い違う。
func Defaults() GameSettings {
	return GameSettings{
		difficulty:         DifficultyNormal,
		motd:               "A Minecraft Server on Docker",
		maxPlayers:         5,
		viewDistance:       7,
		simulationDistance: 5,
	}
}

// New はゲーム設定を検証して作る。
func New(
	difficulty Difficulty,
	motd string,
	maxPlayers, viewDistance, simulationDistance int,
) (GameSettings, error) {
	if _, err := ParseDifficulty(string(difficulty)); err != nil {
		return GameSettings{}, err
	}
	if err := validateMOTD(motd); err != nil {
		return GameSettings{}, err
	}
	if err := validateRange("最大人数", maxPlayers, MinMaxPlayers, MaxMaxPlayers); err != nil {
		return GameSettings{}, err
	}
	if err := validateRange("描画距離", viewDistance, MinDistance, MaxDistance); err != nil {
		return GameSettings{}, err
	}
	if err := validateRange("シミュレーション距離", simulationDistance, MinDistance, MaxDistance); err != nil {
		return GameSettings{}, err
	}
	// 描画されない範囲まで処理しても、見えない場所の CPU を使うだけになる。
	// Pi では距離の設定が負荷を最も左右するので、無駄な組み合わせを許さない。
	if simulationDistance > viewDistance {
		return GameSettings{}, fmt.Errorf(
			"%w: シミュレーション距離（%d）は描画距離（%d）以下にしてください",
			ErrInvalid, simulationDistance, viewDistance)
	}

	return GameSettings{
		difficulty:         difficulty,
		motd:               motd,
		maxPlayers:         maxPlayers,
		viewDistance:       viewDistance,
		simulationDistance: simulationDistance,
	}, nil
}

func validateMOTD(motd string) error {
	switch {
	case strings.TrimSpace(motd) == "":
		// 空にすると compose の既定値に置き換わり、書いた内容と表示が食い違う。
		return fmt.Errorf("%w: MOTD は空にできません", ErrInvalid)
	case !utf8.ValidString(motd):
		return fmt.Errorf("%w: MOTD に読めない文字が含まれています", ErrInvalid)
	case utf8.RuneCountInString(motd) > MaxMOTDLength:
		return fmt.Errorf("%w: MOTD は %d 文字以内にしてください", ErrInvalid, MaxMOTDLength)
	case strings.IndexFunc(motd, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: MOTD に改行や制御文字は使えません", ErrInvalid)
	case strings.ContainsAny(motd, forbiddenMOTDChars):
		return fmt.Errorf("%w: MOTD に \" $ ` \\ は使えません", ErrInvalid)
	default:
		return nil
	}
}

func validateRange(label string, value, low, high int) error {
	if value < low || value > high {
		return fmt.Errorf("%w: %sは %d〜%d の範囲にしてください（%d）", ErrInvalid, label, low, high, value)
	}
	return nil
}

// Difficulty は難易度を返す。
func (s GameSettings) Difficulty() Difficulty { return s.difficulty }

// MOTD はサーバー一覧に出る説明文を返す。
func (s GameSettings) MOTD() string { return s.motd }

// MaxPlayers は最大人数を返す。
func (s GameSettings) MaxPlayers() int { return s.maxPlayers }

// ViewDistance は描画距離（チャンク）を返す。
func (s GameSettings) ViewDistance() int { return s.viewDistance }

// SimulationDistance はシミュレーション距離（チャンク）を返す。
func (s GameSettings) SimulationDistance() int { return s.simulationDistance }
