# ADR 0085: `--request` を request body 用に限定し、list 操作は個別フラグに統一する

- Status: Accepted
- Date: 2026-11-13
- SDK: `github.com/sacloud/sacloud-sdk-go v0.6.0`

## Context

生成器は `request_type` を持つすべての操作に一律で `--request` を生成してきた ([ADR 0007](0007-api-request-input.md))。その結果、HTTP の request body を持たない操作、すなわち query params を受ける list 操作にも `--request` が付いていた。これはコマンド入力の見た目上は body 用の JSON と同じ形であり、利用者が「`--request` は API リクエスト本文を表す」という理解で使うと、実際には query params にマッピングされるという不整合を抱えていた。

全ドメインの `--request` デコード先を監査した結果 ([issue #75](https://github.com/sacloud/skr/issues/75))、次のことが判明した。

- IAM の手書きラッパー型 (`folder.ListParams`、`project.ListParams`、`folder.CreateParams` など) は json タグを持たず、API の wire 名が snake_case (`per_page`、`parent_id`) のため、タグなしデコードの大文字小文字を無視した Go フィールド名マッチでは結びつかない。`--request` で受けた JSON のキーが黙って無視され、ゼロ値のまま送信される実害があった。organization と projectapikey は、wire 名タグ付き中間構造体による明示変換で既に対策済みである。
- それ以外のドメイン (IaaS、kms、secretmanager など) は、タグなしでも wire 形式が PascalCase で Go フィールド名と一致するため機能している。
- 値が黙って落ちる機構は2種類あり、検出方法が異なる。(1) デコード時に落ちる: wire 名キーがタグなし型に結びつかない。`DisallowUnknownFields` で検出できる。(2) エンコード時に落ちる: デコード先の SDK 型自体にフィールドが存在しない。`DisallowUnknownFields` では検出できず、decode から送信形への変換までを通す round-trip テストでのみ捉えられる。
- IaaS の find は、SDK 実装が GET メソッドにリクエストボディを付けて送る (`FindCondition` の `Filter`、`Sort`、`Include`、`Exclude` が body)。`--request` はここでは body 用途として正当であり、Filter や Sort を JSON で受ける唯一の入力経路でもある。
- query params を受ける list 操作の個別フラグ (page、per-page、name など) は、既存のすべての対象操作で query params を過不足なくカバーしている。

## Decision

- `--request` は request body を表す引数と位置づける。query params や path params をマッピングする用途には生成しない。
- query params を受ける list 操作の入力は個別フラグに統一し、`--request` を廃止する。生成器に `no_request_flag` 設定を追加し、`request_type` を持つ生成操作でも `--request` の生成を抑制できるようにする。`request_type` は SDK 呼び出しの型として必要なため残し、設定検証で `handwritten`、`request_file_only`、`request_input_transform`、フラグなし操作との併用を拒否する。
- 適用対象は、IAM list 7 操作 (folder、project、project-api-key、scim、service-policy の list-rule-templates、service-principal の list と list-keys)、AppRun list 2 操作、Security Control list 2 操作である。
- IaaS の find および読み取り系操作 (`Find`、`List`、`list-subnet`、`list-parameter`) は `--request` を維持する。IaaS の検索は GET + body という API 形式であり、`FindRequest` JSON は body として送信される。
- IAM の `folder.CreateParams` と `project.CreateParams` は request body 用の `--request` を維持し、既に対策済みの projectapikey と同じ wire 名タグ付き中間構造体による明示変換 (`DisallowUnknownFields` 併用) で `parent_id` と `parent_folder_id` のマッピングを修正する。
- query params を受ける list 操作の個別フラグは、対応する query params をすべてカバーする。SDK が params 型にフィールドを追加した場合は個別フラグの追加で追従する。

## Consequences

- list 操作の `--request` と個別フラグの相互排他チェック、JSON デコード経路が生成されなくなる。個別フラグが全 query params をカバーしているため、利用者が失う入力経路はない。
- IAM list での `per_page` 等の無視が、`--request` の廃止によって同時に解消される。タグなし `ListParams` はもう JSON でデコードされない。
- body 用 `--request` の残る実害は folder / project create の2型で、明示変換の追加により解消される。`DisallowUnknownFields` の併用により、将来 SDK が params 型にフィールドを追加したときのデコード時追従漏れはエラーとして顕在化する。
- list 操作から JSON 経路がなくなるため、SDK が params 型にフィールドを追加した場合に個別フラグを追加しないと、その query params が指定できなくなる。デコード時に黙って無視される機構 (1) に比べ、欠落がヘルプの記載差として観察しやすい。
- IAM の手書きラッパー型への json タグ追加は sacloud-sdk-go 側の変更であり、長期的な改善として SDK 側へ提案する。タグが付いた時点で skr 側の明示変換 (中間構造体) は撤去できる。
- エンコード時に落ちる機構 (2) は生成器では検出できないため、`--request` を維持するドメインでは decode から SDK 送信形への変換までを検証する round-trip テストを設ける。既知の例として、gslb / proxylb の service 層手書き型は read 出力 (`iaas.ProxyLB`) が持つ `OriginGuard`、`StrictRule` を update 入力型が持たない。

## Alternatives considered

- list 操作も `--request` を維持し、wire 名タグ付き中間構造体で対応する案: query params を JSON で渡す経路が残り、body 用の引数との混同が続く。個別フラグが全 query params をカバーしており `--request` を残す利点が少ないため採用しない。
- 生成器が操作名 (List / Find など) から list を判定して自動抑制する案: 操作の公開範囲と入力経路は設定が明示する原則 (SKILL.md) に反し、SDK メソッド名の命名規則に依存するため脆弱である。採用しない。
- list 操作を handwritten 化する案: 10 操作分のコマンド構造体とハンドラーを手書きすることになり、生成器の標準処理 (出力検証、factory 接続) の重複が増えるため採用しない。
- IAM だけを `--request` 廃止にして他ドメインは現状維持とする案: query params を body 用引数で受ける不整合が他ドメインに残る。原則はドメイン横断で一貫させるため採用しない。
