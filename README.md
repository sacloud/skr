# skr

skr は、さくらのクラウドを操作するための CLI ツールです。

このリポジトリでは、さくらのクラウドの CLI ツールの新しいバージョンを `skr` として開発しています。
従来の usacloud との互換性は保証されません。

## 使い方

現在選択されているプロファイル名は、次のコマンドで確認できます。

```console
$ skr config current
my-profile
```

EventBus API のリソースは `eventbus-api` から操作できます。

```console
$ skr eventbus-api process-configuration list
$ skr eventbus-api schedule list
$ skr eventbus-api trigger list
```

各リソースは `list`、`read`、`create`、`update`、`delete` を提供します。作成・更新には
SDK の `CreateCommonServiceItemRequest` または `UpdateCommonServiceItemRequest` JSON を
`--request` に指定してください。作成時の EventBus provider class はコマンドが設定します。
JSON ファイルを使う場合は `@` に続けてパスを指定します。
プロセス設定には `update-secret` もあり、`--secret-file` で JSON ファイルを指定します
（`-` を指定すると標準入力から読み込みます）。

## ライセンス

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
