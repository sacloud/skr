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

SimpleMQ のキュー／メッセージコマンドは次の設定から `internal/simplemqapi/` に再生成します。生成コマンドと Provider クラス補完、キュー設定のフラットなフラグ、メッセージ数と API キーの出力形状を同じ内部パッケージに置き、ルートの `package main` は CLI 登録と Runtime／SDK factory の接続を担当します。

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

生成器は操作名とフィールド名を Go 識別子として扱います。Kong のコマンド名は操作名の小文字表記です。フラグ型は `string`、`bool`、`int`、`int64` に加えて `[]string` をサポートします。文字列スライスは `separator` に区切り文字を指定すると、区切り文字で分割して API リクエストの同名フィールドに設定できます。IaaS API ではトップレベル `Tags` に限って `--tags` とカンマ区切り (`separator: ","`) を使用します。これは配列入力全般をフラグ化するものではなく、他の配列は JSON 経路に残します。`pointer: true` は明示指定された値だけをリクエストのポインター項目へ設定します。更新時の `Tags` がポインター型なら `pointer: true` を指定し、省略時の未指定状態を維持します。`conversion` を指定すると、その型へ変換してから設定します。`required: true` はフラグ経路での指定有無を検証します。`request_validator` を指定すると、JSON／フラグからリクエストを構築した後、SDK 呼び出し前にサービス固有の検証関数を実行します。

リクエストを持つ操作では `--request` にインライン JSON または `@path.json` を指定できます。個別フラグを設定した場合は `--request` と併用できません。JSON 経路では個別フラグの必須指定を要求せず、フラグ経路では必須フラグと、少なくとも 1 つのフラグの指定を確認してから SDK を呼び出します。IaaS API のトップレベル `Tags` 配列は例外として `--tags` でも指定できますが、JSON 経路での `Tags` 配列も引き続き利用でき、両経路を混在させることはできません。その他の配列、map、union、nullable 値、秘密情報など生成器の単純な型で表現しない入力は `--request` に残します。

IaaS の検索では、フラグと `--request` JSON のどちらでも対象ゾーンを明示します。`Zone` に `all` を指定したゾーン横断検索はサポートせず、生成コードにゾーン列挙や複数ゾーンの結果集約を含めません。

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

生成ファイルは編集せず、設定を直して再生成します。`package`、コマンド、API interface、factory、runtime の型名はリソース単位で指定します。生成コマンド package は `internal/` 配下に配置して CLI 本体から分離し、`Runtime` callbacks で JSON decoder、出力の事前検証、出力処理、リクエスト検証を受け取ります。生成されたコマンド構造体を親コマンドへ登録し、生成された API interface を実際の SDK クライアントに結ぶ factory は、CLI 側で設定します。

[ADR 0030](../adr/0030-remove-table-output.md) に従い、出力形式は JSON のみです。`ValidateOutput func(*kong.Context) error` は結果を返す操作の API 呼び出し前にクエリを検証し、`WriteOutput func(*kong.Context, any) error` は結果全体またはクエリ結果を JSON で書き込みます。出力形式と表示用 Zone ラベルは callback に渡しません。手書きハンドラーも同じ契約に揃えます。

`request_validator` は、JSON／フラグから同じリクエストを構築した後、SDK 呼び出し前に行う明示的なサービス固有検証の関数名です。`handwritten: true` は、操作を親コマンドと型付き API インターフェースには登録しながら、そのコマンド構造体と `Run` ハンドラーを手書きにする指定です。これにより、標準操作と個別処理を同じコマンド階層に配置できます。

Switch では `find`、`read`、`create`、`update`、`delete` を生成します。すべての操作で対象ゾーンを明示し、`Zone` が `all` の場合は `request_validator` で SDK 呼び出し前に拒否します。個別の Zone／ID 検証も `request_validator` で明示します。`handwritten: true` を設定した操作は、親コマンドと API インターフェースに登録しながらコマンド構造体と handler を手書きにできます。SDK の provider class 設定、秘密情報の読み込み、複雑なリクエスト構築、通常と異なる出力や失敗処理は引き続き手書きの実装に残します。対象コマンドで `go test` を実行し、SDK 呼び出し、入力、出力、エラー経路を確認してから生成コードを採用します。

Server では `find`、`read`、`create`、`update`、`delete` を生成します。設定は [`api/commands/iaas-server.json`](../../api/commands/iaas-server.json)、生成先は `internal/iaas/serverapi/` です。`find` を含む各操作では対象ゾーンを明示し、フラグと JSON のどちらでも `Zone: "all"` を SDK 呼び出し前に拒否します。Server 作成では Zone、Name、CPU、MemoryGB と単純な独立スカラー値をフラグで指定できます。IaaS のトップレベル `Tags` は `--tags` にカンマ区切りで指定できます。CPU と MemoryGB のプラン組み合わせ、ディスク、ネットワークインターフェースなどを含む複雑な構成は JSON リクエストで渡し、JSON と個別フラグの併用を拒否します。ディスク作成の待機設定 `NoWait` はディスク構成や `BootAfterCreate` と関係するため JSON 経路に残します。更新では単純なスカラーと `--tags` を任意フラグにし、未指定値を保持します。指定項目がない更新リクエストは拒否します。削除はディスクを既定で残し、`WithDisks` と `Force` を明示した場合だけ、それぞれ接続ディスクの削除と起動中サーバの強制停止を許可します。サービス固有の検証は `internal/iaas/server/` に置きます。これらの操作は `server_test.go` と `internal/iaas/server/validation_test.go` で検証します。

Disk では `find`、`read`、`create`、`update`、`delete` を生成します。設定は [`api/commands/iaas-disk.json`](../../api/commands/iaas-disk.json)、生成先は `internal/iaas/diskapi/` です。各検索では対象ゾーンを明示し、`--tags` でタグ条件をカンマ区切り指定できます。作成ではディスクプラン、接続インターフェース、サイズなどの関連する値を同時に扱い、更新では SDK の optional 値を保持するため、どちらも JSON リクエストのみ受け付けます。サービス固有の入力検証は `internal/iaas/disk/` に置きます。
