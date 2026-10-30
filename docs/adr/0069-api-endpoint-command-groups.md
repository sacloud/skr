# ADR 0069: 低レベル API コマンドのグループを API endpoint 構造で整理する

- Status: Accepted
- Date: 2026-10-30

## Context

`iaas-api` に `ipaddress`、`ipv6addr`、`ipv6net` を追加するにあたり、コマンドグループの配置を決める必要がある。対象の 3 リソースは、いずれもゾーン単位の平坦な endpoint (`/{zone}/api/cloud/1.1/ipv6net` など) を持ち、SDK も親リソースの ID を引数に取らず `Find(ctx, zone, conditions)` / `Read(ctx, zone, id)` の形である。

一方、既存の `internet` グループは、ルータ＋スイッチ配下のネストした endpoint (`/{zone}/api/1.0/internet/{id}/ipv6net` など) を `read-ipv6`、`enable-ipv6`、`disable-ipv6`、`add-subnet` などのサブコマンドとして既に公開している。

旧 usacloud は IPv6 系を独立したコマンドとしていたが、本プロジェクトは usacloud との互換性を前提にしないため、endpoint 構造だけを根拠に配置を決める。

## Decision

低レベル API コマンド (`skr <domain>-api <resource> ...`) のコマンドグループは、対象 API の endpoint 構造に対応させる。

- ゾーン単位の平坦な endpoint (`/{zone}/api/cloud/1.1/<resource>`) は、親リソースの ID を取らないため、独立したコマンドグループ (`skr iaas-api <resource>`) として公開する。
- 親リソース配下のネストした endpoint (`/{zone}/api/1.0/<parent>/{id}/<sub>`) は、親リソースの ID を取るため、親リソースのグループのサブコマンド (`skr iaas-api <parent> <sub>`) として公開する。

この対応を `ipaddress`、`ipv6addr`、`ipv6net` に適用する。3 リソースはいずれも平坦な endpoint なので、`skr iaas-api ipaddress`、`skr iaas-api ipv6addr`、`skr iaas-api ipv6net` として公開する。ルータ＋スイッチ配下の IPv6 設定は、既存の `skr iaas-api internet` のサブコマンドのままとする。

旧 usacloud のコマンド構成は根拠にしない。

## Consequences

- コマンドツリーから、API の endpoint 構造と識別子の種類 (数値 ID、IP アドレス文字列) が読み取れる。`internet read-ipv6` (ルータ＋スイッチの ID) と `ipv6net read` (IPv6 ネットワークの ID) は別コマンドとして両方残り、ヘルプで識別子の違いを説明する。
- 同じ IPv6 領域の操作が `internet` のサブコマンドと独立グループに分かれる。`ipv6net` のヘルプで、IPv6 の有効化と無効化は `internet` の `enable-ipv6` と `disable-ipv6` を使うことを案内する。
- 親 ID を取らない API に対して親リソース配下のコマンドを作らないため、ルータ＋スイッチを指定すると誤解する形のコマンドを追加しない。
- 今後リソースを追加する場合も、endpoint 構造を根拠にグループの有無と配置を決める。

## Alternatives considered

- **`skr iaas-api internet ipv6net` として配置する**: ルータ＋スイッチの指定に見えるが、実際はゾーン全体の一覧であり誤解を招く。`internet` の既存の IPv6 サブコマンド (`read-ipv6` など) と語彙が衝突し、ネストした endpoint を持たない `ipv6addr`、`ipaddress` を `internet` 配下に置く根拠もない。採用しない。
- **IPv6 系を `skr iaas-api ipv6` の 1 グループにまとめる**: endpoint 構造と対応しないため、数値 ID (`ipv6net`) とアドレス文字列 (`ipv6addr`) の識別子の違いがコマンドツリーに現れない。採用しない。
