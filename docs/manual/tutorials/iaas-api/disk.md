# IaaS Disk API: ディスクを作成・確認・削除する

[ディスク](https://manual.sakura.ad.jp/cloud/storage/disk.html)は、サーバ作成時に同時に作成するほか、単独で作成して後からサーバへ接続できます。このチュートリアルでは `is1b` ゾーンにサーバ未接続のディスクを作成し、検索、参照、名前の更新、削除までを扱います。サーバへの接続やディスク内のデータ操作は行いません。

> [!WARNING]
> ディスク作成・更新・削除は選択中のプロジェクトに影響します。対象プロファイル、プロジェクト、ゾーン、ディスクプラン、サイズを確認し、必要な承認を得てから実行してください。ディスクの料金はプラン、サイズ、ゾーンによって異なります。削除したディスクは復旧できません（[ディスクの作成/削除/変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)）。

## 前提条件

- `skr iaas-api disk` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。
- 対象プロジェクトへの認証情報を SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。ディスクの作成・更新・削除に必要な権限も必要です。認証情報をコマンドや JSON ファイルに記載しないでください。
- `is1b` で作成可能な SSD プランとサイズを確認できること。このチュートリアルでは `DiskPlanID: 4`、`SizeGB: 20` を使用します。ディスクサイズはプランやゾーンによって異なります。別のゾーンやプランを使う場合は、その環境で利用できる値に置き換えてください。
- `TEST-DISK-NAME` と `UPDATED-TEST-DISK-NAME` を、対象プロジェクト内で重複しないテスト専用の名前に置き換えること。後述の `123456789012` は仮の ID です。必ず作成結果の `ID` に置き換えてください。

## 作成するリソースと順序

作成するのは `is1b` の SSD プランであるディスク1個です。`Connection` は `virtio`、サイズは 20 GB とし、サーバには接続しません。SDK の `DiskPlanID`、`Connection`、`SizeGB` はディスク作成リクエストのフィールドです（[Disk API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/disk)、[DiskPlan API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/diskplan)）。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api disk --help
$ skr iaas-api disk find --help
$ skr iaas-api disk read --help
$ skr iaas-api disk create --help
$ skr iaas-api disk update --help
$ skr iaas-api disk delete --help
```

意図したプロファイル、プロジェクト、認証情報を確認します。作成と更新では単純な値を個別フラグで指定でき、複雑な値は `--request` にインライン JSON または `@ファイル名` で指定できます。両方の入力経路は併用できません。作成の `DistantFrom` と `EditParameter`、更新の `EditParameter` は JSON で指定します。タグは個別フラグではカンマ区切り、JSON では `Tags` の文字列配列で指定できます。API の結果は JSON で出力し、`--query` で必要な項目を抽出できます。

検索では対象のゾーンを明示してください。

## Step 2: ディスク作成の入力を用意する

`TEST-DISK-NAME` を対象プロジェクト内で重複しないテスト専用名に置き換えてください。`DiskPlanID` 4 は `is1b` の SSD プラン、`SizeGB` 20 はそのプランで利用できるサイズです。これらの値は対象ゾーンとプランに合わせてください。

単純な入力には個別フラグを使えます。`Zone`、`Name`、`DiskPlanID`、`Connection`、`SizeGB` は必須です。`Tags` や `Description` などの独立したスカラー値も指定できます。

```console
$ skr iaas-api disk create \
    --zone is1b \
    --name TEST-DISK-NAME \
    --disk-plan-id 4 \
    --connection virtio \
    --size-gb 20 \
    --tags tutorial
```

同じ値は JSON リクエストでも指定できます。JSON の `Tags` は文字列配列です。複雑な入力が必要な場合は JSON ファイルを使ってください。

次の内容を `disk-create.json` として保存してください。
```json
{
  "Zone": "is1b",
  "Name": "TEST-DISK-NAME",
  "DiskPlanID": 4,
  "Connection": "virtio",
  "SizeGB": 20,
  "Tags": ["tutorial"]
}
```

この例ではディスクの内容のコピー元やサーバ ID を指定していないため、サーバ未接続のディスクを作成します。個別フラグと JSON は同じリクエストを組み立てますが、1回のコマンドでは併用できません。作成時の `DistantFrom` と `EditParameter` は JSON で指定します。既存ディスクやアーカイブからコピーする場合は、`disk create --help` にある `SourceDiskID` または `SourceArchiveID` などを指定してください。コピー元、サイズ、プランの組み合わせは作成前に確認してください。

## Step 3: ディスクを作成し、検索・参照する

Step 2 のフラグ例、または次の JSON 入力例のどちらか一方を使います。
```console
$ skr iaas-api disk create --request @disk-create.json
```

成功すると Disk の情報が出力されます。2つの入力例からは同じフィールドを持つリクエストが作られます。以降の操作で使う `ID` を控えてください。`123456789012` は仮の値です。作成結果の ID に置き換えて検索・参照してください。

```console
$ skr iaas-api disk find --zone is1b --name TEST-DISK-NAME --tags tutorial --query 'map({ID,Name,SizeMB})'
$ skr iaas-api disk find --zone is1b --tags tutorial --query 'map({ID,Name,SizeMB})'
```

`--name` は単一名の部分一致検索です。カンマを含む値も1つの名前として扱います。上の1行目のように名前とタグの両方を個別フラグで指定できます。複数名の検索条件は `--request` の `Names` 配列を使います。JSON では `Tags` も文字列配列で指定し、個別フラグとは併用できません。

次は CLI テストで確認した出力形式の例です。ID と名前は置き換えています。

```json
[
  {
    "ID": 123456789012,
    "Name": "TEST-DISK-NAME",
    "SizeMB": 20480
  }
]
```

```console
$ skr iaas-api disk read --zone is1b --id 123456789012
```

検索結果と参照結果の `ID`、`Name`、`SizeMB` が作成したディスクに一致することを確認してください。

## Step 4: 名前を更新する

ID と `UPDATED-TEST-DISK-NAME` を実際の値に置き換えて、名前をフラグで更新します。更新する項目だけを指定し、省略した項目は変更しません。

```console
$ skr iaas-api disk update --zone is1b --id 123456789012 --name UPDATED-TEST-DISK-NAME
```

JSON を使う場合は、次の内容を `disk-update.json` として保存します。タグなどの配列を更新する場合も JSON を利用できます。

```json
{
  "Zone": "is1b",
  "ID": 123456789012,
  "Name": "UPDATED-TEST-DISK-NAME"
}
```

JSON 入力を使う場合は、次のコマンドで更新します。
```console
$ skr iaas-api disk update --request @disk-update.json
```

フラグ例または JSON 入力例のどちらかで更新した後、名前が変わったことを次のコマンドで確認します。
```console
$ skr iaas-api disk read --zone is1b --id 123456789012
```

再度参照し、`Name` が更新後の値であることを確認してください。

## Step 5: ディスクを削除する

削除前に `read` で ID と名前を確認してください。サーバに接続中のディスクは削除できません。削除は復旧できないため、対象を取り違えないようにしてください。

```console
$ skr iaas-api disk delete --zone is1b --id 123456789012 --fail-if-not-found
$ skr iaas-api disk find --zone is1b --name UPDATED-TEST-DISK-NAME --query 'map({ID,Name})'
```

削除成功時、`delete` は標準出力に何も出しません。検索結果に削除した ID が含まれないことを確認してください。

```json
[]
```

## 参考情報

- [さくらのクラウドマニュアル: ディスクの作成/削除/変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.0: Disk サービスとリクエスト型](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/disk)
- [sacloud-sdk-go v0.3.0: DiskPlan サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/diskplan)
