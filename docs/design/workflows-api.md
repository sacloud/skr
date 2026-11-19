# Workflows API コマンド設計

- Status: Accepted
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0`
- ADR: [0089](../adr/0089-workflows-api-commands.md)

## コマンド

コマンド名は operationId から機械派生します ([ADR 0074](../adr/0074-api-command-name-structure.md))。

```text
skr workflows-api list-workflows [--page ... --page-limit ... --sort-by ... --order ... --published --name ... --name-match-type ...]
skr workflows-api list-suggestions --name ... [--page ... --page-limit ... --sort-by ... --order ...]
skr workflows-api create-workflow (--name ... --runbook-file ... [--description ... --publish --logging --tags ... --revision-alias ... --concurrency-mode ...] | --request ...)
skr workflows-api read-workflow <id>
skr workflows-api patch-workflow <id> [--name ... --description ... --publish --logging --tags ... --concurrency-mode ... | --request ...]
skr workflows-api delete-workflow <id>

skr workflows-api create-revision <workflow-id> (--runbook-file ... [--revision-alias ...] | --request ...)
skr workflows-api list-revisions <workflow-id> [--page ... --page-limit ... --sort-by ... --order ... --published]
skr workflows-api read-revision <workflow-id> <revision-number>
skr workflows-api update-revision-alias <workflow-id> <revision-number> (--revision-alias ... | --request ...)
skr workflows-api delete-revision-alias <workflow-id> <revision-number>

skr workflows-api create-execution <workflow-id> [--revision-id ... | --revision-alias ...] [--args ...] [--name ...]
skr workflows-api list-executions <workflow-id> [--page ... --page-limit ... --order ...]
skr workflows-api read-execution <workflow-id> <execution-id>
skr workflows-api cancel-execution <workflow-id> <execution-id>
skr workflows-api delete-execution <workflow-id> <execution-id>
skr workflows-api list-execution-histories <workflow-id> <execution-id> [--page ... --page-limit ... --sort-order ...]

skr workflows-api list-plans
skr workflows-api read-subscription
skr workflows-api create-subscription (--plan-id ... | --request ...)
skr workflows-api delete-subscription
```

## SDK 操作との対応

| CLI | SDK operationId | SDK |
| --- | --- | --- |
| `list-workflows` | `listWorkflows` | `WorkflowAPI.List` |
| `list-suggestions` | `listSuggestions` | `WorkflowAPI.ListSuggest` |
| `create-workflow` | `createWorkflow` | `WorkflowAPI.Create` |
| `read-workflow` | `readWorkflow` | `WorkflowAPI.Read` |
| `patch-workflow` | `patchWorkflow` | `WorkflowAPI.Update` |
| `delete-workflow` | `deleteWorkflow` | `WorkflowAPI.Delete` |
| `create-revision` | `createRevision` | `RevisionAPI.Create` |
| `list-revisions` | `listRevisions` | `RevisionAPI.List` |
| `read-revision` | `readRevision` | `RevisionAPI.Read` |
| `update-revision-alias` | `updateRevisionAlias` | `RevisionAPI.UpdateAlias` |
| `delete-revision-alias` | `deleteRevisionAlias` | `RevisionAPI.DeleteAlias` |
| `create-execution` | `createExecution` | `ExecutionAPI.Create` |
| `list-executions` | `listExecutions` | `ExecutionAPI.List` |
| `read-execution` | `readExecution` | `ExecutionAPI.Read` |
| `cancel-execution` | `cancelExecution` | `ExecutionAPI.Cancel` |
| `delete-execution` | `deleteExecution` | `ExecutionAPI.Delete` |
| `list-execution-histories` | `listExecutionHistories` | `ExecutionAPI.ListExecutionHistories` |
| `list-plans` | `listPlans` | `SubscriptionAPI.ListPlans` |
| `read-subscription` | `readSubscription` | `SubscriptionAPI.Read` |
| `create-subscription` | `createSubscription` | `SubscriptionAPI.Create` |
| `delete-subscription` | `deleteSubscription` | `SubscriptionAPI.Delete` |

## 入力と出力

- Runbook YAML は `--runbook-file` で読み込みます。`-` で標準入力から読み込めます。`--request` JSON の `Runbook` フィールドでも指定できます。
- Tags は反復可能な `--tags NAME` で受け取ります。要素は API の `Tags` 配列 (要素は `Name`) に変換されます。
- `ConcurrencyMode` は `--concurrency-mode` で受け取ります。`parallel`、`lock`、`queue` のいずれかで、CLI が値を検証します。
- `ServicePrincipalId` は `--request` JSON でのみ指定できます。公式マニュアルは、サービスプリンシパルを利用するワークフローは API トークンまたはサービスプリンシパルを認証主体とした API 経由では作成できないことを案内しています (<https://manual.sakura.ad.jp/cloud/appliance/workflows/index.html>)。値の検証は API に委ねます。
- `patch-workflow` の個別フラグ経路では、指定した項目だけをリクエストに含めます (patch の意味論)。`--tags` を省略すると `Tags` をリクエストに含めません。既存タグを維持したい場合は `--request` JSON で現在値を再指定してください。
- `create-execution` の `--revision-id` と `--revision-alias` は相互排他です。どちらも未指定のときは API が最新リビジョンを実行します。`Args` は `--args` (インライン JSON、`@path.json`、`-` による標準入力) で渡し、CLI が JSON として妥当か検証します。実行にはワークフローの公開設定が必要です。
- `create-subscription` は `--plan-id` と `--request` を相互排他で受け取ります。利用可能なプランは `list-plans` で確認できます。`delete-subscription` は契約の解約です。

## 生成と手書きの分担

- `create-workflow`、`create-revision`、`list-revisions`、`create-execution`、`list-executions`、`list-execution-histories` は手書きコマンドです。Runbook ファイルの読み込み、Tags の変換、実行リビジョン指定の相互排他、Args の JSON 検証、位置引数の ID から list 系パラメータの組み立てを行います。
- その他の 15 操作は生成コマンドです。設定は `api/commands/workflows-workflow.json`、`workflows-revision.json`、`workflows-execution.json`、`workflows-subscription.json` の 4 ファイルです。
- `patch-workflow` の `--tags` は生成器の repeated_flags と request_transform (`SetPatchWorkflowTags`) で `[]v1.PatchWorkflowReqTagsItem` に変換します。

## テスト

- 単体テストは sakumock v0.14.0 の `workflows` モックサーバーに対して行います。ワークフローとリビジョンの作成から実行、解約までの一連の流れを確認します。
- `list-suggestions` は sakumock が API 応答スキーマ上必須の `Count` を返さないためデコードが失敗します。詳細と現状の扱いは [sakumock の既知の差分と未整備](sakumock-gaps.md) を参照してください。
- ライブ E2E はプラン契約 (課金) を伴うため、シナリオと費用の範囲を提示して合意した上で実施します。
