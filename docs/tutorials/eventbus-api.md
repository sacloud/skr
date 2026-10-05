# EventBus API: イベントを検知してシンプル通知を送る

EventBus はイベントの検知やスケジュールをきっかけにジョブを実行します。このチュートリアルでは、ネットワークスイッチの作成イベントを検知し、シンプル通知の通知先グループへ通知するトリガーを作成します。EventBus のジョブ実行はベストエフォート型であり、厳密なリアルタイム性は保証されません（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)）。

> [!WARNING]
> この手順では EventBus の設定を作成し、テスト用のネットワークスイッチ作成イベントを発生させます。実行前に `skr config current` で対象プロファイルを確認し、通知先と対象ゾーンを確認してください。スイッチの作成・削除には `skr eventbus-api` 以外の手段を使います。テスト後はトリガーを先に削除してください。

## 前提条件

- EventBus API を操作できる skr の認証設定があること。EventBus API の利用には「作成・削除」のアクセスレベルが必要です（[EventBus API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/eventbus/api.html)）。
- 受信先と通知先グループを設定済みのシンプル通知、および EventBus からの呼び出しに使う API キーがあること。シンプル通知の通知先とグループは事前に設定します（[EventBus クイックスタート](https://manual.sakura.ad.jp/cloud/appliance/eventbus/basic.html)）。
- テスト用の通常スイッチを作成できるプロジェクトとゾーンがあること。テストでは、作成後にすぐ削除できる専用スイッチを使ってください。
- スイッチの作成イベントを発生させるため、対象ゾーンで他の通常スイッチ作成が重ならない時間に実施すること。この例ではイベントの `zone` 条件を指定しますが、同じゾーンで発生するほかのスイッチ作成イベントも対象になります。

## 作成するリソース

1. **実行設定（process-configuration）** — シンプル通知の通知先グループと送信メッセージを指定します。
2. **トリガー（trigger）** — 指定ゾーンの通常スイッチ作成イベントで実行設定を呼び出します。
3. **テスト用スイッチ** — EventBus が検知するイベントを発生させます。スイッチはこの CLI ではなく、作成に使ったコントロールパネルまたは別の操作手段で削除します。

## Step 1: プロファイルとコマンドを確認する

```console
$ skr config current
$ skr eventbus-api --help
$ skr eventbus-api process-configuration create --help
$ skr eventbus-api process-configuration update-secret --help
$ skr eventbus-api trigger create --help
```

`skr config current` の結果が対象のプロファイルであることを確認します。作成時の `--request` は `CommonServiceItem` を含む JSON で、ファイルは `@ファイル名` で指定できます。作成結果は JSON で出力されるので、後続手順用に `ID` を控えます。

## Step 2: シンプル通知の実行設定を作成する

次の JSON を `process-configuration.json` として保存し、`NOTIFICATION-GROUP-ID` を既存の通知先グループ ID に置き換えます。`Parameters` は実行先サービス固有の JSON を文字列として指定します。ここでは通知先グループと通知メッセージを設定します（[実行設定の項目](https://manual.sakura.ad.jp/cloud/appliance/eventbus/control_panel.html)）。

```json
{
  "CommonServiceItem": {
    "Name": "eventbus-tutorial-switch-created",
    "Settings": {
      "Destination": "simplenotification",
      "Parameters": "{\"group_id\":\"NOTIFICATION-GROUP-ID\",\"message\":\"A network switch was created.\"}"
    }
  }
}
```

EventBus からシンプル通知を呼び出す API キーのアクセストークンとアクセストークンシークレットを、作業端末上の `notification-secret.json` に保存します。値をコマンドラインやリポジトリに記録しないでください。ファイルのアクセス権を制限します。

```json
{
  "AccessToken": "ACCESS-TOKEN",
  "AccessTokenSecret": "ACCESS-TOKEN-SECRET"
}
```

```console
$ chmod 600 notification-secret.json
$ skr eventbus-api process-configuration create --request @process-configuration.json
```

作成結果の `ID` を `PROCESS-CONFIGURATION-ID` として控えます。次に、作成した実行設定へシークレットを登録します。

```console
$ skr eventbus-api process-configuration update-secret PROCESS-CONFIGURATION-ID --secret-file notification-secret.json
$ skr eventbus-api process-configuration read PROCESS-CONFIGURATION-ID
```

## Step 3: スイッチ作成イベントのトリガーを作成する

イベントログのソースは `//eventbus.sakura.ad.jp/eventlog`、通常スイッチの作成タイプは `jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created` です。これらは [EventBus のイベントタイプ一覧](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html) と[ネットワーク関連のタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)に記載されています。`ZONE` は対象ゾーン（`is1a`、`is1b`、`is1c`、`tk1a`、`tk1b` のいずれか）に置き換えます。

次の JSON を `trigger.json` として保存し、`PROCESS-CONFIGURATION-ID` も Step 2 の実行設定 ID に置き換えます。`Conditions` はイベントの `zone` が指定値と一致する場合に絞り込みます。イベント条件で `zone` を使用できることは[イベントタイプ一覧](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)で確認できます。

```json
{
  "CommonServiceItem": {
    "Name": "eventbus-tutorial-switch-created",
    "Settings": {
      "Source": "//eventbus.sakura.ad.jp/eventlog",
      "Types": [
        "jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created"
      ],
      "Conditions": [
        {
          "Key": "zone",
          "Op": "eq",
          "Values": ["ZONE"]
        }
      ],
      "ProcessConfigurationID": "PROCESS-CONFIGURATION-ID"
    }
  }
}
```

```console
$ skr eventbus-api trigger create --request @trigger.json
$ skr eventbus-api trigger list
```

作成結果の `ID` を `TRIGGER-ID` として控えます。`list` にトリガーが表示されることを確認してください。

## Step 4: イベントを発生させて通知を確認する

対象ゾーンに、他の用途と共有しない通常スイッチを作成します。スイッチの作成操作は `skr eventbus-api` の対象外です。コントロールパネルなど、利用環境でスイッチを管理している手段を使ってください。EventBus がイベントを検知すると、実行設定で指定したメッセージがシンプル通知から通知先グループへ送られます。シンプル通知は設定済みのメールまたは Webhook などの通知先へ通知します（[シンプル通知の概要](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)）。

ジョブの実行はベストエフォート型です。即時実行を前提にせず、通知先グループで結果を確認してください。

実機では別の一時 SimpleMQ キューを実行先にして、同じスイッチ作成イベントを検証しました。`skr eventbus-api process-configuration create`、`update-secret`、`trigger create`、`trigger list` を実行し、`is1b` で作成したスイッチに対応するメッセージをキューから受信できました。試験で作成したトリガー、スイッチ、実行設定、キューは削除済みです。

この確認では、EventBus のイベント検知から SimpleMQ への配送までを実際のコマンドで検証しました。sakumock テストだけでは実際のイベント検知を検証できません。一方、このチュートリアルに記載したシンプル通知宛ての経路や、メール・Webhook などの通知到達は実機では検証していません。

## Step 5: 作成したリソースを削除する

まずトリガーを削除し、以降のイベントで通知が発生しないようにします。続いて、作成したテスト用スイッチを作成時と同じ管理手段で削除し、最後に実行設定を削除します。既存の通知先グループや API キーはこの手順では作成していないため削除しません。

```console
$ skr eventbus-api trigger delete TRIGGER-ID
$ skr eventbus-api trigger list
```

次に、テスト用スイッチを利用したコントロールパネルなどから削除します。その後、実行設定を削除し、`list` の結果に EventBus リソースが残っていないことを確認します。

```console
$ skr eventbus-api process-configuration delete PROCESS-CONFIGURATION-ID
$ skr eventbus-api process-configuration list
```

最後にローカルの `notification-secret.json` を削除してください。

## 参考資料

- [EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)
- [EventBus クイックスタート](https://manual.sakura.ad.jp/cloud/appliance/eventbus/basic.html)
- [EventBus API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/eventbus/api.html)
- [EventBus のトリガーとイベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)
- [EventBus のネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)
- [EventBus コントロールパネルの実行設定](https://manual.sakura.ad.jp/cloud/appliance/eventbus/control_panel.html)
- [シンプル通知の概要](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)
- `skr eventbus-api --help`
- `skr eventbus-api process-configuration create --help`
- `skr eventbus-api process-configuration update-secret --help`
- `skr eventbus-api trigger create --help`
