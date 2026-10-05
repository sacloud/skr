# ADR 0010: SimpleMQ API コマンドを SDK 操作に対応させる

- Status: Accepted
- Date: 2026-10-05

## Context

`sacloud-sdk-go v0.3.0` は SimpleMQ のキュー管理 API とメッセージ API を公開しています。
EventBus の実行先として SimpleMQ のキューが参照されるため、キューの管理とメッセージ操作を
CLI から直接利用できるようにする必要があります。

## Decision

- `skr simplemq-api queue` は SDK の `QueueAPI` が公開する list、read、create、config、delete、
  count-messages、rotate-api-key、clear-messages を個別コマンドとして提供します。
- `skr simplemq-api message` は SDK の `MessageAPI` が公開する send、receive、extend-timeout、delete
  を個別コマンドとして提供します。
- SDK のクライアント、操作、リクエスト、レスポンスをそのまま利用します。Queue の create は
  コマンドが SimpleMQ の Provider を設定します。`queue config` は可視性タイムアウトと保存期間を
  個別フラグで受け付け、SDK リクエストの `Settings` に組み立てます。その他の複雑な作成・設定項目は
  SDK 型の JSON で受け付けます。
- キュー管理 API は sacloud-sdk-go の標準プロファイル／環境変数を使います。メッセージ API の
  API キーはフラグに直接含めず、ファイルまたは標準入力から読み込みます。キー発行操作は SDK が
  返す値を通常のコマンド出力として返します。
- SimpleMQ のエンドポイント、認証、リクエスト制約は SDK と同梱の OpenAPI 定義に従い、
  CLI 独自のサービス挙動は追加しません。

## Consequences

- キュー ID とメッセージ API が使うキュー名は別のモノです。キュー管理操作は ID、メッセージ操作は
  キュー名と API キーを使います。
- `config` は `--visibility-timeout-seconds` と `--expire-seconds` の両方を必要とします。
  `--request` による SDK 型 JSON 入力も利用でき、個別フラグとは併用できません。
- `rotate-api-key` のレスポンスには秘密情報が含まれます。利用者は出力先を安全に扱う必要があります。

## Alternatives considered

- メッセージ操作を EventBus 専用にする方法: キューとメッセージの SDK 操作を直接使いたい利用者の
  用途を満たさないため採用しません。
- キー発行からメッセージ送受信までを一連の高レベル操作にまとめる方法: SDK の各操作を直接公開する
  `simplemq-api` の役割に合わないため採用しません。
