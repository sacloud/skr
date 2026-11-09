# ADR 0074: API コマンド名を operationId から機械派生する

- Status: Accepted (2026-11-09 に全対象ドメインの移行が完了)
- Date: 2026-11-09

## Context

低レベル API コマンド (`<domain>-api`) の名前構造には、おもに次の 2 系統があります。

- リソース名をコマンドグループ、操作名を末端サブコマンドに置く構造 (`skr iam-api user list`)
- operationId を末端サブコマンド名に機械的に変換し、ドメイン直下に配置する構造 (`skr iam-api list-users`。AWS CLI の `aws s3api list-buckets` や `aws iam list-users` と同じ方式)

skr の現行実装は前者に従い、[ADR 0005](0005-low-level-and-high-level-command-tiers.md) の低レベル名前空間の定義と、[CLI コマンド体系の設計](../design/cli-command-architecture.md)の構文 `skr <domain>-api <resource> <sdk-operation>` で実装してきました。

リソースグループ型では、コマンド名と operationId の対応が機械的になりません。`skr simplemq-api queue config` が `updateQueue` を呼ぶ、`skr iam-api sso link` が `assignSsoProfile` を呼ぶ、といった対応は、マッピング表の管理と CI 検証で維持する必要があります。operationId から名前を機械派生する方式なら、対応は完全に機械的になり、検証は派生名との一致だけで済みます。

また、operationId にはリソース名の語が含まれる (`listQueues` など) ため、機械派生名をリソースグループの配下に置くと、`skr iam-api user list-users` のようにリソース語が重複して冗長になります。

`eventbus-api` と `simple-notification-api` が操作する CommonServiceItem は、さくらのクラウドのサービスを統合的に扱うための枠組みです。spec では `listCommonServiceItems` などの operationId を、`trigger`、`schedule`、`destination` など複数のリソースで共有します。リソースの違いは Provider.Class で表されるため、同じ SDK 操作をリソースごとのコマンドに複製する必要はありません。

## Decision

- OpenAPI の operationId を持つ API ドメイン (`iam-api`、`simplemq-api` など) のコマンドは、`skr <domain>-api <operation>` の構造にします。ドメイン直下に末端サブコマンドを配置し、リソースのグループ階層を設けません。
- 末端サブコマンド名は operationId から機械派生します。operationId は SDK (ogen) が operation 名 (PascalCase) に正規化し、その語列を Kong と同じ camelcase 処理で分割してハイフンで結合します (`sendMessage` → `send-message`、`seg_apply` → `seg-apply`)。
- operationId を複数のリソースで共有する CommonServiceItem 操作 (`eventbus-api`、`simple-notification-api`) は、spec のとおり 1 組の操作としてコマンドを公開します。リソースの種別は Provider.Class として `--provider-class` オプションで渡します。リソースごとに使いやすい操作の形が必要な場合は、高レベルコマンドで提供します。
- IaaS API など operationId を持たないコマンドは本 ADR の対象外で、endpoint 構造に対応させるグループ階層 ([ADR 0069](0069-api-endpoint-command-groups.md)) を維持します。
- 高レベルコマンド (`skr iaas ...`) も本 ADR の対象外です。

命名規則、全コマンドの対応表、CI 設計、段階的な移行計画は [API コマンド名の operationId 準拠と移行計画](../design/api-command-operation-id-naming.md) に記録します。

## Consequences

- コマンド名と API 操作の対応が完全に機械的になり、CI が派生名との一致を検証できます。operationId に存在しない語や、operationId と別の動詞 (`config`、`link`) がコマンド名に入る余地がなくなります。
- operationId を共有する操作は 1 つのコマンドに統合されるため、公開対象の各 operationId に対応する CLI コマンドは 1 つになります。
- Ogen 系ドメインでは、リソースごとの操作一覧を `--help` の階層で示せなくなります。リソース名は操作名の語に含まれるため、ドメイン配下の一覧で操作と対象リソースを読み取れます。
- usacloud の `server list` 型と語順が変わるため、usacloud から移行する利用者は操作感が変わります。
- 既存の Ogen 系コマンドはすべて名前が変わります。usacloud との互換性を含め、互換性は保証しません ([ADR 0001](0001-new-cli-without-usacloud-compatibility.md))。

## Alternatives considered

- **リソースグループ型を維持し、コマンド名の語を operationId の語列の部分列に制約する方式**: コマンド名の語の由来と順序だけを CI で検証し、語の選択と省略はマッピング表に記録する方式です。破壊的変更を最小化できる一方、対応が機械的でなく、全コマンドのマッピング表の登録と維持が必要になります。operationId の語を省略する基準が設計の裁量に残るため、採用しません。
- **グループ階層を維持し、リーフ名だけ operationId 派生にする方式 (`skr iam-api user list-users`)**: operationId との機械的な対応を保ちながら階層も維持できますが、リソース語がグループとリーフで重複して冗長です。採用しません。
- **operationId を camelCase のまま使う方式 (`skr iam-api listUsers`)**: Kong の既定命名 (kebab-case) に反します。採用しません。
- **operationId を共有する操作をリソースごとのコマンドに複製する方式 (現行)**: 利用者はリソースごとのコマンドで操作できますが、同じ SDK 操作に複数のコマンド名が対応し、operationId との機械的な対応が失われます。Provider.Class をオプションで渡す共通コマンドに統合するため、採用しません。
