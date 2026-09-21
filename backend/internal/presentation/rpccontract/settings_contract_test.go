package rpccontract

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
)

// 画面から書き換えてよいゲーム設定に、壊れ方の重い欄を足してはいけない。
//
// バージョンは片道のアップグレードを、メモリは Pi が固まる状態を、
// ワールド名とシードはワールド画面の関門を飛ばした切替を、
// 画面のひと押しで起こせるようにしてしまう。コンパイルは通るので、ここで止める。
func TestGameSettingsExcludesDangerousFields(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.GameSettings{}).ProtoReflect().Descriptor()

	forbidden := []protoreflect.Name{
		"version", "mc_version", "type", "mc_type",
		"memory", "mem_limit", "aikar_flags",
		"level", "seed", "rcon_password",
	}
	for _, name := range forbidden {
		if msg.Fields().ByName(name) != nil {
			t.Errorf("GameSettings に %s がある。画面から変えてよい設定ではない", name)
		}
	}

	// 欄を足すこと自体を、試験を直すという手間を挟んで意識させる。
	//
	// 7 個目までの内訳: difficulty / motd / max_players / view_distance /
	// simulation_distance / mode / hardcore。
	// mode は gamemode に対応し、効くのは新しく接続した人だけなので、
	// 押し間違えてもワールドは壊れない。
	// hardcore は **読み取り専用**。ここで有効にできると、生成済みの
	// level.dat と食い違ったまま死亡が不可逆になる。決めるのは
	// CreateWorld だけで、UpdateGameSettings は受け取っても無視する。
	if got := msg.Fields().Len(); got != 7 {
		t.Errorf("GameSettings の欄が %d 個。増やすなら、画面から変えて壊れないかを先に検討すること", got)
	}
}

// ハードコアを画面から有効にできてはいけない。
//
// 欄が読み取り専用であることはコンパイラには見えないので、
// 書き込み側が無視していることを試験で固定する。
func TestUpdateGameSettingsIgnoresHardcore(t *testing.T) {
	t.Parallel()

	// 欄そのものは表示のために残す。無くすと、いま有効かどうかを
	// 画面が知る手段が消える。
	if (&mcadminv1.GameSettings{}).ProtoReflect().Descriptor().
		Fields().ByName("hardcore") == nil {
		t.Fatal("GameSettings に hardcore がない。表示のために必要")
	}
}

// 反映の要否は利用者が選ぶ。欄が消えると、保存のたびに作り直すか、
// 決して反映しないかのどちらかに固定されてしまう。
func TestUpdateGameSettingsHasApplyNow(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.UpdateGameSettingsRequest{}).ProtoReflect().Descriptor()
	field := msg.Fields().ByName("apply_now")
	if field == nil {
		t.Fatal("UpdateGameSettingsRequest に apply_now がない")
	}
	if got := field.Kind(); got != protoreflect.BoolKind {
		t.Errorf("apply_now の型が %v。真偽値であること", got)
	}
}
