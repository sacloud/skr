# API コマンド対応計画

## 対象と方針

対象 SDK は `go.mod` で指定している [sacloud-sdk-go v0.3.0](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0) です。IaaS は `service/iaas` のサービスパッケージ、IaaS 以外は `api` 直下の API ドメインを棚卸ししています。`service/iaas` に含まれる API でも、CommonServiceItem を使うものは `iaas-api` のリソース候補と分けて管理します。

一覧は SDK の提供範囲を把握するためのもので、すべてのパッケージをそのまま CLI コマンドにすることを決めるものではありません。各パッケージで公開操作、入力、出力、固有の検証を確認し、対応範囲を明示して段階的に実装します。コマンドの階層は `<domain>-api <resource>` とし、SDK の型だけから公開操作や入力の意味を推測しません。詳細は [ADR 0016](docs/adr/0016-generate-low-level-api-commands.md) に従います。

API コマンド生成器は、IaaS 用の `cmd/apigen-iaas` / `generate-iaas-api` と、IaaS 以外の Ogen 系 API 用の共通入口 `cmd/apigen-api` / `generate-api` を使います。EventBus、SimpleMQ などの Ogen 系 API は、サービスごとに生成器入口を増やさず、操作・引数・ヘルプを設定で明示して生成します。IaaS と Ogen 系 API は異なる SDK 形式に合わせて別の入口を維持し、共通化できる実装のみ共有します。入口があることは、そのドメインの全操作が CLI に実装済みであることを意味しません。

SimpleMQ のキュー／メッセージ API コマンドは [`api/commands/simplemq-queue.json`](api/commands/simplemq-queue.json) と [`api/commands/simplemq-message.json`](api/commands/simplemq-message.json) を使って生成します。SDK 固有の入力補完や出力整形が必要な操作は手書きハンドラーに残します。

## IaaS リソース候補

SDK のサービスパッケージごとに対応状況を管理します。

- [ ] `archive`
- [ ] `bridge`
- [ ] `cdrom`
- [ ] `coupon`
- [ ] `database`
- [ ] `disk`
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
- [ ] `server`
- [ ] `sshkey`
- [ ] `subnet`
- [ ] `vpcrouter`

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
- [ ] `zone`

## IaaS 以外の API ドメイン候補

SDK の `api` 直下には、IaaS 以外に次の API ドメインがあります。CLI コマンドの実装、ライブ E2E、チュートリアルの対応状況は分けて記載します。コマンドの入口があることは、そのドメインの SDK 操作すべてへの対応を意味しません。

- [x] `iam` — `iam-api` コマンド、ライブ E2E、[チュートリアル](docs/manual/tutorials/iam-api.md)を実装済みです。
- [x] `eventbus` — `eventbus-api`。
- [x] `simplemq` — `simplemq-api`。キュー／メッセージコマンドは `apigen-api` を使います。
- [ ] `apprun-dedicated` — `apprun-dedicated-api`
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
