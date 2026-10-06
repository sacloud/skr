# HTTP API: `get_zone` でゾーン一覧を取得する

`skr http` を使うと、sacloud-sdk-go の認証設定を利用して HTTPS URL に直接リクエストできます。このチュートリアルでは、設備関連 API の `get_zone` を呼び出し、ゾーン一覧を取得します。対象の API は読み取り専用です。

> [!WARNING]
> `skr http` は指定 URL のホストへアカウントの認証情報を送信します。この例の接続先が `secure.sakura.ad.jp` であることを確認し、別の URL を使う場合も信頼できるホストだけを指定してください。

## 前提条件

- `skr http` を含む `skr` バイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数が設定されていること。
- 選択中のプロファイルまたは API キーが、対象 API の読み取りリクエストを実行できること。認証情報をコマンド例や URL に記載しないでください。

この手順は GET リクエストだけを実行し、クラウドリソースを作成・変更・削除しません。したがって、チュートリアル用のリソースのクリーンアップはありません。

## Step 1: 認証設定とコマンドを確認する

```console
$ skr config current
$ skr --help
$ skr http --help
```

`config current` で対象プロファイルを確認します。環境変数で認証する場合は、対象プロジェクトのトークンとシークレットをシェル環境に設定してください。

## Step 2: ゾーン一覧を取得する

設備関連 API の `GET /zone` はゾーン一覧を取得します（[get_zone API](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html#get_zone)）。ドキュメントの呼び出し例に合わせ、URL の `is1a` を含むパスを指定します。

```console
$ skr http 'https://secure.sakura.ad.jp/cloud/zone/is1a/api/cloud/1.1/zone' --method GET
```

HTTP メソッドの既定値は `GET` なので、`--method GET` は省略できます。レスポンス本文はそのまま標準出力へ出力され、グローバル `--output` で JSON、YAML、table への変換は行いません。HTTP ステータスが 2xx 以外の場合も本文を出力し、ステータスを標準エラーに表示して失敗として終了します。実際のレスポンス構造は API ドキュメントで確認してください。

## 参考資料

- [さくらのクラウド API v1.1: 設備関連 API の `get_zone`](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html#get_zone)
- [sacloud-sdk-go v0.3.0: 認証付き HTTP クライアント `saclient.Client.Do`](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/common/saclient#Client.Do)
