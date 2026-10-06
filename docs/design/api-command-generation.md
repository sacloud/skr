# IaaS 低レベル API コマンド生成器

- Status: Implemented
- Related ADR: [ADR 0016](../adr/0016-generate-low-level-api-commands.md)

## 目的

`cmd/apigen-iaas` は、明示した IaaS SDK 操作のコマンド登録、型付き API インターフェース、操作ハンドラー、標準的なリクエスト入力と出力を Go ソースに生成します。生成コードはリポジトリに含め、実行時や通常のビルド時に生成器を必要としません。生成処理の共通実装は `internal/apigen` にあります。

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

生成器は操作名とフィールド名を Go 識別子として扱います。Kong のコマンド名は操作名の小文字表記です。フラグ型は `string`、`bool`、`int`、`int64` に限定し、API リクエストの同名フィールドに代入します。`pointer: true` は明示指定された値だけをリクエストのポインター項目へ設定します。`conversion` を指定すると、その型へ変換してから設定します。`required: true` はフラグ経路での指定有無を検証します。`request_validator` を指定すると、JSON／フラグからリクエストを構築した後、SDK 呼び出し前にサービス固有の検証関数を実行します。

リクエストを持つ操作では `--request` にインライン JSON または `@path.json` を指定できます。個別フラグを設定した場合は `--request` と併用できません。JSON 経路では個別フラグの必須指定を要求せず、フラグ経路では必須フラグと、少なくとも 1 つのフラグの指定を確認してから SDK を呼び出します。配列、map、union、nullable 値、秘密情報など生成器の単純な型で表現しない入力は `--request` に残します。

IaaS のゾーン横断検索には、操作の `zone_search` に `flag_field` と `request_field` を指定します。対象フラグを `--zone all` にした場合、生成コードは Zone API で取得した各ゾーンに同じ検索条件を適用し、結果を連結します。いずれかの検索が失敗した場合は部分結果を出力しません。table 出力には結果ごとの Zone 列を付けます。JSON の `--request` 経路では `all` を展開せず、入力された値をそのまま SDK に渡します。IaaS リソース初期化時には、生成された `setZoneFactory` に Zone API factory を設定します。

リポジトリルートから実行し、生成先を明示します。

```console
$ make generate-iaas-api API_CONFIG=path/to/example.json API_OUTPUT=example_api_generated.go
```

Switch コマンドは [`api/commands/iaas-switch.json`](../../api/commands/iaas-switch.json) を設定として使います。生成コードは `internal/iaas/switchapi/` に配置し、再生成時は次を実行します。

```console
$ make generate-iaas-api API_CONFIG=api/commands/iaas-switch.json API_OUTPUT=internal/iaas/switchapi/switch_api_generated.go
```

同じ処理は `go run ./cmd/apigen-iaas -config path/to/example.json -out example_api_generated.go` でも実行できます。設定を不正な JSON、未知のフィールド、重複した操作名／メソッド、未対応のフラグ型で受け付けず、出力は `go/format` で整形します。将来別の API 形式に専用の生成器を追加する場合は、IaaS の機能や名称と混同しないコマンド入口を設けます。

## 手書きコードとの境界

生成ファイルは編集せず、設定を直して再生成します。`package`、コマンド、API interface、factory、runtime の型名はリソース単位で指定します。生成コマンド package は CLI 本体から分離し、`Runtime` callbacks で JSON decoder、出力形式の選択、出力処理、リクエスト検証を受け取ります。生成されたコマンド構造体を親コマンドへ登録し、生成された API interface を実際の SDK クライアントに結ぶ factory は、CLI 側で設定します。

`request_validator` は、JSON／フラグから同じリクエストを構築した後、SDK 呼び出し前に行う明示的なサービス固有検証の関数名です。`handwritten: true` は、操作を親コマンドと型付き API インターフェースには登録しながら、そのコマンド構造体と `Run` ハンドラーを手書きにする指定です。これにより、標準操作と個別処理を同じコマンド階層に配置できます。

Switch では `find`、`read`、`create`、`update`、`delete` を生成します。`find` の `zone_search` 設定が `--zone all` のゾーン取得、検索の反復、結果集約、table の Zone 列を共通処理として生成します。個別の Zone／ID 検証は `request_validator` で明示します。`handwritten: true` を設定した操作は、親コマンドと API インターフェースに登録しながらコマンド構造体と handler を手書きにできます。SDK の provider class 設定、秘密情報の読み込み、複雑なリクエスト構築、通常と異なる出力や失敗処理は引き続き手書きの実装に残します。対象コマンドで `go test` を実行し、SDK 呼び出し、入力、出力、エラー経路を確認してから生成コードを採用します。
