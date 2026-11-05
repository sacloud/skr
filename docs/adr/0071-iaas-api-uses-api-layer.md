# ADR 0071: IaaS API コマンドから service 層を除外する

- Status: Accepted
- Date: 2026-11-02

## Context

`sacloud-sdk-go/service/iaas` は API 操作に加えて、サービス固有の入力変換やワークフローを提供します。一方、`skr iaas-api` は低レベル API コマンドとして API 操作を直接公開するため、service 層の変換や検証を暗黙に取り込むべきではありません。生成設定、コマンド package、CLI 配線に service 層への依存があり、API 層の操作を直接呼び出す Database などの実装とも方針が揃っていません。

## Decision

`skr iaas-api` のすべての操作は `sacloud-sdk-go/api/iaas` の公開 API 操作を直接呼び出します。`internal/iaas/` のコマンド実装・生成物・テスト、および IaaS API 用の `internal/cli` 配線では `sacloud-sdk-go/service/iaas` を import しません。

API 操作用の入力型と API 層のリクエスト条件への変換は `internal/iaas/<resource>api/` に置きます。SDK の API 操作が要求するゾーン、ID、リクエスト本文を明示して呼び出し、service 層固有の入力変換やワークフローは引き継ぎません。これにより既存コマンドの動作が変わる場合は、設定、ヘルプ、テスト、利用者向け文書に反映します。IaaS API の生成配線は、共有 `api/iaas.Client` を受け取る各 resource package の `NewAPI` を呼び出します。

この禁止範囲は `skr iaas-api` の実装です。ほかの CLI ドメインで service 層を利用するかどうかは、この ADR の対象外です。

## Consequences

- IaaS API の生成設定と生成コードは service 層の request 型や操作 interface に依存しません。
- `internal/iaas/<resource>api/` は API 層の操作へ変換する request 型と adapter を持ちます。service 層の機能を引き継がないことで利用者向け動作が変わる場合は、その差を明示して検証します。
- SDK のバージョン更新で service 層にしかない便利な操作が変更されても、IaaS API コマンドの依存契約には影響しません。API 層に対応する操作がない場合は、利用前に SDK の公開 API と対象仕様を確認します。

## Alternatives considered

- service 層を利用し続け、API interface だけを独自型に置き換える方法は、IaaS API コマンドの実行が引き続き service 層の変換・挙動に依存するため採用しません。
- `api/iaas.Client.Do` で各 HTTP route を直接組み立てる方法は、SDK が提供する型付き API 操作を経由せず route とリクエスト形式を重複管理するため採用しません。
