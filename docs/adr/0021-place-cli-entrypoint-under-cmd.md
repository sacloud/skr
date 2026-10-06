# ADR 0021: CLI エントリーポイントを cmd/skr に配置する

- Status: Accepted
- Date: 2026-10-06

## Context

CLI の実装、補助コード、テストがリポジトリルートに並び、生成器や内部パッケージと一緒に見えるため、ルートディレクトリが整理されていません。実行可能ファイルのコードを `cmd/skr/` にまとめ、ルートをモジュール設定やプロジェクト資料の配置場所として整理します。

## Decision

- 実行可能ファイルの `main` を `cmd/skr/` に配置し、CLI の実装とテストを `internal/cli/` に配置します。
- Makefile の build、install、ドキュメント生成では `./cmd/skr` を CLI パッケージとして指定します。
- CLI の機能は引き続き `internal/` のパッケージを利用し、`cmd/skr/` からルートディレクトリの `package main` に依存する構成にはしません。

## Consequences

- Go の実装ファイルとテストがルートから `cmd/skr/` にまとまります。
- ルートに Go ソースはなくなり、CLI のビルドとインストールには `./cmd/skr` を指定します。`cmd/skr/main.go` は `internal/cli` の実行関数だけを呼び出します。
- CLI のコマンド全体を変更するときは `go test ./...` で `cmd/skr` のテストを含めて実行します。

## Alternatives considered

- `main.go` だけを移動する方法: 他の CLI 実装やテストは同じ `package main` であり、単一ファイルだけの移動ではパッケージが成立しないため採用しません。
- 実装の一部だけを `internal/` に切り出す方法: ルートの見通しを改善しつつ CLI 機能の構造まで変える必要はないため、今回は採用しません。
