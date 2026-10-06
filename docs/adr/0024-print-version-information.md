# ADR 0024: CLI からバージョン情報を表示する

- Status: Accepted
- Date: 2026-10-06

## Context

リリースバイナリには GoReleaser がアプリケーションのバージョンとコミットの短縮ハッシュを埋め込みますが、CLI からそれらを確認するコマンドがありませんでした。

## Decision

- `skr version` サブコマンドと `skr --version` フラグを提供します。
- どちらも `version` パッケージの共通フォーマットを使い、バージョン、実行 OS・CPU、コミットの短縮ハッシュを標準出力に表示します。
- `--version` の処理には Kong の `VersionFlag` を使い、GoReleaser が埋め込むバージョン情報を渡します。

## Consequences

利用者は CLI からビルド情報を確認できます。開発ビルドでは、ビルド時に値を設定しない限り `version` パッケージに定義された既定値が表示されます。

## Alternatives considered

- 片方のインターフェースだけを提供する方法: どちらの呼び出し方にも対応できるよう、サブコマンドとフラグを併せて提供します。
