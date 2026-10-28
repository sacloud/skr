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

## 読み取り専用 E2E

`test/e2e/http` は、設備関連 API の `GET /zone` を `skr http` 経由で1回呼び出し、終了コードと JSON 本文を確認します。ライブ E2E の対象ゾーンに合わせ、URL は `https://secure.sakura.ad.jp/cloud/zone/is1b/api/cloud/1.1/zone` とします。API のリクエスト／レスポンス形状を独自に再実装せず、生のレスポンスが空でなく有効な JSON であることだけを検証します。

シナリオは `test/e2e/http/scenario_http_test.go` にあり、testing.T ベースのテストとして `SKR_E2E` 環境変数が設定された場合にだけ実行します ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。実行前のプロファイル確認は skr CLI で実行して記録に残します。ライブ実行の認証はプロファイルディレクトリの `skr-e2e` プロファイルを使い、認証情報を表示しません。テストはリソースを作成・変更・削除しませんが、選択中のプロファイルの認証情報を `secure.sakura.ad.jp` へ送信します。実行記録は helper が Git 対象外の `test/e2e/http/e2e-output/` 配下に保存します ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。2026-10-28 にシナリオをライブ実行し、zone API の GET リクエストが有効な JSON 応答を返すことを確認しました。

```console
$ make build
$ SKR_E2E=1 go test ./test/e2e/http/ -run TestScenarioHTTP -v
```

ユニットテストでは live API に接続せず、応答の JSON 検証を確認します。コマンド引数とプロファイル確認の追跡は、シナリオのコードと helper のテストで行います。
