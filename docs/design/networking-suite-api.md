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

シナリオは `test/e2e/networking-suite/scenario_networking_suite_test.go` にあり、testing.T ベースのテストとして `SKR_E2E` 環境変数が設定された場合にだけ実行します ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。ライブ実行の認証はプロファイルディレクトリの `skr-e2e` プロファイルを使い、認証情報を表示しません。`SAKURA_ENDPOINTS_NETWORKING_SUITE` が is1b を指すことを確認します。既定 endpoint は is1c のため、環境変数が未設定の場合はテストをスキップします。

cloudimg Archive とディスクプランはテストの事前確認として sacloud SDK で自動選択します ([ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md))。選択処理は Server E2E と共通の helper ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md)) に実装しています。アーカイブは Ubuntu Server 24.04 LTS の `(cloudimg)` に限定し、最新のもの (ID が最大のもの) を選びます。他の OS やリリースの cloudimg は受け付けません。ディスクプランは指定サイズが利用可能なプランのうち ID が最小のものを選びます。ディスクサイズは `SKR_E2E_NETWORKING_SUITE_DISK_SIZE_GB` (既定 20) で指定できます。

シナリオはランダムな名前と固定の説明で、サブネットグループ `10.0.0.0/16`、2 個の `/24` サブネット、cloudimg Archive から作る Server 2 台と Disk を作成します。Server A は最初の NIC を共有セグメント、追加 NIC をネットワークスイートへ接続し、Server B はネットワークスイートだけへ接続します。NIC は [ADR 0041](../adr/0041-server-repeatable-disk-and-nic-options.md) の反復可能オプションで指定します。接続前に各 Server の NIC ID を読み、同一ゾーンのインターフェース SRN を構成します。

両 NIC のネットワークスイートアドレスを Address API で確認し、Server A の共有側 IP へ SSH 接続して、Server B の別サブネット IPv4 address に TCP 接続します。Server A の user-data で `openssh-server` を導入し、認証鍵はテスト中だけローカルの private temp directory に置きます。応答を受信できた場合にだけ疎通確認を成功とします。

既存の同名リソースを変更または削除しません。Server の停止後に接続を解除し、Server／Disk、アドレス、サブネット、サブネットグループを照合して削除します。SSH による停止が失敗した場合も、作成した Server／Disk の全識別情報を再確認してから `--force` で Server と Disk を削除し、関連アドレスの不在を確認します。削除直前は ID、名前、説明、CIDR、親 SRN、NIC ID、Disk の Archive／Plan／サイズを照合し、削除後の不在も確認します。

Disk 作成前に失敗して Server だけが残った場合も、名前、説明、ID、NIC ID を照合して削除します。Disk が存在する場合は Disk の照合を必須にします。Update の適用後に応答や実行記録の保存が失敗する場合に備え、サブネットグループと最初のサブネットは、この実行で予定した更新前後の名前だけを後始末時に許可します。2個目のサブネットでは更新後の名前を許可しません。名前以外の識別情報の照合は維持します。

Disk ID は Server の作成応答から独立した状態に保存し、後続の Server read では上書きしません。削除直前の接続 Disk ID と Disk read の ID の両方を保存値と照合し、属性が同じ別 Disk も削除しません。保存済み Disk が外れている場合も手動確認を必要とします。作成応答を失った場合、Server は名前、説明、ID、NIC ID を復元できますが、Disk の属性だけでは今回作成した Disk と確定できません。Disk が接続されていれば自動削除を拒否し、実行記録と実リソースを照合する手動確認が必要なエラーを返します。Disk がない Server だけは識別確認後に自動回収します。

NIC ID は SSH 待機、停止要求前、停止待機中にも保存値と照合します。SSH 待機の読み取り結果で保存済みの NIC 一覧を上書きしません。変更を検知した場合は SSH と接続解除を拒否し、Server 削除時にも元の NIC 一覧との照合を維持します。

事前確認と起動待ち・停止待ちのポーリング、削除後の不在確認は SDK で静かに実行します ([ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md))。テスト対象の操作は skr CLI で実行して記録に残し、SSH の接続試行は helper を通じて実行記録へ保存します ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。実行記録は Git 対象外の `test/e2e/networking-suite/e2e-output/` 配下に保存し、秘密鍵の内容と user-data の内容は含めません。秘密鍵は一時ディレクトリにのみ置き、テスト終了時に削除します。

```console
$ make build
$ SKR_E2E=1 \
    SAKURA_ENDPOINTS_NETWORKING_SUITE=ENDPOINT \
    go test ./test/e2e/networking-suite/ -run TestScenarioNetworkingSuite -v -timeout 60m
```

この実行では Server と Disk を作成し、起動と別サブネット間の TCP 接続を確認してから削除します。利用料金が発生し、外部接続も行うため、環境変数による明示的な同意を必須にします。2026-10-28 に testing.T ベースのシナリオをライブ実行し、サブネットグループ・サブネット・インターフェース接続・IP アドレスの操作、別サブネット間の TCP 疏通、後片付けを確認しました。

## 参照

- [Networking Suite API チュートリアル](../manual/tutorials/networking-suite-api.md)
- [CLI コマンド体系](cli-command-architecture.md)
