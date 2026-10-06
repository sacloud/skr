# 利用者向けドキュメント

このディレクトリには、`skr` の利用者向けドキュメントをまとめます。API を使った具体的な操作手順は `tutorials/` に、`--zone` やプロファイルなど複数のコマンドに関係する機能の解説はこのディレクトリ直下に配置します。

## チュートリアル

- [IaaS Disk API: ディスクを作成・確認・削除する](tutorials/iaas-api/disk.md)
- [IaaS Server API: サーバを作成・確認・削除する](tutorials/iaas-api/server.md)
- [IaaS Switch API: Sandbox でスイッチの管理手順を確認する](tutorials/iaas-api/switch.md)
- [IaaS Zone API: ゾーン一覧を jq で加工する](tutorials/iaas-api/zone.md)
- [SimpleMQ API: キューを作成してメッセージを送受信する](tutorials/simplemq-api.md)
- [EventBus API: スイッチ作成イベントを SimpleMQ で受信する](tutorials/eventbus-api.md)
- [HTTP API: `get_zone` でゾーン一覧を取得する](tutorials/http-request.md)

## 共通機能

- [API 出力の jq 加工](query.md)
- [認証付き HTTP リクエスト](http-request.md)
