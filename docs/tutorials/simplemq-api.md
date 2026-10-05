# SimpleMQ API: キューを作成してメッセージを送受信する

シンプルMQは、キューを介してソフトウェア間でメッセージを送受信するマネージド型のメッセージキューサービスです。このチュートリアルでは、キューを作成し、メッセージを送信・受信して処理済みメッセージを削除します（[シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

> [!WARNING]
> この手順では、現在選択されているアカウントにキューを作成し、メッセージを送受信して削除します。キュー管理には「作成・削除」アクセスレベルを持つ API キーが必要です。実行前に `skr config current` で対象プロファイルを確認し、既存キューと重複しない名前を使用してください。キューの設定変更や削除は、手順で作成したキューだけに行ってください。

## 前提条件

- `skr simplemq-api` が利用でき、SDK のプロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- キューを管理する API キーに「作成・削除」のアクセスレベルがあること（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。
- API キー出力をキューファイルに保存するため、`jq` が利用できること。
- この例ではキュー名 `simplemq-tutorial-UNIQUE-SUFFIX` と説明 `simplemq-tutorial/UNIQUE-SUFFIX` を使います。`UNIQUE-SUFFIX` は英数字とハイフンで構成する未使用の値に置き換えてください。

SimpleMQ のキューはゾーンを指定せずに操作するグローバルリソースです。キュー名は同じプロジェクト内で一意である必要があり、作成後に変更できません（[コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。

## 作成するリソース

この手順では、専用の SimpleMQ キューを1つ作成します。メッセージ操作にはキュー名と、そのキュー用の API キーを使います。キュー管理 API が返すリソース ID と、メッセージ API に渡すキュー名は異なる値です。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr simplemq-api --help
$ skr simplemq-api queue --help
$ skr simplemq-api queue create --help
$ skr simplemq-api queue config --help
$ skr simplemq-api queue rotate-api-key --help
$ skr simplemq-api message send --help
$ skr simplemq-api message receive --help
$ skr simplemq-api message extend-timeout --help
$ skr simplemq-api message delete --help
```

`config current` が対象のプロファイル名であることを確認します。キュー管理 API は SDK のプロファイルまたは環境変数の認証情報を使います。メッセージ API はキューごとの API キーを必要とし、`--api-key-file` でファイルを指定できます。値をファイルから読み込むには `--api-key-file -` を使います。

## Step 2: 専用キューを作成して設定する

次のコマンドでキューを作成します。作成結果の `ID` を控え、以降の `QUEUE-ID` に指定してください。`QUEUE-NAME` は同じプロジェクト内で未使用の名前に置き換えます。

```console
$ skr simplemq-api queue create --name QUEUE-NAME --description simplemq-tutorial/UNIQUE-SUFFIX --output json
```

キューの可視性タイムアウトと未処理メッセージ保存期間を設定します。`VisibilityTimeoutSeconds` は5〜900秒、`ExpireSeconds` は60〜1209600秒です。どちらも必要なので、通常は対応する CLI フラグで指定します。

```console
$ skr simplemq-api queue config QUEUE-ID --visibility-timeout-seconds 30 --expire-seconds 345600 --output json
$ skr simplemq-api queue read QUEUE-ID --output json
```

`Description`、`Tags`、`Icon` も設定する場合は、ConfigQueueRequest の JSON を `--request` で指定します。個別フラグとは併用できません。

`queue-settings.json`:

```json
{
  "CommonServiceItem": {
    "Settings": {
      "VisibilityTimeoutSeconds": 30,
      "ExpireSeconds": 345600
    }
  }
}
```

```console
$ skr simplemq-api queue config QUEUE-ID --request @queue-settings.json --output json
```

## Step 3: API キーを用意してメッセージを送受信する

キューごとの API キーを発行します。再発行すると以前のキーは無効になるため、この手順で作成したキューだけを対象にしてください。キーはコマンドの出力に含まれるので、端末に表示せず、アクセス権を制限したファイルに保存します。

```console
$ umask 077
$ skr simplemq-api queue rotate-api-key QUEUE-ID --output json | jq -er .APIKey > simplemq.key
```

キュー名は `QUEUE-NAME` の値に置き換えます。メッセージ本文は API 定義に従い、最大256000文字で、英数字と `+`、`/`、`=` のみを使います。次の例では `HelloSimpleMQ` を送信します。

```console
$ skr simplemq-api message send --queue-name QUEUE-NAME --api-key-file simplemq.key --content HelloSimpleMQ --output json
$ skr simplemq-api message receive --queue-name QUEUE-NAME --api-key-file simplemq.key --output json
```

送信結果の `id` と受信結果の `id` が一致し、受信結果の `content` が `HelloSimpleMQ` であることを確認してください。受信結果の `id` を続く操作の `MESSAGE-ID` に指定します。

受信リクエストはキュー内の古いメッセージから配信します。配信保証は At least once です。同じメッセージを複数回受信する場合があるため、アプリケーション側で重複を避ける設計にしてください（[シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

処理に可視性タイムアウトより長い時間がかかる場合は、受信したメッセージのタイムアウトを延長します。処理が終わったら、そのメッセージを削除します。

```console
$ skr simplemq-api message extend-timeout --queue-name QUEUE-NAME --api-key-file simplemq.key MESSAGE-ID --output json
$ skr simplemq-api message delete --queue-name QUEUE-NAME --api-key-file simplemq.key MESSAGE-ID
$ skr simplemq-api queue count-messages QUEUE-ID --output json
```

この例では作成直後のキューに送信したメッセージを削除するため、`count-messages` の `Count` が0であることを確認できます。
`extend-timeout` の出力では、同じメッセージ `id` が返り、`visibility_timeout_at` の値が受信時より増えていることも確認できます。

## Step 4: 作成したキューを削除する

削除前に `queue read` で ID、名前、説明がこの手順で作成したキューと一致することを確認します。続けてキュー内のメッセージを消去し、キューを削除します。既存のキューは削除しないでください。

```console
$ skr simplemq-api queue read QUEUE-ID --output json
$ skr simplemq-api queue clear-messages QUEUE-ID
$ skr simplemq-api queue delete QUEUE-ID
$ skr simplemq-api queue list --output table
$ rm simplemq.key
```

`queue list` に対象キューが残っていないことを確認します。削除後に結果が空の場合の出力は次のとおりです。

```text
+------------+
| No results |
+------------+
```

API キーを別の場所に保存した場合は、そのファイルもこの手順で作成したものだけを削除してください。

## 参考資料

- [シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)
- [SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)
- [SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)
- [SimpleMQ API: メッセージ操作](https://manual.sakura.ad.jp/api/cloud/portal/simplemq-api/index.html)
- [SimpleMQ さくらのクラウド API: キュー管理](https://manual.sakura.ad.jp/api/cloud/portal/simplemq-sacloud-api/index.html)
