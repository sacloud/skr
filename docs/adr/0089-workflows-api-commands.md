# ADR 0089: Workflows API コマンド

- Status: Accepted
- Date: 2026-11-18
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0`

## Context

Workflows は Runbook (YAML) をリビジョン管理し、実行と履歴を扱うワークフローサービスである。さくらのクラウド公式マニュアルは <https://manual.sakura.ad.jp/cloud/appliance/workflows/> にあり、API リファレンスは <https://manual.sakura.ad.jp/cloud/appliance/workflows/api/index.html> で公開されている。

SDK (`api/workflows`) は次の 4 つの API クライアントを公開する。

- `WorkflowAPI`: ワークフロー定義の CRUD、一覧、名前候補の検索 (7 操作)
- `RevisionAPI`: Runbook リビジョンの作成・一覧・取得、エイリアスの付け替えと削除 (5 操作)
- `ExecutionAPI`: 実行の開始・一覧・取得・キャンセル・削除、実行履歴の一覧 (6 操作)
- `SubscriptionAPI`: プラン一覧とサブスクリプション (契約) の取得・開始・解約 (4 操作)

SDK v0.7.0 時点での合計は 21 操作である。operationId はすべて ADR 0074 の命名対応表で kebab-case に機械派生できる。

実装にあたって検討した入力上の特性は次のとおり。

- ワークフロー作成・更新、リビジョン作成には Runbook YAML 全文が必要である。YAML は複数行かつ長大になるため、CLI 引数へのインライン指定は現実的でない。
- `CreateWorkflowReq.Tags` は `[]TagsItem` (要素は `Name` のみ) という構造化配列である。
- `ConcurrencyMode` は列挙値 (`parallel`、`lock`、`queue`) である。
- `ServicePrincipalId` は OpenAPI で `oneOf: string|number` として定義されたスカラーではなく単一値だが、公式マニュアルは「サービスプリンシパルを利用するワークフローは、API トークンまたはサービスプリンシパルを認証主体とした API 経由では作成できない」ことを案内している。
- 実行開始 (`createExecution`) は対象リビジョンを `RevisionId` または `RevisionAlias` で指定でき、未指定のとき API は最新リビジョンを実行する。`Args` は実行時に Runbook へ渡す JSON 文字列である。
- `SubscriptionAPI.Create` はプラン契約の開始、`Delete` は解約であり、課金に関わる操作である。

## Decision

- 21 操作すべてを `skr workflows-api` 配下にフラットに配置し、コマンド名は operationId から機械派生する ([ADR 0074](0074-api-command-name-structure.md))。一覧は design doc ([workflows-api](../design/workflows-api.md)) の表にまとめる。
- Runbook の入力は `--runbook-file` で受け取る (KMS の `--plain-key-file` と同様のファイル入力。秘密情報ではないためインライン指定は不要)。`-` で標準入力から読み込める。`--request` JSON の `Runbook` フィールドでも指定できる経路を維持する。
- Tags の入力は反復可能な `--tags NAME` とする。`patch-workflow` は生成器の repeated_flags と request_transform で `[]v1.PatchWorkflowReqTagsItem` に変換する。`create-workflow` は手書きコマンドで同じ変換を行う。patch の個別フラグ経路では `--tags` を省略すると `Tags` をリクエストに含めない (patch の意味論を維持)。配列全体の置換は `--request` JSON で行える。
- `ConcurrencyMode` は `--concurrency-mode` フラグで受け取り、CLI が値を検証する。
- `ServicePrincipalId` は個別フラグ化せず、`--request` JSON のみで指定できる経路とする。API 側の制約 (API 経由では作成できない) があるため、値の検証は API に委ねる。
- `create-execution` の `--revision-id` と `--revision-alias` は相互排他とする。両指定時の API の挙動が仕様に明記されていないため、CLI で拒否する。どちらも未指定のときは最新リビジョンで実行される。`Args` は `--args` (インライン JSON 文字列、または `@path.json` と `-` によるファイル読み込み) で受け取り、CLI が JSON として妥当か検証する。`@path` 形式は AGENTS.md の入力経路の規約に揃えている。
- `create-subscription` は `--plan-id` と `--request` を相互排他で受け取り、どちらかの指定を必須にする。`delete-subscription` は契約の解約であることをヘルプに明記する。
- 待機系のオプションは設けない ([ADR 0082](0082-high-level-command-wait-options.md)。本コマンド群は低レベルの `<service>-api` である)。
- 単体テストは sakumock の `workflows` モックサーバー (`sakumock/workflows`) に対して行う。ライブ E2E はプラン契約 (課金) を伴うため、シナリオと費用の範囲を提示して合意した上で実施する。

## Consequences

- Runbook YAML を含むすべてのリクエスト項目は `--request` JSON でも指定できるため、個別フラグがカバーしない項目 (`ServicePrincipalId` など) の入力経路が失われない。
- patch の個別フラグ経路で `--tags` を省略した場合の挙動 (Tags をリクエストに含めない) をヘルプとテストで固定する。既存タグを維持したい利用者は `--request` JSON で現在値を再指定する必要がある。
- `--revision-id` と `--revision-alias` の相互排他は CLI 固有の制約である。API が将来的に両指定の意味を定義した場合は見直す。
- `workflowsapi` package は 4 つの生成コマンド群 (`WorkflowCommands`、`WorkflowRevisionCommands`、`WorkflowExecutionCommands`、`WorkflowSubscriptionCommands`) と手書き 6 コマンド (`create-workflow`、`create-revision`、`list-revisions`、`create-execution`、`list-executions`、`list-execution-histories`) を持つ。手書きコマンドも Runbook ファイル読み込みと Tags 変換を共有ヘルパーで実装する。手書きの list 操作は位置引数の ID を API パラメータへ組み立てるため、生成器の request_by_value ではなく手書きで対応する。
- ライブ E2E は契約開始・解約を含むため、実施前に利用者と費用の範囲を合意する必要がある。合意まで `PLAN.md` ではライブ E2E を未実施として扱う。

## Alternatives considered

- `--runbook-file` ではなく Runbook YAML を値として受ける引数 (`--runbook 'key: value'`) を設ける案: 複数行 YAML のシェル引用が困難で、実用にならない。採用しない。
- Tags をカンマ区切り `--tags a,b` で受ける案: [ADR 0035](0035-iaas-tags-flag.md) は IaaS のトップレベル `Tags` 配列 (文字列) のみを対象とする。Workflows の Tags は構造化配列であり、反復可能オプションの方が要素境界が明確である。採用しない。
- `ServicePrincipalId` も個別フラグ化する案: OpenAPI で oneOf 型であり、API 経由での作成が公式に案内されていない操作に対して入力経路を広げる価値がない。採用しない。
- サブスクリプション操作を公開しない案: プランの確認 (`list-plans`) と契約状態の確認 (`read-subscription`) は安全な参照操作であり、開始・解約も API として存在する。CLI から隠す根拠がない。採用しない。
