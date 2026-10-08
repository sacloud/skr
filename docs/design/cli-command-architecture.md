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
skr http <url> [--method ...] [--data ...]
```

| 階層 | 例 | 役割 |
| --- | --- | --- |
| 低レベル API | `skr iaas-api ...`、`skr eventbus-api ...` | SDK の公開 API 操作を直接実行します。SDK のリソース、操作名、入力モデル、戻り値を基本的にそのまま公開します。 |
| 高レベル | `skr iaas ...`、`skr event ...` | skr 内の service layer をコマンドから利用し、SDK の複数の API 操作を作業単位として提供します。定義ファイル、差分表示、適用、状態確認などを必要に応じて実装します。 |
| 認証付き HTTP | `skr http <url>` | SDK の認証を利用して任意の HTTPS URL へ直接リクエストを送り、レスポンス本文をそのまま出力します。 |

`iaas-api` と `eventbus-api` は、API を直接操作したい利用者やスクリプト向けです。`iaas` と `event` は、実施手順や複数リソース間の関係をコマンド側で扱いたい利用者向けです。同じリソースを扱う場合でも、低レベルコマンドは SDK 操作の薄い入口、高レベルコマンドは明示的に設計したワークフローとして責務を分けます。

`http` は、SDK に対応する API 操作がまだない場合の低レベルな入口です。SDK の認証付き HTTP クライアントへ利用者が指定した HTTPS URL とリクエストを渡します。SDK 対応済みの操作には、サービス固有の型や検証を提供する低レベル API コマンドを優先します。詳細は[認証付き HTTP リクエストコマンドの設計](http-command.md)に記録します。

## 低レベル API コマンド

### SDK 操作との対応

低レベルコマンドは SDK の公開された操作メソッドを対象に生成します。対象とする各操作メソッドを CLI 上の操作に 1 対 1 で対応させ、操作をまとめたり、SDK にない操作を補ったりしません。SDK 側の `Find`、`Read`、`Create`、個別操作などの違いもコマンド名に保持します。

対応表は固定の CRUD 一覧ではなく、利用中の SDK バージョンが提供する公開操作を根拠にします。コンストラクター、内部実装、単なる変換・補助関数は CLI 操作にしません。context の有無だけが異なる同一操作のバリアントは別コマンドにせず、CLI の実行コンテキストを使って適切な SDK メソッドを呼び出します。

### 入出力と実装方針

- SDK のリクエストモデルを入力に、SDK の戻り値を出力に使用します。リソース固有の型や union 型を汎用 JSON に置き換えて意味を失わせないようにします。
- API 操作に必要な識別子、zone、request などは、その SDK メソッドの引数に沿って受け取ります。位置引数や JSON ファイルなどの具体的な表現は、対象メソッドのシグネチャと入力モデルを調査して定めます。
- [ADR 0007](../adr/0007-api-request-input.md) に従い、フラグだけで有効な最小リクエストを表せる操作には名前付きフラグを追加します。複雑な入力には `--request` の JSON を残し、両者の併用はエラーにします。任意値の未指定と明示指定、更新時の省略、`null` の意味を保って SDK のリクエスト型に変換します。
- SDK が出すエラーを呼び出し元へ返し、失敗を成功のような出力に変換しません。認証、endpoint、profile は sacloud-sdk-go の既存の仕組みを使います。
- 定型的なコマンド登録、SDK 操作への接続、リクエスト受け渡しと出力処理は、明示的に選んだ操作の設定から Go コードを生成します。SDK の公開メソッドを自動的にすべて公開せず、操作名、フラグ、必須入力、ヘルプ、サービス固有の処理は明示します。生成物は SDK の公開シグネチャとモデルに照らして確認し、ヘルプ・入出力・エラー経路をテストします。生成物を手編集せず、SDK 更新時に対象コマンドを再生成して確認します。詳細は [ADR 0016](../adr/0016-generate-low-level-api-commands.md) に記録します。
- コマンドヘルプには、SDK の操作、必要な入力、戻り値、リソース固有の注意点を記載します。API の意味や制約は SDK のドキュメントまたは公式マニュアルで確認できた範囲に限ります。

既存の [API コマンド生成スキル](../../.agents/skills/generate-skr-api-command/SKILL.md)にある、SDK インターフェースを使うこと、コマンドヘルプをユーザーガイドとして扱うこと、sakumock で API 経路を検証することを生成後の品質基準にも適用します。

## 高レベルコマンド

### サービス層

高レベルコマンドで必要となる処理は、skr 内の service layer として実装し、CLI はその層を呼び出します。この service layer は SDK の API 操作を組み合わせ、定義の適用やリソース間の調整などユーザーの作業単位を提供します。SDK 内のディレクトリ名やパッケージ配置だけから、高レベル CLI の境界を決めません。高レベルコマンドから低レベル CLI プロセスを起動するのではなく、SDK の API と skr 内の service layer を直接呼び出します。

高レベル操作は低レベル操作と同じリソースを使えますが、SDK 操作との 1 対 1 対応は要求しません。操作順序、参照 ID、検証、差分、失敗時の扱いを明示し、暗黙に作成・削除する範囲をヘルプとドキュメントで説明します。

### IaaS

`skr iaas-api` は sacloud-sdk-go が公開する IaaS の API 操作を直接公開する境界とします。現状の IaaS API が OpenAPI 定義ではないことは、SDK の定義形式の違いであり、高レベル層であることを意味しません。低レベルコマンドは、OpenAPI 定義の有無によらず、SDK の公開メソッドと型を基準に生成します。

`skr iaas` の高レベル操作が必要になった場合は、SDK の API 操作を組み合わせる skr 内の service layer を実装し、それをコマンドから呼び出します。SDK の `service/iaas/<resource>` という配置を、そのまま `skr iaas` の実装境界とみなす設計ではありません。全リソースに同じ操作があるとは仮定せず、SDK の実際の操作とサービス上の意味を確認して公開範囲を決めます。

実行可能ファイルの入口は `cmd/skr/main.go` に置き、CLI のコマンド構成、IaaS の登録、出力処理などの実装とテストは `internal/cli/` にまとめます。各 IaaS リソースのコマンドと SDK クライアントはリソース専用パッケージに分け、別リソースを追加するときも Switch や EventBus のファイルを編集せずに登録できます。

#### IaaS のゾーン指定

IaaS の各操作で対象ゾーンを指定します。フラグまたは JSON で `all` を指定すると、API を呼ぶ前に拒否します。ゾーン名の一覧は `skr iaas-api zone find` で取得できます。CLI は複数ゾーンを横断した検索や結果の合成をしません。

#### Switch API

`skr iaas-api switch` は `sacloud-sdk-go/service/iaas/swytch` の公開操作に合わせて `find`、`read`、`create`、`update`、`delete` を提供します。各操作は SDK の request 型を `--request` の JSON か個別フラグで受け取り、SDK が返す Switch を共通の出力形式で出力します。各操作で対象の `Zone` を明示し、`all` は拒否します。フラグ経路では操作ごとの `Name` や `ID`、独立した任意スカラーを設定します。`Names`、`Tags`、`Sort` などの配列は JSON 経路に残します。ポインタ型の任意フラグで更新時の未指定と明示的な空文字列・ゼロ値を区別し、両経路の併用を拒否します。

#### Zone API

`skr iaas-api zone find` は sacloud-sdk-go v0.3.0 の `service/iaas/zone` が提供する `FindWithContext` を使ってゾーン一覧を取得します。リクエストは省略でき、指定する場合は `zone.FindRequest` JSON を使います。結果は他の API コマンドと同じ JSON と jq の出力経路を通します。この読み取り専用コマンドを jq の例とライブ E2E に使い、ゾーン名の抽出を検証します。詳細は [Zone チュートリアル](../manual/tutorials/iaas-api/zone.md) を参照してください。

### API コマンドの出力形式

API コマンドの結果は JSON のみで出力し、SDK が返すデータ形状とインデントを維持します。[ADR 0030](../adr/0030-remove-table-output.md) に従い、`--output` と出力形式の選択処理を廃止します。プロファイルの `cli.default_output_type` と `DefaultOutputType` は参照しません。

API コマンドではグローバル `--query` に jq 式を指定できます。クエリは API の結果全体に適用し、複数の結果値は JSON として順に出力します。評価には Go 実装の gojq を使い、外部 jq バイナリには依存しません。`skr http` の生レスポンスには適用しません。

Runtime は `ValidateOutput(ctx)` と `WriteOutput(ctx, value)` を受け取ります。結果を返す操作ではクエリの構文とコンパイルを API 呼び出し前に検証します。評価に失敗した場合は途中の結果を書き込みません。表示用の E2E 一覧では `map({ID,Name})` などで必要な項目を投影し、空一覧は `[]` として確認します。後始末と安全性検証には必要な識別情報を残します。

`sakumock v0.9.1` に IaaS mock がないため、Switch の CLI テストでは `internal/sakumock/iaas` のインメモリ実装を使用します。この実装は SDK の `api/iaas.APICaller` を満たし、Switch の API 操作をテストします。将来 sakumock に IaaS 対応が追加された場合は、テストで使う API caller を置き換え、CLI と SDK の操作テストを維持します。

ライブの管理操作は `test/e2e/switch` の Go スクリプトで再現します。ビルド済みの skr CLI を起動し、単純な操作では個別フラグ、`Names` による検索では JSON 経路を使います。`tk1v` だけを対象とし、実行確認フラグを必須にします。実行前に `skr-e2e-` で始まるスイッチを全ページから探し、E2E 用の説明値が一致するものだけ ID と内容を再確認して削除します。その後、固定名 `skr-e2e-switch` で作成し、JSON で操作結果を検証するとともに、ID と名前を投影した JSON も証跡に記録します。確認後、ID と名前・説明を照合して削除します。コマンドの入出力は共通の `test/e2e/internal/evidence` を使い、`tmp/switch-api/<YYYYMMDDHHmm>/` に記録して異常終了後の調査に使います。完了時には全 E2E 共通の `REPORT.md` を生成し、実行コマンド、標準出力、標準エラー、終了コード、最終結果を一覧します。

Disk API のライブ操作は `test/e2e/disk` で検証します。対象は `is1b` の SSD プラン ID 4、20 GB のディスクです。ランダムな名前を使い、同名のディスクがあれば変更せず中止します。作成後は検索・参照・名前更新・再参照・削除し、削除後に対象が存在しないことを確認します。途中で失敗した場合も ID、名前、E2E 説明、サイズを照合してから作成済みのディスクだけを削除します。実行確認フラグを必須とし、コマンドの入出力は `test/e2e/internal/evidence` を使って `tmp/disk-api/<YYYYMMDDHHmm>/` に保存します。

Switch runner は `--confirm-is1b-live` でも実行できます。`--confirm-tk1v` との併用は拒否します。この経路では `is1b` にランダム名のスイッチを作成し、既存リソースの事前削除は行いません。検索、参照、更新、投影 JSON の確認後に今回作成した ID と名前・説明を照合して削除し、不在を検証します。

Zone API の読み取り専用ライブ E2E は `test/e2e/zone` にあります。対象プロファイルと IaaS API の読み取り権限を確認してから `make build`、`./skr config current`、`go run ./test/e2e/zone --skr ./skr --confirm-zone-live` を実行します。runner は選択中プロファイルを確認してから `zone find --query 'map(.Name)'` を実行し、ゾーン名の JSON 配列を検証します。クラウドリソースの作成・変更・削除は行いません。入力・出力・エラーは共通の private evidence 機構で `tmp/zone-api/<YYYYMMDDHHmm>/` に保存します。

実行する開発者は対象プロファイルとプロジェクトを確認します。`tk1v` の既存テスト用スイッチの事前削除を含む変更の承認を得てください。その後、リポジトリのルートで `make build`、`./skr config current`、`go run ./test/e2e/switch --skr ./skr --confirm-tk1v` を実行します。スクリプトは選択中のプロファイルを要求します。同じ分に複数回実行した場合は `-02` 以降を付けて既存の証跡を上書きしません。各 JSON の先頭にある連番と `ORDER.txt` で実行順を確認し、`RESULT.txt` で最終結果を確認します。証跡のディレクトリは `0700`、入力と出力を含む JSON ファイルは `0600` です。実際の ID やアカウント情報を含むため Git に追加・共有しません。削除に失敗した場合はエラーと証跡から対象を確認します。

### EventBus

`skr eventbus-api` は `sacloud-sdk-go/api/eventbus` の操作を直接公開します。SDK `v0.3.0` では process-configuration、schedule、trigger の操作があり、process-configuration には `UpdateSecret` もあります。現在の EventBus チュートリアルにある作成順序などの利用手順は、高レベル `skr event` を設計する際の根拠にできます。

`test/e2e/eventbus-api` は `simplemq-api` と `iaas-api switch` を組み合わせ、`is1b` の通常スイッチ作成イベントから SimpleMQ への配送を実機で検証します。実行条件、秘密情報と証跡の扱い、リソースの後片付けは [EventBus API ライブ E2E 設計](eventbus-api.md) に記録します。

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

- 低レベル API コマンドでは、実際の CLI パスから SDK のリクエスト／レスポンスに至る経路を sakumock または対象 SDK の公開 caller interface を実装するテストモックで検証し、SDK 型の JSON 入出力とエラーを確認します。
- 高レベルコマンドでは、定義の読み込み、依存関係の解決、差分、API 呼び出し順序、失敗時の動作、secret の非表示を対象にテストします。API 呼び出し経路は利用可能な sakumock でも確認します。
- 新たなユーザー向けコマンドは、CLI ヘルプと関連ドキュメントに入力、出力、必要な認証、作成・削除されるリソースを記載します。
- SDK の更新で生成対象が変わる場合は、API グループ、メソッド対応表、ヘルプ、テストを更新します。

## 参照資料

- [`go.mod`](../../go.mod) — sacloud-sdk-go の採用バージョン
- `github.com/sacloud/sacloud-sdk-go/api/` — API グループ
- `github.com/sacloud/sacloud-sdk-go/service/iaas/` — SDK 内の IaaS 関連パッケージ構成
- `github.com/sacloud/sacloud-sdk-go/api/eventbus/` — EventBus 操作
- [EventBus API チュートリアル](../manual/tutorials/eventbus-api.md)
- [API コマンド生成スキル](../../.agents/skills/generate-skr-api-command/SKILL.md)
- [fujiwara/apprun-cli](https://github.com/fujiwara/apprun-cli)
