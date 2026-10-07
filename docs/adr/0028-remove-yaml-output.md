# ADR 0028: YAML API 出力を廃止する

- Status: Accepted
- Date: 2026-10-07

## Context

[ADR 0009](0009-api-output-formats.md) で API コマンドに JSON、YAML、table の出力形式を導入しました。現在まで YAML 出力への需要は確認されていません。このため、利用者向け CLI と実装から YAML 出力を除きます。

## Decision

- API コマンドの出力形式は JSON と table に限定します。JSON は未設定時の既定形式として維持します。
- `--output yaml` は受け付けず、出力形式として `yaml` を設定したプロファイルもエラーにします。
- プロファイル設定ファイルの YAML 形式読み込みは維持します。廃止するのは API 結果の YAML 出力であり、YAML 形式の設定ファイルではありません。

## Consequences

- CLI ヘルプ、API 出力処理、テスト、利用者向け文書と設計文書は JSON と table に合わせます。
- `DefaultOutputType` または `cli.default_output_type` に `yaml` を設定しているプロファイルは、API コマンド実行時に未対応形式のエラーになります。
- YAML 出力の実装は削除しますが、プロファイル設定を読むための YAML パーサーは引き続き必要です。

## Alternatives considered

- YAML 出力をそのまま維持する方法: 需要がなく、機能を維持する価値がないため採用しません。
- プロファイルで `yaml` が選択されても JSON へフォールバックする方法: 不正な設定はエラーとして明示する既存方針を優先し、採用しません。
