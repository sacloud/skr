# ADR 0077: EventBus の service 層を service/eventbus-service-go に切り出す

- Status: Accepted
- Date: 2026-11-10

## 背景

[ADR 0075](0075-high-level-command-tier.md) では、高レベルドメインの service 層と CLI コマンドを同じパッケージ (`internal/<domain>`) に置き、service 層のファイルを CLI の型やヘルパーに参照させない規律で移植性を確保するとしました。その上で「service 層を独立パッケージに分離する方式は、この規模では分割の利益が小さい」として見送っています。

`skr eventbus` を実装しライブ E2E で動作確認した結果、service 層 (`Service`、リクエスト型、検証、変換、出力射影) は CLI に依存しない自前の単体テストと sakumock 通信テストで完結することが確認できました。service 層を独立モジュールとして切り出すと、CLI を経由せずに低レベル API を使う利用者へ同じ高レベル操作を提供でき、将来の `sacloud-sdk-go` への移植 ([EventBus 高レベルコマンドの設計](../design/eventbus.md) の「SDK への移植手順」) も、ファイル移動ではなくモジュールの移動として行えます。

## 決定

- EventBus の service 層を `service/eventbus-service-go/` 配下の独立モジュールに切り出します。モジュールパスは暫定で `github.com/sacloud/skr/service/eventbus-service-go` とし、パッケージ名は `eventbus` です。ルートの `go.mod` は `replace` ディレクティブで参照します。
- ライブラリは `sacloud-sdk-go` と標準ライブラリのみに依存し、kong、CLI の Runtime、出力処理に依存しません。
- 出力射影 (`ProjectProcessConfiguration`、`ProjectSchedule`、`ProjectTrigger`) をライブラリに含めます。[ADR 0075](0075-high-level-command-tier.md) の「射影は CLI の出力処理が担う」を本 ADR で改定します。secret を出力に含めない処理は利用者の安全に関わる共通の関心事であり、低レベル API を直接使う利用者にも有用です。CLI は射影結果をそのまま JSON 出力します。
- CLI 固有の処理は `internal/eventbus` に残します。`--condition` の構文解析 (`ParseConditions`)、secret の入力読み込み (`--secret-file @path` と標準入力)、kong コマンド、`Runtime`、`ServiceFactory` が対象です。これらはライブラリの `TriggerCondition` や `Service` を参照します。
- ライブラリの公開 API には、コードを読むだけで使える doc comment と、コンパイルが通る利用例 (`example_test.go`) を付けます。
- `make test`、`make lint-go`、`make vulncheck` は、`includes/go/common.mk` の `SUBMODULES` 変数に列挙した独立モジュールにも実行します。新しい service モジュールを切り出したときはこの変数へ追加します。
- service 層の wire 形式やマージ規則のテストはライブラリの単体テスト (fake を注入) と sakumock 通信テストで行い、`internal/eventbus` では CLI 固有の処理だけを単体テストします。コマンド全体の正常系は `internal/cli` の sakumock テストで検証します。

## 影響

- `service/eventbus-service-go/` は自身の `go.mod`、`go.sum`、`.golangci.yml`、`README.md` を持ち、ルートのビルドから独立してテストと lint ができます。service/ 配下のファイルと README では skr の ADR を参照しません。判断の背景はリポジトリ側の設計文書で追跡します。
- 新しい service 層を切り出すドメインが出た場合は、この構成 (`service/<name>-service-go`) を踏襲し、`SUBMODULES` 変数への追加と設計文書・ADR の記録が必要です。
- `sacloud-sdk-go` へ移植する際は、モジュールパスと設計文書の参照を更新します。移植前のモジュールパスは暫定であり、利用者に直接 import させる前提ではありません。
- ルート `go.mod` の `require` に暫定バージョンのエントリと `replace` が並びます。リリースの依存解決に影響しないよう、`replace` はコミットし続けます。

## 検討した選択肢

- service 層を `internal/eventbus/service` サブパッケージに分離する方式 ([ADR 0075](0075-high-level-command-tier.md) で見送り): 参照時の名前が曖昧で、upstream 時の移動単位にもなりません。独立モジュールとして切り出す方式を採用します。
- 直ちに `sacloud-sdk-go` へ移動する方式: 公開 API の確定と運用実績が前提です。skr 内で暫定管理し、設計文書の移植手順に従って提案します。
- `go.mod` を置かずリポジトリ内のサブディレクトリのままにする方式: 依存の分離と単独の lint・ビルドができず、CLI からの参照規律がファイル配置に頼ることになります。採用しません。
