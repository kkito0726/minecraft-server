# admin-console アーキテクチャ

**作成日**: 2026-09-10
**対象**: 管理コンソール（`mcadmind`）
**関連**: [requirements.md](requirements.md) / [sequences.md](sequences.md) / [note.md](note.md)

バックエンドは **DDD + クリーンアーキテクチャ**、フロントエンドは **Atomic Design** で構成する。

---

## 1. なぜこの構成にするか

この管理コンソールには、外部に出さないと守れない**本物のドメイン規則**がいくつかある。

| 規則 | 破ったときに起きること | 該当要件 |
|---|---|---|
| ワールド名は `^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$` で予約名を除く | 不正な名前がファイルシステムに到達する。フロントとバックで規則がずれると片方だけ緩む | REQ-122, NFR-304 |
| バージョン比較は `DataVersion`（整数）で行う | 表示文字列が同じ `"26.2"` でも中身が違うワールドを取り違える。アップグレードは片道で戻せない | REQ-412 |
| 保持ポリシーは最低 1 世代を必ず残す | 設定ミスでバックアップが全損する | EDGE-103 |
| 操作は同時に 1 つだけ。状態は PENDING → RUNNING → 終端の一方向 | `data/` の同時変更で破損する | REQ-101 |
| 復元は「退避してから展開」の順序 | 古い地形と新しい地形が同居した壊れたワールドになる | REQ-114 |

これらは「docker をどう呼ぶか」「zip をどう読むか」とは独立した規則である。
インフラの都合に混ぜると、たとえば「`docker compose` の呼び出しを変えたらバージョン判定が壊れた」
といった事故が起きうる。ドメイン層に隔離し、**外部を一切知らない状態でテストできる**ようにする。

**間接層は増える。** 単一利用者の管理画面としては厚い構成であることは認識したうえで、
上記の規則の重要度（間違えるとワールドが失われる）に対して妥当な投資と判断した。

---

## 2. バックエンド: レイヤ構成

```
backend/
├── cmd/mcadmind/                起動と依存の組み立てだけ
└── internal/
    ├── domain/                  ← 何にも依存しない
    │   ├── world/               ワールドの集約・値オブジェクト・リポジトリIF
    │   ├── backup/              バックアップの集約・保持ポリシー・バージョン判定
    │   ├── operation/           操作の集約・状態遷移・イベント列
    │   ├── server/              サーバー状態の値オブジェクト
    │   └── shared/              層をまたぐ値オブジェクト（DataVersion など）
    │
    ├── application/             ← domain のみに依存
    │   ├── port/                外部システムへの出口（IF のみ）
    │   └── usecase/             ユースケース。手順の順序を持つ
    │
    ├── infrastructure/          ← domain / application の IF を実装
    │   ├── config/dotenv/       .env の読み書き
    │   ├── leveldat/            level.dat の NBT 読み取り
    │   ├── archive/             zip の作成・展開
    │   ├── container/compose/   docker compose の実行
    │   ├── console/rcon/        rcon-cli の実行
    │   ├── filesystem/          ワールドディレクトリの操作
    │   └── persistence/         操作の保持（メモリ）とロックファイル
    │
    └── presentation/            ← application に依存
        ├── rpc/                 Connect ハンドラ。proto ↔ ドメインの変換
        ├── http/                HTTP サーバー・認証・静的配信
        └── webui/               フロントエンドの埋め込み
```

### 依存の向き

```
presentation ─┐
              ├─→ application ─→ domain
infrastructure ┘
```

**domain は何も import しない**（標準ライブラリを除く）。とくに以下を持ち込まない。

- 生成された proto の型（`gen/mcadmin/v1`）
- Connect / HTTP
- `os` / `os/exec`（ファイルシステムやプロセスの操作）

proto の型はあくまで**転送の形式**であり、ドメインの表現ではない。
`presentation/rpc` が両者を変換する。この変換があるおかげで、
proto を変えてもドメインが壊れず、その逆も成り立つ。

### 依存の逆転

`application/port` に「外部にこうしてほしい」というインターフェースを置き、
`infrastructure` がそれを実装する。ユースケースは実装を知らない。

```go
// application/port — ユースケースが必要とするもの
type ContainerRuntime interface {
    Up(ctx context.Context, sink LogSink) error
    Down(ctx context.Context, sink LogSink) error
    Status(ctx context.Context) (server.ContainerStatus, error)
    WaitReady(ctx context.Context, timeout time.Duration) error
}

// infrastructure/container/compose — その実装
type Runner struct { /* docker compose を叩く */ }
```

これによって、ユースケースのテストは実際の docker を必要としない。
「復元で `Quarantine` が `Extract` より前に呼ばれること」のような**順序の検証**が、
偽の実装への呼び出し記録だけで書ける。

---

## 3. ドメインモデル

### 値オブジェクト（不変・自己検証）

| 型 | 不変条件 | 置き場所 |
|---|---|---|
| `world.Name` | 正規表現に一致し、予約名・退避名でないこと | `domain/world` |
| `shared.DataVersion` | 正の整数 | `domain/shared` |
| `shared.WorldVersion` | `readable` が偽なら他は未設定 | `domain/shared` |
| `backup.ID` | ファイル名。ディレクトリ区切りを含まない | `domain/backup` |
| `backup.RetentionPolicy` | `keepCount >= 1` | `domain/backup` |
| `operation.ID` | ULID | `domain/operation` |

不正な値では**そもそも生成できない**ようにする（`New...` がエラーを返す）。
生成できた時点で以降の検証が不要になり、「検証を忘れた経路」が構造的に無くなる。

```go
// これはコンパイルは通るが実行時に必ず失敗する
name, err := world.NewName("../etc")  // err != nil
```

### 集約

- **`operation.Operation`** — 状態遷移とイベント列の単調性を守る。
  `Advance()` / `Log()` / `Succeed()` / `Fail()` を通してのみ変化し、
  終端状態からは動かない。イベントの `seq` は 1 始まりで単調増加する。
- **`world.World`** — 名前・稼働中か・サイズ・最終プレイ・バージョン。
- **`backup.Backup`** — アーカイブの実体に対応する。バージョンは**アーカイブ内の
  `level.dat` から読んだ値**が正であり、ファイル名から推測した値は参考にすぎない。

### ドメインサービス

集約ひとつに収まらない規則を置く。

- **`backup.CompareVersions(archive, current) Verdict`**
  `MATCH` / `OLDER_WILL_UPGRADE` / `NEWER_INCOMPATIBLE` / `UNKNOWN` を返す。
  比較は `DataVersion` どうしで行い、`MC_VERSION` の文字列は使わない。
- **`backup.SelectForDeletion(backups, policy) []ID`**
  世代数と日数の両方を満たさないものを選ぶ。**最低 1 世代は必ず残す。**

---

## 4. フロントエンド: Atomic Design

```
frontend/src/
├── components/
│   ├── atoms/       これ以上分割できない部品。状態も業務知識も持たない
│   ├── molecules/   atoms を 2〜3 個組み合わせた最小の意味単位
│   ├── organisms/   画面の一区画。業務上の意味を持つ
│   └── templates/   配置のみを決める骨組み。実データを持たない
├── pages/           ルートに対応。データ取得と organisms の配置
├── features/        コンポーネント以外の関心事（フック・スキーマ・変換）
├── lib/             通信・書式・共通ユーティリティ
└── gen/             proto から生成（対象外）
```

### 各層の判断基準

| 層 | 持ってよいもの | 持ってはいけないもの | 例 |
|---|---|---|---|
| **atoms** | 見た目、`disabled` 等の UI 状態 | 業務知識、API、`useQuery` | `Button` `Badge` `Spinner` `TextInput` |
| **molecules** | atoms の組み合わせ、局所的な状態 | API 呼び出し | `FormField` `ConfirmInput` `StatItem` |
| **organisms** | 業務上の意味、フックの利用 | ルーティング | `WorldTable` `OperationBanner` `RestoreDialog` |
| **templates** | 配置 | 実データ | `AppShell` `PageLayout` |
| **pages** | データ取得、organisms の配置 | 細かい見た目 | `ServerPage` `WorldsPage` |

**`features/` を分けている理由**: Atomic Design はコンポーネントの分類法であって、
フック・zod スキーマ・型変換の置き場所を定めていない。これらを atoms/molecules に混ぜると
「コンポーネントでないものが components/ にある」状態になるため、隣に分ける。

### atoms に業務知識を持ち込まない

```tsx
// ✗ atoms が業務を知っている
<VersionBadge version={world.version} />

// ✓ atoms は見た目だけ。意味づけは molecules 以上で行う
<Badge tone="warning">{formatWorldVersion(world.version)}</Badge>
```

これを守ると、atoms は proto の型を import しなくなり、
生成コードの変更がデザイン部品に波及しなくなる。

---

## 5. 検証の分担

| 層 | 何を検証するか | 外部依存 |
|---|---|---|
| `domain` | 不変条件と規則。値オブジェクトの生成可否、バージョン判定、保持ポリシー、状態遷移 | なし。純粋なユニットテスト |
| `application` | 手順の**順序**。「退避が展開より前」「失敗しても `save-on` が呼ばれる」 | port の偽実装（呼び出しを記録） |
| `infrastructure` | 実物との相互作用。実ファイル・実 zip・実 `level.dat` | 実 FS。docker はスタブ実行ファイル |
| `presentation` | proto ↔ ドメインの変換、認証 | Connect のテストサーバー |
| フロント | reducer の純関数、zod の境界値、コンポーネントの表示 | フェイクトランスポート |
| E2E | 利用者から見た振る舞い | 実バイナリ + 偽 docker |

ドメイン層が外部を知らないことが、カバレッジ 80% を実機なしで達成できる根拠になっている。

---

## 6. この構成で守られること

- **ワールド名の規則がフロントとバックで一致する。** 値オブジェクトの正規表現を単一の出典とし、両方にその一致を確かめるテストを置く（NFR-304）
- **バージョン判定が表示文字列に引きずられない。** `DataVersion` を型として持つため、文字列との比較はコンパイルが通らない
- **保持ポリシーの誤設定で全損しない。** `keepCount >= 1` が型の不変条件
- **復元の手順の順序が壊れない。** ユースケースのテストが呼び出し順を検証する
- **docker の呼び方を変えてもドメインが壊れない。** 依存の向きが一方向
