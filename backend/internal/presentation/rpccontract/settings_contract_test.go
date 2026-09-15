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
	if got := msg.Fields().Len(); got != 5 {
		t.Errorf("GameSettings の欄が %d 個。増やすなら、画面から変えて壊れないかを先に検討すること", got)
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
