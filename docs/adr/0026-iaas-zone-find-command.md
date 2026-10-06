# ADR 0026: IaaS Zone 一覧コマンドを追加する

- Status: Accepted
- Date: 2026-10-06

## Context

`--query` を jq で使う具体例が必要ですが、既存の IaaS コマンドでは一覧取得にゾーン指定やリソース作成を要します。ゾーン一覧自体は IaaS Zone API の読み取り操作で取得できます。

## Decision

- `skr iaas-api zone find` を追加し、SDK v0.3.0 の `service/iaas/zone.FindWithContext` を呼び出します。
- `FindRequest` は任意とし、未指定の場合はゾーン一覧を取得します。出力は共通の JSON、YAML、table、`--query` の処理を利用します。
- ライブ E2E は選択中プロファイルを確認した後、一覧取得と jq による名前抽出だけを行います。クラウドリソースの変更はしません。
- チュートリアルではゾーン一覧を table で確認し、jq で名前の配列を出力する手順を示します。

## Consequences

- jq 出力加工を、リソース作成やゾーン指定をせずに試せます。
- コマンドはゾーンの一覧取得だけを公開し、Zone API の `Read` や書き込み操作は公開しません。
- ライブ E2E は API への読み取りアクセスと有効なプロファイルを必要とします。

## Alternatives considered

- 既存の `switch find --zone all` を使う方法: スイッチが存在しない環境では結果を観察できず、全ゾーン検索を必要とするため採用しません。
- `skr http` でゾーン API を呼ぶ方法: 生の HTTP レスポンスを jq 出力機能に渡せないため、共通出力経路を通る SDK コマンドを追加します。
