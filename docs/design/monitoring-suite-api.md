# Monitoring Suite API コマンド設計

- Status: Implemented
- Related ADR: [ADR 0094](../adr/0094-monitoring-suite-api-commands.md)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0` (`api/monitoring-suite`)

## コマンド構成

`monitoring-suite-api` は SDK がラッパー層で公開する 90 操作のうち 88 操作を生成器で生成し、2 操作を手書きする ([ADR 0074](../adr/0074-api-command-name-structure.md))。生成コードと SDK アダプタは `internal/monitoringsuiteapi/` に置き、CLI 登録と SDK factory の接続は `internal/cli/` が担当する。

| リソース | 操作 | 備考 |
| --- | --- | --- |
| alert-project | list / read / create / patch / delete | `list-alert-project-histories` は手書き |
| alert-project rule | list / read / create / patch / delete | `--alert-query` (PromQL)。history の一覧もあり |
| alert-project notification-target | list / read / create / patch / delete | create は手書き (URL 検証) |
| alert-project notification-routing | list / read / create / delete | create は `--request` のみ。並べ替え (`reorder-*`) は `--request` のみ |
| alert-project log-measure-rule | list / read / create / patch / delete | create と patch は `--request` のみ |
| dashboard | list / read / create / patch / delete | create の `--description` は必須 |
| log-storage | list / read / create / patch / delete / set-expire | `--days` で保存期間を指定 |
| log-storage key | list / read / create / patch / delete | |
| log-routing | list / read / create / delete | |
| metrics-storage | list / read / create / patch / delete | 保存期間変更はなし (API が固定) |
| metrics-storage key | list / read / create / patch / delete | |
| metrics-routing | list / read / create / delete | |
| traces-storage | list / read / create / patch / delete / set-expire | |
| publisher | list / read | 読み取り専用 |
| management | read-resource-limits / read-provisioning-state / initialize-provisioning | initialize は `--request` のみ |

部分更新は `patch-*` で、未指定の項目はリクエストに含まれない。`--request` と個別フラグは相互排他とし、暗黙にマージしない。

## 入力

独立した単純スカラー値で有効なリクエストになる操作は個別フラグを設ける。配列やネストした構造を要求する操作は `--request` JSON 経路のみとする ([ADR 0007](../adr/0007-api-request-input.md))。

- `--request` の JSON キーは SDK パラメータ型のフィールド名 (例: `NotificationTargetUID`) に従う。ogen 型由来の構造を持つリクエスト (reorder、initialize-provisioning) は API の JSON 名 (例: `notification_routing_uid`) に従う。
- 通知ルーティングの作成は `--request '{"NotificationTargetUID":"...","MatchLabels":[{"name":"severity","value":"critical"}]}'`。並べ替えは `--request '[{"notification_routing_uid":"...","order":3}]'`。
- プロビジョニング初期化は `--request '{"Logs":{"system_exist":true,"user_exist":true},"Metrics":{"system_exist":true,"user_exist":true}}'`。
- ログ計測ルールの作成は version 付きの構造化クエリを含むため `--request` のみ。`"Rule":{"version":"v1","query":{...}}` の形で送る。
- アラートルールの `query` に対応するフラグは `--alert-query`。CLI の共通オプション `--query` (jq 式) との衝突を避けるための名称で、値は PromQL 形式の文字列。
- `set-log-storage-expire` / `set-trace-storage-expire` は対象のリソース ID を位置引数、保存期間を `--days` で指定する。
- アラートルールの `threshold_warning` / `threshold_critical` は比較演算子を先頭に付けた形式 (例: `>=70`) で指定する。`enabled_warning` / `enabled_critical` は少なくとも一方を true にする必要がある。

## ライブ検証で確認した API の挙動

- ダッシュボードプロジェクト作成の `description` は必須。省略したリクエストは 500 (`X-Sakura-Content-Length: 0`、応答本文なし) になる。CLI では `--description` を必須フラグにしている。
- アラートルール作成の `threshold_warning` / `threshold_critical` に演算子を含まない値 (例: `10`) を送ると 400 (`invalid threshold_warning operator`) になる。比較演算子を先頭に付けた形式 (例: `>=10`) は受け付けられる ([チュートリアルの手順 7](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/tutorial.html) の UI 入力「条件: `>=`、値: 70」に対応する形式)。
- アラートルールの `query` は PromQL 形式で検証される。構文的に不正な文字列 (例: `| rate() > 1` のようなパイプ構文) は 400 (`invalid query`) になる。
- `enabled_warning` と `enabled_critical` の両方を省略 (false) したアラートルール作成は 400 (`Either enabled_warning or enabled_critical must be set to true`) になる。
- 保存期間変更 (`set-log-storage-expire`、`set-trace-storage-expire`) は、応答に新しい保存期間を返す (ログストレージは `expire_day`、トレースストレージは `retention_period_days`)。
- 一覧操作の応答の `id` は数値 (int64) で、`resource_id` と同じ値を返す。
- 一覧操作は API 側の絞り込み (`--bucket-classification`、`--status`、`--resource-id` など) 以外に `--name` フラグを持たない。名前での絞り込みは `--query-raw` の jq 式で行う ([ADR 0049](../adr/0049-internet-workflow-options.md) の `--name` 拡張は IaaS の対象操作に限る)。
- ストレージの作成直後は参照が 404 になることがあるため、反映待ちは期限付きのリトライで吸収する。

## テストとライブ E2E

`internal/cli/` のテスト (`monitoring_suite_test.go`) は sakumock v0.14.0 の `monitoringsuite` モックサーバーを使い、全コマンドのライフサイクル、JSON／フラグ相互排他、URL 検証、ヘルプを確認する。sakumock は実 API より入力検証が緩いため、実 API の入力規則 (前節) はライブ E2E とヘルプで補う。

`test/e2e/monitoring-suite/scenario_lifecycle_test.go` は testing.T ベースのシナリオテストである ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。`SKR_E2E=1` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test` では実行環境を変更しない。

シナリオは、読み取り系コマンド (パブリッシャー、リソース上限、プロビジョニング状態) に続き、ダッシュボードプロジェクト、アラートプロジェクト、メトリクス・ログ・トレースの各ストレージを作成し、それぞれの参照・部分更新・一覧を照合する。アラートルール (PromQL と演算子付きしきい値、`enabled-*` の両有効)、通知先 (URL 更新を含む)、通知ルーティング (作成、参照、部分更新、並べ替え、一覧の順序確認)、ログ計測ルール、ログ・メトリクスルーティング (一覧と部分更新を含む) を検証する。さらに各ストレージのアクセスキー (作成、参照、一覧、部分更新、削除)、保存期間変更、利用量 (日次・月次)、履歴の一覧を検証する。アクセスキーの作成・参照・一覧は応答にキーの値が含まれるため、`CallHidingStdout` で応答本文を実行記録に残さない。作成したリソースはすべて `defer` で削除し、削除後の不在を期限付きで確認する。defer は各リソースの作成直後に登録し、途中のステップが失敗しても作成済みリソースを残さない。

プロビジョニング状態の参照は行うが、`initialize-provisioning` は実行しない (既存環境の保存領域構成を変更するため、[ADR 0094](../adr/0094-monitoring-suite-api-commands.md) で対象外とした)。

メトリクス・ログ・トレースの各ストレージは課金リソースのため、実行ごとにストレージの課金が発生する (作成して削除する)。

```console
$ SKR_E2E=1 go test ./test/e2e/monitoring-suite/ -run TestScenarioMonitoringSuiteLifecycle -timeout 30m -v
```

2026-11-19 にライブ実行し、全ステップの成功と後片付け (残存リソースなし) を確認した。

履歴の参照 3 コマンドは、アラート発生後でないと正常応答を確認できないため未ライブ検証とし、command-coverage baseline では `implemented` に留めている。シナリオは履歴が存在する場合に参照を確認する。`initialize-provisioning` は [ADR 0094](../adr/0094-monitoring-suite-api-commands.md) でライブ検証の対象外としている。
