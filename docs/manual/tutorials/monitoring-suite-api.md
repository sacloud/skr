# Monitoring Suite API: ストレージとアラートを CLI で作る

モニタリングスイートは、ログ・メトリクス・トレースの保存領域 (ストレージ) と、メトリクスに基づくアラート設定、他サービスからのデータ送信経路 (ルーティング) を提供します。この手順では、ストレージとアラートプロジェクトを作成し、アラートルールや通知先、ルーティングを設定したあと、作成したリソースをすべて削除します ([モニタリングスイートのマニュアル](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/index.html))。

> [!WARNING]
> ストレージ (メトリクス・ログ・トレース) とアラートプロジェクトは課金リソースです。作成中は料金が発生します。料金体系は[課金仕様](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/billing.html)を参照してください。この手順では作成したリソースをすべて削除しますが、既存のストレージやアラートプロジェクトは変更しません。

モニタリングスイートのプロジェクトの利用を開始していない場合、最初に保存領域の初期化 (プロビジョニング) が必要です。この手順では既存のプロビジョニング状態を確認するだけで、初期化は実行しません。初期化が必要な場合はコントロールパネルから実施してください ([サービス基本情報](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/about.html))。

## 前提条件

- `skr` を利用できる環境が必要です。
- モニタリングスイートの操作が許可された認証プロファイルを用意してください。
- 以下のコマンドは同じシェルで実行します。作成したリソースの ID はコマンド出力から設定してください。

## 作成するリソースと順序

1. プロビジョニング状態、パブリッシャー、リソース上限を確認します。
2. メトリクスストレージとログストレージを作成します (課金)。
3. アラートプロジェクト、通知先、通知ルーティングを作成します。
4. アラートルールを設定します。
5. ログ計測ルールを設定します。
6. ログルーティングとメトリクスルーティングを作成します。
7. トレースストレージを作成し、保存期間を変更します (課金)。
8. アラートの履歴を確認し、作成したリソースをすべて削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr monitoring-suite-api --help
```

プロビジョニング状態を確認します。`logs` と `metrics` の各領域に `system_exist` と `user_exist` が表示され、保存領域が初期化済みかどうか分かります。

```console
$ skr monitoring-suite-api read-provisioning-state
```

パブリッシャーの一覧を確認します。パブリッシャーはさくらのクラウドの各種サービスからログやメトリクスを送信する送信元で、ルーティングの作成で指定します。

```console
$ skr monitoring-suite-api list-publishers --query 'map({code, variants: [.variants[] | {name, storage}]})'
```

契約のリソース上限も確認できます。

```console
$ skr monitoring-suite-api read-resource-limits
```

## Step 2: メトリクスストレージとログストレージを作成する

重複しない名前でメトリクスストレージとログストレージを作成します。ログストレージの `--classification` は `shared` (共用プラン) または `dedicated` (専有プラン) を指定します。専有プランでは暗号化の設定 (`--kms-key-id`、`--service-principal-id`) が可能です。メトリクスストレージの作成リクエストにはプランの指定がありません。

```console
$ SUFFIX=$(date +%s)
$ METRICS_STORAGE_ID=$(skr monitoring-suite-api create-metric-storage \
    --name "skr-tutorial-ms-metrics-$SUFFIX" \
    --description "Monitoring suite tutorial" --query-raw '.id')
$ LOG_STORAGE_ID=$(skr monitoring-suite-api create-log-storage \
    --name "skr-tutorial-ms-logs-$SUFFIX" \
    --description "Monitoring suite tutorial" \
    --classification shared --query-raw '.id')
$ echo "$METRICS_STORAGE_ID $LOG_STORAGE_ID"
```

`--query-raw` は文字列の結果を引用符なしで出力するため、ID をそのままシェル変数に取り込めます。作成直後の参照は反映完了まで 404 を返すことがあるため、その場合は数十秒待って再確認します。

```console
$ skr monitoring-suite-api read-metric-storage "$METRICS_STORAGE_ID" --query '{id,name,classification}'
$ skr monitoring-suite-api read-log-storage "$LOG_STORAGE_ID" --query '{id,name,classification,expire_day}'
```

ストレージにデータを書き込むためのアクセスキーを作成できます。アクセスキーの一覧はストレージごとに管理されます。

```console
$ skr monitoring-suite-api create-log-storage-key "$LOG_STORAGE_ID" \
    --description "Tutorial key" --query '{uid,description}'
```

アクセスキーのシークレットは作成時のみ表示されます。書き込み設定の詳細は[各エージェントの設定例](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/agent/index.html)を参照してください。

## Step 3: アラートプロジェクトと通知先を作成する

アラートルールや通知先を管理するためのアラートプロジェクトを作成します。

```console
$ PROJECT_ID=$(skr monitoring-suite-api create-alert-project \
    --name "skr-tutorial-ms-alert-$SUFFIX" \
    --description "Monitoring suite tutorial" --query-raw '.id')
$ echo "$PROJECT_ID"
```

通知先を作成します。`--service-type` は `SAKURA_SIMPLE_NOTICE` (シンプル通知) を指定し、`--url` には通知の送信先 URL を指定します。

```console
$ TARGET_UID=$(skr monitoring-suite-api create-alert-project-notification-target "$PROJECT_ID" \
    --service-type SAKURA_SIMPLE_NOTICE \
    --url "https://example.test/hook" \
    --description "Tutorial target" --query-raw '.uid')
$ echo "$TARGET_UID"
```

シンプル通知でメールを受け取る場合は、通知先アドレスの本登録と通知先グループが必要です。詳細は[チュートリアルの手順 7](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/tutorial.html)を参照してください。

通知ルーティングは、発生したアラートを条件に応じて通知先へ振り分ける設定です。条件にはアラートのラベルマッチャーを指定します。構造化された条件は `--request` JSON で指定します。

```console
$ ROUTING_UID=$(skr monitoring-suite-api create-alert-project-notification-routing "$PROJECT_ID" \
    --request '{"NotificationTargetUID":"'"$TARGET_UID"'","MatchLabels":[{"name":"severity","value":"critical"}]}' \
    --query-raw '.uid')
$ echo "$ROUTING_UID"
```

通知ルーティングは複数設定でき、評価順序 (`order`) は並べ替えコマンドで変更します。

```console
$ skr monitoring-suite-api reorder-alert-project-notification-routings "$PROJECT_ID" \
    --request '[{"notification_routing_uid":"'"$ROUTING_UID"'","order":3}]'
```

## Step 4: アラートルールを設定する

アラートルールは、メトリクスストレージに保存されたメトリクスに対する PromQL 形式のクエリ (`--alert-query`) と、しきい値でアラートを発生させる設定です。しきい値 (`--threshold-warning`、`--threshold-critical`) は比較演算子を先頭に付けた形式 (例: `>=70`) で指定します。`--enabled-warning` と `--enabled-critical` は少なくとも一方を指定します。

```console
$ RULE_UID=$(skr monitoring-suite-api create-alert-project-rule "$PROJECT_ID" \
    --metrics-storage-id "$METRICS_STORAGE_ID" \
    --alert-query '100 * (1 - avg(system_cpu_utilization_ratio{state="idle"}))' \
    --name "skr-tutorial-rule-$SUFFIX" \
    --threshold-warning ">=70" \
    --threshold-duration-warning 60 \
    --enabled-warning --query-raw '.uid')
$ echo "$RULE_UID"
```

PromQL の書き方は[チュートリアルの手順 6](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/tutorial.html)や[さくらのナレッジの解説記事](https://knowledge.sakura.ad.jp/32878/)を参照してください。

## Step 5: ログ計測ルールを設定する

ログ計測ルールは、ログストレージに保存されたログの行数をメトリクスとしてメトリクスストレージへ保存する設定です。フィルタ条件を構造化クエリで指定するため、`--request` JSON で指定します。

```console
$ MEASURE_RULE_UID=$(skr monitoring-suite-api create-alert-project-log-measure-rule "$PROJECT_ID" \
    --request '{"Name":"skr-tutorial-measure-'"$SUFFIX"'","LogStorageID":"'"$LOG_STORAGE_ID"'","MetricsStorageID":"'"$METRICS_STORAGE_ID"'","Rule":{"version":"v1","query":{"matchers":[{"type":"string","operator":"eq","field":"log_name","value":"skr-tutorial","value_list":[]}]}}}' \
    --query-raw '.uid')
$ echo "$MEASURE_RULE_UID"
```

作成したルールは参照・部分更新・一覧・削除ができます。

```console
$ skr monitoring-suite-api list-alert-project-log-measure-rules "$PROJECT_ID" --query 'map({uid,name})'
```

## Step 6: ログルーティングとメトリクスルーティングを作成する

さくらのクラウドの各種サービスからのログやメトリクスをストレージへ送信するルーティングを作成します。`--publisher-code` は送信元サービスを表すパブリッシャー、`--variant` は種別 (`logs`、`metrics`) を指定します。利用可能なパブリッシャーと種別は `list-publishers` の出力で確認し、ここではログの種別を持つ最初のパブリッシャーを取り込みます。

```console
$ PUBLISHER_CODE=$(skr monitoring-suite-api list-publishers \
    --query-raw 'map(select(any(.variants[]; .storage == "logs"))) | .[0].code')
$ echo "$PUBLISHER_CODE"
$ LOG_ROUTING_UID=$(skr monitoring-suite-api create-log-routing \
    --publisher-code "$PUBLISHER_CODE" --variant logs --log-storage-id "$LOG_STORAGE_ID" \
    --query-raw '.uid')
$ echo "$LOG_ROUTING_UID"
$ METRIC_ROUTING_UID=$(skr monitoring-suite-api create-metric-routing \
    --publisher-code "$PUBLISHER_CODE" --variant metrics --metrics-storage-id "$METRICS_STORAGE_ID" \
    --query-raw '.uid')
$ echo "$METRIC_ROUTING_UID"
```

## Step 7: トレースストレージを作成し、保存期間を変更する

トレースストレージを作成します。トレースストレージはユーザ領域のみで、`--classification` は `shared` (共用プラン) または `dedicated` (専有プラン) を指定します。

```console
$ TRACE_STORAGE_ID=$(skr monitoring-suite-api create-trace-storage \
    --name "skr-tutorial-ms-traces-$SUFFIX" \
    --description "Monitoring suite tutorial" \
    --classification shared --query-raw '.id')
$ echo "$TRACE_STORAGE_ID"
```

ログストレージとトレースストレージの保存期間は `--days` で変更できます。メトリクスストレージの保存期間は変更できません。保存期間によっては追加料金が発生します ([コントロールパネル操作ガイド](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/ui.html))。

```console
$ skr monitoring-suite-api set-log-storage-expire "$LOG_STORAGE_ID" --days 90 --query '{expire_day}'
$ skr monitoring-suite-api set-trace-storage-expire "$TRACE_STORAGE_ID" --days 60 --query '{retention_period_days}'
```

保存期間の変更はストレージの部分更新でも可能です。

## Step 8: アラートの履歴を確認する

アラートプロジェクトの履歴を確認します。アラートが発生していない場合、空の一覧が返ります。履歴は重要度 (`--severity`) や解消状態 (`--open`、`--closed`) で絞り込めます。

```console
$ skr monitoring-suite-api list-alert-project-histories "$PROJECT_ID" --query 'map({id,severity,open})'
$ skr monitoring-suite-api list-alert-project-rule-histories "$PROJECT_ID" \
    "$RULE_UID" --query 'map({id,severity,open})'
```

ダッシュボードプロジェクトを作成すると、モニタリングスイート専用 Web UI (Perses) でメトリクスを可視化できます。作成の際は `--description` が必要です。

```console
$ DASHBOARD_ID=$(skr monitoring-suite-api create-dashboard-project \
    --name "skr-tutorial-dashboard-$SUFFIX" \
    --description "Monitoring suite tutorial" --query-raw '.id')
$ echo "$DASHBOARD_ID"
```

## Step 9: 後片付けする

作成したリソースをすべて削除します。まず ID と名前を照合してから削除し、削除後は参照して残っていないことを確認します。

```console
$ skr monitoring-suite-api delete-log-routing "$LOG_ROUTING_UID"
$ skr monitoring-suite-api delete-metric-routing "$METRIC_ROUTING_UID"
$ skr monitoring-suite-api delete-alert-project-log-measure-rule "$PROJECT_ID" "$MEASURE_RULE_UID"
$ skr monitoring-suite-api delete-alert-project-notification-routing "$PROJECT_ID" "$ROUTING_UID"
$ skr monitoring-suite-api delete-alert-project-notification-target "$PROJECT_ID" "$TARGET_UID"
$ skr monitoring-suite-api delete-alert-project-rule "$PROJECT_ID" "$RULE_UID"
$ skr monitoring-suite-api delete-trace-storage "$TRACE_STORAGE_ID"
$ skr monitoring-suite-api delete-log-storage "$LOG_STORAGE_ID"
$ skr monitoring-suite-api delete-metric-storage "$METRICS_STORAGE_ID"
$ skr monitoring-suite-api delete-alert-project "$PROJECT_ID"
$ skr monitoring-suite-api delete-dashboard-project "$DASHBOARD_ID"
```

Step 8 でダッシュボードプロジェクトを作成しなかった場合は、削除の行をスキップしてください。

削除が反映されるまで数秒かかることがあります。参照が 404 を返せば削除済みです。削除に失敗したリソースがあると料金が発生し続けるため、`list-*` コマンドで `skr-tutorial` から始まる名前のリソースが残っていないことを確認してください。

```console
$ skr monitoring-suite-api list-alert-projects --query-raw 'map(select(.name | test("skr-tutorial")))'
[]
```

ストレージの課金は削除後に止まります。課金の詳細は[課金仕様](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/billing.html)を参照してください。

## 参考資料

- [モニタリングスイート](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/index.html)
- [モニタリングスイート 課金仕様](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/billing.html)
- [モニタリングスイート コントロールパネル操作ガイド](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/ui.html)
- [モニタリングスイート チュートリアル](https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/tutorial.html)
- [sacloud-sdk-go v0.7.0: monitoring-suite API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.7.0/api/monitoring-suite)
