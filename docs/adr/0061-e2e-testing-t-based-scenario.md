# ADR 0061: E2E テストを testing.T ベースのシナリオテストで書く

- Status: Accepted
- Date: 2026-10-27

## Context

[ADR 0060](0060-e2e-runner-shared-library.md) で E2E runner を共有ライブラリ化し、シナリオを scenario ファイルに分離しました。しかし runner は `main.go` と flag 解析、独自の後片付けを伴い、テスト実行の仕組みが skr 固有のままでした。通常の `go test` から実行できず、`go test -run` による個別実行や fail-fast といった標準的な仕組みも使えませんでした。

c0nfig の E2E テストを testing.T ベースで書き直した結果、標準の `go test` でシナリオを実行できる方が簡潔でした。

## Decision

- E2E シナリオは `test/e2e/<service-or-resource>/scenario_<snake_case>_test.go` に testing.T ベースのテスト関数として書きます。
- 共有の helper (`test/e2e/internal/helper`) で skr バイナリを実行し、実行結果は Markdown で `e2e-output/` 配下に保存します。
- `helper.New` は `SKR_E2E` 環境変数が未設定のときにテストをスキップするため、通常の `go test` では実環境を変更しません。ライブ実行は明示的に `SKR_E2E` を設定して行います。
- helper は `SAKURA_PROFILE_DIR` をテスト用の一時ディレクトリに分離するため、ライブ実行の認証は `skr` が対応する `SAKURA_ACCESS_TOKEN` / `SAKURA_ACCESS_TOKEN_SECRET` 環境変数を使います。
- 途中で失敗した場合の後片付けはテスト関数内の `defer` で行います。`t.Cleanup` はテスト終了後に `t.Context()` がキャンセルされた状態で呼ばれるため、CLI 呼び出しを伴う後片付けは `defer` で実行します。
- ADR 0060 の共有ライブラリ (`test/e2e/internal/e2e`) は本方針に置き換えるため廃止します。
- 既存 runner の一括移行は行わず、各サービスの作業時に段階的に移行します。移行しない runner の挙動は変更しません。
- [ADR 0058](0058-live-e2e-command-coverage.md) の配置・命名ルールは変更しません。`scenario_<snake_case>_test.go` は runner のシナリオ別実装と同名の単体テストという位置づけから、シナリオ実装そのものに変わります。

## Consequences

- シナリオは標準の `go test` で実行でき、`go test -run` で個別のシナリオを実行できます。fail-fast や並列実行など `testing` パッケージの仕組みを使えます。
- シナリオの流れがテスト関数とその呼び出しに集まるため、シナリオファイルだけを読めば実行されるステップと確認内容が分かります。
- 失敗時の後片付けは `defer` で実行され、後片付けの失敗もテストの結果に反映されます。
- helper の `Call` は失敗時に `t.Fatal` で停止するため、後片付けの途中で CLI 呼び出しが失敗すると、残りの後片付けは実行されません。失敗したこと自体はテスト結果と証跡で確認できます。
- 認証情報を環境変数で渡すため、プロファイルを用いたライブ実行から環境変数ベースの実行に切り替わります。

## Alternatives considered

- ADR 0060 の runner 共有ライブラリ (`test/e2e/internal/e2e`) を維持する案は採用しません。runner は `main.go` と flag 解析を runner ごとに必要とし、`go test` の標準的な仕組みから外れるためです。
- 後片付けを `t.Cleanup` に登録する案は採用しません。`t.Cleanup` の実行時点では `t.Context()` がキャンセル済みのため、helper による CLI 呼び出しが失敗するためです。
- 証跡を従来の evidence Recorder (`REPORT.md` とステップ名) に合わせる案は採用しません。helper の Markdown レポートに説明単位で統一する方が、呼び出し元と証跡の対応が単純になるためです。
