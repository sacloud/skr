# API コマンド対応計画

## 対象と方針

対象 SDK は `go.mod` で指定している [sacloud-sdk-go v0.3.0](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0) です。IaaS は `service/iaas` のサービスパッケージ、IaaS 以外は `api` 直下の API ドメインを棚卸ししています。

一覧は SDK の提供範囲を把握するためのもので、すべてのパッケージをそのまま CLI コマンドにすることを決めるものではありません。各パッケージで公開操作、入力、出力、固有の検証を確認し、対応範囲を明示して段階的に実装します。コマンドの階層は `<domain>-api <resource>` とし、SDK の型だけから公開操作や入力の意味を推測しません。詳細は [ADR 0016](docs/adr/0016-generate-low-level-api-commands.md) に従います。

API コマンド生成器は、IaaS 用の `cmd/apigen-iaas` / `generate-iaas-api` と、IaaS 以外の Ogen 系 API 用の共通入口 `cmd/apigen-api` / `generate-api` を使います。EventBus、SimpleMQ などの Ogen 系 API は、サービスごとに生成器入口を増やさず、操作・引数・ヘルプを設定で明示して生成します。IaaS と Ogen 系 API は異なる SDK 形式に合わせて別の入口を維持し、共通化できる実装のみ共有します。入口があることは、そのドメインの全操作が CLI に実装済みであることを意味しません。

SimpleMQ のキュー／メッセージ API コマンドは [`api/commands/simplemq-queue.json`](api/commands/simplemq-queue.json) と [`api/commands/simplemq-message.json`](api/commands/simplemq-message.json) を使って生成します。SDK 固有の入力補完や出力整形が必要な操作は手書きハンドラーに残します。

## IaaS リソース候補

SDK のサービスパッケージごとに対応状況を管理します。`swytch` は `skr iaas-api switch` の入口が実装済みです。その他は未対応候補です。

| 状況 | SDK サービスパッケージ |
| --- | --- |
| 未対応 | `archive` |
| 未対応 | `autobackup` |
| 未対応 | `autoscale` |
| 未対応 | `bridge` |
| 未対応 | `cdrom` |
| 未対応 | `certificateauthority` |
| 未対応 | `containerregistry` |
| 未対応 | `coupon` |
| 未対応 | `database` |
| 未対応 | `disk` |
| 未対応 | `dns` |
| 未対応 | `enhanceddb` |
| 未対応 | `esme` |
| 未対応 | `gslb` |
| 未対応 | `icon` |
| 未対応 | `iface` |
| 未対応 | `internet` |
| 未対応 | `ipaddress` |
| 未対応 | `ipv6addr` |
| 未対応 | `ipv6net` |
| 未対応 | `license` |
| 未対応 | `loadbalancer` |
| 未対応 | `localrouter` |
| 未対応 | `mobilegateway` |
| 未対応 | `nfs` |
| 未対応 | `note` |
| 未対応 | `packetfilter` |
| 未対応 | `privatehost` |
| 未対応 | `proxylb` |
| 未対応 | `server` |
| 未対応 | `sim` |
| 未対応 | `simplemonitor` |
| 未対応 | `sshkey` |
| 未対応 | `subnet` |
| 入口あり | `swytch` — `switch` |
| 未対応 | `vpcrouter` |

## IaaS の補助・プラン系サービス候補

これらも SDK にサービスパッケージがあります。リソース操作コマンドと同じ形にせず、公開する操作と適切な CLI の配置を個別に確認します。

| 状況 | SDK サービスパッケージ |
| --- | --- |
| 未対応 | `authstatus` |
| 未対応 | `bill` |
| 未対応 | `diskplan` |
| 未対応 | `internetplan` |
| 未対応 | `licenseinfo` |
| 未対応 | `privatehostplan` |
| 未対応 | `region` |
| 未対応 | `serverplan` |
| 未対応 | `serviceclass` |
| 未対応 | `zone` |

## IaaS 以外の API ドメイン候補

SDK の `api` 直下には、IaaS 以外に次の API ドメインがあります。`eventbus-api` と `simplemq-api` はコマンドの入口が実装済みです。入口の実装は、そのドメインの SDK 操作すべてへの対応を意味しません。

| 状況 | SDK API ドメイン |
| --- | --- |
| 未対応 | `addon` |
| 未対応 | `apigw` |
| 未対応 | `apprun` |
| 未対応 | `apprun-dedicated` |
| 未対応 | `cloudhsm` |
| 未対応 | `dedicated-storage` |
| 入口あり | `eventbus` — `eventbus-api` |
| 未対応 | `iam` |
| 未対応 | `kms` |
| 未対応 | `monitoring-suite` |
| 未対応 | `networking-suite` |
| 未対応 | `nosql` |
| 未対応 | `object-storage` |
| 未対応 | `secretmanager` |
| 未対応 | `security-control` |
| 未対応 | `service-endpoint-gateway` |
| 未対応 | `simple-notification` |
| 入口あり | `simplemq` — `simplemq-api` (queue / message commands use `apigen-api`) |
| 未対応 | `webaccel` |
| 未対応 | `workflows` |

## 進め方

1. SDK のサービス／API ごとに公開操作とリクエスト・レスポンス型を確認します。
2. CLI に公開する操作、コマンド名、必須入力、ヘルプ、個別処理を明示します。全リソースに同一の CRUD 操作を仮定しません。
3. 既存の生成器を使える操作は設定と生成コードを追加し、生成器の対象外となる固有処理は手書きで実装します。
4. 対象パッケージのテストで SDK 呼び出し、入力、出力、エラー経路を検証し、関連ドキュメントを更新してから一覧の状況を反映します。
