# Paper とバニラのワールドの構成

Paper（サーバー）とバニラ（シングルプレイのクライアント）は、ワールドの
ディレクトリの中身が一部違う。バックアップをどちらで使うかによって、
置き場所を揃える必要がある。

この文書は実機で確認した事実だけを書く。確認日は 2026-10-11〜12、
Paper 26.2 build 123/133（結合テスト環境）と Paper 26.3 build 174（本番）、
クライアントは 26.3（DataVersion 5023）。

## 結論

| | Paper | バニラ |
| --- | --- | --- |
| 変換がいるか | — | **いる**（クライアントに変換器が無い） |
| 相手の形を読めるか | **読める**（起動時に自動で移行する） | 読めない（起動を中断する） |

**非対称である。** サーバー側は Paper が吸収してくれるが、クライアント側は
こちらで形を揃えるしかない。

## 違いは 5 ファイルの置き場所だけ

Paper はワールドごとに別の設定を持てるようにするため、次の 5 つを
**ディメンション側**に置く。バニラは**ワールド直下**しか見ない。

| ファイル | Paper | バニラ |
| --- | --- | --- |
| `world_gen_settings.dat` | `dimensions/minecraft/overworld/data/minecraft/` | `data/minecraft/` |
| `game_rules.dat` | 同上 | 同上 |
| `scheduled_events.dat` | 同上 | 同上 |
| `wandering_trader.dat` | 同上 | 同上 |
| `weather.dat` | 同上 | 同上 |

これ以外は同じ場所にある。

### 両方で同じ場所のもの

ワールド直下の `data/minecraft/`:

```
custom_boss_events.dat  random_sequences.dat  scoreboard.dat
stopwatches.dat         world_clocks.dat
```

ディメンション側の `data/minecraft/`:

```
chunk_tickets.dat  raids.dat  world_border.dat
ender_dragon_fight.dat（エンドのみ）
```

ワールド直下:

```
level.dat  level.dat_old  icon.png  datapacks/  players/
```

### Paper にしか無いもの

```
dimensions/minecraft/<ディメンション>/paper-world.yml
dimensions/minecraft/<ディメンション>/data/paper/level_overrides.dat
dimensions/minecraft/<ディメンション>/data/paper/metadata.dat
dimensions/minecraft/<ディメンション>/data/paper/persistent_data_container.dat
```

バニラは `paper` 名前空間を読まないので、残しておいても害がない。
`level.dat` の `DataPacks.Enabled` にも `paper` が入り、クライアントは
`Missing data pack paper` を警告するが、読み込みは続く。

## world_gen_settings.dat は全ディメンション分を持つ

移す元を**オーバーワールドに限る**のはこれが理由である。オーバーワールド側の
`world_gen_settings.dat` を NBT で読むと、3 つのディメンションすべての
生成器定義が入っている。

```
data.seed                           = -8880171749971551971
data.dimensions
  minecraft:overworld   type=minecraft:overworld   generator.settings=minecraft:overworld
  minecraft:the_nether  type=minecraft:the_nether  generator.settings=minecraft:nether
  minecraft:the_end     type=minecraft:the_end     generator.settings=minecraft:end
```

ネザーやエンド側のものを移すと、全体の設定を 1 ディメンション分で
上書きすることになる。

なお生成設定は `level.dat` には**入っていない**（`WorldGenSettings` という
欄は無い）。このファイルが唯一の情報源である。

## バニラに食わせるとどうなるか

5 つを移さずに `saves/` へ置くと、**ワールド一覧には出る**が読み込めない。

```
[Worker-Main-5/ERROR]: Unable to read or access the world gen settings file!
  Falling back to the default settings with a random world seed.
  .../saves/main/data/minecraft/world_gen_settings.dat
[Render thread/WARN]: Failed to load level data or datapacks, can't proceed with server load
java.lang.IllegalStateException: Overworld settings missing
```

シードも地形生成器も決められないため、既定値へフォールバックしようとして
止まる。一覧に出てしまうので原因が分かりにくい。

## Paper に食わせるとどうなるか

Paper は `VanillaWorldMigration` という**バニラからの取り込み**を持っている。
バニラの配置を検出すると、警告を出して 30 秒待ったあと、自分の配置へ
書き換えてから起動する。

```
[WorldFolderMigration] ===================== ! ALERT ! =====================
[WorldFolderMigration] World storage migration is required during startup.
[WorldFolderMigration] If you do not have a backup: interrupt the server now.
[WorldFolderMigration] =====================================================
[WorldFolderMigration] Continuing in 30 seconds...
[WorldFolderMigration] Continuing with startup world migration.
[VanillaWorldMigration] Starting Vanilla import for world 'X' (minecraft:overworld)
[VanillaWorldMigration] Completed Vanilla import for world 'X' (minecraft:overworld)
```

**バニラの配置は Paper にとって有効な入力だが、動作中の形式ではない。**
読む前に自分の形へ移してしまう。だからディスクにバニラの配置を置くたびに、
次の起動で移行が走る。

### 移行は無損失（実測）

結合テスト環境で、97MB・3 ディメンション・region 62 ファイルのワールドを
バニラの配置にして実物の Paper に食わせ、前後の指紋を照合した。

| 項目 | 結果 |
| --- | --- |
| 消えたファイル | **ゼロ**（移した 5 つが元の場所へ戻るだけ） |
| region ファイル 62 個 | **全て 1 バイトも変化なし**（sha256 で照合） |
| シード | 維持 |
| 生成設定の 3 ディメンション | 維持 |
| ネザー / エンド | 両方ロードされた |
| `level.dat` | `LastPlayed` / `Time` / `LevelName` / `Bukkit.Version` のみ変化。`DataVersion`・`spawn`・`singleplayer_uuid` は不変 |

`level.dat` の変化はサーバーが動いた分の通常の更新で、移行によるものではない。
**移行は 5 ファイルを動かすだけの、正確で可逆な操作である。**

### 30 秒は復元ごとに 1 回（実測）

| 配置 | 起動完了まで | 移行 |
| --- | --- | --- |
| Paper 配置 | 28 秒 / 24 秒 / 23 秒 | なし |
| バニラ配置（初回） | 58 秒 / 53 秒 | 走る |
| バニラ配置（移行後に再起動） | 24 秒 | なし |

ALERT の猶予はログで 2 回とも `23:57:12 → 23:57:42` のようにちょうど
30.0 秒だった。差分も約 30 秒で一致する。

**一度移行すればディスクは Paper の配置になるので、以降の再起動では
発生しない。** 30 秒を払うのは、バニラの配置をディスクに置いた直後の
1 回だけである。

`config/paper-global.yml` と `config/paper-world-defaults.yml` を見たが、
この猶予を無効化する設定は見つからなかった。

## 管理コンソールがどうしているか

保管するアーカイブは**バニラの配置**にしてある。理由は次のとおり。

- 直せない側（クライアント）には手が届かず、吸収できる側（サーバー）には
  手が届く。だから複雑さは吸収できる側に寄せる
- 利用者に「どちらの形か」を意識させない。保管が 1 形式なら、選ぶ場面が無い
- 移行が無損失で、30 秒が復元ごとに 1 回であることを実測で確かめた

復元では**両方の形を受け付ける**。過去に取った Paper 配置のアーカイブは
これまでどおり移行なしで戻る。

アーカイブの中身は `docs/backup-restore.md` の「バックアップ対象」を参照。
置き場所の変換規則は `backend/internal/domain/backup/vanilla.go` にあり、
そこが唯一の出典である。
