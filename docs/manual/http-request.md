# 認証付き HTTP リクエスト

`skr http` は、SDK のプロファイルまたは `SAKURA_ACCESS_TOKEN` と
`SAKURA_ACCESS_TOKEN_SECRET` 環境変数を使って、指定した HTTPS URL にリクエストを送ります。
SDK でまだ利用できない API を直接呼び出す場合に使用できます。

このコマンドは指定した URL のホストへアカウントの認証情報を送信します。HTTPS であっても接続先の信頼性は保証されないため、さくらのクラウド API など信頼できるエンドポイントだけを指定してください。HTTP URL、URL 内のユーザー情報、フラグメントは指定できません。

## リクエスト

URL を位置引数で指定します。HTTP メソッドは `--method`（`-X`）で指定でき、省略時は `GET` です。

```console
$ skr http 'https://API-HOST/PATH'
$ skr http 'https://API-HOST/PATH' --method PATCH
```

`--data`（`-d`）で本文を指定できます。値をそのまま渡す方法、`@ファイルパス` でファイルを読み込む方法、`-` で標準入力から読む方法があります。必要な `Content-Type` などの HTTP ヘッダーは `--header`（`-H`）で指定します。ヘッダーは複数指定できます。

```console
$ skr http 'https://API-HOST/PATH' \
    --method POST \
    -H 'Content-Type: application/json' \
    --data @request.json
$ cat request.json | skr http 'https://API-HOST/PATH' --method POST --data -
```

`skr http` は、既定で `User-Agent: skr-http/v<version> (<os>/<arch>; sacloud-sdk-go/v<SDK version>)` を送信します。その他の API コマンドは `User-Agent: skr/v<version> (<os>/<arch>; sacloud-sdk-go/v<SDK version>)` を送信します。`<os>/<arch>` は実行ファイルのビルド対象 OS とアーキテクチャです。

`--header 'User-Agent: ...'` を指定すると、`skr http` の既定値を上書きできます。

コマンドライン引数はプロセス一覧やシェル履歴から見える場合があります。秘密情報を本文に含める場合は、保護したファイルまたは標準入力を使ってください。

## レスポンスとエラー

レスポンス本文を加工せず標準出力へ書き込みます。グローバル `--query` は本文を加工しません。HTTP ステータスが 2xx 以外の場合もレスポンス本文を出力したうえで、ステータスを標準エラーに表示してコマンドを失敗として終了します。成功時に本文が空であれば、何も出力しません。

## 参照先

- [さくらのクラウド API ドキュメント](https://developer.sakura.ad.jp/cloud/api/1.1/)
- [sacloud-sdk-go の認証付き HTTP クライアント](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go/common/saclient#Client.Do)
