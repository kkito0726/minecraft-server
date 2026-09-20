# ユースケース別シーケンス

**作成日**: 2026-09-11
**対象**: 管理コンソール（`mcadmind`）
**関連**: [requirements.md](../spec/admin-console/requirements.md) / [architecture.md](../spec/admin-console/architecture.md) / [note.md](../spec/admin-console/note.md)

画面の操作が、どの層を通って `data/` や `.env` に届くかを追う。
手順名は実装（`*Steps`）そのままなので、画面の帯に出る文字列と一致する。

1 ユースケースにつき 1 ファイルに分けてある。読む順に並べているが、
どれも単独で読める。

| シーケンス |
|---|
| [1. 変更系に共通する骨格](01-common-skeleton.md) |
| [2. トークンによる認可](02-authorization.md) |
| [3. サーバーの起動・停止・再起動](03-server-lifecycle.md) |
| [4. ワールドの切り替え](04-world-switch.md) |
| [5. ワールドの新規作成・複製・改名・削除](05-world-manage.md) |
| [6. バックアップの取得](06-backup-create.md) |
| [7. 復元](07-backup-restore.md) |
| [8. zip の取り込み](08-backup-import.md) |
| [9. 保持ポリシーの適用](09-backup-prune.md) |
| [10. 中断の回復](10-interruption-recovery.md) |
| [11. 再読み込みからの復帰](11-watch-resume.md) |
| [12. ゲーム設定の保存と反映](12-game-settings.md) |
| [13. バックアップの持ち出し（ダウンロード）](13-backup-download.md) |

---

## 読み方

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
