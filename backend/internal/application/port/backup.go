package port

import (
	"context"
	"io"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// StoredBackup は保管済みのアーカイブ 1 つ。ファイルとしての事実だけを持つ。
//
// バージョンやワールド名はここに入れない。それらはアーカイブの中身を
// 読まないと分からず、一覧のたびに全件を開くのは Pi では重いため、
// 必要になった時点で Inspect / OpenLevelDat を呼ぶ。
type StoredBackup struct {
	ID        backup.ID
	SizeBytes int64
	// CreatedAt はファイルの更新時刻。ファイル名の日時とは別の事実。
	CreatedAt time.Time
}

// ArchiveInfo はアーカイブを展開せずに読み取った構成。
type ArchiveInfo struct {
	// Level はエントリの接頭辞から判定したワールド名。判定できなければ空。
	Level string
	// EntryRoots は含まれるルート（data/world、data/plugins など）。
	EntryRoots []string
	// TotalBytes は展開後の合計サイズ。
	TotalBytes int64
	// HasLevelDat は level.dat のエントリが存在するか。
	HasLevelDat bool
}

// BackupStore はアーカイブの保管先。
//
// zip の作り方も保管先の場所も infrastructure の関心事であり、
// ユースケースは「対象のワールドと名前を渡すと 1 件増える」ことだけを知る。
type BackupStore interface {
	// Directory は保管先のパスを返す。表示にのみ使う。
	Directory() string
	// List は保管済みのアーカイブを新しい順で返す。
	//
	// 解釈できないファイル（作成途中の一時ファイルなど）は
	// エラーにせず読み飛ばす。1 つの壊れたファイルで一覧が
	// 空になってはならない。
	List(ctx context.Context) ([]StoredBackup, error)
	// Create はアーカイブを作る。
	//
	// 一時ファイルへ書いてから名前を変更する。途中で失敗した場合、
	// 中途半端なアーカイブが List に現れてはならない。
	Create(ctx context.Context, id backup.ID, level world.Name, progress Progress) (StoredBackup, error)
	// Delete はアーカイブを削除し、解放されたバイト数を返す。
	Delete(ctx context.Context, id backup.ID) (freedBytes int64, err error)
	// Inspect はアーカイブの構成を、展開せずに読む。
	Inspect(ctx context.Context, id backup.ID) (ArchiveInfo, error)
	// OpenLevelDat はアーカイブ内の level.dat を、展開せずに開く。
	OpenLevelDat(ctx context.Context, id backup.ID) (io.ReadCloser, error)
	// Extract はアーカイブを data/ の親ディレクトリへ展開する。
	//
	// rewriteLevel が空でなければ、アーカイブ内のワールド名を
	// その名前へ書き換えて展開する。利用者が別のワールドへ
	// 切り替えている場合に、稼働中のワールドを置き換えるために使う。
	//
	// 危険なエントリが 1 つでもあれば 1 バイトも書かずに拒否する。
	Extract(ctx context.Context, id backup.ID, rewriteLevel string, progress Progress) error
}
