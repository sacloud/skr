# ADR 0073: SecurityControl API コマンドの入力と生成の分割

- Status: Accepted
- Date: 2026-11-06
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## Context

セキュリティコントロール (Security Control) は、プロジェクトのリソース設定を推奨構成と照合して評価するサービスです。SDK は `activation`、`evaluation_rules`、`automated_actions` の 3 API を公開し、ゾーンは tk1b 固定のためゾーン引数を持ちません。コマンドグループは API エンドポイントに対応させます ([ADR 0069](0069-api-endpoint-command-groups.md))。

SDK の `SetupEvaluationRuleInput` は、ルール ID ごとに異なる union 型 (`parameter` に `servicePrincipalId` のみ、`zones` を持つ、`sites` を持つ) を、ルール ID と `servicePrincipalId`、`zones`/`sites` の補助入力から組み立てます。これらの補助入力は API リクエストのフィールドに直接対応せず、生成器が個別フラグをリクエストフィールドに割り当てる方式では表現できません。また SDK の入力補完は未知のルール ID で panic し、対応しない補助入力を黙って無視します。

自動アクションの `action` は `simpleNotification` と `workflows` の 2 種類の union で、実行先ごとに必須項目と追加項目が異なります。実行先を一意に組み立てられる単純なスカラー値の組合せで有効なリクエストを表現できるため、個別フラグ化の条件を満たします。

評価ルールと自動アクションの一覧 API はページングのパラメータをすべて任意とします。`--request` と個別フラグの併用可否を要求する既定の生成方式では、条件なしの `list` が実行できません。

sakumock v0.13.0 にはセキュリティコントロールのモックサーバーがないため、統合テストは別の手段が必要です。

## Decision

- `security-control-api` に 3 グループ 11 操作を追加し、`internal/securitycontrolapi/` で入力と SDK 呼び出しを扱います。CLI 登録と SDK factory の接続は `internal/cli/` が担当し、生成コードは `api/commands/security-control-*.json` から `make generate-api` と `make generate-wiring` で再生成します。
- `activation` の 3 操作は生成コードとします。`create` は `--service-principal-id` (必須)、`update` は `--service-principal-id` (必須) と `--is-active` (必須) を受け付けます。API の更新リクエストは両方のフィールドを送信するため、片方だけの指定はエラーにします。
- `evaluation-rule` の `list` と `read` は生成コードとします。`list` は `--page-size`、`--next`、`--prev` をすべて任意とし、条件なしの実行を許可します。
- `evaluation-rule update` は手書きとします。個別フラグ経路では SDK の `SetupEvaluationRuleInput` を呼び、`--request` JSON 経路では union をそのままデコードします。ルール ID は SDK が公開する 14 種類の `evaluationRuleId` に CLI で検証します。パラメータを持たないルールでの `--service-principal-id`、パラメータに `zones`/`sites` を持たないルールでの `--targets` はエラーにします。`--service-principal-id` を省略した場合、SDK の入力補完が設定する空文字列を省略状態に戻して送信します。
- `automated-action` の `list`、`read`、`delete` は生成コードとします。`list` は評価ルールと同様に条件なしの実行を許可します。
- `automated-action` の `create` と `update` は手書きとします。個別フラグでは `--name`、`--execution-condition`、`--is-enabled`、`--service-principal-id` と、実行先として `--notification-group-id` か `--workflow-id` のどちらか一方を必須にします。`--revision-id`、`--revision-alias`、`--workflow-name`、`--args` は `--workflow-id` と併用する場合だけ受け付けます。実行先の両指定と `--workflow-id` 用フラグの `--notification-group-id` との併用はエラーにします。
- 手書き操作でも API interface、factory、Runtime、`Commands` 型のフィールドは生成コードと共有し、型名全体の衝突は設定検証で拒否します。
- 統合テストは sakumock の代わりに `httptest.NewServer` と `SAKURA_ENDPOINTS_SECURITY_CONTROL` 環境変数によるエンドポイント差し替えで実装します。作成系 API が 201 を返すこと、応答の必須フィールド (`iamRolesRequired`、`createdAt` など) を含めることをテストで固定します。
- ライブ E2E は未実施です。`activation` と `evaluation-rule` の更新はプロジェクトの状態を変更するため、既存状態の取得と後片付けでの復元を含むシナリオを提示して承認を得てから実装します。

## Consequences

- よく使う操作は個別フラグで、評価ルールの union や自動アクションの実行先は意味を明確に表せるフラグの組合せで指定できます。`--request` JSON (`@path.json` または `-`) の経路も維持し、両経路は相互排他です。
- SDK の入力補完が黙って無視する入力を CLI で明示的にエラーにするため、入力間の制約は CLI でも検証されます。検証のメンテナンスが必要です。
- `evaluation-rule update` と `automated-action` の `create`/`update` は手書きのため、SDK のリクエスト型の変更時に個別に追従が必要です。
- sakumock を使わないため、セキュリティコントロールの統合テストは他ドメインのテストと実装方式が異なります。
- ライブ E2E が未実施の間、`activation` と `evaluation-rule` の実際の応答と状態遷移は未確認です。シナリオの承認後に実装します。

## Alternatives considered

- `evaluation-rule update` を `--rule-id` と `--parameter` JSON で受ける案: ルール ID ごとの有効なパラメータを利用者が知っている必要があり、CLI での検証範囲が曖昧になります。SDK の入力補完を使う現案では、ルール ID の検証と対応しない入力の拒否を CLI で明示できます。
- 自動アクションの実行先を `--action-type` で選択させる案: 実行先は `--notification-group-id` か `--workflow-id` の指定だけで一意に決まるため、選択フラグは冗長になります。
- 一覧 API を `--request` 必須のままにする案: すべて任意のパラメータを持つ一覧で、条件なしの実行ができず利用体験を損ないます。
