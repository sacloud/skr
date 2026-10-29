# ADR 0066: monitor-router を個別フラグで完結させる

- Status: Accepted
- Date: 2026-10-29

## Context

`skr iaas-api internet monitor-router` はルータ＋スイッチのトラフィックを指定期間で取得します。リクエストは Zone、ID、Start、End のスカラー 4 項目だけで有効に表現できますが、生成設定で `flags` を持たない JSON 入力専用 (`--request` のみ) になっていました。JSON 専用を正当化するネスト、配列、秘密情報はなく、生成器に日時フラグの前例がないという型の都合だけが根拠でした。他の internet 操作と入力経路が不揃いです。

## Decision

- `monitor-router` に `--zone` (必須) と `--id` (必須、`types.ID` 変換) を追加します。他の internet 操作と同じ必須指定です。
- `monitor-router` に `--start` と `--end` (RFC3339 形式の日時) を追加します。いずれも任意で、省略時は SDK の既定 (終了が現在時刻、開始がその 1 時間前) に従います。
- 生成器のフラグ型に `time.Time` を追加します。Kong が RFC3339 形式でデコードし、形式が不正な値は CLI の解析エラーとして拒否します。生成コードに標準ライブラリの `time` インポートは、`time.Time` フラグを持つ設定だけ含めます。
- `time.Time` フラグは API リクエストの単純なスカラー日時項目に限定します。配列、map、union、nullable 値は JSON 経路に残します。
- JSON 入力 (`--request`) を維持し、個別フラグとの併用は既存の規約 (ADR 0007) どおり拒否します。E2E には新しい個別フラグを使います。

## Consequences

モニタリングの手順で JSON リクエストが不要になり、internet 操作の入力経路が揃います。Start と End を省略した場合は直近 1 時間のモニタ値を取得します。生成器・生成物・ヘルプ・入力処理のテストに `time.Time` フラグの正常系とエラー系を追加します。

## Alternatives considered

- Start と End を必須にする方法は、API と SDK が省略時の既定期間を用意するため採用しません。
- `--start` と `--end` を文字列フラグにして手書き変換で日時へパースする方法は、Kong の `time.Time` デコードで足りるため採用しません。
- RFC3339 以外の日時形式を受け付ける方法は、API の入力形式と一致しないため採用しません。
