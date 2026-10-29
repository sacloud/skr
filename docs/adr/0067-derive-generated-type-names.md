# ADR 0067: 生成型名の設定宣言を廃止して導出に統一する

- Status: Accepted
- Date: 2026-10-29

## Context

生成設定 (`api/commands/*.json`) はトップレベルの `command_type` / `api_type` / `factory_type` / `runtime_type` と操作ごとの `command_type` を必須で宣言します (ADR 0016 / 0019)。IaaS の 15 設定では 4 つの型名がすべて `Commands` / `API` / `APIFactory` / `Runtime` で同一です。Ogen 系の 30 設定では `<接頭辞>Commands` / `<接頭辞>API` / `<接頭辞>APIFactory` / `<接頭辞>Runtime` と、同じ接頭辞を 4 フィールドで繰り返し宣言します。操作ごとの型名も IaaS は `<操作名>Command`、Ogen 系は `<接頭辞><操作名>Command` で、package 名・接頭辞・操作名から機械的に導出できる冗長な宣言です。

冗長な宣言は乖離を生みます。実際に `iam-scim.json` は `SCIMCommands` と `ScimAPI` / `ScimAPIFactory` / `ScimRuntime` で表記が分かれています。設定検証は型名の形式と生成型名全体の衝突を検査しますが、宣言同士の一致までは保証しません。

## Decision

- トップレベルの 4 つの型名フィールドと操作ごとの `command_type` を廃止します。
- IaaS は生成器が固定名 `Commands` / `API` / `APIFactory` / `Runtime` と操作ごとの `<操作名>Command` を出力します。
- Ogen 系は設定に 1 つの接頭辞 (例: `command_prefix`) を宣言し、`<接頭辞>Commands` / `<接頭辞>API` / `<接頭辞>APIFactory` / `<接頭辞>Runtime` と操作ごとの `<接頭辞><操作名>Command` を導出します。複数リソースが 1 つのドメイン package を共有するため、接頭辞は package 内の識別子として明示的に宣言します。
- 接頭辞を設定ファイル名から導出しません。設定ファイル名には `c0nfig` 綴りの規則を避けるための表記 (例: `eventbus-process-c0nfiguration.json`) が含まれ、正しい型名になりません。
- 頭字語を含む操作名 (例: `ReadIpv6`) の生成型名は導出規則に従います。利用者向けコマンド名は `command_name` (ADR 0048) で指定するため、生成型名の表記変更は CLI の公開名に影響しません。
- SCIM の表記ゆれは接頭辞 1 つに統一します。生成型名の変更は internal/cli の配線とテストの参照更新を伴いますが、usacloud との互換性は保証されない方針 (ADR 0001) に従います。
- 設定検証は接頭辞が Go の識別子として有効な表記であることを検査し、導出された生成型名全体の衝突を拒否します (現行の衝突検査を維持します)。

## Consequences

45 設定ファイルがスリムになり、宣言と実際の型の乖離が構造的に起こらなくなります。生成型名の表記ルールが生成器に一本化されます。既存 45 ファイルの一括更新と再生成、internal/cli とテストの参照更新 (SCIM 型名の変更など) が必要です。生成器の設定解析・検証とそのテストも更新します。

## Alternatives considered

- 省略時デフォルトと明示的上書きを残す方法は、上書き経路が乖離を再び生むため採用しません。
- 接頭辞を設定ファイル名から導出する方法は、`c0nfig` 綴りの規則と衝突して誤った型名を生むため採用しません。
- 現状維持は、冗長な宣言と乖離リスクが残るため採用しません。
