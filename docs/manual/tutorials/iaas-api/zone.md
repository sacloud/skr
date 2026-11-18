# IaaS Zone API: ゾーン一覧とゾーン情報を参照する

`skr iaas-api zone` は IaaS API からゾーン一覧とゾーン情報を取得します。このチュートリアルでは、JSON の一覧を確認し、`--query` でゾーン名だけを取り出し、指定した ID のゾーン情報を参照します。ゾーン ID は `zone find` の結果から `--query` で取り込み、シェルの変数に設定して使います。

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
$ skr iaas-api zone read --help
```

### 2. ゾーン一覧を JSON で確認する

```console
$ skr iaas-api zone find
```

表示されるゾーンは利用環境によって異なります。必要な項目だけを確認するには、次のクエリを使います。

### 3. jq 式でゾーン名を取り出す

```console
$ skr iaas-api zone find --query 'map(.Name)'
```

`map(.Name)` は取得結果の各要素から `Name` を選び、名前の配列を JSON で出力します。次は CLI テストで確認した出力例です。名前はテスト用の値に置き換えています。

```json
[
  "test-zone-a",
  "test-zone-b"
]
```

jq 式は skr に組み込まれた gojq で評価するため、外部の jq コマンドは不要です。

### 4. ID でゾーン情報を参照する

`zone read` に `--id` でゾーン ID を指定すると、そのゾーンの情報を取得します。ゾーン ID は環境によって異なるため、`zone find` の結果から `--query` で ID を取り込み、変数に設定します。ここでは ID が最小のゾーンを選びます。

```console
$ ZONE_ID=$(skr iaas-api zone find --query 'sort_by(.ID) | .[0].ID')
$ skr iaas-api zone read --id "$ZONE_ID"
```

応答には ID、表示順、名前、説明のほか、VNC プロキシや FTP サーバなどの接続情報が含まれます。必要な項目だけを確認するには、`--query` でオブジェクトを構築します。`read` の応答は配列ではなく単一オブジェクトのため、`map()` ではなく `{ID,Name}` の形式を使います。

```console
$ skr iaas-api zone read --id "$ZONE_ID" --query '{ID,Name,Region}'
```

`--id` と `--request` は併用できません。`ReadRequest` JSON を直接指定する場合は、変数を使って次のようにします。

```console
$ skr iaas-api zone read --request "{\"ID\":$ZONE_ID}"
```

## クリーンアップ

この手順は読み取り専用の参照です。削除するリソースはありません。

## 参考資料

- [さくらのクラウド API v1.1: 設備関連 API の `get_zone`](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html#get_zone)
- [sacloud-sdk-go v0.7.0: IaaS Zone API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.7.0/service/iaas/zone)
- [jq マニュアル](https://jqlang.github.io/jq/manual/)
- [gojq の jq との差異](https://github.com/itchyny/gojq#differences-from-jq)
