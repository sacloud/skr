# ADR 0068: internal/cli の配線コードを生成する

- Status: Accepted
- Date: 2026-10-29

## Context

internal/cli の init 関数群 (約 309 行) は、IaaS 15 リソースと各 Ogen ドメインで同じ形の繰り返しです。`NewCommands` に共有の Runtime callbacks (`decodeRequest` / `validateOutput` / `writeOutput`) と遅延評価のクライアント factory クロージャを渡し、変数部分は SDK コンストラクタと代入先のフィールドだけです。1 リソースの追加は設定 JSON と internal/cli の配線の 2 か所の編集を必要とし、乖離は配線側のビルドエラーで検出されます。

非標準の配線も存在します。SimpleMQ メッセージ API は `factory_arguments` (キュー名と API キーファイル) を factory へ渡し、eventbus はドメイン固有の decoder (`eventbusapi.DecodeRequest`) を使います。transform も iaas-server はドメイン package 内の関数、iaas-packetfilter は internal/cli 内の手書き関数を参照します (ADR 0042)。

## Decision

- 生成器がコマンド package に加えて internal/cli 配下の配線コードも、同じ設定 JSON から生成します (ADR 0016 / 0019 の生成設定形式を拡張します)。
- 設定に SDK コンストラクタと配線先を明示するフィールドを追加します (例: `sdk_factory: "archive.New"`、`wiring_target: "IaaSAPI.Archive"`)。
- 生成対象は標準パターンのみです。共有の Runtime callbacks と遅延評価のクライアント factory を `NewCommands` へ接続し、`request_transform` / `request_validator` で参照するドメイン package 内の関数を接続します (ADR 0044 / 0045 の型付き callback と constructor 注入は維持します)。
- transform 関数はドメイン package に置きます。internal/cli にある手書き transform (packetfilter) は `internal/iaas/packetfilterapi/` へ移動します。
- ドメイン固有の decoder が必要な場合は設定で参照先を明示します (例: `decoder`)。既定は共有の入力処理です。
- `factory_arguments` を使うクライアント生成 (SimpleMQ メッセージ API) は手書き init に残し、設定で配線対象から外せるようにします (例: `wiring: "handwritten"`)。手書き init と生成配線は internal/cli に共存します。
- 配線コードはドメインごとに 1 ファイルにまとめます (例: `internal/cli/iaas_wiring_generated.go`)。生成器は 1 回の実行で複数の設定を処理し、具体的な Makefile の呼び出し形は設計ドキュメントに記録します。
- ルート構造体、フラグ定義、`run()` からの init 呼び出しは手書きのままにします。生成配線は cli struct のフィールドへの代入のみを行います。
- テスト注入 (`set*Factory`) は維持します。生成配線と同じ package にあるテストが、生成配線が設定した Commands を差し替えられます。

## Consequences

標準リソースの追加と変更が「設定 1 ファイル + 再生成」で完結し、internal/cli の手書き init は例外のみになります。リソース追加時に設定と配線が乖離する経路がなくなります。internal/cli に生成と手書きが混在するため、生成ファイルのヘッダーと DO NOT EDIT を明確にします。生成器の設定解析・検証、Makefile、設計ドキュメント (api-command-generation.md) も更新します。

## Alternatives considered

- 汎用 helper (generics) による重複削減は、factory クロージャの形がリソースごとに変わり表現しきれず、重複削減に留まるため採用しません。
- ルート構造体ごと生成する方法は、hand-written コマンド (config / http / version) とフラグ定義が混在し、混在が悪化するため採用しません。
- 配線コードを設定ファイルごとに分割する方法は、internal/cli に小さなファイルが増えるため採用しません。
- 現状維持は、2 か所宣言と乖離リスクが残るため採用しません。
