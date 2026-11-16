# ADR 0086: トレースと秘密情報の方針を注意喚起に統一する

- Status: Accepted
- Date: 2026-11-16
- SDK: `github.com/sacloud/sacloud-sdk-go v0.6.0`

## Context

SDK のトレース実装 ([`common/saclient/tracer.go`](https://github.com/sacloud/sacloud-sdk-go/blob/main/common/saclient/tracer.go)) は、HTTP リクエストとレスポンスの本文を `httputil.DumpRequest` でそのまま出力します。[ADR 0020](0020-global-sdk-tracing-option.md) で導入した `--trace` と、環境変数・プロファイルでのトレース設定が有効な場合、リクエスト本文に含まれる秘密情報がログへ出力されます。

秘密情報を含む操作では、これを防ぐために CLI 層で実効トレース設定を検査し、操作を拒否するガードを実装してきました。

- EventBus の `process-configuration set-secret`: secret の値 (`APIKey`、`AccessToken`、`AccessTokenSecret`) を送信する前に検査 ([EventBus 高レベルコマンドの設計](../design/eventbus.md))
- AppRun 専有型の `deploy`: `registryPassword` や `secret=true` の環境変数の値を送信する前に検査 ([AppRun 専有型高レベルコマンドの設計](../design/apprun-dedicated.md))

この方針には次の問題がありました。

- ガードは上記 2 操作に限られ、kms の `--plain-key-file` や secretmanager の `--value-file`、proxylb・local-router の保護済み `--request` など、他の秘密情報入力経路には適用されていない。同じ性質の入力経路で扱いが分かれ、利用者が挙動を予測しにくい。
- トレースが有効かどうかは、`--trace` フラグに加えて `SAKURA_TRACE` などの環境変数とプロファイル設定の組み合わせで決まる。ガードは SDK クライアントの実効設定を検査するため、CLI の実装が SDK 内部の設定解決に依存し、エラーメッセージも SDK の設定内容 (`TraceMode` の値) を利用者へ露出していた。
- 秘密情報を含む操作と含まない操作で factory を分離する構成が、ドメインごとの手書きコードに複製されていた。

一方で、トレース出力の秘密情報は利用者の選択 (`--trace` の指定、環境変数・プロファイルの設定) に起因するものであり、CLI が操作の実行を阻止する必然性は限定的です。リクエスト本文への秘密情報の混入は、入力経路の保護 (インライン指定の拒否、`@path` 形式と標準入力への限定) で軽減済みであり、トレース有効時のログ取り扱いは利用者の判断に委ねられる性質のものです。

## Decision

- トレースと秘密情報の方針を「実行時の拒否」から「注意喚起」に統一します。秘密情報を含む操作でも、トレースが有効な場合に操作を拒否しません。
- `CheckTraceSettings` による実効トレース設定の検査を廃止します。EventBus の set-secret と AppRun 専有型の deploy からガードを取り除き、それ専用だった factory の分離 (`SecretServiceFactory`、`DeployServiceFactory`) も解消して通常の factory に統一します。
- `--trace` のヘルプに、リクエストとレスポンスの本文がそのまま出力され、秘密情報も含まれることを明記します。秘密情報を扱うコマンドのヘルプにも、`--trace` 併用時に値がログへ出力される旨を記載します。
- 設計文書 ([EventBus 高レベルコマンドの設計](../design/eventbus.md)、[AppRun 専有型高レベルコマンドの設計](../design/apprun-dedicated.md)、[local-router-api コマンドの設計](../design/local-router-api.md)) では、トレース有効時に秘密情報がログへ記録されることと、拒否しない方針を記述します。
- 入力経路の保護 (秘密情報のインライン指定の拒否、`@path` 形式と標準入力への限定) は維持します。これはトレースの出力抑制ではなく、コマンド引数への秘密情報の混入を防ぐ仕組みです。

## Consequences

- 秘密情報を含む操作でも `--trace` を付けたまま実行でき、その場合は値が stderr のログへ出力されます。利用者はログの共有・保存の前に値が含まれていないか確認する必要があります。
- トレースの有効化経路 (フラグ、環境変数、プロファイル) によらず挙動が一貫し、CLI の実装が SDK の設定解決 (`Profile`、`JSON()`) に依存しなくなります。
- ガード用の factory 分離とそのテストが不要になり、EventBus と AppRun 専有型のコマンド構成が他の高レベルコマンドと同じ形になります。
- 秘密情報がログへ出力される状態を CLI が防げないため、SDK 側での機微な値のマスキングやトレースの粒度制御は、今後 SDK 側の改善提案 (issue #75 の文脈) で扱う可能性があります。

## Alternatives considered

- ガードを全ドメインへ拡張する案: 秘密情報を含む操作の網羅と実効トレース設定の解決を CLI 側で維持する必要があり、構成の複雑さが残る。上記の問題を解消しないため採用しません。
- ガードを維持し、拒否時に警告のみへ緩和する案: 「実行するが警告する」挙動では、警告を無視する運用が定着し、ガードの維持コスト (factory 分離、実効設定の検査) に見合う利点がないため採用しません。
- 既知の秘密情報フィールド名をトレース出力でマスキングする案: SDK の tracer 出力を CLI で後処理する方式では、フィールド名の一覧の維持と漏れのリスクが続く。SDK 側の機能であり、CLI の責務を超えるため採用しません。
- `--trace` を廃止し環境変数のみに統一する案: [ADR 0020](0020-global-sdk-tracing-option.md) で一時的な調査の入口として `--trace` を採用済みであり、撤回の理由がないため採用しません。
