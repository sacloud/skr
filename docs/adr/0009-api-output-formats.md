# ADR 0009: API コマンドの出力形式を選択可能にする

- Status: Accepted
- Date: 2026-10-05

## Context

出力形式の選択は [ADR 0030](0030-remove-table-output.md) で置き換えました。以下は導入時の決定です。

IaaS API と EventBus API の結果は JSON のみで出力されます。プロファイル共通仕様では CLI の既定出力形式として table、JSON、YAML を定義しており、特に一覧結果を見やすくする table と、設定ファイルでの既定値指定が必要です。

## Decision

- API コマンドにグローバルな `--output json|yaml|table` を追加します。JSON を既定形式として維持し、CLI フラグがプロファイル設定より優先されます。
- 選択中プロファイルの v1 `cli.default_output_type` と v0 `DefaultOutputType` を読み取ります。いずれも未設定の場合は JSON を使い、未知の形式や不正な設定はエラーにします。
- JSON と YAML は SDK の返却データの形状を維持します。table は端末幅に合わせてトップレベル項目の列幅を調整し、ID、Name、Status、Description など識別や状態確認に重要な項目を前方に配置します。幅に収まらない場合は優先度の低い列を省略し、省略数を表示します。
- `--zone all` の table 表示に限り、結果の各行に Zone 列を先頭追加します。JSON/YAML の既存出力には Zone プロパティを追加しません。

## Consequences

- スクリプトとの既存連携に影響しないよう、未設定時の形式は JSON のままです。
- プロファイルの形式指定は API コマンドの結果表示に適用します。`config current` など、API 結果ではない出力形式は変更しません。
- table の列数は端末幅に応じて調整します。優先項目は先頭にまとまり、その他の列は名前順に表示します。長い値は省略して幅からはみ出さないようにします。
- `--zone all` の Zone 列は表示専用であり、JSON/YAML のスキーマを変えません。

## Alternatives considered

- table を既定値にする方法: 既存の JSON 出力を利用するスクリプトを壊すため採用しません。
- `--output` のみを追加し、プロファイル既定値を無視する方法: 指定がない場合も形式を統一したいというプロファイル共通仕様に合わないため採用しません。
- 全ゾーン検索の Zone を API 結果データに含める方法: JSON/YAML の既存スキーマを変えるため採用しません。
