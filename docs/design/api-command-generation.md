# 低レベル API コマンド生成器

- Status: Implemented
- Related ADR: [ADR 0016](../adr/0016-generate-low-level-api-commands.md)

## 目的

`cmd/apigen-iaas` は、明示した IaaS SDK 操作のコマンド登録、型付き API インターフェース、操作ハンドラー、標準的なリクエスト入力と出力を Go ソースに生成します。IaaS 以外の Ogen 系 API には共通入口 `cmd/apigen-api` を使います。生成コードはリポジトリに含め、実行時や通常のビルド時に生成器を必要としません。設定解析と共有可能な生成処理は `internal/apigen` にあります。IaaS と Ogen 系 API は別の CLI 入口を保ち、共通化できる範囲で実装を共有します。

生成器は SDK の操作を探索して公開範囲や入力の意味を決めません。対象操作、SDK メソッド名、リクエスト／レスポンス型、個別フラグ、ヘルプは設定で明示します。設定内容は SDK の公開インターフェースとリクエスト型に照らして確認します。

## 設定と実行

設定は JSON ファイルです。たとえば、次の項目をリソースごとに記述します。

```json
{
  "package": "main",
  "resource": "example",
  "command_type": "exampleCommands",
  "api_type": "exampleAPI",
  "factory_type": "exampleAPIFactory",
  "runtime_type": "exampleRuntime",
  "imports": {
    "example": "github.com/sacloud/example-sdk/api/example"
  },
  "operations": [
    {
      "name": "Find",
      "command_type": "exampleFindCommand",
      "help": "項目を検索します。",
      "method": "FindWithContext",
      "request_type": "example.FindRequest",
      "response_type": "[]*example.Item",
      "flags": [
        {
          "name": "name",
          "field": "Name",
          "type": "string",
          "help": "検索する名前です。",
          "required": false
        }
      ]
    }
  ]
}
```

SDK のメソッドは `context.Context` と、指定した場合は `*request_type` を受け取ります。結果を返す操作には `response_type` を指定し、戻り値と `error` を出力します。`response_type` を省略した操作は `error` のみを返し、成功時に出力しません。`request_type` を省略した操作は入力なしで呼び出します。インポートの別名を含む型名は、`imports` に定義します。

## IaaS 以外の API ドメイン

IaaS 以外の EventBus と SimpleMQ も、同じ `cmd/apigen-api` / `generate-api` から生成します。たとえば次の項目は、位置引数を取る操作と値渡しのリクエストを表します。

```json
{
  "arguments": [
    {
      "name": "id",
      "field": "ID",
      "type": "string",
      "help": "対象リソースの ID。"
    }
  ],
  "request_type": "v1.UpdateRequest",
  "request_by_value": true
}
```

`arguments` は SDK 操作の `context.Context` の後に並ぶ引数を定義します。既定では Kong の位置引数になり、`flag: true` を指定すると名前付きフラグになります。`required: true` は名前付きフラグの必須指定を表します。`factory_arguments` は各操作コマンドに共通して生成する名前付き入力と、API クライアント factory に渡す引数を定義します。SimpleMQ メッセージ API のキュー名と API キーファイルはこの方法でクライアント生成へ渡します。`request_by_value` は `request_type` をポインターではなく値で SDK 操作へ渡します。省略時はポインターで渡します。これらは SDK のメソッドシグネチャに合わせるための指定であり、操作範囲、引数の意味、ヘルプを自動推測するものではありません。

リポジトリルートから、共通生成器に対象操作の設定と出力先を渡します。

```console
$ make generate-api API_CONFIG=path/to/eventbus-example.json API_OUTPUT=path/to/eventbus_api_generated.go
$ make generate-api API_CONFIG=path/to/simplemq-example.json API_OUTPUT=path/to/simplemq_api_generated.go
```

上記のパスは設定例です。設定ファイルと生成先は対象 API とパッケージ構成に合わせて用意します。生成器が利用できることは、EventBus / SimpleMQ の全操作が生成済み、または CLI で公開済みであることを意味しません。プロバイダークラスの補完、秘密情報の扱い、独自の入力検証などは設定から意味を推測せず、対象の設計に従って手書きに残します。

SimpleMQ のキュー／メッセージコマンドは次の設定から `internal/simplemqapi/` に再生成します。生成コマンドと Provider クラス補完、キュー設定のフラットなフラグ、メッセージ数と API キーの出力形状を同じ内部パッケージに置き、ルートの `package main` は CLI 登録と Runtime／SDK factory の接続を担当します。コマンド群の依存は生成された `New...Commands(runtime, factory)` constructor で一度に注入します。

```console
$ make generate-api API_CONFIG=api/commands/simplemq-queue.json API_OUTPUT=internal/simplemqapi/simplemq_queue_api_generated.go
$ make generate-api API_CONFIG=api/commands/simplemq-message.json API_OUTPUT=internal/simplemqapi/simplemq_message_api_generated.go
```

EventBus の実行設定、スケジュール、トリガーコマンドも `internal/eventbusapi/` に再生成します。実行設定の `update-secret`、Provider クラス補完、SDK クライアント生成は同じ内部パッケージの手書きコードに置きます。ユーザー向けコマンド名は `process-configuration` のままにし、生成設定と生成先のファイル名だけは編集ツールが誤判定しにくいよう `c0nfiguration` 表記にします。

```console
$ make generate-api API_CONFIG=api/commands/eventbus-process-c0nfiguration.json API_OUTPUT=internal/eventbusapi/eventbus_process_c0nfiguration_api_generated.go
$ make generate-api API_CONFIG=api/commands/eventbus-schedule.json API_OUTPUT=internal/eventbusapi/eventbus_schedule_api_generated.go
$ make generate-api API_CONFIG=api/commands/eventbus-trigger.json API_OUTPUT=internal/eventbusapi/eventbus_trigger_api_generated.go
```

生成器は操作名とフィールド名を Go 識別子として扱います。Kong のコマンド名は操作名の小文字表記です。フラグ型は `string`、`bool`、`int`、`int64` に加えて、IaaS API のトップレベル `Tags` 専用の `[]string` をサポートします。配列フラグは `name: "tags"`、`field: "Tags"`、`type: "[]string"`、`separator: ","` の組み合わせだけを受け付けます。これは配列入力全般をフラグ化するものではなく、他の配列は JSON 経路に残します。`pointer: true` は明示指定された値だけをリクエストのポインター項目へ設定します。更新時の `Tags` がポインター型なら `pointer: true` を指定し、省略時の未指定状態を維持します。`conversion` を指定すると、その型へ変換してから設定します。`required: true` はフラグ経路での指定有無を検証します。`request_validator` を指定すると、JSON／フラグからリクエストを構築した後、SDK 呼び出し前にサービス固有の検証関数を実行します。

リクエストを持つ操作では `--request` にインライン JSON または `@path.json` を指定できます。個別フラグを設定した場合は `--request` と併用できません。JSON 経路では個別フラグの必須指定を要求せず、フラグ経路では必須フラグと、少なくとも 1 つのフラグの指定を確認してから SDK を呼び出します。IaaS API のトップレベル `Tags` 配列は例外として `--tags` でも指定できますが、JSON 経路での `Tags` 配列も引き続き利用でき、両経路を混在させることはできません。その他の配列、map、union、nullable 値、秘密情報など生成器の単純な型で表現しない入力は `--request` に残します。

IaaS の検索では、フラグと `--request` JSON のどちらでも対象ゾーンを明示します。`Zone` に `all` を指定したゾーン横断検索はサポートせず、生成コードにゾーン列挙や複数ゾーンの結果集約を含めません。

[ADR 0046](../adr/0046-common-iaas-zone-validation.md) に従い、単一ゾーンを対象とする生成操作には `validate_zone: true` を指定します。生成コードはフラグの代入・入力変換または JSON デコードの後、`request_validator` と API クライアント生成の前に `zones.ValidateSingleZone(request.Zone)` を呼び出します。空文字列と `all` を両経路で拒否し、空文字列は `Zone が必要です (--request JSON も指定できます)` と表示します。既存の必須フラグの指定有無の確認は維持します。Zone は文字列のフィールドを前提とし、型の一致は生成コードのビルドで検証します。この設定はリクエストを持つ生成操作だけで利用でき、手書きハンドラーでは共通関数を直接呼び出します。フラグ名から検証を自動推測せず、設定で有効化しない操作には検証とインポートを追加しません。

リポジトリルートから実行し、生成先を明示します。

```console
$ make generate-iaas-api API_CONFIG=path/to/example.json API_OUTPUT=example_api_generated.go
```

Switch コマンドは [`api/commands/iaas-switch.json`](../../api/commands/iaas-switch.json) を設定として使います。生成コードは `internal/iaas/switchapi/` に配置し、再生成時は次を実行します。

```console
$ make generate-iaas-api API_CONFIG=api/commands/iaas-switch.json API_OUTPUT=internal/iaas/switchapi/switch_api_generated.go
```

同じ処理は `go run ./cmd/apigen-iaas -config path/to/example.json -out example_api_generated.go` でも実行できます。設定を不正な JSON、未知のフィールド、重複した操作名／メソッド、未対応のフラグ型で受け付けず、出力は `go/format` で整形します。IaaS と Ogen 系 API は生成対象の形が異なるため、IaaS は専用入口を保ち、サービス数が増える Ogen 系 API は共通入口 `cmd/apigen-api` を使います。

## 手書きコードとの境界

生成ファイルは編集せず、設定を直して再生成します。`package`、コマンド、API interface、factory、runtime の型名はリソース単位で指定します。生成コマンド package は `internal/` 配下に配置して CLI 本体から分離し、`Runtime` callbacks で JSON decoder、出力の事前検証、出力処理を受け取ります。リクエスト検証関数はコマンドと同じ package に置き、生成コードから直接呼び出します。設定した関数が存在しない場合や引数型が一致しない場合はビルド時に検出されます。[ADR 0044](../adr/0044-typed-request-validator-callbacks.md) に従い、文字列キー、`any` による振り分け、CLI の検証 callback 接続は使いません。生成されたコマンド群の `Runtime` と SDK API factory は `New...Commands(runtime, factory)` でまとめて注入し、setter による段階的な初期化を避けます。[ADR 0045](../adr/0045-initialize-api-commands-with-constructors.md) を参照してください。

[ADR 0030](../adr/0030-remove-table-output.md) に従い、出力形式は JSON のみです。`ValidateOutput func(*kong.Context) error` は結果を返す操作の API 呼び出し前にクエリを検証し、`WriteOutput func(*kong.Context, any) error` は結果全体またはクエリ結果を JSON で書き込みます。出力形式と表示用 Zone ラベルは callback に渡しません。手書きハンドラーも同じ契約に揃えます。

`request_validator` は、JSON／フラグからリクエストを構築した後、SDK 呼び出し前に実行する検証関数名です。生成コードと同じ package に関数を定義し、生成器は `ValidateRequest(request)` のように直接呼び出します。`handwritten: true` の操作も、手書きハンドラーから同じ package の検証関数を直接呼び出します。これは操作を親コマンドと型付き API インターフェースに登録しながら、コマンド構造体と `Run` ハンドラーを手書きにする指定です。これにより、標準操作と個別処理を同じコマンド階層に配置できます。

### 反復可能な入力と型付き変換

[ADR 0042](../adr/0042-generated-request-transform.md) に従い、標準処理を生成したまま、フラグ経路の入力変換だけを手書き callback に委ねられます。操作の設定例は次のとおりです。

```json
{
  "request_transform": "TransformCreateRequest",
  "repeated_flags": [
    {
      "name": "disk",
      "field": "Disks",
      "help": "ディスク1個を指定します。繰り返し指定できます。"
    }
  ]
}
```

これは Server Create の設定の抜粋です。実際のヘルプでは対応キー、必須条件、値の形式を説明します。`repeated_flags` は `name`、`field`、`help` を持ち、コマンドに `[]string` の入力フィールドを生成します。各指定の文字列を分割・補正せず、空文字列も含めて指定順に保持します。API リクエストの同名フィールドへ直接代入する設定ではありません。反復入力の指定も `--request` と相互排他です。

`request_transform` は生成する Runtime callback の公開フィールド名です。Server Create では `TransformCreateRequest func(*CreateCommand, *server.CreateRequest) error` を生成し、`internal/cli` が `serverapi.TransformCreateRequest` を接続します。生成器は callback の型を明示し、関数の実装を探索しません。Runtime の既存フィールドや他の callback と同じ名前は拒否します。反復入力には変換を必須とし、`handwritten: true` またはリクエストのない操作では使えません。

フラグ経路では必須入力の確認とスカラー値の代入後に変換し、その後で `request_validator` と API 呼び出しへ進みます。反復入力のない指定でも変換は実行します。変換エラーは API 呼び出し前に返します。JSON 経路は変換を通らず、デコード後に同じ検証関数を直接呼び出します。

生成器は `key=value` の構文、キーの意味、値の範囲、要素間の制約を扱いません。Server ではドメイン側がディスク名の補完、virtio 接続、正の整数の検証、shared NIC の位置制約を担当します。反復入力は対象操作が明示的に採用するもので、任意の配列や秘密情報をフラグ化する機構ではありません。

Switch では `find`、`read`、`create`、`update`、`delete` を生成します。すべての操作で対象ゾーンを明示し、`validate_zone: true` による共通検証で空文字列と `all` を SDK 呼び出し前に拒否します。ID などの固有制約は `request_validator` で検証します。`handwritten: true` を設定した操作は、親コマンドと API インターフェースに登録しながらコマンド構造体と handler を手書きにできます。SDK の provider class 設定、秘密情報の読み込み、複雑なリクエスト構築、通常と異なる出力や失敗処理は引き続き手書きの実装に残します。対象コマンドで `go test` を実行し、SDK 呼び出し、入力、出力、エラー経路を確認してから生成コードを採用します。

Server では `find`、`read`、`create`、`update`、`delete` を生成します。`create` のディスク／NIC 入力は型付き変換 callback に接続し、`boot` は `handwritten: true` で登録した手書きハンドラーとして実装します。設定は [`api/commands/iaas-server.json`](../../api/commands/iaas-server.json)、生成先は `internal/iaas/serverapi/` です。各操作では対象ゾーンを明示し、対応するフラグと JSON のどちらでも `Zone: "all"` を SDK 呼び出し前に拒否します。

Server 作成では Zone、Name、CPU、MemoryGB と単純な独立スカラー値をフラグで指定できます。IaaS のトップレベル `Tags` は `--tags` にカンマ区切りで指定できます。[ADR 0041](../adr/0041-server-repeatable-disk-and-nic-options.md) の限定例外として、反復可能な `--disk` と `--nic` でディスクと NIC を指定順に構築します。ディスクは plan-id と size-gb が必須で、任意の archive-id を受け付け、virtio 接続にします。NIC は upstream=shared または upstream=disconnected を受け付け、shared は先頭に限定します。それ以外の複雑な構成は JSON リクエストで渡し、JSON と個別フラグの併用を拒否します。ディスク作成の待機設定 `NoWait` はディスク構成や `BootAfterCreate` と関係するため JSON 経路に残します。

Boot は Zone と ID を必須とし、任意の user-data を `--user-data @path` または `--user-data -` で読み込みます。省略時は通常どおり起動し、インラインの user-data は拒否します。更新では単純なスカラーと `--tags` を任意フラグにし、未指定値を保持します。指定項目がない更新リクエストは拒否します。削除はディスクを既定で残し、`WithDisks` と `Force` を明示した場合だけ、それぞれ接続ディスクの削除と起動中サーバの強制停止を許可します。サービス固有の検証は `internal/iaas/server/`、手書きハンドラーと入力処理は `internal/iaas/serverapi/` に置きます。各パッケージの単体テストと `internal/cli/server_test.go` で検証します。

Disk では `find`、`read`、`create`、`update`、`delete` を生成します。設定は [`api/commands/iaas-disk.json`](../../api/commands/iaas-disk.json)、生成先は `internal/iaas/diskapi/` です。各検索では対象ゾーンを明示し、`--tags` でタグ条件をカンマ区切り指定できます。Create は Zone、Name、DiskPlanID、Connection、SizeGB と独立した単純値および Tags を個別フラグで指定でき、DistantFrom と EditParameter は JSON に残します。SDK の CreateRequest にある OSType は API に送るフィールドではなく、SDK が OS 種別からコピー元アーカイブを解決する補助入力のため、個別フラグにはしません。Update は Zone と ID を必須とし、変更する Name、Description、Tags、IconID、Connection を個別フラグで指定できます。更新フラグは pointer 値として生成し、省略時の未指定を維持します。両操作とも `--request` JSON を維持し、個別フラグとの併用を拒否します。サービス固有の入力検証は `internal/iaas/disk/` に置きます。

PacketFilter では `find`、`read`、`create`、`update`、`delete` を生成します。設定は [`api/commands/iaas-packetfilter.json`](../../api/commands/iaas-packetfilter.json)、生成先は `internal/iaas/packetfilterapi/` です。`Expression` はルールの複雑な配列なので、各ルールを個別の CLI フラグにはせず、`--expressions` に JSON 配列を直接または `@path.json` で渡せる入力変換 callback を接続します。`--request` の完全な JSON 入力も維持し、両経路は併用できません。更新時の `OriginalExpressionHash` は読み取り結果の `ExpressionHash` を受け取るスカラー値です。サービス固有の単一ゾーン、ID、空更新の検証は `internal/iaas/packetfilter/` に置きます。ライブ E2E はフィルタの作成・ルール更新・削除を確認しますが、NIC への適用と通信の許可／拒否は行いません。

PacketFilter のライブ E2E は、選択中プロファイルが指すプロジェクトの `is1b` を使います。実行前に対象と変更内容の承認を得て、次のコマンドを実行します。

```console
$ make build
$ ./skr config current
$ go run ./test/e2e/packetfilter --skr ./skr --confirm-is1b-live
```

作成・検索・参照・ルール更新・削除と削除後の不在をライブ環境で確認済みです。runner はランダムな専用名を使い、同名リソースがある場合は変更せず停止します。成功時と途中失敗時のどちらでも、今回作成したフィルタの ID、名前、説明を削除直前に照合してから後片付けします。サーバや NIC は作成・変更しません。証跡は共通の private evidence 機構で `tmp/packetfilter-api/<YYYYMMDDHHmm>/` に保存し、Git へ追加しません。
