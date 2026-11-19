# ADR 0093: DNS API コマンドとレコードの key=value 入力オプションを設ける

- Status: Accepted
- Date: 2026-11-19
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0`

## Context

さくらのクラウド DNS はゾーン (CommonServiceItem、`Provider.Class=dns`) と、ゾーンに属するレコード一覧を扱います。ゾーン名は `Name` に FQDN で保持され、レコードは `Records` 配列 (`Name`、`Type`、`RData`、`TTL`) として API に送ります。レコードの種別は A・AAAA・ALIAS・CNAME・NS・MX・TXT・SRV・CAA・HTTPS・SVCB・PTR の 12 種類です。DNS は IaaS のゾーンを持つリソースではなく、CommonServiceItem 系 API として別の `dns-api` コマンドに配置します。GSLB ([ADR 0051](0051-gslb-api-command-options.md)) と同じく、SDK が公開する Find / Read / Create / Update / Delete をそのまま提供できます。

レコードはネストした配列ですが、ゾーン作成時に A・TXT・MX・CNAME といった基本的なレコードをそろえるのが DNS の主要な作業です。レコード入力を JSON 専用にすると、最初のゾーン作成にも構造体全体の記述が必要になります。一方で、TXT の値にはカンマを含む複雑な値があり得るため、レコード要素のすべての表現を key=value 構文に押し込むことはできません。

[ADR 0041](0041-server-repeatable-disk-and-nic-options.md) は Server の `--disk` / `--nic` に限って反復可能な key=value オプションを認めており、[ADR 0078](0078-shared-text-to-json-notation.md) の共通パーサーで解析します。AppRun Dedicated の `--port` ([ADR 0092](0092-apprun-dedicated-create-cluster.md)) でも同じ方式を再採用しました。

## Decision

- `dns-api` に SDK が公開する Find / Read / Create / Update / Delete を追加し、`internal/dnsapi/` で入力と SDK 呼び出しを扱います。CLI 登録と SDK factory の接続は `internal/cli/` に置きます。
- 名前 (ゾーン名 FQDN)、説明、タグ、アイコン ID は個別フラグにします。Find の `--name` は [ADR 0049](0049-internet-workflow-options.md) と同じく SDK の `Names` への 1 要素変換 (部分一致 AND) とし、`--tags` はカンマ区切りを複数要素の `Tags` に変換します。
- レコードは `--record 'name=www,type=A,rdata=192.0.2.1,ttl=3600'` を反復して指定します。4 キーすべて必須で、`type` は SDK が公開する 12 種類のいずれか、`ttl` は 0 以上の整数を受け付けます。解析は `internal/textjson` の共通パーサーを使い、未知キー、重複キー、空値、不正な種別と TTL は拒否します。Create で指定しない場合は空のレコード一覧 (`[]`) を送ります。
- Update の `--record` を指定するとレコード一覧全体を指定内容に置き換えます。説明・タグ・アイコン ID・監視ログ設定は未指定を現状維持とします。`MonitoringSuiteLog` の有効・無効は `--monitoring-suite-log-enabled` フラグで指定できます。
- レコードの値にカンマを含む場合など、key=value 構文で表現できない入力は `--request` JSON 経路を使います。JSON 経路と個別フラグは相互排他とし、暗黙にマージしません。`--request` には直接指定と `@path.json` を使えます。
- この key=value 構文は DNS の `--record` に限って再採用し、他の配列へ自動的に適用しません。
- ライブ E2E は委譲されていない無料のダミーゾーンを作成し、A・TXT・MX・CNAME のレコード付きで作成、参照、検索、レコード一覧の置換更新、削除を確認します。補助確認として、参照応答の権威 DNS サーバーに `dig` で直接問い合わせ、A レコードの応答値を作成時と更新後で照合します。`dig` が無い環境では API 応答の照合だけで判定します。後片付けは保持した ID と識別情報を再照合してから削除し、不在を確認します。課金リソースは作成しません。
- DNS ゾーン名は API が検証します。文書用に予約されたドメイン (`example.com` など)、Public Suffix List にマッチする名前、存在しない TLD、予約名 (`*.sakura.ad.jp` など) は登録できず、実在 TLD 配下で public suffix と一致しない名前だけを受け付けます。ダミーゾーンは実在 gTLD 配下のランダムな名前を使います。
- ライブ検証で確認した API の入力規則として、全レコードの TTL は同一である必要があり、MX・CNAME の値は末尾が `.` の完全修飾ドメイン名で保存されます。MX の宛先ホストにはゾーン内に A または AAAA レコードが必要です。これらの規則の詳細は設計文書に記録します。

## Consequences

- ゾーン作成と基本的なレコード設定をフラグだけで完結でき、JSON 入力も維持されます。
- key=value 構文ではカンマを含むレコード値を表現できません。その場合は `--request` JSON を使い、ヘルプとチュートリアルで案内します。
- 構造化オプションの解析とヘルプを保守する必要があるため、正常系・異常系と SDK リクエストへの変換をパッケージ内テストで固定します。

## Alternatives considered

- Create / Update を JSON 専用にする案: 基本的なレコード設定にも SDK リクエスト全体の JSON が必要になるため採用しません。
- レコード要素ごとに個別フラグ (`--record-name` など) を設ける案: レコードは複数要素の集合であり、要素間の対応が分からなくなるため採用しません。
- カンマ区切りの値に引用符やエスケープを解釈する独自構文を追加する案: 共通パーサーの字義どおりの解釈を崩すため採用しません。複雑な値は JSON 経路を使います。
