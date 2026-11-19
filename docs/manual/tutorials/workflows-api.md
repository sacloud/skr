# Workflows API: Runbook を定義してワークフローを実行する

Workflows は、定義された順序や条件に基づいて、リソースの起動や API の呼び出しなどの特定の操作を自動的に実行するサービスです。ワークフローは YAML 形式の Runbook で定義します ([Workflowsについて](https://manual.sakura.ad.jp/cloud/appliance/workflows/about.html))。このチュートリアルでは、`skr workflows-api` でワークフローを定義し、引数付きで実行し、結果とキャンセルを確認するまでの一連の流れを行います。

> [!WARNING]
> この手順では Workflows のワークフロー (2件)、リビジョン、実行 (2件) を作成し、最後にすべて削除します。Workflows の利用にはプランの契約が必要で、契約とステップ数に応じた課金が発生します ([Workflowsを始めよう](https://manual.sakura.ad.jp/cloud/appliance/workflows/getting_started.html))。実行前に `skr config current` で対象プロファイルとプロジェクトを確認してください。

## このサービスを使う場面

Runbook に実行したい操作をステップとして定義すると、Workflows のワーカーがその内容を実行します。定義したワークフローはコントロールパネルから手動実行できるほか、API から引数を渡して実行できます ([Workflowsについて](https://manual.sakura.ad.jp/cloud/appliance/workflows/about.html)、[Workflowsを始めよう](https://manual.sakura.ad.jp/cloud/appliance/workflows/getting_started.html))。定期的な運用タスクの自動化や、複数の API 操作を順序立てて行う処理の実装に利用します。

Runbook の構文 (`meta`、`args`、`steps`) や、ステップで使える外部関数はマニュアルのリファレンスにまとめられています ([Runbook 構文リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-syntax.html)、[Runbook callの外部関数リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-call.html))。

## 前提条件

- `skr` と `skr workflows-api` が利用できること。
- 対象プロジェクトのプロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- Workflows のワークフローと実行を操作できる権限を持つ API キーが設定されていること。Workflows の API 利用には API キーによる認証が必要です ([APIリファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-api.html)、[APIキー](https://manual.sakura.ad.jp/cloud/api/apikey.html))。
- Workflows のプラン契約が済んでいること。未契約の場合は Step 2 で確認し、[プランを選択する（プラン選択画面）](https://manual.sakura.ad.jp/cloud/appliance/workflows/operation-subscription.html) に従って契約してください。このチュートリアルでは契約操作 (課金操作) 自体は実行しません。
- ID の抽出に `jq` を利用します。事前に導入してください。
- `UNIQUE-SUFFIX` を他のリソース名と重複しない短い英数字またはハイフン列に置き換えること。
- `REVISION-ALIAS` をリビジョンに付ける任意のエイリアス名 (英数字とハイフンなど) に置き換えること。

## 作成するリソースと順序

1. **Runbook ファイル** — ワークフローの定義です。作業端末のローカルファイルとして用意します。
2. **ワークフロー (本体)** — Runbook を登録して作成します。公開状態で作成します。
3. **リビジョンとエイリアス** — Runbook の改訂版をリビジョンとして追加し、エイリアスを設定して確認します。
4. **実行** — 引数を渡してワークフローを実行し、完了を待って結果を確認します。
5. **キャンセル確認用のワークフローと実行** — 待機する Runbook を実行し、実行中にキャンセルします。
6. **削除** — 実行とワークフローを依存順に削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr workflows-api create-workflow --help
$ skr workflows-api create-execution --help
```

意図したプロファイルとプロジェクトを確認してください。Workflows はゾーンを指定しないグローバルなサービスです。

## Step 2: プランと契約を確認する

```console
$ skr workflows-api read-subscription
```

現在の契約が表示されます。以下は契約していない場合の出力例です。`CurrentPlan` が `null` の場合は利用が開始されていないため、先に契約を開始してください。

```json
{
  "CurrentPlan": null,
  "is_ok": true
}
```

利用できるプランは次のコマンドで確認できます。以下は出力形式の例で、プランの内容と値は契約時の案内を確認してください ([プランを選択する（プラン選択画面）](https://manual.sakura.ad.jp/cloud/appliance/workflows/operation-subscription.html))。

```console
$ skr workflows-api list-plans --query '.Plans | map({id,name})'
```

```json
[
  {
    "id": 1,
    "name": "PLAN-NAME"
  },
  {
    "id": 2,
    "name": "PLAN-NAME"
  }
]
```

## Step 3: Runbook を用意する

Runbook は、`meta` (説明と引数) と `steps` (実行するステップ) からなる YAML です。ここでは文字列の引数 `name` を受け取り、挨拶の文字列を返す 1 ステップの Runbook を作成します。引数の型とステップの種類は [Runbook 構文リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-syntax.html) を参照してください。

```console
$ cat > runbook.yaml <<'EOF'
meta:
  description: Workflows チュートリアル
  args:
    name:
      type: string
      default: world
steps:
  greet:
    return: "hello, ${args.name}"
EOF
```

`return` の `${args.name}` は引数を参照する式です。引数を省略した場合は `default` の値が使われます。

## Step 4: ワークフローを作成する

Runbook を登録してワークフローを作成します。`--publish` はワークフローを公開状態で作成します。作成結果の `Id` は `--query-raw` でコマンド置換により `WORKFLOW_ID` に取り込みます (以降の ID も同じ手順です)。

```console
$ WORKFLOW_ID=$(skr workflows-api create-workflow --name workflows-tutorial-UNIQUE-SUFFIX --description "Workflows チュートリアル" --publish --runbook-file runbook.yaml --query-raw '.Id')
$ skr workflows-api read-workflow "$WORKFLOW_ID" --query '{Id,Name,Publish,ConcurrencyMode}'
```

参照結果に指定した名前と公開状態が表示されることを確認します。以下は出力形式の例で、ID は置き換えています。`ConcurrencyMode` は同時実行の制御方法で、指定しない場合は並列実行 (Parallel) になります ([Workflowsを始めよう](https://manual.sakura.ad.jp/cloud/appliance/workflows/getting_started.html))。

```json
{
  "ConcurrencyMode": "parallel",
  "Id": "123456789012",
  "Name": "workflows-tutorial-UNIQUE-SUFFIX",
  "Publish": true
}
```

名前の部分一致で検索し、作成したワークフローが見つかることも確認します。

```console
$ skr workflows-api list-workflows --name workflows-tutorial-UNIQUE-SUFFIX --query '.Workflows | map({Id,Name})'
```

```json
[
  {
    "Id": "123456789012",
    "Name": "workflows-tutorial-UNIQUE-SUFFIX"
  }
]
```

## Step 5: リビジョンを追加してエイリアスを設定する

Runbook を変更してリビジョンとして追加します。ここでは `sed` で戻り値の挨拶を書き換えたファイルを改訂版として使います。リビジョンには連番の `RevisionId` が付き、最新のリビジョンが実行の既定になります。

```console
$ sed 's/hello, /こんにちは、/' runbook.yaml > runbook-v2.yaml
$ skr workflows-api create-revision "$WORKFLOW_ID" --runbook-file runbook-v2.yaml --query '{RevisionId,WorkflowId}'
```

作成されたリビジョンは 2 番です。リビジョン一覧で確認します。

```console
$ skr workflows-api list-revisions "$WORKFLOW_ID" --query '.Revisions | map({RevisionId,RevisionAlias})'
```

```json
[
  {
    "RevisionAlias": "",
    "RevisionId": 1
  },
  {
    "RevisionAlias": "",
    "RevisionId": 2
  }
]
```

リビジョン 2 にエイリアスを設定します。エイリアスはリビジョンの別名で、`create-execution` の `--revision-alias` に指定して実行するリビジョンを選べます。`REVISION-ALIAS` は前提条件に従って決めた値に置き換えます。

```console
$ skr workflows-api update-revision-alias "$WORKFLOW_ID" 2 --revision-alias REVISION-ALIAS --query '{RevisionId,RevisionAlias}'
```

```json
{
  "RevisionAlias": "REVISION-ALIAS",
  "RevisionId": 2
}
```

エイリアスを削除して、外れたことを確認します。

```console
$ skr workflows-api delete-revision-alias "$WORKFLOW_ID" 2
$ skr workflows-api read-revision "$WORKFLOW_ID" 2 --query '{RevisionId,RevisionAlias}'
```

```json
{
  "RevisionAlias": "",
  "RevisionId": 2
}
```

## Step 6: ワークフローを実行して結果を確認する

引数を渡してワークフローを実行します。引数は Runbook の `args` に合わせた JSON です。リビジョンを指定しない場合、実行は最新のリビジョン (ここではリビジョン 2) で行われます ([Workflowsを始めよう](https://manual.sakura.ad.jp/cloud/appliance/workflows/getting_started.html))。

```console
$ EXECUTION_ID=$(skr workflows-api create-execution "$WORKFLOW_ID" --name exec-UNIQUE-SUFFIX --args '{"name": "Workflows"}' --query-raw '.ExecutionId')
```

実行はワーカーにより非同期で行われます。ステータスは `Queued` (キュー待ち) から `Running` (実行中) を経て `Succeeded` (成功) に変わります。次のコマンドで完了を待ちます。

```console
$ until [ "$(skr workflows-api read-execution "$WORKFLOW_ID" "$EXECUTION_ID" --query-raw .Status)" = Succeeded ]; do sleep 3; done
```

完了したら、ステータスと実行されたリビジョンを確認します。

```console
$ skr workflows-api read-execution "$WORKFLOW_ID" "$EXECUTION_ID" --query '{Status,Revision}'
```

```json
{
  "Revision": 2,
  "Status": "Succeeded"
}
```

実行結果は `Result` に戻り値が JSON として文字列化されて格納されます。`--query-raw` で取り出すと、文字列の戻り値は引用符付きで表示されます。

```console
$ skr workflows-api read-execution "$WORKFLOW_ID" "$EXECUTION_ID" --query-raw .Result
"こんにちは、Workflows"
```

実行一覧と、実行のステップごとの履歴を確認します。履歴の項目数と内容は実行内容により異なります。

```console
$ skr workflows-api list-executions "$WORKFLOW_ID" --query '.Executions | map({ExecutionId,Name,Status})'
```

```json
[
  {
    "ExecutionId": "f70a1cc3-a8b0-4b35-af3b-02a0d2262c7d",
    "Name": "exec-UNIQUE-SUFFIX",
    "Status": "Succeeded"
  }
]
```

```console
$ skr workflows-api list-execution-histories "$WORKFLOW_ID" "$EXECUTION_ID" --query '.Histories | map({Type})'
```

```json
[
  {
    "Type": "workflowWillStart"
  },
  {
    "Type": "stepWillExecute"
  },
  {
    "Type": "workflowDidCompleted"
  }
]
```

## Step 7: 実行をキャンセルする

実行中のワークフローはキャンセルできます。ここでは `sys.sleep` で待機する Runbook を使って、実行中のキャンセルを確認します。`sys.sleep` は指定した秒数だけ待機する外部関数です ([Runbook callの外部関数リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-call.html))。

```console
$ cat > runbook-sleep.yaml <<'EOF'
meta:
  description: 待機して終了するワークフロー
steps:
  wait:
    call: sys.sleep
    args:
      seconds: 60
EOF
```

```console
$ SLEEP_WORKFLOW_ID=$(skr workflows-api create-workflow --name workflows-tutorial-sleep-UNIQUE-SUFFIX --description "キャンセル確認用" --publish --runbook-file runbook-sleep.yaml --query-raw '.Id')
$ CANCEL_EXECUTION_ID=$(skr workflows-api create-execution "$SLEEP_WORKFLOW_ID" --name cancel-UNIQUE-SUFFIX --query-raw '.ExecutionId')
$ until [ "$(skr workflows-api read-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID" --query-raw .Status)" = Running ]; do sleep 3; done
```

実行をキャンセルします。キャンセルできるのは `Queued` と `Running` の実行です。キャンセル処理の間はステータスが `Canceling` になり、完了すると `Canceled` になります。

```console
$ skr workflows-api cancel-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID" --query '{ExecutionId,Status}'
$ until [ "$(skr workflows-api read-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID" --query-raw .Status)" = Canceled ]; do sleep 3; done
$ skr workflows-api read-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID" --query '{Status}'
```

```json
{
  "Status": "Canceled"
}
```

## Step 8: 作成したリソースを削除する

まず実行を削除します。削除の前に `read-execution` で ID とステータスがこの手順のものと一致することを確認してください。

```console
$ skr workflows-api read-execution "$WORKFLOW_ID" "$EXECUTION_ID" --query '{ExecutionId,Status}'
$ skr workflows-api delete-execution "$WORKFLOW_ID" "$EXECUTION_ID"
$ skr workflows-api read-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID" --query '{ExecutionId,Status}'
$ skr workflows-api delete-execution "$SLEEP_WORKFLOW_ID" "$CANCEL_EXECUTION_ID"
```

続けてワークフローを削除します。削除の前に `read-workflow` で ID と名前がこの手順で作成したものと一致することを確認してください。

```console
$ skr workflows-api read-workflow "$WORKFLOW_ID" --query '{Id,Name}'
$ skr workflows-api delete-workflow "$WORKFLOW_ID"
$ skr workflows-api read-workflow "$SLEEP_WORKFLOW_ID" --query '{Id,Name}'
$ skr workflows-api delete-workflow "$SLEEP_WORKFLOW_ID"
```

最後に、この手順で作成したワークフローが残っていないことを名前で検索して確認します。

```console
$ skr workflows-api list-workflows --name workflows-tutorial-UNIQUE-SUFFIX --query '.Workflows | map({Id,Name})'
[]
```

## まとめ

`skr workflows-api` で Runbook を登録したワークフローを作成し、引数付きの実行と完了待ち、結果と履歴の確認、実行中のキャンセルを一連の流れで行いました。あわせて、リビジョンの追加とエイリアスの設定・削除も確認しました。

このチュートリアルでは扱っていない操作があります。ワークフローの部分更新 (`patch-workflow`)、エイリアスで実行するリビジョンを選ぶ `create-execution --revision-alias`、プランの契約 (`create-subscription` と `delete-subscription`) です。契約操作は課金を伴うため、チュートリアルでは実行していません。条件分岐や繰り返しなどのステップ種別は Runbook の構文で表現します。ワークフローへの登録は、スキーマに沿った YAML を `--runbook-file` で指定します ([Runbook 構文リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-syntax.html))。

## 参考資料

- [Workflowsについて](https://manual.sakura.ad.jp/cloud/appliance/workflows/about.html)
- [Workflowsを始めよう](https://manual.sakura.ad.jp/cloud/appliance/workflows/getting_started.html)
- [Workflowsを利用する](https://manual.sakura.ad.jp/cloud/appliance/workflows/operation.html)
- [プランを選択する（プラン選択画面）](https://manual.sakura.ad.jp/cloud/appliance/workflows/operation-subscription.html)
- [Runbookによるワークフロー定義の概要](https://manual.sakura.ad.jp/cloud/appliance/workflows/about-runbook.html)
- [Runbook 構文リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-syntax.html)
- [Runbook callの外部関数リファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-call.html)
- [APIリファレンス](https://manual.sakura.ad.jp/cloud/appliance/workflows/reference-api.html)
- [APIキー](https://manual.sakura.ad.jp/cloud/api/apikey.html)
- [sacloud-sdk-go v0.7.0: Workflows API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.7.0/api/workflows)
