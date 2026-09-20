# 4. ワールドの切り替え

> [シーケンス一覧](README.md)　登場人物は一覧の「読み方」にある。
> 変更系の共通の骨格（[1. 変更系に共通する骨格](01-common-skeleton.md)）は省略している。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as worldctl
    participant rcon
    participant compose
    participant .env

    画面->>ユースケース: SwitchWorld(名前)
    ユースケース->>ユースケース: 名前を検証（world.NewName）

    alt 現在の MC_LEVEL と同じ
        ユースケース-->>画面: 何もせず成功（停止と起動を挟む理由がない）
    else 違う
        ユースケース->>rcon: 1. save-all（停止中なら省略）
        ユースケース->>compose: 2. down → MISSING を確認
        ユースケース->>.env: 3. MC_LEVEL を書き換え
        Note over ユースケース,.env: 行指向で値だけ差し替える。<br/>日本語コメントとクォートは一度も触らない
        ユースケース->>compose: 4. up -d
        ユースケース->>compose: 5. 起動を確認
    end
```

**停止は必須。** `LEVEL` は起動時に一度しか読まれない。
**切り替え前のワールドは 1 バイトも動かない**（`data/<名前>/` に残る）。
