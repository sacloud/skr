# プロファイルの管理

`skr` は sacloud-sdk-go のプロファイルを使って認証情報を読み込みます。プロファイルの管理には `skr config` 以下のコマンドを使います。

## 利用可能なプロファイルを確認する

```console
$ skr config list
```

利用可能なプロファイル名を一覧します。端末への出力時は、現在選択されているプロファイル名の先頭に `* ` が付きます。

## 現在のプロファイル名を確認する

```console
$ skr config current
```

現在選択されているプロファイル名を出力します。プロファイルが未選択の場合はエラーになります。

## プロファイルの内容を確認する

```console
$ skr config show
```

現在選択されているプロファイルの内容を JSON で表示します。プロファイル名を指定すると、そのプロファイルを表示します。
認証情報を含む属性も出力されるため、表示内容を共有する際は注意してください。

```console
$ skr config show <profile-name>
```

## 関連項目

- [sacloud-sdk-go v0.3.0: プロファイル操作 `saclient.ProfileOp`](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/common/saclient#ProfileOp)
