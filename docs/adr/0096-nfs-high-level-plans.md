# ADR 0096: NFS プラン一覧の高レベルコマンドを追加する

- Status: Accepted
- Date: 2026-11-21
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0` (`api/iaas`)

## Context

NFS の作成にはプラン ID が必要です ([ADR 0095](0095-nfs-api-commands.md))。プラン ID は、さくらのクラウドがゾーンごとに公開する共有リソース Note (`sys-nfs`) の Content に JSON で記録されています。`sys-nfs` は CommonServiceItem の 1 つで、Content は `plans` オブジェクトの下に SSD と HDD のプラン配列を持ち、各要素は `size`、`availability`、`planId`、`cloneable` のフィールドです。配列には利用可能 (`available`) なプランと利用不可 (`disabled`) なプランが混在し、プラン ID は `1001012001` のような 10 桁の数値になります。

[ADR 0095](0095-nfs-api-commands.md) 時点では、プラン ID の解決方法は CLI の対象外で、チュートリアルが `curl` と `jq` を使う生 API の手順に頼っていました。これは次の問題があります。

- 利用者に API キーの環境変数への配置と URL と jq 式の組み立てを強いる。
- Note (CommonServiceItem) を直接取得する低レベル操作を `skr` に追加しても、利用者が `@zone=` タグの照合と Content の JSON 解析を自分で行う必要がある。

プラン一覧の確認は NFS 作成の必須ステップであり、CLI で完結させることが利用者にとっての価値になります。

## Decision

- 高レベルコマンドグループ `skr nfs` を追加します ([ADR 0075](0075-high-level-command-tier.md) の高レベル `<domain>` 名前空間)。最初のサブコマンドは `skr nfs plans` とし、ゾーンで利用できる NFS プランの一覧を出力します。将来の NFS 高レベル操作の入口もこのグループに置きます。
- `skr nfs plans` の挙動は次のとおりです。
  - `--zone` は必須です。Note のタグ (`@zone=<ゾーン>`) が一致する Note を検索します。見つからない、複数見つかる、Note が利用可能な状態でない、のいずれかはエラーにします。
  - Note の Content を解析し、ストレージ種別 (SSD、HDD) とディスク容量、プラン ID、利用可否、複製可否を JSON 配列で出力します。出力は SSD、HDD の順、同じ種別内はサイズ昇順です。
  - 省略時は利用可能 (`available`) なプランのみを出力します。`--all` で利用不可 (`disabled`) のプランも含めます。Note の配列には利用不可のプランが含まれるため、デフォルトの絞り込みが最小サイズ選択の手順を短くします。
  - 出力フィールド名は既存の高レベル出力と同じく PascalCase (`Storage`、`Size`、`PlanID`、`Availability`、`Cloneable`) です。`PlanID` は SDK の `types.ID` です。
- service 層と CLI コマンドを `internal/nfs/` に置きます。service 層は Note の検索と Content の解析を担い、`sacloud-sdk-go` と標準ライブラリだけに依存します。規模が小さいため [ADR 0077](0077-eventbus-service-library.md) のような独立モジュールへの切り出しはしません。Note 操作は SDK の `iaas.NoteAPI` を interface で受け、単体テストで差し替えます。
- プラン ID の解決を必要とする E2E の安全確認は、引き続き SDK 直呼びで実施します ([ADR 0064](0064-e2e-safety-checks-via-sdk.md))。`skr nfs plans` のライブ検証では、SDK で解決したプラン ID がコマンドの出力に含まれることを照合します。
- sakumock は NFS を含まず、Note (`sys-nfs`) も含まないため、チュートリアルのハーネステストは引き続き延期します ([ADR 0095](0095-nfs-api-commands.md))。

## Consequences

- チュートリアルのプラン解決手順が `curl` と `jq` と API キーの環境変数から `skr nfs plans` へ置き換わり、前提条件から `curl`、`jq`、API キーの環境変数が不要になります。
- プラン一覧の取得が CLI で完結するため、`skr nfs-api create` への `--plan-id` の入力をコマンド置換で組み立てられます。
- Note Content の解析は service 層の単体テストで固定します。Note の形式が変わった場合は service 層の更新で対応します。既知のストレージ種別は SSD と HDD で、未知の種別は出力しません。将来のストレージ種別の追加があった場合は本 ADR と service 層を更新します。
- 高レベルコマンドの追加に伴い、ユーザーズマニュアル ([NFS の高レベルコマンド](../manual/nfs.md)) を作成します ([ADR 0081](0081-high-level-command-users-manual.md))。

## Alternatives considered

- 低レベル `skr iaas-api note` (CommonServiceItem 操作) を公開する案: Note を生 JSON として扱うため、利用者がタグ照合と Content の解析を自分で行う必要があります。プラン解決の作業単位の処理が利用者に漏れるため採用しません。
- `skr nfs-api` のサブコマンドにする案: プラン一覧は API 操作のラップではなく Note の解析を伴う作業単位の処理で、低レベル層の「API 操作と 1 対 1」の規則 ([アーキテクチャ](../design/cli-command-architecture.md)) に合いません。高レベル層に置きます。
- `--available` フラグで絞り込みを切り替える案: 利用不可のプランの確認は例外的な用途であり、デフォルトを利用可能のみ、`--all` で全件とする方が一般的な手順を短くします。
