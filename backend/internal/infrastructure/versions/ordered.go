package versions

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// orderedVersions は JSON のオブジェクトを、書かれた順のまま読む。
//
// Go の map に読むと順序が失われ、系列の新旧が崩れる。Fill API は
// 新しい系列から順に書いているので、その順序をそのまま使いたい。
type orderedVersions [][]string

// UnmarshalJSON は {"26.3": [...], "26.2": [...]} を順に読む。
func (o *orderedVersions) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))

	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return fmt.Errorf("versions がオブジェクトではありません")
	}

	var out orderedVersions
	for dec.More() {
		// 系列の名前は使わない。中の版の名前に全て含まれている。
		if _, err := dec.Token(); err != nil {
			return err
		}
		var group []string
		if err := dec.Decode(&group); err != nil {
			return fmt.Errorf("versions の中身を読めません: %w", err)
		}
		out = append(out, group)
	}
	if _, err := dec.Token(); err != nil {
		return err
	}

	*o = out
	return nil
}
