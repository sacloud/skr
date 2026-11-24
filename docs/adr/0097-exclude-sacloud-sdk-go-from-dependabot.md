# ADR 0097: sacloud-sdk-go を dependabot の自動更新対象から外す

- Status: Proposed
- Date: 2026-11-21
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.1`

## Context

`sacloud-sdk-go` は、ogen 定義 (`api/<domain>/openapi/openapi.yaml`) とそこから生成されたコード、生成コードを包む手書きラッパー (`*Op` など)、SDK 内部ツール (`tools/*`) から構成されます。skr は SDK の生成コードを包む手書きコード (`internal/<domain>api/`、`internal/iaas/<resource>api/`)、生成コマンド、ユーザー向けドキュメントを持ち、API 定義の変更がこれらに波及します。

dependabot は gomod の更新を daily で提案します。しかし依存の自動更新は、変更内容の検討と skr 側への影響判断を経ずにマージされる恐れがあります。SDK の更新で API 定義が変わった場合は、次のような追従の判断が必要です。

- 生成コードの再生成 (`make generate-iaas-api` / `make generate-api` / `make generate-wiring`) の要否。
- 手書きコードが SDK の型を直接参照する箇所の追従。
- JSON 出力のフィールド名や型の変化に伴う、ヘルプ・チュートリアル・baseline テスト (`TestRequestCommandCoverageBaseline`、`test/tutorials/baseline.txt`) の見直し。
- ユーザー向け挙動が変わる場合のライブ E2E の再実行。

v0.7.1 (PR #293) では ogen 定義が無変更で、手書きラッパーへの nil 補完追加のみだったため追従作業は不要でした。この判別自体が、更新のたびに必要になる作業です。

## Decision

- dependabot の gomod 更新から `github.com/sacloud/sacloud-sdk-go` を除外します (`.github/dependabot.yml` の `ignore`)。他の依存と github-actions、docker の更新は自動提案を続けます。
- SDK の更新は手動で行い、[`sacloud-sdk-go-update` スキル](../../.agents/skills/sacloud-sdk-go-update/SKILL.md)の手順に従います。スキルは、変更内容の把握、層ごとの影響の分類、skr 側の追従判断、検証、記録を手順化します。

## Consequences

- SDK の更新は dependabot PR ではなく、このスキルに従った手動の PR になります。影響判断を書いた PR 説明が残ります。
- dependabot の自動更新がなくなるため、セキュリティ修正を含む SDK の更新も自動では届きません。スキルの手順に従い、定期的に SDK のリリースを確認して更新します。
- 除外の対象は `sacloud-sdk-go` だけです。`sakumock` など他の sacloud 関連依存は引き続き dependabot が提案します。`sakumock` で SDK の型が変わる変更が提案された場合は、このスキルの観点で評価します。

## Alternatives considered

- **dependabot を続けて、コメントだけで対応する**: 自動更新のマージが先に進むと、ogen 定義の変更を見落とす恐れがあります。レビュー時に毎回同じ判別作業をします。
- **グループ設定で sacloud 依存をまとめて自動更新する**: 依存の組み合わせはまとめられても、skr 側の追従判断は自動化されません。
- **cooldown で更新頻度を下げる**: 更新のタイミングを遅らせるだけで、影響判断を伴わないマージのリスクは変わりません。
