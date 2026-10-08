# ADR 0033: User-Agent で CLI とバージョンを識別する

- Status: Accepted
- Date: 2026-10-08

## Context

API へのリクエストで、利用者がどの CLI とバージョンを使ったかを識別できるようにする必要があります。`skr http` は任意の HTTPS URL へ直接リクエストを送るため、他の API コマンド経由のリクエストとも区別できる必要があります。

## Decision

- 共通の API クライアントで `User-Agent: skr/v<version> (<os>/<arch>; sacloud-sdk-go/v<SDK version>)` を設定します。
- `skr http` では `User-Agent: skr-http/v<version> (<os>/<arch>; sacloud-sdk-go/v<SDK version>)` を設定し、HTTP コマンド経由であることを区別します。
- `<version>` には `version` パッケージのビルド時バージョンを使います。
- `<os>/<arch>` には Go が実行ファイルに埋め込むビルド対象 OS とアーキテクチャを使い、SDK のバージョンには sacloud-sdk-go の `Version` を使います。ホスト名、ユーザー名などの実行時環境情報は収集しません。
- `skr http --header 'User-Agent: ...'` が指定された場合は、利用者の値を優先します。

## Consequences

API の受信側は User-Agent から CLI 名、CLI と SDK のバージョン、ビルド対象の OS・アーキテクチャ、`skr http` 経由かどうかを識別できます。リリース時は GoReleaser が埋め込む CLI バージョンを使い、開発ビルドでは `version` パッケージの既定値を使います。

## Alternatives considered

- すべてのコマンドで同じ User-Agent を使う方法: `skr http` 経由のリクエストを見分けられないため採用しません。
- SDK の既定 User-Agent を使う方法: `skr` とそのバージョンを識別できないため採用しません。
