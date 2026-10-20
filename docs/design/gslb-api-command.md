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

更新では値型のオプションもポインターで保持し、省略と明示したゼロ値を区別します。`--request` では SDK のリクエスト JSON を使えるため、複数名検索、`MonitoringSuiteLog`、空の宛先一覧などフラグで扱わない値を指定できます。JSON ファイルは `--request @request.json` の形式で読み込みます。標準入力には対応しません。

## テストとライブ E2E

`internal/gslbapi/` の単体テストで構造化入力の正常系・異常系と SDK リクエスト変換を確認します。`internal/cli/` のテストでは全コマンド経路、JSON／フラグ相互排他、API エラー伝播、ヘルプ、SDK が生成する API リクエスト本文を確認します。sakumock v0.13.0 に IaaS API mock は含まれないため、CLI の domain API interface と SDK `APICaller` のテスト double を使います。

`test/e2e/gslb/` の is1b runner は、指定された cloud-init アーカイブとディスクプランを使い、1 vCPU／1 GB メモリの一時サーバーとディスクを作成します。使い捨て SSH 鍵でサーバーを起動し、GSLB の ping ヘルスチェックが DNS 応答に反映されることを検証します。稼働中のサーバー IP が応答されること、SSH 経由で停止した後にその IP が応答から外れること、再起動後に応答へ戻ることを確認します。GSLB の Find / Read / Update / Delete も実行します。

runner は既存リソースとの名前衝突を事前に拒否します。失敗時も作成した GSLB とサーバーの ID・説明を確認してから削除します。サーバー削除時に接続ディスクを一括削除せず、切り離されたディスクの名前・プラン・作成元アーカイブ・サイズを照合します。さらに ID で読み直して同一性と未接続状態を確認してから個別に削除し、サーバーとディスクの不在を確認します。確認できないディスクは削除せず、手動確認が必要なエラーを返します。DNS CNAME は作成・変更しません。

DNS の確認には `dig` の応答ヘッダーと A レコードを使います。応答コードが `NOERROR` の場合だけ IP の有無を判定し、`SERVFAIL`、`NXDOMAIN`、応答ヘッダーの欠落はエラーとして証跡に残します。コマンドの終了コードや空の短縮出力だけを成功条件にしません。

実行環境には `ssh`、`ssh-keygen`、`dig` と is1b で利用できるアーカイブ ID／ディスクプラン ID を用意してください。

サーバーとディスクの利用状況に応じて、料金が発生します。
