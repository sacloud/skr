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

意図したプロファイル、プロジェクト、認証情報を確認します。ディスク作成と更新は SDK のリクエスト JSON を使います。JSON は `--request` に直接指定するか、`@ファイル名` で読み込めます。検索、参照、削除では個別フラグまたは JSON を使い、両方は併用できません。API の出力形式は `--output json` または `--output table` で選択できます。

`find --zone all` は全ゾーンを検索できます。`--count` と `--from` は各ゾーンの検索に適用されます。全ゾーン検索は個別フラグで指定するため、複雑な条件を含む `--request` JSON とは併用できません。

## Step 2: ディスク作成リクエストを用意する

次の内容を `disk-create.json` として保存し、`TEST-DISK-NAME` を重複しないテスト専用名に置き換えてください。`DiskPlanID` 4 は `is1b` の SSD プラン、`SizeGB` 20 はそのプランで利用できるサイズです。

```json
{
  "Zone": "is1b",
  "Name": "TEST-DISK-NAME",
  "DiskPlanID": 4,
  "Connection": "virtio",
  "SizeGB": 20
}
```

この例ではディスクの内容のコピー元やサーバ ID を指定していないため、サーバ未接続のディスクを作成します。既存ディスクやアーカイブからコピーする場合は、`disk create --help` にある `SourceDiskID` または `SourceArchiveID` などを JSON に加えてください。コピー元、サイズ、プランの組み合わせは作成前に確認してください。

## Step 3: ディスクを作成し、検索・参照する

```console
$ skr iaas-api disk create --request @disk-create.json
```

成功すると Disk の情報が出力されます。以降の操作で使う `ID` を控えてください。`123456789012` は仮の値です。作成結果の ID に置き換えて検索・参照してください。

```console
$ skr iaas-api disk find --request='{"Zone":"is1b","Names":["TEST-DISK-NAME"]}' --output table
$ skr iaas-api disk read --zone is1b --id 123456789012
```

検索結果と参照結果の `ID`、`Name`、`SizeMB` が作成したディスクに一致することを確認してください。

## Step 4: 名前を更新する

次の JSON を `disk-update.json` として保存し、ID と `UPDATED-TEST-DISK-NAME` を実際の値に置き換えてください。更新で省略した項目は変更されません。

```json
{
  "Zone": "is1b",
  "ID": 123456789012,
  "Name": "UPDATED-TEST-DISK-NAME"
}
```

```console
$ skr iaas-api disk update --request @disk-update.json
$ skr iaas-api disk read --zone is1b --id 123456789012
```

再度参照し、`Name` が更新後の値であることを確認してください。

## Step 5: ディスクを削除する

削除前に `read` で ID と名前を確認してください。サーバに接続中のディスクは削除できません。削除は復旧できないため、対象を取り違えないようにしてください。

```console
$ skr iaas-api disk delete --zone is1b --id 123456789012 --fail-if-not-found
$ skr iaas-api disk find --request='{"Zone":"is1b","Names":["UPDATED-TEST-DISK-NAME"]}' --output table
```

削除成功時、`delete` は標準出力に何も出しません。検索結果に削除した ID が含まれないことを確認してください。

## ライブ E2E の実行

リポジトリの E2E runner で、対象プロジェクトの `is1b` におけるディスク作成・更新・削除を検証できます。対象プロファイルとプロジェクトを確認してから実行してください。runner はランダムな名前のディスクだけを作成し、同名のディスクが既にある場合は中止します。実行にはディスク作成の承認が必要です。

```console
$ make build
$ ./skr config current
$ go run ./test/e2e/disk --skr ./skr --confirm-is1b-live
```

runner は検索、参照、作成、更新、削除と削除後の不在を確認します。実行の入出力は `tmp/disk-api/` にプライベートな証跡として保存されます。実際のリソース ID やアカウント情報を含むため、Git に追加したり他者へ共有したりしないでください。失敗時にクリーンアップできなかった場合は、標準エラーと証跡から対象を確認し、不要なディスクだけを削除してください。

## 参考情報

- [さくらのクラウドマニュアル: ディスクの作成/削除/変更](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.0: Disk サービスとリクエスト型](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/disk)
- [sacloud-sdk-go v0.3.0: DiskPlan サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/diskplan)
