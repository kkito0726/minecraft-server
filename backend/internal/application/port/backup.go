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

// ArchiveFile は読み出し用に開いたアーカイブ。
//
// Seek できる形で返す。HTTP の Range 要求に応えるために要る。
// バックアップは 200MB 規模になるため、転送が途中で切れたときに
// 最初からやり直させない。
type ArchiveFile struct {
	// Body はアーカイブの中身。呼び出し側が閉じる。
	Body io.ReadSeekCloser
	// Info は開いた時点の大きさと更新時刻。
	Info StoredBackup
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
	/*
		TriggersWorldMigration は、復元後の最初の起動で Paper の
		ワールド移行が走るかどうか。

		バニラの配置で保管したアーカイブ（現在の形式）では真になる。
		30 秒の足止めがあるので、復元の前に伝える必要がある。
	*/
	TriggersWorldMigration bool
}

/*
ArchiveImport は取り込み途中のアーカイブ。保管先の一時ファイルを指す。

中身を見てから受け取るかを決める必要があるため（ワールドが無い、
複数ある、名前が使えない）、「受け取る」と「確定する」を 2 段に
分けてある。確定しなければ何も増えない。
*/
type ArchiveImport interface {
	// LevelDatEntries はアーカイブ内の level.dat のエントリ名。
	// どのワールドが入っているかの判断材料になる。
	LevelDatEntries() []string
	// TotalBytes は展開後の合計サイズ。
	TotalBytes() int64
	// OpenEntry はエントリを展開せずに開く。版の読み取りに使う。
	OpenEntry(ctx context.Context, entry string) (io.ReadCloser, error)
	// Adopt は受け取ったアーカイブを、中身を触らずに保管先へ確定する。
	//
	// 復元が新旧どちらの形も受け付けるので、入口で包み直す必要がない。
	// 確定できた時点で一時ファイルは残らない。
	Adopt(ctx context.Context, id backup.ID) (StoredBackup, error)
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
	// Open は保管済みのアーカイブを読み出し用に開く。
	//
	// 中身をメモリに載せない。ダウンロードは 200MB 規模になるため、
	// 呼び出し側がそのまま流せる形で返す（REQ-415）。
	Open(ctx context.Context, id backup.ID) (ArchiveFile, error)
	// OpenVanilla は保管済みのアーカイブを、シングルプレイで読める形へ
	// 包み直して開く。
	//
	// 保管しているアーカイブには手を触れない。シングルプレイ用を
	// 別に保管はしない。同じワールドが二重に場所を取るため。
	OpenVanilla(ctx context.Context, id backup.ID) (ArchiveFile, error)
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
