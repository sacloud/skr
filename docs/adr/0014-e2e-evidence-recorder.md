# ADR 0014: E2E 証跡処理を共通化する

- Status: Accepted
- Date: 2026-10-05

## Context

EventBus E2E、SimpleMQ E2E、Switch E2E は、CLI 呼び出しの証跡をそれぞれ記録しています。EventBus は実行ごとの保存先、連番、順序一覧、最終結果を持つ一方、SimpleMQ と Switch は OS の一時ディレクトリに操作名だけの JSON を保存しており、保存先や実行順の追い方が揃っていませんでした。

## Decision

- `test/e2e/internal/evidence` に証跡ディレクトリと CLI 呼び出しの記録を共通化します。
- 証跡をリポジトリルートの `tmp/<service>-api/<YYYYMMDDHHmm>/` に保存し、同じ分の再実行では `-02` 以降を付けて既存ファイルを上書きしません。
- 実行ディレクトリは `0700`、JSON、`ORDER.txt`、`RESULT.txt` は `0600` にします。呼び出しごとに連番 JSON と順序一覧を記録し、実行終了時に最終結果を記録します。
- SimpleMQ と EventBus の API キー発行結果は証跡では `[REDACTED: APIKey]` に置き換えます。

## Consequences

- 3つの E2E で証跡の場所と形式が揃い、失敗後も操作順と結果を同じ方法で調べられます。
- 証跡はアカウントやリソースを特定できる情報を含むことがあります。Git 管理外に保ち、共有やコミットは避けます。
- 新しい E2E が証跡を記録する場合は、共通パッケージを使い、CLI 固有の秘密情報だけを呼び出し側でマスクします。

## Alternatives considered

- 各 E2E に証跡処理を個別に実装する方法: 保存形式のずれが再発し、秘匿処理や順序記録の更新漏れにつながるため採用しません。
- すべての E2E に単一の保存先を使う方法: サービスごとに証跡を探しやすくするため、`tmp/<service>-api/` ごとに分けます。
