# EventBus: スイッチ作成イベントを SimpleMQ で受信する

EventBus はイベントやスケジュールをきっかけにジョブを実行します。このチュートリアルでは、`skr eventbus` の個別フラグで通常スイッチの作成イベントを SimpleMQ に送る構成を作り、`skr simplemq-api receive-message` で受信します。SimpleMQ はキューを介してソフトウェア間でメッセージを送受信するサービスです（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)、[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

`skr eventbus` は EventBus API の CommonServiceItem 操作を、宛先別の個別フラグで扱える高レベルコマンドです。低レベルの `skr eventbus-api` は JSON ボディで同じリソースを操作します。対応は次のとおりです。

| 高レベルコマンド | 低レベルコマンド |
| --- | --- |
| `skr eventbus process-configuration create` | `skr eventbus-api create-common-service-item --provider-class eventbusprocessconfiguration` |
| `skr eventbus trigger create` | `skr eventbus-api create-common-service-item --provider-class eventbustrigger` |
| `skr eventbus schedule create` | `skr eventbus-api create-common-service-item --provider-class eventbusschedule` |

実行設定の secret 登録はどちらも `--secret-file` で保護したファイルまたは標準入力から読み込みます。完全な JSON ボディ、条件の消去、条件値にカンマを含む入力が必要なときは低レベルの `eventbus-api` を使ってください。

> [!WARNING]
> この手順では SimpleMQ キュー、EventBus の実行設定とトリガー、テスト用スイッチを作成し、SimpleMQ キューからメッセージを受信します。実行前に `skr config current` で対象プロファイルとプロジェクトを確認してください。テスト専用の名前を使い、同じゾーンで他の通常スイッチを作成する操作と重ならない時間に実施してください。ゾーン条件が一致すると、そのゾーンで発生した他の通常スイッチ作成イベントもトリガーします。EventBus のジョブ実行はベストエフォート型で、即時実行は保証されません（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)、[ネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。

## 前提条件

- ID の抽出に `jq` を利用します。事前に導入してください。
- `skr` が利用でき、対象プロジェクトの SDK プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- EventBus API で実行設定とトリガーを作成・削除できる権限があること（[EventBus API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/eventbus/api.html)）。
- SimpleMQ キューを作成・削除できる API 認証と、テスト用スイッチを作成・削除できる権限があること。SimpleMQ のキュー管理には「作成・削除」のアクセスレベルが必要です（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。
- `UNIQUE-SUFFIX` を他のリソース名と重複しない短い英数字またはハイフン列に置き換えること。SimpleMQ キュー名は5〜64文字で、同一プロジェクト内で一意にします。作成後にキュー名は変更できません（[SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。
- `ZONE` はイベントタイプが受け付けるゾーン（`is1a`、`is1b`、`is1c`、`tk1a`、`tk1b`）に置き換えること。受け付けるゾーンはイベントタイプごとに確認してください（[EventBus のネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。

## 作成するリソースと順序

1. **SimpleMQ キュー** — EventBus の送信先です。キューごとの API キーを発行します。
2. **EventBus 実行設定** — 送信先のキュー名とメッセージ本文を個別フラグで指定し、キューの API キーをシークレットとして登録します。
3. **EventBus トリガー** — 指定ゾーンの通常スイッチ作成イベントで実行設定を呼び出します。
4. **テスト用スイッチ** — `skr iaas-api switch` で作成し、対象イベントを発生させます。
5. **受信確認** — `skr simplemq-api receive-message` で実行設定から送られた本文を確認します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr eventbus process-configuration create --help
$ skr eventbus process-configuration set-secret --help
$ skr eventbus trigger create --help
$ skr simplemq-api rotate-api-key --help
$ skr simplemq-api receive-message --help
$ skr simplemq-api clear-queue --help
$ skr iaas-api switch create --help
```

意図したプロファイルとプロジェクトを確認してください。SimpleMQ はゾーンを指定しないグローバルリソースです。メッセージ API にはキューごとの API キーを使います（[SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。
`UNIQUE-SUFFIX` と `QUEUE-NAME` は前提条件に従って決め、`ZONE` は作成先に置き換えます。作成前に各一覧から同じ名前のリソースがないことを確認します。以下は空一覧の出力例です。ほかのリソースが表示された場合は、作成予定の名前と重複しないことを確認してください。

```console
$ skr simplemq-api list-queues --query 'map({ID,Name})'
[]
$ skr eventbus process-configuration list --query 'map({ID,Name})'
[]
$ skr eventbus trigger list --query 'map({ID,Name})'
[]
$ skr iaas-api switch find --zone ZONE --query 'map({ID,Name})'
[]
```

## Step 2: SimpleMQ キューと API キーを用意する

`UNIQUE-SUFFIX` は使用していない値に、`QUEUE-NAME` はその値を使った5〜64文字の一意な名前に置き換えます。Step 1 で同じ名前のキューがないことを確認してから作成します。作成結果の `ID` はコマンド置換で `QUEUE_ID` に取り込みます。`--query` は JSON で出力するため、`jq -er '.'` で文字列をデコードしてから変数に取り込みます (以降の ID 取り込みも同じ手順です)。

```console
$ umask 077
$ QUEUE_ID=$(skr simplemq-api create-queue --name QUEUE-NAME --description eventbus-tutorial/UNIQUE-SUFFIX --query '.ID' | jq -er '.')
$ skr simplemq-api list-queues --query 'map({ID,Name})'
```

作成したキューの ID と名前が一覧に表示されることを確認します。以下は出力形式の例で、ID と名前は置き換えています。キューの ID は数値で返ります。ほかのキューがある場合は同じ配列に含まれます。

```json
[
  {
    "ID": 123456789012,
    "Name": "QUEUE-NAME"
  }
]
```

キューごとの API キーを発行します。発行結果を画面に表示せず、EventBus のシークレット登録に使う JSON ファイルと、メッセージ受信に使う API キーファイルを作成します。再発行すると以前のキーは無効になります（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。

```console
$ skr simplemq-api rotate-api-key "$QUEUE_ID" > eventbus-secret.json
$ jq -er '.APIKey' eventbus-secret.json > simplemq.key
$ chmod 600 eventbus-secret.json simplemq.key
```

`eventbus-secret.json` は `set-secret` 用、`simplemq.key` は `receive-message --api-key-file` 用です。両ファイルとも作業端末上だけで管理し、コマンドラインやリポジトリにキーを書かないでください。

## Step 3: SimpleMQ 宛ての実行設定を作成する

実行設定でジョブの宛先を定義します。simplemq 宛先は `--queue-name` と `--content` で指定します。`QUEUE-NAME` は Step 2 で作成したキュー名に、メッセージ本文は半角英数字と `+`、`/`、`=` で構成します。この例の `EventBusSwitchCreated` はその条件を満たします（[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)、[実行設定の項目](https://manual.sakura.ad.jp/cloud/appliance/eventbus/control_panel.html)）。
低レベルの `eventbus-api` では、これらの値を `Parameters` の JSON 文字列として指定します。高レベルコマンドは同じ API への入力を個別フラグとして受け付けます。

```console
$ PROCESS_CONFIGURATION_ID=$(skr eventbus process-configuration create --name eventbus-simplemq-job-UNIQUE-SUFFIX --description eventbus-simplemq/UNIQUE-SUFFIX --destination simplemq --queue-name QUEUE-NAME --content EventBusSwitchCreated --query '.ID' | jq -er '.')
$ skr eventbus process-configuration read "$PROCESS_CONFIGURATION_ID"
```

参照結果には宛先とパラメーターが表示されます。以下は出力形式の例で、ID は置き換えています。EventBus のリソース ID は文字列で返ります。

```json
{
  "ID": "123456789013",
  "Name": "eventbus-simplemq-job-UNIQUE-SUFFIX",
  "Description": "eventbus-simplemq/UNIQUE-SUFFIX",
  "Destination": "simplemq",
  "Parameters": {
    "queue_name": "QUEUE-NAME",
    "content": "EventBusSwitchCreated"
  },
  "Tags": [],
  "CreatedAt": "2026-01-01T00:00:00Z",
  "ModifiedAt": "2026-01-01T00:00:00Z"
}
```

作成した実行設定にキューの API キーを secret として登録します。secret は `--secret-file` で保護したファイル (`@path`) または標準入力 (`-`) から読み込みます。SimpleMQ 宛先の secret は `{"APIKey":"..."}` の形式です。secret は読み戻せないため、API が受け付けたことだけを確認します。

```console
$ skr eventbus process-configuration set-secret "$PROCESS_CONFIGURATION_ID" --secret-file @eventbus-secret.json
```

## Step 4: スイッチ作成イベントのトリガーを作成する

イベントログの送信元は `//eventbus.sakura.ad.jp/eventlog` です。通常スイッチの作成タイプは `jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created` です（[EventBus のイベントタイプ一覧](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)、[ネットワーク関連イベントタイプ](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events_network.html)）。
条件は `--condition 'key=名前,op=eq|in,value=値'` の形式で指定します。ここでは `zone` がスイッチを作成するゾーンと一致するイベントだけを対象にします。`ZONE` は Step 5 でスイッチを作成するゾーンと同じ値にします。

```console
$ TRIGGER_ID=$(skr eventbus trigger create --name eventbus-simplemq-trigger-UNIQUE-SUFFIX --description eventbus-simplemq/UNIQUE-SUFFIX --source //eventbus.sakura.ad.jp/eventlog --type jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created --condition 'key=zone,op=eq,value=ZONE' --process-configuration-id "$PROCESS_CONFIGURATION_ID" --query '.ID' | jq -er '.')
$ skr eventbus trigger read "$TRIGGER_ID"
```

参照結果の `Types` と `Conditions` に指定したイベントタイプと条件が表示されることを確認します。

トリガーの作成後、設定が反映されるまで60秒待ってからスイッチを作成します。EventBus のジョブ実行はベストエフォート型のため、即時実行を前提にしないでください（[EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)）。

## Step 5: 通常スイッチを作成してイベントを発生させる

対象ゾーンで他の通常スイッチを作成する操作が重ならないことを確認してください。名前には Step 2 と同じ一意な接尾辞を使います。`ZONE` は Step 4 の値に置き換えます。

```console
$ SWITCH_ID=$(skr iaas-api switch create --zone ZONE --name eventbus-switch-UNIQUE-SUFFIX --description "Temporary switch for EventBus tutorial" --query '.ID' | jq -er '.')
$ skr iaas-api switch read --zone ZONE --id "$SWITCH_ID" --query '{ID,Name,Description}'
```

作成したスイッチを対象にするイベントタイプは通常スイッチの作成です。作成結果の `ID`、`Name`、`Description` を確認し、後で削除するために `SWITCH_ID` を保持してください（[スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)）。

## Step 6: SimpleMQ からメッセージを受信する

実行設定に指定した本文は EventBus からキューへ送信されます。次のコマンドでキューを受信してください。返却されたメッセージの `content` が `EventBusSwitchCreated` と一致することを確認してください。空配列の場合は、少し待ってからコマンドを再実行します。メッセージの到着には時間を要する場合があります。EventBus は即時実行を保証しません。

```console
$ skr simplemq-api receive-message --queue-name QUEUE-NAME --api-key-file simplemq.key
```

SimpleMQ は Pull 型で、受信リクエストごとにメッセージを配信します。配信保証は At least once です。そのため、同じメッセージを複数回受信する場合があります（[SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

## Step 7: スケジュールで定期実行を設定する (任意)

トリガー以外に、開始日時からの繰り返しで実行設定を呼び出すスケジュールも作成できます。`--starts-at` に開始日時を RFC3339 形式 (小数秒はミリ秒まで) で指定し、`--recurring-step` と `--recurring-unit` で間隔を指定します。`--recurring-step` と `--recurring-unit` の代わりに `--crontab` で Crontab 形式の繰り返しを指定する方法もありますが、このチュートリアルでは開始日時からの繰り返しを使います。
`STARTS-AT` は現在より後の日時に置き換えてください。発火させないよう、削除までの時間より十分に後の日時を指定します。作成後、`read` で ID、名前、開始日時、繰り返しの設定がこの手順で作成したものと一致することを確認し、一致しない場合は削除を中止してください。確認後にスケジュールを削除します。

```console
$ SCHEDULE_ID=$(skr eventbus schedule create --name eventbus-simplemq-schedule-UNIQUE-SUFFIX --description eventbus-simplemq/UNIQUE-SUFFIX --process-configuration-id "$PROCESS_CONFIGURATION_ID" --starts-at STARTS-AT --recurring-step 1 --recurring-unit hour --query '.ID' | jq -er '.')
$ skr eventbus schedule read "$SCHEDULE_ID"
$ skr eventbus schedule delete "$SCHEDULE_ID"
$ skr eventbus schedule list --query 'map({ID,Name})'
[]
```

## Step 8: 作成したリソースを削除する

まずトリガーを削除して以降のイベントで実行設定が起動しないようにします。Step 4 で保持した `TRIGGER_ID` を削除し、一覧に試験用の名前がないことを確認します。削除の前に `read` で ID、名前、説明、`ProcessConfigurationID` がこの手順で作成したものと一致することを確認してください。一致しない場合は別の用途に変更されている可能性があるため、削除を中止してください。

```console
$ skr eventbus trigger read "$TRIGGER_ID"
$ skr eventbus trigger delete "$TRIGGER_ID"
$ skr eventbus trigger list --query 'map({ID,Name})'
[]
```

続けて実行設定を削除し、一覧に試験用の名前がないことを確認します。削除の前に `read` で ID、名前、説明、宛先 (`Destination`) と宛先別のパラメーターがこの手順で作成したものと一致することを確認してください。一致しない場合は削除を中止してください。Step 7 でスケジュールを作成した場合は、削除済みであることを同じ方法で確認してください。

```console
$ skr eventbus process-configuration read "$PROCESS_CONFIGURATION_ID"
$ skr eventbus process-configuration delete "$PROCESS_CONFIGURATION_ID"
$ skr eventbus process-configuration list --query 'map({ID,Name})'
[]
```

スイッチを削除します。削除の前に `read` で ID、名前、説明がこの手順で作成したものと一致することを確認してください。一覧から消失したことも確認します。

```console
$ skr iaas-api switch read --zone ZONE --id "$SWITCH_ID" --query '{ID,Name,Description}'
$ skr iaas-api switch delete --zone ZONE --id "$SWITCH_ID" --fail-if-not-found
$ skr iaas-api switch find --request='{"Zone":"ZONE","Names":["eventbus-switch-UNIQUE-SUFFIX"]}' --query 'map({ID,Name})'
[]
```

最後に、キュー内のメッセージを消去してからキューを削除します。キューの `Name` と `Description` がこの手順で作成したものと一致することを `read-queue` で確認してください。

```console
$ skr simplemq-api read-queue "$QUEUE_ID"
$ skr simplemq-api clear-queue "$QUEUE_ID"
$ skr simplemq-api delete-queue "$QUEUE_ID"
$ skr simplemq-api list-queues --query 'map({ID,Name})'
[]
$ rm eventbus-secret.json simplemq.key
```

一覧に対象のキュー、トリガー、実行設定、スイッチが残っていないことを確認してください。

## まとめ

`skr eventbus` の個別フラグで、SimpleMQ 宛ての実行設定とスイッチ作成イベントのトリガーを作成し、キューへのメッセージ配送を確認しました。secret は保護したファイルから読み込むため、キーの値がコマンド引数や履歴に残りません。
スケジュールの発火による実際のジョブ実行、simplemq 以外の宛先 (`simplenotification`、`autoscale`)、条件値にカンマを含むトリガーなどは、このチュートリアルでは扱っていません。条件の消去などの完全な JSON 入力が必要な操作は低レベルの `skr eventbus-api` を使ってください。

## 参考資料

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
- [sacloud-sdk-go v0.5.0: EventBus API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.5.0/api/eventbus)
- [sacloud-sdk-go v0.5.0: SimpleMQ API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.5.0/api/simplemq)
- [sacloud-sdk-go v0.5.0: Switch サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.5.0/service/iaas/swytch)
