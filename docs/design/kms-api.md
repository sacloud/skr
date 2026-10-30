# KMS API コマンド設計

## 対象範囲

`kms-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` の KMS (Key Management Service) を操作します。公開する操作は `key` リソースの次のとおりです。

| リソース | 実装操作 |
| --- | --- |
| `key` | `list`、`read`、`create`、`update`、`delete`、`rotate`、`change-status`、`schedule-destruction`、`encrypt`、`decrypt` |

KMS は FIPS 140-3 level 3 認証の HSM と連携し、対称鍵 AES-256 bit の暗号化・復号を提供します。キーは取り出しやバックアップができず、プロジェクトあたり最大 100 個まで作成できます。削除は猶予期間 7〜90 日の削除予約制で、削除予約中のキーは状態変更で取り消せます。ローテーションは有効 (active) なキーだけが対象で、BYOK モードでインポートしたキーはローテーションできません。レガシーキー (`cloud/kms/key/legacy`) は課金対象外で、新規作成はできません（[KMS 基本情報](https://manual.sakura.ad.jp/cloud/appliance/kms/index.html)）。

API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/{ゾーン名}/api/cloud/1.1/kms` で、SDK の既定値は tk1a ゾーンです。コマンドはゾーン引数を持たず、SDK の既定値を使います。エンドポイントは `SAKURA_ENDPOINTS_KMS` 環境変数で変更できます（[KMS API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/kms-key)）。

## パッケージと生成

操作の大半は `api/commands/kms-key.json` で明示し、`make generate-api` から `internal/kmsapi/kms_key_api_generated.go` に生成します。生成するのは `list`、`read`、`update`、`delete`、`rotate`、`schedule-destruction` のコマンドと、API interface、factory、Runtime です。`internal/cli/` の配線は `make generate-wiring` から生成し、設定との一致を `internal/cli/wiring_test.go` で確認します。

次の操作は手書きで `internal/kmsapi/commands.go` に実装します。

- `create`: PlainKey (BYOK の秘密鍵) を引数やインライン JSON で受け取らないため。秘密鍵は `--plain-key-file` (`@` ではなくパスまたは `-` で標準入力) から読み込みます。
- `change-status`: 状態の選択値 (`active`、`restricted`、`suspended`) を CLI 側で検証するため。
- `encrypt`: 平文を引数に含めないため。平文は `--plain-file` (パスまたは `-`) から読み込みます。
- `decrypt`: 暗号文を引数に含めず、復号結果をバイト列のまま標準出力へ出力するため。

手書き操作でも API interface、factory、Runtime、`Commands` 型のフィールドは生成コードと共有します。型名全体の衝突は `internal/apigen` の設定検証で拒否します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/kmsapi/` の単体テストで、CLI 登録とヘルプは `internal/cli/` のテストで確認します。

## 入力と秘密情報

- キーの作成は `--name` (必須)、`--description`、`--plain-key-file` の個別フラグか、`--request` JSON を `@path.json` または `-`（標準入力）で受け付けます。両経路は併用できません。
- PlainKey を含む可能性があるため、`create --request` はインライン JSON を受け付けません。`@path.json` または `-` だけを許可し、インライン指定はエラーにします。
- `--plain-key-file` は 256 bit の対称鍵を base64 エンコードした内容のファイルを想定します。前後の空白は正規化してから API へ送ります。
- キーの更新は `--name` (必須) と `--description` の個別フラグか、`--request` JSON (`@path.json` または `-`) を受け付けます。API の更新リクエストは `Name` が必須で、`Description` は指定した場合だけ本文に含まれ、`Tags` は常に本文に含まれます。個別フラグのみでの更新ではタグをクリアしないよう、`Tags` を指定しない個別フラグ経路は SDK の入力モデルで null (タグのクリア) として送信します。タグを設定・維持する場合は `--request` JSON で `Tags` を指定します。
- `encrypt` は平文を `--plain-file` (パスまたは `-`) から読み込み、API 向けに base64 エンコードして送ります。アルゴリズムは `--algo` で `aes-256-gcm` (既定)、`aes-256-cbc`、`aes-256-kw` から選択し、CLI 側で検証します。
- `decrypt` は暗号文を `--cipher-file` (パスまたは `-`) から読み込みます。`encrypt` の JSON 出力をリダイレクトしたファイル (引用符付き) もそのまま使えます。復号結果はバイト列のまま標準出力へ出力し、`--query` は適用しません。

API キーを使う場合は、対象キーへの操作権限を付与します（[KMS API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/kms-key)）。CLI の認証情報は SDK プロファイルまたは既存の SDK 認証機構から読み込みます。

秘密情報の入力解析で発生したエラーは、入力値を含まないメッセージで表示します。

## 出力

KMS API の応答フィールドは PascalCase (`ID`、`Name`、`Status`、`KeyOrigin` など) です。`list`、`read`、`create`、`update`、`rotate` の出力は JSON で、`--query` による加工に対応します。`change-status`、`encrypt` は文字列、`schedule-destruction` はステータスと削除予定日時の JSON です。`decrypt` のみ復号結果をバイト列のまま出力します。

## ライブ E2E と後片付け

`test/e2e/kms-api/` はビルド済み CLI を呼び出す testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。シナリオは生成モードのキー作成から、参照、一覧、更新、ローテーション、暗号化と復号、状態変更、削除予約とその取り消し、削除までを検証します。BYOK モードのキーも作成し、`KeyOrigin` が `imported` になることを確認します。作成するのは KMS キー 2 件のみで、テスト関数内の `defer` で削除と不在確認まで行います。

```console
$ SKR_E2E=1 go test ./test/e2e/kms-api/ -run TestScenarioKmsKey
```

実行には `SKR_E2E` の設定と `skr-e2e` プロファイルが必要です。シナリオは作成前に同名キーが存在しないことを確認し、削除時は保持した ID で削除してから一覧で不在を確認します。

sakumock を使った CLI と SDK の統合テストは `internal/cli/kms_test.go` で、手書き処理の単体テストは `internal/kmsapi/commands_test.go` で確認します。

## 参考資料

- [KMS 基本情報](https://manual.sakura.ad.jp/cloud/appliance/kms/index.html)
- [KMS API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/kms-key)
- [KMS / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/kms)
