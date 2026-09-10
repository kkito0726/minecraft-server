# pages

ルートに対応する画面。データ取得と organisms の配置を行う。

`ServerPage` (`/`) / `WorldsPage` (`/worlds`) / `BackupsPage` (`/backups`)。

どの画面もデータ取得と組み立てだけを行い、細かい見た目は organisms に置く。
復元だけは事前確認を取り直してからダイアログを出すため、その取得も pages が持つ。
