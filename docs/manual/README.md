# 利用者向けドキュメント

このディレクトリには、`skr` の利用者向けドキュメントをまとめます。API を使った具体的な操作手順は `tutorials/` に、`--zone` やプロファイルなど複数のコマンドに関係する機能の解説はこのディレクトリ直下に配置します。

## チュートリアル

- [IaaS Archive API: ゾーン内のアーカイブを検索・参照する](tutorials/iaas-api/archive.md)
- [IaaS Disk API: ディスクを作成・確認・削除する](tutorials/iaas-api/disk.md)
- [IaaS DiskPlan API: ゾーン別のディスクプランとサイズを確認する](tutorials/iaas-api/disk-plan.md)
- [IaaS Internet API: ルータ＋スイッチに Server を接続して SSH 接続する](tutorials/iaas-api/internet.md)
- [IaaS Icon API: 画像ファイルからユーザー作成アイコンを管理する](tutorials/iaas-api/icon.md)
- [IaaS PacketFilter API: ルールを設定・管理する](tutorials/iaas-api/packetfilter.md)
- [IaaS Region API: リージョン情報を検索・参照する](tutorials/iaas-api/region.md)
- [IaaS ServiceClass API: ゾーン内のサービスクラスを検索する](tutorials/iaas-api/serviceclass.md)
- [IaaS Server API: cloud-init でサーバを起動して SSH 接続する](tutorials/iaas-api/server.md)
- [IaaS ServerPlan API: サーバプランを検索・参照する](tutorials/iaas-api/server-plan.md)
- [IaaS Switch API: Sandbox でスイッチの管理手順を確認する](tutorials/iaas-api/switch.md)
- [IaaS Zone API: ゾーン一覧を jq で加工する](tutorials/iaas-api/zone.md)
- [GSLB API: 実サーバーを監視して DNS 応答先を制御する](tutorials/gslb-api.md)
- [SimpleMQ API: キューを作成してメッセージを送受信する](tutorials/simplemq-api.md)
- [Simple Notification API: グループに通知を送り、履歴を確認する](tutorials/simple-notification-api.md)
- [Container Registry API: レジストリと認証ユーザーを管理する](tutorials/container-registry-api.md)
- [EventBus API: スイッチ作成イベントを SimpleMQ で受信する](tutorials/eventbus-api.md)
- [IAM API: 組織と IAM リソースを管理する](tutorials/iam-api.md)
- [Networking Suite API: 別サブネット間の通信を確認する](tutorials/networking-suite-api.md)
- [HTTP API: `get_zone` でゾーン一覧を取得する](tutorials/http-request.md)

## 共通機能

- [プロファイルの管理](c0nfig.md)
- [API 出力の jq 加工](query.md)
- [認証付き HTTP リクエスト](http-request.md)
