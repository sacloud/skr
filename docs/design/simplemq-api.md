# SimpleMQ API コマンド設計

- Status: Accepted
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`
- ADR: [0010](../adr/0010-simplemq-api-command.md)

## コマンド

```text
skr simplemq-api queue list
skr simplemq-api queue read <id>
skr simplemq-api queue create [--name ... | --request ...]
skr simplemq-api queue config <id> (--visibility-timeout-seconds ... --expire-seconds ... | --request ...)
skr simplemq-api queue delete <id>
skr simplemq-api queue count-messages <id>
skr simplemq-api queue rotate-api-key <id>
skr simplemq-api queue clear-messages <id>

skr simplemq-api message send --queue-name ... --api-key-file ... --content ...
skr simplemq-api message receive --queue-name ... --api-key-file ...
skr simplemq-api message extend-timeout --queue-name ... --api-key-file ... <message-id>
skr simplemq-api message delete --queue-name ... --api-key-file ... <message-id>
```

## SDK 操作との対応

| CLI | SDK |
| --- | --- |
| `queue list` | `QueueAPI.List` |
| `queue read` | `QueueAPI.Read` |
| `queue create` | `QueueAPI.Create` |
| `queue config` | `QueueAPI.Config` |
| `queue delete` | `QueueAPI.Delete` |
| `queue count-messages` | `QueueAPI.CountMessages` |
| `queue rotate-api-key` | `QueueAPI.RotateAPIKey` |
| `queue clear-messages` | `QueueAPI.ClearMessages` |
| `message send` | `MessageAPI.Send` |
| `message receive` | `MessageAPI.Receive` |
| `message extend-timeout` | `MessageAPI.ExtendTimeout` |
| `message delete` | `MessageAPI.Delete` |

`QueueAPI` は queue API クライアントで作成し、メッセージ操作ではキュー名と API キーを使って
message API クライアントを作成します。キュー管理の API 認証とメッセージ API のキー認証は
SDK が行います。認証情報を含む値を CLI のフラグとして受け取らず、API キー入力は既存の
秘密ファイル／標準入力読み込み関数を利用します。

## 入力と出力

- `queue create` は `Name`、任意の `Description` をフラグで受け取れます。タグや Icon などの
  複雑な値には SDK の `CreateQueueRequest` JSON を使います。`Provider.Class` はコマンドが設定します。
  JSON と個別フラグは併用できません。
- `queue config` は `--visibility-timeout-seconds` と `--expire-seconds` をフラットな CLI フラグで受け取り、
  SDK の `ConfigQueueRequest.CommonServiceItem.Settings` に設定します。JSON の `--request` 経路も維持し、
  `Description`、`Tags`、`Icon` など SDK リクエストのその他の項目を指定できます。個別フラグと JSON は
  併用できません。`VisibilityTimeoutSeconds` は5〜900秒、`ExpireSeconds` は60〜1209600秒です。
- `queue rotate-api-key` は SDK が返した API キーを `APIKey` フィールドとして通常の出力形式で返します。
- `message send` は SDK のスカラー引数として本文を受け取ります。SDK OpenAPI 定義では最大256000文字で、
  英数字、`+`、`/`、`=` の文字に制約されています。
- 操作結果はグローバル `--output` に従って JSON、YAML、table で出力します。delete と clear-messages
  は成功時に出力しません。

## 検証

`github.com/sacloud/sakumock/simplemq` のテストサーバーに両方の SDK エンドポイントを向け、
実際の CLI パスから queue と message の操作を検証します。テストでは SDK の認証情報にダミー値を
使い、message API は strict モードでキュー発行キーを確認します。

## ライブ E2E

`test/e2e/simplemq` は、ビルド済み skr を使って SimpleMQ API 全体の操作を再実行します。ライブ API
を使うため、選択中プロファイルと対象プロジェクトを確認してから、作成・削除を含む実行を明示的に
承認してください。

```console
$ make build
$ skr config current
$ go run ./test/e2e/simplemq --skr ./skr --confirm-simplemq-live
```

スクリプトはランダムな名前と専用説明値を付けたキューを作成し、設定、API キー発行、メッセージの
送信・受信・可視性タイムアウト延長・削除、メッセージ数を確認します。作成したキューの一覧を
table 形式でも記録し、利用者向けチュートリアルの出力例に使います。実行前に同じキュー名が
ないことを確認し、既存リソースの削除や再利用はしません。後始末では ID、キュー名、説明を再読込で
照合してから、メッセージを消去してキューを削除し、一覧から消えたことを確認します。識別情報の不一致、
読み取り失敗、または削除後の残存を検知した場合は推測で削除せず、エラーを返します。

操作ごとの引数、出力、エラーは共通の `test/e2e/internal/evidence` を使い、`tmp/simplemq-api/<YYYYMMDDHHmm>/` に記録します。同じ分に
複数回実行した場合は `-02` 以降を付け、実行ごとのディレクトリは mode `0700`、証跡ファイルは
`0600` にします。API キー出力は証跡で redact し、メッセージ操作用のキーは別の一時ディレクトリに
mode `0600` で保存して実行終了時に削除します。証跡にはプロジェクト ID やリソース ID を含むため、
共有・コミットせず、確認後に削除してください。E2E のシナリオ、衝突拒否、失敗時の後始末、識別情報の再確認は
`test/e2e/simplemq/main_test.go` の fake CLI テストで確認します。2026-10-05 に承認済みプロファイルで
ライブ E2E を実行し、キュー作成からメッセージ操作、キュー削除までのシナリオが成功することを確認しました。
