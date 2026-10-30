# SecretManager API コマンド設計

## 対象範囲

`secretmanager-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` のシークレットマネージャー (Secret Manager) を操作します。公開する操作は次のとおりです。

| リソース | 実装操作 |
| --- | --- |
| `vault` | `list`、`read`、`create`、`update`、`delete` |
| `secret` | `list`、`create`、`delete`、`unveil` |

シークレットマネージャーは KMS キーで暗号化したシークレットを契約単位で保管し、シークレットは最新を含む複数世代を保持します。シークレットの作成は upsert で、同名シークレットへの作成は新しいバージョンの登録として扱われます。Vault 単位で課金が行われ、Vault が作成されてから削除されるまでの間課金されます。保管できるシークレット数、保持できる世代数、API のレート制限などの上限は[シークレットマネージャー基本情報](https://manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html)を参照してください。

シークレットの値の取得は `secret unveil` で行います。SDK のシークレット取得 API は `unveil` のみを提供し、値を返す read 操作はありません。バージョンを指定しない場合は最新バージョンを取得します。

API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/{ゾーン名}/api/cloud/1.1/secretmanager` で、全ゾーンで利用できます。コマンドはゾーン引数を持たず、SDK の既定値を使います。エンドポイントは `SAKURA_ENDPOINTS_SECRETMANAGER` 環境変数で変更できます（[SecretManager API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/secretmanager-vault)）。

## パッケージと生成

操作は `api/commands/secretmanager-vault.json` と `api/commands/secretmanager-secret.json` で明示し、`make generate-api` から `internal/secretmanagerapi/` に生成します。生成するのは `vault` の全操作と `secret list`、`secret delete` のコマンドと、API interface、factory、Runtime です。`internal/cli/` の配線は `make generate-wiring` から生成し、設定との一致を `internal/cli/wiring_test.go` で確認します。

次の操作は手書きで `internal/secretmanagerapi/commands.go` に実装します。

- `secret create`: シークレットの値を引数やインライン JSON で受け取らないため。値は `--value-file` (パスまたは `-` で標準入力) から読み込みます。
- `secret unveil`: 取得した値を JSON にせずそのまま標準出力へ出力するため。`--query` は適用しません。

SDK のシークレット操作は Vault スコープのクライアント (`NewSecretOp(client, vaultId)`) で提供されます。CLI では Vault ID を位置引数で受け付けるため、`secretOp` アダプタがメソッド引数の Vault ID から SDK の Vault スコープクライアントを生成して委譲します。

手書き操作でも API interface、factory、Runtime、`Commands` 型のフィールドは生成コードと共有します。型名全体の衝突は `internal/apigen` の設定検証で拒否します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/secretmanagerapi/` の単体テストで、CLI 登録とヘルプは `internal/cli/` のテストで確認します。

## 入力と秘密情報

- Vault の作成は `--name` (必須)、`--description`、`--kms-key-id` (必須) の個別フラグか、`--request` JSON (`@path.json` または `-`) を受け付けます。両経路は併用できません。`--kms-key-id` にはシークレットの暗号化に使う既存の KMS キー ID を指定します。Tags は `--request` JSON でのみ指定できます。
- Vault の更新は `--name` (必須) と `--description` の個別フラグか、`--request` JSON (`@path.json` または `-`) を受け付けます。API の更新リクエストは `Name` が必須で、`Description` は指定した場合だけ本文に含まれ、`Tags` は常に本文に含まれます。個別フラグのみでの更新ではタグをクリアしないよう、`Tags` を指定しない個別フラグ経路は SDK の入力モデルで null (タグのクリア) として送信します。タグを設定・維持する場合は `--request` JSON で `Tags` を指定します。
- `secret create` はシークレット名を `--name` (必須) で、値を `--value-file` (パスまたは `-` で標準入力) から受け取ります。値をコマンド引数に含めることはできません。値の前後の空白は正規化してから API へ送ります。同名シークレットが存在する場合は新しいバージョンとして登録されます (upsert)。
- `secret unveil` は Vault ID とシークレット名を位置引数で受け取り、`--secret-version` でバージョンを指定できます。省略時は最新バージョンを取得します。取得した値はそのまま標準出力へ出力し、`--query` は適用しません。出力に秘密情報が含まれるため、リダイレクト先とシェル履歴の取り扱いに注意してください。

シークレットの値の入力解析で発生したエラーは、入力値を含まないメッセージで表示します。

## 出力

SecretManager API の応答フィールドは PascalCase (`ID`、`Name`、`KmsKeyID`、`LatestVersion` など) です。`vault` の全操作と `secret list`、`secret create` の出力は JSON で、`--query` による加工に対応します。`secret list` はシークレットの値を含まないメタデータ (`Name`、`LatestVersion`) を返します。`secret unveil` のみ値をそのまま標準出力へ出力します。

## ライブ E2E と後片付け

`test/e2e/secretmanager-api/` はビルド済み CLI を呼び出す testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。シナリオはテスト専用の KMS キー作成から、Vault 作成、参照、一覧、更新、シークレットの作成 (2 回で upsert とバージョン増加を確認)、unveil、シークレット削除、Vault 削除までを検証します。`secret unveil` の出力にはシークレットの値が含まれるため、実行記録からは非表示にします。

```console
$ SKR_E2E=1 go test ./test/e2e/secretmanager-api/ -run TestScenarioSecretManager
```

実行には `SKR_E2E` の設定と `skr-e2e` プロファイルが必要です。シナリオは作成前に同名 Vault が存在しないことを確認し、Vault 作成に必要な KMS キーはテスト専用キーとして生成モードで作成して、後片付けで Vault、シークレット、KMS キーの順に削除と不在確認まで行います。既存の KMS キーは変更・削除しません。2026-10-30 にライブ実行し、Vault 作成から削除後の不在までの全ステップと、シークレットの upsert によるバージョン増加、unveil で取得した値 (最新とバージョン 1) を確認しました。ライブ実行では実 API が存在しないシークレットの削除に 422 を返すことを確認したため、後片付けは削除前に不在を確認し、不在なら変更せず終了します。

sakumock を使った CLI と SDK の統合テストは `internal/cli/secretmanager_test.go` で、手書き処理の単体テストは `internal/secretmanagerapi/commands_test.go` で確認します。

## 参考資料

- [シークレットマネージャー基本情報](https://manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html)
- [SecretManager API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/secretmanager-vault)
- [SecretManager / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/secretmanager)
