# 認証付き HTTP リクエストコマンドの設計

- Status: Accepted
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`
- ADR: [0018](../adr/0018-authenticated-http-command.md)

## コマンド

```text
skr http <url> [--method <method>] [-H, --header "name: value"]... [--data <value|@path|->]
```

`<url>` は絶対 HTTPS URL です。URL 内のユーザー情報とフラグメントは拒否します。`--method`（`-X`）は省略時 `GET` です。HTTP ヘッダーは `--header`（`-H`）で複数指定できます。`--data`（`-d`）はインラインの文字列、`@path` で読み込むファイル、`-` で読む標準入力です。

## 認証とリクエスト

CLI は sacloud-sdk-go `v0.3.0` の `saclient.Client` を初期化し、その公開 `Do` メソッドに標準の `net/http.Request` を渡します。プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数による認証を再利用し、個別 API の SDK 型・操作・URL 組み立ては使用しません。

既定の `User-Agent` は `skr-http/v<version> (<os>/<arch>; sacloud-sdk-go/v<SDK version>)` とします。CLI と SDK のバージョンに加え、`skr http` 経由のリクエストであることを識別できます。OS・アーキテクチャには実行時環境の情報ではなく、Go が実行ファイルに埋め込む `runtime.GOOS` と `runtime.GOARCH` を使います。利用者が `--header 'User-Agent: ...'` を指定した場合はその値を優先し、空の値ならヘッダーを送信しません。

このリクエスト経路は指定した HTTPS ホストに SDK の認証情報を送ります。HTTPS URL に限定しても、ホストの信頼性は保証できません。ヘルプとドキュメントで接続先確認を促し、利用者の明示したホストを自動書き換えしません。

## レスポンスとエラー

HTTP レスポンス本文はバイト列のまま標準出力へコピーします。出力形式の変換、改行の追加、ヘッダーやステータスの標準出力への混入は行いません。2xx 以外のステータスでは、本文を出力してからステータスを含むエラーを返します。通信、リクエスト構築、本文読み込み、本文書き込みのエラーも呼び出し元へ返します。

## テスト

HTTP コマンドのテストでは、偽の request doer によるメソッド、URL、ヘッダー、本文、レスポンス保持、ステータスエラーを検証します。SDK のクライアントは TLS テストサーバーを用いて、CLI 経路から認証ヘッダーが付くことも検証します。URL 検証、本文ファイル／標準入力、ヘッダー検証、`--help` も対象にします。

## キャンセルと期限

`skr http` は実行中に SIGINT と SIGTERM を受けて、実行中のリクエストを context のキャンセルで中断します。シグナル待ちはこのコマンドの実行中に限るため、他のコマンドの終了動作は変更しません。中断されたリクエストは、送信、応答の受信、本文の読み込み、リトライの待ち時間のすべてで停止し、`(interrupted by signal)` を含むエラーで終了します。決定は [ADR 0080](../adr/0080-http-command-signal-cancellation.md) を参照してください。

リクエストの期限は sacloud-sdk-go が管理します。調査時点の v0.5.0 では、`saclient.Client` は `http.Client.Timeout` に次の優先順位で値を設定します。

1. `SetWith` によるランタイム設定 (skr は使用しない)
2. コマンドライン引数 (skr は使用しない)
3. 環境変数 `SAKURA_API_REQUEST_TIMEOUT`、次に `SAKURACLOUD_API_REQUEST_TIMEOUT` (秒)
4. プロファイル設定の `api_request_timeout` (秒)。プロファイルに `0` が明記されている場合は `0` が採用され、タイムアウトが無効になる
5. SDK 既定の 300 秒

`http.Client.Timeout` は 1 回の試行の接続、送信、応答受信、本文の読み込みまでに適用されます。リトライは `retryablehttp` が各試行で `http.Client.Do` を呼び出すため、期限は試行ごとに適用され、リトライ全体の期限にはなりません。リトライ回数と待ち時間は `SAKURA_RETRY_MAX`、`SAKURA_RETRY_WAIT_MIN`、`SAKURA_RETRY_WAIT_MAX` (または `SAKURACLOUD_` 接頭辞) で変更できます。SDK v0.5.0 の既定は、リトライ待ち 1 秒、リトライ待ち上限 64 秒、リトライ回数 10 回です。リトライ待ちもリクエストの context を見るため、キャンセルすると待ち時間を待たずに中断します。プロファイルの設定ファイルに `0` が明記されている場合も有効値として採用されるため、リトライなし・タイムアウトなしで動作します。

CLI 側で全体期限 (リトライ全体を含む上限) は設けません。全体期限は SDK の設定を上書きする固定値になるため、必要になった場合は別の設定経路と既定値を検討します。

### テスト

- 単体テストでは、context の差し替えでキャンセル済み context がリクエストへ伝播することと、`(interrupted by signal)` のエラーを確認します。
- Unix のテストでは実際に SIGINT を送り、本文の読み込み中の中断と、リトライ待ち時間中の中断 (待ち時間を待たずに終了し、リトライが発生しないこと) を確認します。テストは `SAKURA_PROFILE_DIR` を一時ディレクトリに切り離し、プロファイルの設定 (`RetryMax: 0` のように 0 が明記された設定によるリトライ無効など) の影響を受けないようにします。

## 読み取り専用 E2E

`test/e2e/http` は、設備関連 API の `GET /zone` を `skr http` 経由で1回呼び出し、終了コードと JSON 本文を確認します。ライブ E2E の対象ゾーンに合わせ、URL は `https://secure.sakura.ad.jp/cloud/zone/is1b/api/cloud/1.1/zone` とします。API のリクエスト／レスポンス形状を独自に再実装せず、生のレスポンスが空でなく有効な JSON であることだけを検証します。

シナリオは `test/e2e/http/scenario_http_test.go` にあり、testing.T ベースのテストとして `SKR_E2E` 環境変数が設定された場合にだけ実行します ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。ライブ実行の認証はプロファイルディレクトリの `skr-e2e` プロファイルを使い、認証情報を表示しません。テストはリソースを作成・変更・削除しませんが、プロファイルの認証情報を `secure.sakura.ad.jp` へ送信します。実行記録は helper が Git 対象外の `test/e2e/http/e2e-output/` 配下に保存します ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。2026-10-28 にシナリオをライブ実行し、zone API の GET リクエストが有効な JSON 応答を返すことを確認しました。

```console
$ make build
$ SKR_E2E=1 go test ./test/e2e/http/ -run TestScenarioHTTP -v
```

ユニットテストでは live API に接続せず、応答の JSON 検証を確認します。コマンド引数の追跡は、シナリオのコードと helper のテストで行います。
