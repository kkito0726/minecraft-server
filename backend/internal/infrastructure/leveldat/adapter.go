package leveldat

import (
	"context"
	"io"
	"path/filepath"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// Adapter は port.LevelReader の実装。
//
// 読めなかった場合はエラーではなく「読めない」を表す WorldVersion を返す。
// 破損したワールドが 1 つあっても一覧そのものが使えなくなってはいけない。
type Adapter struct {
	dataDir string
}

// NewAdapter は data/ を基準にする Adapter を作る。
func NewAdapter(dataDir string) *Adapter { return &Adapter{dataDir: dataDir} }

// ReadWorld は data/<名前>/level.dat を読む。
func (a *Adapter) ReadWorld(_ context.Context, name world.Name) shared.WorldVersion {
	v, _, err := a.ReadFile(filepath.Join(a.dataDir, name.String(), "level.dat"))
	if err != nil {
		return shared.UnreadableWorldVersion()
	}
	return v
}

// Read は任意の入力から読む。アーカイブ内の level.dat に使う。
func (a *Adapter) Read(_ context.Context, r io.Reader) shared.WorldVersion {
	info, err := Read(r)
	if err != nil {
		return shared.UnreadableWorldVersion()
	}
	return toWorldVersion(info)
}

// ReadFile はパスを指定して読む。最終プレイ日時も返す。
func (a *Adapter) ReadFile(path string) (shared.WorldVersion, time.Time, error) {
	info, err := ReadFile(path)
	if err != nil {
		return shared.UnreadableWorldVersion(), time.Time{}, err
	}
	return toWorldVersion(info), info.LastPlayed, nil
}

func toWorldVersion(info Info) shared.WorldVersion {
	dv, err := shared.NewDataVersion(info.DataVersion)
	if err != nil {
		// DataVersion が読めなければバージョン比較ができない。
		// 「読めない」として扱い、復元時に承諾を求める。
		return shared.UnreadableWorldVersion()
	}
	return shared.NewWorldVersion(info.VersionName, dv, info.Snapshot, info.LevelName)
}

var _ port.LevelReader = (*Adapter)(nil)
