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

/*
ArchiveImport は取り込み途中のアーカイブ。保管先の一時ファイルを指す。

外から持ち込まれた zip は、まだ data/ 配下に無いかもしれない。
中身を見てから置き場所を決める必要があるため、「受け取る」と
「確定する」を 2 段に分けてある。確定しなければ何も増えない。
*/
type ArchiveImport interface {
	// LevelDatEntries はアーカイブ内の level.dat のエントリ名。
	// どのワールドが入っているかの判断材料になる。
	LevelDatEntries() []string
	// TotalBytes は展開後の合計サイズ。
	TotalBytes() int64
	// OpenEntry はエントリを展開せずに開く。版の読み取りに使う。
	OpenEntry(ctx context.Context, entry string) (io.ReadCloser, error)
	// Adopt は prefix を前置して保管先へ確定する。
	//
	// prefix が空なら中身を触らずに置く。空でなければ data/ 配下へ
	// 包み直してから置く。確定できた時点で一時ファイルは残らない。
	Adopt(ctx context.Context, id backup.ID, prefix string) (StoredBackup, error)
	// Discard は一時ファイルを捨てる。確定済みなら何もしない。
	//
	// 失敗しても呼び出し側にできることは無いので、エラーを返さない。
	Discard()
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
	// Stage は受け取ったバイト列を保管先の一時ファイルへ書き、中身を調べる。
	//
	// maxBytes を超えたら打ち切ってエラーにする。ディスクを埋めきる前に
	// 止めるためで、Pi では空き容量がそのままサーバーの生死に関わる。
	Stage(ctx context.Context, src io.Reader, maxBytes int64) (ArchiveImport, error)
	// Extract はアーカイブを data/ の親ディレクトリへ展開する。
	//
	// rewriteLevel が空でなければ、アーカイブ内のワールド名を
	// その名前へ書き換えて展開する。利用者が別のワールドへ
	// 切り替えている場合に、稼働中のワールドを置き換えるために使う。
	//
	// 危険なエントリが 1 つでもあれば 1 バイトも書かずに拒否する。
	Extract(ctx context.Context, id backup.ID, rewriteLevel string, progress Progress) error
}
