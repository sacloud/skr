# Networking Suite API コマンド設計

- Status: Accepted
- ADR: [0050](../adr/0050-networking-suite-api-commands.md)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`

## コマンド対応範囲

| リソース | 操作 |
| --- | --- |
| `subnet-group` | `list`, `read`, `create`, `update`, `delete` |
| `subnet` | `list`, `read`, `create`, `update`, `delete` |
| `interface-connection` | `create`, `delete` |
| `address` | `list`, `read` |

コマンドは SDK の公開操作に対応します。SDK に公開されていないインターフェースコネクションの `read` や、別サブネット間の通信を CLI が仲介する操作はありません。

### SRN と入力

リソース参照は位置引数の SRN で受け取り、API 呼び出し前にリソース種別を検証します。`subnet list` は親サブネットグループの SRN、`address list` はサブネットの SRN を位置引数にします。`interface-connection create` は NIC とサブネットの SRN をフラグで受け取ります。NIC SRN の位置識別子と ID の組み立て方は SDK v0.3.0 の CreateInterfaceConnection API ドキュメントに記載されています。

サブネットグループ／サブネットの Create は、名前、説明、CIDR、リージョンまたはゾーン、親グループ SRN をフラグで構成できます。Update の API 入力は `Name` と `Description` の値型フィールドであるため、フラグ経路では両方を必須にします。`--request` を指定する場合は個別フラグを併用せず、SDK のリクエスト JSON 全体を渡します。直接 JSON、`@path.json`、標準入力 `-` に対応します。

SDK のリクエスト／レスポンスモデルは独自の JSON 実装を持つため、CLI はそれらの `UnmarshalJSON` と `MarshalJSON` を通して JSON を扱います。未知フィールドは SDK のモデル実装に従って無視され、`json.Unmarshal` によって末尾に別の JSON 値がある入力はエラーになります。

InterfaceConnection Create は SDK のパラメーター型を通して NIC SRN、Subnet SRN、任意の Ephemeral IPv4 address を受け付けます。明示アドレスを省略した場合は SDK の入力値を省略し、自動割り当てを利用します。Address List は接続 SRN を省略可能な絞り込み条件として扱います。

### エラーと出力

操作結果は共通の JSON／`--query` 出力経路へ渡し、SDK と入力処理のエラーを呼び出し元へ返します。Delete 操作は出力を生成しません。リソース一覧の全要素は SDK が返す配列として出力されます。

## ライブ E2E

runner は `test/e2e/networking-suite/` にあり、`--confirm-is1b-live` を必須にします。実行前に `SAKURA_ENDPOINTS_NETWORKING_SUITE` が is1b を指すこと、SDK プロファイル／プロジェクトの対象を確認します。Archive ID と DiskPlan ID は同じ is1b 環境で利用できる値を指定します。Disk の読み取りで Archive、DiskPlan、Server、サイズを照合し、利用可能になるまで待ってから Server を起動します。

runner はランダムな名前と固定の説明で、サブネットグループ `10.0.0.0/16`、2 個の `/24` サブネット、Cloud-init Archive から作る Server 2 台と Disk を作成します。Server A は最初の NIC を共有セグメント、追加 NIC をネットワークスイートへ接続し、Server B はネットワークスイートだけへ接続します。接続前に各 Server の NIC ID を読み、同一ゾーンのインターフェース SRN を構成します。

両 NIC のネットワークスイートアドレスを Address API で確認し、Server A の共有側 IP へ SSH 接続して、Server B の別サブネット IPv4 address に TCP 接続します。Server A の user-data で `openssh-server` を導入し、認証鍵はテスト中だけローカルの private temp directory に置きます。応答を受信できた場合にだけ疎通確認を成功とします。

既存の同名リソースを変更または削除しません。Server の停止後に接続を解除し、Server／Disk、アドレス、サブネット、サブネットグループを照合して削除します。SSH による停止が失敗した場合も、作成した Server／Disk の全識別情報を再確認してから `--force` で Server と Disk を削除し、関連アドレスの不在を確認します。削除直前は ID、名前、説明、CIDR、親 SRN、NIC ID、Disk の Archive／Plan／サイズを照合し、削除後の不在も確認します。証跡は `test/e2e/internal/evidence` を用いて `tmp/networking-suite-api/` に private 権限で保存します。秘密鍵は一時ディレクトリにのみ置き、証跡へ含めません。

## 参照

- [Networking Suite API チュートリアル](../manual/tutorials/networking-suite-api.md)
- [CLI コマンド体系](cli-command-architecture.md)
