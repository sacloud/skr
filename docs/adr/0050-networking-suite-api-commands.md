# ADR 0050: Networking Suite API コマンドを手書きで実装する

- Status: Accepted
- Date: 2026-10-16
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`

## Context

SDK v0.3.0 はネットワークスイート API として、サブネットグループ 5 操作、サブネット 5 操作、インターフェースコネクション 2 操作、アドレス 2 操作を公開しています。SRN を受け取る操作、親サブネットグループ SRN を受け取る一覧、NIC とサブネットを SRN に変換して渡す接続作成など、SDK メソッドの引数が同じ形ではありません。

ネットワークスイートのサブネット間通信は、サブネットグループとサブネットを作成するだけで確認できません。対象ゾーン内の Server NIC を各サブネットへ接続し、ゲスト OS に配布される IP アドレスと経路を利用して通信を確認する必要があります。

## Decision

- `networking-suite-api` を追加し、SDK が公開する前述の 14 操作を 1 対 1 で提供します。SDK にない操作や「インターフェースコネクションの参照」は追加しません。
- コマンドと SDK 操作の接続、SRN 種別検証、JSON／フラグ入力を `internal/networkingsuiteapi/` に実装します。SDK が独自の入力型や SRN 引数を使うため、Ogen API コマンド生成器の設定へ無理に統合しません。CLI 登録と Runtime の注入は `internal/cli/` が担当します。
- Create と Update のうち、API 入力が独立したスカラー値で構成される操作はフラグと `--request` JSON の両方を受け付けます。両経路は相互排他にし、JSON で SDK の公開リクエストモデルを受け取ります。JSON は直接値、`@path.json`、標準入力 `-` に対応し、SDK モデルの `UnmarshalJSON` を通します。未知フィールドは SDK のデコード動作に従い、末尾の余分な JSON 値は拒否します。
- API の更新モデルに含まれる `Name` と `Description` は値型であるため、Update のフラグ経路では両方を必須にします。部分更新や `null` によるクリアを仮定しません。
- InterfaceConnection Create は `--interface-srn`、`--subnet-srn`、任意の `--ephemeral-ipv4-address` を受け付けます。NIC SRN はサーバ詳細の NIC ID と配置ゾーンから組み立てます。アドレス一覧はサブネット SRN を位置引数にし、任意の接続 SRN で絞り込みます。
- ライブ E2E は is1b にテスト専用サブネットグループ、別 CIDR のサブネット 2 個、一時 Server 2 台を作成します。Server A は共有セグメントとネットワークスイート、Server B はネットワークスイートだけへ接続し、A から B の TCP 応答を確認します。生成した資源だけを内容と ID で照合して停止・削除し、不在を確認します。実行には専用の確認フラグ、is1b のネットワークスイート endpoint、利用可能な Archive と DiskPlan が必要です。

## Consequences

- 入出力と SDK 操作の対応をドメイン内で追跡でき、JSON モデル、SRN の種別、一覧の親リソースをコマンドごとに検証できます。
- JSON デコードは SDK の生成モデルを使うため、未知フィールドの受け入れは SDK のモデル実装に従います。末尾データの拒否は `json.Unmarshal` による入力全体の検証で維持します。
- SDK の既定ネットワークスイート endpoint は is1c です。is1b など別ゾーンを使う場合は `SAKURA_ENDPOINTS_NETWORKING_SUITE` を対象 endpoint に設定する必要があります。
- ライブ E2E は Server と Disk を作成するため利用料金が発生します。停止・切断・削除に失敗した場合は private evidence の場所を示し、既存リソースを推測で削除しません。

## Alternatives considered

- 生成器の汎用 Request 形式へ全操作を変換する方法: SDK 操作の位置引数、SRN 変換、接続パラメーターの差を隠すために生成器へサービス固有処理を追加することになり、共通生成機構の目的に合わないため採用しません。
- サービスを高レベルの一括構成コマンドとして公開する方法: 今回は SDK 操作を直接提供する低レベル API コマンドを追加する依頼であり、作成順序やリソース所有権を暗黙にするワークフローは追加しません。
- Server 間でライブ疎通せず CRUD だけを E2E とする方法: サービスの主要な L3 通信を観察できないため採用しません。
