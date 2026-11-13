# ADR 0084: Local Router API コマンドを追加する

- Status: Accepted
- Date: 2026-11-13
- SDK: `github.com/sacloud/sacloud-sdk-go v0.6.0`

## Context

ローカルルータは拠点間やゾーン間をまたぐ閉域網 (L3) ネットワークを構成する CommonServiceItem 系のグローバルリソースです。`sacloud-sdk-go` は `service/iaas/localrouter` (Find / Read / Create / Update / Delete / Health / MonitorLocalRouter / Apply) と `api/iaas.LocalRouterAPI` (Find / Create / Read / Update / UpdateSettings / Delete / HealthStatus / MonitorLocalRouter) を公開します。service 層に `UpdateSettings` はなく、設定だけを更新する操作は API 層にあります。

API 層の `Update` と `UpdateSettings` は設定の全体置換です。`LocalRouterUpdateRequest` や `LocalRouterUpdateSettingsRequest` で `Switch`、`Interface`、`Peers`、`StaticRoutes` を nil のまま送ると API には null が送られ、既存の設定が失われます。このため未指定の項目を現在値で引き継ぐ部分更新の経路を CLI 側で用意する必要があります。service 層の `Update` は `BuilderFromResource` が現在値を読み込んで未指定のポインター項目を引き継ぐため、部分更新として利用できます。

スイッチ接続の設定は、スイッチを特定する `Switch` (`Code`、`Category`、`ZoneID`) と VRRP の `Interface` (`VirtualIPAddress`、`IPAddress`、`NetworkMaskLen`、`VRID`) が対になります。service 層の Create はスイッチとインターフェースの設定が不完全な場合、設定の送信を黙って省略します。CLI は不完全な設定を暗黙に無視してはいけません。ピア設定の `Peers[].SecretKey` は秘密情報のため、コマンド引数やインライン JSON に含めない入力経路が必要です。ローカルルータはゾーンを持たないため、GSLB や ProxyLB と同じ `<service>-api` コマンドとして配置します ([ADR 0034](0034-container-registry-api-commands.md)、[ADR 0072](0072-proxylb-api-domain-specific-options.md) の系譜)。[ADR 0071](0071-iaas-api-uses-api-layer.md) の service 層の禁止範囲は `skr iaas-api` であり、CommonServiceItem 系ドメインでは service 層の公開 API を利用できます。

`StaticRoutes` は `Prefix` と `NextHop` の 2 つの単純なスカラー値だけを持つ配列です。スイッチ接続とスタティックルートはローカルルータ利用の主要な作業であり、これらをすべて JSON に強制すると、単純な設定のためにリクエスト全体の記述が必要になります。

## Decision

- `skr local-router-api` を追加し、`find`、`create`、`read`、`update`、`update-settings`、`delete`、`health-status`、`monitor-local-router` の 8 操作を公開します。実装は `internal/localrouterapi/` に置き、CLI 登録と SDK factory の接続は `internal/cli/` に置きます。service 層の `Apply` は公開しません。
- Create、Read、Update、Delete、Health、MonitorLocalRouter は service 層を利用します。UpdateSettings は API 層の `LocalRouterAPI.UpdateSettings` を直接呼び出し、skr 側で現在値を読み込んでマージするアダプターを実装します。`SettingsHash` には Read で取得した現在値を渡します。これは libsacloud のライブテスト (`test/local_router_op_test.go`) と同じ経路です。
- Find は `--name` (部分一致、1要素の `Names` への変換)、`--tags`、`--count`、`--from` を受け付けます。Sort など生成フラグにない入力は `--request` JSON に残します。
- Create / Update / UpdateSettings の接続設定は個別フラグで指定します。スイッチは `--switch-id` (API の `Switch.Code`)、`--switch-category` (API の `Switch.Category`)、`--switch-zone-id` (API の `Switch.ZoneID`)、インターフェースは `--interface-virtual-ip-address`、`--interface-ip-address` (カンマ区切り)、`--interface-network-mask-len`、`--interface-vrid` を受け付けます。既定値の補完は行わず、指定した値だけを送ります。
- スタティックルートは反復可能な `--static-route 'prefix=CIDR,next-hop=IP'` で指定します。`prefix` と `next-hop` は必須で、未知キー、重複キー、空値を拒否します。指定すると一覧全体を置き換えます。この構文はローカルルータ専用であり、他のサービスや配列へ自動的には適用しません ([ADR 0041](0041-server-repeatable-disk-and-nic-options.md) と同じ限定です)。
- Update と UpdateSettings は部分更新です。未指定の設定は現在値が引き継がれ、明示した `false`、`0`、空文字列も入力として区別します。`--static-route` と `--interface-ip-address` を指定すると一覧全体を置き換えます。JSON 経路で `StaticRoutes` や `Peers` に空配列を指定すると一覧を消去します。スイッチ接続の解除 (`Switch` と `Interface` のクリア) は本 CLI では対応せず、設定の解除が必要な場合は生の API 操作を使うことをヘルプに記載します。
- スイッチとインターフェースのフラグは、それぞれのグループ内で設定全体を組み立てます。スイッチのフラグを指定した場合は `--switch-id` が必須、インターフェースのフラグを指定した場合は 4 項目すべてが必須です。Create では service 層が設定の送信を黙って省略するため、設定系の項目を 1 つでも指定した場合にスイッチとインターフェースの完全な設定を必須とします。UpdateSettings のアダプターは、マージ結果がスイッチとインターフェースの片側だけになる入力を拒否します。
- `--request` には各操作のリクエスト JSON を渡せます。Create / Update / UpdateSettings の `--request` は `Peers[].SecretKey` など秘密情報を受け取れるため、直接指定は受け付けず、`@path.json` または `-`（標準入力）のみを受け付けます。その他の操作は直接指定と `@path.json` を受け付けます。JSON 経路と個別フラグは相互排他とし、暗黙にマージしません。UpdateSettings の JSON は skr の部分更新入力型にデコードし、生の API リクエストとはしないため、未指定の設定は消えません。
- `health-status` は `--id` を受け付けます。`monitor-local-router` は `--id` と任意の `--start`、`--end` (RFC3339 形式) を受け付け、未指定時は SDK の既定動作 (終了時刻が現在、開始時刻がその 1 時間前) に従います。
- sakumock にローカルルータの実装がないため、SDK の公開 `iaas.APICaller` interface を実装するテスト caller で SDK の API 操作経路とマージの変換を検証します。CLI 操作自体はドメイン API mock と再実行可能なライブ E2E でも検証します。
- ライブ E2E はテスト用プロジェクトでローカルルータとスイッチを作成し、スイッチ接続、スタティックルート、検索、参照、ヘルスステータス、モニタリング、メタデータ更新を確認したあと、ID を照合して削除し、不在を確認します。

## Consequences

- よく使うスイッチ接続とスタティックルートをフラグで指定しつつ、ピア設定などの複雑な入力は保護した JSON で扱えます。
- `--static-route` の解析とヘルプを保守する必要があるため、正常系・異常系と SDK リクエストへの変換をパッケージ内テストで固定します。
- Update と UpdateSettings は未指定の設定を現在値で引き継ぐため、設定を消すには空配列の JSON 指定が必要です。スイッチ接続そのものの解除はできません。
- UpdateSettings は service 層に存在しないため、skr 側のアダプターが Read とマージを担います。service 層の Update と API 層の UpdateSettings の 2 つの更新経路が存在することになります。

## Alternatives considered

- Create / Update / UpdateSettings を JSON 専用にする案: スイッチ接続やスタティックルートの設定にもリクエスト全体の JSON が必要になり、単純な作業の入力が重くなるため採用しません。
- `Switch` 全体を `--switch 'key=value'` 形式の反復可能オプションにする案: 項目数が 3 と少なく個別フラグで意味が明確なため採用しません。`--static-route` の独自構文を他のフィールドに自動適用しません。
- `--switch-category` に既定値 `cloud` を設定する案: 未指定の値をリクエストに勝手に設定しない方針に反するため採用しません。さくらのクラウド以外の接続先では明示的に指定します。
- UpdateSettings を JSON で生の API リクエストとして受ける案: API が全体置換のため、未指定の設定が黙って消えます。skr の部分更新の意味と一致しないため採用しません。
- `iaas-api localrouter` として低レベル API 操作を公開する案: グローバルリソースでありゾーン指定型の IaaS リソースとは形が異なります。service 層を利用できる CommonServiceItem 系ドメインとして `<service>-api` に配置します。
