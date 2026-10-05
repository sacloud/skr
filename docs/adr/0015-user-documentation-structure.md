# ADR 0015: 利用者向けドキュメントを `docs/manual/` にまとめる

- Status: Accepted
- Date: 2026-10-05

## Context

利用者向け API チュートリアルは `docs/tutorials/` にありますが、`--zone` やプロファイルのようなコマンド横断の解説も必要です。チュートリアルだけの階層では、利用者向け文書全般の置き場所を表せません。

## Decision

- 利用者向けドキュメントを `docs/manual/` にまとめ、既存のチュートリアルを `docs/manual/tutorials/` に移します。
- API の操作手順は `tutorials/` 以下に置き、コマンド横断の機能や設定の解説は `docs/manual/` 直下に置きます。
- 開発者向けの設計記録は引き続き `docs/design/` と `docs/adr/` に置きます。
- この配置規則を [利用者向けドキュメントの構成](../design/user-documentation-structure.md) に記録します。

## Consequences

- 利用者向けのチュートリアルと共通機能の解説を `docs/manual/` から探せます。
- チュートリアルのリンク、生成スキル、リンクチェッカーの既定対象も新しい配置に合わせる必要があります。

## Alternatives considered

- `docs/tutorials/` を維持し、共通機能の解説だけ別の場所に置く方法: 利用者向けドキュメントが分散するため採用しません。
- `docs/manual/` 直下にチュートリアルと共通解説を混在させる方法: 文書の種類を区別しやすくするため、チュートリアルは `tutorials/` 以下に分けます。
