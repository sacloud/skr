# IAM API: 組織と IAM リソースを管理する

IAM ポリシーは、ユーザー、グループ、サービスプリンシパルなどのプリンシパルにロールを割り当て、組織・フォルダ・プロジェクトの各スコープでアクセス権限を管理します。グループを使うと、ユーザーごとではなくグループ単位でアクセス権限を設定できます（[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)、[ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)）。

`skr iam-api` では、ユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパル、各種ポリシー、認証設定、API キー、SSO、SCIM、2 要素認証などを操作できます。このチュートリアルではユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパルを作成して確認し、ポリシーや認証設定を安全に参照・更新する手順を説明します。

> [!WARNING]
> この手順では IAM ユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパルを作成・更新・削除します。実行前に `skr config current` で使用する認証情報を確認し、他と重複しない名前を指定してください。プロジェクトの削除は取り消せません。ユーザーの作成・更新はパスワードを扱います。パスワードファイルはアクセス権を制限し、処理後に削除してください。
>
> グループの所属更新と IAM ポリシーバインディングの更新は、対象の現在の一覧全体を置き換えます。指定しなかった既存の所属や割り当ては削除されます。読み取った内容を基に変更し、自分自身や運用に必要な権限を取り消さないよう、更新前に内容を確認してください（[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。

## 前提条件

- `skr` と `jq` が利用でき、SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数が設定されていること。
- 選択した認証情報に、チュートリアルで扱うユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパルの参照・作成・更新・削除に必要な権限があること。ユーザーの追加には会員 ID または管理者権限を持つユーザーが必要です。IAM グループの編集には管理者または適切な権限を持つユーザーが必要です（[ユーザー](https://manual.sakura.ad.jp/cloud/controlpanel/user-project-user.html)、[ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)、[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。
- IAM API はゾーンを指定しません。プロジェクトやフォルダのポリシーを扱う場合は、それぞれの ID を事前に用意してください。

以降の JSON 出力例は CLI の形式を示すもので、ID は説明用の値に置き換えています。ライブ環境から取得した出力ではありません。

## 作成するリソースと順序

1. **IAM ユーザーとグループ** — ユーザーを作成・更新し、グループを作成してユーザーを所属させます。
2. **フォルダ、プロジェクト、サービスプリンシパル** — フォルダ内にプロジェクトを作成し、そのプロジェクトにサービスプリンシパルを作成します。
3. **IAM ポリシーバインディング** — 組織・プロジェクト・フォルダの割り当てを確認し、必要な場合は読み取った内容を基に更新します。
4. **クリーンアップ** — 作成したサービスプリンシパルとグループを先に削除し、ユーザー、プロジェクト、フォルダを削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr iam-api --help
$ skr iam-api user list --help
$ skr iam-api user create --help
$ skr iam-api project create --help
$ skr iam-api user register-email --help
$ skr iam-api user update --help
$ skr iam-api group --help
$ skr iam-api group list --help
$ skr iam-api group update-memberships --help
$ skr iam-api policy --help
$ skr iam-api policy read-organization --help
$ skr iam-api policy update-organization --help
$ skr iam-api policy update-project --help
$ skr iam-api policy update-folder --help
$ skr iam-api folder --help
$ skr iam-api project --help
$ skr iam-api folder move --help
$ skr iam-api project move --help
$ skr iam-api service-principal --help
$ skr iam-api service-principal list-keys --help
$ skr iam-api auth --help
$ skr iam-api id-policy --help
$ skr iam-api project-api-key --help
$ skr iam-api scim --help
$ skr iam-api sso --help
$ skr iam-api user-2fa --help
```

表示されたプロファイルが対象の組織に対応していることを確認してください。IAM API には `--zone` の指定はありません。次の名前を使う場合は、作成前に同名のユーザーやグループがないことを確認します。既に存在する場合は別の名前に変更してください。

```console
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user" or .code == "e2e_iam_tutorial_user") | {id,name,code})'
$ skr iam-api group list --query 'map(select(.name == "iam-tutorial-group") | {id,name})'
$ skr iam-api folder list --query '.items | map(select(.name == "iam-tutorial-folder") | {id,name})'
$ skr iam-api project list --query '.items | map(select(.name == "iam-tutorial-project" or .code == "iamtutorialproject20261008") | {id,name,code})'
$ skr iam-api service-principal list --query '.items | map(select(.name == "iam-tutorial-principal") | {id,name,project_id})'
```

ユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパルの確認結果がすべて `[]` であれば、例の名前とプロジェクトコードは一覧で見つかっていません。1つでも項目が表示された場合は、作成せず別の名前とコードに変更してください。別の値を使う場合は、以降のコマンドと `--query` の条件も同じ値に変更します。ユーザー一覧は `--ordering` に `code` または `-code`、グループ一覧は `name` または `-name` を指定して並べ替えられます。

次はそれぞれの確認コマンドで、該当項目がない場合に得られる出力です。

```json
[]
```

## Step 2: IAM ユーザーを作成する

ユーザー名、ユーザーコード、説明、パスワードを指定して作成します。ユーザーコードは名前とは別に指定し、組織内で重複しない値にしてください。説明は空文字列も指定できます（`--description ''`）。

パスワードは `--password-file` でファイルまたは標準入力から指定します。パスワードをコマンド引数やシェル履歴に残さないため、個別フラグはありません。`-` を指定すると標準入力から読み込みます。パスワードは英数字と ASCII 記号のみを使い、英字と数字を含め、組織のパスワードポリシーに従ってください。

```console
$ umask 077
$ vi password.txt
$ skr iam-api user create --name iam-tutorial-user --code e2e_iam_tutorial_user --description "IAM tutorial user" --password-file password.txt > user-result.json
$ USER_ID=$(jq -er '.id' user-result.json)
$ rm password.txt
```

`password.txt` に作成するユーザーのパスワードを記載します。ファイルにはパスワード以外の値を書かず、`umask 077` の状態で作成してください。作成後はファイルを削除します。コマンドが失敗した場合も、ファイルを不要なまま残さないでください。

作成したユーザーが一覧に表示され、指定したコードが登録されたことを確認します。次は出力形式の例で、ID は実際の値に置き換わります。

```console
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user") | {id,name,code})'
```

```json
[
  {
    "id": 123456789012,
    "name": "iam-tutorial-user",
    "code": "e2e_iam_tutorial_user"
  }
]
```

## Step 3: ユーザーを更新してメールアドレスを登録する

ユーザーの名前と説明を更新します。どちらも必須で、指定した値で上書きされます。パスワードを変更する場合だけ `--password-file` を追加します。

```console
$ skr iam-api user update "$USER_ID" --name iam-tutorial-user --description "IAM tutorial user (updated)"
$ skr iam-api user read "$USER_ID" --query '{id,name,code,description,email}'
```

```json
{
  "id": 123456789012,
  "name": "iam-tutorial-user",
  "code": "e2e_iam_tutorial_user",
  "description": "IAM tutorial user (updated)",
  "email": ""
}
```

`register-email` でユーザーのメールアドレスを登録します。ユーザー作成時の `--email` は SSO プロファイル有効時に外部 IdP のログインに使うメールアドレスです。`register-email` で登録するものとは別の項目です。

```console
$ skr iam-api user register-email "$USER_ID" --email iam-tutorial@example.com
$ skr iam-api user read "$USER_ID" --query '{id,email}'
```

```json
{
  "id": 123456789012,
  "email": "iam-tutorial@example.com"
}
```

メールアドレスの登録を解除すると、`email` は空に戻ります。

```console
$ skr iam-api user unregister-email "$USER_ID"
$ skr iam-api user read "$USER_ID" --query '{id,email}'
```

```json
{
  "id": 123456789012,
  "email": ""
}
```

## Step 4: IAM グループを作成してユーザーを所属させる

グループ名と説明を指定して作成します。説明は空文字列も指定できます。グループを更新するときは名前と説明の両方を指定してください。

```console
$ skr iam-api group create --name iam-tutorial-group --description "IAM tutorial group" > group-result.json
$ GROUP_ID=$(jq -er '.id' group-result.json)
$ skr iam-api group update "$GROUP_ID" --name iam-tutorial-group --description "IAM tutorial group (updated)"
$ skr iam-api group read "$GROUP_ID"
```

作成したグループにユーザーを所属させます。所属はユーザー ID の JSON 配列で指定します。現在の所属は保持されず、指定した配列で上書きされるため、既存の所属にユーザーを追加する場合は `read-memberships` の出力に追加後のユーザー ID を加えた配列を指定してください。

```console
$ skr iam-api group update-memberships "$GROUP_ID" --request "[$USER_ID]"
$ skr iam-api group read-memberships "$GROUP_ID"
```

所属ユーザーの一覧に出力されます。次は出力形式の例で、ID は実際の値に置き換わります。

```json
[
  {
    "id": 123456789012
  }
]
```

`--user-id` を指定したグループ一覧で、ユーザーが所属するグループを確認します。

```console
$ skr iam-api group list --user-id "$USER_ID" --query 'map({id,name})'
```

```json
[
  {
    "id": 123456789013,
    "name": "iam-tutorial-group"
  }
]
```

## Step 5: IAM ポリシーバインディングを参照・更新する

ポリシーバインディングは、プリンシパル（ユーザー、グループ、サービスプリンシパル）とロールの割り当てです。組織、プロジェクト、フォルダの各スコープについて、対応する `read-*` コマンドで現在の割り当てを確認します。プロジェクトとフォルダのコマンドには、それぞれ対象の ID を指定してください。上位のスコープから継承されたポリシーもあります（[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)、[リソース管理機能](https://manual.sakura.ad.jp/cloud/controlpanel/resource-management.html)）。

```console
$ skr iam-api policy read-organization
$ skr iam-api policy read-project PROJECT-ID
$ skr iam-api policy read-folder FOLDER-ID
```

出力は割り当ての配列です。各要素は `role` と `principals` を持ちます。`role` は `type` と `id`、`principals` はプリンシパルの `type` と数値の `id` の配列です。次は構造を示す形式例で、プレースホルダーを実在する値に置き換えない限り更新には使えません。

```json
[
  {
    "role": {
      "type": "preset",
      "id": "ROLE-ID"
    },
    "principals": [
      {
        "type": "user",
        "id": 123456789012
      }
    ]
  }
]
```

割り当てを変更する場合は、変更対象のスコープから読み取った配列全体を保存して編集し、そのスコープに対応する `update-*` コマンドへ `--request @ファイル名` で渡します。`@` で始まる値は JSON ファイルとして読み込まれます。JSON 内のロールとプリンシパルには、その組織で有効な値を指定してください。利用できるロールと割り当て可能な範囲は [IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html) で確認します。

> [!WARNING]
> 更新は対象スコープの現在の割り当て全体を置き換えます。読み取った配列から必要な割り当てを誤って除外すると、その権限が失われる場合があります。更新する JSON を保存する前に、対象スコープと配列全体を確認してください。

```console
$ skr iam-api policy read-organization > policy-bindings.json
$ cp policy-bindings.json policy-bindings-original.json
$ vi policy-bindings.json
$ skr iam-api policy update-organization --request @policy-bindings.json > policy-result.json
$ skr iam-api policy read-organization
```

更新後は同じスコープを再度読み取り、変更した割り当てと保持した割り当ての両方が意図した内容になっていることを確認します。プロジェクトとフォルダも同様に、対象スコープを読み取って保存・編集してから更新します。

```console
$ skr iam-api policy read-project PROJECT-ID > project-policy-bindings.json
$ cp project-policy-bindings.json project-policy-bindings-original.json
$ vi project-policy-bindings.json
$ skr iam-api policy update-project PROJECT-ID --request @project-policy-bindings.json
$ skr iam-api policy read-project PROJECT-ID

$ skr iam-api policy read-folder FOLDER-ID > folder-policy-bindings.json
$ cp folder-policy-bindings.json folder-policy-bindings-original.json
$ vi folder-policy-bindings.json
$ skr iam-api policy update-folder FOLDER-ID --request @folder-policy-bindings.json
$ skr iam-api policy read-folder FOLDER-ID
```

## 動作の確認

ユーザーは `read` で名前・説明・メールアドレスを、グループは `read-memberships` と `list --user-id` で所属を確認できます。ポリシーは更新後に同じスコープの `read-*` を実行し、割り当て配列全体が意図した内容になっていることを確認します。特にポリシー更新は対象スコープの割り当て全体を置き換えるため、変更内容を確認し、必要な権限を保持してください。

## Step 6: フォルダ、プロジェクト、サービスプリンシパルを管理する

フォルダにプロジェクトをまとめ、そのプロジェクト内にサービスプリンシパルを作成できます。名前は既存リソースと重複しない値に変更してください。`--query` では一覧の `items` を必要な項目だけに投影して確認できます。

```console
$ skr iam-api folder list --query '.items | map({id,name,description})'
$ skr iam-api project list --query '.items | map({id,name,code,description})'
$ skr iam-api service-principal list --query '.items | map({id,name,project_id,description})'
```

フォルダを作成し、その ID を使ってプロジェクトを作成します。プロジェクトコードは名前とは別に指定し、組織内で重複しない値にしてください。例では `iamtutorialproject20261008` を使っています。日付部分を実行日に合わせるなど、既存のプロジェクトコードと重ならない値に変更してください。サービスプリンシパルはプロジェクト ID を指定して作成します。

```console
$ skr iam-api folder create --name iam-tutorial-folder --description "IAM tutorial folder" > folder-result.json
$ FOLDER_ID=$(jq -er '.id' folder-result.json)
$ skr iam-api project create --code iamtutorialproject20261008 --name iam-tutorial-project \
  --description "IAM tutorial project" --parent-folder-id "$FOLDER_ID" > project-result.json
$ PROJECT_ID=$(jq -er '.id' project-result.json)
$ skr iam-api service-principal create --project-id "$PROJECT_ID" \
  --name iam-tutorial-principal --description "IAM tutorial service principal" > principal-result.json
$ PRINCIPAL_ID=$(jq -er '.id' principal-result.json)
```

作成した ID を使ってフォルダとプロジェクトを更新し、読み取りと一覧の両方で結果を確認します。更新では名前と説明を指定した値で置き換えます。

```console
$ skr iam-api folder update "$FOLDER_ID" --name iam-tutorial-folder --description "IAM tutorial folder (updated)"
$ skr iam-api project update "$PROJECT_ID" --name iam-tutorial-project --description "IAM tutorial project (updated)"
$ skr iam-api folder read "$FOLDER_ID"
$ skr iam-api project read "$PROJECT_ID"
$ skr iam-api service-principal read "$PRINCIPAL_ID"
$ skr iam-api service-principal list --project-id "$PROJECT_ID" \
  --query '.items | map({id,name,project_id,description})'
```

フォルダとプロジェクトは、既存の項目を別の親フォルダへ移動できます。移動する ID は `--ids` にカンマ区切りで指定し、移動先のフォルダ ID も明示してください。フォルダの移動では階層が有効であることを確認してください。移動先や影響範囲を確認できない既存リソースは移動しないでください。

```console
$ skr iam-api folder move --ids FOLDER-ID --parent-id PARENT-FOLDER-ID
$ skr iam-api project move --ids PROJECT-ID --parent-folder-id FOLDER-ID
```

サービスプリンシパルに公開鍵を登録する場合は、公開鍵を `--public-key-file` から渡します。トークン発行で使うアサーションも `--assertion-file` または標準入力から渡し、秘密情報をコマンドライン引数に含めないでください。API キーを作成する場合は、必須の IAM ロール配列を含む JSON リクエストを `--request @path.json` で指定してください。作成結果にはアクセストークンとシークレットが含まれるため、出力をアクセス制限したファイルへ保存してください（[APIキー](https://manual.sakura.ad.jp/cloud/api/apikey.html)）。

サービスプリンシパルを更新するときは、更新後の名前が必須です。`--name` を使うか、`--request` の JSON に `name` を含めてください。`description` を指定する場合は JSON リクエストに含めます。

```console
$ skr iam-api service-principal update "$PRINCIPAL_ID" --name iam-tutorial-principal-renamed
```

JSON リクエストの例です。`name` を更新後の名前に置き換えてファイルに保存します。

```json
{
  "name": "iam-tutorial-principal-renamed",
  "description": "IAM tutorial service principal"
}
```

```console
$ skr iam-api service-principal update "$PRINCIPAL_ID" --request @service-principal.json
```

SCIM 設定の更新も新しい設定名が必要です。名前だけを更新する場合は `--name` を使えます。JSON リクエストを使う場合も `name` を含めてください。

```console
$ skr iam-api scim update SCIM_ID --name iam-tutorial-scim-renamed
```

## 組織全体に影響する設定と認証情報

パスワードポリシーと認証条件は組織全体の設定です。更新前に `read-*` で現在値を確認してください。パスワードポリシーは4項目すべてをフラグで指定するか、全項目を含む JSON を `--request @ファイル名` で渡します。認証条件は現在値をファイルに保存し、完全な JSON を編集してから `update-auth-conditions --request @ファイル名` で更新してください。認証条件の IP 制限や二要素認証設定は、ログインに影響する可能性があります（[認証設定](https://manual.sakura.ad.jp/cloud/controlpanel/settings/index.html)）。

```console
$ skr iam-api auth read-password-policy
$ skr iam-api auth update-password-policy --min-length 12 --require-uppercase \
  --require-lowercase --require-symbols=false
$ skr iam-api auth read-auth-conditions
$ skr iam-api auth update-password-policy --help
$ skr iam-api auth update-auth-conditions --help
$ skr iam-api id-policy read-organization
$ skr iam-api organization read-service-policy
$ skr iam-api organization read-service-policy --is-active=true --name SERVICE_NAME
$ skr iam-api service-policy is-enabled
$ skr iam-api service-policy list-rule-templates
```

ID ポリシーとサービス利用ポリシーの更新は、対象範囲の現在の設定を確認してから行ってください。サービス利用ポリシーの変更はリソース操作の可否に影響し、設定内容は即時反映されます（[IDポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/id-policy.html)、[サービスポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/settings/service-policy.html)）。

SSO プロファイルをリンクすると、メールアドレスが登録されたユーザーのログイン方法が変わります。組織で有効にできるプロファイルは1つです。IdP の設定と対象ユーザーへのメールアドレス登録を確認してから `sso link` を実行してください（[シングルサインオン連携](https://manual.sakura.ad.jp/cloud/controlpanel/settings/sso.html)）。

SCIM のユーザープロビジョニングは外部 IdP からユーザーとグループを同期します。再発行したシークレットトークンを連携先へ設定し直してください。再発行すると古いトークンは使えなくなります。また、プロビジョニングされたユーザーとグループをすべて削除してから SCIM 設定を削除してください（[ユーザープロビジョニング](https://manual.sakura.ad.jp/cloud/controlpanel/settings/user-provisionings.html)）。

`user-2fa` の各操作は `--user-id` で対象を指定します。OTP の無効化や信頼済みデバイス・セキュリティキーの削除は、対象ユーザーの認証方法に影響するため、操作前に ID と現在の状態を確認してください。

IAM のページング一覧は `items`、`count`、`next`、`previous` を含む JSON オブジェクトを出力します。`--page` と `--per-page` で取得範囲を指定し、`--query '.items | map({id,name})'` のように必要な項目を投影できます。ユーザーとグループの一覧は配列を出力します。JSON リクエストは `--request` に直接指定するか `@path.json` でファイルから読み込みます。`--request` と個別フラグは併用できません。

## Step 7: 作成したリソースを削除する

ポリシーバインディングを変更した場合、リソースを削除する前に、元に戻す必要があるか確認してください。元の割り当てに戻す場合は、変更前に保存したファイルを対象スコープへ適用し、読み直して確認します。

```console
$ skr iam-api policy update-organization --request @policy-bindings-original.json
$ skr iam-api policy read-organization
$ skr iam-api policy update-project PROJECT-ID --request @project-policy-bindings-original.json
$ skr iam-api policy read-project PROJECT-ID
$ skr iam-api policy update-folder FOLDER-ID --request @folder-policy-bindings-original.json
$ skr iam-api policy read-folder FOLDER-ID
```

この手順で作成したリソースだけを削除してください。ユーザーが所属するグループとプロジェクトに属するサービスプリンシパルを先に削除します。続いてユーザー、プロジェクト、フォルダを削除します。プロジェクトの削除は取り消せず、プロジェクト内のリソースを先に削除する必要があります（[プロジェクトの削除](https://manual.sakura.ad.jp/cloud/controlpanel/user-project-project.html)）。削除前に ID がこの手順で保存した値であることを確認してください。

```console
$ skr iam-api service-principal delete "$PRINCIPAL_ID"
$ skr iam-api group delete "$GROUP_ID"
$ skr iam-api user delete "$USER_ID"
$ skr iam-api project delete "$PROJECT_ID"
$ skr iam-api folder delete "$FOLDER_ID"
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user" or .code == "e2e_iam_tutorial_user") | {id,name,code})'
[]
$ skr iam-api group list --query 'map(select(.name == "iam-tutorial-group") | {id,name})'
[]
$ skr iam-api service-principal list --query '.items | map(select(.name == "iam-tutorial-principal") | {id,name,project_id})'
[]
$ skr iam-api project list --query '.items | map(select(.name == "iam-tutorial-project" or .code == "iamtutorialproject20261008") | {id,name,code})'
[]
$ skr iam-api folder list --query '.items | map(select(.name == "iam-tutorial-folder") | {id,name})'
[]
$ rm -f user-result.json group-result.json folder-result.json project-result.json principal-result.json \
  policy-bindings.json policy-result.json project-policy-bindings.json folder-policy-bindings.json
```

名前で絞り込んだ一覧が `[]` であることを確認します。別の名前を使った場合は、一覧確認の `--query` にもその名前を指定してください。変更前のポリシーバインディングを保存した `*-original.json` は、復元が不要であることを確認してから削除してください。チュートリアルで作成していないリソースやポリシーは削除・変更しないでください。

## 参考情報

- [IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)
- [プロジェクトとユーザの関係](https://manual.sakura.ad.jp/cloud/controlpanel/user-project.html)
- [ユーザー](https://manual.sakura.ad.jp/cloud/controlpanel/user-project-user.html)
- [プロジェクトの削除](https://manual.sakura.ad.jp/cloud/controlpanel/user-project-project.html)
- [ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)
- [リソース管理機能](https://manual.sakura.ad.jp/cloud/controlpanel/resource-management.html)
- [サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)
- [認証設定](https://manual.sakura.ad.jp/cloud/controlpanel/settings/index.html)
- [シングルサインオン連携](https://manual.sakura.ad.jp/cloud/controlpanel/settings/sso.html)
- [ユーザープロビジョニング](https://manual.sakura.ad.jp/cloud/controlpanel/settings/user-provisionings.html)
- [IDポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/id-policy.html)
- [サービスポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/settings/service-policy.html)
- [APIキー](https://manual.sakura.ad.jp/cloud/api/apikey.html)
- [sacloud-sdk-go v0.3.0: IAM API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam)
- [sacloud-sdk-go v0.3.0: User API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis/user)
- [sacloud-sdk-go v0.3.0: Group API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis/group)
- [sacloud-sdk-go v0.3.0: Project API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis/project)
- [sacloud-sdk-go v0.3.0: IAM API packages](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis)
- [IAM API コマンド設計](../../design/iam-api-commands.md)
