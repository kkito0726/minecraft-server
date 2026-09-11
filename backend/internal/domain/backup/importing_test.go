package backup

import (
	"errors"
	"testing"
)

/*
外部から持ち込まれた zip の形を見分ける。

管理コンソールが作るアーカイブは data/<名前>/... の形だが、配布されて
いるワールドや他のサーバーから持ってきたものは <名前>/... の形をして
いることが多い。そのまま展開すると data/ の外に書くことになるため、
取り込みの時点で包み直す必要がある。

どちらとも判断できないものは受け取らない。曖昧なまま進めると、
利用者の意図と違う場所にワールドが置かれる。
*/
func TestPlanImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entries    []string
		wantLevel  string
		wantPrefix string
	}{
		{
			name:      "コンソールが作った形はそのまま置ける",
			entries:   []string{"data/world/level.dat"},
			wantLevel: "world",
		},
		{
			name:      "他のワールドが混ざっていても data/ 配下を優先する",
			entries:   []string{"data/creative/level.dat"},
			wantLevel: "creative",
		},
		{
			name:       "配布ワールドの形は data/ 配下へ包み直す",
			entries:    []string{"MyWorld/level.dat"},
			wantLevel:  "MyWorld",
			wantPrefix: "data/",
		},
		{
			name:       "深い階層を持つ配布ワールドも包み直せる",
			entries:    []string{"SkyBlock/level.dat"},
			wantLevel:  "SkyBlock",
			wantPrefix: "data/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanImport(tt.entries)
			if err != nil {
				t.Fatalf("拒否された: %v", err)
			}
			if got.Level != tt.wantLevel {
				t.Errorf("ワールド名が %q（期待 %q）", got.Level, tt.wantLevel)
			}
			if got.Prefix != tt.wantPrefix {
				t.Errorf("接頭辞が %q（期待 %q）", got.Prefix, tt.wantPrefix)
			}
		})
	}
}

func TestPlanImportRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []string
		// reason は利用者が次に何をすればよいか分かる語を含むこと。
		reason string
	}{
		{"level.dat が無い", []string{"data/plugins/foo.jar"}, "ワールド"},
		{"空", nil, "ワールド"},
		// ルート直下の level.dat はワールド名が決まらない。
		// 名前を勝手に決めると、利用者の意図と違う名前で保存される。
		{"ルート直下の level.dat", []string{"level.dat"}, "フォルダ"},
		// 包み直す側でワールドが複数あると、どれを入れるのか決まらない。
		{"包み直す形でワールドが複数", []string{"A/level.dat", "B/level.dat"}, "複数"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := PlanImport(tt.entries)
			if !errors.Is(err, ErrNotImportable) {
				t.Fatalf("受理された、または別のエラー: %v", err)
			}
			if !contains(err.Error(), tt.reason) {
				t.Errorf("理由に %q が含まれない: %v", tt.reason, err)
			}
		})
	}
}

// data/ 配下が複数あるのはコンソールが作ったものには起こらないが、
// 起きたときに黙って片方を選ぶと、戻したつもりのワールドが別物になる。
func TestPlanImportIsDeterministicWithMultipleDataWorlds(t *testing.T) {
	t.Parallel()

	first, err := PlanImport([]string{"data/b/level.dat", "data/a/level.dat"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanImport([]string{"data/a/level.dat", "data/b/level.dat"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Level != second.Level {
		t.Errorf("並び順で結果が変わる: %q と %q", first.Level, second.Level)
	}
}

// 取り込んだワールドはそのままディレクトリ名になる。
// 名前の規則を通らないものを受け取ると、ファイルシステムに届いてしまう。
func TestPlanImportRejectsUnsafeWorldName(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"../escape/level.dat", "with space/level.dat"} {
		if _, err := PlanImport([]string{entry}); err == nil {
			t.Errorf("%q を受理した", entry)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
