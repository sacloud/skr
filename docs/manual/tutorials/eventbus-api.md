# EventBus API: スイッチ作成イベントを SimpleMQ で受信する

EventBus はイベントやスケジュールをきっかけにジョブを実行します。このチュートリアルでは、通常スイッチの作成イベントで SimpleMQ にメッセージを送り、`skr simplemq-api message receive` で受信します。SimpleMQ はキューを介してソフトウェア間でメッセージを送受信するサービスです（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)、[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

> [!WARNING]
> この手順では SimpleMQ キュー、EventBus の実行設定とトリガー、テスト用スイッチを作成し、SimpleMQ キューからメッセージを受信します。実行前に `skr config current` で対象プロファイルとプロジェクトを確認してください。テスト専用の名前を使い、同じゾーンで他の通常スイッチを作成する操作と重ならない時間に実施してください。ゾーン条件が一致すると、そのゾーンで発生した他の通常スイッチ作成イベントもトリガーします。EventBus のジョブ実行はベストエフォート型で、即時実行は保証されません（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)、[ネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。

## 前提条件

- `skr` と `jq` が利用でき、対象プロジェクトの SDK プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- EventBus API で実行設定とトリガーを作成・削除できる権限があること（[EventBus API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/eventbus/api.html)）。
- SimpleMQ キューを作成・削除できる API 認証と、テスト用スイッチを作成・削除できる権限があること。SimpleMQ のキュー管理には「作成・削除」のアクセスレベルが必要です（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。
- `UNIQUE-SUFFIX` を他のリソース名と重複しない短い英数字またはハイフン列に置き換えること。SimpleMQ キュー名は5〜64文字で、同一プロジェクト内で一意にします。作成後にキュー名は変更できません（[SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。
- `ZONE` はイベントタイプが受け付けるゾーン（`is1a`、`is1b`、`is1c`、`tk1a`、`tk1b`）に置き換えること（[EventBus のネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。

## 作成するリソースと順序

1. **SimpleMQ キュー** — EventBus の送信先です。キューごとの API キーを発行します。
2. **EventBus 実行設定** — 送信先のキュー名とメッセージ本文を指定し、キューの API キーをシークレットとして登録します。
3. **EventBus トリガー** — 指定ゾーンの通常スイッチ作成イベントで実行設定を呼び出します。
4. **テスト用スイッチ** — `skr iaas-api switch` で作成し、対象イベントを発生させます。
5. **受信確認** — `skr simplemq-api message receive` で実行設定から送られた本文を確認します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr simplemq-api queue create --help
$ skr simplemq-api queue rotate-api-key --help
$ skr simplemq-api message receive --help
$ skr simplemq-api queue clear-messages --help
$ skr eventbus-api process-configuration create --help
$ skr eventbus-api process-configuration update-secret --help
$ skr eventbus-api trigger create --help
$ skr iaas-api switch create --help
```

意図したプロファイルとプロジェクトを確認してください。SimpleMQ はゾーンを指定しないグローバルリソースです。メッセージ API にはキューごとの API キーを使います（[SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。
`UNIQUE-SUFFIX` と `QUEUE-NAME` は前提条件に従って決め、`ZONE` は作成先に置き換えます。作成前に各一覧から同じ名前のリソースがないことを確認します。以下は CLI テストで確認した投影 JSON の形式に基づく、空一覧の出力例です。ほかのリソースが表示された場合は、作成予定の名前と重複しないことを確認してください。

```console
$ skr simplemq-api queue list --query 'map({ID,Name})'
[]
$ skr eventbus-api process-configuration list --query 'map({ID,Name})'
[]
$ skr eventbus-api trigger list --query 'map({ID,Name})'
[]
$ skr iaas-api switch find --zone ZONE --query 'map({ID,Name})'
[]
```

## Step 2: SimpleMQ キューと API キーを用意する

`UNIQUE-SUFFIX` は使用していない値に、`QUEUE-NAME` はその値を使った5〜64文字の一意な名前に置き換えます。Step 1 で同じ名前のキューがないことを確認してから作成します。

```console
$ umask 077
$ skr simplemq-api queue create --name QUEUE-NAME --description eventbus-tutorial/UNIQUE-SUFFIX > queue.json
$ QUEUE_ID=$(jq -er '.ID' queue.json)
$ skr simplemq-api queue list --query 'map({ID,Name})'
```

作成したキューの ID と名前が一覧に表示されることを確認します。次は CLI テストで確認した出力形式の例で、ID と名前は置き換えています。ほかのキューがある場合は同じ配列に含まれます。

```json
[
  {
    "ID": "123456789012",
    "Name": "QUEUE-NAME"
  }
]
```

キューごとの API キーを発行します。発行結果を画面に表示せず、EventBus のシークレット登録に使う JSON ファイルと、メッセージ受信に使う API キーファイルを作成します。再発行すると以前のキーは無効になります（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。

```console
$ skr simplemq-api queue rotate-api-key "$QUEUE_ID" > eventbus-secret.json
$ jq -er '.APIKey' eventbus-secret.json > simplemq.key
$ chmod 600 queue.json eventbus-secret.json simplemq.key
```

`eventbus-secret.json` は `process-configuration update-secret` 用、`simplemq.key` は `message receive --api-key-file` 用です。両ファイルとも作業端末上だけで管理し、コマンドラインやリポジトリにキーを書かないでください。

## Step 3: SimpleMQ 宛ての実行設定を作成する

`process-configuration.json` を作成します。`QUEUE-NAME` と `UNIQUE-SUFFIX` は Step 2 で決めた値に置き換えてください。`Parameters` には `queue_name` と `content` を持つ JSON を文字列として指定します。SimpleMQ のメッセージ本文には半角英数字と `+`、`/`、`=` を使います。この例の `EventBusSwitchCreated` はその条件を満たします（[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)、[実行設定の項目](https://manual.sakura.ad.jp/cloud/appliance/eventbus/control_panel.html)）。

```json
{
  "CommonServiceItem": {
    "Name": "eventbus-simplemq-job-UNIQUE-SUFFIX",
    "Description": "eventbus-simplemq/UNIQUE-SUFFIX",
    "Settings": {
      "Destination": "simplemq",
      "Parameters": "{\"queue_name\":\"QUEUE-NAME\",\"content\":\"EventBusSwitchCreated\"}"
    }
  }
}
```

実行設定を作成し、結果の `ID` を控えます。続いて、キューの API キーを実行設定のシークレットとして登録します。シークレットは `--secret-file` から読み込み、コマンド引数には含めません。

```console
$ skr eventbus-api process-configuration create --request @process-configuration.json > process-configuration-result.json
$ PROCESS_CONFIGURATION_ID=$(jq -er '.ID' process-configuration-result.json)
$ skr eventbus-api process-configuration update-secret "$PROCESS_CONFIGURATION_ID" --secret-file eventbus-secret.json
$ skr eventbus-api process-configuration read "$PROCESS_CONFIGURATION_ID"
$ skr eventbus-api process-configuration list --query 'map({ID,Name})'
```

作成した実行設定の `ID` と `Name` が一覧に表示されることを確認します。以下は CLI テストで確認した出力形式の例で、ID と名前は置き換えています。全件一覧から対象の要素だけを抜粋しています。

```json
[
  {
    "ID": "123456789013",
    "Name": "eventbus-simplemq-job-UNIQUE-SUFFIX"
  }
]
```

## Step 4: スイッチ作成イベントのトリガーを作成する

イベントログのソースは `//eventbus.sakura.ad.jp/eventlog` です。通常スイッチの作成タイプは `jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created` です（[EventBus のイベントタイプ一覧](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)、[ネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。次の内容を `trigger.json` として保存し、名前と `PROCESS-CONFIGURATION-ID` を作成した値に置き換えます。

```json
{
  "CommonServiceItem": {
    "Name": "eventbus-simplemq-trigger-UNIQUE-SUFFIX",
    "Description": "eventbus-simplemq/UNIQUE-SUFFIX",
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

`ZONE` は Step 5 でスイッチを作成するゾーンと同じ値にします。`Conditions` により `zone` が一致するイベントだけを対象にします。トリガーを作成して ID を控え、設定を読み戻します。

```console
$ skr eventbus-api trigger create --request @trigger.json > trigger-result.json
$ TRIGGER_ID=$(jq -er '.ID' trigger-result.json)
$ skr eventbus-api trigger read "$TRIGGER_ID"
$ skr eventbus-api trigger list --query 'map({ID,Name})'
```

作成したトリガーの `ID` と `Name` が一覧に表示されることを確認します。以下は CLI テストで確認した出力形式の例で、ID と名前は置き換えています。全件一覧から対象の要素だけを抜粋しています。

```json
[
  {
    "ID": "123456789014",
    "Name": "eventbus-simplemq-trigger-UNIQUE-SUFFIX"
  }
]
```

トリガーの作成後、設定が反映されるまで60秒待ってからスイッチを作成します。EventBus のジョブ実行はベストエフォート型のため、即時実行を前提にしないでください（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)）。

## Step 5: 通常スイッチを作成してイベントを発生させる

対象ゾーンで他の通常スイッチを作成する操作が重ならないことを確認してください。名前には Step 2 と同じ一意な接尾辞を使います。`ZONE` は Step 4 の値に置き換えます。

```console
$ skr iaas-api switch create --zone ZONE --name eventbus-switch-UNIQUE-SUFFIX --description "Temporary switch for EventBus tutorial" > switch-result.json
$ SWITCH_ID=$(jq -er '.ID' switch-result.json)
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --query 'map({ID,Name})'
```

作成したスイッチを対象にするイベントタイプは通常スイッチの作成です。作成結果の `ID`、`Name`、`Description` を確認し、後で削除するために `SWITCH_ID` を保持してください（[スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)）。
検索結果で作成したスイッチの ID と名前を照合します。以下は CLI テストで確認した出力形式の例で、ID と名前は置き換えています。

```json
[
  {
    "ID": 123456789015,
    "Name": "eventbus-switch-UNIQUE-SUFFIX"
  }
]
```

## Step 6: SimpleMQ からメッセージを受信する

実行設定に指定した本文は EventBus からキューへ送信されます。次のコマンドでキューを受信してください。

返却されたメッセージの `content` が `EventBusSwitchCreated` と一致することを確認してください。空配列の場合は、少し待ってからコマンドを再実行します。

メッセージの到着には時間を要する場合があります。EventBus は即時実行を保証しません。

```console
$ skr simplemq-api message receive --queue-name QUEUE-NAME --api-key-file simplemq.key
```

SimpleMQ は Pull 型で、受信リクエストごとにメッセージを配信します。配信保証は At least once です。そのため、同じメッセージを複数回受信する場合があります（[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

## Step 7: 作成したリソースを削除する

まずトリガーを削除して以降のイベントで実行設定が起動しないようにします。ID と名前を確認し、この手順で作成したリソースだけを削除してください。

```console
$ skr eventbus-api trigger delete "$TRIGGER_ID"
```

トリガーを削除した後、一覧に試験用の名前がないことを確認します。以下は一覧自体が空の場合の出力例です。実行設定とキューの削除確認も同様です。ほかのリソースが表示される場合は、試験用の名前の要素がないことを確認してください。

```console
$ skr eventbus-api trigger list --query 'map({ID,Name})'
[]
```

```console
$ skr iaas-api switch read --zone ZONE --id "$SWITCH_ID"
$ skr iaas-api switch delete --zone ZONE --id "$SWITCH_ID" --fail-if-not-found
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --query 'map({ID,Name})'
```

スイッチの検索結果がないことを確認します。

```json
[]
```

続けて実行設定を削除し、一覧に試験用の名前がないことを確認します。以下は一覧自体が空の場合の出力例です。

```console
$ skr eventbus-api process-configuration delete "$PROCESS_CONFIGURATION_ID"
$ skr eventbus-api process-configuration list --query 'map({ID,Name})'
[]
```

最後に、キュー内のメッセージを消去してからキューを削除します。キューの `Name` と `Description` がこの手順で作成したものと一致することを `queue read` で確認してください。

```console
$ skr simplemq-api queue read "$QUEUE_ID"
$ skr simplemq-api queue clear-messages "$QUEUE_ID"
$ skr simplemq-api queue delete "$QUEUE_ID"
$ skr simplemq-api queue list --query 'map({ID,Name})'
[]
$ rm queue.json eventbus-secret.json simplemq.key process-configuration.json process-configuration-result.json trigger.json trigger-result.json switch-result.json
```

一覧に対象のキュー、トリガー、実行設定、スイッチが残っていないことを確認してください。

## 参考情報

- [EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)
- [EventBus API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/eventbus/api.html)
- [EventBus の実行設定](https://manual.sakura.ad.jp/cloud/appliance/eventbus/control_panel.html)
- [EventBus のイベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)
- [EventBus のネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)
- [SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)
- [SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)
- [SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)
- [SimpleMQ メッセージ API](https://manual.sakura.ad.jp/api/cloud/portal/simplemq-api/index.html)
- [スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)
- [スイッチの作成・削除](https://manual.sakura.ad.jp/cloud/network/switch/router-switch.html)
- [sacloud-sdk-go v0.3.0: EventBus API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/eventbus)
- [sacloud-sdk-go v0.3.0: SimpleMQ API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/simplemq)
- [sacloud-sdk-go v0.3.0: Switch サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/swytch)
