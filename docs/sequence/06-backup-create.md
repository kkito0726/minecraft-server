# 6. バックアップの取得

> [シーケンス一覧](README.md)　登場人物は一覧の「読み方」にある。
> 変更系の共通の骨格（[1. 変更系に共通する骨格](01-common-skeleton.md)）は省略している。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as backupctl
    participant rcon
    participant compose
    participant backups/

    alt HOT（既定・3 手順）
        画面->>ユースケース: CreateBackup(HOT, メモ)
        ユースケース->>ユースケース: ロックに SaveDisabled=true を記録
        Note over ユースケース: 記録は save-off の前。<br/>逆順だと、その隙間で落ちたとき<br/>「保存は正常」と誤って残る
        ユースケース->>rcon: 1. save-off → save-all
        ユースケース->>backups/: 2. .part へストリーム書き込み → fsync → rename
        ユースケース->>rcon: save-on（失敗しても必ず通る defer）
        ユースケース->>backups/: 3. 保持ポリシーを適用
    else COLD（4 手順）
        画面->>ユースケース: CreateBackup(COLD, メモ)
        ユースケース->>compose: 1. down
        ユースケース->>backups/: 2. アーカイブを作成
        ユースケース->>compose: 3. up -d
        ユースケース->>backups/: 4. 保持ポリシーを適用
    end
```

対象は `data/<MC_LEVEL>/` `data/plugins/` `data/config/` `data/bukkit.yml`
`data/spigot.yml` の 5 つ。**`server.properties` は含めない**
（`rcon.password` と `management-server-secret` を平文で持つ）。

ファイル名は `backup-<版>-<ワールド名>-<日時>[-<メモ>].zip`。
