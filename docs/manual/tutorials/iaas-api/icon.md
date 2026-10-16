# IaaS Icon API: 画像ファイルからユーザー作成アイコンを管理する

コントロールパネルの「設定」にあるアイコン管理では、ユーザー作成アイコンを登録・確認・削除できます。登録できる画像形式は PNG と JPEG です（[アイコンの管理](https://manual.sakura.ad.jp/cloud/controlpanel/icon.html)）。この手順では `skr iaas-api icon` を使い、画像ファイルからアイコンを作成し、検索・参照・更新した後に削除します。

> [!WARNING]
> この手順は、選択中のプロファイルが指すプロジェクトにアイコンを作成・更新・削除します。実行前に対象プロファイルと変更内容を確認してください。この手順で作成したアイコンだけを削除してください。

## 前提条件

- `skr iaas-api icon` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドです。
- 対象プロジェクトの認証情報を SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。
- 登録する PNG または JPEG 画像ファイルがあること。
- `TEST-ICON-NAME` と `UPDATED-TEST-ICON-NAME` を、他のアイコンと重複しないテスト専用の名前に置き換えること。`123456789012` は仮の ID です。作成結果の `ID` に置き換えてください。

## 作成するリソースと順序

この手順で作成するのはユーザー作成アイコン1個です。IaaS Icon API のリクエストには `Zone` がないため、各操作で `--zone` は指定しません。アイコンの API 情報には ID、名前、タグなどが含まれます（[sacloud-sdk-go v0.3.0: Icon](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iaas#Icon)）。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api icon --help
$ skr iaas-api icon create --help
$ skr iaas-api icon find --help
$ skr iaas-api icon read --help
$ skr iaas-api icon update --help
$ skr iaas-api icon delete --help
```

`config current` で意図したプロファイルが選択されていることを確認します。作成時は `--name` と `--image-file` を指定します。画像ファイルは API 用に Base64 形式へ変換されます。`--request` を使う場合は `CreateRequest` JSON を指定しますが、個別フラグとは併用できません。JSON は直接指定するか `@ファイル名` で読み込めます。

## Step 2: テスト用アイコンを作成する

`TEST-ICON-NAME` を他のアイコンと重複しない名前に置き換え、画像ファイルのパスを指定します。タグはカンマ区切りで指定できます。

```console
$ skr iaas-api icon create --name TEST-ICON-NAME --image-file ./icon.png --tags tutorial,managed
```

成功時は作成されたアイコンの JSON が出力されます。後続の参照・更新・削除に使う `ID` を控えてください。次の出力は形式を示すため、ID と名前を置き換えた例です。

```json
{
  "ID": 123456789012,
  "Name": "TEST-ICON-NAME",
  "Tags": ["tutorial", "managed"]
}
```

JSON 入力で作成する場合は、`Name`、Base64 形式の `Image`、必要に応じて `Tags` を `CreateRequest` に指定し、保護されたファイルから読み込みます。たとえば、画像データを Base64 文字列として用意した JSON を `icon-create.json` に保存して、次のように実行します。プレースホルダーは実際の Base64 データに置き換えてください。

```json
{
  "Name": "TEST-ICON-NAME",
  "Image": "BASE64-ENCODED-IMAGE",
  "Tags": ["tutorial", "managed"]
}
```

```console
$ skr iaas-api icon create --request @icon-create.json
```

個別フラグと `--request` は併用できません。画像データそのものをコマンド引数に含めないでください。

## Step 3: アイコンを検索・参照して更新する

検索の `Names` は JSON の文字列配列で指定します。`icon-find.json` に次の内容を保存し、ID、名前、タグだけを抽出して確認します。

```json
{
  "Names": ["TEST-ICON-NAME"]
}
```

```console
$ skr iaas-api icon find --request @icon-find.json --query 'map({ID,Name,Tags})'
```

出力が作成したアイコンの ID、名前、タグと一致することを確認します。以下は値を置き換えた出力例です。

```json
[
  {
    "ID": 123456789012,
    "Name": "TEST-ICON-NAME",
    "Tags": ["tutorial", "managed"]
  }
]
```

作成結果の ID で参照します。ID は実際の値に置き換えてください。

```console
$ skr iaas-api icon read --id 123456789012
```

名前とタグを更新します。`UPDATED-TEST-ICON-NAME` を一意なテスト用の名前に置き換えてください。更新で指定できるのは名前とタグです。省略した項目は変更されません。

```console
$ skr iaas-api icon update --id 123456789012 --name UPDATED-TEST-ICON-NAME --tags tutorial,updated
$ skr iaas-api icon read --id 123456789012
```

読み取った `Name` と `Tags` が更新後の値であることを確認します。

## Step 4: 作成したアイコンを削除する

削除前に `read --id 123456789012` で ID と名前を再確認し、仮の ID を作成結果の ID に置き換えてください。`--fail-if-not-found` を指定すると、対象が見つからない場合はエラーになります。

```console
$ skr iaas-api icon delete --id 123456789012 --fail-if-not-found
$ skr iaas-api icon find --request @icon-find-updated.json --query 'map({ID,Name,Tags})'
```

`icon-find-updated.json` は更新後の名前で検索する内容にします。

```json
{
  "Names": ["UPDATED-TEST-ICON-NAME"]
}
```

削除後の検索結果が空配列であることを確認します。

```json
[]
```

## 参考情報

- [さくらのクラウドマニュアル: アイコンの管理](https://manual.sakura.ad.jp/cloud/controlpanel/icon.html)
- [さくらのクラウド API v1.1: アイコン関連 API](https://manual.sakura.ad.jp/cloud-api/1.1/icon/index.html)
- [sacloud-sdk-go v0.3.0: Icon API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/icon)
