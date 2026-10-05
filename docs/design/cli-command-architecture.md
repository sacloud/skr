# CLI コマンド体系の設計

- Status: Proposed
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`

## 目的

skr では、SDK の API 操作を直接実行する低レベルコマンドと、複数の API 操作をユーザーの作業単位にまとめる高レベルコマンドを分けます。低レベルコマンドは SDK のカバレッジを広く保ち、高レベルコマンドは実際のユースケースに沿った手順や差分管理を提供します。

コマンド名、入力形式、エラー、出力は skr として設計します。従来の usacloud との互換性は前提にしません。

## コマンドの階層

```text
skr <domain>-api <resource> <sdk-operation> ...
skr <domain> <workflow-or-resource> <operation> ...
```

| 階層 | 例 | 役割 |
| --- | --- | --- |
| 低レベル API | `skr iaas-api ...`、`skr eventbus-api ...` | SDK の公開 API 操作を直接実行します。SDK のリソース、操作名、入力モデル、戻り値を基本的にそのまま公開します。 |
| 高レベル | `skr iaas ...`、`skr event ...` | skr 内の service layer をコマンドから利用し、SDK の複数の API 操作を作業単位として提供します。定義ファイル、差分表示、適用、状態確認などを必要に応じて実装します。 |

`iaas-api` と `eventbus-api` は、API を直接操作したい利用者やスクリプト向けです。`iaas` と `event` は、実施手順や複数リソース間の関係をコマンド側で扱いたい利用者向けです。同じリソースを扱う場合でも、低レベルコマンドは SDK 操作の薄い入口、高レベルコマンドは明示的に設計したワークフローとして責務を分けます。

## 低レベル API コマンド

### SDK 操作との対応

低レベルコマンドは SDK の公開された操作メソッドを対象に生成します。対象とする各操作メソッドを CLI 上の操作に 1 対 1 で対応させ、操作をまとめたり、SDK にない操作を補ったりしません。SDK 側の `Find`、`Read`、`Create`、個別操作などの違いもコマンド名に保持します。

対応表は固定の CRUD 一覧ではなく、利用中の SDK バージョンが提供する公開操作を根拠にします。コンストラクター、内部実装、単なる変換・補助関数は CLI 操作にしません。context の有無だけが異なる同一操作のバリアントは別コマンドにせず、CLI の実行コンテキストを使って適切な SDK メソッドを呼び出します。

### 入出力と実装方針

- SDK のリクエストモデルを入力に、SDK の戻り値を出力に使用します。リソース固有の型や union 型を汎用 JSON に置き換えて意味を失わせないようにします。
- API 操作に必要な識別子、zone、request などは、その SDK メソッドの引数に沿って受け取ります。位置引数や JSON ファイルなどの具体的な表現は、対象メソッドのシグネチャと入力モデルを調査して定めます。
- SDK が出すエラーを呼び出し元へ返し、失敗を成功のような出力に変換しません。認証、endpoint、profile は sacloud-sdk-go の既存の仕組みを使います。
- AI による生成を前提としますが、生成物は SDK の公開シグネチャとモデルに照らして確認し、ヘルプ・入出力・エラー経路をテストします。生成コードの手編集を避け、SDK 更新時に対象コマンドを再確認できる形にします。
- コマンドヘルプには、SDK の操作、必要な入力、戻り値、リソース固有の注意点を記載します。API の意味や制約は SDK のドキュメントまたは公式マニュアルで確認できた範囲に限ります。

既存の [API コマンド生成スキル](../../.agents/skills/generate-skr-api-command/SKILL.md)にある、SDK インターフェースを使うこと、コマンドヘルプをユーザーガイドとして扱うこと、sakumock で API 経路を検証することを生成後の品質基準にも適用します。

## 高レベルコマンド

### サービス層

高レベルコマンドで必要となる処理は、skr 内の service layer として実装し、CLI はその層を呼び出します。この service layer は SDK の API 操作を組み合わせ、定義の適用やリソース間の調整などユーザーの作業単位を提供します。SDK 内のディレクトリ名やパッケージ配置だけから、高レベル CLI の境界を決めません。高レベルコマンドから低レベル CLI プロセスを起動するのではなく、SDK の API と skr 内の service layer を直接呼び出します。

高レベル操作は低レベル操作と同じリソースを使えますが、SDK 操作との 1 対 1 対応は要求しません。操作順序、参照 ID、検証、差分、失敗時の扱いを明示し、暗黙に作成・削除する範囲をヘルプとドキュメントで説明します。

### IaaS

`skr iaas-api` は sacloud-sdk-go が公開する IaaS の API 操作を直接公開する境界とします。現状の IaaS API が OpenAPI 定義ではないことは、SDK の定義形式の違いであり、高レベル層であることを意味しません。低レベルコマンドは、OpenAPI 定義の有無によらず、SDK の公開メソッドと型を基準に生成します。

`skr iaas` の高レベル操作が必要になった場合は、SDK の API 操作を組み合わせる skr 内の service layer を実装し、それをコマンドから呼び出します。SDK の `service/iaas/<resource>` という配置を、そのまま `skr iaas` の実装境界とみなす設計ではありません。全リソースに同じ操作があるとは仮定せず、SDK の実際の操作とサービス上の意味を確認して公開範囲を決めます。

### EventBus

`skr eventbus-api` は `sacloud-sdk-go/api/eventbus` の操作を直接公開します。SDK `v0.3.0` では process-configuration、schedule、trigger の操作があり、process-configuration には `UpdateSecret` もあります。現在の EventBus チュートリアルにある作成順序などの利用手順は、高レベル `skr event` を設計する際の根拠にできます。

`skr event` は EventBus のイベント設定をユーザーの作業単位として扱う高レベルコマンドの候補です。
AppRun CLI のように、定義の初期化・表示、差分確認、適用、状態確認をまとめる方向を検討します。
たとえば、1つの定義で process-configuration と trigger を関連付け、参照関係を解決する構成が考えられます。
具体的な定義形式、ID の追跡方法、コマンド名、削除の所有権境界は、EventBus のユースケースを確認して決定します。

EventBus の secret は通常の定義表示や差分に含めず、専用の安全な入力経路で扱います。高レベル操作の一部として secret を登録する場合も、標準出力、ログ、定義ファイル、状態保存先に値を記録しません。

## sacloud-sdk-go の対象範囲

本設計の確認対象は、リポジトリの `go.mod` が指定する sacloud-sdk-go `v0.3.0` です。SDK 内の API 定義形式やディレクトリ構成は領域ごとに異なります。これらは低レベルコマンドを SDK の公開メソッドに対応させるための調査対象であり、CLI の高レベル層を決めるものではありません。

SDK の `api/` 直下で確認した 21 の API グループは次のとおりです。

```text
addon
apigw
apprun
apprun-dedicated
cloudhsm
dedicated-storage
eventbus
iaas
iam
kms
monitoring-suite
networking-suite
nosql
object-storage
secretmanager
security-control
service-endpoint-gateway
simple-notification
simplemq
webaccel
workflows
```

SDK の `service/` 直下には `iaas` と `webaccel` があり、`service/iaas/` には次のリソース／操作パッケージがあります。これは SDK 内部の既存構成の記録であり、これらを skr の高レベル層としてそのまま採用する決定ではありません。

```text
archive
authstatus
autobackup
autoscale
bill
bridge
cdrom
certificateauthority
containerregistry
coupon
database
disk
diskplan
dns
enhanceddb
esme
gslb
icon
iface
internet
internetplan
ipaddress
ipv6addr
ipv6net
license
licenseinfo
loadbalancer
localrouter
mobilegateway
nfs
note
packetfilter
privatehost
privatehostplan
proxylb
region
server
serverplan
serviceclass
setup
sim
simplemonitor
sshkey
subnet
swytch
vpcrouter
zone
```

この一覧は SDK `v0.3.0` 時点の調査スナップショットであり、すべてを一括実装する約束ではありません。新しい SDK バージョンを採用するときは API グループ、service package、公開操作を再調査し、低レベル生成対象と高レベルで扱う範囲を更新します。

## AppRun CLI を参考にする範囲

[fujiwara/apprun-cli](https://github.com/fujiwara/apprun-cli) は、高レベル CLI のユーザー体験を考える参考にします。定義ファイルを中心に `init`、`render`、`diff`、`deploy`、`status` などの作業をまとめている点を参考にし、skr の各ドメインで必要なライフサイクルを設計します。

コマンド体系や実装、設定形式、依存関係をそのまま移植するものではありません。さくらのクラウドの動作や制約は、sacloud-sdk-go と公式マニュアルで確認し、AppRun CLI の挙動から推測しません。

## テストとドキュメント

- 低レベル API コマンドでは、実際の CLI パスから SDK のリクエスト／レスポンスに至る経路を sakumock で検証し、SDK 型の JSON 入出力とエラーを確認します。
- 高レベルコマンドでは、定義の読み込み、依存関係の解決、差分、API 呼び出し順序、失敗時の動作、secret の非表示を対象にテストします。API 呼び出し経路は利用可能な sakumock でも確認します。
- 新たなユーザー向けコマンドは、CLI ヘルプと関連ドキュメントに入力、出力、必要な認証、作成・削除されるリソースを記載します。
- SDK の更新で生成対象が変わる場合は、API グループ、メソッド対応表、ヘルプ、テストを更新します。

## 参照資料

- [`go.mod`](../../go.mod) — sacloud-sdk-go の採用バージョン
- `github.com/sacloud/sacloud-sdk-go/api/` — API グループ
- `github.com/sacloud/sacloud-sdk-go/service/iaas/` — SDK 内の IaaS 関連パッケージ構成
- `github.com/sacloud/sacloud-sdk-go/api/eventbus/` — EventBus 操作
- [EventBus API チュートリアル](../tutorials/eventbus-api.md)
- [API コマンド生成スキル](../../.agents/skills/generate-skr-api-command/SKILL.md)
- [fujiwara/apprun-cli](https://github.com/fujiwara/apprun-cli)
