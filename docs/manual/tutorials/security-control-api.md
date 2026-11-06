# SecurityControl API: セキュリティコントロールを有効化し、評価ルールと自動アクションを操作する

さくらのクラウド セキュリティコントロール (Security Control) は、プロジェクトのリソース設定を推奨構成と照合して評価し、評価結果をイベントログに記録するサービスです。有効化には評価対象リソースへのアクセス権限を付与するサービスプリンシパルの指定が必要で、評価ルールごとに有効状態と評価対象を変更でき、評価結果をきっかけに通知やワークフローを実行する自動アクションを登録できます（[イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)）。

> [!WARNING]
> この手順では、プロジェクトのセキュリティコントロールの有効化状態と評価ルールの設定を変更し、自動アクションを 1 件作成してから削除します。変更前の状態を記録し、後片付けで元の状態に戻します。実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。

## 前提条件

- `skr security-control-api` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。
- 有効化と評価ルールの更新で指定するサービスプリンシパル ID (`SERVICE-PRINCIPAL-ID`) を決めること。評価対象リソースへのアクセス権限を付与する IAM のサービスプリンシパルを指定します。
- 自動アクションの通知先として、シンプル通知の通知先グループが既にあること。作成方法は `skr simple-notification-api group create --help` を参照してください。
- API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/tk1b/api/securitycontrol/1.0/` です。エンドポイントは `SAKURA_ENDPOINTS_SECURITY_CONTROL` 環境変数で変更できます。

## 変更する設定と順序

1. セキュリティコントロールの有効化状態を確認し、未有効化なら有効化します。
2. 評価ルールの一覧を確認し、1 件を参照します。
3. 評価ルールの 1 件を更新し、元の設定に戻します。
4. 自動アクションを作成し、一覧と詳細を確認して、無効化してから削除します。
5. 後片付けで、変更した有効化状態を元に戻します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr security-control-api --help
$ skr security-control-api activation read --help
$ skr security-control-api activation create --help
$ skr security-control-api activation update --help
$ skr security-control-api evaluation-rule list --help
$ skr security-control-api evaluation-rule read --help
$ skr security-control-api evaluation-rule update --help
$ skr security-control-api automated-action list --help
$ skr security-control-api automated-action read --help
$ skr security-control-api automated-action create --help
$ skr security-control-api automated-action update --help
$ skr security-control-api automated-action delete --help
```

認証は選択中の SDK プロファイルを使います。`SERVICE-PRINCIPAL-ID` は、以降のコマンドで同じサービスプリンシパル ID に置き換えてください。

## Step 2: 有効化状態を確認して有効化する

現在の有効化状態を確認します。プロジェクトがまだ一度も有効化されていない場合は `404` が返ります。

```console
$ skr security-control-api activation read
```

```json
{
  "servicePrincipalId": "SERVICE-PRINCIPAL-ID",
  "isActive": true,
  "automatedActionLimit": 10
}
```

未有効化のプロジェクトでは `404` が返ります。この場合、次のコマンドで有効化します。
既に有効な場合はこの手順を飛ばし、後片付けで有効化状態を変更しません。
一度でも有効化したプロジェクトでは、`update --is-active=false` で無効化した後も `create` は `409` で拒否されます。
無効化した状態から再度有効化する場合は `create` ではなく `update` を使います。

```console
$ skr security-control-api activation create --service-principal-id 'SERVICE-PRINCIPAL-ID'
```

```json
{
  "servicePrincipalId": "SERVICE-PRINCIPAL-ID",
  "isActive": true,
  "automatedActionLimit": 10
}
```

有効化した場合は、後片付け (Step 6) で `activation update` を実行します。

## Step 3: 評価ルールの一覧を確認して 1 件を参照する

```console
$ skr security-control-api evaluation-rule list
```

```json
{
  "items": [
    {
      "rule": {
        "evaluationRuleId": "server-no-public-ip"
      },
      "iamRolesRequired": [],
      "isEnabled": true
    }
  ],
  "totalSize": 1
}
```

出力が長い場合は `--query` で絞り込めます。

```console
$ skr security-control-api evaluation-rule list --query 'map(.items[] | select(.isEnabled) | {id: .rule.evaluationRuleId})'
```

ルール ID をシェル変数に取り込み、1 件を参照します。評価ルールの ID は固定のため、以降の手順では `server-no-public-ip` を使います。

```console
$ RULE_ID=$(skr security-control-api evaluation-rule list --query '.items[] | select(.rule.evaluationRuleId == "server-no-public-ip") | .rule.evaluationRuleId')
$ skr security-control-api evaluation-rule read "$RULE_ID"
```

```json
{
  "rule": {
    "evaluationRuleId": "server-no-public-ip"
  },
  "iamRolesRequired": [],
  "isEnabled": true
}
```

後片付けで元に戻すため、更新するルールの現在の `isEnabled` と、ゾーン対象ルールでは `parameter.zones`、オブジェクトストレージ対象ルールでは `parameter.sites` の値を記録してください。

## Step 4: 評価ルールを更新して元に戻す

`--service-principal-id` は評価対象リソースへのアクセス権限を付与するサービスプリンシパル ID です。省略した場合は、有効化時に設定されたサービスプリンシパルが使われます。`--targets` は `parameter` に `zones` または `sites` を持つルールでのみ指定でき、反復可能なオプション (`--targets is1b --targets tk1b`) で指定します。省略した場合は空配列 (全ゾーン・全サイト) が送信されます。

```console
$ skr security-control-api evaluation-rule update "$RULE_ID" --is-enabled=true --targets is1b
```

```json
{
  "rule": {
    "evaluationRuleId": "server-no-public-ip",
    "parameter": {
      "zones": [
        "is1b"
      ]
    }
  },
  "iamRolesRequired": [],
  "isEnabled": true
}
```

評価ルールの設定は自動アクションの評価に反映されるため、確認後は Step 2 で記録した値に戻します。`--is-enabled` は必須です。`parameter` を持たないルール (`addon-threat-detections`、`addon-vulnerability-detections`) では `--service-principal-id` を指定できません。

```console
$ skr security-control-api evaluation-rule update "$RULE_ID" --is-enabled=true
```

複雑な設定を 1 度に指定する場合は、`--request` で JSON を指定できます。`--request` と個別フラグは併用できません。JSON はファイル (`@path.json`) でも指定できます。

```console
$ skr security-control-api evaluation-rule update "$RULE_ID" --request '{"rule":{"evaluationRuleId":"server-no-public-ip","parameter":{"zones":[]}},"isEnabled":true}'
```

## Step 5: 自動アクションを作成し、確認してから削除する

自動アクションは `automatedActionLimit` 内で登録します。実行先として、シンプル通知の通知先グループかワークフローのどちらか一方を指定します。ここではシンプル通知の通知先グループを指定します。`NOTIFICATION-GROUP-ID` には既存の通知先グループ ID を指定してください。

```console
$ skr simple-notification-api group list --query 'map({ID,Name})'
```

```json
[
  {
    "ID": "112700056789",
    "Name": "NOTIFICATION-GROUP-NAME"
  }
]
```

通知先グループ ID をシェル変数に取り込みます。名前で絞り込む場合は選択条件を明記してください。

```console
$ GROUP_ID=$(skr simple-notification-api group list --query '.[] | select(.Name == "NOTIFICATION-GROUP-NAME") | .ID')
$ echo "$GROUP_ID"
```

自動アクションを作成します。`--name`、`--execution-condition`、`--is-enabled`、`--service-principal-id` と、実行先として `--notification-group-id` か `--workflow-id` のどちらか一方が必須です。`--execution-condition` は評価結果を判定する CEL 形式の条件式です。

```console
$ ACTION_NAME="security-control-tutorial-action"
$ skr security-control-api automated-action create --name "$ACTION_NAME" --execution-condition 'event.evaluationResult.status == "REJECTED"' --is-enabled --service-principal-id 'SERVICE-PRINCIPAL-ID' --notification-group-id "$GROUP_ID"
```

```json
{
  "automatedActionId": "ACTION-ID",
  "name": "security-control-tutorial-action",
  "action": {
    "actionType": "simpleNotification",
    "actionParameter": {
      "servicePrincipalId": "SERVICE-PRINCIPAL-ID",
      "notificationGroupId": "112700056789"
    }
  },
  "executionCondition": "event.evaluationResult.status == \"REJECTED\"",
  "isEnabled": true,
  "createdAt": "CREATED-AT"
}
```

作成コマンドがエラーになり `automatedActionId` を取得できなかった場合は、名前だけを手掛かりに削除しないでください。`automated-action list` で対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

作成したアクションを一覧と詳細で確認します。

```console
$ ACTION_ID=$(skr security-control-api automated-action list --query ".items[] | select(.name == \"$ACTION_NAME\") | .automatedActionId")
$ echo "$ACTION_ID"
$ skr security-control-api automated-action read "$ACTION_ID"
```

確認後、アクションを無効化してから削除します。`update` は `name`、`action`、`executionCondition`、`isEnabled` を自動アクション全体として置き換えます。維持したい項目は `read` の結果から再指定してください。

```console
$ skr security-control-api automated-action update "$ACTION_ID" --name "$ACTION_NAME" --execution-condition 'event.evaluationResult.status == "REJECTED"' --is-enabled=false --service-principal-id 'SERVICE-PRINCIPAL-ID' --notification-group-id "$GROUP_ID"
$ skr security-control-api automated-action delete "$ACTION_ID"
```

`--args` はワークフロー実行時に渡す引数の JSON 文字列で、`--workflow-id` と併用する場合だけ指定できます。複数行の引数や秘密情報を含む引数は、`--request` JSON を `@path.json` で指定してください。

## Step 6: 後片付け

Step 2 で有効化した場合は、有効化状態を元に戻します。既に有効だったプロジェクトでは、有効化状態を変更しません。

```console
$ skr security-control-api activation update --service-principal-id 'SERVICE-PRINCIPAL-ID' --is-active=false
```

Step 4 で評価ルールを変更した場合は、Step 2 で記録した `isEnabled` と `parameter` の値に戻してください。

作成したローカルの JSON ファイルがあれば削除します。評価結果は[イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)で確認できます。
