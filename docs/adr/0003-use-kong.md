# ADR 0003: CLI フレームワークに Kong を採用する

- Status: Accepted
- Date: 2026-10-05

## Context

従来の `usacloud` は Cobra を利用していた。新しい CLI で使用するフレームワークを選ぶにあたり、関連プロジェクトとの一貫性も考慮する必要がある。モック用ツールの `sakumock` は Kong を利用している。

## Decision

`skr` の CLI フレームワークに Kong を採用し、`sakumock` と合わせる。

## Consequences

- `skr` と `sakumock` で CLI フレームワークを揃えられる。
- 従来の `usacloud` が利用していた Cobra とは異なるフレームワークとなる。

## Alternatives considered

- Cobra: 従来の `usacloud` で利用していたが、関連プロジェクトの `sakumock` に合わせるため採用しない。
