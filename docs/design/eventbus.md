# EventBus 高レベルコマンドの設計

- Status: 実装完了 (ライブ E2E 成功)
- ADR: [ADR 0075](../adr/0075-high-level-command-tier.md)、[ADR 0076](../adr/0076-eventbus-trigger-condition-option.md)
- Issue: [#214](https://github.com/sacloud/skr/issues/214)
- SDK: `github.com/sacloud/sacloud-sdk-go` v0.5.0

関連 ADR の方針に従い実装しました。service 層と条件パーサー、CLI 接続、秘密入力処理、E2E シナリオ、チュートリアルを実装し、ライブ E2E も成功しています。

## 目的と範囲

EventBus の高レベルコマンド `skr eventbus` を設計します。`eventbus-api` は CommonServiceItem が入出力に直接現れるため、利用者は `Settings` の JSON を種別 (実行設定・スケジュール・トリガー) ごとに組み立てる必要があります ([#214](https://github.com/sacloud/skr/issues/214))。高レベルコマンドは、公式マニュアルと SDK の仕様で確認できた入力をフラグ化し、作業単位の検証と変換を service 層に集めます。

この文書の対象は次のとおりです。

- `internal/eventbus` パッケージの service 層 (eventbus-service 相当) と CLI コマンド
- 実行設定 (process-configuration)、スケジュール (schedule)、トリガー (trigger) の操作

対象外は次のとおりです。

- 低レベル `eventbus-api` の変更。現行のコマンドと JSON 経路を維持します。
- 定義ファイルを使うワークフロー (init、diff、apply など)。将来課題として扱います。

## 全体構成

### パッケージ構成

`internal/eventbus` (package `eventbus`) に、service 層と CLI コマンドを置きます。

```text
internal/eventbus/
  service.go               # Service 構造体、New(caller)、共通の変換
  process_c0nfiguration.go  # 実行設定のリクエスト型、検証、変換、操作
  schedule.go              # スケジュール
  trigger.go               # トリガー (TriggerCondition を含む)
  commands.go              # kong コマンド (service のリクエストを組み立てる)
  output.go                # 出力射影
  *_test.go
```

### 依存規律

service 層のファイル (`service.go`、`process_c0nfiguration.go`、`schedule.go`、`trigger.go`) は `sacloud-sdk-go` と標準ライブラリのみに依存し、kong、CLI の Runtime、出力処理には依存しません。同じパッケージ内でも CLI 側の型やヘルパーは参照しません。ファイル名は `config` を `c0nfig` に置き換えるリポジトリの規則に従います。

`internal/cli` が `NewCommands(runtime, factory)` でコマンド群の依存を一括注入し、SDK クライアントの生成と共通の出力処理を接続します。`internal/eventbus` から `internal/cli` には依存せず、段階的な setter 初期化は行いません。

### Service の構造

service 層は SDK の service 層の構成 ([service/iaas/swytch](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0/service/iaas/swytch) など) を参考にします。本書のメソッド名や型が SDK の規約と同一であることは前提にしません。

- `Service` 構造体は SDK の公開操作 interface (`eventbus.ProcessConfigurationAPI`、`eventbus.ScheduleAPI`、`eventbus.TriggerAPI`) を保持します。`New(caller *v1.Client)` が種別ごとの操作を構築します。同じパッケージの単体テストでは各 interface に fake を注入します。
- 操作ごとのリクエスト型が、検証と API リクエスト (`v1.CreateCommonServiceItemRequest` など) への変換を持ちます。
- 戻り値は SDK の応答モデル (`*v1.CommonServiceItem` など) とします。出力射影は行いません。

### SDK への移植手順

SDK に `service/eventbus` が追加された場合は、次の事項を確認して切り替えを判断します。SDK 側への提案・採用や、ファイル移動だけでの移植は保証しません。

1. SDK の公開 API、検証、未指定値、マージ、エラー、context の扱いを本設計と照合します。
2. CLI の入力・出力契約を維持できる場合に、factory とリクエスト変換を SDK へ接続し替え、不要になった service 層の実装を削除します。
3. 単体テストと実際の SDK を通す通信テストで差異を検証し、必要なライブ E2E を承認後に再実行します。
4. 本設計文書と ADR を更新します。

## Service 層 API

### メソッド

```go
func New(caller *v1.Client) *Service

// 実行設定
func (s *Service) ListProcessConfigurations(ctx context.Context) ([]v1.CommonServiceItem, error)
func (s *Service) ReadProcessConfiguration(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error)
func (s *Service) CreateProcessConfiguration(ctx context.Context, req *CreateProcessConfigurationRequest) (*v1.CommonServiceItem, error)
func (s *Service) UpdateProcessConfiguration(ctx context.Context, req *UpdateProcessConfigurationRequest) (*v1.CommonServiceItem, error)
func (s *Service) DeleteProcessConfiguration(ctx context.Context, req *DeleteRequest) error
func (s *Service) SetProcessConfigurationSecret(ctx context.Context, req *SetProcessConfigurationSecretRequest) error

// スケジュール
func (s *Service) ListSchedules(ctx context.Context) ([]v1.CommonServiceItem, error)
func (s *Service) ReadSchedule(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error)
func (s *Service) CreateSchedule(ctx context.Context, req *CreateScheduleRequest) (*v1.CommonServiceItem, error)
func (s *Service) UpdateSchedule(ctx context.Context, req *UpdateScheduleRequest) (*v1.CommonServiceItem, error)
func (s *Service) DeleteSchedule(ctx context.Context, req *DeleteRequest) error

// トリガー
func (s *Service) ListTriggers(ctx context.Context) ([]v1.CommonServiceItem, error)
func (s *Service) ReadTrigger(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error)
func (s *Service) CreateTrigger(ctx context.Context, req *CreateTriggerRequest) (*v1.CommonServiceItem, error)
func (s *Service) UpdateTrigger(ctx context.Context, req *UpdateTriggerRequest) (*v1.CommonServiceItem, error)
func (s *Service) DeleteTrigger(ctx context.Context, req *DeleteRequest) error
```

list は種別ごとの SDK 操作の `List` を呼び出します。SDK は `Provider.Class` のフィルターを設定して `ListCommonServiceItems` を呼び出します。高レベル CLI に `--provider-class` は設けません。共通リクエスト型は次のとおりです。

```go
type ReadRequest struct {
    ID string
}

type DeleteRequest struct {
    ID string
}

type SetProcessConfigurationSecretRequest struct {
    ID     string
    Secret v1.SetSecretRequestSecret
}
```

### 実行設定のリクエスト型

```go
type CreateProcessConfigurationRequest struct {
    Name        string
    Description string
    Settings    ProcessConfigurationSettings
}

type ProcessConfigurationSettings struct {
    Destination string // simplenotification | simplemq | autoscale
    GroupID     *string // simplenotification 用 (API の group_id)
    Message     *string // simplenotification 用 (API の message)
    QueueName   *string // simplemq 用 (API の queue_name)
    Content     *string // simplemq 用 (API の content)
    Action      *string // autoscale 用 (scale_up | scale_down)
    ResourceID  *string // autoscale 用 (API の resource_id)
}
```

宛先ごとの `Parameters` への変換は次のとおりです。

| 宛先 | フラグで指定するフィールド | `Parameters` のキー |
| ---- | ------------------------ | ------------------- |
| simplenotification | `GroupID`、`Message` | `group_id`、`message` |
| simplemq | `QueueName`、`Content` | `queue_name`、`content` |
| autoscale | `Action`、`ResourceID` | `action`、`resource_id` |

対応は [SDK の実行設定ヘルパー](https://github.com/sacloud/sacloud-sdk-go/blob/v0.5.0/api/eventbus/process_configurations.go)で確認しています。`Parameters` は API 上では JSON を格納した**文字列**です。SDK のヘルパーと同じキーでオブジェクトを JSON 化し、その文字列を設定します。上表の各キーは未指定時に空文字列を使い、明示した空文字列との指定有無は宛先の整合性検証に使います。

検証は次のとおりです。

- create では `Name` と `Destination` を必須にします。
- `Destination` は 3 種のいずれかであることを検証します。
- 指定した宛先に属さないフィールドが指定されていたら、空文字列でもエラーにします。nil を未指定とし、値だけで指定有無を判定しません。
- 宛先ごとの必須キーの有無は検証しません。API が検証する内容であり、クライアント側で追加の制約を作りません。

update のリクエスト型は、未指定 (nil) を「現状維持」として扱います。

```go
type UpdateProcessConfigurationRequest struct {
    ID          string
    Name        *string
    Description *string
    Destination *string
    GroupID     *string
    Message     *string
    QueueName   *string
    Content     *string
    Action      *string
    ResourceID  *string
}
```

### スケジュールのリクエスト型

```go
type CreateScheduleRequest struct {
    Name        string
    Description string
    Settings    ScheduleSettings
}

type ScheduleSettings struct {
    ProcessConfigurationID string
    StartsAt               time.Time
    RecurringStep          *int    // RecurringUnit と組で指定
    RecurringUnit          *string // min | hour | day
    Crontab                *string
}
```

検証は次のとおりです。

- create では `Name`、`ProcessConfigurationID`、`StartsAt` を必須にします。
- 周期指定 (`RecurringStep`、`RecurringUnit` の組) は `Crontab` と排他です。どちらか一方だけを完全に指定します。両方を指定した場合、周期指定の片方だけを指定した場合はエラーにします。
- `RecurringUnit` は `min`、`hour`、`day` のいずれかであることを検証します。

`StartsAt` は `time.Time` として受け付け、service 層が API リクエスト用のエポックミリ秒 (整数) に変換します。CLI は `--starts-at` に RFC3339 形式を受け付け、`time.Time` に変換して渡します。日時は利用者が意図した時点 (タイムゾーン付き) として扱います。ミリ秒未満は切り捨てます。`Crontab` は API への受け渡しと読み戻しに対応し、クライアントでの時差補正は行いません。

評価タイムゾーンについては、JST (日本標準時、UTC+09:00) であるとの利用者の確認結果を受領しています。公開ヘルプとチュートリアルへの記載時には一次情報の参照を追加します。StartsAt との発火上の関係は引き続き未確認です。ライブでの実発火検証は今回の対象に追加しません。

update のリクエスト型は次のとおりです。

```go
type UpdateScheduleRequest struct {
    ID                     string
    Name                   *string
    Description            *string
    ProcessConfigurationID *string
    StartsAt               *time.Time
    RecurringStep          *int
    RecurringUnit          *string
    Crontab                *string
}
```

周期指定のモード切り替えは、マージ規則 (後述) に従います。

### トリガーのリクエスト型

```go
type CreateTriggerRequest struct {
    Name        string
    Description string
    Settings    TriggerSettings
}

type TriggerSettings struct {
    Source                 string
    Types                  []string           // nil = 指定なし (全タイプ)
    Conditions             []TriggerCondition // nil = 条件なし
    ProcessConfigurationID string
}

type TriggerCondition struct {
    Key    string
    Op     string // eq | in
    Values []string
}
```

検証は次のとおりです。

- create では `Name`、`Source`、`ProcessConfigurationID` を必須にします。
- `TriggerCondition.Key` は API と同じ条件で検証します。Go では `^[a-z0-9]{1,20}$` の照合と `data` の除外を組み合わせます。
- `Op` は `eq` または `in` です。`eq` は `Values` がちょうど 1 件、`in` は 1 件以上であることを検証します ([ADR 0076](../adr/0076-eventbus-trigger-condition-option.md))。
- 新しい入力の `Types` と `Conditions` は nil または非空スライスを許し、非 nil で要素 0 件のスライスはエラーにします。この入力契約は API の配列長制約ではなく、高レベル層が「指定なし」と「全体置換」を区別するためのものです。`--type` の空文字列もエラーにします。既存値の保持にはこの制約を遡及適用しません。

update のリクエスト型は次のとおりです。

```go
type UpdateTriggerRequest struct {
    ID                     string
    Name                   *string
    Description            *string
    Source                 *string
    Types                  []string           // nil = 現状維持、非 nil = 全体置換
    Conditions             []TriggerCondition // nil = 現状維持、非 nil = 全体置換
    ProcessConfigurationID *string
}
```

トリガーの送信元 (Source) は次の URI を受け付けます ([EventBus のイベント](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html))。高レベル層が対応するソースとして完全一致で検証します。API 全体の許可リストとして一般化せず、新しいソースへの対応は仕様を確認して追加します。

- `//eventbus.sakura.ad.jp/monitoringsuite/alert`
- `//eventbus.sakura.ad.jp/eventlog`

### API への変換

service 層は入力リクエストを `v1.CreateCommonServiceItemRequest` / `v1.UpdateCommonServiceItemRequest` に変換します。型と null の扱いの根拠は [SDK 同梱の OpenAPI 定義](https://github.com/sacloud/sacloud-sdk-go/blob/v0.5.0/api/eventbus/openapi/openapi.json)です。

- create: `CommonServiceItem.Name`、`Description` (空文字列のとき null)、`Settings`、`Provider.Class` に種別の provider-class (`eventbusprocessconfiguration` など) を設定します。
- update: トップレベルは指定されたフィールドだけを設定します (`OptString`、`OptSettings` など)。`Settings` は内部項目を個別に送らず、後述の規則でマージした全体を送ります。`Description` は空文字列の指定を空文字列として送ります (null への消去は低レベルのみ)。
- `StartsAt` はリクエスト時にエポックミリ秒 (整数) へ変換します。
- create: `Types` と `Conditions` が nil のときは API の null に変換します。update: リクエストの nil は「現状維持」であり、null への変換指示ではありません。既存値の未設定・null・空配列・非空配列を区別して引き継ぎます。
- `Parameters` は宛先ごとのフィールドから JSON オブジェクトを組み立て、その JSON 文字列を設定します。

### 更新のマージ規則

update は read-modify-write です。service 層が現在値を Read API で取得し、設定項目の指定がある場合だけ、指定フィールドを上書きした完全な `Settings` を Update API に送ります。Name や Description だけの更新では `Settings` を送りません。読み取り・既存値の解釈に失敗した場合は更新 API を呼び出しません。

SDK の種別別操作は一覧を絞り込みますが、Read と Delete は共通エンドポイントを ID で操作します。高レベル層は read、update、delete、set-secret で取得結果の ID、`Provider.Class`、`Settings` の種別が対象コマンドと一致することを確認します。delete と set-secret も変更前に Read を行い、不一致・不正応答・取得失敗では変更しません。これは secret 自体を読み取るためではありません。通常の delete は不在も API エラーとして返します。

同時更新に対する原子性は保証しません。Read と Update の間に他の変更が入ると、保持したつもりの設定を古い値で上書きする可能性があります。`SettingsHash` を更新の事前条件として送る仕様は確認できていないため利用せず、この制約を update のヘルプにも明記します。

実行設定の update:

- `--destination` または宛先固有のフィールドを 1 つでも指定したときだけ `Settings` を送ります。すべて未指定のときは `Settings` を送りません。
- 宛先を変更する場合は、現状の宛先固有のフィールドを引き継ぎません。新しい宛先のフィールドだけを指定値から組み立てます。
- 宛先を変更しない場合は、現状の `Parameters` を JSON オブジェクトとして読み、未知のキーとその値も保持して、指定フィールドの対応キーだけを上書きします。値は `json.RawMessage` などで保持し、数値を float64 へ変換して精度を失わないようにします。既知のキーだけを構造体に読み戻して再構築しません。現状の JSON が不正、オブジェクト以外、または変更に必要な既知フィールドが解釈できない場合は更新しません。名前だけの更新では `Parameters` の解析は不要です。
- マージ後の `Settings` に対して、create と同じ検証 (宛先とフィールドの整合) を行います。

スケジュールの update:

- 周期指定のモードは、今回指定したフラグで決まります。`--crontab` を指定すると Crontab 方式になり、周期指定は送りません。`--recurring-step` または `--recurring-unit` を指定すると周期方式になり、Crontab は送りません。
- `--crontab` と周期指定を同時に指定した場合は、優先順位で解決せずエラーにします。
- 周期方式に切り替えたとき、今回指定しなかった片方 (`--recurring-step` だけの指定など) は、現状が周期方式なら現状値を引き継ぎ、現状が Crontab 方式ならエラーにします。
- どちらも指定しないときは、現状のモードと値を維持します。
- マージ後に対して create と同じ検証 (モードの完全性) を行います。
- 既存の `StartsAt` はエポックミリ秒の整数または整数文字列として解釈し、更新リクエストでは整数に直します。文字列のまま再送しません。解釈できない既存値を現在時刻などで補いません。

トリガーの update:

- `--type` を指定したときは `Types` 全体を置き換え、未指定のときは現状を維持します。
- `--condition` を指定したときは `Conditions` 全体を置き換え、未指定のときは現状を維持します ([ADR 0076](../adr/0076-eventbus-trigger-condition-option.md))。
- Source の変更でも未指定の Types と Conditions は保持します。新しいソース向けに変更が必要な場合は、同じ update で明示的に指定します。null への消去が必要な場合は低レベル層を使います。

実行設定の secret 登録 (set-secret) は種別確認後、指定された secret だけを SetSecret API に送ります。既存 secret の読み取りやマージは行いません。

### secret の扱い

実行設定の secret は次のとおり扱います。

- CLI は `--secret-file` だけを受け付けます。値は `@path` 形式 (ファイル) または `-` (標準入力) です。インライン指定や環境変数からの読み込みは行いません。
- ファイルの内容は `Secret` ラッパーのないオブジェクトとし、`{"APIKey":"..."}` または `{"AccessToken":"...","AccessTokenSecret":"..."}` を受け付けます。未知フィールド、両形式の混在、必須値の欠落・空文字列、型違い、末尾の余分なデータを拒否します。既存の入力処理を確認して再利用可能な部分を使い、低レベル層の入力仕様は変更しません。低レベルの `--secret-file PATH` と異なり、高レベル層は `@path` が必須であることをヘルプに記載します。
- service 層は secret を構造体として受け取り、ログやエラーメッセージに含めません。
- secret の現在値は取得できないため、secret 自体の読み取りとマージは行いません。実行設定の種別確認は別途行います。
- SDK の[トレース実装](https://github.com/sacloud/sacloud-sdk-go/blob/v0.5.0/common/saclient/tracer.go)はリクエスト本文を出力します。`--trace` を渡さないだけでは、環境変数・プロファイルのトレース設定が残ります。set-secret は SDK の有効なトレース設定を調べます。有効なら秘密情報の読み込み・送信前にエラーを返します。`all` だけでなくエラー時のみのモードも対象にします。
- JSON デコードエラー、API 応答、ログから入力した秘密情報が漏れないことを検証します。set-secret ではサーバーからのエラー本文も無条件に表示せず、ステータスなど秘密情報を含まない診断情報を返します。

## CLI コマンド

### コマンドツリー

```text
skr eventbus process-configuration list
skr eventbus process-configuration read ID
skr eventbus process-configuration create [flags]
skr eventbus process-configuration update ID [flags]
skr eventbus process-configuration delete ID
skr eventbus process-configuration set-secret ID --secret-file @path|-

skr eventbus schedule list
skr eventbus schedule read ID
skr eventbus schedule create [flags]
skr eventbus schedule update ID [flags]
skr eventbus schedule delete ID

skr eventbus trigger list
skr eventbus trigger read ID
skr eventbus trigger create [flags]
skr eventbus trigger update ID [flags]
skr eventbus trigger delete ID
```

末端コマンドは 16 個です。位置引数は対象リソースの ID とします。認証は低レベル層と同じ SDK のプロファイル機構を使い、グローバルフラグと共通の出力処理を接続します。ただし set-secret と有効なトレース設定の併用は拒否します。各サービス操作へ同じ context を渡し、読み取りと変更の間にキャンセルされた場合も変更を続行しません。

### フラグ一覧

実行設定:

| フラグ | create | update | 説明 |
| ------ | ------ | ------ | ---- |
| `--name` | 必須 | 任意 | リソース名 |
| `--description` | 任意 | 任意 | 説明 (update では未指定時に現状維持) |
| `--destination` | 必須 | 任意 | `simplenotification`、`simplemq`、`autoscale` |
| `--group-id` | 宛先別 | 宛先別 | simplenotification 宛先のグループ ID |
| `--message` | 宛先別 | 宛先別 | simplenotification 宛先のメッセージ |
| `--queue-name` | 宛先別 | 宛先別 | simplemq 宛先のキュー名 |
| `--content` | 宛先別 | 宛先別 | simplemq 宛先のメッセージ本文 |
| `--action` | 宛先別 | 宛先別 | autoscale 宛先のアクション (`scale_up`、`scale_down`) |
| `--resource-id` | 宛先別 | 宛先別 | autoscale 宛先のリソース ID |

宛先別のフラグは `--destination` と整合しない指定 (simplemq 以外の宛先での `--queue-name` など) をエラーにします。

スケジュール:

| フラグ | create | update | 説明 |
| ------ | ------ | ------ | ---- |
| `--name` | 必須 | 任意 | リソース名 |
| `--description` | 任意 | 任意 | 説明 |
| `--process-configuration-id` | 必須 | 任意 | 実行設定の ID |
| `--starts-at` | 必須 | 任意 | 実行開始日時 (RFC3339) |
| `--recurring-step` | 周期方式で必須 | 任意 | 周期の間隔数 |
| `--recurring-unit` | 周期方式で必須 | 任意 | 周期の単位 (`min`、`hour`、`day`) |
| `--crontab` | Crontab 方式で必須 | 任意 | crontab 式 |

create では周期指定 (`--recurring-step` と `--recurring-unit` の組) と `--crontab` が相互排他で、どちらか一方だけを完全に指定します。両方を指定した場合、周期指定の片方だけを指定した場合、どちらも指定しない場合はエラーにします。update でも両方式の同時指定はエラーですが、両方の未指定は現状維持です。

トリガー:

| フラグ | create | update | 説明 |
| ------ | ------ | ------ | ---- |
| `--name` | 必須 | 任意 | リソース名 |
| `--description` | 任意 | 任意 | 説明 |
| `--source` | 必須 | 任意 | 送信元 URI |
| `--type` | 任意 (反復) | 任意 (反復) | 対象のイベントタイプ。指定時は全体置換 |
| `--condition` | 任意 (反復) | 任意 (反復) | 条件 ([ADR 0076](../adr/0076-eventbus-trigger-condition-option.md))。指定時は全体置換 |
| `--process-configuration-id` | 必須 | 任意 | 実行設定の ID |

`--type` と `--condition` は反復可能オプションで、指定順序を保持します。Kong の既定のカンマ分割は無効にし、1回の指定を1要素として扱います。`--condition` 内部のカンマだけを専用パーサーで解釈します。`value` の反復以外の重複キーは拒否し、値にカンマを含めることはできません。

### 検証の分担

- 構文の解析 (`--condition` の `key=value` 形式、`--starts-at` の RFC3339 解析) は CLI が行います。
- 解析後の意味検証 (`key` の pattern、`eq`/`in` の値数、宛先とフィールドの整合、モードの完全性) は service 層が行います。
- ローカルで判定できる入力形式のエラーは API を呼び出す前に返します。既存値が必要な更新では Read 後にマージの整合性を判定し、検証失敗時に変更 API を呼び出しません。
- [ADR 0047](../adr/0047-minimize-cli-request-body-validation.md) に従い、サービス上の値の範囲、宛先の実在性、crontab 式の全仕様、イベントタイプの一覧は再実装しません。本書で追加する検証は、フラグ経路の必須指定、独自構文の変換、宛先別入力の取り違え防止、更新モードの決定、種別確認、秘密情報の安全性に限定します。service 層のエラーは API のフィールドで表し、フラグ名への対応付けは CLI が担います。

## 出力設計

出力は JSON 専用で、`--query` による絞り込みを低レベル層と同じ方式で受け付けます ([ADR 0030](../adr/0030-remove-table-output.md))。list はオブジェクトの配列、read、create、update は単一オブジェクトを出力します。delete と set-secret は出力しません。

出力は CommonServiceItem を種別ごとのリソース形状に射影します。射影の内容は次のとおりです。

- `ID`、`Name`、`Description`、`Tags`、`CreatedAt`、`ModifiedAt` を出力します。
- `Settings` の内容を、種別ごとのフィールドへ展開します。
- `Parameters` は JSON 文字列からオブジェクトへ変換し、宛先ごとの対応表にある非秘密フィールドだけを出力します。未知のキーは更新時には保持しますが、自動的には公開しません。低レベル層では生の応答を確認できます。
- `StartsAt` はエポックミリ秒の整数・整数文字列のどちらも受け取り、ミリ秒精度を失わない RFC3339 形式 (UTC、`time.RFC3339Nano`) の文字列へ変換します。解釈できない値を生の値に切り替えて成功扱いにせず、エラーを返します。
- `Description` の null は null のまま出力します。
- `SettingsHash`、`Status`、`ServiceClass`、`Availability`、`Provider`、`Icon` は出力しません。EventBus の操作手順で用途が確認できていないためです。用途が確認できた時点で追加を検討します。

種別の不一致、不正な `Parameters`、既知の出力フィールドの型違いも射影エラーとして返します。list は全要素を検証・射影してから出力し、途中までの JSON を成功応答として残しません。create や update の API 成功後に射影が失敗する可能性はあるため、その場合は変更後の応答処理が失敗したことと確認できた ID を伝え、自動再送しません。

実行設定の出力例 (simplemq 宛先):

```json
{
  "ID": "123456789012",
  "Name": "job-1",
  "Description": null,
  "Destination": "simplemq",
  "Parameters": {"queue_name": "q-1", "content": "m-1"},
  "Tags": [],
  "CreatedAt": "2026-11-10T01:23:45Z",
  "ModifiedAt": "2026-11-10T01:23:45Z"
}
```

スケジュールの出力例 (周期方式):

```json
{
  "ID": "123456789013",
  "Name": "every-10min",
  "Description": null,
  "ProcessConfigurationID": "123456789012",
  "StartsAt": "2027-01-10T00:00:00Z",
  "RecurringStep": 10,
  "RecurringUnit": "min",
  "Crontab": null,
  "Tags": [],
  "CreatedAt": "2026-11-10T01:23:45Z",
  "ModifiedAt": "2026-11-10T01:23:45Z"
}
```

トリガーの出力例:

```json
{
  "ID": "123456789014",
  "Name": "switch-created",
  "Description": null,
  "Source": "//eventbus.sakura.ad.jp/eventlog",
  "Types": ["jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created"],
  "Conditions": [{"Key": "zone", "Op": "eq", "Values": ["is1b"]}],
  "ProcessConfigurationID": "123456789012",
  "Tags": [],
  "CreatedAt": "2026-11-10T01:23:45Z",
  "ModifiedAt": "2026-11-10T01:23:45Z"
}
```

## エラー設計

- service 層の検証エラーは、変更 API を呼び出す前に返します。ローカルな検証と Read 後の整合性確認を区別します。利用者向けの文言はフラグと対応する API のフィールドで説明し、SDK の内部型は表示しません。
- 通常操作の SDK エラーは分類を維持して伝播し、既存の CLI の方式で表示します。set-secret は秘密情報を含む本文や原因エラーをそのまま表示せず、前述の秘匿規則を適用します。
- 更新のマージで現状の取得に失敗した場合は、更新 API を呼び出さずエラーにします。
- 更新の競合 (別プロセスからの同時変更) は検出しません。`SettingsHash` を競合検出に使う仕様は確認できていません。API 全体に楽観排他の仕組みがないとまでは断定せず、採用できる仕様が確認できた時点で再検討します。

## テスト方針

### service 層の単体テスト

- 操作 interface を fake に差し替えて、検証、変換、マージ規則を検証します。
- 正常系: 各宛先の `Parameters` 組み立て、`StartsAt` のエポックミリ秒変換、`Conditions` の変換、update のマージ (各リソース)。
- エラー系: 必須項目の欠落、宛先とフラグの不整合、周期指定と Crontab の排他、`key` の pattern 違反、`eq`/`in` の値数違反、現状の取得失敗時に更新 API を呼ばないこと。
- 更新の保持: 名前だけの更新で `Settings` を送らないこと、未指定値と明示した空文字列の区別、同じ宛先の未知パラメーター保持、宛先変更時の破棄、周期方式の切り替えと排他を確認します。StartsAt の整数文字列から整数への変換、Types と Conditions の未設定・null・空配列の保持も確認します。
- 安全性: read・update・delete・set-secret の種別違い、ID 不一致、Read の不正応答・失敗、変更直前のキャンセルでは変更 API を呼び出さないことを確認します。

### CLI と SDK 通信のテスト

- ドメインパッケージに入力パーサーと出力射影の単体テストを置きます。`internal/cli` にはコマンド登録、factory と Runtime の接続、共通出力と `--query` のテストを置きます。
- sakumock の EventBus サーバへ実際の SDK を接続し、16 個の末端コマンドそれぞれの正常系と送信本文を検証します。service の fake だけで通信経路の検証を代替しません。sakumock の対応不足があれば、SDK が読むステータスと応答型を再現する HTTP テストサーバーで補います。
- エラー系: `--condition` の構文エラーと検証エラー、`--starts-at` の解析エラー、create で周期指定が両方・片方とも指定なしの場合のエラー、`--destination` と宛先別フラグの不整合。
- 条件構文: `key`・`op` の重複、`value` の反復、空要素、区切りだけの値、追加の `=` の保持、カンマを含む不正指定、暗黙の空白補正がないこと、Kong が指定を先にカンマ分割しないことを確認します。
- 出力: `Parameters` の既知フィールドへの射影と未知キーの非表示、StartsAt の整数・整数文字列とミリ秒精度、不正応答のエラー、null と空配列、list の空配列出力、変更成功後の射影失敗を確認します。
- secret: `--secret-file @path` と標準入力 (`-`)、インライン・接頭辞なしパスの拒否、ファイル・標準入力の読み込み失敗、未知フィールド・型違い・末尾データ・秘密形式の混在を確認します。デコード失敗、API エラー、`--trace`・環境変数・プロファイルでのトレース有効化 (成功時だけでなくエラー時のモードを含む) でも秘密情報が stdout、stderr、ログへ現れず、トレース有効時は送信しないことを確認します。

### ライブ E2E

場所は `test/e2e/eventbus/scenario_eventbus_test.go` (識別名 `eventbus`) とし、helper には `test/e2e/internal/helper` を使います ([ADR 0058](../adr/0058-live-e2e-command-coverage.md)、[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。通常の `go test` ではスキップし、承認後に `SKR_E2E` と `skr-e2e` プロファイルでライブ実行します。実行記録は Git 対象外の `e2e-output/` に保存し、秘密情報を含めません。シナリオは実装済みで、ライブ実行に成功しています。

16 個の末端コマンドをすべて1回以上正常実行し、応答と期待する内容を照合します。コマンドツリーとシナリオの対応をテストし、既存の `eventbus-api` のライブ成功を高レベル層の成功として流用しません。実装前にシナリオ、成功条件、作成・変更・削除するリソースと費用の範囲を提示し、承認を得ます。本書のレビューはライブ操作の承認を兼ねません。

想定するシナリオの骨子は次のとおりです。

- 一意な試験名で既存リソースとの衝突がないことを確認し、SimpleMQ キューを作成して API キーを発行します。
- 実行設定を simplemq 宛先で作成し、read と list で内容を照合して set-secret で secret を登録します。
- トリガーを作成し (`--source` はイベントログ、`--type` はスイッチ作成、`--condition` は `zone` の eq)、read と list で照合します。description の update 後にも読み戻します。今回作成して削除するリソースのため、説明の復元は不要です。後片付けの再照合には更新後の説明を追跡して使います。
- 設定の反映待ちを行って `is1b` の通常スイッチを作成し、SimpleMQ キューへの試験メッセージ到着を期限付きで待ちます。受信ステータス、メッセージ ID、期待する本文を照合し、通信失敗を不在とみなしません。
- 実行設定の content を update で変更し、読み戻しと反映待ちの後、1個目のスイッチを再照合つきで削除してから別 ID のスイッチを作成して変更後の本文の受信を確認します。同時に保持するスイッチは1個で、ライブ実行で確認したゾーンのリソース数上限を避けます。
- 実行中に発火しない将来時刻のスケジュールについて create、read、list、update (`--starts-at` の変更)、delete を実行し、不在を確認します。スケジュール発火による配送成功とは区別します。
- スケジュール、トリガー、実行設定、キューを依存関係の逆順に削除し、不在を確認します。1個目のスイッチは本文更新後に削除済みのため、後片付けではスキップされます。delete もすべて実行記録に残る CLI 経路を使います。

作成対象は SimpleMQ キュー1個とその API キー、実行設定1個、トリガー1個、スケジュール1個、通常スイッチ2個 (同時に保持するのは1個) です。キュー操作、イベント配送、スイッチなどに関わる費用は実装前の承認時に確認します。既存シナリオと同額であるとは扱いません。

作成直後に ID・名前・説明・設定・参照先を照合し、その場で削除と不在確認を含む `defer` を登録します。削除直前にも保持した ID、種別、今回設定した値、所属・接続状態を取得し直します。不在なら変更せず終了し、不一致・不正応答・照会失敗なら削除せずテストを失敗させます。作成結果が不確定で ID を確定できない場合、同名という理由だけで自動回収しません。

安全確認は [ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md) に従い SDK から実行できます。途中の失敗やキャンセルでも後片付けを試み、削除の失敗とローカルの秘密情報ファイルの削除失敗も結果へ反映します。別 ID の同名リソース、設定不一致、照会失敗、キャンセル後の変更では削除を呼ばないことをテストします。

## ドキュメントと PLAN

- チュートリアルを `docs/manual/tutorials/eventbus.md` に作成しました。`skr eventbus` のコマンドと `eventbus-api` の対応、条件の指定方法、メッセージの受信までの流れを説明しています。
- `PLAN.md` の高レベル `eventbus` の節に、実装した操作の範囲、ライブ E2E、チュートリアルの対応状況を記録しています。
- 低レベル `eventbus-api` の設計文書 ([eventbus-api](eventbus-api.md)) に高レベル層への誘導を追記しています。

## 将来課題

- 定義ファイルを使うワークフロー (定義の初期化、差分確認、適用、状態確認)。AppRun CLI の形を参考に、別の設計として検討します。
- 消去系の入力 (`--description` の null 指定、条件の消去、`Types` の空配列)。低レベル層の JSON 経路で対応し、高レベル層での表現が必要になった時点で設計します。
- `Tags` と `Icon` の指定。EventBus での利用手順が確認できた時点で検討します。
- SDK への `service/eventbus` 追加後の切り替え (本書の「SDK への移植手順」)。
- Crontab の JST 評価について、利用者からの確認結果に対応する一次情報の参照を追加します。StartsAt との発火上の関係は別途確認し、確認前に実発火時刻を保証する説明は追加しません。

## 実機確認

2026-11-10 に `is1b` の `skr-e2e` プロファイルでライブ E2E を実行し、16 個の末端コマンドすべての正常実行を確認しました。スイッチ作成イベントにより、実行設定に指定した本文を SimpleMQ キューから受信でき、本文の update 後には変更後の本文を受信できることを確認しました。スケジュールの create / read / list / update / delete と、trigger の description 更新後の読み戻しも同じ実行で検証しました。後片付けでは作成した ID と識別情報を再照合してからトリガー、実行設定、スイッチ、キューの順で削除し、各一覧からの消失を確認しました (実行記録: `test/e2e/eventbus/e2e-output/eventbus-20261110185952.md`)。

ライブ実行での観察を設計に反映しています。

- SimpleMQ キューの ID は実環境では数値、EventBus のリソース ID は文字列で返ります。E2E の出力解釈は両方に対応しています。
- ゾーンのリソース数上限に達していたため、スイッチは同時に1個だけ保持し、1個目を受信確認と本文更新後に削除してから2個目を作成します。
