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
- 操作結果は JSON で出力し、`--query` で加工できます。delete と clear-messages
  は成功時に出力しません。

## 検証

`github.com/sacloud/sakumock/simplemq` のテストサーバーに両方の SDK エンドポイントを向け、
実際の CLI パスから queue と message の操作を検証します。テストでは SDK の認証情報にダミー値を
使い、message API は strict モードでキュー発行キーを確認します。

## ライブ E2E

SimpleMQ API のライブ E2E は `test/e2e/simplemq` の `scenario_simplemq_test.go` で検証します。
`SKR_E2E` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test`
では実行環境を変更しません。

シナリオはランダムな名前と説明値を付けたキューを作成し、設定、API キー発行、メッセージの送信・受信・
可視性タイムアウト延長・削除、メッセージ数を確認します。作成したキューの一覧を
`--query 'map({ID,Name})'` で投影した JSON でも確認し、ID と名前を照合します。実行前に同じキュー名が
ないことを確認し、既存リソースの削除や再利用はしません。後始末では ID、キュー名、説明を再読込で
照合してから、メッセージを消去してキューを削除し、一覧から消えたことを確認します。識別情報の不一致、
読み取り失敗、または削除後の残存を検知した場合は推測で削除せず、エラーを返します。CLI 呼び出しのたびに
説明を付けて実行し、実行記録は helper が Git 管理外の `e2e-output/` 配下の Markdown に保存します。
API キーの出力は CLI テストで redact を確認し、メッセージ操作用のキーはテスト終了時に削除する一時
ファイル経由で扱います。CLI の入力エラーと sakumock を使った API 操作の確認は `internal/cli` の
単体テストで行います。2026-10-05 に承認済みプロファイルでライブ E2E を実行し、キュー作成から
メッセージ操作、キュー削除までのシナリオが成功することを確認しました。
