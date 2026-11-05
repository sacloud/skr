# ADR 0072: ProxyLB API コマンドにドメイン固有の入力オプションを設ける

- Status: Accepted
- Date: 2026-11-04 (2026-11-05 ヘルスチェックとタイムアウトの必須項目を追記)
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## Context

エンハンスドロードバランサ (ProxyLB) は、リッスンポート、配下サーバー、ヘルスチェック、証明書など複数の配列とネストした設定を扱う CommonServiceItem 系 API です。リクエスト全体を JSON だけにすると、基本的な作成とサーバー設定にも構造体全体の記述が必要になります。一方、任意の API 配列を汎用的にフラグ化すると、入力の意味や省略値を曖昧にします。

`service/iaas/proxylb` は Find / Read / Create / Update / Delete / GetCertificates / SetCertificates / DeleteCertificates / RenewLetsEncryptCert / HealthStatus / MonitorConnection を公開します。ProxyLB は IaaS のゾーンを持つリソースではなく、GSLB と同じ `<service>-api` コマンドとして配置します。[ADR 0071](0071-iaas-api-uses-api-layer.md) の service 層の禁止範囲は `skr iaas-api` であり、GSLB API と同じ CommonServiceItem 系ドメインでは service 層の公開 API を利用できます。

証明書の設定 (SetCertificates) は秘密鍵を扱います。秘密情報を引数に直接指定させず、保護したファイルまたは標準入力から読み込む必要があります。

SDK v0.3.1 のリクエスト変換は、未指定のヘルスチェックとタイムアウトも空オブジェクトとして常に送信します。ライブ確認の結果、API は空のヘルスチェック (`Protocol` なし、`DelayLoop` なし) と空のタイムアウト (`InactiveSec` なし) を拒否し、他の空オブジェクト (ソーリーサーバー、Sticky session、Gzip、Proxy Protocol、syslog、Rules) は受け入れることを確認しました。このため、Create と Update はヘルスチェックの必須項目を検証し、未指定のタイムアウトに SDK が未指定時に返す値 (`ProxyLBTimeout.GetInactiveSec()` の既定値 10) を適用する必要があります。

## Decision

- `proxylb-api` に SDK が公開する 11 操作を追加し、`internal/proxylbapi/` で入力と SDK 呼び出しを扱います。CLI 登録と SDK factory の接続は `internal/cli/` に置きます。ProxyLB はグローバルリソースのため、ゾーンを受け付けません。
- Find は `--name`（部分一致、1要素の `Names` への変換）、`--tags`、`--count`、`--from` を受け付けます。Sort など生成フラグにない入力は `--request` JSON に残します。
- Create は名前、説明、タグ、アイコン ID、プラン、リージョン、VIP フェイルオーバー、ヘルスチェック、ソーリーサーバー、Let's Encrypt、Sticky session、Gzip、HTTP 持続接続、Proxy Protocol、syslog、アイドルタイムアウト、モニタリングスイートログ、Strict ルールを個別フラグにします。プランは SDK が公開する CPS 値 (100～400000) を受け付けます。ヘルスチェックのプロトコルは ProxyLB が対応する `http` と `tcp` のみを受け付けます。GSLB と異なり `https` と `ping` は ProxyLB の監視プロトコルに存在しないため拒否します。
- ヘルスチェックは API が必須とする設定です。プロトコルと監視間隔を必須とし、プロトコルが `http` の場合は監視パスも必須にします。未指定、部分的な指定、未対応のプロトコルはエラーにします。Update も API がヘルスチェックの設定を常に要求するため再指定が必要です。この検証は `--request` JSON 経路にも適用し、エラーメッセージは入力経路に応じて個別フラグ名または API のフィールド名で案内します。
- アイドルタイムアウトは未指定時に `InactiveSec` 10 を適用します。空のタイムアウトオブジェクトは API に拒否されるため、常に有効な値を送ります。指定された値はそのまま送ります。
- リッスンポートは反復可能な `--bind-port 'proxy-mode=http|https|tcp,port=ポート番号[,redirect-to-https=…][,support-http2=…][,ssl-policy=…]'` で指定します。`redirect-to-https` は `http` のみ、`support-http2` と `ssl-policy` は `https` のみ指定できます。未知キー、重複キー、空値、不正な bool、範囲外のポート、プロキシモードと整合しないキーは拒否します。`AddResponseHeader` は構造が複雑なため JSON 経路に残します。
- 配下サーバーは反復可能な `--server 'ip-address=IP,port=ポート番号[,server-group=グループ名][,enabled=true|false][,proxy-ssl=true|false]'` で指定します。`ip-address` と `port` は必須です。`enabled` を省略すると API は実サーバーを無効で登録するため、トラフィックを振り分けるには `enabled=true` を指定します。未知キー、重複キー、空値、不正な bool、範囲外のポートは拒否します。
- Update の個別フラグは未指定を維持し、明示した `false`、`0`、空文字列も入力として区別します。ヘルスチェック、ソーリーサーバー、Let's Encrypt、syslog のフラグはそれぞれ1つの設定として組み立て、設定全体を置き換えます。`--bind-port` と `--server` を指定すると一覧全体を置き換えます。`--plan` は service 層の Update 経由でプラン変更 API を呼びます。空配列や `AddResponseHeader`、`Rules`、`OriginGuard` などフラグで扱わない入力は JSON 経路を使います。
- 証明書の操作は GetCertificates / SetCertificates / DeleteCertificates / RenewLetsEncryptCert を公開します。SetCertificates の証明書は秘密鍵を含むため、`--primary-cert`、`--primary-key`、`--primary-intermediate`、`--additional-certs` に直接値を受け付けず、`@path` または `-`（標準入力）のみを受け付けます。主証明書と秘密鍵は必須です。API は証明書一式を置き換えるため、追加証明書だけの差し替えでも主証明書の再指定が必要です。`--additional-certs` は追加証明書の JSON 配列を受け付けます。
- `--request` には各操作のリクエスト JSON を直接または `@path.json` で渡せます。JSON 経路と個別フラグは相互排他とし、暗黙にマージしません。SetCertificates の `--request` も秘密鍵を含むため、保護したファイルまたは標準入力のみを受け付けます。
- 独自の構造化オプションを追加する場合も、他のサービスや配列にはこの構文を適用しません。詳細な設計・テスト・チュートリアル更新は [ProxyLB API コマンド設計](../design/proxylb-api-command.md)に記録します。
- ライブ E2E は is1b に cloud-init で起動した一時サーバーを配下サーバーとし、plan 100 の ProxyLB を作成します。FQDN は API が非同期に割り当てるため、作成直後の応答には含まれません。トラフィックの確認は作成直後に割り当てられる VIP 宛ての HTTP GET と HTTPS GET で行い、自己署名証明書の設定・取得・削除、Update、`renew-lets-encrypt-cert` のエラー系も実行します。作成したリソースの ID と識別情報を照合して削除し、不在を確認します。

## Consequences

- よく使う設定をフラグで指定しつつ、SDK のリクエスト型による JSON 入力も利用できます。
- 構造化オプションの解析とヘルプを保守する必要があるため、正常系・異常系と SDK リクエストへの変換をパッケージ内テストで固定します。
- 証明書入力はファイルまたは標準入力のみのため、インラインで証明書を貼り付ける操作はできません。秘密情報をシェル履歴やプロセス一覧に残さない利点と引き換えにします。
- ヘルスチェック、ソーリーサーバー、Let's Encrypt、syslog の設定は置換であり、既存設定とマージしません。保持したい項目は再指定する必要があります。ヘルスチェックは API が必須とするため、Create と Update の両方で指定が必要です。
- 未指定のタイムアウトに既定値を適用するため、リクエスト JSON でタイムアウトを省略した場合も `InactiveSec` 10 が送信されます。ヘルプに未指定時の値を記載します。

## Alternatives considered

- Create / Update を JSON 専用にする案: 基本的なリッスンポートと配下サーバーの設定にも SDK リクエスト全体の JSON が必要になるため採用しません。
- 全 API 配列を共通フラグへ一般化する案: 要素の意味、既定値、置換と追加の違いがサービスごとに異なるため採用しません。
- 証明書をインライン値でも受け付ける案: 秘密鍵を引数に含めない方針に反するため採用しません。
- `iaas-api proxylb` として低レベル API 操作を公開する案: service 層の Update がプラン変更ワークフローを担い、証明書やモニタリングの操作も service 層に集約されているため、GSLB と同じ `<service>-api` ドメインとして service 層を利用します。
