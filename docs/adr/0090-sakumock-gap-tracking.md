# ADR 0090: sakumock の既知の差分を 1 ファイルに集約する

- Status: Superseded by [ADR 0091](0091-split-sakumock-and-sakumockiaas-gap-tracking.md)
- Date: 2026-11-19

## Context

sakumock を使うテスト (CLI 操作テスト、チュートリアル harness、ライブ E2E のドライラン) では、実 API との差分や未整備が段階的に判明する。たとえば Workflows の `list-suggestions` は sakumock が API 応答スキーマ上必須の `Count` を返さないためデコードが失敗し、E2E はそのエラーだけを制約として許容している。

これらの情報は、これまでサービスごとの設計文書、`PLAN.md`、チュートリアル baseline (`test/tutorials/baseline.txt`) の deferred 理由、テストコードのコメントに分散していた。

- 同じ差分が複数の文書に異なる粒度で書かれ、どれが最新か分からない。
- sakumock のバージョン更新で解消した差分の後追いができない。実例として、security-control は v0.13.0 時点で「sakumock にサーバーが無い」と記録したが、v0.14.0 (2026-11-07) で `securitycontrol` モックが追加されたことが `PLAN.md` には反映されていない。
- 「sakumock が未整備」という deferred 理由が、実際には「sakumock にサーバーがあるがチュートリアル harness に未接続」の意味で使われ、原因の分類ができない。

## Decision

sakumock に関する既知の差分・未整備・モック非対応の情報は、[docs/design/sakumock-gaps.md](../design/sakumock-gaps.md) の 1 ファイルに集約して追跡する。

- サービスごとの設計文書は「sakumock による検証の状況」のみを書き、差分の詳細は sakumock-gaps.md への参照に置き換える。
- 差分の記載には「現象」と「影響と現状の扱い」(どのテストで許容・除外・自前モック・deferred にしたか) を書く。解消したら日付と確認内容を残して更新する。
- チュートリアル baseline の deferred 理由は、原因の分類 (モックサーバーの有無、harness 未接続、モックの応答範囲、手動置換プレースホルダー) に沿って書き、詳細は sakumock-gaps.md を参照する。

## Consequences

- 差分の一覧と現状が 1 箇所で確認でき、sakumock のバージョン更新時の解消確認が容易になる。
- 設計文書と baseline の理由が重複しなくなる一方、sakumock 関係の確認は sakumock-gaps.md の更新を忘れない運用が必要になる。
- 既存の記述 (PLAN.md の各サービス行など) は直ちに書き直さず、各ドメインの作業時に正確化する。

## Alternatives considered

- **サービスごとの設計文書に差分を書き続ける**: 検索性は高いが、横断的な状況 (どのサービスが未整備か) の把握と解消追跡ができない。分散が問題の発端であるため採用しない。
- **Issue トラッカーで管理する**: リポジトリ外に情報が置かれ、テストコードや baseline との対応が追いにくい。モックの差分はコードの近くに置く方が更新されやすいため採用しない。
