# SimpleMQ API: キューを作成してメッセージを送受信する

シンプルMQは、キューを介してソフトウェア間でメッセージを送受信するマネージド型のメッセージキューサービスです。このチュートリアルでは、キューを作成し、メッセージを送信・受信して処理済みメッセージを削除します（[シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

> [!WARNING]
> この手順では、現在選択されているアカウントにキューを作成し、メッセージを送受信して削除します。キュー管理には「作成・削除」アクセスレベルを持つ API キーが必要です。実行前に `skr config current` で対象プロファイルを確認し、既存キューと重複しない名前を使用してください。キューの設定変更や削除は、手順で作成したキューだけに行ってください。

## 前提条件

- `skr simplemq-api` が利用でき、SDK のプロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- キューを管理する API キーに「作成・削除」のアクセスレベルがあること（[SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)）。
- この例ではキュー名 `simplemq-tutorial-UNIQUE-SUFFIX` と説明 `simplemq-tutorial/UNIQUE-SUFFIX` を使います。`UNIQUE-SUFFIX` は英数字とハイフンで構成する未使用の値に置き換えてください。

SimpleMQ のキューはゾーンを指定せずに操作するグローバルリソースです。キュー名は同じプロジェクト内で一意である必要があり、作成後に変更できません（[コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)）。

## 作成するリソース

この手順では、専用の SimpleMQ キューを1つ作成します。メッセージ操作にはキュー名と、そのキュー用の API キーを使います。キュー管理 API が返すリソース ID と、メッセージ API に渡すキュー名は異なる値です。作成結果の ID は `QUEUE_ID` に、受信したメッセージの ID は `MESSAGE_ID` に取り込みます。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr simplemq-api --help
$ skr simplemq-api create-queue --help
$ skr simplemq-api update-queue --help
$ skr simplemq-api rotate-api-key --help
$ skr simplemq-api send-message --help
$ skr simplemq-api receive-message --help
$ skr simplemq-api extend-message-timeout --help
$ skr simplemq-api delete-message --help
```

`config current` が対象のプロファイル名であることを確認します。キュー管理 API は SDK のプロファイルまたは環境変数の認証情報を使います。メッセージ API はキューごとの API キーを必要とし、`--api-key-file` でファイルを指定できます。値をファイルから読み込むには `--api-key-file -` を使います。

## Step 2: 専用キューを作成して設定する

次のコマンドでキューを作成します。作成結果の `ID` はコマンド置換で `QUEUE_ID` に取り込みます。文字列の抽出には `--query-raw` を使い、引用符なしで変数に取り込みます (以降の ID 取り込みも同じ手順です)。`UNIQUE-SUFFIX` は同じプロジェクト内で未使用の値に置き換えます。

```console
$ QUEUE_ID=$(skr simplemq-api create-queue --name QUEUE-NAME --description simplemq-tutorial/UNIQUE-SUFFIX --query-raw '.ID')
$ skr simplemq-api list-queues --query 'map({ID,Name})'
```

一覧から ID と名前だけを取り出して確認します。次は CLI テストで確認した出力形式の例で、ID とキュー名は置き換えています。ほかのキューがある場合は、同じ配列に含まれます。

```json
[
  {
    "ID": "123456789012",
    "Name": "QUEUE-NAME"
  }
]
```

キューの可視性タイムアウトと未処理メッセージ保存期間を設定します。`VisibilityTimeoutSeconds` は5〜900秒、`ExpireSeconds` は60〜1209600秒です。どちらも必要なので、通常は対応する CLI フラグで指定します。設定後、`read-queue` で反映を確認します。

```console
$ skr simplemq-api update-queue "$QUEUE_ID" --visibility-timeout-seconds 30 --expire-seconds 345600
$ skr simplemq-api read-queue "$QUEUE_ID" --query '{ID,Name,Settings}'
```

```json
{
  "ID": "123456789012",
  "Name": "QUEUE-NAME",
  "Settings": {
    "VisibilityTimeoutSeconds": 30,
    "ExpireSeconds": 345600
  }
}
```

`Description`、`Tags`、`Icon` も設定する場合は、ConfigQueueRequest の JSON を `--request` で指定します。個別フラグとは併用できません。次の例ではキュー作成時と同じ説明を合わせて指定します。

```console
$ cat > queue-settings.json <<'EOF'
{
  "CommonServiceItem": {
    "Description": "simplemq-tutorial/UNIQUE-SUFFIX",
    "Settings": {
      "VisibilityTimeoutSeconds": 30,
      "ExpireSeconds": 345600
    }
  }
}
EOF
$ skr simplemq-api update-queue "$QUEUE_ID" --request @queue-settings.json
$ rm queue-settings.json
```

## Step 3: API キーを用意してメッセージを送受信する

キューごとの API キーを発行します。再発行すると以前のキーは無効になるため、この手順で作成したキューだけを対象にしてください。キーはコマンドの出力に含まれるので、端末に表示せず、アクセス権を制限したファイルに保存します。

```console
$ umask 077
$ skr simplemq-api rotate-api-key "$QUEUE_ID" --query-raw '.APIKey' > simplemq.key
```

キュー名は `QUEUE-NAME` の値に置き換えます。メッセージ本文は API 定義に従い、最大256000文字で、英数字と `+`、`/`、`=` のみを使います。次の例では `HelloSimpleMQ` を送信し、受信したメッセージの `id` をコマンド置換で `MESSAGE_ID` に取り込みます。キューに受信できるメッセージがない場合は `MESSAGE_ID` が `null` になるため、少し待ってから受信し直してください。

```console
$ skr simplemq-api send-message --queue-name QUEUE-NAME --api-key-file simplemq.key --content HelloSimpleMQ
$ MESSAGE_ID=$(skr simplemq-api receive-message --queue-name QUEUE-NAME --api-key-file simplemq.key --query-raw '.[0].id')
```

送信結果の `id` と受信したメッセージの `id` が同じメッセージを指します。

受信リクエストはキュー内の古いメッセージから配信します。配信保証は At least once です。同じメッセージを複数回受信する場合があるため、アプリケーション側で重複を避ける設計にしてください（[シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)）。

処理に可視性タイムアウトより長い時間がかかる場合は、受信したメッセージのタイムアウトを延長します。延長結果にはメッセージの全文が含まれるため、`content` が送信した `HelloSimpleMQ` と一致することを確認してください。処理が終わったら、そのメッセージを削除します。

```console
$ skr simplemq-api extend-message-timeout --queue-name QUEUE-NAME --api-key-file simplemq.key "$MESSAGE_ID"
$ skr simplemq-api delete-message --queue-name QUEUE-NAME --api-key-file simplemq.key "$MESSAGE_ID"
$ skr simplemq-api read-message-count "$QUEUE_ID"
{"Count": 0}
```

この例では作成直後のキューに送信したメッセージを削除するため、`read-message-count` の `Count` が0であることを確認できます。
`extend-message-timeout` の出力では、同じメッセージ `id` が返り、`visibility_timeout_at` の値が受信時より増えていることも確認できます。

## Step 4: 作成したキューを削除する

削除前に `read-queue` で ID、名前、説明がこの手順で作成したキューと一致することを確認します。続けてキュー内のメッセージを消去し、キューを削除します。既存のキューは削除しないでください。

```console
$ skr simplemq-api read-queue "$QUEUE_ID" --query '{ID,Name,Description}'
{
  "ID": "123456789012",
  "Name": "QUEUE-NAME",
  "Description": "simplemq-tutorial/UNIQUE-SUFFIX"
}
$ skr simplemq-api clear-queue "$QUEUE_ID"
$ skr simplemq-api delete-queue "$QUEUE_ID"
$ skr simplemq-api list-queues --query 'map({ID,Name})'
[]
$ rm simplemq.key
```

`list-queues` に対象キューが残っていないことを確認します。API キーを別の場所に保存した場合は、そのファイルもこの手順で作成したものだけを削除してください。

## 参考資料

- [シンプルMQの基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)
- [SimpleMQ API 利用の基本手順](https://manual.sakura.ad.jp/cloud/appliance/simplemq/api.html)
- [SimpleMQ コントロールパネルでの操作](https://manual.sakura.ad.jp/cloud/appliance/simplemq/control_panel.html)
- [SimpleMQ API: メッセージ操作](https://manual.sakura.ad.jp/api/cloud/portal/simplemq-api/index.html)
- [SimpleMQ さくらのクラウド API: キュー管理](https://manual.sakura.ad.jp/api/cloud/portal/simplemq-sacloud-api/index.html)
