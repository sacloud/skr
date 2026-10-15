# IaaS Archive API: ゾーン内のアーカイブを検索・参照する

Archive API で対象ゾーンのアーカイブを検索し、ID、名前、説明、ディスクプラン、サイズ、タグなどを確認できます。アーカイブをディスクのコピー元として選ぶ場合は、[ディスクの作成・変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)で作成先のゾーンとディスクプラン、サイズも確認してください。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。
- 検索するゾーン名が分かること。参照するアーカイブ ID は検索結果から取得します。

## 手順

この手順は読み取り専用です。クラウドリソースは作成、変更、削除しません。

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api archive --help
$ skr iaas-api archive find --help
$ skr iaas-api archive read --help
```

検索では単一のゾーン名を指定します。`--zone all` によるゾーン横断検索はできません。

### 2. OS 種別で公開アーカイブを検索する

```console
$ skr iaas-api archive find --zone ZONE --os-type ubuntu2404 --query 'map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})'
```

`ZONE` は検索対象のゾーンに置き換えます。`--os-type` には `ubuntu2404`、`debian12`、`almalinux9` などの公開アーカイブ OS 種別を指定できます。OS 種別の指定では、SDK が提供する[アーカイブ OS 種別](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iaas/ostype)を使います。

名前またはタグは OS 種別と併用できます。OS 種別から構築した検索条件のうち、名前の条件は `Names`、タグの条件は `Tags` の指定で上書きされます。その他の条件は残ります（[検索条件の変換](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/archive#FindRequest.ToRequestParameter)）。名前の検索は `Names` 配列を JSON で渡します。次を `archive-find.json` に保存し、`ZONE` とアーカイブ名を置き換えてください。

```json
{
  "Zone": "ZONE",
  "Names": ["ARCHIVE-NAME"]
}
```

```console
$ skr iaas-api archive find --request @archive-find.json --query 'map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})'
```

タグで検索する場合は `--tags` にカンマ区切りで指定するか、JSON の `Tags` 配列を使います。`--request` と個別フラグは併用できません。

### 3. アーカイブを ID で参照する

検索結果から確認した ID を使い、同じゾーンのアーカイブ情報を取得します。

```console
$ skr iaas-api archive read --zone ZONE --id ARCHIVE-ID
```

`ZONE` と `ARCHIVE-ID` は実際の値に置き換えます。検索結果と参照結果は JSON で出力されます。検索結果を必要なフィールドだけの配列で表示するには、`map({ID,Name,DiskPlanID,DiskPlanName,SizeMB,Tags})` を `--query` に指定します。参照結果を必要なフィールドだけのオブジェクトで表示するには、`{ID,Name,DiskPlanID,DiskPlanName,SizeMB,Tags}` を指定します。

## クリーンアップ

この手順は読み取り専用の検索・参照です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: アーカイブ](https://manual.sakura.ad.jp/cloud/storage/archive.html)
- [さくらのクラウドマニュアル: ディスクの作成/削除/変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.0: Archive API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/archive)
- [sacloud-sdk-go v0.3.0: Archive OS types](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iaas/ostype)
