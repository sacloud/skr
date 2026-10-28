# API コマンド対応計画

## 対象と方針

対象 SDK は `go.mod` で指定している [sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1) です。IaaS は `service/iaas` のサービスパッケージ、IaaS 以外は `api` 直下の API ドメインを棚卸ししています。SDK の配置ではなく実際の API route とドメインに基づいて CLI の境界を決めます。`service/iaas` に含まれる API でも、System API や CommonServiceItem を使うものは `iaas-api` のリソース候補と分けて管理します。

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
- [x] `internet` — `iaas-api internet` の Find / Read / Create / Update / Delete、サブネット操作、IPv6 操作、MonitorRouter を実装済みです。Find は `--name` による単一名の部分一致検索、Delete は `--force` と `--fail-if-not-found` に対応します。ルータ＋スイッチ、Server、起動 Disk を作成し、サブネットから選んだ IPv4 アドレスを NIC の OS 設定へ適用して SSH 接続する [ライブ E2E runner](test/e2e/internet/) と[チュートリアル](docs/manual/tutorials/iaas-api/internet.md)があります。個別オプションだけの操作で is1b の作成・NIC 接続・名前検索・サブネット参照・SSH ログイン・削除後の不在に加え、Update(更新と復元)、MonitorRouter、追加サブネットの追加・NextHop 更新・削除、IPv6 の有効化・参照・無効化のライブ検証を確認済みです。
- [x] `icon` — `iaas-api icon` の Find / Read / Create / Update / Delete を実装済みです。Create は `--image-file` で画像ファイルを読み込み、API 用の Base64 文字列へ変換します。更新できる項目は Name と Tags です。CRUD の[ライブ E2E runner](test/e2e/icon/) と[チュートリアル](docs/manual/tutorials/iaas-api/icon.md)を追加済みです。ライブ E2E で作成・検索・参照・更新・削除と削除後の不在を確認済みです。
- [x] `packetfilter` — `iaas-api packetfilter` の Find / Read / Create / Update / Delete を実装済みです。ルールの Expression 配列は `--expressions` の JSON 値または `--request` で指定し、ゾーン、名前、ID などの単純値はフラグで指定できます。is1b で作成・検索・参照・ルール更新・削除を確認する [ライブ E2E runner](test/e2e/packetfilter/) と[チュートリアル](docs/manual/tutorials/iaas-api/packetfilter.md)があります。ライブ E2E で各操作と削除後の不在を確認済みです。NIC への適用とパケット通信の検証は対象外です。
- [x] `server`
    - [x] Find / Read / Create / Boot / Update / Delete を実装済みです。
    - Create では基本項目に加え、反復可能な `--disk` と `--nic` で複数ディスクと共有／未接続／スイッチ ID 接続の NIC を指定できます。Find は `--name` による単一名の部分一致検索に対応します。Boot は user-data のファイル／標準入力に対応します。
    - Create の定型的な入力処理・検証・API 呼び出しは生成し、ディスク／NIC の固有変換だけを型付き callback に接続します。生成への移行後も is1b のライブ E2E で作成・起動・SSH 接続・削除後の不在を確認済みです。生成器との境界は [ADR 0042](docs/adr/0042-generated-request-transform.md) に記載しています。Boot は手書きを維持します。
    - [ ] SDK の Apply / ChangePlan / EjectCDROM / InsertCDROM / MonitorCPU / Reset / SendNMI / Shutdown / WaitBoot / WaitShutdown は **未対応**です。
    - is1b で新記法による cloud-init 起動・SSH 接続・削除・一時 SSH ファイルの後始末、Update(更新・復元)のライブ検証を確認済みです。2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従う testing.T ベースのシナリオテスト ([test/e2e/server/](test/e2e/server/)) へ移行し、SSH 鍵の生成と SSH 接続の確認は [helper](test/e2e/internal/helper/) ([ADR 0063](docs/adr/0063-reusable-ssh-and-local-command-recording.md)) に移しました。同じ日に移行後のシナリオをライブ再実行し、cloud-init 起動・SSH 接続・更新・復元・削除と後片付けを確認済みです。同日に作成前の同名確認、後片付けの再照合と不在確認、起動待ちのポーリングを SDK で直接実行するよう変更しました ([ADR 0064](docs/adr/0064-e2e-safety-checks-via-sdk.md))。[チュートリアル](docs/manual/tutorials/iaas-api/server.md)があります。設計は [ADR 0040](docs/adr/0040-server-cloud-init-and-ssh-tutorial.md)、[ADR 0041](docs/adr/0041-server-repeatable-disk-and-nic-options.md) と[設計文書](docs/design/server-cloud-init-ssh.md)を参照してください。
- [x] `switch` — SDK の `swytch` に対応する `iaas-api switch` の Find / Read / Create / Update / Delete を実装済みです。SDK の公開操作はすべて CLI にあります。is1b で作成・検索・参照・更新・削除を検証する [ライブ E2E](test/e2e/switch/)（testing.T ベースのシナリオ）と[チュートリアル](docs/manual/tutorials/iaas-api/switch.md)があります。2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行し、同じ日に移行後のシナリオをライブ再実行して作成・検索・射影・参照・更新・削除と後片付けを確認済みです。

## CommonServiceItem 系 API 候補

SDK の `service/iaas` にありますが、CommonServiceItem を使う API は通常の `iaas-api <resource>` 候補と分けます。CLI では `containerregistry` の例のように、`container-registry-api` などサービスごとの API コマンドにすることを想定します。
Simple Notification の group／destination API も CommonServiceItem を使いますが、候補一覧では非 IaaS API ドメイン欄の `simple-notification` にまとめます。

- [ ] `autobackup`
- [ ] `autoscale`
- [x] `containerregistry` — `container-registry-api` でレジストリと認証ユーザーを管理します。ライブ E2E テストと[チュートリアル](docs/manual/tutorials/container-registry-api.md)を追加済み
- [ ] `dns`
- [x] `gslb` — `gslb-api` で Find / Read / Create / Update / Delete を実装済みです。単純値、ヘルスチェックのフラットなフラグ、反復可能な `--destination-server` と JSON 入力に対応します。[is1b ライブ E2E runner](test/e2e/gslb/) で実サーバーとソーリーサーバーを各1台作成し、ping 監視による停止・復帰時の応答先切り替えを `NOERROR` と期待 IP の完全一致で確認済みです。GSLB の CRUD、両サーバーの削除、ディスクの個別削除と各リソースの不在も確認済みです。以前の単一宛先・空応答による停止確認は不十分なため、この構成で置き換えました。[チュートリアル](docs/manual/tutorials/gslb-api.md)では両サーバーの準備、ソーリーサーバーへの切り替えと復帰、更新、ディスクの個別削除を説明します
- [ ] `localrouter`
- [ ] `proxylb`
- [ ] `simplemonitor`

## IaaS の補助・プラン系サービス候補

IaaS API のコマンド階層は SDK のサービスパッケージ名ではなく、実際の API route に合わせます。たとえば `/api/cloud/1.1/product/server` は `iaas-api product server`、`/api/cloud/1.1/public/price` は `iaas-api public price` です。低レベル API 操作が SDK にある場合は `sacloud-sdk-go/api/iaas` を直接使い、Find の応答 envelope を含む API 応答モデルを JSON 出力します。JSON は SDK の型から再エンコードするため HTTP body のバイト列そのものではありません。既存 IaaS コマンドの他の出力経路は別途段階的に見直します。

SDK v0.3.1 の route を確認し、未実装候補の CLI 階層を次のように整理します。

| API | route | CLI 階層 |
| --- | --- | --- |
| InternetPlan | `/api/cloud/1.1/product/internet` | `product internet` |
| LicenseInfo | `/api/cloud/1.1/product/license` | `product license` |
| PrivateHostPlan | `/api/cloud/1.1/product/privatehost` | `product privatehost` |
| Region | `/api/cloud/1.1/region` | `region` |
| ServerPlan | `/api/cloud/1.1/product/server` | `product server` |
| ServiceClass | `/api/cloud/1.1/public/price` | `public price` |
| Zone | `/api/cloud/1.1/zone` | `zone` |
| DiskPlan | `/api/cloud/1.1/product/disk` | `product disk` |

- [x] `region` — `iaas-api region` の Find / Read を実装済みです。Find は件数・開始位置をフラグで指定でき、検索名・Sort は JSON で指定します。Read は ID で参照します。Region API はゾーン指定を受け付けません。SDK の公開操作はすべて CLI にあります。読み取り専用の[ライブ E2E runner](test/e2e/region/) と[チュートリアル](docs/manual/tutorials/iaas-api/region.md)を追加し、実環境で Find / Read の結果一致を確認済みです。
- [x] `serverplan` — `iaas-api product server` の Find / Read を実装済みです。`api/iaas.ServerPlanAPI` を使い、Find の API 応答 envelope を含む JSON を出力します。CPU、メモリ、GPU、CPU モデル、利用可否などを検索・参照できます。読み取り専用の [testing.T ベースのシナリオテスト](test/e2e/serverplan/scenario_serverplan_test.go) ([ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md)) と[チュートリアル](docs/manual/tutorials/iaas-api/server-plan.md)があります。is1b のライブ E2E で Find の結果と Read の ID・プラン情報の一致を確認済みです。2026-10-27 に testing.T ベースのシナリオテストへ移行しました。
- [x] `serviceclass` — 現在は `iaas-api service-class` の Find を実装済みです。指定ゾーンのサービスクラス一覧を取得できます。SDK route は `/public/price` のため、route に沿う `iaas-api public price` への移行は未対応です。読み取り専用の [testing.T ベースのシナリオテスト](test/e2e/serviceclass/scenario_serviceclass_test.go) ([ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md)) と[チュートリアル](docs/manual/tutorials/iaas-api/serviceclass.md)があります。`is1b` でライブ E2E を実行し、517 件のサービスクラスを取得して、各項目の ID・名前・パスを検証済みです。2026-10-27 に testing.T ベースのシナリオテストへ移行しました。
- [x] `zone` — `iaas-api zone` の Find と Read を実装済みです。ゾーン一覧の取得、jq での名前抽出、ID での参照を確認する読み取り専用の[シナリオテスト](test/e2e/zone/scenario_zone_test.go)と[チュートリアル](docs/manual/tutorials/iaas-api/zone.md)があります。
- [x] `diskplan` — `iaas-api product disk` の Find / Read を実装済みです。`api/iaas.DiskPlanAPI` を使い、Find の API 応答 envelope を含む JSON を出力します。検索結果の API `Availability` と `Size` から、指定ゾーンの利用可否とサイズを確認できます。SDK の公開操作はすべて CLI にあります。Archive と合わせて検証する読み取り専用の [ライブ E2E runner](test/e2e/iaas-lookup/) と[チュートリアル](docs/manual/tutorials/iaas-api/disk-plan.md)があります。is1b で検索・参照を確認済みです。
- [x] `internetplan` — `iaas-api product internet` の Find / Read を実装済みです。ゾーン別プランの帯域幅と利用可否を確認できます。
- [x] `licenseinfo` — `iaas-api product license` の Find を実装済みです。API に Read 操作はありません。
- [x] `privatehostplan` — `iaas-api product privatehost` の Find / Read を実装済みです。CPU、CPU モデル、メモリ、利用可否を確認できます。
- 3 リソースの読み取り専用 [ライブ E2E runner](test/e2e/iaas-product-plans/) と[チュートリアル](docs/manual/tutorials/iaas-api/internet-plan.md)、[チュートリアル](docs/manual/tutorials/iaas-api/license-info.md)、[チュートリアル](docs/manual/tutorials/iaas-api/private-host-plan.md)があります。is1b のライブ E2E で InternetPlan / PrivateHostPlan の Find と Read の一致を確認し、LicenseInfo の Find 応答を検証済みです。クラウドリソースの変更はありません。
- コマンド範囲と読み取り専用 E2E の判断は [ADR 0055](docs/adr/0055-iaas-product-plan-api-commands.md) に記録しています。

## IaaS SDK に同居する System API 候補

`service/iaas/bill` は SDK 上では IaaS 配下ですが、対象 route は `/api/system/1.0/bill` であり、IaaS resource route ではありません。そのため `iaas-api` には含めず、独立した `bill-api` コマンドとして扱う方向です。SDK の公開操作に合わせて `bill-api list` と `bill-api csv` を候補とし、応答と CSV の出力形式を含む実装範囲は着手時に確認します。CLI 実装、ライブ E2E、チュートリアルはいずれも未対応です。

## IaaS 以外の API ドメイン候補

SDK の `api` 直下には、IaaS 以外に次の API ドメインがあります。CLI コマンドの実装、ライブ E2E、チュートリアルの対応状況は分けて記載します。コマンドの入口があることは、そのドメインの SDK 操作すべてへの対応を意味しません。

- [x] `iam` — `iam-api` コマンドと[チュートリアル](docs/manual/tutorials/iam-api.md)を実装済みです。[ライブ E2E runner](test/e2e/iam-api/)は59の末端コマンドのうち57コマンドを2026-10-26 にライブ検証済みで、sso link/unlink の2操作は [#144](https://github.com/sacloud/skr/issues/144) で対応します。残りの状況は[ライブ E2E の対応状況](docs/design/live-e2e-coverage.md)で管理します。
- [x] `eventbus` — `eventbus-api`。Provider 補完・リクエスト検証・シークレット入力とエラー伝播のドメイン単体テストを実装済みです。[ライブ E2E](test/e2e/eventbus-api/)は process-configuration と trigger の作成・参照・削除、update-secret、schedule のライフサイクル、SimpleMQ 連携を検証済みです。2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行し、全工程を再実行しました。
- [x] `simplemq` — `simplemq-api`。キュー／メッセージコマンドは `apigen-api` を使います。12の末端コマンドを1シナリオで実行する[ライブ E2E](test/e2e/simplemq/)（testing.T ベースのシナリオ）と[チュートリアル](docs/manual/tutorials/simplemq-api.md)があります。2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行し、同じ日に移行後のシナリオをライブ再実行して作成・参照・射影・設定更新・API キー発行・メッセージ操作・削除を確認済みです。
- [x] `simple-notification` — `simple-notification-api`。通知先、グループ、ルーティング、履歴の SDK 公開操作を実装済みです。送信は `--message` と JSON 入力に対応します。[ライブ E2E](test/e2e/simple-notification/)（testing.T ベースのシナリオ）と[チュートリアル](docs/manual/tutorials/simple-notification-api.md)があります。2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行しました。既存通知先へのテスト送信シナリオと、通知を送信しない CRUD シナリオがあります。ライブではメール通知の処理完了、利用者による実受信、履歴の個別取得、一時通知先・グループ・ルーティングの作成／参照／更新／削除、設定省略時の保持と設定変更、一時ルーティングだけの並べ替え、既存設定の保持、削除後の不在を確認済みです。Webhook の実送信・到達確認は [#130](https://github.com/sacloud/skr/issues/130) で対応します。
- [ ] `apprun-dedicated` — `apprun-dedicated-api`。リクエスト入力・秘密情報のインライン拒否・エラー伝播のドメイン単体テストを実装済みです。
  - CLI 実装済み、ライブ E2E とチュートリアルは未対応。不足分は [#59](https://github.com/sacloud/skr/issues/59) で対応予定
- [x] `apprun` — `apprun-api` でアプリケーション CRUD・ステータス、バージョンの一覧／参照／削除／ステータス、トラフィック、パケットフィルター、ユーザー操作を実装済みです。Ogen 系 API 共通生成器の設定と生成コードを使います。認証情報を含み得るアプリケーションの作成・更新リクエストは保護したファイルまたは標準入力に限定します。sakumock による CLI 操作テスト、[ライブ E2E](test/e2e/apprun/)（testing.T ベースのシナリオ、[ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md)）、[チュートリアル](docs/manual/tutorials/apprun-api.md)があります。ライブでは Application の作成・一覧・参照、`Healthy` への遷移、公開 URL の HTTP 200 とサンプル本文、削除と削除後の不在を確認済みです。Application の Update(1 vCPU / 2Gi の新バージョン作成)、Version の一覧・参照・ステータス・削除、Traffic の一覧・更新、PacketFilter の参照・更新、User の参照も 2026-10-23 に確認済みです。2026-10-27 には testing.T ベースのシナリオとして全工程を再実行しました。チュートリアルに応答例と安全な後片付けを反映しました
- [ ] `addon`
- [ ] `apigw`
- [ ] `cloudhsm`
- [ ] `dedicated-storage`
- [ ] `kms`
- [ ] `monitoring-suite`
- [x] `networking-suite` — `networking-suite-api` でサブネットグループ、サブネット、インターフェースコネクション、アドレスの SDK 公開14操作を実装済みです。別サブネット間の TCP 疎通を確認する is1b [ライブ E2E runner](test/e2e/networking-suite/) と[チュートリアル](docs/manual/tutorials/networking-suite-api.md)があります。cloudimg Archive の Server A から Server B へ TCP 接続し、`PONG networking-suite` を受信するライブ検証を完了し、一時リソースを削除しました。共有 NIC と Networking Suite NIC を併用する Server A では、ゲスト OS 側で NWS NIC の DHCP と経路を設定する必要があるため、runner とチュートリアルで MAC アドレスに一致する netplan を設定します。Server A は利用者の状態確認用に起動したまま保持しています
- [ ] `nosql`
- [x] `object-storage` — `object-storage-api` で SDK v0.3.1 の `SiteAPI`、`AccountAPI`、`BucketAPI`、`BucketExtraAPI`、`PermissionsAPI`、`SiteStatusAPI` が公開する全34管理操作を実装済みです。サイト・サイトアカウント・バケット・パーミッション、アクセスキー、暗号化、レプリケーション、状態・クォータ・利用量を操作できます。パーミッションの作成・更新は構造化された JSON を `--request` で受け付けます。バケット作成は `isk01`、`tky01`、`arc02` に対応し、`arc02` では `--plan` を必須にしています。S3 互換 API によるオブジェクト本体の操作は対象外です。testing.T ベースの[ライブ E2E シナリオ](test/e2e/object-storage-api/)の6シナリオで全34コマンドを対象にし、[チュートリアル](docs/manual/tutorials/object-storage-api.md)の手順も含めています。ライブでは29コマンドの正常応答を確認しました。通常・アーカイブバケット、サイトアカウント、パーミッション更新、両種類のキー、サイト間レプリケーションと後片付けが成功しました。状態・クォータ・利用量は東京第1サイトで成功しましたが、`bucket penalty` は404、`site status bucket-metering` は400のため成功未確認です（[#142](https://github.com/sacloud/skr/issues/142)）。暗号化3操作は KMS キー作成が未実装のため、合意により延期しています（[#141](https://github.com/sacloud/skr/issues/141)）。sakumock v0.13.0 の `objectstorage` サーバーに対し、実際の CLI と SDK を経由して全34コマンドの正常系を確認し、403と削除後の404も検証しています。ドメインの単体テストには SDK interface の差し替えも使います。対応表と実行条件は[設計文書](docs/design/object-storage-api.md)に記載しています
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

## ライブ E2E の対応状況

[ADR 0058](docs/adr/0058-live-e2e-command-coverage.md) に従い、公開するすべての末端サブコマンドをライブ E2E で検証します。2026-10-23 時点で末端コマンド一覧と runner 実装を突合しました。

多くの runner は対象コマンドを網羅しています。タスク3完了後の未検証は35コマンドで、apprun-dedicated-api 35 です。

2026-10-23 にタスク1として `server update`、`internet update`、`internet monitor-router`、`version` の4コマンドをライブ検証しました。既存 runner へのシナリオ追加と専用 runner の新規作成、そのライブ実行を完了しています。version の E2E は 2026-10-27 に [ADR 0062](docs/adr/0062-e2e-testing-t-based-scenario.md) に従う testing.T ベースのシナリオテスト ([test/e2e/version/](test/e2e/version/)) へ移行し、同日に移行後のシナリオをライブ再実行しました。

2026-10-23 にタスク2として 25 コマンドをライブ検証しました。internet のサブネット3操作と IPv6 3操作です。eventbus-api の schedule 5操作、process-configuration update、trigger update です。apprun の Update、Version 4操作、Traffic 2操作、PacketFilter 2操作、User 2操作です。

既存 runner へのシナリオ追加とそのライブ実行を完了しています。

2026-10-26 にタスク3として iam-api の59コマンドのうち57コマンドをライブ検証しました。読み取り15操作、リソーススコープの書き込み27操作、アカウント全体やログインユーザーへ影響する書き込み15操作です。sso link/unlink は全組織ユーザーのログイン方法に影響するため、[#144](https://github.com/sacloud/skr/issues/144) で影響範囲の確認と設計を進めます。期待どおりの失敗を確認するステップは、証跡に `expected-error` として記録されます。

網羅表、未検証コマンドの一覧、段階的なタスク、命名ルールの適用状況は[ライブ E2E の対応状況](docs/design/live-e2e-coverage.md)に記録しています。末端コマンドごとの状況は [ADR 0065](docs/adr/0065-e2e-command-coverage-baseline.md) に従い [`test/e2e/command-coverage/baseline.txt`](test/e2e/command-coverage/baseline.txt) に記録し、`make e2e-coverage` で検証します。2026-10-28 の baseline 作成時に、iam-api の旧 runner が `iam-api group update` を実行していない網羅漏れを検出し、未整備として記録しました。

## 進め方

1. SDK のサービス／API ごとに公開操作とリクエスト・レスポンス型を確認します。
2. CLI に公開する操作、コマンド名、必須入力、ヘルプ、個別処理を明示します。全リソースに同一の CRUD 操作を仮定しません。
3. 既存の生成器を使える操作は設定と生成コードを追加し、生成器の対象外となる固有処理は手書きで実装します。
4. 対象パッケージのテストで SDK 呼び出し、入力、出力、エラー経路を検証し、関連ドキュメントを更新してから一覧の状況を反映します。
