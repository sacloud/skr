# skr

`skr` は、さくらのクラウドを操作する CLI ツールです。

> [!WARNING]
> 🚧 **開発中バージョンです。** `skr` は現在開発中の CLI であり、コマンド、オプション、設定、入出力は変更されることがあります。利用前やアップデート前に、対象コマンドの `--help` と関連ドキュメントを確認してください。既存の `usacloud` との互換性は保証しません。

## コマンド

現在利用できる主なコマンドは次のとおりです。

| コマンド | 操作できる API |
| --- | --- |
| `skr iaas-api server` | サーバの検索、参照、作成、更新、削除 |
| `skr iaas-api switch` | スイッチの検索、参照、作成、更新、削除 |
| `skr simplemq-api queue` | キューの管理、メッセージ数の確認、キュー内メッセージの削除 |
| `skr simplemq-api message` | メッセージの送信、受信、タイムアウト延長、削除 |
| `skr eventbus-api process-configuration` | 実行設定の管理、実行先サービス用シークレットの登録 |
| `skr eventbus-api schedule` | スケジュールの管理 |
| `skr eventbus-api trigger` | イベントトリガーの管理 |
| `skr config current` | 現在選択されているプロファイル名の確認 |

コマンドの一覧と使い方は、`skr --help` または各コマンドの `--help` で確認できます。

```console
$ skr --help
$ skr iaas-api switch --help
$ skr simplemq-api --help
$ skr eventbus-api --help
```

Server creation requires a CPU/memory combination available in the target zone. Use a JSON `--request` file for array-based configuration. Confirm the target environment and pricing before creating a server ([server creation and deletion](https://manual.sakura.ad.jp/cloud/server/create-delete.html)).

```console
$ skr iaas-api server create --zone ZONE --name SERVER-NAME --cpu 1 --memory-gb 1
```

## 認証と出力

IaaS API、EventBus API、SimpleMQ のキュー管理 API は、SDK のプロファイル、または
`SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で認証します。SimpleMQ の
メッセージ API では対象キューの API キーが必要です。キーは `--api-key-file` でファイルから
読み込み、コマンドライン引数に直接含めないでください。

API コマンドの出力形式は `--output json`、`--output yaml`、`--output table` で選択できます。
省略時はプロファイルの `cli.default_output_type`（プロファイル v0 では `DefaultOutputType`）を
使い、未設定の場合は JSON で出力します。

## 利用者向けドキュメント

[利用者向けドキュメント一覧](docs/manual/README.md)から、チュートリアルや共通機能の解説を確認できます。
HTML 版は [GitHub Pages](https://sacloud.github.io/skr/) で公開しています。CLI の全サブコマンドのヘルプも一覧で確認できます。

## 開発

リポジトリのルートで次のコマンドを実行します。

```console
$ make build
$ ./skr --help
$ make test
```

## ライセンス

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
