# DNS API コマンド設計

- Status: Implemented
- Related ADR: [ADR 0093](../adr/0093-dns-api-commands.md)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0`

## コマンド構成

`dns-api` は `service/iaas/dns` が公開する Find / Read / Create / Update / Delete を提供します。DNS ゾーンはゾーンを指定しない CommonServiceItem 系 API として扱います。SDK interface、コマンド、入力変換は `internal/dnsapi/` に置き、CLI 登録と SDK factory の接続は `internal/cli/` が担当します。

## 入力

Find の `--name` は SDK の `Names` に1要素を設定する部分一致検索 ([ADR 0049](../adr/0049-internet-workflow-options.md))、`--tags` はカンマ区切りを複数要素の `Tags` に変換します。複数名検索など生成フラグにない入力は `--request` JSON に残します。Read / Update / Delete の ID と Create / Update の単純値は個別フラグで指定できます。Create / Update は `--request` とフラグを併用できません。

レコードは反復可能な `--record` で指定します。構文は次のとおりです。

```console
$ skr dns-api create --name www.example.com \
    --record 'name=www,type=A,rdata=192.0.2.10,ttl=3600'
```

`name`、`type`、`rdata`、`ttl` はすべて必須です。`type` は `A`、`AAAA`、`ALIAS`、`CNAME`、`NS`、`MX`、`TXT`、`SRV`、`CAA`、`HTTPS`、`SVCB`、`PTR` のいずれか、`ttl` は 0 以上の整数です。未知・重複キー、空値、不正な種別と TTL は CLI が拒否します。解析は `internal/textjson` の共通パーサー ([ADR 0078](../adr/0078-shared-text-to-json-notation.md)) を使うため、値は字義どおり保持されます。空白はトリムせず、引用符とエスケープを解釈しないため、カンマを含むレコード値 (長い TXT など) はこの構文で表現できません。その場合は `--request` JSON を使います。

Create で `--record` を指定しない場合は空のレコード一覧 (`[]`) を送ります。Update で `--record` を指定するとレコード一覧全体を指定内容に置き換えます。レコードを1つだけ変更したい場合も、変更後の一覧全体を指定するか、JSON リクエストを使います。Update の説明・タグ・アイコン ID・`--monitoring-suite-log-enabled` は未指定を現状維持とし、明示した `false` も入力として区別します。Update は SDK が現在値とマージするため、未指定のレコード以外の設定は維持されます。

`--request` には SDK の Create / Update / Find / Read / Delete リクエスト JSON を直接または `@path.json` で渡せます。JSON 経路と個別フラグは相互排他とし、暗黙にマージしません。複数名検索、カンマを含むレコード値、JSON でしか表現できない項目は JSON 経路を使います。

## ライブ検証で確認した API の挙動

- DNS ゾーン名は API が検証します。文書用に予約されたドメイン (`example.com`、`example.net` など、[RFC 2606](https://www.rfc-editor.org/rfc/rfc2606.html)) は「ゾーン名の形式が不正」として拒否されます。Public Suffix List にマッチする名前と存在しない TLD (`test`、`invalid` など) は「Public Suffix Listにマッチするか存在しないTLD」として拒否されます。`*.sakura.ad.jp` などの予約名は「予約名」として拒否されます。実在 TLD 配下で public suffix と一致しない名前を受け付けます。エラーメッセージは拒否理由ごとに変わります。
- 全レコードの TTL は同一である必要があります。TTL が異なるレコードを送ると 400 になります。
- MX の値は `優先度 宛先ホスト` の形式で、宛先ホストは末尾が `.` の完全修飾ドメイン名である必要があります。末尾の `.` がない場合は API がエラーを返します。
- MX の宛先ホストにはゾーン内の A または AAAA レコードが必要です。
- MX・CNAME の値は、末尾が `.` の完全修飾ドメイン名として正規化されて保存されます。フラグで指定した値と Read 応答の値は一致しない場合があるため、照合には正規化後の値を使います。
- ゾーン作成直後、権威 DNS サーバーへの問い合わせは反映完了まで `REFUSED` を返します。反映後は `NOERROR` と `aa` フラグ付きの権威応答が返ります。反映待ちは期限付きのリトライで吸収します。

## テストとライブ E2E

`internal/dnsapi/` の単体テストで構造化入力の正常系・異常系と SDK リクエスト変換を確認します。`internal/cli/` のテストでは全コマンド経路、JSON／フラグ相互排他、API エラー伝播、ヘルプを確認します。SDK が生成する API リクエスト本文 (`/commonserviceitem`、`Provider.Class=dns`、`Settings.DNS.ResourceRecordSets`) も確認します。DNS は IaaS API mock に含まれないため、CLI の domain API interface と SDK `APICaller` のテスト double を使います。

`test/e2e/dns/` は testing.T ベースのシナリオテストです ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。`SKR_E2E=1` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test` では実行環境を変更しません。テスト対象の操作は skr CLI と `dig` で実行し、事前確認と後片付けの確認は SDK で静かに実行します。

シナリオは、委譲されていない無料のダミーゾーン `skr-e2e-<接尾辞>.cloud` を作成します。A・TXT・MX・CNAME の 4 レコード付きで作成します。MX の宛先ホストは A レコードを持つ `www` とし、全レコードの TTL は 3600 に統一します。参照と部分一致検索で ID・ゾーン名・説明・タグ・レコード一覧・権威 DNS サーバーを照合します。照合には MX・CNAME の末尾が `.` の正規化後の値を使います。更新では説明を変更し、`--record` で A (IP と TTL を変更)・AAAA (追加)・TXT (値を変更) の一覧へ置き換え、MX と CNAME が消えることを含めて照合します。削除後は SDK で不在を確認します。

DNS の確認には `dig` を使い、参照応答の権威 DNS サーバーに直接問い合わせます。委譲されていないゾーンでも、権威サーバーへの直接問い合わせでレコードの応答を確認できます。応答コードが `NOERROR` の場合だけ A・AAAA レコードを判定し、`SERVFAIL`、`NXDOMAIN`、応答ヘッダーの欠落はエラーとして実行記録に残します。作成・更新の反映を最大5分待ち、反映待ちの間の問い合わせエラーは再試行します。期限内で正常な期待応答にならなければテストを失敗させます。コマンドの終了コードや空の短縮出力だけを成功条件にしません。`dig` が無い環境では API 応答の照合だけで判定し、補助確認を省略します。

実行前に、テスト用ゾーンと同名の DNS ゾーンが存在しないことを SDK で確認し、既存リソースとの名前衝突を拒否します。後片付けは、保持した ID と許容する識別情報 (ゾーン名と説明) を再照合してから削除します。識別できないリソースは削除せず、手動確認が必要なエラーをテストの失敗として報告します。削除は skr CLI で行い、削除後は SDK で不在を確認します。

DNS ゾーンは無料のため、このシナリオでは料金が発生しません。

```console
$ SKR_E2E=1 go test ./test/e2e/dns/ -run TestScenarioDNS -timeout 20m -v
```
