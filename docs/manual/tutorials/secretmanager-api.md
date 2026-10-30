# SecretManager API: シークレット保管庫を作成し、シークレットを保管・取得する

さくらのクラウド シークレットマネージャー (Secret Manager) は、KMS キーで暗号化したシークレットを契約単位で保管するサービスです。シークレットは Vault (シークレット保管庫) 単位で管理され、最新を含む 50 世代まで保持します。シークレットの作成は upsert で、同名シークレットへの作成は新しいバージョンの登録として扱われます（[シークレットマネージャー基本情報](https://manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html)）。

> [!WARNING]
> この手順では、KMS キー 1 件とシークレット保管庫 (Vault) 1 件を作成し、シークレットの保管と取得を確認してから削除します。Vault は作成から削除まで課金対象です。実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。契約あたりのシークレット上限は 100 個、API の利用は 1 分あたり 100 リクエストまでです。

## 前提条件

- `skr secretmanager-api` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。
- シークレットの暗号化に使う既存の KMS キーがあること。作成方法は [KMS API チュートリアル](kms-api.md) を参照してください。
- 他の Vault と重複しない `VAULT-NAME`、他のシークレットと重複しない `SECRET-NAME` を決めること。
- シークレットの値を保存するローカルファイルを用意すること。値に秘密情報を含める場合は、ファイルの権限を `0600` にして、確認後すぐに削除してください。値をコマンド引数に含めないでください。
- API エンドポイントは `https://secure.sakura.ad.jp/cloud/zone/{ゾーン名}/api/cloud/1.1/secretmanager` で、全ゾーンで利用できます。エンドポイントは `SAKURA_ENDPOINTS_SECRETMANAGER` 環境変数で変更できます。

## 作成するリソースと順序

1. シークレット保管庫 (Vault) を 1 件作成します。
2. 作成した Vault を参照し、一覧に含まれることを確認します。
3. Vault の名前と説明を更新します。
4. シークレットを作成し、同名シークレットの再作成でバージョンが増えることを確認します (upsert)。
5. シークレットを取得し、値が一致することを確認します。
6. シークレットを削除します。
7. ID と名前を照合したうえで、Vault を削除します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr secretmanager-api --help
$ skr secretmanager-api vault create --help
$ skr secretmanager-api vault read --help
$ skr secretmanager-api vault list --help
$ skr secretmanager-api vault update --help
$ skr secretmanager-api vault delete --help
$ skr secretmanager-api secret list --help
$ skr secretmanager-api secret create --help
$ skr secretmanager-api secret delete --help
$ skr secretmanager-api secret unveil --help
```

認証は選択中の SDK プロファイルを使います。`VAULT-NAME` と `SECRET-NAME` は、以降のコマンドと JSON で同じ一意な名前に置き換えてください。`KMS-KEY-ID` はシークレットの暗号化に使う既存の KMS キー ID に置き換えてください。

## Step 2: Vault を作成する

新しい Vault 名に一致する Vault がないことも確認します。名前が未使用の場合の出力は `[]` です。これは期待する出力形式であり、ライブ環境で取得した結果ではありません。実際に別の要素が表示された場合は、その Vault を変更・削除せず、異なる名前を選んでください。

```console
$ skr secretmanager-api vault list --query 'map(select(.Name == "VAULT-NAME") | {ID,Name})'
```

```json
[]
```

Vault を作成します。`--kms-key-id` にはシークレットの暗号化に使う既存の KMS キー ID を指定します。`--description` は省略できます。Tags は `--request` JSON でのみ指定できます。

```console
$ skr secretmanager-api vault create --name 'VAULT-NAME' --description 'SecretManager tutorial' --kms-key-id 'KMS-KEY-ID'
```

```json
{
  "ID": "VAULT-ID",
  "CreatedAt": "CREATED-AT",
  "ModifiedAt": "MODIFIED-AT",
  "Name": "VAULT-NAME",
  "Description": "SecretManager tutorial",
  "KmsKeyID": "KMS-KEY-ID",
  "Tags": []
}
```

作成結果の `ID` を控え、以降の `VAULT-ID` に指定してください。作成コマンドがエラーになり ID を取得できなかった場合は、名前だけを手掛かりに削除しないでください。作成された可能性がある場合は、`secretmanager-api vault list` やコントロールパネルで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

## Step 3: 作成した Vault を参照する

```console
$ skr secretmanager-api vault read VAULT-ID --query '{ID,Name,KmsKeyID}'
```

```json
{
  "ID": "VAULT-ID",
  "Name": "VAULT-NAME",
  "KmsKeyID": "KMS-KEY-ID"
}
```

一覧でも同じ内容が返ることを確認します。

```console
$ skr secretmanager-api vault list --query 'map(select(.ID == "VAULT-ID") | {ID,Name})'
```

名前と説明の更新は `vault update` で行います。API の更新リクエストは `Name` が必須です。タグを設定・維持する場合は `--request` JSON で `Tags` を指定します。

```console
$ skr secretmanager-api vault update VAULT-ID --name 'VAULT-NAME' --description 'updated description'
```

## Step 4: シークレットを作成する

シークレットの値をローカルファイルに保存します。値は `--value-file` でファイルから読み込み、値の前後の空白は正規化されて送信されます。`-` を指定すると標準入力から読み込みます。値をコマンド引数に含めることはできません。

```console
$ printf 'db-password-value' > secret-value.txt
$ skr secretmanager-api secret create VAULT-ID --name 'SECRET-NAME' --value-file secret-value.txt
```

```json
{
  "Name": "SECRET-NAME",
  "LatestVersion": 1
}
```

同名シークレットに再作成すると、新しいバージョンとして登録されます (upsert)。シークレットは最新を含む 50 世代まで保持されます。

```console
$ printf 'db-password-value-2' > secret-value.txt
$ skr secretmanager-api secret create VAULT-ID --name 'SECRET-NAME' --value-file secret-value.txt
```

```json
{
  "Name": "SECRET-NAME",
  "LatestVersion": 2
}
```

Vault 内のシークレット一覧は `secret list` で確認できます。一覧はシークレットの値を含まないメタデータ (`Name`、`LatestVersion`) を返します。

```console
$ skr secretmanager-api secret list VAULT-ID
```

## Step 5: シークレットを取得する

`secret unveil` でシークレットの値を取得します。値はそのまま標準出力へ出力され、`--query` は適用されません。出力に秘密情報が含まれるため、リダイレクト先とシェル履歴の取り扱いに注意してください。確認後、ローカルの値ファイルは削除してください。

```console
$ skr secretmanager-api secret unveil VAULT-ID 'SECRET-NAME'
db-password-value-2
```

`--secret-version` でバージョンを指定すると、過去のバージョンの値を取得できます。省略時は最新バージョンを取得します。

```console
$ skr secretmanager-api secret unveil VAULT-ID 'SECRET-NAME' --secret-version 1
db-password-value
```

## Step 6: シークレットを削除する

```console
$ skr secretmanager-api secret delete VAULT-ID 'SECRET-NAME'
$ skr secretmanager-api secret list VAULT-ID
```

削除後の一覧に対象シークレットが残っていないことを確認してください。

## Step 7: Vault を削除する

削除前に `vault read` で ID と名前がこの手順で作成した Vault と一致することを再確認します。一致しない場合や ID を確認できない場合は削除しません。

```console
$ skr secretmanager-api vault read VAULT-ID --query '{ID,Name}'
$ skr secretmanager-api vault delete VAULT-ID
$ skr secretmanager-api vault list --query 'map(select(.ID == "VAULT-ID") | {ID,Name})'
```

対象 ID が一覧に残っていない場合の出力は `[]` です。以下は E2E テストで確認している削除後の出力形式です。

```json
[]
```

Vault を削除すると、保管していたシークレットは取得できなくなります。連携リソースがある場合は、削除前に依存関係を確認してください（[シークレットマネージャー基本情報](https://manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html)）。

Vault とシークレットの操作一覧は `skr secretmanager-api vault --help` と `skr secretmanager-api secret --help` で確認できます。

## 参考資料

- [シークレットマネージャー基本情報](https://manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html)
- [SecretManager API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/secretmanager-vault)
- [SecretManager / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/secretmanager)
