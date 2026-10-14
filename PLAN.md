# API コマンド対応計画

## 対象と方針

対象 SDK は `go.mod` で指定している [sacloud-sdk-go v0.3.0](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0) です。IaaS は `service/iaas` のサービスパッケージ、IaaS 以外は `api` 直下の API ドメインを棚卸ししています。`service/iaas` に含まれる API でも、CommonServiceItem を使うものは `iaas-api` のリソース候補と分けて管理します。

一覧は SDK の提供範囲を把握するためのもので、すべてのパッケージをそのまま CLI コマンドにすることを決めるものではありません。各パッケージで公開操作、入力、出力、固有の検証を確認し、対応範囲を明示して段階的に実装します。コマンドの階層は `<domain>-api <resource>` とし、SDK の型だけから公開操作や入力の意味を推測しません。詳細は [ADR 0016](docs/adr/0016-generate-low-level-api-commands.md) に従います。

API コマンド生成器は、IaaS 用の `cmd/apigen-iaas` / `generate-iaas-api` と、IaaS 以外の Ogen 系 API 用の共通入口 `cmd/apigen-api` / `generate-api` を使います。EventBus、SimpleMQ などの Ogen 系 API は、サービスごとに生成器入口を増やさず、操作・引数・ヘルプを設定で明示して生成します。IaaS と Ogen 系 API は異なる SDK 形式に合わせて別の入口を維持し、共通化できる実装のみ共有します。入口があることは、そのドメインの全操作が CLI に実装済みであることを意味しません。

SimpleMQ のキュー／メッセージ API コマンドは [`api/commands/simplemq-queue.json`](api/commands/simplemq-queue.json) と [`api/commands/simplemq-message.json`](api/commands/simplemq-message.json) を使って生成します。SDK 固有の入力補完や出力整形が必要な操作は手書きハンドラーに残します。

この一覧の `[x]` は、説明に列挙した操作が CLI に実装済みであることを示します。SDK の全操作への対応を示すものではありません。ライブ E2E runner とチュートリアルの有無は、それぞれ別に記載します。

## IaaS リソース候補

SDK のサービスパッケージごとに対応状況を管理します。実装済みコマンドの操作範囲は「実装済み IaaS コマンドの対応範囲」に記載し、ここには未実装の候補を記載します。

- [ ] `archive`
- [ ] `bridge`
- [ ] `cdrom`
- [ ] `coupon`
- [ ] `database`
- [ ] `icon`
- [ ] `iface`
- [ ] `internet`
- [ ] `ipaddress`
- [ ] `ipv6addr`
- [ ] `ipv6net`
- [ ] `license`
- [ ] `loadbalancer`
- [ ] `mobilegateway`
- [ ] `nfs`
- [ ] `note`
- [ ] `packetfilter`
- [ ] `privatehost`
- [ ] `sshkey`
- [ ] `subnet`
- [ ] `vpcrouter`

## 実装済み IaaS コマンドの対応範囲

チェックは列挙した操作の CLI 実装状況を示します。SDK にある未対応操作、ライブ E2E、チュートリアルの状況も併記します。

- [x] `disk` — `iaas-api disk` の Find / Read / Create / Update / Delete を実装済みです。SDK の Apply / ConnectToServer / DisconnectFromServer / Edit / MonitorDisk / ResizePartition / WaitReady は未対応です。is1b で作成・検索・参照・更新・削除と削除後を確認する [ライブ E2E runner](test/e2e/disk/) と[チュートリアル](docs/manual/tutorials/iaas-api/disk.md)があります。
- [x] `server`
    - [x] Find / Read / Create / Boot / Update / Delete を実装済みです。
    - Create では基本項目に加え、反復可能な `--disk` と `--nic` で複数ディスクと共有／未接続 NIC を指定できます。Boot は user-data のファイル／標準入力に対応します。
    - [ ] SDK の Apply / ChangePlan / EjectCDROM / InsertCDROM / MonitorCPU / Reset / SendNMI / Shutdown / WaitBoot / WaitShutdown は **未対応**です。
    - is1b で新記法による cloud-init 起動・SSH 接続・削除・一時 SSH ファイルの後始末を確認済みです。[ライブ E2E runner](test/e2e/server/) と[チュートリアル](docs/manual/tutorials/iaas-api/server.md)があります。設計は [ADR 0040](docs/adr/0040-server-cloud-init-and-ssh-tutorial.md)、[ADR 0041](docs/adr/0041-server-repeatable-disk-and-nic-options.md) と[設計文書](docs/design/server-cloud-init-ssh.md)を参照してください。
- [x] `switch` — SDK の `swytch` に対応する `iaas-api switch` の Find / Read / Create / Update / Delete を実装済みです。SDK の公開操作はすべて CLI にあります。Sandbox と is1b で作成・検索・参照・更新・削除を検証する [ライブ E2E runner](test/e2e/switch/) と[チュートリアル](docs/manual/tutorials/iaas-api/switch.md)があります。

## CommonServiceItem 系 API 候補

SDK の `service/iaas` にありますが、CommonServiceItem を使う API は通常の `iaas-api <resource>` 候補と分けます。CLI では `containerregistry` の例のように、`container-registry-api` などサービスごとの API コマンドにすることを想定します。
Simple Notification の group／destination API も CommonServiceItem を使いますが、候補一覧では非 IaaS API ドメイン欄の `simple-notification` にまとめます。

- [ ] `autobackup`
- [ ] `autoscale`
- [ ] `certificateauthority`
- [x] `containerregistry` — `container-registry-api` でレジストリと認証ユーザーを管理します。ライブ E2E runner と[チュートリアル](docs/manual/tutorials/container-registry-api.md)を追加済み
- [ ] `dns`
- [ ] `enhanceddb`
- [ ] `esme`
- [ ] `gslb`
- [ ] `localrouter`
- [ ] `proxylb`
- [ ] `simplemonitor`
- [ ] `sim`

## IaaS の補助・プラン系サービス候補

これらも SDK にサービスパッケージがあります。リソース操作コマンドと同じ形にせず、公開する操作と適切な CLI の配置を個別に確認します。

- [ ] `authstatus`
- [ ] `bill`
- [ ] `diskplan`
- [ ] `internetplan`
- [ ] `licenseinfo`
- [ ] `privatehostplan`
- [ ] `region`
- [ ] `serverplan`
- [ ] `serviceclass`
- [x] `zone` — `iaas-api zone` の Find のみ実装済みです。SDK の Read は未対応です。ゾーン一覧を取得して jq で名前を抽出する読み取り専用の [ライブ E2E runner](test/e2e/zone/) と[チュートリアル](docs/manual/tutorials/iaas-api/zone.md)があります

## IaaS 以外の API ドメイン候補

SDK の `api` 直下には、IaaS 以外に次の API ドメインがあります。CLI コマンドの実装、ライブ E2E、チュートリアルの対応状況は分けて記載します。コマンドの入口があることは、そのドメインの SDK 操作すべてへの対応を意味しません。

- [x] `iam` — `iam-api` コマンド、ライブ E2E、[チュートリアル](docs/manual/tutorials/iam-api.md)を実装済みです。
- [x] `eventbus` — `eventbus-api`。Provider 補完・リクエスト検証・シークレット入力とエラー伝播のドメイン単体テストを実装済みです。
- [x] `simplemq` — `simplemq-api`。キュー／メッセージコマンドは `apigen-api` を使います。
- [ ] `apprun-dedicated` — `apprun-dedicated-api`。リクエスト入力・秘密情報のインライン拒否・エラー伝播のドメイン単体テストを実装済みです。
  - CLI 実装済み、ライブ E2E とチュートリアルは未対応。不足分は [#59](https://github.com/sacloud/skr/issues/59) で対応予定
- [ ] `addon`
- [ ] `apigw`
- [ ] `apprun`
- [ ] `cloudhsm`
- [ ] `dedicated-storage`
- [ ] `kms`
- [ ] `monitoring-suite`
- [ ] `networking-suite`
- [ ] `nosql`
- [ ] `object-storage`
- [ ] `secretmanager`
- [ ] `security-control`
- [ ] `service-endpoint-gateway`
- [ ] `simple-notification`
- [ ] `webaccel`
- [ ] `workflows`

## 進め方

1. SDK のサービス／API ごとに公開操作とリクエスト・レスポンス型を確認します。
2. CLI に公開する操作、コマンド名、必須入力、ヘルプ、個別処理を明示します。全リソースに同一の CRUD 操作を仮定しません。
3. 既存の生成器を使える操作は設定と生成コードを追加し、生成器の対象外となる固有処理は手書きで実装します。
4. 対象パッケージのテストで SDK 呼び出し、入力、出力、エラー経路を検証し、関連ドキュメントを更新してから一覧の状況を反映します。
