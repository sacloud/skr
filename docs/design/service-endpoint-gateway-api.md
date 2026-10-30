# Service Endpoint Gateway API コマンド設計

## 対象範囲

`service-endpoint-gateway-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` のサービスエンドポイントゲートウェイを操作します。サービスエンドポイントゲートウェイはスイッチの拡張機能で、プライベートネットワーク内からオブジェクトストレージやコンテナレジストリなどのマネージドサービスへ閉域接続します。スイッチ単体が対象で、ルータ＋スイッチでは利用できません。1 つのスイッチに対して複数のサービスエンドポイントゲートウェイは作成できません。

公開する操作は `appliance` グループの次のとおりです。

| グループ | 実装操作 |
| --- | --- |
| `appliance` | `list`、`read`、`create`、`update`、`apply`、`delete`、`read-interface`、`read-power-status`、`power-on`、`shutdown`、`reset` |

コマンドグループは [ADR 0069](../adr/0069-api-endpoint-command-groups.md) のとおり API endpoint 構造に対応させます。対応するエンドポイントの一覧は ADR 0069 を参照してください。ゲートウェイ配下の操作は `appliance` グループのサブコマンドとして公開します。

作成の入力経路は [ADR 0070](../adr/0070-service-endpoint-gateway-create-input.md) に従います。

API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/{ゾーン名}/api/cloud/1.1/` 配下です。コマンドはゾーン引数を持たず、SDK 認証設定のゾーン (プロファイルの `Zone` または `SAKURA_ZONE` 環境変数) を使います。未設定の場合の既定値は is1a です。エンドポイントは `SAKURA_ENDPOINTS_SERVICE_ENDPOINT_GATEWAY` 環境変数で変更できます。ゾーンを設定した場合はエンドポイントの上書きが無効になるため、注意してください。SDK プロファイル、または `SAKURA_ACCESS_TOKEN` / `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で認証します。

## パッケージと生成

操作の大半は `api/commands/service-endpoint-gateway.json` で明示します。コマンドコードは `make generate-api` から `internal/serviceendpointgatewayapi/service_endpoint_gateway_api_generated.go` に生成します。生成するのは `list`、`read`、`update`、`apply`、`delete`、`read-interface`、`read-power-status`、`power-on`、`shutdown`、`reset` です。API interface、factory、Runtime も生成します。`internal/cli/` の配線は `make generate-wiring` から生成し、設定との一致を `internal/cli/wiring_test.go` で確認します。

次の操作は手書きで `internal/serviceendpointgatewayapi/commands.go` に実装します。

- `create`: 個別フラグと `--request` JSON の両経路を設けるため ([ADR 0070](../adr/0070-service-endpoint-gateway-create-input.md))。

手書き操作でも API interface、factory、Runtime、`Commands` 型のフィールドは生成コードと共有します。型名全体の衝突は `internal/apigen` の設定検証で拒否します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/serviceendpointgatewayapi/` の単体テストで、CLI 登録とヘルプは `internal/cli/` のテストで確認します。

## 入力

- 作成は、必須の `--switch-id` と `--network-mask-len`、任意で繰り返し指定できる `--server-ip` の個別フラグか、`--request` JSON (`@path.json` 形式に対応) を受け付けます。両経路は併用できません。
- `--network-mask-len` はゲートウェイに割り当てるサブネットマスク長、`--server-ip` はゲートウェイに割り当てる IPv4 アドレス (ゲートウェイアドレス) です。`--server-ip` は指定順に API リクエストの `Servers` 配列を組み立てます。未指定の場合は空配列を送ります。
- `--request` は、利用者が指定する備考情報 (`Appliance.Remark`) のみを受け付けます。`Class` と `Plan` は自動設定されるため JSON に含める必要がありません。スイッチ ID、ネットマスク長、サーバー配列の指定はデコード時に検証します。
- サーバー IP の形式、サブネットへの所属、スイッチの種類は CLI で検証せず、API の応答に委ねます。
- 設定の更新は `--request` JSON のみを受け付けます。`Settings` は有効化するサービスごとに構造が異なり、全体置換であることをヘルプに明記します。既存の設定を維持したい項目は JSON に再指定してください。`EnabledServices` には接続先マネージドサービスを `Type` と `Config` で指定します。設定の反映は `apply` で行い、変更時はゲートウェイ経由の通信が一時的に中断することがあります。

API キーやトークンを引数に受け付ける操作はありません。CLI の認証情報は SDK プロファイルまたは既存の SDK 認証機構から読み込みます。

## 出力

API の応答フィールドは PascalCase です。`list`、`read`、`create`、`update`、`read-interface`、`read-power-status`、`power-on` の出力は JSON で、`--query` による加工に対応します。`apply`、`delete`、`shutdown`、`reset` は応答本文を出力しません。作成・読み取りの応答は `Appliance` 配下に本体を含みます。

## ライブ E2E と後片付け

`test/e2e/service-endpoint-gateway-api/` はビルド済み CLI を呼び出す testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。

```console
$ SKR_E2E=1 go test ./test/e2e/service-endpoint-gateway-api/ -run TestScenarioServiceEndpointGateway
```

実行には `SKR_E2E` の設定と `skr-e2e` プロファイルが必要です。シナリオは接続先スイッチの作成から、ゲートウェイの作成、参照、一覧、電源操作、設定更新と反映、削除と不在確認までを検証します。作成するのはスイッチ 1 件とゲートウェイ 1 件のみで、テスト関数内の `defer` で削除と不在確認まで行います。サービスエンドポイントゲートウェイは作成から削除まで課金される可能性があります。

`is1b` はスイッチのゾーン内上限が 1/1 のため、シナリオは上限が緩い `is1c` を使います。ゲートウェイはゾーン引数を持たず、`SAKURA_ZONE` 環境変数でスイッチと同じゾーンに合わせます。update 直後の反映 (`apply`) は一時的に失敗することがあるため、シナリオは期限までリトライします。

`read-interface` はライブ検証をしていません。openapi の `Appliance.Interfaces` 定義には interface 自身の ID フィールドがなく、パスパラメータの `interfaceID` に指定できる値を応答から取得できないためです (接続先スイッチの ID を指定するとライブ API は 404 を返します)。詳細は [#189](https://github.com/sacloud/skr/issues/189) を参照してください。

sakumock を使った CLI と SDK の統合テストは `internal/cli/service_endpoint_gateway_test.go` で確認します。手書き処理の単体テストは `internal/serviceendpointgatewayapi/commands_test.go` で確認します。

## 参考資料

- [サービスエンドポイントゲートウェイ](https://manual.sakura.ad.jp/cloud/network/switch/seg.html)
- [概要](https://manual.sakura.ad.jp/cloud/network/switch/seg/about.html)
- [クイックスタート](https://manual.sakura.ad.jp/cloud/network/switch/seg/getting_started.html)
- [サービスエンドポイントゲートウェイ / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/service-endpoint-gateway)
