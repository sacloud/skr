# IaaS Region API: リージョン情報を検索・参照する

`iaas-api region` でリージョンの一覧を取得し、ID を指定して個別の情報を確認できます。Region API が返す ID、名前、説明、ネームサーバーを JSON で確認できます。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。

この手順は読み取り専用です。クラウドリソースを作成、変更、削除しません。

## 手順

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api region --help
$ skr iaas-api region find --help
$ skr iaas-api region read --help
```

Region API は `--zone` を受け付けません。Find の `--count` と `--from` は任意の取得条件です。検索名や Sort は `--request` JSON で指定します。`--request` と個別フラグは併用できません。

### 2. リージョン一覧を検索する

```console
$ skr iaas-api region find --query 'map({ID,Name,Description,NameServers})'
```

Find はリージョンの配列を JSON で出力します。表示項目は `--query` で選べます。取得開始位置や件数を指定する場合は次のようにします。

```console
$ skr iaas-api region find --count 10 --from 0 --query 'map({ID,Name})'
```

出力は配列です。次は形式を示すため、値をプレースホルダーに置き換えた例です。

```json
[
  {
    "ID": 123,
    "Name": "REGION-NAME",
    "Description": "DESCRIPTION",
    "NameServers": ["NAME-SERVER"]
  }
]
```

検索名と Sort を指定する場合は、SDK の FindRequest 形式の JSON をファイルに保存して渡します。

```json
{
  "Names": ["NAME"]
}
```

`NAME` は検索する名前に置き換えてください。`Sort` を指定する場合も FindRequest JSON に含めます。

```console
$ skr iaas-api region find --request @region-find.json --query 'map({ID,Name})'
```

### 3. ID でリージョン情報を参照する

一覧から確認した ID を指定します。

```console
$ skr iaas-api region read --id REGION-ID --query '{ID,Name,Description,NameServers}'
```

`REGION-ID` は一覧で確認した数値 ID に置き換えます。Read は対象のリージョン情報を JSON で出力します。

```json
{
  "ID": 123,
  "Name": "REGION-NAME",
  "Description": "DESCRIPTION",
  "NameServers": ["NAME-SERVER"]
}
```

上記は実際の応答形式を示す例で、値は置き換えています。Find で取得した先頭の ID を使って Read し、ID、名前、説明、ネームサーバーが一致することを確認できます。

## クリーンアップ

この手順は読み取り専用です。削除するリソースはありません。

## 参考資料

- [さくらのクラウド API v1.1: 設備関連 API](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html)
- [sacloud-sdk-go v0.3.1: Region API の検索操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/find_service.go)
- [sacloud-sdk-go v0.3.1: Region API の参照操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/read_service.go)
- [sacloud-sdk-go v0.3.1: Region のリクエストとモデル](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/find_request.go)
- [sacloud-sdk-go v0.3.1: Region model](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/api/iaas/zz_models.go#L3625)
