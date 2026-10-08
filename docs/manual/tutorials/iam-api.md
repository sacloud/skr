# IAM API: ユーザー、グループ、ポリシーバインディングを管理する

IAM ポリシーは、ユーザー、グループ、サービスプリンシパルなどのプリンシパルにロールを割り当て、組織・フォルダ・プロジェクトの各スコープでアクセス権限を管理します。グループを使うと、ユーザーごとではなくグループ単位でアクセス権限を設定できます（[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)、[ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)）。

このチュートリアルでは `skr iam-api` で現在利用できる中核の3リソース、`user`、`group`、`policy` を扱います。IAM の全16リソースを網羅するものではありません。対象外のリソースは段階的に追加されます。

> [!WARNING]
> この手順では IAM ユーザーとグループを作成・更新・削除します。実行前に `skr config current` で使用する認証情報を確認し、他と重複しない名前を指定してください。ユーザーの作成・更新はパスワードを扱います。パスワードファイルはアクセス権を制限し、処理後に削除してください。
>
> グループの所属更新と IAM ポリシーバインディングの更新は、対象の現在の一覧全体を置き換えます。指定しなかった既存の所属や割り当ては削除されます。読み取った内容を基に変更し、自分自身や運用に必要な権限を取り消さないよう、更新前に内容を確認してください（[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。

## 前提条件

- `skr` と `jq` が利用でき、SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数が設定されていること。
- 選択した認証情報に、ユーザーとグループの参照・作成・更新・削除、および対象スコープの IAM ポリシーバインディングの参照・更新に必要な権限があること。IAM グループの編集には管理者または適切な権限を持つユーザーが必要です（[ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)、[IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。
- IAM API はゾーンを指定しません。プロジェクトやフォルダのポリシーを扱う場合は、それぞれの ID を事前に用意してください。

以降の JSON 出力例は CLI の形式を示すもので、ID は説明用の値に置き換えています。ライブ環境から取得した出力ではありません。

## 作成するリソースと順序

1. **IAM ユーザー** — 作成し、属性とメールアドレスを更新・確認します。
2. **IAM グループ** — 作成し、ユーザーを所属させてから所属とグループ情報を確認します。
3. **IAM ポリシーバインディング** — 組織・プロジェクト・フォルダの割り当てを確認し、必要な場合は読み取った内容を基に更新します。
4. **クリーンアップ** — 作成したグループを先に削除し、続いてユーザーを削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr iam-api --help
$ skr iam-api user list --help
$ skr iam-api user create --help
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
```

表示されたプロファイルが対象の組織に対応していることを確認してください。IAM API には `--zone` の指定はありません。次の名前を使う場合は、作成前に同名のユーザーやグループがないことを確認します。既に存在する場合は別の名前に変更してください。

```console
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user") | {id,name})'
$ skr iam-api group list --query 'map(select(.name == "iam-tutorial-group") | {id,name})'
```

どちらも `[]` であれば、例の名前は使用されていません。別の名前を使う場合は、以降のコマンドと `--query` の条件も同じ名前に変更してください。ユーザー一覧は `--ordering` に `code` または `-code`、グループ一覧は `name` または `-name` を指定して並べ替えられます。

## Step 2: IAM ユーザーを作成する

ユーザー名、ユーザーコード、説明、パスワードを指定して作成します。例のユーザー名とユーザーコードは重複しない値にしてください。説明は空文字列も指定できます（`--description ''`）。

パスワードは `--password-file` でファイルまたは標準入力から指定します。パスワードをコマンド引数やシェル履歴に残さないため、個別フラグはありません。`-` を指定すると標準入力から読み込みます。パスワードは英数字と ASCII 記号のみを使い、英字と数字を含め、組織のパスワードポリシーに従ってください。

```console
$ umask 077
$ vi password.txt
$ skr iam-api user create --name iam-tutorial-user --code iam-tutorial-user --description "IAM tutorial user" --password-file password.txt > user-result.json
$ USER_ID=$(jq -er '.id' user-result.json)
$ rm password.txt
```

`password.txt` に作成するユーザーのパスワードを記載します。ファイルにはパスワード以外の値を書かず、`umask 077` の状態で作成してください。作成後はファイルを削除します。コマンドが失敗した場合も、ファイルを不要なまま残さないでください。

作成したユーザーが一覧に表示されることを確認します。次は CLI テストで確認した出力形式の例で、ID は実際の値に置き換わります。

```console
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user") | {id,name})'
```

```json
[
  {
    "id": 123456789012,
    "name": "iam-tutorial-user"
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
  "code": "iam-tutorial-user",
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

## Step 6: 作成したリソースを削除する

ポリシーバインディングを変更した場合、リソースを削除する前に、元に戻す必要があるか確認してください。元の割り当てに戻す場合は、変更前に保存したファイルを対象スコープへ適用し、読み直して確認します。

```console
$ skr iam-api policy update-organization --request @policy-bindings-original.json
$ skr iam-api policy read-organization
$ skr iam-api policy update-project PROJECT-ID --request @project-policy-bindings-original.json
$ skr iam-api policy read-project PROJECT-ID
$ skr iam-api policy update-folder FOLDER-ID --request @folder-policy-bindings-original.json
$ skr iam-api policy read-folder FOLDER-ID
```

この手順で作成したグループとユーザーだけを削除してください。グループを先に削除し、続いてユーザーを削除します。削除前に ID がこの手順で保存した値であることを確認してください。

```console
$ skr iam-api group delete "$GROUP_ID"
$ skr iam-api user delete "$USER_ID"
$ skr iam-api user list --query 'map(select(.name == "iam-tutorial-user") | {id,name})'
[]
$ skr iam-api group list --query 'map(select(.name == "iam-tutorial-group") | {id,name})'
[]
$ rm -f user-result.json group-result.json policy-bindings.json policy-result.json \
  project-policy-bindings.json folder-policy-bindings.json
```

名前で絞り込んだ一覧が `[]` であることを確認します。別の名前を使った場合は、一覧確認の `--query` にもその名前を指定してください。変更前のポリシーバインディングを保存した `*-original.json` は、復元が不要であることを確認してから削除してください。チュートリアルで作成していないリソースやポリシーは削除・変更しないでください。

## 参考情報

- [IAMポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)
- [プロジェクトとユーザの関係](https://manual.sakura.ad.jp/cloud/controlpanel/user-project.html)
- [ユーザグループ機能](https://manual.sakura.ad.jp/cloud/controlpanel/user-group.html)
- [リソース管理機能](https://manual.sakura.ad.jp/cloud/controlpanel/resource-management.html)
- [サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)
- [sacloud-sdk-go v0.3.0: IAM API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam)
- [sacloud-sdk-go v0.3.0: User API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis/user)
- [sacloud-sdk-go v0.3.0: Group API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iam/apis/group)
