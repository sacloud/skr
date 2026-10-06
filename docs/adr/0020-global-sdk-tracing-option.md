# ADR 0020: SDK の HTTP トレースをグローバルオプションで有効にする

- Status: Accepted
- Date: 2026-10-06

## Context

API 呼び出しの調査では、SDK が送受信する HTTP リクエストとレスポンスを確認する必要があります。`saclient` は trace mode を提供しますが、利用者が環境設定を変更せずに一時的な調査で有効化できる、CLI 共通の方法が必要です。

## Decision

- ルート CLI に `--trace` を追加し、指定時は `saclient.WithTraceMode("all")` を適用します。
- IaaS、ゾーン検索、EventBus、SimpleMQ、`http` の各クライアントで同じ trace 設定を使います。
- `--trace` を指定しない場合は trace mode を上書きせず、既存のプロファイルや環境設定を維持します。
- ヘルプと README で、トレース出力に認証情報などが含まれる場合があることを明示します。

## Consequences

- すべての対応 API 経路で同じグローバルフラグを使って HTTP 通信を調査できます。
- `--trace` は SDK の認証情報やリクエスト内容をログに出力する可能性があるため、出力を共有・保存する場合は秘密情報の扱いに注意が必要です。

## Alternatives considered

- SDK の trace mode を環境変数だけで設定する方法: 利用者ごとの一時的な有効化に追加の設定手順が必要なため採用しません。
- 各 API サブコマンドに個別の trace フラグを設ける方法: コマンド間で挙動と設定が重複するため採用しません。
