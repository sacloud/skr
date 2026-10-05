# ADR 0002: Go を採用する

- Status: Accepted
- Date: 2026-10-05

## Context

このプロジェクトで使用するプログラミング言語を決定する必要がある。チームは Go に慣れており、さくらのクラウドの公式 SDK は現時点で `sacloud-sdk-go` のみである。

## Decision

`skr` の開発言語として Go を採用し、さくらのクラウドの操作には `sacloud-sdk-go` を利用する。

## Consequences

- チームが慣れている言語と公式 SDK を使って開発できる。
- 公式 SDK の選択肢が増えた場合は、言語や SDK の選択を改めて検討できる。

## Alternatives considered

現時点では `sacloud-sdk-go` 以外に公式のさくらのクラウド SDK がないため、公式 SDK を利用する別の言語を選ぶ選択肢はない。
