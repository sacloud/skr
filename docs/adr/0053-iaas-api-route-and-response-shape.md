# ADR 0053: IaaS API コマンドの route と応答形状を維持する

- Status: Accepted
- Date: 2026-10-22

## Context

`iaas-api` は SDK の低レベル IaaS API 操作を CLI に公開します。SDK のパッケージ名をそのままコマンド名にすると、実際の API route と異なる階層になる場合があります。たとえば ServerPlan の操作は `/product/server`、DiskPlan は `/product/disk` です。

また、SDK の `service/iaas` は低レベル API 操作の上にあるサービス層です。サービス層が Find 結果をリソース配列だけに変換すると、API 応答に含まれる `Total`、`From`、`Count` などの情報が CLI 出力から失われます。`iaas-api-go` は `sacloud-sdk-go/api/iaas` に移管されており、SDK は低レベルの API 操作と応答モデルを公開しています。

## Decision

- `iaas-api` のリソース階層は、SDK のサービスパッケージ名ではなく、API route のリソース階層に合わせます。route の API バージョンなど CLI の利用者に不要な要素はコマンド名に含めません。例として `/product/server` は `iaas-api product server`、`/product/disk` は `iaas-api product disk` とします。
- 低レベル操作が `sacloud-sdk-go/api/iaas` にある場合は、その公開 API 操作を利用します。操作の入力変換は必要な SDK の公開変換処理を利用できます。
- JSON 出力は低レベル API の応答モデルを使い、Find の応答 envelope と API が返すフィールドを維持します。`--query` は応答モデル全体に適用します。出力は SDK の型から JSON に再エンコードするため、HTTP response body のバイト列そのものとは保証しません。
- 移行は `PLAN.md` に記録し、対象を絞って段階的に行います。この決定は、既存のすべての IaaS コマンドを一度に移行することを要求しません。

## Consequences

- コマンド階層から実際の IaaS API route を読み取れます。
- Find 出力にページング情報と応答 envelope を含めます。既存の配列直返し出力とは形状が異なります。利用者向けの例と `--query` は応答モデルに合わせます。
- 低レベル API 操作が用意されていない場合や、API 応答モデルでは得られないデータを使う場合は個別に設計し、サービス層を使う理由と応答形状を明確にします。
- HTTP body の原文が必要な場合は、低レベル操作の応答モデル出力とは別の要件として設計します。応答本文をそのまま出力すると `--query` を適用する前に JSON として解析する処理との境界が生じるためです。

## Alternatives considered

- SDK のサービスパッケージ名から `server-plan` のようなコマンド名を作る方法: API route の階層を表さないため採用しません。
- `service/iaas` が返すリソース配列だけを出力する方法: Find 応答 envelope の情報を失うため、API route を直接ラップするコマンドでは採用しません。
- HTTP response body のバイト列をそのまま出力する方法: SDK が公開する応答モデルや共通の `--query` 出力経路を利用できなくなるため、今回の決定には含めません。
- SDK を介さず HTTP 通信を実装する方法: 認証、通信、エラー処理を SDK と別に実装することになるため採用しません。
