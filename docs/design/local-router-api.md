# Local Router API コマンド設計

## 範囲と配置

`local-router-api` は `service/iaas/localrouter` (service 層) と `api/iaas.LocalRouterAPI` (API 層) を組み合わせます。ローカルルータは CommonServiceItem を使うグローバルリソースのため、`iaas-api` のゾーン指定型リソースとは別のルートコマンドにします。パッケージは `internal/localrouterapi`、CLI の SDK client factory と Runtime 接続は `internal/cli/localrouter.go` に置きます。設計判断は [ADR 0084](../adr/0084-local-router-api-commands.md) に記録しています。

service 層は Find / Read / Create / Update / Delete / Health / MonitorLocalRouter / Apply を公開します。本コマンドはこのうち Find / Read / Create / Update / Delete / Health / MonitorLocalRouter を利用し、Apply は公開しません。設定だけを更新する `update-settings` は service 層に存在しないため、API 層の `LocalRouterAPI.UpdateSettings` を直接呼び出します。マージ処理は現在値を読み込むアダプターとして `internal/localrouterapi` に実装します。

## コマンド構成

以下のコマンドは `skr local-router-api` に続けて指定します。ローカルルータはグローバルリソースで、ゾーンの指定は不要です。

| コマンド | SDK 操作 | 入力・出力 |
| --- | --- | --- |
| `find [--name NAME] [--tags TAG,TAG] [--count N] [--from N] [--request JSON]` | `FindWithContext` | 任意の検索条件。結果はローカルルータ配列です |
| `create --name NAME [--description DESC] [--tags TAG,TAG] [--icon-id ID] [接続フラグ]` または `create --request JSON` | `CreateWithContext` | 個別フラグまたは作成 JSON。結果はローカルルータです |
| `read --id ID` | `ReadWithContext` | ローカルルータ ID。結果はローカルルータです。ピアの `SecretKeys` を含みます |
| `update --id ID [スカラーと接続フラグ]` または `update --request JSON` | `UpdateWithContext` | 部分更新。未指定は現状維持。結果はローカルルータです |
| `update-settings --id ID [接続フラグ]` または `update-settings --id ID --request JSON` | `LocalRouterAPI.UpdateSettings` (skr マージアダプター) | 設定のみの部分更新。`--id` は常に必須で、`--request` と併用できます。未指定は現状維持。結果はローカルルータです |
| `delete --id ID [--fail-if-not-found]` | `DeleteWithContext` | 成功時の出力はありません |
| `health-status --id ID` | `HealthWithContext` | ピアごとの稼働状態と学習したルートです |
| `monitor-local-router --id ID [--start RFC3339] [--end RFC3339]` | `MonitorLocalRouterWithContext` | 送受信バイト/秒の推移。未指定時は直近 1 時間です |

## リクエストと秘密情報

Create / Update / UpdateSettings では個別の設定フラグと `--request` を相互排他で受け付けます。`update-settings` の `--id` は対象リソースの指定であり設定フラグではないため、`--request` とも併用できます。リクエスト JSON に `ID` を含めた場合、`--id` と一致しない入力は拒否します。`Peers[].SecretKey` は秘密情報のため、これらの操作の `--request` は `@path.json` または `-`（標準入力）のみを受け付け、インライン JSON は拒否します。Find / Read / Delete / Health / MonitorLocalRouter の `--request` はインラインと `@path.json` を受け付けます。

`--trace` を指定すると、SDK が通信ログとしてリクエスト本文と応答本文をそのまま出力します。秘密情報を含むリクエストを送る操作 (`update-settings` の `--request` など) では、トレースログに秘密情報が含まれるため、ログの取り扱いに注意してください。この振る舞いは SDK の通信ログの仕様で、`--trace` を渡す全コマンド共通です。

個別フラグは次の通りです。

- スイッチ接続: `--switch-id` (API の `Switch.Code`)、`--switch-category` (API の `Switch.Category`)、`--switch-zone-id` (API の `Switch.ZoneID`)。スイッチのフラグを指定した場合は `--switch-id` が必須で、スイッチ設定全体を組み立てます。既定値の補完は行わず、さくらのクラウドのスイッチでも `--switch-category cloud` と明示します。
- インターフェース: 次の 4 項目を受け付けます。
  - `--interface-virtual-ip-address`: API の `Interface.VirtualIPAddress`
  - `--interface-ip-address`: カンマ区切りまたは繰り返し指定、API の `Interface.IPAddress`
  - `--interface-network-mask-len`: API の `Interface.NetworkMaskLen`
  - `--interface-vrid`: API の `Interface.VRID`

インターフェースのフラグを指定した場合は 4 項目すべてが必須で、インターフェース設定全体を組み立てます。`--interface-ip-address` は kong の `*[]string` (最後の指定のみ有効) ではなく `[]string` を使い、繰り返し指定を累積します。複数の IP アドレスを指定漏れなく受け取るためです。空の要素は解析時に拒否します。
- スタティックルート: 反復可能な `--static-route 'prefix=CIDR,next-hop=IP'`。`prefix` と `next-hop` は必須で、未知キー、重複キー、空値を拒否します。指定すると一覧全体を置き換えます。
- その他: `--name`、`--description`、`--tags` (カンマ区切り)、`--icon-id`。Update では未指定と明示的な空文字列を区別するためポインターとして扱います。

## マージの意味

生の API (`LocalRouterAPI.Update` / `UpdateSettings`) は設定の全体置換で、nil の設定は null として送られ既存値が失われます。本コマンドは 2 つの経路で部分更新を実現します。

- `update`: service 層の `Update` を利用します。service 層は現在値を Read して未指定のポインター項目を引き継ぐため、未指定の設定は現状維持になります。
- `update-settings`: skr のアダプターが現在値を Read し、指定された設定だけを上書きして `LocalRouterAPI.UpdateSettings` に渡します。`SettingsHash` には Read で取得した現在値を渡します。

両方の経路で、`--static-route` や `--interface-ip-address` の指定は一覧全体の置換です。JSON 経路で `StaticRoutes` や `Peers` に空配列を指定すると一覧を消去します。スイッチ接続の解除 (スイッチとインターフェースのクリア) は対応せず、ヘルプに生の API 操作が必要な旨を記載します。

Create では service 層が不完全な接続設定の送信を黙って省略するため、設定系の項目を 1 つでも指定した場合にスイッチとインターフェースの完全な設定を必須とします。UpdateSettings のアダプターは、マージ結果がスイッチとインターフェースの片側だけになる入力を拒否します。

## 検証

`internal/localrouterapi` のテストは、公開 `iaas.APICaller` を実装するテスト caller で SDK の API 操作経路を検証します。UpdateSettings のアダプターは、未指定の設定が現在値で置き換わること、空配列が一覧の消去になること、マージ結果がスイッチとインターフェースの片側だけになる入力を拒否することを確認します。フラグ経路と JSON 経路が同じリクエストになること、両方の指定が拒否されることも確認します。

`internal/cli` のテストは実際の Kong コマンドパス、`--request` の秘密情報制約、ヘルプの内容を確認します。`test/e2e/local-router` は testing.T ベースのライブシナリオ ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)) です。

ビルド済み CLI とプロファイルディレクトリの `skr-e2e` プロファイルを確認し、ライブ API に接続する操作を承認した場合のみ、次のコマンドを実行します。

```console
$ go build -o skr ./cmd/skr
$ SKR_E2E=1 go test ./test/e2e/local-router/
```

シナリオはランダム名のスイッチ (is1b) とローカルルータを作成し、`update-settings` でスイッチ接続とインターフェース、スタティックルートを設定して、ヘルスステータスとモニタリングを確認します。`--name` での検索、メタデータの更新も行い、後片付けでは保持した ID に加えて、直前に CLI で確認した状態 (識別情報と接続設定) と再照合して一致する場合だけ削除し、不在を確認します。不在の判断に使う検索結果が1ページに収まらない場合は、不在と判断せずエラーにします。途中で失敗した場合は、作成されたリソースだけを削除します。後片付けは各リソースの作成直後に、削除と不在確認を含む `defer` としてその場で登録します。コマンドの入出力証跡は Git 管理外の `e2e-output/` 配下に Markdown で保存します。

## ライブ検証で確認した API の挙動

- ローカルルータは作成直後から `Availability: available` で、非同期なプロビジョニング待ちが必要ないことを確認しています。
- 設定更新 (PUT `commonserviceitem/{id}`) も、メタデータ作成と同様にサービスプリンシパルの JWT 認証で実行できることをライブ E2E で確認しています。
- 繰り返し指定とカンマ区切りの両形式で `--interface-ip-address` を指定し、API に 2 つの IP アドレスが反映されることを確認しています。
