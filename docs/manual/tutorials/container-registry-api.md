# Container Registry API: レジストリと認証ユーザーを管理する

Container Registry はコンテナイメージを保管するレジストリです。このチュートリアルでは、`skr` でレジストリと認証ユーザーを作成・確認・更新・削除します。イメージの push / pull はコンテナクライアントで行います。サービスの利用方法は [Container Registry のマニュアル](https://manual.sakura.ad.jp/cloud/appliance/container-registry/index.html)を参照してください。

> [!WARNING]
> この手順は選択中のプロファイルが指す環境にレジストリとユーザーを作成し、最後に削除します。実行前に `skr config current` と対象環境を確認してください。既存のレジストリは更新・削除しません。

## 前提条件

- `skr` が利用でき、SDK プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること
- 対象のコンテナレジストリ API を操作できる認証情報を使用すること
- 他ユーザーが使っていないレジストリ名を用意すること
- 認証ユーザーのパスワードを保護されたファイルまたは標準入力から読み込めること

## 手順とリソース

1. コンテナレジストリを作成します。`Name` は `sakuracr.jp` のサブドメインに使われ、他ユーザーの名前と重複できません。作成後に名前は変更できません。
2. 認証ユーザーを作成し、ユーザー名と権限を設定します。認証が必要な操作や初回のイメージ登録には、あらかじめユーザーを作成します。
3. 作成したリソースを読み取り、ユーザー一覧と設定を確認します。最後に、ユーザーを削除してからレジストリを削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr container-registry-api --help
$ skr container-registry-api registry create --help
$ skr container-registry-api registry user add --help
```

コマンドは SDK のプロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で認証します。API の結果は JSON で出力し、`--query` で必要な項目を抽出できます。

## Step 2: コンテナレジストリを作成する

名前と説明だけを指定する場合は、個別フラグで作成できます。`REGISTRY-NAME` は今回作成する一意の名前に置き換えてください。

```console
$ skr container-registry-api registry create --name REGISTRY-NAME --description "Example container registry"
```

任意の `--icon-id` と `--virtual-domain` も指定できます。タグを指定する場合は、以下の JSON ファイル入力を使います。個別フラグと `--request` は相互排他です。どちらか一方の方法で作成してください。

`registry.json` を作成します。`Name` はレジストリ接続名として `sakuracr.jp` のサブドメインに使われます。小文字の英字で始まり、小文字英数字またはハイフンで構成し、英数字で終わる名前にします。他ユーザーがすでに使っている名前は指定できません。レジストリ名は作成後に変更できません。

以下の `skr-example-registry` は例示用の名前です。JSON と後続の検索コマンド内の名前を、今回作成する一意の名前に置き換えてください。

```json
{
  "Name": "skr-example-registry",
  "Description": "Example container registry",
  "Tags": ["example"]
}
```

`Description` は最大512文字です。`Tags` と `IconID` は任意です。`VirtualDomain` を使う場合は、そのドメインから `Name.sakuracr.jp` への CNAME を設定します。独自ドメインを変更するときは、CNAME とレジストリの `VirtualDomain` 設定の両方を更新します。

2026年6月25日以降に新規作成するレジストリは非公開です。公開設定はこのコマンドから変更できません。

```console
$ skr container-registry-api registry create --request @registry.json
```

出力された `ID` を後続コマンドで使います。作成後の一覧は、必要な項目だけを JSON 配列として確認できます。

```console
$ skr container-registry-api registry list --request '{"Names":["skr-example-registry"]}' --query 'map({ID,Name,SubDomainLabel,Description})'
```

次の出力例は、確認した一覧の ID、名前、説明を例示用の値に置き換えています。

```json
[
  {
    "Description": "Example container registry",
    "ID": "REGISTRY-ID",
    "Name": "skr-example-registry",
    "SubDomainLabel": "skr-example-registry"
  }
]
```

表示される配列の要素には `ID`、`Name`、`SubDomainLabel`、`Description` が含まれます。`SubDomainLabel` はレジストリ接続名です。出力から対象レジストリの ID を控えます。

## Step 3: 認証ユーザーを作成する

パスワードはコマンド引数に書かず、アクセスを制限したファイルに保存してください。`--password-file -` を指定すると標準入力から読み込みます。ユーザー名とパスワードはそれぞれ1〜63文字で、ユーザー名には英数字と `-` `+` `.` `@` `_`、パスワードには英数字と `-` `.` `@` `_` `*` を指定できます。

`REGISTRY-ID` は作成結果の `ID` に、`REGISTRY-USER` は任意のユーザー名に置き換えます。`REGISTRY-PASSWORD-FILE` は安全に用意したパスワードファイルのパスです。権限値は `all`、`readwrite`、`readonly` です。

```console
$ skr container-registry-api registry user add REGISTRY-ID --user-name REGISTRY-USER --password-file REGISTRY-PASSWORD-FILE --permission readwrite
$ skr container-registry-api registry user list REGISTRY-ID --query 'map({UserName,Permission})'
```

次の出力例は、確認したユーザー名を `REGISTRY-USER` に置き換えています。

```json
[
  {
    "Permission": "readwrite",
    "UserName": "REGISTRY-USER"
  }
]
```

一覧にはユーザー名と権限が表示されます。パスワードは出力しません。ユーザーを更新するときは `--permission` と `--password-file` のどちらか、または両方を指定できます。

`--password-file` だけを指定した場合は、現在の権限を読み取って保持します。対象ユーザーを確認できない場合は更新しません。

```console
$ skr container-registry-api registry user update REGISTRY-ID REGISTRY-USER --permission readonly
$ skr container-registry-api registry user list REGISTRY-ID --query 'map({UserName,Permission})'
```

更新後は、対象ユーザーの `Permission` が `readonly` になっていることを確認します。以下のユーザー名は例示用に置き換えています。

```json
[
  {
    "Permission": "readonly",
    "UserName": "REGISTRY-USER"
  }
]
```

## Step 4: レジストリを確認・更新する

読み取りではレジストリ ID を指定します。更新リクエストに含めたフィールドだけが更新され、省略したフィールドは変更されません。`Name` は作成後に変更できません。

`registry-update.json` を作成します。

```json
{
  "Description": "Updated example registry"
}
```

```console
$ skr container-registry-api registry read REGISTRY-ID
$ skr container-registry-api registry update REGISTRY-ID --request @registry-update.json
$ skr container-registry-api registry read REGISTRY-ID
```

更新後の読み取り結果で `Description` が `Updated example registry` になっていることを確認します。`SubDomainLabel` と `FQDN` が変わっていないことも確認してください。

## Step 5: 作成したリソースを削除する

この手順で作成したユーザーだけを先に削除し、続けてレジストリを削除します。`REGISTRY-ID` と `REGISTRY-USER` は、作成時に控えた値を指定してください。

```console
$ skr container-registry-api registry user delete REGISTRY-ID REGISTRY-USER
$ skr container-registry-api registry user list REGISTRY-ID --query 'map({UserName,Permission})'
[]
```

この手順で作成したユーザーが一覧に残っていないことを確認してから、今回作成したレジストリを削除します。

```console
$ skr container-registry-api registry delete REGISTRY-ID
$ skr container-registry-api registry list --request '{"Names":["skr-example-registry"]}' --query 'map({ID,Name,SubDomainLabel,Description})'
[]
```

検索結果が `[]` であれば、指定したレジストリは一覧に残っていません。

この CLI はレジストリと認証ユーザーの管理 API を操作します。リポジトリやタグの表示、コンテナイメージの push / pull はコンテナクライアントで行います。

## 参考資料

- [Container Registry の利用方法](https://manual.sakura.ad.jp/cloud/appliance/container-registry/index.html)
- [sacloud-sdk-go v0.3.0 Container Registry service](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/containerregistry)
- [sacloud-sdk-go v0.3.0 ContainerRegistryAPI](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iaas#ContainerRegistryAPI)
