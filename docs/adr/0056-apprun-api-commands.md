# ADR 0056: AppRun 共用型 API コマンドを共通生成器で実装する

- Status: Accepted
- Date: 2026-10-25

## Context

`sacloud-sdk-go v0.3.1` は AppRun 共用型 API のアプリケーション、バージョン、パケットフィルター、トラフィック、ユーザーの操作を公開しています。AppRun Dedicated API とは別の API ドメインであり、両者のリソースと操作を混在させずに CLI から利用できるようにする必要があります。

アプリケーションの作成・更新リクエストにはコンテナレジストリの認証情報や Secret 値を含められます。これらの値をコマンドライン引数に露出せず、SDK が公開する操作範囲とリクエストモデルを保つ必要があります。

## Decision

- `skr apprun-api` を AppRun 共用型 API の低レベルコマンドとして提供し、実装を `internal/apprunapi/` に置きます。定型的なコマンドと SDK 接続は `cmd/apigen-api` の共通生成器で生成し、操作範囲とヘルプは `api/commands/apprun-*.json` に明示します。
- SDK v0.3.1 の公開操作に合わせて、次の範囲を公開します。
- Application は List / Read / Create / Update / Delete / Status を、Version は List / Read / Delete / Status を公開します。SDK にない Version Create は追加しません。
- PacketFilter は Read / Update、Traffic は List / Update、User は Read / Create を公開します。SDK にない操作は追加しません。
- AppRun の作成・更新リクエストは認証情報を含む可能性があるため、`--request @path.json` または `--request -`（標準入力）のみ受け付けます。ほかの JSON リクエストは既存の `--request` 入力に従い、直接指定、`@path.json`、標準入力を利用できます。
- SDK プロファイルまたは既存の認証環境変数を使い、共通 JSON 出力と `--query` の処理へ接続します。API エラーは呼び出し元へ返します。
- sakumock の AppRun サーバーを使い、CLI 経由の操作、入力制約、応答と削除後の不在を検証します。ライブ E2E runner は明示的な確認フラグを必須にし、作成したアプリケーションの ID と内容を再確認してから削除します。

## Consequences

- SDK の公開操作を共通生成器の設定と生成コードで管理し、SDK の API モデルを CLI の入出力に保ちます。
- アプリケーションの作成・更新では JSON リクエストをファイルまたは標準入力で準備する必要があります。秘密情報が引数、ヘルプ、エラー、通常出力へ漏れることを避けます。
- ライブ E2E はアプリケーションを1件作成し、公開 URL へ接続してから削除します。URL へのリクエストは課金対象のインスタンスを起動する可能性があり、実行時にはプロジェクトと料金を確認する必要があります。
- AppRun Dedicated の操作範囲、SDK API、ドキュメントは別に管理します。

## Alternatives considered

- 手書きで全コマンドを実装する案は採用しません。SDK の定型的な操作接続は Ogen 系 API の共通生成器で表現でき、設定からの再生成とテストを維持できます。
- 作成・更新リクエストをインライン JSON でも受け付ける案は採用しません。リクエストに認証情報や Secret 値を含められるため、値がプロセス引数に残る入力経路を許可しません。
- AppRun Dedicated API と同じコマンド群にまとめる案は採用しません。API ドメインと SDK のリソースが異なり、操作の意味と入力も別々に維持する必要があります。
