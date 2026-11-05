# ProxyLB API コマンド設計

- Status: Implemented
- Related ADR: [ADR 0072](../adr/0072-proxylb-api-command-options.md)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## コマンド構成

`proxylb-api` は `service/iaas/proxylb` が公開する 11 操作を提供します。Find / Read / Create / Update / Delete に加え、証明書の取得・設定・削除、Let's Encrypt 証明書の更新 (`RenewLetsEncryptCert`)、ヘルス状態と接続数モニタリングを含みます。ProxyLB はゾーンを指定しない CommonServiceItem 系 API として扱います。エンハンスドロードバランサの利用方法は [エンハンスドロードバランサ (ProxyLB)](https://manual.sakura.ad.jp/cloud/appliance/proxylb/index.html) を参照してください。

SDK interface、コマンド、入力変換は `internal/proxylbapi/` に置き、CLI 登録と SDK factory の接続は `internal/cli/` が担当します。GSLB API と同じ CommonServiceItem 系ドメインのため、service 層の公開 API を利用します（[ADR 0071](../adr/0071-iaas-api-uses-api-layer.md) の禁止範囲は `skr iaas-api`）。

## 入力

Find の `--name` は SDK の `Names` に1要素を設定し、部分一致検索として扱います（[ADR 0049](../adr/0049-internet-workflow-options.md)）。`--tags` はカンマ区切り、`--count` と `--from` はページングに使います。検索条件はすべて任意で、条件なしの Find は全件検索します。Sort など生成フラグにない入力は `--request` JSON に残します。Read / Update / Delete / 証明書操作 / モニタリングの ID と Create の単純値は個別フラグで指定できます。すべての操作で `--request` と個別フラグは併用できません。SetCertificates の `--request` と `--id` も併用できません。JSON 内の ID を黙って上書きしないためです。

Create のプランは `--plan` で CPS 値を指定します。指定できる値は 100、500、1000、5000、10000、50000、100000、400000 です。リージョンは `--region` で `tk1`、`is1`、`anycast` を指定し、未指定時は SDK の既定値に従います。ヘルスチェックは `--health-check-protocol`（`http` または `tcp`）と `--health-check-delay-loop`、`--health-check-host`、`--health-check-path` で構成します。指定した値は1つの `HealthCheck` に組み立てます。API はヘルスチェックの設定を常に要求するため、プロトコルと監視間隔は必須で、プロトコルが `http` の場合は監視パスも必須です。未指定、部分的な指定、未対応のプロトコルはエラーにします。この検証は `--request` JSON 経路にも適用します。ProxyLB の監視プロトコルは `http` と `tcp` のみのため、`https` と `ping` は拒否します。

リッスンポートは反復可能な `--bind-port` で指定します。構文は次のとおりです。

```console
$ skr proxylb-api create --name example --plan 100 \
    --bind-port 'proxy-mode=http,port=80,redirect-to-https=true' \
    --bind-port 'proxy-mode=https,port=443,support-http2=true,ssl-policy=TLS-1-2-2019-04' \
    --server 'ip-address=192.0.2.10,port=80,enabled=true'
```

`proxy-mode` と `port` は必須です。`redirect-to-https` は `proxy-mode=http` のみ、`support-http2` と `ssl-policy` は `proxy-mode=https` のみ指定できます。未知・重複キー、空値、bool 形式の誤り、1～65535 外のポート、プロキシモードと整合しないキーは CLI が拒否します。`AddResponseHeader` は JSON 経路で指定します。

配下サーバーは反復可能な `--server` で指定します。`ip-address` と `port` は必須で、`server-group`、`enabled`、`proxy-ssl` は省略できます。`enabled` を省略すると API は実サーバーを無効で登録するため、トラフィックを振り分けるには `enabled=true` を指定します。未知・重複キー、空値、bool 形式の誤り、範囲外のポートは拒否します。

ソーリーサーバーは `--sorry-server-ip` と `--sorry-server-port` で構成します。Let's Encrypt は `--lets-encrypt-enabled` と `--lets-encrypt-common-name`、`--lets-encrypt-subject-alt-names` で構成します。Sticky session の方式は cookie です。Gzip、Proxy Protocol v2、モニタリングスイートログ、Strict ルールはブール値で、それぞれ `--gzip`、`--proxy-protocol`、`--monitoring-suite-log-enabled`、`--strict-rule` で指定します。HTTP 持続接続は `--backend-http-keep-alive` で `safe` または `aggressive` を指定します。syslog は `--syslog-server` と `--syslog-port` で指定します。アイドルタイムアウトは `--timeout-inactive-sec` で指定し、未指定時は 10 が設定されます。空のタイムアウトオブジェクトは API に拒否されるため、常に有効な値を送ります。Let's Encrypt と syslog の値は設定全体の置換なので、片方だけの変更でも両方を再指定できます。

Update では値型のオプションもポインターで保持し、省略と明示したゼロ値を区別します。未指定の項目は service 層の Update が現在のリソースを Read して現在値で補完するため、既存の設定を引き継ぎます。ヘルスチェック、ソーリーサーバー、Let's Encrypt、syslog のフラグは設定全体の置換です。保持したい項目は再指定してください。ヘルスチェックは未指定時に現在の設定が引き継がれるため再指定は不要で、指定した場合のみ Create と同じ検証を適用します。未指定のタイムアウトも現在値が引き継がれ、既存のタイムアウトを持つ対象を未指定の更新で変えません。`--bind-port` と `--server` を指定すると一覧全体を指定内容に置き換えます。`--plan` を指定すると service 層の Update 経由でプラン変更 API を呼びます。空配列、`AddResponseHeader`、`Rules`、`OriginGuard` などフラグで扱わない値は `--request` JSON で指定します。Create と Update の `--request` は `OriginGuard.Token` など秘密情報を受け取れるため、直接指定は受け付けず、保護したファイル (`@path.json`) または標準入力 (`-`) から読み込みます。

## 証明書の操作

証明書の設定 (SetCertificates) は秘密鍵を扱うため、証明書は引数に直接指定できません。主証明書と秘密鍵の PEM を `@path` または `-`（標準入力）で渡します。

```console
$ skr proxylb-api set-certificates --id 123456789012 \
    --primary-cert @server.crt --primary-key @server.key --primary-intermediate @intermediate.crt
```

`--primary-cert` と `--primary-key` は必須です。`--additional-certs` は追加証明書の JSON 配列を受け付け、各要素は `ServerCertificate`、`IntermediateCertificate`、`PrivateKey` を持ちます。秘密鍵を含むため、こちらも `@path` または `-` のみを受け付けます。

```console
$ skr proxylb-api set-certificates --id 123456789012 \
    --primary-cert @server.crt --primary-key @server.key \
    --additional-certs @additional.json
```

API は証明書一式を置き換えるため、追加証明書だけの差し替えでも主証明書の再指定が必要です。証明書の取得は `get-certificates`、削除は `delete-certificates`、Let's Encrypt 設定の証明書更新は `renew-lets-encrypt-cert` で行います。Let's Encrypt による自動更新には `--lets-encrypt-enabled` での設定が必要です。

## テストとライブ E2E

`internal/proxylbapi/` の単体テストで構造化入力の正常系・異常系と SDK リクエスト変換を確認します。`internal/cli/` のテストでは全コマンド経路、JSON／フラグ相互排他、API エラー伝播、ヘルプ、証明書入力の保護、SDK が生成する API リクエスト本文を確認します。sakumock v0.13.0 に CommonServiceItem 系の ProxyLB mock は含まれないため、CLI の domain API interface と SDK `APICaller` のテスト double を使います。

`test/e2e/proxylb/` は testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。`SKR_E2E=1` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test` では実行環境を変更しません。Server と Disk の作成・起動・SSH 待機・削除と後片付けは `test/e2e/internal/helper` の server 共有処理を使い、ProxyLB 固有の確認は `test/e2e/proxylb/` に置きます。

シナリオは、SDK で自動選択した cloud-init アーカイブとディスクプランで 1 vCPU／1 GB メモリの一時サーバーを作成し、plan 100 の ProxyLB を作成してその IP を配下サーバーに登録します。テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認と起動待ちのポーリングは SDK で静かに実行します。FQDN は API が非同期に割り当てるため、作成直後の応答には含まれません。トラフィックの確認は作成直後に割り当てられる VIP 宛ての HTTP GET と HTTPS GET で行い、`health-status` の応答に配下サーバーが現れることも確認します。証明書は `openssl` で自己署名証明書を生成し、`set-certificates` → `get-certificates` → `delete-certificates` の順に確認します。Update で説明と監視間隔を変更します。`renew-lets-encrypt-cert` は Let's Encrypt 未設定の対象で API エラーになること (異常系) のみを確認し、正常系には実在ドメインの DNS 検証が必要なため Issue #199 で追跡します。失敗応答は異常系の確認であり、証跡に残します。

作成前に同名リソースが存在しないことを SDK で確認し、既存リソースとの名前衝突を拒否します。後片付けは、ProxyLB を識別情報の再照合つきで削除した後、server 共有の後片付けでサーバーをディスクとともに削除します。削除は skr CLI で行い、削除後は SDK で不在を確認します。識別できないリソースは削除せず、手動確認が必要なエラーをテストの失敗として報告します。

ライブ実行は、サーバー作成と ProxyLB の準備待ちを含むため `-timeout` を指定します。実行環境には `openssl` と is1b で利用できるアーカイブ ID／ディスクプラン ID を用意してください。

```console
$ SKR_E2E=1 go test ./test/e2e/proxylb/ -run TestScenarioProxyLB -timeout 30m -v
```

ProxyLB (plan 100) とサーバー・ディスクの利用状況に応じて、料金が発生します。
