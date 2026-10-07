# ADR 0018: 認証付き HTTP リクエストコマンドを `http` とする

- Status: Accepted
- Date: 2026-10-06

## Context

`--output` に関する説明は [ADR 0030](0030-remove-table-output.md) のフラグ削除により置き換えました。生のレスポンス本文を維持する方針は変更しません。

SDK がまだ公開していない API を呼び出す場合にも、skr のプロファイルや API キー認証を利用できる低レベルの入口が必要です。さくらのクラウド API の URL を直接指定し、SDK が提供する認証付き HTTP クライアントでリクエストを送ることで、SDK の操作モデルを介さず API を呼び出せます。

旧 usacloud の同様のコマンドは `rest` という名前です。この命名が Azure CLI の `az rest` を意識した可能性があるという経緯が伝えられていますが、ここでは未確認の背景情報として扱います。Azure CLI は REST API を呼び出すコマンドとして `az rest` を提供しています。一方、さくらのクラウドの API ドキュメントは「さくらのクラウド API」と呼称しており、この機能が対象とする URL も REST アーキテクチャであることを前提にしません。

## Decision

- コマンド名をトップレベルの `http` とし、`skr http <url>` で任意の HTTPS URL にリクエストを送ります。`rest` のように API の設計様式を示すのではなく、HTTP リクエストを直接送る機能であることを表します。
- 認証には sacloud-sdk-go `v0.3.0` の公開 `saclient.Client.Do` を使用します。SDK のプロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数を利用し、独自の認証処理は実装しません。
- メソッドは `--method`（`-X`）で指定し、省略時は `GET` とします。リクエスト本文は `--data`（`-d`）でインライン文字列、`@path` のファイル、または `-` の標準入力から受け取ります。任意の HTTP ヘッダーは繰り返し指定できる `--header`（`-H`）で受け取ります。
- URL は絶対 HTTPS URL に限定し、URL 内のユーザー情報とフラグメントを拒否します。指定されたホストに認証情報が送られることをヘルプと利用者向けドキュメントに明記します。
- レスポンス本文をそのまま標準出力へ書き込みます。JSON/YAML/table への変換は行わず、HTTP ステータスが 2xx 以外の場合は本文を出力した後にエラーを返します。

## Consequences

- SDK にまだ対応していない API も、API 仕様に沿った URL、メソッド、ヘッダー、本文を指定して呼び出せます。SDK の API モデルや個別サービスコマンドを追加せずに済みます。
- HTTPS は通信路を暗号化しますが、指定ホストが信頼できることまでは保証しません。利用者は認証情報を送信する接続先を確認する必要があります。
- 生のレスポンスを維持するため、`--output` は `http` のレスポンス形式に影響しません。エラー応答でも本文を標準出力に出すため、機械処理では終了コードと標準エラーも確認します。
- このコマンドは SDK の通常の API コマンドに代わるものではありません。SDK 対応済みの操作では、型付きリクエスト、サービス固有の検証、構造化出力を提供する既存コマンドを優先します。

## Alternatives considered

- `rest`: Azure CLI の `az rest` などの先例はありますが、REST であることを前提にしないため選びません。
- `api`: 既存の `iaas-api`、`eventbus-api`、`simplemq-api` と同じ API ドメインコマンドに見え、任意 URL に対する HTTP 呼び出しと区別しにくいため選びません。
- `request`: HTTP 以外の要求にも見える汎用名であり、実際の通信方法が分かりにくいため選びません。
- HTTP と HTTPS の両方を許可する方法: HTTP では認証情報が平文で送信されるため、採用しません。
- レスポンスを既存の `--output` 形式へ変換する方法: JSON 以外のレスポンスや API 固有の出力を壊すおそれがあるため、本文をそのまま出力します。

## 参照資料

- [Azure CLI `az rest` コマンド](https://learn.microsoft.com/ja-jp/cli/azure/use-azure-cli-rest-command?view=azure-cli-latest&tabs=bash)
- [さくらのクラウド API ドキュメント](https://developer.sakura.ad.jp/cloud/api/1.1/)
- [sacloud-sdk-go `saclient.Client.Do`](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go/common/saclient#Client.Do)
