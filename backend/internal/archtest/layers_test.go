// Package archtest は依存の向きを機械的に検証する。
//
// クリーンアーキテクチャの層は、規約を口で言うだけでは守られない。
// 「ちょっとここで proto の型を使えば早い」が一度通ると、
// 以降なし崩しになる。コンパイルは通ってしまうので、テストで縛る。
package archtest

import (
	"go/build"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/kkito0726/minecraft-server/backend"

// layerRule は「この層は、これら以外の内部パッケージを import してはいけない」。
type layerRule struct {
	layer string
	// allowedInternal は import してよい内部パッケージの接頭辞。
	allowedInternal []string
	// forbidden は層に関わらず持ち込んではいけないパッケージ。
	forbidden []string
	reason    string
}

func TestLayerDependencies(t *testing.T) {
	t.Parallel()

	rules := []layerRule{
		{
			layer:           "internal/domain",
			allowedInternal: []string{"internal/domain"},
			forbidden: []string{
				modulePath + "/gen",
				"connectrpc.com/connect",
				"net/http",
				"os/exec",
			},
			reason: "ドメインは外部を知らない。proto・Connect・プロセス実行を持ち込むと、" +
				"転送形式やインフラの変更がドメイン規則を壊す",
		},
		{
			layer:           "internal/application",
			allowedInternal: []string{"internal/domain", "internal/application"},
			forbidden: []string{
				modulePath + "/gen",
				"connectrpc.com/connect",
				"net/http",
				"os/exec",
			},
			reason: "ユースケースは port インターフェース越しにしか外部と話さない。" +
				"具体的な実装を知ると、テストに実 docker が必要になる",
		},
		{
			layer: "internal/infrastructure",
			allowedInternal: []string{
				"internal/domain", "internal/application", "internal/infrastructure",
			},
			forbidden: []string{
				modulePath + "/internal/presentation",
			},
			reason: "インフラは presentation を知らない",
		},
	}

	for _, rule := range rules {
		t.Run(rule.layer, func(t *testing.T) {
			t.Parallel()
			checkLayer(t, rule)
		})
	}
}

func checkLayer(t *testing.T, rule layerRule) {
	t.Helper()

	for _, pkg := range packagesUnder(t, rule.layer) {
		for _, imp := range pkg.imports {
			if violatesForbidden(imp, rule.forbidden) {
				t.Errorf("%s が %s を import している\n理由: %s", pkg.dir, imp, rule.reason)
				continue
			}
			if violatesLayer(imp, rule.allowedInternal) {
				t.Errorf("%s が %s を import している（%s の外）\n理由: %s",
					pkg.dir, imp, rule.layer, rule.reason)
			}
		}
	}
}

func violatesForbidden(imp string, forbidden []string) bool {
	for _, f := range forbidden {
		if imp == f || strings.HasPrefix(imp, f+"/") {
			return true
		}
	}
	return false
}

// violatesLayer は、モジュール内部の import が許可された層の外を指しているかを返す。
// 標準ライブラリと外部モジュールはここでは判定しない。
func violatesLayer(imp string, allowed []string) bool {
	rel, ok := strings.CutPrefix(imp, modulePath+"/")
	if !ok {
		return false
	}
	for _, a := range allowed {
		if rel == a || strings.HasPrefix(rel, a+"/") {
			return false
		}
	}
	return true
}

type pkgInfo struct {
	dir     string
	imports []string
}

// packagesUnder は指定したディレクトリ配下の Go パッケージを集める。
func packagesUnder(t *testing.T, layer string) []pkgInfo {
	t.Helper()

	root := filepath.Join("..", "..", layer)
	dirs := walkGoDirs(t, root)

	var out []pkgInfo
	for _, dir := range dirs {
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			// Go ファイルの無いディレクトリは飛ばす
			continue
		}
		// テストファイルの import は対象外。テストは層をまたいで検証することがある。
		out = append(out, pkgInfo{dir: dir, imports: pkg.Imports})
	}
	return out
}

func walkGoDirs(t *testing.T, root string) []string {
	t.Helper()

	var dirs []string
	err := filepathWalkDir(root, func(path string, isDir bool) error {
		if isDir {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%s を走査できない: %v", root, err)
	}
	return dirs
}
