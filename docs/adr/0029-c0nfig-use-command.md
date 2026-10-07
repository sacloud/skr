# ADR 0029: config use コマンドの追加

- Status: Proposed
- Date: 2026-10-07

## Context

`skr` にはプロファイルの一覧・表示・作成・編集コマンドがあります。現在の profile の切り替えは `config create --use` と `config edit --use` でのみ行えます。既存 profile を後から切り替えるには `config edit <name> --use` を使うか、profile ディレクトリの `current` ファイルを手動で書き換える必要がありました。

既存 usacloud には `config use` コマンドがあり、指定した profile を現在の profile に切り替えられます。[ADR 0028](0028-use-service-principal-key-for-authentication-setup.md) では、profile の切り替えは作成・編集とは別の操作にすると定めています。

## Decision

- `skr config use [name]` コマンドを追加し、指定した profile を現在の profile に設定します。切り替えには sacloud-sdk-go の `saclient.ProfileOp.SetCurrentName` を使います。
- profile 名は位置引数で受け取り、省略した場合は現在の profile が対象になります。これは `config show` / `config edit` と同じ引数の扱いです。
- 存在しない profile 名を指定した場合はエラーになり、現在の profile は変更されません。存在確認は `SetCurrentName` が行います。
- 成功時には何も出力しません。切り替え後の選択は `config current` と `config list` で確認します。
- このコマンドは profile の選択のみを変更し、profile の内容は更新しません。
- この決定は CLI の profile 選択コマンドに関するものです。usacloud と同名のコマンドを提供しますが、usacloud との互換性は保証しません。

## Consequences

- 既存 profile を後から現在の profile に切り替えられます。`current` ファイルの手動書き換えが不要になります。
- 利用者向けドキュメント（[プロファイルの管理](../manual/c0nfig.md)）と [認証とプロファイルの設計](../design/authentication-and-profile.md) に `config use` の説明を追加します。
- 単体テストと `test/e2e/c0nfig` の E2E シナリオに、profile 切り替えの検証ステップを追加します。

## Alternatives considered

- profile 名を必須引数にする方法: 意図しない実行を防げますが、`config show` / `config edit` と引数の扱いが分かれます。usacloud と同じ省略可能な引数を採用します。
- 成功メッセージを表示する方法: `config create` / `config edit` は成功メッセージを表示しますが、`use` は選択だけを変える操作のため、usacloud に合わせて何も出力しません。確認は `config current` で行えます。
- `config edit --use` での代用: 切り替えだけが目的の場合に認証情報の編集フローを経由する必要があり、[ADR 0028](0028-use-service-principal-key-for-authentication-setup.md) の「切り替えは作成・編集とは別の操作」という方針に合いません。
