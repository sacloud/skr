# Simple Notification API: グループに通知を送り、履歴を確認する

シンプル通知は、メールまたは Webhook を使って任意のタイミングで通知を送信します。通知先グループに送信先をまとめておくことで、送信元が個別のメールアドレスや Webhook URL を意識せずに利用できます（[シンプル通知の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)、[通知先グループ](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_group.html)）。

> [!WARNING]
> この手順では、既存の通知先を含む一時的な通知先グループを作成し、実際の通知を1件送信してから、そのグループだけを削除します。実行前に `skr config current` でプロファイルを確認し、通知先と影響範囲について必要な承認を得てください。通知の到達は保証されません。メールマガジン、確実性が必要な障害通知、短時間に同一宛先へ繰り返し通知する用途には使わないでください（[サービス基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)）。

## 前提条件

- `skr simple-notification-api` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。
- 通知 API の対象操作を利用する権限が付与されていること。API キーを使う場合は「作成・削除」アクセスレベルが必要です（[通知API](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/api.html)）。
- 通知先 ID を把握していること。メールはアドレス確認済みで、通知先が有効かつ送信可能であること。送信可能状態は `destination read-status` で確認できます（[通知先](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination.html)）。
- 他の通知先グループと重複しない `GROUP-NAME` と説明を決めること。通知送信の本文はテスト用の文字列にし、秘密情報や個人情報を含めないでください。
- チュートリアル中に作成するのは通知先グループ1件だけです。既存の通知先は作成・更新・削除しません。

## 作成するリソースと順序

1. 既存の通知先の送信可能状態を確認します。
2. その通知先だけを含む一時的な通知先グループを作成します。
3. グループにテスト通知を送り、通知履歴で送信処理の状態を確認します。
4. ID と設定を照合したうえで、一時グループを削除します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr simple-notification-api --help
$ skr simple-notification-api destination read-status --help
$ skr simple-notification-api group --help
$ skr simple-notification-api group create --help
$ skr simple-notification-api group read --help
$ skr simple-notification-api group send-message --help
$ skr simple-notification-api group delete --help
$ skr simple-notification-api history list --help
```

認証は選択中の SDK プロファイルを使います。追加の環境変数は不要です。`GROUP-NAME` は、以降のコマンドと JSON で同じ一意な名前に置き換えてください。

## Step 2: 通知先とグループ名を確認する

`DESTINATION-ID` は、利用を承認された既存の通知先 ID に置き換えます。`read-status` は送信可能状態を返します。`IsValid` が `true` であることを確認してください。通知先の詳細出力にはアドレスが含まれるため、この手順では詳細を表示しません。

新しいグループ名に一致するリソースがないことも確認します。名前が未使用の場合の出力は `[]` です。これは期待する出力形式であり、ライブ環境で取得した結果ではありません。実際に別の要素が表示された場合は、そのリソースを変更・削除せず、異なる名前を選んでください。

```console
$ skr simple-notification-api destination read-status DESTINATION-ID
$ skr simple-notification-api group list --query '.CommonServiceItems | map(select(.Name == "GROUP-NAME") | {ID,Name,Description})'
```

```json
[]
```

## Step 3: 一時グループを作成する

次の内容を `group-create.json` に保存します。`GROUP-NAME`、説明、`DESTINATION-ID` を自分の値に置き換えてください。`Settings.Destinations` は通知先 ID の配列です。`Provider.Class` と `ServiceClass` は CLI が設定するため JSON に書きません。

```json
{
  "CommonServiceItem": {
    "Name": "GROUP-NAME",
    "Description": "Temporary group for Simple Notification tutorial",
    "Settings": {
      "Destinations": ["DESTINATION-ID"],
      "Disabled": false
    }
  }
}
```

作成は保護したファイルから読み込みます。作成結果の `CommonServiceItem.ID` を控え、後続の `GROUP-ID` に指定してください。続けて `group read` を実行し、ID、名前、説明、通知先 ID が指定した値と一致することを確認します。

作成コマンドがエラーになり ID を取得できなかった場合は、名前だけを手掛かりに削除しないでください。作成された可能性がある場合は、コントロールパネルなどで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

```console
$ skr simple-notification-api group create --request @group-create.json
$ skr simple-notification-api group read GROUP-ID
```

作成直後に一覧でも確認する場合は、名前で対象を絞り、確認に必要な項目だけを表示します。次はテストで使う応答形式を ID と名前のプレースホルダーに置き換えた例であり、ライブ環境の実行結果ではありません。

```console
$ skr simple-notification-api group list --query '.CommonServiceItems | map(select(.Name == "GROUP-NAME") | {ID,Name,Description,Settings})'
```

```json
[
  {
    "ID": "GROUP-ID",
    "Name": "GROUP-NAME",
    "Description": "Temporary group for Simple Notification tutorial",
    "Settings": {
      "Destinations": ["DESTINATION-ID"],
      "Disabled": false
    }
  }
]
```

## Step 4: グループへテスト通知を送信する

通知 API は通知先 ID ではなく通知先グループ ID を受け取ります。本文は API の `Message` フィールドに対応し、`--message` と `--request` は併用できません。API の成功応答は受付を示すもので、通知先への到達を示すものではありません（[通知API](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/api.html)）。

```console
$ skr simple-notification-api group send-message GROUP-ID --message 'Simple Notification tutorial test'
```

## Step 5: 通知履歴を確認する

通知履歴には通知リクエストの受信時刻、本文、通知先ごとの状態が含まれます。API で取得できる履歴は直近30日以内の最大100件です。次のコマンドで対象メッセージを見つけ、`statuses` のグループ ID と通知先 ID が今回の値に一致することを確認してください。状態 `2` は通知処理の完了を示しますが、メールや Webhook の到達完了を保証しません（[通知履歴](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/history.html)、[通知 API の仕様](https://manual.sakura.ad.jp/api/cloud/portal/simple-notification-api/index.html)）。

```console
$ skr simple-notification-api history list --query '.NotificationHistories | map({request_id,received_at,message:.message.body,statuses:[.statuses[] | {group_id,destination_id,status}]})'
```

次は API 仕様のレスポンス例を基に、ID、時刻、本文をプレースホルダーに置き換えた形状例です。ライブ環境での実行結果ではありません。

```json
[
  {
    "request_id": "REQUEST-ID",
    "received_at": "RECEIVED-AT",
    "message": "Simple Notification tutorial test",
    "statuses": [
      {
        "group_id": "GROUP-ID",
        "destination_id": "DESTINATION-ID",
        "status": 2
      }
    ]
  }
]
```

通知先の詳細状態は `destination read-status` で確認できます。通知処理が失敗した場合は履歴の状態を確認し、到達を確認したものとして扱わないでください。

## Step 6: 作成したグループだけを削除する

削除前に `group read` で ID、名前、説明、通知先 ID がこの手順で作成したグループと一致することを再確認します。一致しない場合や ID を確認できない場合は削除しません。通知先は既存リソースのため削除しません。

```console
$ skr simple-notification-api group read GROUP-ID
$ skr simple-notification-api group delete GROUP-ID
$ skr simple-notification-api group list --query '.CommonServiceItems | map(select(.ID == "GROUP-ID") | {ID,Name,Description})'
```

対象 ID が一覧に残っていない場合の出力は `[]` です。以下は E2E テストで確認している削除後の出力形式です。

```json
[]
```

通知先、通知先グループ、ルーティング、通知履歴の操作一覧は、それぞれの `--help` で確認できます。ルーティングは通知を受け取るサービスを指定する設定です（[通知先ルーティング](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_routing.html)）。

## 参考資料

- [シンプル通知の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/about.html)
- [通知先](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination.html)
- [通知先グループ](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_group.html)
- [通知先ルーティング](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/destination_routing.html)
- [通知履歴](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/history.html)
- [通知API](https://manual.sakura.ad.jp/cloud/appliance/simplenotification/api.html)
- [Simple Notification API / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/simple-notification)
