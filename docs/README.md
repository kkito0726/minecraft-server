# docs

このディレクトリに運用ナレッジを貯めていく。README.md には概要と入口だけを置き、
詳細な手順はここに切り出す。

| ドキュメント | 内容 |
|---|---|
| [raspberry-pi.md](raspberry-pi.md) | Pi 5 (4GB) のセットアップ、Tailscale、SSD 起動、スワップ、既定値の根拠、計測と切り分け、mcadmind の常駐 |
| [backup-restore.md](backup-restore.md) | ワールドのバックアップ取得と復元、ファイル名の規約、退避の後始末、バージョンとの関係、保管場所 |
| [runbook.md](runbook.md) | 日々の起動・停止・確認と障害対応、結合テスト環境の使い方 |
| [admin-console.md](admin-console.md) | 管理コンソール（mcadmind）の置き方と使い方。画面の操作と手作業の対応、困ったとき |
| [spec/admin-console/sequences.md](spec/admin-console/sequences.md) | ユースケースごとのシーケンス図。画面の操作が data/ と .env に届くまで |
| [spec/admin-console/](spec/admin-console/) | 管理コンソールの要件定義。EARS 記法の機能要件、ユーザストーリー、受け入れ基準、ヒアリング記録、実測ノート |
