# ADR 0017: IaaS API 生成器に専用のコマンド入口を設ける

- Status: Accepted
- Date: 2026-10-06

## Context

低レベル IaaS API コマンドを生成する仕組みは、ゾーン横断検索など IaaS 固有の機能を含みます。一方、EventBus API など別形式の API コマンド生成器を将来追加する可能性があります。共通名の `apigen` と `generate-api` では、対象となる API 形式がコマンド名から分かりません。

## Decision

- 現在の IaaS 向け生成器の CLI 入口を `cmd/apigen-iaas`、Make ターゲットを `generate-iaas-api` とします。
- IaaS 固有の生成手順を扱う skill も `apigen-iaas` とします。
- 共通の生成実装は `internal/apigen` に残し、別形式の生成器が必要になった時点で専用の入口を設けます。
- 生成済みコードのヘッダー、設計文書、テスト、skill のコマンド例は専用名に揃えます。

## Consequences

- コマンド名から生成器の対象が分かり、将来の別形式の生成器と区別できます。
- 共通の設定・生成処理は引き続き `internal/apigen` にあり、生成器間の共通化が可能です。
- 将来の API 形式が設定や生成処理を共有できない場合は、独立した入口や実装を追加し、IaaS 固有処理を無理に共通化しません。

## Alternatives considered

- `apigen` と `generate-api` のままにする方法: 現在の対象をコマンド名から判別できず、別形式の生成器が加わると曖昧さが増します。
- 現時点で生成器全体を複数の実装へ分割する方法: 将来の別形式が未実装のため、現段階では不要な設計と保守負担が増えます。
