# skr

skr は、さくらのクラウドを操作するための CLI ツールです。

このリポジトリでは、さくらのクラウドの CLI ツールの新しいバージョンを `skr` として開発しています。
従来の usacloud との互換性は保証されません。

## 使い方

現在選択されているプロファイル名は、次のコマンドで確認できます。

```console
$ skr config current
my-profile
```

EventBus は、スケジュールまたはイベントソースの変更検知をきっかけにジョブを実行する
サービスです。ジョブの実行はベストエフォート型で、厳密なリアルタイム性は保証されません。
操作するには、SDK が利用するプロファイルか `SAKURA_ACCESS_TOKEN`、
`SAKURA_ACCESS_TOKEN_SECRET` 環境変数で認証情報を設定してください。

利用順は「実行先サービス → process-configuration → schedule または trigger」です。
実行設定にはシンプル通知、シンプル MQ、オートスケールのいずれかを指定します。
実行先サービスを先に用意し、schedule / trigger には作成した process-configuration の ID を
指定してください。各コマンドの `--help` にも入力形式と実行例があります。

```console
$ skr eventbus-api process-configuration list
$ skr eventbus-api schedule list
$ skr eventbus-api trigger list
```

作成では `--request` に `CommonServiceItem` を含む JSON を指定します。次の例はシンプル MQ
キューにメッセージを送る実行設定です。`Parameters` は実行先サービス固有の JSON 文字列です。

```console
$ skr eventbus-api process-configuration create --request='{"CommonServiceItem":{"Name":"send-message","Settings":{"Destination":"simplemq","Parameters":"{\"queue_name\":\"QUEUE-NAME\",\"content\":\"hello\"}"}}}'
```

作成結果に含まれる `ID` を使って、定期実行 schedule を作成できます。`StartsAt` は Unix
epoch のミリ秒を整数で指定します。`RecurringStep` と `RecurringUnit` の単位は `min`、
`hour`、`day` です。代わりに `Crontab` の指定も可能です。

```console
$ skr eventbus-api schedule create --request='{"CommonServiceItem":{"Name":"daily-message","Settings":{"ProcessConfigurationID":"PROCESS-CONFIGURATION-ID","StartsAt":1893456000000,"RecurringStep":1,"RecurringUnit":"day"}}}'
```

イベント発生を起点にする場合は trigger を作成します。対象のイベントソースに応じて
`Source` と `Types` を指定し、`ProcessConfigurationID` には実行するジョブを指定します。

```console
$ skr eventbus-api trigger create --request='{"CommonServiceItem":{"Name":"on-change","Settings":{"Source":"EVENT-SOURCE","Types":["EVENT-TYPE"],"ProcessConfigurationID":"PROCESS-CONFIGURATION-ID"}}}'
```

作成・更新の `--request` は JSON を直接指定するか、`@request.json` の形式でファイルから
読み込めます。作成時の `Provider` はコマンドが設定します。更新では指定した項目だけが変更
され、`"Description":null` で説明を消去できます。`list` は JSON 配列、`read` と作成・更新は
JSON オブジェクトを標準出力に返します。各リソースで `list`、`read`、`create`、`update`、
`delete` を利用できます。

実行先サービスの認証情報は `process-configuration update-secret` でファイルまたは標準入力
から登録します。secret オブジェクト自体を指定し、コマンドラインには含めません。
Simple MQ のファイル例:

```json
{"APIKey":"SIMPLEMQ-API-KEY"}
```

```console
$ skr eventbus-api process-configuration update-secret PROCESS-CONFIGURATION-ID --secret-file secret.json
$ skr eventbus-api schedule create --help
```

JSON をファイルに保存して `--request @request.json` と指定すれば、引用符のエスケープを避けて
作成・更新できます。

## IaaS Switch API

`skr iaas-api switch` は、スイッチの `find`、`read`、
`create`、`update`、`delete` を実行します。認証には SDK のプロファイルか
`SAKURA_ACCESS_TOKEN`、`SAKURA_ACCESS_TOKEN_SECRET` 環境変数を使用してください。
各操作は `--zone` などの個別フラグか、各操作の `--help` に示すキーと値で構成した JSON を `--request` に指定します。
両方の経路は併用できません。JSON は直接指定するか `@request.json` でファイルから読み込めます。
`Zone` はいずれの経路でも必須です。`find` は個別フラグの `--zone all` で全ゾーンを検索できます。SDK からゾーン一覧を取得して各ゾーンを検索し、結果をまとめて出力します。`--request` の JSON は従来どおり単一ゾーンのリクエストとして扱います。`read`、`create`、`update`、`delete` では個別フラグに `--zone all` を指定できません。

```console
$ skr iaas-api switch create --zone ZONE --name example
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["example"]}'
$ skr iaas-api switch find --zone all
$ skr iaas-api switch read --zone ZONE --id 123456789012
```

作成では `Name` が必須です。`Description`、`Tags`、`IconID`、`NetworkMaskLen`、
`DefaultRoute` も指定できます。配列である `Tags` は JSON で指定します。
更新では `Zone` と `ID` を指定し、変更する項目だけをフラグまたは JSON に含めます。
フラグの `--description ''` や `--network-mask-len 0` は明示的な更新として扱います。
更新結果には SDK が読み取ったリソースを反映します。
`find` は JSON 配列、`read`／`create`／`update` は JSON オブジェクトを標準出力に返します。
`delete` は成功時に標準出力へ出力しません。

削除時は `WaitForRelease` を有効にすると、他リソースから参照されている間の削除を待ち合わせます。
待ち時間を調整する場合は `WaitForReleaseTimeout` と `WaitForReleaseTick` を秒単位で指定します。
SDK の既定値はそれぞれ 3600 秒と 5 秒です。

配列を含むリクエストはファイルに保存して指定できます。更新例:

```json
{
  "Zone": "ZONE",
  "ID": 123456789012,
  "Tags": ["test"]
}
```

```console
$ skr iaas-api switch update --request @update.json
$ skr iaas-api switch delete --zone ZONE --id 123456789012 --fail-if-not-found
```

`ZONE` と `123456789012` は実際のゾーンと作成結果の ID に置き換えてください。`--request` と個別フラグを併用しないでください。
コマンドの入力項目と各操作の詳細は `skr iaas-api switch <operation> --help` を参照してください。
Sandbox ゾーンで管理操作を試す場合は
[IaaS Switch API チュートリアル](docs/tutorials/iaas-api/switch.md)を参照してください。

## ライセンス

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
