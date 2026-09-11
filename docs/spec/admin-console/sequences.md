# ユースケース別シーケンス

**作成日**: 2026-09-11
**対象**: 管理コンソール（`mcadmind`）
**関連**: [requirements.md](requirements.md) / [architecture.md](architecture.md) / [note.md](note.md)

画面の操作が、どの層を通って `data/` や `.env` に届くかを追う。
手順名は実装（`*Steps`）そのままなので、画面の帯に出る文字列と一致する。

---

## 0. 読み方

| 登場人物 | 実体 |
|---|---|
| **画面** | ブラウザ上の React。`features/*` のフック |
| **RPC** | `presentation/rpc` の Connect ハンドラ |
| **操作管理** | `application/operations.Manager`。排他とイベント列を持つ |
| **ユースケース** | `application/usecase/*`。手順の順序を持つ |
| **compose** | `docker compose`（`infrastructure/container/compose`） |
| **rcon** | `docker compose exec -T mc rcon-cli` |
| **.env** | `infrastructure/config/dotenv` |
| **data/** | ワールドの実体（`infrastructure/filesystem/worldfs`） |
| **backups/** | アーカイブの保管先（`infrastructure/persistence/backupfs`） |

**変更系はすべて同じ骨格を持つ。** RPC は操作を 1 つ作って**すぐ返し**、
本体は別の goroutine で走る。画面は返ってきた操作を購読して進捗を見る。

---

## 1. 変更系に共通する骨格

個別の図ではこの部分を省略する。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant RPC
    participant 操作管理
    participant ユースケース

    画面->>RPC: 変更系の RPC
    RPC->>ユースケース: 実行を依頼
    ユースケース->>操作管理: Start(種別, 手順名, 本体)
    操作管理->>操作管理: ロック取得（プロセス内 + data/.admin-console.lock）
    alt 既に実行中
        操作管理-->>画面: FailedPrecondition（実行中です）
    else 取得できた
        操作管理-)ユースケース: 本体を別 goroutine で実行
        Note over 操作管理,ユースケース: 本体は BaseCtx に紐づく。<br/>応答を返した時点で死なないようにするため
        操作管理-->>画面: Operation（開始時点のスナップショット）
    end

    画面->>画面: begin(operation) で即座に購読を開始
    画面->>RPC: WatchOperation(id, fromSeq=0)
    loop 手順が進むたび
        ユースケース->>操作管理: Step() / Logf() / Bytes()
        操作管理-->>画面: OperationEvent（seq + 完全なスナップショット）
    end
    操作管理-->>画面: 終端のイベント → ストリームを閉じる
    画面->>画面: 一覧と状態を無効化して取り直す
```

**押した直後から帯が出る**のは、応答に含まれる `Operation` をその場で
差し込んでいるから。`GetActiveOperation` の 5 秒ポーリングを待たない。

---

## 2. トークンによる認可

```mermaid
sequenceDiagram
    autonumber
    participant 利用者
    participant 画面
    participant 認証 as auth.Interceptor
    participant RPC

    利用者->>画面: 画面を開く
    画面->>画面: localStorage からトークンを読む

    alt 保存されている
        画面->>RPC: GetStatus（Authorization ヘッダを明示的に付与）
        RPC->>認証: check(ヘッダ)
        認証->>認証: 定数時間比較
        alt 一致
            認証-->>画面: 状態
            画面->>利用者: 画面を表示
        else 不一致
            認証-->>画面: Unauthenticated
            画面->>画面: トークンを破棄
            画面->>利用者: 入力を求める
        end
    else 未保存
        画面->>利用者: 入力を求める
        利用者->>画面: トークンを入力（32 文字以上）
        画面->>RPC: GetStatus（入力値をヘッダに直接指定）
        Note over 画面,RPC: 保存前なので interceptor 任せにはできない。<br/>ここを取り違えて「115 件のテストが通るのに<br/>ログインできない」状態を作ったことがある
        RPC-->>画面: 成功なら保存して入場
    end
```

ストリーム RPC も同じ interceptor を通る（`WrapStreamingHandler`）。
単項だけ実装すると `WatchOperation` が素通りする。

---

## 3. サーバーの起動・停止・再起動

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as serverctl
    participant rcon
    participant compose

    rect rgb(245, 245, 245)
        Note over 画面,compose: 起動（2 手順）
        画面->>ユースケース: StartServer
        ユースケース->>compose: up -d
        ユースケース->>compose: 起動を確認（logs の "Done (" と ps の healthy の OR）
    end

    rect rgb(245, 245, 245)
        Note over 画面,compose: 停止（2 手順）
        画面->>ユースケース: StopServer
        ユースケース->>rcon: save-all（停止中なら省略）
        ユースケース->>compose: down（stop_grace_period 60 秒を尊重）
    end

    rect rgb(245, 245, 245)
        Note over 画面,compose: 再起動（3 手順）
        画面->>ユースケース: RestartServer
        ユースケース->>compose: down
        ユースケース->>compose: up -d
        ユースケース->>compose: 起動を確認
        Note over ユースケース,compose: restart は使わない。<br/>.env の変更はコンテナの再作成でしか反映されない
    end
```

人が接続しているときの停止・再起動は、画面が一度確認を挟む。

---

## 4. ワールドの切り替え

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

---

## 5. ワールドの新規作成・複製・改名・削除

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as worldctl
    participant rcon
    participant compose
    participant .env
    participant data/

    rect rgb(245, 245, 245)
        Note over 画面,data/: 新規作成（5 手順）
        画面->>ユースケース: CreateWorld(名前, シード)
        ユースケース->>rcon: 1. save-all
        ユースケース->>compose: 2. down
        ユースケース->>.env: 3. MC_LEVEL と MC_SEED を書く
        ユースケース->>compose: 4. up -d
        ユースケース->>compose: 5. 生成を待つ（最大 900 秒）
        Note over ユースケース,data/: ディレクトリは作らない。Paper に生成させる
    end

    rect rgb(245, 245, 245)
        Note over 画面,data/: 複製（2 手順・サーバーは止めない）
        画面->>ユースケース: CloneWorld(元, 先)
        ユースケース->>ユースケース: 1. 空き容量（元のサイズ × 1.1）
        opt 稼働中
            ユースケース->>rcon: save-off → save-all
            Note over ユースケース,rcon: defer で save-on を必ず戻す
        end
        ユースケース->>data/: 2. コピー（session.lock は除く）
        ユースケース->>rcon: save-on
    end

    rect rgb(245, 245, 245)
        Note over 画面,data/: 改名（稼働中なら 6 手順、そうでなければ 1 手順）
        画面->>ユースケース: RenameWorld(旧, 新)
        opt 稼働中
            ユースケース->>compose: 保存 → down
        end
        ユースケース->>data/: 改名
        opt 稼働中
            ユースケース->>.env: MC_LEVEL も新しい名前に
            ユースケース->>compose: up -d → 起動を確認
        end
    end

    rect rgb(245, 245, 245)
        Note over 画面,data/: 削除（1 手順）
        画面->>ユースケース: DeleteWorld(名前, 入力した名前, 退避=true)
        ユースケース->>ユースケース: 稼働中なら拒否／名前の完全一致を要求
        ユースケース->>data/: data/<名前>.deleted-<日時> へ mv
        Note over ユースケース,data/: 既定は退避。完全な削除は退避一覧から改めて選ぶ
    end
```

---

## 6. バックアップの取得

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

---

## 7. 復元

**唯一の 2 段構え。** 事前確認は副作用を持たず、何度でも呼べる。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as backupctl
    participant backups/
    participant data/

    rect rgb(245, 245, 245)
        Note over 画面,data/: 事前確認（副作用なし・ダイアログを開くたび）
        画面->>ユースケース: PreflightRestore(id)
        ユースケース->>backups/: エントリ接頭辞からワールド名を判定
        ユースケース->>backups/: data/<名前>/level.dat だけを開いて版を読む
        Note over ユースケース,backups/: zip 全体は展開しない
        ユースケース->>data/: 稼働中のワールドの level.dat を読む
        ユースケース->>ユースケース: DataVersion どうしを比較（表示文字列は使わない）
        ユースケース-->>画面: 判定 / 名前の食い違い / 警告文 / 必要容量と空き
    end

    画面->>画面: 関門 2 つ（復元先の名前の完全一致 ＋ 警告への承諾）
```

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant ユースケース as backupctl
    participant compose
    participant data/
    participant backups/

    画面->>ユースケース: RestoreBackup(id, 復元先, 承諾, 入力した名前)

    ユースケース->>ユースケース: 1. 事前確認をやり直す
    Note over ユースケース: 画面の主張は信用しない。<br/>その画面が古い判定に基づいている可能性がある
    alt 承諾が足りない／名前が違う
        ユースケース-->>画面: 中止（まだ何も壊していない）
    end

    ユースケース->>ユースケース: 2. 空き容量（展開後 × 1.2）
    ユースケース->>compose: 3. down → MISSING を確認
    ユースケース->>data/: 4. data/<名前>.broken-<日時> へ mv
    Note over ユースケース,data/: 退避が展開より先。 これが最重要の不変条件。<br/>上書き展開だと古い地形と新しい地形が同居する
    ユースケース->>backups/: 5. 展開（data/ 配下のみ許可）

    alt 展開に失敗
        ユースケース->>data/: 展開途中を削除 → 退避から戻す
        ユースケース->>compose: up -d
        ユースケース-->>画面: 失敗（元の状態に戻っている）
    else 成功
        ユースケース->>ユースケース: 6. 復元先がアーカイブ側なら MC_LEVEL を更新
        ユースケース->>compose: 7. up -d → 起動を確認
        alt 起動に失敗
            ユースケース-->>画面: 失敗。巻き戻さず退避を残す（手で直す余地）
        else 成功
            ユースケース-->>画面: 成功（attributes に退避先）
        end
    end
```

退避は**自動で消さない**。`/worlds` の「退避したワールド」に出す。

---

## 8. zip の取り込み

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

---

## 9. 保持ポリシーの適用

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant RPC
    participant backups/

    画面->>RPC: PruneBackups(dry_run=true)
    RPC->>backups/: 一覧して選別
    RPC-->>画面: 削除の対象
    画面->>画面: 対象を並べて見せる
    画面->>RPC: PruneBackups(dry_run=false)
    RPC->>backups/: 削除
    RPC-->>画面: 実際に消えたもの
    Note over 画面: 予定ではなく実際に消えたものを出す。<br/>確認のあとに取得が走れば対象は変わる
```

世代数と日数の**両方**を満たさないものだけを削除し、
設定がどうであれ最も新しい 1 世代は必ず残す。

---

## 10. 中断の回復

この設計で**最も静かに壊れる**経路。`save-off` が残っても症状が出ない。

```mermaid
sequenceDiagram
    autonumber
    participant systemd
    participant 回復 as reconcile
    participant ロック as .admin-console.lock
    participant rcon
    participant 画面

    systemd->>回復: mcadmind 起動
    回復->>ロック: 中身を読む

    alt ロックがあり、取得したプロセスが既にいない
        回復->>回復: 中断として記録（save_disabled も残っている）
        回復->>ロック: 削除
        回復-->>画面: GetStatus に interrupted=true → 赤い帯
    end

    回復->>rcon: save-on を無条件に送る
    Note over 回復,rcon: RCON には「保存が有効か」を問う手段がない。<br/>冪等性に頼って送る。ログには何も出ない

    loop 10 秒ごと
        回復->>回復: ヘルスチェックを確認
        alt 非 healthy → healthy に変わった
            alt 生きているロックが「保存を止めている」と言っている
                回復->>回復: 送らない
                Note over 回復: バックアップの最中に送ると、<br/>zip を書いている途中で保存が再開され、<br/>静かに壊れたアーカイブができる
            else それ以外
                回復->>rcon: save-on を送り直す
            end
        end
    end
```

---

## 11. 再読み込みからの復帰

`WatchOperation(from_seq)` のリプレイが効いていることが前提。

```mermaid
sequenceDiagram
    autonumber
    participant 画面
    participant RPC
    participant 操作管理

    Note over 画面: 復元の最中に再読み込み（あるいは別の端末で開く）

    画面->>RPC: GetActiveOperation
    RPC-->>画面: 実行中の Operation
    画面->>RPC: WatchOperation(id, fromSeq=0)
    操作管理-->>画面: 記録済みのイベントを最初から再送
    Note over 画面,操作管理: これが無いと、再読み込み直後の画面が<br/>スピナーだけになる
    操作管理-->>画面: 以降は追従

    alt ストリームが切れた
        画面->>画面: 1 秒から倍々（上限 15 秒）で待つ
        画面->>RPC: WatchOperation(id, fromSeq=最後に受け取った seq)
    end

    alt 正常に閉じたが、まだ終端に達していない
        画面->>RPC: 続きから取り直す
        Note over 画面,RPC: 「閉じた＝終わった」と決めつけると、<br/>操作は終わっているのに画面が実行中のまま固まり、<br/>変更系のボタンが全部押せなくなる
    else 終端のイベントを受け取った
        画面->>画面: 購読を終える
    end

    loop 購読していない間だけ 5 秒ごと
        画面->>RPC: GetActiveOperation
        Note over 画面,RPC: 別のタブ・別の端末・systemd の回復処理で<br/>始まった操作に気づくための経路
    end
```
