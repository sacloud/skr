# ADR 0051: GSLB API にドメイン固有の入力オプションを設ける

- Status: Accepted
- Date: 2026-10-19
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## Context

GSLB の作成・更新では、名前などの単純値に加えて、ヘルスチェック設定と複数の実サーバーを扱います。リクエスト全体を JSON だけにすると、基本的な GSLB の作成とサーバー設定にも構造体全体の記述が必要になります。一方、任意の API 配列を汎用的にフラグ化すると、入力の意味や省略値を曖昧にします。

`service/iaas/gslb` は Find / Read / Create / Update / Delete を公開します。GSLB は IaaS のゾーンを持つリソースではなく、CommonServiceItem 系 API として別の `<service>-api` コマンドに配置します。Create / Update のリクエストにはヘルスチェック、重み付け、ソーリーサーバー、実サーバー配列などがあり、SDK の API リクエスト変換でこれらが GSLB 設定へ送られます。

## Decision

- `gslb-api` に SDK が公開する Find / Read / Create / Update / Delete を追加し、`internal/gslbapi/` で入力と SDK 呼び出しを扱います。CLI 登録と SDK factory の接続は `internal/cli/` に置きます。
- 名前、説明、タグ、アイコン ID、チェック間隔、重み付け、ソーリーサーバーは個別フラグにします。ヘルスチェックの値は `--health-check-*` にフラット化し、いずれかを指定するときはプロトコルを必須にします。プロトコルは SDK が公開する `http`、`https`、`tcp`、`ping` を受け付けます。
- 実サーバーは `--destination-server 'ip-address=IP[,enabled=true|false][,weight=1..10000]'` を反復して指定します。`ip-address` は必須です。`enabled` と `weight` の省略時は SDK の `iaas.NewGSLBServer` が設定する有効状態と重み 1 を使います。未知キー、重複キー、空値、不正な bool、範囲外または不正な重みは拒否します。
- Update の個別フラグは未指定を維持し、明示した `false`、`0`、空文字列も入力として区別します。ヘルスチェックのフラグは1つの HealthCheck 設定として SDK リクエストに組み立てます。実サーバーを1つ以上指定した場合は宛先一覧を置き換えます。空配列による削除などフラグで表せない入力は JSON 経路を使います。
- `--request` には SDK の Create / Update / Find / Read / Delete リクエスト JSON を直接または `@path.json` で渡せます。JSON 経路と個別フラグは相互排他とし、暗黙にマージしません。`MonitoringSuiteLog` など専用フラグのない項目も JSON で指定できます。
- 独自の構造化オプションを追加する場合も、他のサービスや配列にはこの構文を適用しません。詳細な設計・テスト・チュートリアル更新は [GSLB API コマンド設計](../design/gslb-api-command.md)に記録します。
- ライブ E2E は is1b に一時サーバーと GSLB を作成し、ping ヘルスチェックが DNS 応答に反映されることを確認します。サーバーの稼働中・停止中・再起動後に GSLB FQDN の応答先が切り替わることを検証し、Find / Read / Update / Delete も実行します。サーバー用ディスクを含めて今回作成したリソースの ID と識別情報を照合して削除し、不在を確認します。DNS CNAME は変更しません。
- 実行には cloud-init アーカイブとディスクプランの ID が必要です。利用料金の発生に備えて、専用確認フラグを必須とします。

## Consequences

- よく使う設定をフラグで指定しつつ、SDK のリクエスト型による JSON 入力も利用できます。
- 構造化オプションの解析とヘルプを保守する必要があるため、正常系・異常系と SDK リクエストへの変換をパッケージ内テストで固定します。
- ライブ E2E は一時サーバー1台の ping 監視と GSLB FQDN の応答先切り替えを確認します。クライアント側の全リゾルバーでの DNS キャッシュ切り替え時間は検証対象外です。

## Alternatives considered

- Create / Update を JSON 専用にする案: 基本的な監視設定と実サーバー設定にも SDK リクエスト全体の JSON が必要になるため採用しません。
- 全 API 配列を共通フラグへ一般化する案: 要素の意味、既定値、置換と追加の違いがサービスごとに異なるため採用しません。
- 旧 CLI の入力構文を前提にする案: 本 CLI のオプションは SDK と公式仕様から個別に設計し、互換性を前提にしないため採用しません。
