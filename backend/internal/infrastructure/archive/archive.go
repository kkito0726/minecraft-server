// Package archive はバックアップの zip を作成・展開する。
//
// Raspberry Pi 5 (4GB) 上で MC が既に 2.7GB 使うため、内容を丸ごとメモリに
// 載せない。固定長のバッファで受け渡し、io.ReadAll は使わない。
//
// 圧縮は速度優先にしてある。ワールドの大半を占める .mca は内部で既に
// zlib 圧縮済みで、最大圧縮にしても縮まらず Pi の CPU を焼くだけになる。
//
// ファイル構成:
//   - create.go   アーカイブの作成
//   - extract.go  アーカイブの展開
//   - inspect.go  構造の読み取りと単一エントリの取り出し
//   - safepath.go エントリ名の検証（zip-slip 対策）
package archive

// copyBufferSize はストリーミングに使うバッファの大きさ。
const copyBufferSize = 32 * 1024

// Progress は進捗の通知。総量が不明な場合 total は 0。
type Progress func(done, total int64)

func (p Progress) report(done, total int64) {
	if p != nil {
		p(done, total)
	}
}
