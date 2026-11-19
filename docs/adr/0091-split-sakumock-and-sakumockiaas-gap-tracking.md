# ADR 0091: sakumock と sakumockiaas の差分管理を分離する

- Status: Accepted
- Date: 2026-11-19

## Context

[ADR 0090](0090-sakumock-gap-tracking.md) で、sakumock に関する既知の差分・未整備を [docs/design/sakumock-gaps.md](../design/sakumock-gaps.md) の 1 ファイルに集約した。しかし集約対象には、管理主体が異なる 2 種類のモックが含まれていた。

| 名称 | 場所 | 差分の管理主体 |
| --- | --- | --- |
| sakumock | `github.com/sacloud/sakumock` (外部ライブラリ) | upstream (sacloud/sakumock) の issue で修正を追跡する |
| sakumockiaas | `internal/sakumock/iaas` (リポジトリ内実装) | リポジトリ側の作業として追跡する |

1 ファイルに混在すると、Workflows の `list-suggestions` の `Count` 欠落のように upstream issue として報告して解消を待つ項目と、IaaS チュートリアルの `deferred` のようにリポジトリ内の対応範囲で解決する項目が同じ文書に並び、「どちらのモックの話か」「誰が修正するのか」が読み手に伝わりにくい。

## Decision

差分管理のファイルをモックの種類ごとに分ける。

- sakumock (外部ライブラリ) の差分は [docs/design/sakumock-gaps.md](../design/sakumock-gaps.md) で追跡する。upstream に issue を起票した項目は issue へのリンクを残す。
- sakumockiaas (リポジトリ内実装) の対応範囲と未対応は [docs/design/sakumockiaas-gaps.md](../design/sakumockiaas-gaps.md) で追跡する。upstream issue は存在せず、リポジトリの作業として管理する。

[ADR 0090](0090-sakumock-gap-tracking.md) の次の判断は引き継ぐ。

- 差分の詳細をサービスごとの設計文書に書かず、差分管理文書への参照に置き換える。
- 差分の扱いの 4 分類 (許容、除外、自前モック、未整備) と、「現象」「影響と現状の扱い」の記載形式。
- チュートリアル baseline の deferred 理由を原因の分類 (モックサーバーの有無、harness 未接続、モックの応答範囲、手動置換プレースホルダー) に沿って書く。
- 解消したら日付と確認内容を残して更新する。

## Consequences

- 「upstream の修正を待つ項目」と「リポジトリで対応する項目」がファイル単位で分かれ、sakumock のバージョン更新時の確認対象が明確になる。
- 差分の扱いの分類など共通の定義は sakumock-gaps.md に置き、sakumockiaas-gaps.md から参照する。定義を変える場合は両文書の整合を確認する。
- 既存の参照 (設計文書、`test/tutorials/baseline.txt`、テストコードのコメント) は、IaaS モックに関する記載を sakumockiaas-gaps.md へ誘導するように更新する。

## Alternatives considered

- **1 ファイルのまま節で分離する (ADR 0090 のまま)**: ファイル名が `sakumock-gaps` である一方で IaaS モックの話も含む状態が続き、管理主体の違いも表題から分からないため採用しない。
- **IaaS モックの対応状況を設計文書 (tutorial-script-harness.md) に戻す**: 差分が複数文書に分散して横断的な把握と解消追跡ができない問題 (ADR 0090 の Context) が再発するため採用しない。
