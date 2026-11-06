# SecurityControl API: セキュリティコントロールを有効化し、評価ルールと自動アクションを操作する

さくらのクラウド セキュリティコントロール (Security Control) は、プロジェクトのリソース設定を推奨構成と照合して評価し、評価結果をイベントログに記録するサービスです。有効化には評価対象リソースへのアクセス権限を付与するサービスプリンシパルの指定が必要で、評価ルールごとに有効状態と評価対象を変更でき、評価結果をきっかけに通知やワークフローを実行する自動アクションを登録できます（[イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)）。

> [!WARNING]
> この手順では、プロジェクトのセキュリティコントロールの有効化状態と評価ルールの設定を変更し、自動アクションを 1 件作成してから削除します。変更前の状態を記録し、後片付けで元の状態に戻します。実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。

## 前提条件

- `skr` と `jq` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。
- 有効化と評価ルールの更新で指定するサービスプリンシパル ID (`SERVICE-PRINCIPAL-ID`) を決めること。評価対象リソースへのアクセス権限を付与する IAM のサービスプリンシパルを指定します。
- 自動アクションの通知先として、シンプル通知の通知先グループが既にあること。作成方法は `skr simple-notification-api group create --help` を参照してください。
- API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/tk1b/api/securitycontrol/1.0/` です。エンドポイントは `SAKURA_ENDPOINTS_SECURITY_CONTROL` 環境変数で変更できます。

## 変更する設定と順序

1. セキュリティコントロールの有効化状態を確認し、未有効化なら有効化します。
2. 評価ルールの一覧を確認し、1 件を参照して元の状態を保存します。
3. 評価ルールの 1 件を更新し、保存した元の状態に戻します。
4. 自動アクションを無効状態で作成し、一覧と詳細を確認して、更新・削除します。
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
$ skr security-control-api evaluation-rule list --query '.items | map(select(.isEnabled) | {id: .rule.evaluationRuleId})'
```

ルール ID をシェル変数に取り込み、1 件を参照します。`--query` の結果も JSON 出力のため文字列には引用符が付きます。`jq -er '.'` で引用符を取り除いて取り込みます。評価ルールの ID は固定のため、以降の手順では `server-no-public-ip` を使います。

```console
$ RULE_ID=$(skr security-control-api evaluation-rule list --query '.items[] | select(.rule.evaluationRuleId == "server-no-public-ip") | .rule.evaluationRuleId' | jq -er '.')
$ echo "$RULE_ID"
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

後片付けと復元のために、更新するルールの `rule` と `isEnabled` 全体をシェル変数に保存します。保存した JSON を `--request` で送り返すことで、ゾーン対象ルールの `parameter.zones`、オブジェクトストレージ対象ルールの `parameter.sites` も含めて元の状態へ戻せます。

```console
$ RULE_BACKUP=$(skr security-control-api evaluation-rule read "$RULE_ID" --query '{rule, isEnabled}')
$ echo "$RULE_BACKUP"
```

## Step 4: 評価ルールを更新して元に戻す

`--service-principal-id` は評価対象リソースへのアクセス権限を付与するサービスプリンシパル ID です。省略した場合は、有効化時に設定されたサービスプリンシパルが使われます。`--targets` は `parameter` に `zones` または `sites` を持つルールでのみ指定でき、反復可能なオプション (`--targets is1b --targets tk1b`) で指定します。省略した場合は空配列 (全ゾーン・全サイト) が送信されます。

評価ルールを有効化する前に、既存の有効な自動アクションを確認します。有効な自動アクションが存在する場合、このルールの評価結果が既存の実行条件に一致すると、Step 5 で作るアクションを無効にしていても、既存の通知やワークフローが実行されます。次のコマンドで `--next` によるページングで全ページを走査して有効な自動アクションを数えます。`ENABLED_COUNT` が 0 の場合だけ、後続のコマンドで評価ルールを有効化してください。0 以外の場合は、評価ルールを変更せずに手順を中止してください。

```console
$ ENABLED_COUNT=0
$ NEXT=""
$ while :; do
      if [ -n "$NEXT" ]; then
          PAGE=$(skr security-control-api automated-action list --next "$NEXT")
      else
          PAGE=$(skr security-control-api automated-action list)
      fi
      ENABLED_COUNT=$((ENABLED_COUNT + $(echo "$PAGE" | jq '[.items[] | select(.isEnabled)] | length')))
      NEXT=$(echo "$PAGE" | jq -er '.nextPageToken // empty') || break
  done
$ echo "$ENABLED_COUNT"
```

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

評価ルールの設定は自動アクションの評価に反映されるため、確認後は元の状態へ戻します。`--is-enabled` は必須です。`parameter` を持たないルール (`addon-threat-detections`、`addon-vulnerability-detections`) では `--service-principal-id` を指定できません。

復元の前に、現在値が今回の設定と一致することを再照合します。読み取りが失敗する場合や、`isEnabled` が `true` 以外、`parameter.zones` が今回指定した値と一致しない場合は、別の操作がルールを変更した可能性があるため、復元コマンドを実行しないでください。一致を確認したら、Step 3 で保存した JSON を `--request` で送り返して元の状態に戻します。

```console
$ skr security-control-api evaluation-rule read "$RULE_ID"
$ skr security-control-api evaluation-rule update "$RULE_ID" --request "$RULE_BACKUP"
```

保存した JSON を引用符付きで `--request` に渡すため、シェル変数は必ず二重引用符で囲ってください。

複雑な設定を 1 度に指定する場合は、`--request` で JSON を指定できます。`--request` と個別フラグは併用できません。JSON はファイル (`@path.json`) でも指定できます。

```console
$ skr security-control-api evaluation-rule update "$RULE_ID" --request '{"rule":{"evaluationRuleId":"server-no-public-ip","parameter":{"zones":[]}},"isEnabled":true}'
```

## Step 5: 自動アクションを作成し、確認してから削除する

自動アクションは `automatedActionLimit` 内で登録します。実行先として、シンプル通知の通知先グループかワークフローのどちらか一方を指定します。ここではシンプル通知の通知先グループを指定します。`NOTIFICATION-GROUP-ID` には既存の通知先グループ ID を指定してください。評価ルールが有効なプロジェクトでは、評価結果が実行条件と一致した場合、通知が送られます。この手順では自動アクションを無効状態のまま作成・操作するため、通知が発生することはありません。

```console
$ skr simple-notification-api group list --query '.CommonServiceItems | map({ID,Name})'
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
$ GROUP_ID=$(skr simple-notification-api group list --query '.CommonServiceItems[] | select(.Name == "NOTIFICATION-GROUP-NAME") | .ID' | jq -er '.')
$ echo "$GROUP_ID"
```

自動アクションを作成します。`--name`、`--execution-condition`、`--is-enabled`、`--service-principal-id` と、実行先として `--notification-group-id` か `--workflow-id` のどちらか一方が必須です。`--execution-condition` は評価結果を判定する CEL 形式の条件式です。作成応答の `automatedActionId` を取り込んで保持します。以降の操作はこの ID を対象にします。

```console
$ ACTION_NAME="security-control-tutorial-action"
$ ACTION_ID=$(skr security-control-api automated-action create --name "$ACTION_NAME" --execution-condition 'event.evaluationResult.status == "REJECTED"' --is-enabled=false --service-principal-id 'SERVICE-PRINCIPAL-ID' --notification-group-id "$GROUP_ID" --query '.automatedActionId' | jq -er '.')
$ echo "$ACTION_ID"
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
  "isEnabled": false,
  "createdAt": "CREATED-AT"
}
```

作成コマンドがエラーになり `automatedActionId` を取得できなかった場合は、名前だけを手掛かりに削除しないでください。`automated-action list` で対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

作成したアクションを読み取りと一覧で確認します。一覧では保持した ID で絞り込みます。

```console
$ skr security-control-api automated-action read "$ACTION_ID"
$ skr security-control-api automated-action list --query ".items[] | select(.automatedActionId == \"$ACTION_ID\") | {name, isEnabled}"
```

更新の例として、アクション全体を同じ内容で置き換えます。ここでも無効状態を維持します。`update` は `name`、`action`、`executionCondition`、`isEnabled` を自動アクション全体として置き換えます。維持したい項目は `read` の結果から再指定してください。

```console
$ skr security-control-api automated-action update "$ACTION_ID" --name "$ACTION_NAME" --execution-condition 'event.evaluationResult.status == "REJECTED"' --is-enabled=false --service-principal-id 'SERVICE-PRINCIPAL-ID' --notification-group-id "$GROUP_ID"
```

削除の直前に、読み取りで ID・名称・実行先・実行条件・有効状態を再照合します。出力が今回指定した内容と一致しない場合や、読み取りが失敗する場合は、別の操作がアクションを変更した可能性があるため、削除コマンドを実行しないでください。

```console
$ skr security-control-api automated-action read "$ACTION_ID"
$ skr security-control-api automated-action delete "$ACTION_ID"
```

`--args` はワークフロー実行時に渡す引数の JSON で、`--workflow-id` と併用する場合だけ指定できます。引数に秘密情報を含められるため、値を直接指定できない点に注意してください。`@path` で保護したファイルから、`-` で標準入力から読み込みます。`--request` JSON も同様に `@path.json` か `-` で指定してください。

## Step 6: 後片付け

Step 2 で `create` を実行した場合は、有効化状態を元に戻します。無効化の直前に `read` で現在値を確認し、`servicePrincipalId` が手順で指定した SERVICE-PRINCIPAL-ID と一致し、`isActive` が `true` のときだけ、次のコマンドで無効化します。不一致や読み取りが失敗する場合は、別の操作が有効化状態を変更した可能性があるため、操作を止めて現在の状態を確認してください。

```console
$ skr security-control-api activation read
$ skr security-control-api activation update --service-principal-id 'SERVICE-PRINCIPAL-ID' --is-active=false
```

Step 4 で評価ルールを変更した場合は、Step 3 で保存した `RULE_BACKUP` を `--request` で送り返して元の状態に戻してください (Step 4 の復元手順を参照)。

作成したローカルの JSON ファイルがあれば削除します。評価結果は[イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)で確認できます。
