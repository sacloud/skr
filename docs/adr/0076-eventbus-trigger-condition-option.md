# ADR 0076: EventBus トリガー条件を反復可能な --condition オプションで指定する

- Status: Accepted
- Date: 2026-11-10

## 背景

EventBus トリガーの条件 (Conditions) は、API では配列で指定します。次はイベントログソースの条件例です。

```json
[
  {"Key": "zone", "Op": "eq", "Values": ["is1b"]}
]
```

モニタリングスイートの Alert ソースで複数の重要度を指定する例は次のとおりです。

```json
[
  {"Key": "severity", "Op": "in", "Values": ["warning", "critical"]}
]
```

[SDK v0.5.0 同梱の EventBus OpenAPI 定義](https://github.com/sacloud/sacloud-sdk-go/blob/v0.5.0/api/eventbus/openapi/openapi.json)では、`Key` は 1〜20 文字の小文字英数字で `data` を禁止する pattern (`^(?!data$)[a-z0-9]{1,20}$`)、`eq` は `Values` がちょうど 1 件 (minItems=1、maxItems=1)、`in` は 1 件以上 (minItems=1) とされています。条件に使うキーは、モニタリングスイートの Alert ソースでは `severity` と `subject`、イベントログソースでは `zone` と `subject` です ([EventBus のイベント](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html))。

高レベルの `skr eventbus trigger` ([ADR 0075](0075-high-level-command-tier.md)) で条件を JSON なしに指定する構文が必要です。[ADR 0041](0041-server-repeatable-disk-and-nic-options.md) は Server の `--disk` / `--nic` に反復可能な `key=value` 構文を導入しましたが、他の配列への自動適用は禁止されています。トリガー条件の構文は個別に決定します。構文の共通規則は、のちに [ADR 0078](0078-shared-text-to-json-notation.md) で skr 全体の text → JSON 簡易表記として抽出しました。

## 決定

`skr eventbus trigger create` と `update` に反復可能な `--condition` オプションを設けます。1 回の指定を条件 1 件に対応させ、指定順に API の `Conditions` 配列を組み立てます。各指定はカンマ区切りの `key=value` 形式とします。

```console
$ skr eventbus trigger create --name example --process-configuration-id 123456789012 \
    --source '//eventbus.sakura.ad.jp/eventlog' \
    --type 'jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created' \
    --condition 'key=zone,op=eq,value=is1b'

$ skr eventbus trigger create --name alert-example --process-configuration-id 123456789012 \
    --source '//eventbus.sakura.ad.jp/monitoringsuite/alert' \
    --type 'jp.ad.sakura.eventbus.alert.firing' \
    --condition 'key=severity,op=in,value=warning,value=critical'
```

実行設定 ID は構文を示すための仮の値です。例はソースごとに分け、イベントログに Alert 用の `severity` 条件を組み合わせません。

- 対応キーは `key`、`op`、`value` とします。`key` と `op` は必須で反復できず、`value` は反復を許可します。未知のキー、必須キーの欠落・重複、構文エラーはエラーにします。
- 構文の共通規則 (要素の分解、値の字義どおりの扱い、カンマの制限、エラーメッセージへの入力値の非表示など) は、skr 全体で統一された text → JSON 簡易表記 ([ADR 0078](0078-shared-text-to-json-notation.md)) に従います。解析は共有パーサー `internal/textjson` が行います。
- `key` は API と同じ条件で検証します。Go の標準正規表現は否定先読みを扱えないため、`^[a-z0-9]{1,20}$` の照合と `data` の除外を組み合わせます。`op` は `eq` または `in` を受け付けます。
- `value` は条件の値で、**同一指定内での反復を許可します**。これは [ADR 0041](0041-server-repeatable-disk-and-nic-options.md) の重複キー拒否に対する、本オプション固有の例外です。API の `Values` 配列に対応するための例外で、`eq` はちょうど 1 件、`in` は 1 件以上を要求します。値自体にカンマを含めることはできず、その場合は低レベル `eventbus-api` の JSON 経路を使います。シェルで全体を引用しても、この制限は変わりません。
- update で `--condition` を指定した場合は `Conditions` 全体を置き換え、未指定の場合は現状を維持します。条件の消去 (`null`) は低レベル `eventbus-api` で指定します。
- 構文の解析は CLI が共通パーサーで行い、解析後の意味検証 (`key` の pattern、`eq`/`in` の値数) は service 層が行います。
- この検証は、独自構文から条件の union 型へ曖昧さなく変換するためのものです。[ADR 0047](0047-minimize-cli-request-body-validation.md) の原則に従い、イベントタイプの網羅的な許可リストやソース別の値の一覧は再実装しません。ヘルプにはソースごとのキーと参照先を示し、サービス上の有効性は API に委ねます。
- 本構文の対象は `skr eventbus trigger` の create と update だけです。他の配列や操作へ自動的に適用しません。

## 影響

- 独自構文の解析、検証、API リクエストへの変換と、その正常系・エラー系の単体テストが必要です。
- ヘルプ、チュートリアル、ライブ E2E を新しい構文に合わせて整備します。
- 条件の消去や `--condition` で表現できない入力は、低レベル `eventbus-api` の JSON 経路を利用します。

## 検討した選択肢

- `--conditions-file` で API 形式の JSON を渡す方式: 高レベル層でありながら JSON を書かせるため、[#214](https://github.com/sacloud/skr/issues/214) の解決として不十分です。採用しません。
- `--condition` と `--conditions-file` を併設する方式: カンマを含む値や条件の消去は本構文で表現できませんが、低レベル層の JSON 経路で指定できます。高レベル層に別の JSON 経路は追加しません。
- `op` ごとに個別フラグを設ける方式 (`--eq-condition`、`--in-condition` など): 値の所属を表す構文は別途必要で、フラグ数も増えます。今回は条件1件を1回の指定で完結させる方式を採用します。
