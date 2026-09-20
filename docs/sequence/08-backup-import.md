# 8. zip の取り込み

> [シーケンス一覧](README.md)　登場人物は一覧の「読み方」にある。

Connect の外側にある唯一の経路。ブラウザが平文 HTTP/2 へ昇格しないため、
client-streaming の RPC が使えない。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant 認証 as auth.Middleware
    participant ハンドラ as upload.Handler
    participant ユースケース as backupctl
    participant backups/

    画面->>認証: POST /upload/backup（本文は zip そのもの）
    認証->>ハンドラ: トークンを確認して通す

    ハンドラ->>ユースケース: Import(本文, Content-Length)
    ユースケース->>ユースケース: 空き容量を確認（包み直しのため 2 倍を見込む）
    ユースケース->>backups/: 一時ファイルへ流す（上限で打ち切る）
    Note over backups/: 拡張子を付けない。付けると<br/>取り込み中の壊れたアーカイブが一覧に出る
    ユースケース->>ユースケース: level.dat の位置から形を判断

    alt data/<名前>/level.dat
        ユースケース->>backups/: そのまま rename
    else <名前>/level.dat（配布ワールド）
        ユースケース->>backups/: data/ 配下へ包み直してから rename
        Note over ユースケース,backups/: 展開側の「data/ 配下のみ」は緩めない。<br/>入口で形を揃える
    else 判断できない
        ユースケース-->>画面: 理由を返す（フォルダに入れて／複数ある／ワールドが無い）
    end

    ユースケース->>ユースケース: アーカイブ内の level.dat から版を読む
    ユースケース-->>画面: id / ワールド名 / 版 / 包み直したか
    画面->>画面: 一覧を取り直す
```

**取り込みは操作にしない。** `data/` を触らないので排他の枠を消費する
理由がなく、待たせると取り込み中に他の操作ができなくなる。
差し替えるかどうかは、このあと復元の関門を通して決める。
