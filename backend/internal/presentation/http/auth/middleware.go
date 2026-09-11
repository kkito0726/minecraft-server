package auth

import "net/http"

/*
Middleware は素の HTTP ハンドラを同じトークンで守る。

Connect の外側に置くエンドポイント（zip のアップロード）のために要る。
ブラウザは h2c へ昇格しないため client-streaming の RPC が使えず、
大きなファイルの受け取りだけは通常の POST で行う。守りが 2 系統に
分かれてしまうので、判定そのもの（check）は Interceptor と共有する。

守り漏れは静かに起きる。ここを通していないルートを足すと、
トークン無しで叩ける口がひとつ増えたことに誰も気づかない。
*/
func (i *Interceptor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := i.check(r.Header.Get("Authorization")); err != nil {
			// 本文は返さない。認証の前に内部の情報を出す理由が無い。
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "認証が必要です", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
