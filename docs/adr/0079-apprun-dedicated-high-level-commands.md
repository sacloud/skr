# ADR 0079: AppRun Dedicated の高レベルデプロイフローコマンドを導入する

- Status: Proposed
- Date: 2026-11-12

## Context

低レベル `apprun-dedicated-api` は SDK v0.5.0 のクラスタ、アプリケーション、バージョン、オートスケーリンググループ、ワーカノード、ロードバランサ、証明書、サービスクラスの操作を35コマンドで公開します ([ADR 0027](0027-apprun-dedicated-api-commands.md))。しかし、アプリケーションをデプロイする作業は、個別の操作を次の順序で組み立てる必要があり、利用者が手順を誤りやすいです。

1. クラスタ名からクラスタ ID を検索する
2. アプリケーション名でアプリケーションを検索し、なければ作成する
3. バージョンを作成する (`registryPasswordAction` を前回値との関係で選ぶ)
4. バージョンを有効化する
5. コンテナの状態をポーリングして反映を待つ

同様の作業単位を提供する [fujiwara/apprun-dedicated-cli](https://github.com/fujiwara/apprun-dedicated-cli) が公開されており、アプリケーション定義ファイルを1つ書いてデプロイする形が利用者に分かりやすいことが示されています。同 CLI は定義を Jsonnet で書きますが、skr では Jsonnet の評価機構 (環境変数・tfstate・Secret Manager の参照関数) を組み込まず、JSON 定義に絞って高レベルコマンドを提供します。[ADR 0075](0075-high-level-command-tier.md) は高レベル層の位置づけと規則を、[ADR 0077](0077-eventbus-service-library.md) は service 層を独立モジュールへ切り出す構成を定めています。

## Decision

- 高レベルコマンドの名前空間は `skr apprun-dedicated` とします ([ADR 0075](0075-high-level-command-tier.md) の規則)。
- 高レベル層の対象はデプロイフローに限定します。`init`、`deploy`、`diff`、`render`、`status`、`versions`、`rollback`、`deactivate`、`delete`、`containers` の10コマンドです。
- クラスタ、オートスケーリンググループ、ワーカノード、ロードバランサ、証明書、サービスクラスの参照・管理は低レベル `apprun-dedicated-api` が担うため、高レベル層では繰り返しません。インフラの構築管理 (Terraform など) と、アプリケーションのデプロイを分ける参考実装の方針に従います。
- アプリケーション定義は JSON ファイルのみを受け付けます。Jsonnet、環境変数参照、tfstate-lookup、Secret Manager 参照の関数は組み込みません。定義のフィールド名は API リクエストのフィールド名 (camelCase) と一致させ、高レベル層の変換を最小にします。
- 定義ファイルの入力はファイルパスと標準入力 (`-`) とします。インライン JSON は受け付けません。`registryPassword` などの秘密情報を含み得るため、低レベル層の `--request` と同じ入力経路の規律に合わせます。
- service 層は `service/apprun-dedicated-service-go/` 配下の独立モジュールに切り出します ([ADR 0077](0077-eventbus-service-library.md) の構成)。デプロイフローのロジック (名前検索、リクエスト変換、`registryPasswordAction` の判定、待機、差分用の正規化) は CLI 非依存で、sakumock 通信テストと fake 注入の単体テストで検証できます。ライブ E2E がクラスタの課金承認を待つ間 ([#59](https://github.com/sacloud/skr/issues/59)) でも service 層の検証は完結します。
- 変更系の操作 (`delete`、`deactivate`、`rollback`) は確認プロンプトを持ち、`--force` で省略します。
- 待機処理は期限を持ちます (`--no-wait` と `--wait-timeout`)。ポーリングの結果、期限までに完了しなければエラーにします。

## Consequences

- 利用者は JSON 定義1つの保守でデプロイ・ロールバック・停止を扱えます。一方、定義内の環境変数置換や秘密情報の外部参照ができなくなり、CI などでは事前にファイルを生成する運用になります。
- Jsonnet の辞書式評価や既存フィールドの省略時解釈がないため、定義はデプロイのたびに完全な形を保つ必要があります。バージョンは毎回新規作成です。
- 高レベル層は低レベル層とコマンド、ヘルプ、テスト、文書が分かれます。低レベル層の変更はありません。
- ライブ E2E は低レベル層と同じくクラスタ作成が前提のため、この ADR では実行しません。シナリオ設計と承認は [#59](https://github.com/sacloud/skr/issues/59) で管理します。
- 詳細設計は [AppRun Dedicated 高レベルコマンドの設計](../design/apprun-dedicated.md) に記録します。

## Alternatives considered

- 定義を Jsonnet でも受け付ける方式 (参考実装と同じ): 秘密情報の外部参照や環境変数置換は有用ですが、評価機構とネイティブ関数の保守・検証が重く、skr が提供する価値は JSON デプロイフローの部分に限られるため採用しません。Jsonnet を使う利用者は参考実装を使うか、CI で JSON を生成します。
- インフラ参照系も高レベル層に含める方式 (`cluster`、`load-balancer` などのコマンド): 低レベル `apprun-dedicated-api` が同一の入出力で既に提供しており、高レベル層での追加価値が確認できていないため採用しません。
- service 層を `internal/apprundedicated` に同居させる方式 ([ADR 0075](0075-high-level-command-tier.md) の当初方針): Eventbus での実績 ([ADR 0077](0077-eventbus-service-library.md)) により、service 層のロジックはライブ実行なしでも sakumock と単体テストで検証できることが確認済みです。初めから独立モジュールにする方が、後日の切り出し作業が不要になるため採用します。
- 既存アプリケーションの更新を read-modify-write にする方式 (EventBus 高レベル層と同じ): AppRun Dedicated のバージョンは作成専用の世代管理であり、更新は新バージョンの作成と有効化で表現されます。マージ規則を持たず、定義が毎回完全な入力となる方が挙動を説明しやすいため採用しません。
