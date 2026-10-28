# GSLB API コマンド設計

- Status: Implemented
- Related ADR: [ADR 0051](../adr/0051-gslb-api-command-options.md)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## コマンド構成

`gslb-api` は `service/iaas/gslb` が公開する Find / Read / Create / Update / Delete を提供します。GSLB はゾーンを指定しない CommonServiceItem 系 API として扱います。SDK interface、コマンド、入力変換は `internal/gslbapi/` に置き、CLI 登録と SDK factory の接続は `internal/cli/` が担当します。

## 入力

Find の `--name` は SDK の `Names` に1要素を設定し、完全一致検索として扱います。複数名、並び順など生成フラグにない入力は `--request` JSON に残します。Read / Update / Delete の ID と Create / Update の単純値は個別フラグで指定できます。Create / Update は `--request` とフラグを併用できません。

ヘルスチェックのプロトコルは `--health-check-protocol` で指定します。ホスト名、パス、応答コード、ポートは、それぞれ `--health-check-host-header`、`--health-check-path`、`--health-check-response-code`、`--health-check-port` で設定します。指定した値は1つの `HealthCheck` に組み立てます。プロトコル以外の値を指定する場合は `--health-check-protocol` も必要です。選択肢は `http`、`https`、`tcp`、`ping` です。

実サーバーは反復可能な `--destination-server` で指定します。構文は次のとおりです。

```console
$ skr gslb-api create --name example --health-check-protocol ping \
    --destination-server 'ip-address=192.0.2.10,enabled=true,weight=3'
```

`ip-address` は必須で、`enabled` と `weight` は省略できます。省略時は SDK の公開 `iaas.NewGSLBServer` が設定する有効状態と重み 1 を使います。重みを指定する場合は 1～10000 の整数にします。未知・重複キー、空値、bool 形式の誤りは CLI が拒否します。Update でこのオプションを指定すると、宛先一覧全体を指定内容に置き換えます。宛先を空にする場合は `DestinationServers: []` を含む JSON リクエストを使います。

更新では値型のオプションもポインターで保持し、省略と明示したゼロ値を区別します。ヘルスチェックは設定全体の置換です。`--health-check-*` を指定する場合、保持したいポート・ホストヘッダー・パス・応答コードもプロトコルと共に再指定します。個々の監視項目を既存設定とマージしません。`--request` では SDK のリクエスト JSON を使えるため、複数名検索、`MonitoringSuiteLog`、空の宛先一覧などフラグで扱わない値を指定できます。JSON ファイルは `--request @request.json` の形式で読み込みます。標準入力には対応しません。

## テストとライブ E2E

`internal/gslbapi/` の単体テストで構造化入力の正常系・異常系と SDK リクエスト変換を確認します。`internal/cli/` のテストでは全コマンド経路、JSON／フラグ相互排他、API エラー伝播、ヘルプ、SDK が生成する API リクエスト本文を確認します。sakumock v0.13.0 に IaaS API mock は含まれないため、CLI の domain API interface と SDK `APICaller` のテスト double を使います。

`test/e2e/gslb/` は testing.T ベースのシナリオテストです（[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md)）。`SKR_E2E=1` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test` では実行環境を変更しません。Server と Disk の作成・起動・SSH 待機・削除と後片付けは `test/e2e/internal/helper` の server 共有処理を使い、GSLB 固有の確認と DNS 待機は `test/e2e/gslb/` に置きます。

シナリオは、SDK で自動選択した cloud-init アーカイブとディスクプランで 1 vCPU／1 GB メモリの一時サーバーを2台作成します。1台を実サーバー、もう1台をソーリーサーバーとして使い、使い捨て SSH 鍵と cloud-init の user-data で両方を起動します。テスト対象の操作は skr CLI と `dig` で実行し、事前確認と後片付けの確認と起動待ちのポーリングは SDK で静かに実行します。SSH 経由で実サーバーを停止すると DNS 応答がソーリーサーバーの IP のみに切り替わり、再起動すると実サーバーの IP のみに戻ることを確認します。空の応答や両 IP の応答は切り替え成功としません。GSLB の Find / Read / Update / Delete も実行します。

全実サーバーが監視エラーの場合、ソーリーサーバー未設定なら全実サーバーの IP、設定済みならソーリーサーバーの IP を応答します（[GSLB FAQ](https://manual.sakura.ad.jp/cloud/support/technical/appliance-gslb.html)）。宛先1台・ソーリーサーバーなしで空応答を期待するシナリオは使いません。

実行前に、GSLB・サーバー・ディスクの同名リソースが存在しないことを SDK で確認し、既存リソースとの名前衝突を拒否します。後片付けは、GSLB を識別情報の再照合つきで削除した後、server 共有の後片付けでサーバーをディスクとともに削除します。削除は skr CLI で行い、削除後は SDK で不在を確認します。識別できないリソースは削除せず、手動確認が必要なエラーをテストの失敗として報告します。DNS CNAME は作成・変更しません。

DNS の確認には `dig` の応答ヘッダーと A レコードを使います。応答コードが `NOERROR` の場合だけ応答先を判定し、`SERVFAIL`、`NXDOMAIN`、応答ヘッダーの欠落はエラーとして実行記録に残します。作成・切り替えの反映を最大10分待ち、反映待ちの間の問い合わせエラーは再試行します。期限内で正常な期待応答にならなければテストを失敗させます。コマンドの終了コードや空の短縮出力だけを成功条件にしません。

ライブ実行は、サーバー作成と DNS の反映待ちを含むため `-timeout` を指定します。実行環境には `ssh`、`ssh-keygen`、`dig` と is1b で利用できるアーカイブ ID／ディスクプラン ID を用意してください。

```console
$ SKR_E2E=1 go test ./test/e2e/gslb/ -run TestScenarioGSLB -timeout 40m -v
```

サーバーとディスクの利用状況に応じて、料金が発生します。
