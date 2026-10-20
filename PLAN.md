# API コマンド対応計画

## 対象と方針

対象 SDK は `go.mod` で指定している [sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1) です。IaaS は `service/iaas` のサービスパッケージ、IaaS 以外は `api` 直下の API ドメインを棚卸ししています。`service/iaas` に含まれる API でも、CommonServiceItem を使うものは `iaas-api` のリソース候補と分けて管理します。

一覧は SDK の提供範囲を把握するためのもので、すべてのパッケージをそのまま CLI コマンドにすることを決めるものではありません。各パッケージで公開操作、入力、出力、固有の検証を確認し、対応範囲を明示して段階的に実装します。コマンドの階層は `<domain>-api <resource>` とし、SDK の型だけから公開操作や入力の意味を推測しません。詳細は [ADR 0016](docs/adr/0016-generate-low-level-api-commands.md) に従います。

API コマンド生成器は、IaaS 用の `cmd/apigen-iaas` / `generate-iaas-api` と、IaaS 以外の Ogen 系 API 用の共通入口 `cmd/apigen-api` / `generate-api` を使います。EventBus、SimpleMQ などの Ogen 系 API は、サービスごとに生成器入口を増やさず、操作・引数・ヘルプを設定で明示して生成します。IaaS と Ogen 系 API は異なる SDK 形式に合わせて別の入口を維持し、共通化できる実装のみ共有します。入口があることは、そのドメインの全操作が CLI に実装済みであることを意味しません。

SimpleMQ のキュー／メッセージ API コマンドは [`api/commands/simplemq-queue.json`](api/commands/simplemq-queue.json) と [`api/commands/simplemq-message.json`](api/commands/simplemq-message.json) を使って生成します。SDK 固有の入力補完や出力整形が必要な操作は手書きハンドラーに残します。

この一覧の `[x]` は、説明に列挙した操作が CLI に実装済みであることを示します。SDK の全操作への対応を示すものではありません。ライブ E2E runner とチュートリアルの有無は、それぞれ別に記載します。

## IaaS リソース候補

SDK のサービスパッケージごとに対応状況を管理します。実装済みコマンドの操作範囲は「実装済み IaaS コマンドの対応範囲」に記載し、ここには未実装の候補を記載します。

- [ ] `bridge`
- [ ] `cdrom`
- [ ] `coupon`
- [ ] `database`
- [ ] `iface`
- [ ] `ipaddress`
- [ ] `ipv6addr`
- [ ] `ipv6net`
- [ ] `license`
- [ ] `loadbalancer`
- [ ] `mobilegateway`
- [ ] `nfs`
- [ ] `note`
- [ ] `privatehost`
- [ ] `subnet`
- [ ] `vpcrouter`

## 実装済み IaaS コマンドの対応範囲

チェックは列挙した操作の CLI 実装状況を示します。SDK にある未対応操作、ライブ E2E、チュートリアルの状況も併記します。

- [x] `archive`
    - `iaas-api archive` の Find / Read を実装済みです。Find はゾーン、タグ、公開アーカイブの OS 種別で検索できます。
    - SDK の Create / Update / Delete / Upload / Download / OpenFTP / CloseFTP / WaitReady は未対応です。
    - 読み取り専用の [ライブ E2E runner](test/e2e/iaas-lookup/) と[チュートリアル](docs/manual/tutorials/iaas-api/archive.md)があります。is1b で検索・参照を確認済みです。
- [x] `disk` — `iaas-api disk` の Find / Read / Create / Update / Delete を実装済みです。Create は必須・任意の単純値とタグ、Update はゾーン・ID・変更する単純値とタグを個別フラグで指定でき、複雑な入力には JSON を使います。Find は `--name` による単一名の部分一致検索に対応します。SDK の Apply / ConnectToServer / DisconnectFromServer / Edit / MonitorDisk / ResizePartition / WaitReady は未対応です。is1b で作成・検索・参照・更新・削除と削除後を確認する [ライブ E2E runner](test/e2e/disk/) と[チュートリアル](docs/manual/tutorials/iaas-api/disk.md)があります。
- [x] `internet` — `iaas-api internet` の Find / Read / Create / Update / Delete、サブネット操作、IPv6 操作、MonitorRouter を実装済みです。Find は `--name` による単一名の部分一致検索、Delete は `--force` と `--fail-if-not-found` に対応します。ルータ＋スイッチ、Server、起動 Disk を作成し、サブネットから選んだ IPv4 アドレスを NIC の OS 設定へ適用して SSH 接続する [ライブ E2E runner](test/e2e/internet/) と[チュートリアル](docs/manual/tutorials/iaas-api/internet.md)があります。個別オプションだけの操作で is1b の作成・NIC 接続・名前検索・サブネット参照・SSH ログイン・削除後の不在を確認済みです。Update、追加サブネット、IPv6、MonitorRouter のライブ検証は未実施です。
- [x] `icon` — `iaas-api icon` の Find / Read / Create / Update / Delete を実装済みです。Create は `--image-file` で画像ファイルを読み込み、API 用の Base64 文字列へ変換します。更新できる項目は Name と Tags です。CRUD の[ライブ E2E runner](test/e2e/icon/) と[チュートリアル](docs/manual/tutorials/iaas-api/icon.md)を追加済みです。ライブ E2E で作成・検索・参照・更新・削除と削除後の不在を確認済みです。
- [x] `packetfilter` — `iaas-api packetfilter` の Find / Read / Create / Update / Delete を実装済みです。ルールの Expression 配列は `--expressions` の JSON 値または `--request` で指定し、ゾーン、名前、ID などの単純値はフラグで指定できます。is1b で作成・検索・参照・ルール更新・削除を確認する [ライブ E2E runner](test/e2e/packetfilter/) と[チュートリアル](docs/manual/tutorials/iaas-api/packetfilter.md)があります。ライブ E2E で各操作と削除後の不在を確認済みです。NIC への適用とパケット通信の検証は対象外です。
- [x] `server`
    - [x] Find / Read / Create / Boot / Update / Delete を実装済みです。
    - Create では基本項目に加え、反復可能な `--disk` と `--nic` で複数ディスクと共有／未接続／スイッチ ID 接続の NIC を指定できます。Find は `--name` による単一名の部分一致検索に対応します。Boot は user-data のファイル／標準入力に対応します。
    - Create の定型的な入力処理・検証・API 呼び出しは生成し、ディスク／NIC の固有変換だけを型付き callback に接続します。生成への移行後も is1b のライブ E2E で作成・起動・SSH 接続・削除後の不在を確認済みです。生成器との境界は [ADR 0042](docs/adr/0042-generated-request-transform.md) に記載しています。Boot は手書きを維持します。
    - [ ] SDK の Apply / ChangePlan / EjectCDROM / InsertCDROM / MonitorCPU / Reset / SendNMI / Shutdown / WaitBoot / WaitShutdown は **未対応**です。
    - is1b で新記法による cloud-init 起動・SSH 接続・削除・一時 SSH ファイルの後始末を確認済みです。[ライブ E2E runner](test/e2e/server/) と[チュートリアル](docs/manual/tutorials/iaas-api/server.md)があります。設計は [ADR 0040](docs/adr/0040-server-cloud-init-and-ssh-tutorial.md)、[ADR 0041](docs/adr/0041-server-repeatable-disk-and-nic-options.md) と[設計文書](docs/design/server-cloud-init-ssh.md)を参照してください。
- [x] `switch` — SDK の `swytch` に対応する `iaas-api switch` の Find / Read / Create / Update / Delete を実装済みです。SDK の公開操作はすべて CLI にあります。Sandbox と is1b で作成・検索・参照・更新・削除を検証する [ライブ E2E runner](test/e2e/switch/) と[チュートリアル](docs/manual/tutorials/iaas-api/switch.md)があります。

## CommonServiceItem 系 API 候補

SDK の `service/iaas` にありますが、CommonServiceItem を使う API は通常の `iaas-api <resource>` 候補と分けます。CLI では `containerregistry` の例のように、`container-registry-api` などサービスごとの API コマンドにすることを想定します。
Simple Notification の group／destination API も CommonServiceItem を使いますが、候補一覧では非 IaaS API ドメイン欄の `simple-notification` にまとめます。

- [ ] `autobackup`
- [ ] `autoscale`
- [x] `containerregistry` — `container-registry-api` でレジストリと認証ユーザーを管理します。ライブ E2E runner と[チュートリアル](docs/manual/tutorials/container-registry-api.md)を追加済み
- [ ] `dns`
- [x] `gslb` — `gslb-api` で Find / Read / Create / Update / Delete を実装済みです。単純値、ヘルスチェックのフラットなフラグ、反復可能な `--destination-server` と JSON 入力に対応します。[is1b ライブ E2E runner](test/e2e/gslb/) で実サーバーとソーリーサーバーを各1台作成し、ping 監視による停止・復帰時の応答先切り替えを `NOERROR` と期待 IP の完全一致で確認済みです。GSLB の CRUD、両サーバーの削除、ディスクの個別削除と各リソースの不在も確認済みです。以前の単一宛先・空応答による停止確認は不十分なため、この構成で置き換えました。[チュートリアル](docs/manual/tutorials/gslb-api.md)では両サーバーの準備、ソーリーサーバーへの切り替えと復帰、更新、ディスクの個別削除を説明します
- [ ] `localrouter`
- [ ] `proxylb`
- [ ] `simplemonitor`

## IaaS の補助・プラン系サービス候補

これらも SDK にサービスパッケージがあります。リソース操作コマンドと同じ形にせず、公開する操作と適切な CLI の配置を個別に確認します。

- [ ] `bill`
- [ ] `internetplan`
- [ ] `licenseinfo`
- [ ] `privatehostplan`
- [x] `region` — `iaas-api region` の Find / Read を実装済みです。Find は件数・開始位置をフラグで指定でき、検索名・Sort は JSON で指定します。Read は ID で参照します。Region API はゾーン指定を受け付けません。SDK の公開操作はすべて CLI にあります。読み取り専用の[ライブ E2E runner](test/e2e/region/) と[チュートリアル](docs/manual/tutorials/iaas-api/region.md)を追加し、実環境で Find / Read の結果一致を確認済みです
- [ ] `serverplan`
- [x] `serviceclass` — `iaas-api service-class` の Find を実装済みです。指定ゾーンのサービスクラス一覧を取得できます。読み取り専用の [ライブ E2E runner](test/e2e/serviceclass/) と[チュートリアル](docs/manual/tutorials/iaas-api/serviceclass.md)があります。`is1b` でライブ E2E を実行し、517 件のサービスクラスを取得して、各項目の ID・名前・パスを検証済みです
- [x] `zone` — `iaas-api zone` の Find のみ実装済みです。SDK の Read は未対応です。ゾーン一覧を取得して jq で名前を抽出する読み取り専用の [ライブ E2E runner](test/e2e/zone/) と[チュートリアル](docs/manual/tutorials/iaas-api/zone.md)があります
- [x] `diskplan` — `iaas-api disk-plan` の Find / Read を実装済みです。検索結果の API `Availability` と `Size` から、指定ゾーンの利用可否とサイズを確認できます。SDK の公開操作はすべて CLI にあります。Archive と合わせて検証する読み取り専用の [ライブ E2E runner](test/e2e/iaas-lookup/) と[チュートリアル](docs/manual/tutorials/iaas-api/disk-plan.md)があります。is1b で検索・参照を確認済みです

## IaaS 以外の API ドメイン候補

SDK の `api` 直下には、IaaS 以外に次の API ドメインがあります。CLI コマンドの実装、ライブ E2E、チュートリアルの対応状況は分けて記載します。コマンドの入口があることは、そのドメインの SDK 操作すべてへの対応を意味しません。

- [x] `iam` — `iam-api` コマンド、ライブ E2E、[チュートリアル](docs/manual/tutorials/iam-api.md)を実装済みです。
- [x] `eventbus` — `eventbus-api`。Provider 補完・リクエスト検証・シークレット入力とエラー伝播のドメイン単体テストを実装済みです。
- [x] `simplemq` — `simplemq-api`。キュー／メッセージコマンドは `apigen-api` を使います。
- [x] `simple-notification` — `simple-notification-api`。通知先、グループ、ルーティング、履歴の SDK 公開操作を実装済みです。送信は `--message` と JSON 入力に対応します。既存の送信可能な通知先へ通知を送り、履歴の処理完了と一時グループの削除を確認する [ライブ E2E runner](test/e2e/simple-notification/) と[チュートリアル](docs/manual/tutorials/simple-notification-api.md)があります。ライブ E2E で通知処理の完了と一時グループの削除を確認済みです。今回のテスト通知は利用者によるメール受信確認も完了しています。Webhook の到達確認は未実施です。
- [ ] `apprun-dedicated` — `apprun-dedicated-api`。リクエスト入力・秘密情報のインライン拒否・エラー伝播のドメイン単体テストを実装済みです。
  - CLI 実装済み、ライブ E2E とチュートリアルは未対応。不足分は [#59](https://github.com/sacloud/skr/issues/59) で対応予定
- [ ] `addon`
- [ ] `apigw`
- [ ] `apprun`
- [ ] `cloudhsm`
- [ ] `dedicated-storage`
- [ ] `kms`
- [ ] `monitoring-suite`
- [x] `networking-suite` — `networking-suite-api` でサブネットグループ、サブネット、インターフェースコネクション、アドレスの SDK 公開14操作を実装済みです。別サブネット間の TCP 疎通を確認する is1b [ライブ E2E runner](test/e2e/networking-suite/) と[チュートリアル](docs/manual/tutorials/networking-suite-api.md)があります。cloudimg Archive の Server A から Server B へ TCP 接続し、`PONG networking-suite` を受信するライブ検証を完了し、一時リソースを削除しました。共有 NIC と Networking Suite NIC を併用する Server A では、ゲスト OS 側で NWS NIC の DHCP と経路を設定する必要があるため、runner とチュートリアルで MAC アドレスに一致する netplan を設定します。Server A は利用者の状態確認用に起動したまま保持しています
- [ ] `nosql`
- [ ] `object-storage`
- [ ] `secretmanager`
- [ ] `security-control`
- [ ] `service-endpoint-gateway`
- [ ] `webaccel`
- [ ] `workflows`

## 当面対応しない API

次の API は当面の実装対象から外し、未実装の対応候補とは分けて管理します。

- `sshkey`
- `sim`
- `esme`
- `enhanceddb`
- `certificateauthority`
- `authstatus`

## 進め方

1. SDK のサービス／API ごとに公開操作とリクエスト・レスポンス型を確認します。
2. CLI に公開する操作、コマンド名、必須入力、ヘルプ、個別処理を明示します。全リソースに同一の CRUD 操作を仮定しません。
3. 既存の生成器を使える操作は設定と生成コードを追加し、生成器の対象外となる固有処理は手書きで実装します。
4. 対象パッケージのテストで SDK 呼び出し、入力、出力、エラー経路を検証し、関連ドキュメントを更新してから一覧の状況を反映します。
