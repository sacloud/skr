# IaaS DiskPlan API: ゾーン別のディスクプランとサイズを確認する

DiskPlan API で対象ゾーンのディスクプラン、利用可否、選択可能なサイズを確認できます。ディスクプランや作成先ゾーンによって選択可能なディスクサイズが異なるため、ディスクを作成する前にこの情報を確認します。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。
- 検索するゾーン名が分かること。参照するディスクプラン ID は検索結果から取得します。

## 手順

この手順は読み取り専用です。クラウドリソースは作成、変更、削除しません。

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api product disk --help
$ skr iaas-api product disk find --help
$ skr iaas-api product disk read --help
```

### 2. 対象ゾーンのプランとサイズを検索する

```console
$ skr iaas-api product disk find --zone ZONE --query '.DiskPlans | map({ID,Name,StorageClass,Availability,Size})'
```

`ZONE` は検索対象のゾーンに置き換えます。Find の JSON は API 応答の `Total`、`From`、`Count`、`DiskPlans` を含みます。`Availability` はプランの利用可否を示し、`Size` には各サイズの利用可否、表示サイズ、単位、MB 単位の値が含まれます。プランやゾーンによって選択可能なディスクサイズが異なります。`--query` で `DiskPlans` 配列を取り出して表示しています。

特定のプラン名で絞り込む場合は、API の `Filter` を JSON で指定します。次を `disk-plan-find.json` に保存し、`ZONE` とプラン名を置き換えてください。

```json
{
  "Zone": "ZONE",
  "Filter": {
    "Name": "PLAN-NAME"
  }
}
```

`Filter` の文字列値は部分一致で検索し、配列値は OR 条件として検索します。

```console
$ skr iaas-api product disk find --request @disk-plan-find.json --query '.DiskPlans | map({ID,Name,StorageClass,Availability,Size})'
```

`--request` と個別フラグは併用できません。

### 3. ディスクプランを ID で参照する

検索結果から確認した ID を使い、同じゾーンのプラン情報を取得します。

```console
$ skr iaas-api product disk read --zone ZONE --id DISK-PLAN-ID
```

`ZONE` と `DISK-PLAN-ID` は実際の値に置き換えます。検索結果と参照結果は JSON で出力されます。必要な項目だけを表示するには、`--query` を指定します。

## クリーンアップ

この手順は読み取り専用の検索・参照です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: ディスクの作成/削除/変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.1: DiskPlan API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#DiskPlanAPI)
- [sacloud-sdk-go v0.3.1: DiskPlan response model](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#DiskPlanFindResult)
