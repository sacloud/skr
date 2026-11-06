# SecurityControl API コマンド設計

## 対象範囲

`security-control-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` のセキュリティコントロール (Security Control) を操作します。公開する操作は次のとおりです。

| グループ | 実装操作 |
| --- | --- |
| `activation` | `create`、`read`、`update` |
| `evaluation-rule` | `list`、`read`、`update` |
| `automated-action` | `list`、`read`、`create`、`update`、`delete` |

セキュリティコントロールは、プロジェクトのリソース設定を推奨構成と照合して評価し、評価結果をイベントログに記録するサービスです。`activation` はプロジェクトでの有効化状態を、`evaluation-rule` は個別の評価ルールの有効状態と評価対象を、`automated-action` は評価結果をきっかけに通知やワークフローを実行する自動アクションを扱います。評価結果の確認は[イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)でも行えます。

API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/tk1b/api/securitycontrol/1.0/` で、ゾーン引数は持たず SDK の既定値を使います。エンドポイントは `SAKURA_ENDPOINTS_SECURITY_CONTROL` 環境変数で変更できます。

## パッケージと生成

操作は `api/commands/` 配下の 3 つの設定ファイルで明示し、`make generate-api` から `internal/securitycontrolapi/` に生成します。設定ファイルは `security-control-activation.json`、`security-control-evaluation-rule.json`、`security-control-automated-action.json` です。生成するのは `activation` の全操作と、`evaluation-rule` の `list`、`read`、`automated-action` の `list`、`read`、`delete` です。API interface、factory、Runtime も生成対象です。`internal/cli/` の配線は `make generate-wiring` から生成します。

次の操作は手書きで `internal/securitycontrolapi/commands.go` に実装します。

- `evaluation-rule update`: `--service-principal-id` と `--targets` は API リクエストのフィールドに直接対応せず、SDK の入力補完 (`SetupEvaluationRuleInput`) が組み立てる補助入力のため。個別フラグ経路では SDK の入力補完を呼びます。`--request` JSON 経路では union をそのままデコードします。
- `automated-action create` / `update`: アクションの実行先 (`action.actionType`) を `--notification-group-id` か `--workflow-id` のどちらか一方から一意に組み立てるため。実行先ごとのパラメータと相互排他を CLI で検証します。

手書き操作でも API interface、factory、Runtime、`Commands` 型のフィールドは生成コードと共有します。型名全体の衝突は `internal/apigen` の設定検証で拒否します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/securitycontrolapi/` の単体テストで、CLI 登録とヘルプは `internal/cli/` のテストで確認します。

## 入力

- `activation create` は `--service-principal-id` (必須) を受け付けます。評価対象リソースへのアクセス権限を付与するサービスプリンシパル ID を指定します。
- `activation update` は `--service-principal-id` (必須) と `--is-active` (必須) を受け付けます。API の更新リクエストは両方のフィールドを送信するため、両方の指定を必須にします。
- `evaluation-rule update` は `--request` JSON か個別フラグ (`--is-enabled` 必須、`--service-principal-id` と `--targets` 任意) を受け付けます。両経路は併用できません。
  - ルール ID は SDK が公開する 14 種類の `evaluationRuleId` であることを検証します。SDK の入力補完は未知のルール ID で panic するため、CLI で検証してから API を呼び出します。
  - `--service-principal-id` はパラメータを持たないルール (`addon-threat-detections`、`addon-vulnerability-detections`) では拒否します。SDK の入力補完は対応しない値を黙って無視するため、CLI で明示的なエラーにします。
  - `--targets` はパラメータに `zones` または `sites` を持つ 7 ルールでのみ受け付けます。ゾーン対象のルールではゾーン、`objectstorage-bucket-encryption-enabled` ではオブジェクトストレージのサイトを指定します。反復可能なオプション (`--targets is1b --targets tk1b`) で、省略時は空配列 (全ゾーン・全サイト) を送ります。`--targets=` のような空配列の明示指定も、Kong が非 nil のスライスを設定するため指定とみなし、`--request` との併用は排他エラーにします。
  - `--service-principal-id` を省略した場合、SDK の入力補完は空文字列を設定します。API では `servicePrincipalId` を省略すると、有効化時に設定したサービスプリンシパルが使われるため、CLI は空文字列を省略状態に戻して送信します。
  - `--is-enabled` は必須です。ルールの有効状態は更新リクエストで常に送信されます。
- `automated-action create` と `update` は `--request` JSON か個別フラグを受け付けます。両経路は併用できません。
  - 個別フラグでは `--name`、`--execution-condition`、`--is-enabled`、`--service-principal-id` が必須で、実行先として `--notification-group-id` か `--workflow-id` のどちらか一方が必須です。
  - `--revision-id`、`--revision-alias`、`--workflow-name`、`--args` は `--workflow-id` と併用する場合だけ受け付けます。`--notification-group-id` と組み合わせた場合はエラーにします。
  - `--args` はワークフローの実行時に渡す引数の JSON です。引数に秘密情報を含められるため、`@path` で保護したファイルか `-` (標準入力) のみを受け付け、インラインの値は拒否します。`--request` JSON も同様に `@path.json` か `-` のみを受け付けます。
  - `update` は `name`、`action`、`executionCondition`、`isEnabled` を自動アクション全体として置き換えます。維持したい項目は `read` の結果から再指定してください。
- `evaluation-rule list` と `automated-action list` は `--page-size`、`--next`、`--prev` を受け付けます。すべて任意で、条件なしの `list` は空のパラメータで先頭ページを取得します。`--next` と `--prev` には前回の出力の `nextPageToken` と `prevPageToken` を指定します。

## 出力

応答フィールドは camelCase (`servicePrincipalId`、`isActive`、`automatedActionLimit`、`evaluationRuleId`、`automatedActionId` など) です。全操作の出力は JSON で、`--query` による加工に対応します。`list` は `items` と `nextPageToken`、`prevPageToken`、`totalSize` を返します。

## テスト

sakumock v0.13.0 にはセキュリティコントロールのモックサーバーがないため、CLI と SDK の統合テストは `httptest.NewServer` によるエンドポイント差し替えで実装します (`internal/cli/securitycontrol_test.go`)。差し替えには `SAKURA_ENDPOINTS_SECURITY_CONTROL` 環境変数を使います。手書き処理の単体テストは `internal/securitycontrolapi/commands_test.go` で、SDK interface の差し替えstubを使います。テストでは、作成系 API が 201 を返すこと、応答の必須フィールド (`iamRolesRequired`、`createdAt` など) を含めることを確認します。

## ライブ E2E

2026-11-06 にライブ E2E を実施しました。シナリオは `test/e2e/security-control-api/` の testing.T ベースのテストで、共有 helper がビルド済みの skr を実行し、CLI 呼び出しごとに説明を付けて記録します。

- `scenario_activation_test.go`: 有効化状態の読み取り、読み取った値をそのまま送り返す同一値の往復での update、アクティベート済みプロジェクトでの create が 409 で拒否されることと拒否後に状態が変わらないことを確認します。`create` の正常系は未アクティベートのプロジェクトが必要なため未検証で、追跡 Issue #200 で管理します。
- `scenario_evaluation_rule_test.go`: 一覧で SDK が公開する全評価ルールの包含を確認し、`server-no-public-ip` の読み取りが一覧と一致することを確認します。変更前に対象ルールが無効で parameter を持たないこと、有効な自動アクションが存在しないことを確認してから、ルールを `--is-enabled` と `--targets is1b` で一時的に有効化して update と反映を確認します。後片付けでは現在値を再照合し、元の状態なら何もせず、今回設定した値 (有効状態・parameter のサービスプリンシパル ID・ゾーン) と一致する場合だけ `--request` JSON により元の状態 (無効・parameter なし) に戻します。不一致・不正応答では変更 API を呼ばずにテストを失敗させます。
- `scenario_automated_action_test.go`: 一覧を取得した後、無効状態の自動アクションを作成して読み取り・一覧を確認し、update でも無効状態を維持して応答を検証します。削除の直前に読み取りで ID・名称・指定内容を再照合してから削除し、不在確認まで行います。実行パラメータの通知先グループは環境変数 `SKR_E2E_SECURITY_CONTROL_NOTIFICATION_GROUP_ID` で指定した既存グループを参照のみで使い、未設定のときはスキップします。後片付けで作成したアクションを ID・名称・指定内容の照合後に削除し、一覧に残っていないことを確認します。

`baseline.txt` では `activation create` を `deferred #200` とし、他の 10 末端コマンドを `live` に更新しました。評価結果はイベントログへの記録のみで、課金対象リソースは作成しません。実行後、有効化状態・評価ルール・自動アクション一覧は実行前の状態に戻ることを確認しました。PR レビュー指摘を受けて追加した parameter なしの事前確認、有効な自動アクションの不在確認、update での無効維持、削除前再照合つきの復元を含めて 2026-11-06 に再実行し、3 シナリオとも成功しました。

## 参考資料

- [イベントログのセキュリティコントロール画面](https://manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1` `api/security-control` ([README](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/security-control))
