# IaaS Zone API: ゾーン一覧を jq で加工する

`skr iaas-api zone find` は IaaS API からゾーン一覧を取得します。このチュートリアルでは、一覧を table で確認し、`--query` でゾーン名だけを取り出します。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。

## 手順

ゾーン一覧の取得だけを行い、クラウドリソースは作成・変更・削除しません。

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api zone --help
$ skr iaas-api zone find --help
```

### 2. ゾーン一覧を table で確認する

```console
$ skr iaas-api zone find --output table
```

表示されるゾーンは利用環境によって異なります。端末幅によっては、table の一部の列を省略して表示します。

### 3. jq 式でゾーン名を取り出す

```console
$ skr iaas-api zone find --query 'map(.Name)'
```

`map(.Name)` は取得結果の各要素から `Name` を選び、名前の配列を JSON で出力します。クエリを指定した場合、`--output` とプロファイル既定の出力形式は適用されず、結果を JSON で返します。jq 式は skr に組み込まれた gojq で評価するため、外部の jq コマンドは不要です。

## クリーンアップ

この手順は読み取り専用の一覧取得です。削除するリソースはありません。

## 参考資料

- [さくらのクラウド API v1.1: 設備関連 API の `get_zone`](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html#get_zone)
- [sacloud-sdk-go v0.3.0: IaaS Zone API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/zone)
- [jq マニュアル](https://jqlang.github.io/jq/manual/)
- [gojq の jq との差異](https://github.com/itchyny/gojq#differences-from-jq)
