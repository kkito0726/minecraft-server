# 12. ゲーム設定の保存と反映

> [シーケンス一覧](README.md)　登場人物は一覧の「読み方」にある。
> 変更系の共通の骨格（[1. 変更系に共通する骨格](01-common-skeleton.md)）は省略している。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant RPC
    participant ユースケース as serverctl.Settings
    participant 操作管理
    participant rcon
    participant compose
    participant .env

    画面->>RPC: UpdateGameSettings(設定, apply_now)
    RPC->>RPC: 設定を検証（settings.New）

    alt apply_now が偽、またはサーバーが停止中
        ユースケース->>操作管理: 実行中の操作があるか
        Note over ユースケース,操作管理: あれば断る。切替や作成も .env を<br/>書き換えるため、割り込むと競合させる
        ユースケース->>.env: 5 項目を書く（操作にはしない）
        RPC-->>画面: 保存した設定（操作なし）
    else apply_now が真で、稼働中
        RPC->>操作管理: 操作を作る（SERVER_APPLY_SETTINGS）
        RPC-->>画面: Operation（すぐ返す）
        ユースケース->>.env: 1. 設定を書き込む
        Note over ユースケース,.env: 書き込みを先に行う。失敗しても<br/>まだ誰も切断していない
        ユースケース->>rcon: 2. save-all（停止中なら省略）
        ユースケース->>compose: 3. down → 停止を確認
        ユースケース->>compose: 4. up -d
        ユースケース->>compose: 5. 起動を確認
    end
```

**反映は作り直しでしか効かない。** `OVERRIDE_SERVER_PROPERTIES=TRUE` により、
起動のたびに `.env` から `server.properties` が作り直される。
**止まっているサーバーは起動しない。** 保存した設定は次の起動で反映される。
**変えられるのは 5 項目だけ。** バージョン・種別・メモリ・ワールド名は画面から変えない（REQ-420）。
