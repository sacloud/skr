# Simple Notification API コマンド設計

## 対象範囲

`simple-notification-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` の Simple Notification API を操作します。通知先、通知先グループ、ルーティング、通知履歴を対象とし、公開する操作は次のとおりです。

| リソース | 実装操作 |
| --- | --- |
| `destination` | `list`、`read`、`create`、`update`、`delete`、`read-status` |
| `group` | `list`、`read`、`create`、`update`、`delete`、`send-message` |
| `routing` | `list`、`read`、`create`、`update`、`delete`、`reorder`、`list-source` |
| `history` | `list`、`read` |

シンプル通知はゾーンに依存せず、メールまたは Webhook で通知を送信します。通知先グループは1つ以上の通知先をまとめ、グループへの送信で登録された通知先に通知します。メール通知先はアドレス確認が完了するまで利用できず、Webhook は Slack、Discord、IFTTT、Zapier の URL が対象です。通知の到達は保証されず、メールマガジン、確実性が必要な障害通知、同一宛先への短時間の反復通知には適しません（[サービス基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)、[通知先](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination.html)、[通知先グループ](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_group.html)）。

## パッケージと生成

各リソースの操作は `api/commands/simple-notification-*.json` で明示し、`make generate-api` から `internal/simplenotificationapi/` に生成します。リソース固有の SDK 接続、リクエスト補完、通知先入力の保護処理も同じパッケージに配置します。CLI の登録と Runtime／SDK factory の接続は `internal/cli/` が担当します。

生成コードを直接編集せず、設定を変更して再生成します。生成コードと設定の一致は `internal/cli/simplenotification_test.go` で確認します。手書き処理と SDK 呼び出しは `internal/simplenotificationapi/` の単体テストで、CLI 登録とヘルプは `internal/cli/` のテストで確認します。

トレース設定はコマンド初期化時に固定せず、引数解析後のクライアント生成時に参照します。`--trace` の有無で HTTP トレースの出力を切り替えることを、CLI から公開 SDK を通すテストで確認します。

## 入力と秘密情報

- 通知先の作成は JSON リクエストを `@path.json` で読み込みます。メールアドレスや Webhook URL を `--request` の値としてコマンド引数に渡せません。
- 通知先の更新は `Settings` を含まないメタデータ変更であればインライン JSON または `@path.json` を利用できます。`Settings` を含む場合は設定値を保護するため `@path.json` を要求します。通知先アドレスは更新できず、変更は別の通知先を作成してグループ設定を更新します。
- 作成リクエストでは、通知先、グループ、ルーティングそれぞれの `Provider.Class` と `ServiceClass` を CLI が設定します。未指定の `Description`、`Icon`、`Tags` も SDK の入力モデルに合わせて補完します。利用者向けヘルプでは、これらの補完をサービス操作の入力として誤解させないよう説明します。
- `group send-message` は `--message` または `--request` JSON を受け付けます。`--message` は API リクエストの `Message` に対応し、両経路は併用できません。
- グループ更新の `Settings` は全体として指定します。宛先や有効状態を維持する場合は、`group read` の結果を確認して更新リクエストにも含めます。
- 通知先、グループ、ルーティングなどの配列や複雑な設定は JSON 経路に残します。独自のフラグ構文は追加しません。

通知 API には対象操作の権限が必要です。API キーを使う場合は「作成・削除」アクセスレベルを付与します（[通知API](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/api.html)）。CLI の認証情報は SDK プロファイルまたは既存の SDK 認証機構から読み込みます。

通知先の入力解析と API 操作で発生したエラーは、入力値や応答本文を含まないメッセージで表示します。元のエラーは `Unwrap` で保持し、`errors.Is` / `errors.As` による分類を維持します。これにより、誤って `Type` に入れた Webhook URL などが検証エラーへ埋め込まれても、通常のエラー出力には表示しません。明示的な `--trace` は従来どおり機密情報を含む可能性のある HTTP トレースを出力するため、取り扱いに注意してください。

## 通知送信と履歴

通知は通知先 ID ではなく通知先グループ ID を指定して送信します。API の `202` 応答と `is_ok: true` は受付結果です。送信後は通知履歴から `request_id`、本文、対象グループと通知先の `statuses` を確認します。履歴 API が取得できるのは直近30日以内の最大100件です。状態 `2` はシンプル通知の送信処理が完了したことを示しますが、通知先への到達を示すものではありません（[通知API](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/api.html)、[通知履歴](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/history.html)、[通知API OpenAPI](https://manual.sakura.ad.jp/api/cloud/portal/simple-notification-api/index.html)）。

## ライブ E2E と後片付け

`test/e2e/simple-notification/` はビルド済み CLI を呼び出す testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。シナリオの概要と実行方法は [`test/e2e/simple-notification/README.md`](../../test/e2e/simple-notification/README.md) を参照してください。実行には `SKR_E2E` の設定、`skr-e2e` プロファイル、および `SKR_E2E_SIMPLE_NOTIFICATION_DESTINATION_ID` 環境変数で利用者が承認した既存通知先の ID が必要です。既存通知先の ID を環境変数で選択することを実行の明示的な同意とし、未設定の場合はテストをスキップします。

```console
$ SKR_E2E=1 SKR_E2E_SIMPLE_NOTIFICATION_DESTINATION_ID=DESTINATION-ID go test ./test/e2e/simple-notification/ -run TestScenarioSimpleNotification
```

`DESTINATION-ID` は利用者が承認した既存通知先の ID に置き換えます。シナリオは通知先の送信可否を確認し、既存グループとの名前衝突を拒否してから、固有の名前と説明、通知先1件を設定した一時グループを作成します。送信 API の受付後、通知履歴で固有メッセージ、グループ ID、通知先 ID、状態 `2` を確認します。送信処理の確認は期限付きで行います。

後片付けでは Create 応答から保持した ID でグループを削除し、一覧で ID と名前が残っていないことを確認します。Create 応答から ID を得られない場合はテストが失敗し、グループは削除しません。通常の送信シナリオでは通知先に既存リソースを使い、変更・削除しません。履歴は一覧との照合に加え、同じ `request_id` の個別取得結果も照合します。

### 通知を送信しない CRUD シナリオ

`TestScenarioSimpleNotificationCRUDOnly` は、既存のメール通知先からアドレスを読み取り、同じアドレスの一時通知先、一時グループ、一時ルーティングを作成します。メールアドレスはコマンド引数へ渡さず、権限 `0600` の一時リクエストファイルに保存します。既存リソースは変更しません。`send-message` は呼び出しません。通知先登録時には確認メールを受信することがあります。

```console
$ SKR_E2E=1 SKR_E2E_SIMPLE_NOTIFICATION_DESTINATION_ID=DESTINATION-ID go test ./test/e2e/simple-notification/ -run TestScenarioSimpleNotificationCRUDOnly
```

- 通知先、グループ、ルーティングの作成、一覧、個別取得、メタデータ更新、設定更新を確認します。通知先とグループは有効状態を切り替えて戻し、ルーティングは固有の照合ラベルを変更します。
- `Settings` を省略したメタデータ更新で既存設定が維持されることと、設定全体の再指定で宛先や有効状態が保持されることを、更新後の個別取得で確認します。
- 一時通知先の `read-status` に `IsValid` が含まれることを確認します。確認メールへの応答は要求せず、未確認の通知先を送信可能と判定しません。
- ルーティングは固有の照合ラベルと一時グループを使います。作成時の優先順位は応答から取得し、`99` と `100` が未使用の場合だけ、その一時ルーティングの順位を変更します。既存ルーティングの ID や順位は並べ替え要求に含めず、前後の設定を照合します。
- 一時リソースはルーティング、グループ、通知先の順に、Create 応答から保持した ID で削除します。削除後は一覧で ID と名前が残っていないことを確認します。

設定の反映待ちは期限付きの再試行で行い、通信失敗は成功扱いしません。後片付けの削除や不在確認に失敗した場合は、後続のリソースの後片付けを続けてからテストを失敗させます。クラウドリソースとローカル一時ファイルの後片付けが終わってから最終結果を保存します。失敗した実行の証跡は成功結果で上書きしません。

更新や並べ替えの各ステップは、応答と更新後の個別参照で状態を確認します。テストが途中で失敗した場合も、Create 応答から保持した ID で後片付けを試みます。API 呼び出しの失敗は、後片付けに成功してもテストの結果へ残ります。

再実行時も対象プロファイル、通知先、影響を確認して明示的な承認を得てください。

### 検証範囲

単体テストでは、シナリオのリソース照合（タグの順序に依存しない比較、Settings の期待キーだけの比較、完全一致）を検証します。公開 SDK と HTTP テストサーバーを通した更新テストでは、`Settings` 自体の省略、内部の `Disabled` の省略、明示的な `true` と `false` の送信内容を区別します。このテストだけで API サーバーの設定保持動作を確認したとは扱いません。

ライブ E2E ではメール通知処理の完了と一時グループの削除を確認済みです。利用者による実メールの受信確認も完了しています。追加ライブ E2E では通知先・グループ・ルーティングの CRUD、設定省略時の保持、有効状態と照合ラベルの更新、一時ルーティングだけの並べ替え、既存設定の保持、一時リソースの削除と不在を確認済みです。元の通知について履歴の個別取得も確認しています。これは実行結果であり、通知の到達保証を意味しません。Webhook の実送信・到達確認は [#130](https://github.com/sacloud/skr/issues/130) で対応します。再実行時も対象プロファイル、通知先、影響を確認して明示的な承認を得てください。

## 参考資料

- [シンプル通知の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)
- [通知先](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination.html)
- [通知先グループ](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_group.html)
- [通知先ルーティング](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_routing.html)
- [通知履歴](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/history.html)
- [Simple Notification API / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/simple-notification)
