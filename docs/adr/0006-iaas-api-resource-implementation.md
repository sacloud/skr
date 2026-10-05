# ADR 0006: IaaS API リソースごとにコマンドと検証を分ける

- Status: Accepted
- Date: 2026-10-05

## Context

skr の低レベル API コマンドは、[ADR 0005](0005-low-level-and-high-level-command-tiers.md) に従って SDK の公開操作を提供します。IaaS には複数のリソースがあり、それぞれ公開操作や入出力が異なります。リソースの追加に伴って親コマンドが肥大化しない構成と、実際の CLI 経路を検証する方法が必要です。

採用中の `sakumock v0.9.1` には IaaS mock package がありません。IaaS リソースの CLI テストでは、sakumock の対応を待たずに SDK 経由の操作を検証する必要があります。

## Decision

- IaaS リソースは `skr iaas-api <resource>` に登録します。ルートの初期化を `main.go`、`iaas-api` の親コマンドとリソース登録を `iaas.go`、各リソースのコマンドと SDK 呼び出しをリソース専用ファイルに分けます。
- 公開する操作、入力モデル、出力、zone や ID の要否はリソースごとに SDK の公開 API を確認して決めます。全リソースに同じ CRUD 操作や引数を仮定しません。
- sakumock が対象の IaaS 操作に対応するまでは、SDK の公開 caller interface を満たすテスト用モックを必要な操作に限って `internal/sakumock/iaas` に実装します。CLI 側の API factory からテスト用 caller を差し替えます。sakumock 対応後は、テスト用 caller としてそちらを利用できます。
- ライブ E2E はリソースごとの再実行可能な Go スクリプトでビルド済み skr CLI を操作します。対象環境と削除を含む副作用の承認を得て、実行確認を必須にします。事前削除を認める場合はテスト専用の識別情報と ID を再照合し、今回作成したリソースも照合してから片付けます。証跡は Git 管理外に保存します。

## Consequences

- IaaS リソースを追加するときは、親コマンドを肥大化させずに固有の操作、ヘルプ、テストを追加できます。
- ローカルモックは対象リソースに必要な API 操作だけを再現します。sakumock 全体や未実装の IaaS リソースの代替にはなりません。
- SDK の公開操作やモデルが変わった場合は、該当するコマンド、ヘルプ、モック、テスト、関連ドキュメントを更新します。
- ライブ E2E で観察できないサービス動作を検証済みとは扱いません。承認できない事前削除や安全に特定できないリソースの削除は行いません。

## Alternatives considered

- sakumock が IaaS に対応するまで実装を待つ方法: 現在利用可能な SDK `APICaller` interface で CLI 経路をテストできるため採用しません。
- SDK を介さず HTTP リクエストを直接実装する方法: SDK の public service API を利用するプロジェクト方針に反するため採用しません。
- リソースごとに `<resource>-api` コマンドを作る方法: ADR 0005 の `<domain>-api <resource>` という階層に従うため採用しません。
