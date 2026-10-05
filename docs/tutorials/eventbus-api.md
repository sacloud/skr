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
`UNIQUE-SUFFIX` と `QUEUE-NAME` は前提条件に従って決め、`ZONE` は作成先に置き換えます。作成前に同名リソースがないことを確認します。結果がすべて `[]` なら続行できます。

```console
$ skr simplemq-api queue list --output json | jq '[.[] | select(.Name == "QUEUE-NAME")]'
[]
$ skr eventbus-api process-configuration list --output json | jq '[.[] | select(.Name == "eventbus-simplemq-job-UNIQUE-SUFFIX")]'
[]
$ skr eventbus-api trigger list --output json | jq '[.[] | select(.Name == "eventbus-simplemq-trigger-UNIQUE-SUFFIX")]'
[]
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --output json | jq .
[]
```

## Step 2: SimpleMQ キューと API キーを用意する

`UNIQUE-SUFFIX` は使用していない値に、`QUEUE-NAME` はその値を使った5〜64文字の一意な名前に置き換えます。Step 1 の確認結果が `[]` であることを確かめてから作成します。

```console
$ umask 077
$ skr simplemq-api queue create --name QUEUE-NAME --description eventbus-tutorial/UNIQUE-SUFFIX --output json > queue.json
$ QUEUE_ID=$(jq -er '.ID' queue.json)
$ skr simplemq-api queue list --output table
```

作成したキューの行が一覧に表示されることを確認します。次はその行の出力例です（ID、名前、説明、QueueName、日時を置換しています）。ほかのキューも別の行に表示されます。表示列と値の省略位置は端末幅で変わります。

```text
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| ID           | Name           | Status | Description | Tags   | Availability | ServiceClass | CreatedAt | ModifiedAt |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| <QUEUE-ID>   | QUEUE-NAME...  | {"Q... | eventbus... |        | available    | cloud/sim... | 2026-1...  | 2026-10... |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
... +4 columns omitted
```

キューごとの API キーを発行します。発行結果を画面に表示せず、EventBus のシークレット登録に使う JSON ファイルと、メッセージ受信に使う API キーファイルを作成します。再発行すると以前のキーは無効になります（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。

```console
$ skr simplemq-api queue rotate-api-key "$QUEUE_ID" --output json > eventbus-secret.json
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
$ skr eventbus-api process-configuration create --request @process-configuration.json --output json > process-configuration-result.json
$ PROCESS_CONFIGURATION_ID=$(jq -er '.ID' process-configuration-result.json)
$ skr eventbus-api process-configuration update-secret "$PROCESS_CONFIGURATION_ID" --secret-file eventbus-secret.json
$ skr eventbus-api process-configuration read "$PROCESS_CONFIGURATION_ID" --output json
$ skr eventbus-api process-configuration list --output table
```

作成した実行設定の `ID` と `Name` が一覧に表示されることを確認します。以下は実機の出力形式に基づく行の抜粋で、ID、名前、説明、日時を置換しています。ほかの実行設定の行や端末幅による省略は環境によって変わります。

```text
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| ID           | Name           | Status | Description | Tags   | Availability | ServiceClass | CreatedAt | ModifiedAt |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| <ID>         | eventbus-job...|        | eventbus... |        | available    |              | 2026-1... | 2026-10... |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
... +4 columns omitted
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
$ skr eventbus-api trigger create --request @trigger.json --output json > trigger-result.json
$ TRIGGER_ID=$(jq -er '.ID' trigger-result.json)
$ skr eventbus-api trigger read "$TRIGGER_ID" --output json
$ skr eventbus-api trigger list --output table
```

作成したトリガーの `ID` と `Name` が一覧に表示されることを確認します。出力例は ID、名前、説明、時刻を編集した抜粋です。実際の列や表示幅は端末サイズと一覧の内容によって変わります。

```text
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| ID           | Name           | Status | Description | Tags   | Availability | ServiceClass | CreatedAt | ModifiedAt |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
| <TRIGGER-ID> | eventbus-trig… |        | eventbus-…  |        | available    | cloud/eve…   | 2026-10…  | 2026-10…   |
+--------------+----------------+--------+-------------+--------+--------------+--------------+-----------+------------+
... +4 columns omitted
```

トリガーの作成後、設定が反映されるまで60秒待ってからスイッチを作成します。EventBus のジョブ実行はベストエフォート型のため、即時実行を前提にしないでください（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)）。

## Step 5: 通常スイッチを作成してイベントを発生させる

対象ゾーンで他の通常スイッチを作成する操作が重ならないことを確認してください。名前には Step 2 と同じ一意な接尾辞を使います。`ZONE` は Step 4 の値に置き換えます。

```console
$ skr iaas-api switch create --zone ZONE --name eventbus-switch-UNIQUE-SUFFIX --description "Temporary switch for EventBus tutorial" --output json > switch-result.json
$ SWITCH_ID=$(jq -er '.ID' switch-result.json)
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --output table
```

作成したスイッチを対象にするイベントタイプは通常スイッチの作成です。作成結果の `ID`、`Name`、`Description` を確認し、後で削除するために `SWITCH_ID` を保持してください（[スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)）。
検索結果で作成したスイッチの ID と名前を照合します。以下は実機の出力形式に基づく行の抜粋で、ID、名前、説明、日時を置換しています。

```text
+--------------+--------------+-------------+--------+-------+-------------+----------------+--------------+-----------+
| ID           | Name         | Description | Tags   | Scope | ServerCount | NetworkMaskLen | DefaultRoute | CreatedAt |
+--------------+--------------+-------------+--------+-------+-------------+----------------+--------------+-----------+
| <ID>         | eventbus-... | eventbus-...|        | user  | 0           | 0              |              | 2026-1...  |
+--------------+--------------+-------------+--------+-------+-------------+----------------+--------------+-----------+
... +4 columns omitted
```

## Step 6: SimpleMQ からメッセージを受信する

実行設定に指定した本文は EventBus からキューへ送信されます。次のコマンドでキューを受信してください。

返却されたメッセージの `content` が `EventBusSwitchCreated` と一致することを確認してください。空配列の場合は、少し待ってからコマンドを再実行します。

メッセージの到着には時間を要する場合があります。EventBus は即時実行を保証しません。

```console
$ skr simplemq-api message receive --queue-name QUEUE-NAME --api-key-file simplemq.key --output json
```

SimpleMQ は Pull 型で、受信リクエストごとにメッセージを配信します。配信保証は At least once です。そのため、同じメッセージを複数回受信する場合があります（[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

## Step 7: 作成したリソースを削除する

まずトリガーを削除して以降のイベントで実行設定が起動しないようにします。ID と名前を確認し、この手順で作成したリソースだけを削除してください。

```console
$ skr eventbus-api trigger delete "$TRIGGER_ID"
$ skr eventbus-api trigger list --output json | jq '[.[] | select(.Name == "eventbus-simplemq-trigger-UNIQUE-SUFFIX")]'
[]
$ skr iaas-api switch read --zone ZONE --id "$SWITCH_ID" --output json
$ skr iaas-api switch delete --zone ZONE --id "$SWITCH_ID" --fail-if-not-found
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --output table
```

スイッチの検索結果がないことを確認します。

```text
+------------+
| No results |
+------------+
```

続けて実行設定を削除し、試験名で検索した結果が空であることを確認します。

```console
$ skr eventbus-api process-configuration delete "$PROCESS_CONFIGURATION_ID"
$ skr eventbus-api process-configuration list --output json | jq '[.[] | select(.Name == "eventbus-simplemq-job-UNIQUE-SUFFIX")]'
[]
```

最後に、キュー内のメッセージを消去してからキューを削除します。キューの `Name` と `Description` がこの手順で作成したものと一致することを `queue read` で確認してください。

```console
$ skr simplemq-api queue read "$QUEUE_ID" --output json
$ skr simplemq-api queue clear-messages "$QUEUE_ID"
$ skr simplemq-api queue delete "$QUEUE_ID"
$ skr simplemq-api queue list --output json | jq '[.[] | select(.Name == "QUEUE-NAME")]'
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
