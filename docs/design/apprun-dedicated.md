# AppRun Dedicated 高レベルコマンドの設計

- Status: 設計中 (実装は未着手)
- ADR: [ADR 0079](../adr/0079-apprun-dedicated-high-level-commands.md)
- SDK: `github.com/sacloud/sacloud-sdk-go` v0.5.0
- 関連: [ADR 0005](../adr/0005-low-level-and-high-level-command-tiers.md)、[ADR 0027](../adr/0027-apprun-dedicated-api-commands.md)、[ADR 0075](../adr/0075-high-level-command-tier.md)、[ADR 0077](../adr/0077-eventbus-service-library.md)
- 参考実装: [fujiwara/apprun-dedicated-cli](https://github.com/fujiwara/apprun-dedicated-cli)

低レベル `apprun-dedicated-api` の設計は [AppRun Dedicated API コマンド設計](apprun-dedicated-api.md) にあります。

## 目的と範囲

AppRun 専有型のアプリケーションを、JSON の定義ファイル1つからデプロイする作業単位を高レベルコマンド `skr apprun-dedicated` として提供します。低レベル `apprun-dedicated-api` は SDK の操作を1対1で公開しますが、デプロイは「クラスタ ID の検索 → アプリケーションの検索または作成 → バージョン作成 → 有効化 → 反映待ち」という複数操作の組み合わせです。利用者は手順の順序と `registryPasswordAction` の選択を毎回判断する必要があります。参考実装 (fujiwara/apprun-dedicated-cli) が示す ecspresso 形のワークフローを、[ADR 0075](../adr/0075-high-level-command-tier.md) の高レベル層の規則に沿って取り込みます。

この文書の対象は次のとおりです。

- service 層を切り出した `service/apprun-dedicated-service-go` モジュールと、`internal/apprundedicated` の CLI コマンド層
- アプリケーションのデプロイフローを担う10コマンド (`init`、`deploy`、`diff`、`render`、`status`、`versions`、`rollback`、`deactivate`、`delete`、`containers`)

対象外は次のとおりです。

- クラスタ、オートスケーリンググループ、ワーカノード、ロードバランサ、証明書、サービスクラスの参照・管理。低レベル `apprun-dedicated-api` が同じ入出力で提供しており、高レベル層では繰り返しません。インフラの構築と管理 (クラスタ作成を含む) は Terraform などの別手段とし、高レベル層はアプリケーションだけを変更します。
- 低レベル `apprun-dedicated-api` の変更。現行のコマンドと JSON 経路を維持します。
- Jsonnet、環境変数参照 (`env`、`must_env`)、tfstate-lookup、Secret Manager 参照の評価機構。定義は JSON のみです。
- 低レベル `apprun-dedicated-api` コマンドのライブ E2E。クラスタ作成を伴うため [#59](https://github.com/sacloud/skr/issues/59) で管理します。高レベルコマンドのライフサイクルシナリオは「ライブ E2E」のとおり実装済みです。

## 全体構成

### パッケージ構成

service 層は `service/apprun-dedicated-service-go` 配下の独立モジュール (package `apprundedicated`) に、CLI コマンド層は `internal/apprundedicated` に置きます ([ADR 0077](../adr/0077-eventbus-service-library.md))。

```text
service/apprun-dedicated-service-go/
  service.go                # Service 構造体、New、名前検索とクラスタ検索
  definition.go             # ApplicationDefinition、検証、CreateParams 変換、VersionDetail からの復元
  deploy.go                 # deploy (検索または作成、バージョン作成、有効化)
  rollback.go               # rollback (前バージョン・指定バージョンの検索と有効化)
  lifecycle.go              # deactivate、delete (非活性化、停止待ち、削除リトライ)
  wait.go                   # 待機処理 (コンテナ状態のポーリング)
  output.go                 # status・versions 用の射影
  example_test.go           # コンパイル検証される利用例
  *_test.go
internal/apprundedicated/
  commands.go               # kong コマンド (定義読み込みと service 呼び出し)
  definition_input.go       # 定義ファイルの読み込み (--app、標準入力、未知フィールド拒否)
  confirm.go                # 確認プロンプトとトレース設定の検査
  diff.go                   # unified diff の生成 (LCS、文脈行、色付き出力)
  *_test.go
```

### 依存規律

service 層は独立モジュール `service/apprun-dedicated-service-go` で、`sacloud-sdk-go` と標準ライブラリのみに依存し、kong、CLI の Runtime、出力処理、端末入力 (確認プロンプト) には依存しません。CLI 固有の処理 (`--app` と標準入力からの定義読み込み、確認プロンプト、Runtime、ServiceFactory) は `internal/apprundedicated` に残り、ライブラリの型を参照します。ファイル名は `config` を `c0nfig` に置き換えるリポジトリの規則に従います。

`internal/cli` が `NewCommands(runtime, factory)` でコマンド群の依存を一括注入し、SDK クライアントの生成と共通の出力処理を接続します ([ADR 0045](../adr/0045-initialize-api-commands-with-constructors.md))。`internal/apprundedicated` から `internal/cli` には依存せず、段階的な setter 初期化は行いません。

service 層は SDK の応答モデルを返しますが、status・versions の利用者向け射影はライブラリの `output.go` が提供します (秘密情報を出力に含めない処理は安全に関わる共通の関心事であり、[ADR 0077](../adr/0077-eventbus-service-library.md) と同じ判断です)。

### Service の構造

service 層は SDK の公開操作 interface (`application.ApplicationAPI`、`version.VersionAPI`、`cluster.ClusterAPI`) を保持します。`New(client *v1.Client)` が操作を構築します。単体テストでは各 interface に fake を注入します。

```go
func New(client *v1.Client) *Service

func (s *Service) Deploy(ctx context.Context, def *ApplicationDefinition, opts *DeployOptions) (*DeployResult, error)
func (s *Service) Rollback(ctx context.Context, def *ApplicationDefinition, opts *RollbackOptions) (*DeployResult, error)
func (s *Service) Deactivate(ctx context.Context, def *ApplicationDefinition, opts *WaitOptions) (*DeactivateResult, error)
func (s *Service) Delete(ctx context.Context, def *ApplicationDefinition, opts *WaitOptions) (*DeleteResult, error)
func (s *Service) Status(ctx context.Context, def *ApplicationDefinition) (*Status, error)
func (s *Service) Versions(ctx context.Context, def *ApplicationDefinition) ([]VersionEntry, error)
func (s *Service) Containers(ctx context.Context, def *ApplicationDefinition) ([]application.Placement, error)
func (s *Service) Render(def *ApplicationDefinition) (map[string]any, error)
func (s *Service) Diff(ctx context.Context, def *ApplicationDefinition) (*DiffResult, error)
func (s *Service) Init(ctx context.Context, clusterName, applicationName string) (*ApplicationDefinition, error)
```

`Diff` は比較対象の取得と正規化までを service 層が行い、unified diff の文字列化は CLI 層が行います。`Init` は既存リソースから定義を復元して返します。ファイルへの書き出しは CLI 層です。

## アプリケーション定義ファイル

定義は JSON ファイルのみです。Jsonnet などの評価機構は持ちません ([ADR 0079](../adr/0079-apprun-dedicated-high-level-commands.md))。定義ファイルは `--app` グローバルフラグで指定し、既定値はありません。skr は複数のドメインを扱う CLI であるため、カレントディレクトリの暗黙のファイルに依存せず、操作対象を常に明示します (PR #231 のレビューでの合意)。`--app -` で標準入力から読み込みます。インラインの JSON 文字列は受け付けません。秘密情報 (`registryPassword`、`secret=true` の環境変数の値) を含む定義は、コマンドライン引数へ露出させないための経路として、ファイルまたは標準入力から読み込みます ([ADR 0007](../adr/0007-api-request-input.md) の入力経路の規律に合わせます)。

フィールド名は API リクエスト (`CreateApplicationVersionRequest`) と同じ camelCase にします。`cluster` と `name` はデプロイ対象の識別子で、API リクエストには含まれません。

| フィールド | 型 | 必須 | 説明 |
| ---------- | -- | ---- | ---- |
| `cluster` | string | 必須 | デプロイ先クラスタの名前 (CLI 固有。API には送らない) |
| `name` | string | 必須 | アプリケーション名 (CLI 固有。API には送らない) |
| `cpu` | integer | 必須 | API の `cpu` |
| `memory` | integer | 必須 | API の `memory` |
| `scalingMode` | string | 必須 | API の `scalingMode`。SDK v0.5.0 の値は `manual`、`cpu` |
| `fixedScale` | integer | 任意 | `manual` 方式のインスタンス数 (API の `fixedScale`) |
| `minScale` | integer | 任意 | `cpu` 方式の下限 (API の `minScale`) |
| `maxScale` | integer | 任意 | `cpu` 方式の上限 (API の `maxScale`) |
| `scaleInThreshold` | integer | 任意 | `cpu` 方式のスケールイン閾値 (API の `scaleInThreshold`) |
| `scaleOutThreshold` | integer | 任意 | `cpu` 方式のスケールアウト閾値 (API の `scaleOutThreshold`) |
| `image` | string | 必須 | コンテナイメージ (API の `image`) |
| `cmd` | array of string | 任意 | 起動コマンド (API の `cmd`) |
| `registryUsername` | string | 任意 | プライベートレジストリのユーザー名 (API の `registryUsername`) |
| `registryPassword` | string | 任意 | プライベートレジストリのパスワード (API の `registryPassword`) |
| `exposedPorts` | array | 任意 | 公開ポートの設定 (API の `exposedPorts`) |
| `env` | array | 任意 | 環境変数 (API の `env`) |

`exposedPorts` の要素:

| フィールド | 型 | 説明 |
| ---------- | -- | ---- |
| `targetPort` | integer | コンテナがリッスンするポート (API の `targetPort`) |
| `loadBalancerPort` | integer | ルーティング先のロードバランサのポート。未指定はロードバランサ経由で公開しない (API では null) |
| `useLetsEncrypt` | boolean | Let's Encrypt を利用するか (API の `useLetsEncrypt`) |
| `host` | array of string | ホスト名ルーティングのホスト名 (API の `host`)。`loadBalancerPort` 指定時は 1〜5 件が必須、未指定時は指定できません |
| `healthCheck` | object | `path`、`intervalSeconds`、`timeoutSeconds` (API の `healthCheck`) |

`env` の要素:

| フィールド | 型 | 説明 |
| ---------- | -- | ---- |
| `key` | string | 変数名 (API の `key`) |
| `value` | string | 値 (API の `value`) |
| `secret` | boolean | 秘密情報かどうか (API の `secret`)。`true` のとき値は API から取得できず、このバージョンの作成で値を指定しなかった場合は前のバージョンの値が引き継がれます (SDK 同梱の OpenAPI 定義の記載) |

値の範囲 (ポート番号、閾値、CPU・メモリの上限など) は API が検証する内容のため、高レベル層では再実装しません ([ADR 0047](../adr/0047-minimize-cli-request-body-validation.md))。数値の単位や範囲は公式マニュアルと SDK 同梱の OpenAPI 定義を参照してください。

### 定義の読み込みと検証

CLI 層の `definition_input.go` が読み込みを担います。

- `--app` で指定したファイルまたは標準入力からバイト列を読み込みます。
- 未知のフィールドを検出したらエラーにします。Jsonnet がなく単純な JSON であるため、typo を確実に検出できます。末尾の余分なデータもエラーにします。
- 検証は service 層の `ApplicationDefinition` に対して行い、操作の種類で必須項目を分けます。参照・停止・削除系の操作 (`status`、`versions`、`containers`、`rollback`、`deactivate`、`delete`) は `cluster` と `name` を必須とし、デプロイ内容を含まない定義でも読み込めるようにします。`init` がバージョンを持たないアプリケーションに生成する識別子だけの定義も、これらの操作でそのまま使えます。デプロイ系の操作 (`deploy`、`render`、`diff`) はそれに加えて `cpu`、`memory`、`scalingMode`、`image` を必須とします。`scalingMode` が `manual` の場合は `fixedScale` を、`cpu` の場合は `minScale` と `maxScale` を必須とします (`scaleInThreshold`、`scaleOutThreshold` は任意)。`exposedPorts` の要素で `loadBalancerPort` を指定した場合は `host` が必須、未指定の場合は `host` が指定不可であることを検証します。この検証は API の制約の再実装ではなく、CLI 経路で `host` の取り違えや欠落を確実に防ぐためのものです ([ADR 0075](../adr/0075-high-level-command-tier.md) の追加検証の判断基準に従います)。
- 定義の未指定 (省略) は API の nullable フィールドで null に変換します。`registryPasswordAction` は定義には現れず、service 層が deploy の文脈から決定します (後述)。

定義の例:

```json
{
  "cluster": "my-cluster",
  "name": "my-app",
  "cpu": 1000,
  "memory": 1000,
  "scalingMode": "manual",
  "fixedScale": 1,
  "image": "ghcr.io/example/app:v1.0.0",
  "exposedPorts": [
    {
      "targetPort": 8080,
      "loadBalancerPort": 80,
      "host": ["app.example.com"]
    }
  ],
  "env": [
    { "key": "LOG_LEVEL", "value": "info", "secret": false }
  ]
}
```

## deploy の流れ

`Deploy` は次の順序で SDK 操作を組み合わせます。参考実装の `runDeploy` と同じ流れです。

1. 定義の検証を済ませ、クラスタ名でクラスタを検索します (`cluster.ClusterAPI.List` のページング)。見つからなければエラーにします。
2. アプリケーション名でアプリケーションを検索します (`application.ApplicationAPI.List` のページング)。同名のアプリケーションはクラスタ内で一意であるため、一覧の名前一致に対して `Read` で `ClusterName` を照合し、クラスタが一致するものだけを対象にします。
3. 見つからない場合はアプリケーションを作成し (`Application.Create`)、`Read` で詳細を取得します。
4. 定義からバージョン作成リクエスト (`version.CreateParams`) へ変換します。`registryPasswordAction` は次の表のとおりに決定します。
5. バージョンを作成し (`Version.Create`)、返されたバージョン番号で `Application.Update` を呼び出して有効化します。
6. 待機が有効なら、コンテナの状態をポーリングしてデプロイの完了を待ちます。

`registryPasswordAction` の決定:

| 条件 | 値 |
| ---- | -- |
| 定義に `registryPassword` の非空値がある | `new` (新しいパスワードを設定) |
| それ以外でアクティブバージョンがある | `keep` (既存のパスワードを維持) |
| それ以外 (初回デプロイ) | `remove` |

`registryPasswordAction` の決定は deploy のたびに必要な判断であり、誤るとパスワードが消去 (`remove`) されるため、service 層に集めてテストで固定します。`registryPassword` はリクエスト・ログ・出力に含めません。

### 待機処理

待機は `Application.Containers` を既定 5 秒間隔でポーリングし、作成したバージョンのコンテナがすべて `running` 状態になったら完了とします。参考実装と同じく、対象バージョンのコンテナ数が 0 より大きく、そのすべてが `running` であることを条件とします。ポーリング中の参照エラーは打ち切らず、期限まで再試行します。デプロイ待ちの間の実際の配置数 (`Placement.Desired`) との照合は、ライブ観察ができる時点で条件の過不足を確認し、必要なら本書を更新します。

- `--no-wait` で待機を省略します。既定は待機します。
- `--wait-timeout` で期限を指定します。既定は 5 分です。期限を超えたらエラーにします。有効化自体は完了しているため、エラーメッセージに `status` や `containers` で状態を確認できることを示します。
- 待機中の進行状況は標準エラー出力のログで示します。

rollback、deactivate、delete も同じポーリング機構を使います。

- rollback: 有効化後のデプロイ完了待ち (deploy と同じ条件)。
- deactivate: `Application.Update` でバージョンを解除した後、`Read` の `DesiredCount` が 0 になったら完了。`DesiredCount` はアクティブバージョンの解除後に null を返すことがあるため、null も停止完了として扱います。
- delete: 非活性化が必要なら行い、停止を待ち、その後 `Application.Delete` を呼び出します。API が削除条件 (アクティブバージョンなし、実行中コンテナなし) を満たすまで即座に失敗する場合は、期限付きで再試行します。

## rollback のバージョン選択

`rollback` は有効バージョンを切り替えます。確認プロンプト (`--force` で省略) を出します。

- アプリケーションが活性 (アクティブバージョンあり) の場合、既定の対象はアクティブバージョンより前のバージョンのうち最大のものです。`--target` でバージョン番号を指定できます。
- アプリケーションが非活性 (`deactivate` 済) の場合、既定の対象は最新のバージョンです。停止したアプリケーションの再開に使えます。`--target` でバージョン番号を指定できます。
- 対象バージョンがすでにアクティブな場合はエラーにします。
- バージョンの存在は `Version.List` のページングで確認します。対象が存在しなければ変更 API を呼び出しません。

`rollback` は新しいバージョンを作成しません。既存バージョンの有効化だけを行います。

## deactivate と delete

- `deactivate` はアプリケーションを停止します。アクティブバージョンを解除し、コンテナを停止します。アプリケーションとバージョンは残ります。確認プロンプト (`--force` で省略)、停止待ち (`--no-wait` で省略) を持ちます。すでに非活性のときは変更せず、その旨を出力して成功とします。
- `delete` はアプリケーションとすべてのバージョンを削除します。アクティブバージョンがあれば先に解除し、停止を待ってから削除します。確認プロンプト (`--force` で省略)、停止待ち (`--no-wait` で省略) を持ちます。削除の成否は削除後の `Read` が見つからないこと (不在) で確認します。
- 非活性化・削除の直前に `Read` で現在値を取得し、定義の `cluster`、`name` と対象の `ClusterName`、`Name`、ID が一致することを確認します。不一致なら変更 API を呼び出しません。

## init の定義生成

`init` は既存アプリケーションから定義ファイルを生成します。デプロイ済みのアプリケーションをこの CLI で管理し始める入口です。

- 位置引数でクラスタ名とアプリケーション名を指定します。`--output-file` で出力先ファイルを指定します (既定はカレントディレクトリの `application.json`)。
- アプリケーションを検索し、アクティブバージョンがあればその `Version.Read` の結果から定義を復元します。アクティブバージョンがなければ最新のバージョンから復元します。バージョンが1つもなければ、識別子だけの定義を生成します。
- 出力は1ファイルの JSON です。インデント付きの JSON を書き出します。Jsonnet のフォーマッタは使わず、JSON として整形します。生成されたファイルのパスは `deploy` などの `--app` にそのまま指定できます。1つのディレクトリに複数のアプリケーションの定義を置けます (`--app` で切り替えます)。
- 同名のファイルが既に存在する場合は上書きせず、エラーにします。既存の定義を壊さないための挙動です。
- `registryPassword` と `secret=true` の環境変数の値は API から取得できないため、生成される定義には含めません。含まれないことを出力またはコメントで明示します。

## diff の比較

`diff` はローカル定義と、デプロイ済みの現状 (アクティブバージョン。なければ識別子のみ) を比較します。

- 現状は `Version.Read` の結果を定義と同じ形式に正規化して比較します。正規化は service 層の `VersionDetail` → `ApplicationDefinition` 変換で行います。
- 比較から除外するフィールド:
  - `registryPassword`: 応答ではマスクされ、値が取得できません。
  - `secret=true` の環境変数の `value`: API は null を返します。
  - `registryPasswordAction`: 定義に現れない service 層の判断フィールドです。
- 除外するフィールドを差分に出力へ含めず、除外したことを標準エラー出力のログで示します。
- アプリケーションが存在しない場合、識別子のみの定義を現状として差分を表示します (初回デプロイで作成される内容の確認)。
- 出力は unified diff 形式のテキストです。JSON 出力方針 ([ADR 0030](../adr/0030-remove-table-output.md)) の例外として、人間が変更前後を目で確認する用途のテキストに限定します。`--query` には対応せず、指定した場合はエラーにします。差分がない場合はその旨を標準エラー出力に示し、標準出力には何も書きません。
- hunk の文脈は GNU diff の unified 形式に合わせます。次の変更までの間隔が `2*context` 行以下のときは 1 つの hunk に統合し、それより長いときは文脈が重ならない位置で hunk を分けます (同じ行を 2 度出力しません)。ランダム生成した入力で GNU diff との一致をテストで確認します。
- diff 文字列の生成は Go 標準ライブラリベースの実装で行うか、ライブラリの採用は実装時に選定します。色付き出力は標準出力が端末のときだけ行います。

`render` は定義を読み込み、検証と正規化 (`cluster`、`name` を含む定義の正規形) を行った結果を JSON で出力します。差分のローカル側と同じ形式です。API に送る内容の確認 (`registryPasswordAction` を含むバージョン作成リクエストの形状) は実装後に必要性を判断し、別オプションにする場合は本書へ追記します。

## CLI コマンド

### コマンドツリー

```text
skr apprun-dedicated init CLUSTER APPLICATION [--output-file FILE]

skr apprun-dedicated deploy --app FILE|- [--no-wait] [--wait-timeout DURATION]
skr apprun-dedicated diff --app FILE|-
skr apprun-dedicated render --app FILE|-
skr apprun-dedicated status --app FILE|-
skr apprun-dedicated versions --app FILE|-
skr apprun-dedicated containers --app FILE|-

skr apprun-dedicated rollback --app FILE|- [--target VERSION] [--force] [--no-wait] [--wait-timeout DURATION]
skr apprun-dedicated deactivate --app FILE|- [--force] [--no-wait] [--wait-timeout DURATION]
skr apprun-dedicated delete --app FILE|- [--force] [--no-wait] [--wait-timeout DURATION]
```

末端コマンドは 10 個です。位置引数は `init` のクラスタ名とアプリケーション名のみです ([ADR 0075](../adr/0075-high-level-command-tier.md) に従い、対象を表す値は位置引数にします)。認証は低レベル層と同じ SDK のプロファイル機構を使い、グローバルフラグと共通の出力処理を接続します。

### フラグ一覧

| フラグ | コマンド | 説明 |
| ------ | -------- | ---- |
| `--app FILE\|-` | init 以外 | 定義ファイル (常に必須、既定値なし)。`-` で標準入力 |
| `--output-file FILE` | init | 出力先ファイル (既定 `application.json`)。既存ファイルは上書きしない |
| `--target VERSION` | rollback | 有効化するバージョン番号 |
| `--force` | rollback、deactivate、delete | 確認プロンプトを省略。標準入力が端末でない場合や `--app -` と併用する場合は必須 |
| `--no-wait` | deploy、rollback、deactivate、delete | 待機を省略 |
| `--wait-timeout DURATION` | deploy、rollback、deactivate、delete | 待機の期限 (既定 5 分) |

### 確認プロンプト

`rollback`、`deactivate`、`delete` は、変更する対象のクラスタ名、アプリケーション名、ID、現在の状態を表示して確認を求めます。`--force` で省略します。確認プロンプトの入力読み込みは CLI 層の責務で、service 層は確認済みの入力を受け取ります。

確認の入力は標準入力から読みますが、標準入力が端末でない場合 (パイプ、リダイレクト) はプロンプトを出せないため `--force` が必須です。指定がなければ操作を開始せず、`--force` の指定を促すエラーにします。`--app -` で標準入力から定義を読む場合も、定義の読み込みで標準入力を EOF まで消費するため、変更系操作では必ず `--force` を指定します。

## 出力設計

出力は JSON で、`--query` による絞り込みを低レベル層と同じ方式で受け付けます ([ADR 0030](../adr/0030-remove-table-output.md))。進行状況のログは標準エラー出力に書き、標準出力は結果の JSON だけを含めます。JSON 以外の出力を持つ `init`、`diff`、`delete` は `--query` を受け付けず、指定した場合はエラーにします (指定を暗黙に無視しない)。

init は既存ファイルの保護を `O_EXCL` 付きの作成で行います。存在確認と書き込みの間に別のプロセスが同じパスへ書き込む競合を避け、上書きを防止します。

- `deploy`、`rollback`、`deactivate`: 操作後の状態をオブジェクトで出力します。クラスタ名、アプリケーション名、アプリケーション ID、有効化したバージョン番号 (deactivate では解除したことを表す値)、作成の有無 (deploy の新規作成時) を含みます。SDK の操作応答の本文は多くの操作で空のため、service 層は操作後の `Read` 結果から結果オブジェクトを組み立てます。
- `delete`: 出力しません (低レベルの delete と同じ)。
- `status`: アプリケーションの状態を出力します。`application` (ID、名前、アクティブバージョン、DesiredCount) と、アクティブバージョンがあればその `activeVersionDetail` (バージョン番号、イメージ、CPU、メモリ、スケーリング方式、アクティブノード数) を含みます。射影から `registryPassword` を除外します。アクティブバージョンがなければ `activeVersionDetail` を null にします。
- `versions`: バージョンの配列を出力します。各要素は `version`、`image`、`activeNodeCount`、`active`、`created` (RFC3339 形式の UTC) を含みます。`active` はアプリケーションのアクティブバージョンとの一致です。
- `containers`: コンテナの配置 (`Application.Containers` の応答) を出力します。SDK の応答モデルをそのまま出力します。
- `render`: 正規化した定義の JSON を出力します。
- `diff`: unified diff のテキスト (前述)。
- `init`: 出力なし。書き出したファイルのパスを標準エラー出力のログで示します。

`status` と `versions` の射影は service 層の `output.go` が提供します。`containers` は SDK 応答をそのまま出力するため射影を設けません。

## 秘密情報の扱い

- `registryPassword` と `secret=true` の環境変数の値は、定義ファイル (または標準入力) からのみ入力します。フラグは設けません。
- service 層は秘密情報をログ・エラー・出力に含めません。デプロイのリクエスト構築で秘密情報を扱う処理は、値を構造体として受け渡します。
- SDK のトレース実装はリクエスト本文を出力します。デプロイ (`registryPassword` や `secret=true` の環境変数を含み得る) では、EventBus と同じ方式で、CLI 層の ServiceFactory が SDK の実効トレース設定 (`TraceMode` など) を検査します。トレースが有効なら、定義の読み込み・送信の前にエラーを返します。検査は必須の設計要素であり、省略しません ([EventBus 高レベルコマンドの設計](eventbus.md) の secret の扱いと同じ判断です)。
- `status` と `versions` の射影から `registryPassword` を除外します (`secret=true` の環境変数の `value` は API が null を返すため、そのまま出力します)。
- 定義ファイルに秘密情報を書く場合のファイル権限への注意をヘルプとチュートリアルに記載します。標準入力 (`--app -`) は CI などでファイルを作らずに済む経路として案内します。
- Jsonnet のネイティブ関数 (`env`、`secret`) がなく、定義内に秘密情報を平文で書かざるを得ない点は、Jsonnet 非対応の制約として ADR に記録済みです ([ADR 0079](../adr/0079-apprun-dedicated-high-level-commands.md))。

## エラー設計

- 定義の検証エラーは API を呼び出す前に返します。エラーメッセージは定義のフィールド名と API のフィールドで説明し、SDK の内部型は表示しません。
- クラスタ・アプリケーションの検索で対象が見つからない場合、deploy は新規作成へ進み、それ以外の操作はエラーとして返します。
- 変更系の操作は、実行直前の再照合 (`Read`) に失敗した場合や、対象の所属・名前が一致しない場合は変更 API を呼び出しません。
- 待機のタイムアウトはエラーにします。有効化などの変更が完了済みであることをメッセージに含めます。
- SDK エラーは分類を維持して伝播し、既存の CLI の方式で表示します。

## テスト方針

### service 層の単体テスト

テストは service/apprun-dedicated-service-go モジュールに置き、CLI 入出力に依存しません。SDK の操作 interface に fake を注入します。

- 正常系: クラスタ・アプリケーションの名前検索 (ページングを含む) と、定義から `CreateParams` への変換を確認します。`registryPasswordAction` の 3 パターン (`new`、`keep`、`remove`) も、バージョン作成と有効化の順序も、rollback のバージョン選択 (活性・非活性・`--target`) も、deactivate・delete の再照合と順序も確認します。
- 待機: ポーリング間隔、完了条件 (対象バージョンの全コンテナ `running`)、期限での打ち切り、ポーリング中の参照エラーの継続、`--no-wait` でポーリングしないこと。fake の時計・ticker を差し替え可能にして検証します。
- エラー系: 必須フィールドの欠落、`scalingMode` とスケール設定の整合、`loadBalancerPort` と `host` の整合、クラスタ不在、同名アプリの他クラスタ混在、削除前の再照合失敗で削除 API を呼ばないこと、タイムアウト。
- 変換: `VersionDetail` → 定義の正規化 (diff・init 用)、`registryPassword` と secret 環境変数の除外、`created` の RFC3339 変換。
- 安全性: deploy で `registryPassword` が fake に渡ったリクエスト以外の場所に現れないこと、trace 有効時 (CLI 層の ServiceFactory での検査) の拒否。

### CLI 層の単体テスト

`internal/apprundedicated` の単体テストで検証します。

- 定義の読み込み: `--app` の既定値、ファイル不在、`--app -` の標準入力、未知フィールドの拒否、末尾データの拒否、必須欠落のエラー文言。
- 確認プロンプト: 端末での確認 (y で続行、n で中止)、非端末と `--app -` で `--force` なしの場合に操作を開始しないこと、`--force` で読み込みを行わないこと。
- init: 既存ファイルがある場合に上書きしないこと、出力パス。

### CLI 全体と SDK 通信のテスト

- `internal/cli` の sakumock テストで、10 個の末端コマンドの正常系を実際の CLI パスと SDK を通して検証します。sakumock の `apprundedicated` サーバー (v0.14.0) を使います。既存の低レベル層のテスト (`internal/cli/apprundedicated_test.go`) と同じサーバーを共有します。
- sakumock で待機条件 (コンテナの `running` 遷移) が再現できない場合は、SDK が読むステータスと応答型を再現する HTTP テストサーバーで補います (`SAKURA_ENDPOINTS_*` によるエンドポイント差し替え)。[EventBus 高レベルコマンドの設計](eventbus.md) のテスト方針と同じ方式です。
- service 層の通信テストは service/apprun-dedicated-service-go に置き、sakumock へ実際の SDK を接続して wire 形式と操作の流れを検証します。
- 秘密情報: 定義ファイル・標準入力・fake リクエストのいずれの経路でも、stdout・stderr・ログ・エラーに `registryPassword` が現れないことを確認します。トレース有効時 (環境変数・プロファイル) に秘密情報を含む操作が拒否されることを確認します。

### ライブ E2E

クラスタ作成を含むライフサイクルシナリオを `test/e2e/apprun-dedicated/scenario_lifecycle_test.go` に実装済みです ([ADR 0058](../adr/0058-live-e2e-command-coverage.md)、[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)、[ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md) に従う testing.T ベースのシナリオ)。2026-11-12 にクラスタと ASG (ワーカノード 1 台) の作成を含むライブ実行で全操作の正常動作と後片付けを確認済みで、`baseline.txt` の高レベル 10 コマンドは `live` です。シナリオの概要:

- 事前の同名確認と、作成直後の SDK による読み戻し (クラスタは ID・名前、ASG は ID・名前・ゾーン・サービスクラス・ノード数・インターフェース) を行います。
- `deploy` (バージョン 1)、`status`、`versions`、`containers`、`init` での参照、環境変数を追加した 2 回目の `deploy` (バージョン 2)、`rollback`、`deactivate`、`delete` の順に実行し、応答と状態を照合します。
- アプリケーション、ASG、クラスタは保持した ID と識別情報を再照合したうえで `defer` で CLI 経路から削除し、非同期の削除は不在になるまで期限付きで再確認します。サービスプリンシパル ID は環境変数 `SKR_E2E_APPRUN_DEDICATED_SERVICE_PRINCIPAL_ID` で渡し、未設定のときはスキップします。
- アプリケーションの後片付けは deploy の成否にかかわらず、名前と所属クラスタで再検索して確定できた ID に対してのみ実行します (`cleanupApplicationAfterDeploy`)。deploy はアプリ作成後の反映待ちで失敗することがあるため、deploy より前に `defer` で登録します。

deploy の反映待ち失敗時の後片付けを `test/e2e/apprun-dedicated/scenario_deploy_failure_test.go` に実装済みです。オートスケーリンググループを作らないためワーカノードの料金は発生しません。再検索の判定 (作成済み、未作成、別クラスタの同名、同名の複数、照会失敗) はスタブによる単体テスト (`TestFindCreatedApplication`) で確認します。

低レベル `apprun-dedicated-api` コマンドのライブ E2E はクラスタ作成を前提とするため [#59](https://github.com/sacloud/skr/issues/59) で管理します (ライフサイクルシナリオのセットアップと後片付けは低レベルの create/delete を利用しますが、各操作の専用検証は別途整備します)。

## ドキュメントと PLAN

- チュートリアル `docs/manual/tutorials/apprun-dedicated.md` は、実装と sakumock 検証の完了後に作成します。`skr apprun-dedicated` のコマンド、定義ファイルの書き方、デプロイからロールバック・削除までの流れ、低レベル `apprun-dedicated-api` との対応を含めます。
- `PLAN.md` の `apprun-dedicated` の節に、高レベル層の実装範囲、ライブ E2E、チュートリアルの対応状況を区別して記録します。
- 低レベル `apprun-dedicated-api` の設計文書に高レベル層への誘導を追記します。

## 将来課題

- `verify` (依存リソースの妥当性確認) や単独の `wait` コマンドなど、ecspresso 由来の追加操作。要望が明確になった時点で設計します。
- クラスタロードバランサの参照・更新 (`read-cluster-load-balancer`、`patch-cluster-load-balancer`) と limits の参照 (`read-limits`) は低レベル層の課題として [#202](https://github.com/sacloud/skr/issues/202) で対応します。高レベル層では扱いません。
- Jsonnet など定義の生成支援は対象外です ([ADR 0079](../adr/0079-apprun-dedicated-high-level-commands.md))。利用手順が固まった後、外部テンプレートの案内だけを行うかどうかを再検討します。
- 待機条件の `Placement.Desired` との照合、ポーリング間隔と既定のタイムアウト値は、ライブ観察が可能になった時点で見直します。
- `render` で `registryPasswordAction` を含むバージョン作成リクエストの確認表示が必要かどうかは、実装後の利用確認で判断します。
