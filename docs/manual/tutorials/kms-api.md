# KMS API: キーを作成し、暗号化と復号を行う

さくらのクラウド KMS (Key Management Service) は、FIPS 140-3 level 3 認証の HSM と連携した暗号鍵の管理サービスです。対称鍵 AES-256 bit の暗号化・復号を提供し、鍵素材は HSM の外に取り出せません。キーの削除は猶予期間 7〜90 日の削除予約制です（[KMS 基本情報](https://manual.sakura.ad.jp/cloud/appliance/kms/index.html)）。

> [!WARNING]
> この手順では、KMS キーを 1 件作成し、暗号化と復号、ローテーション、削除予約の取り消しを確認してから削除します。キーは作成から削除まで課金対象です (レガシーキーを除く)。実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。プロジェクトあたりのキー上限は 100 個です。

## 前提条件

- `skr kms-api` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。
- 対象キーへの操作権限が付与されていること（[KMS API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/kms-key)）。
- 他のキーと重複しない `KEY-NAME` と説明を決めること。
- 暗号化する平文を保存するローカルファイルを用意すること。平文に秘密情報を含める場合は、ファイルの権限を `0600` にして、確認後すぐに削除してください。
- API エンドポイントのゾーンは SDK の既定値 (tk1a) を使います。`SAKURA_ENDPOINTS_KMS` 環境変数で変更できます。

## 作成するリソースと順序

1. 生成モードの KMS キーを 1 件作成します。
2. 作成したキーを参照し、一覧に含まれることを確認します。
3. 平文を暗号化し、暗号文を復号して元の平文と一致することを確認します。
4. キーをローテーションします。
5. 削除を予約し、予約を取り消します。
6. ID と名前を照合したうえで、キーを削除します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr kms-api --help
$ skr kms-api key create --help
$ skr kms-api key read --help
$ skr kms-api key list --help
$ skr kms-api key encrypt --help
$ skr kms-api key decrypt --help
$ skr kms-api key rotate --help
$ skr kms-api key schedule-destruction --help
$ skr kms-api key change-status --help
$ skr kms-api key delete --help
```

認証は選択中の SDK プロファイルを使います。`KEY-NAME` は、以降のコマンドと JSON で同じ一意な名前に置き換えてください。

## Step 2: キーを作成する

新しいキー名に一致するキーがないことも確認します。名前が未使用の場合の出力は `[]` です。これは期待する出力形式であり、ライブ環境で取得した結果ではありません。実際に別の要素が表示された場合は、そのキーを変更・削除せず、異なる名前を選んでください。

```console
$ skr kms-api key list --query 'map(select(.Name == "KEY-NAME") | {ID,Name,Status})'
```

```json
[]
```

生成モードでキーを作成します。`--description` は省略できます。

```console
$ skr kms-api key create --name 'KEY-NAME' --description 'KMS tutorial'
```

```json
{
  "ID": "KEY-ID",
  "CreatedAt": "CREATED-AT",
  "ModifiedAt": "MODIFIED-AT",
  "ServiceClass": "cloud/kms/key",
  "Name": "KEY-NAME",
  "Description": "KMS tutorial",
  "KeyOrigin": "generated",
  "LatestVersion": 1,
  "Status": "active",
  "DeletionScheduledAfter": null,
  "Tags": []
}
```

作成結果の `ID` を控え、以降の `KEY-ID` に指定してください。作成コマンドがエラーになり ID を取得できなかった場合は、名前だけを手掛かりに削除しないでください。作成された可能性がある場合は、`kms-api key list` やコントロールパネルで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

BYOK モードでインポートする場合は、256 bit の対称鍵を base64 エンコードした内容のファイルを用意し、`--plain-key-file` で指定します。ファイルを引数として渡すだけで、鍵素材をコマンド引数に含めないでください。インポートしたキーはローテーションできません。

## Step 3: 作成したキーを参照する

```console
$ skr kms-api key read KEY-ID --query '{ID,Name,Status,KeyOrigin,LatestVersion}'
```

```json
{
  "ID": "KEY-ID",
  "Name": "KEY-NAME",
  "Status": "active",
  "KeyOrigin": "generated",
  "LatestVersion": 1
}
```

一覧でも同じ内容が返ることを確認します。

```console
$ skr kms-api key list --query 'map(select(.ID == "KEY-ID") | {ID,Name,Status})'
```

名前と説明の更新は `key update` で行います。API の更新リクエストは `Name` が必須です。タグを設定・維持する場合は `--request` JSON で `Tags` を指定します。

## Step 4: 平文を暗号化し、復号する

暗号化する平文をローカルファイルに保存します。平文は `--plain-file` でファイルから読み込み、API 向けに base64 エンコードされて送信されます。`-` を指定すると標準入力から読み込みます。アルゴリズムは `--algo` で指定でき、未指定の場合は `aes-256-gcm` が使われます。

```console
$ skr kms-api key encrypt KEY-ID --plain-file plain.txt
```

```json
"CIPHERTEXT"
```

暗号文は base64 エンコードされたテキストです。ファイルに保存し、`--cipher-file` で指定して復号します。`encrypt` の JSON 出力をそのままリダイレクトしたファイルも使えます。復号結果は平文のバイト列のまま標準出力へ出力されます。

```console
$ skr kms-api key encrypt KEY-ID --plain-file plain.txt > cipher.txt
$ skr kms-api key decrypt KEY-ID --cipher-file cipher.txt
HelloKMS
```

出力が平文と一致することを確認してください。確認後、`cipher.txt` は不要になったら削除します。

## Step 5: キーをローテーションする

ローテーションは有効 (active) なキーだけが対象です。実行後、`LatestVersion` が増えることを確認します。ローテーション前のバージョンで暗号化したデータも、同じキー ID で復号できます。

```console
$ skr kms-api key rotate KEY-ID
$ skr kms-api key read KEY-ID --query '{ID,LatestVersion}'
```

## Step 6: 削除を予約し、取り消す

削除は猶予期間 7〜90 日の削除予約制です。`schedule-destruction` の `--pending-days` で猶予日数を指定します。削除予約中のキーはステータス `pending_destruction` になり、利用できません。

```console
$ skr kms-api key schedule-destruction KEY-ID --pending-days 7
```

```json
{
  "Status": "pending_destruction",
  "DeletionScheduledAfter": "DELETION-SCHEDULED-AFTER"
}
```

削除を取り消す場合は、状態を `active` に戻します。

```console
$ skr kms-api key change-status KEY-ID --status active
"active"
```

状態は `active` (有効: 暗号化・復号可)、`restricted` (制限中: 復号のみ可)、`suspended` (一時停止中: 操作不可) のいずれかに変更できます。

## Step 7: キーを削除する

削除前に `key read` で ID と名前がこの手順で作成したキーと一致することを再確認します。一致しない場合や ID を確認できない場合は削除しません。

```console
$ skr kms-api key read KEY-ID --query '{ID,Name,Status}'
$ skr kms-api key delete KEY-ID
$ skr kms-api key list --query 'map(select(.ID == "KEY-ID") | {ID,Name})'
```

対象 ID が一覧に残っていない場合の出力は `[]` です。以下は E2E テストで確認している削除後の出力形式です。

```json
[]
```

キーを削除すると、そのキーで暗号化したデータは復号できなくなります。連携リソースがある場合は、削除前に依存関係を確認してください（[KMS 基本情報](https://manual.sakura.ad.jp/cloud/appliance/kms/index.html)）。

キーの操作一覧は `skr kms-api key --help` で確認できます。

## 参考資料

- [KMS 基本情報](https://manual.sakura.ad.jp/cloud/appliance/kms/index.html)
- [KMS API](https://manual.sakura.ad.jp/api/cloud/security-encryption/#tag/kms-key)
- [KMS / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/kms)
