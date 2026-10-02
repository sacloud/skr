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

イベント発生を起点にする場合は trigger を作成します。`Source`、`Types` は対象のイベント
ソースに合わせた値に置き換え、`ProcessConfigurationID` は実行するジョブを指定します。

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

## ライセンス

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
