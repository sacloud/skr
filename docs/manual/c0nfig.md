# プロファイルの管理

`skr` は sacloud-sdk-go のプロファイルを使って認証情報を読み込みます。プロファイルの管理には `skr config` 以下のコマンドを使います。

## サービスプリンシパルキーを profile に設定する

`skr` の標準的な認証方式はサービスプリンシパルキーです。サービスプリンシパルとサービスプリンシパルキーは、さくらのクラウド コントロールパネルで事前に作成してください。キー作成時に登録した公開鍵と対応する秘密鍵 PEM ファイルを用意し、コントロールパネルで確認できるサービスプリンシパル ID とキーの KID を控えます。詳細は[さくらのクラウド サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)を参照してください。

新しい profile を作成するには `skr config create` を使います。profile 名を省略すると `default` を使います。同名の profile が存在する場合はエラーになります。

```console
$ skr config create <profile-name> \
    --service-principal-id <SERVICE-PRINCIPAL-ID> \
    --service-principal-key-id <KEY-ID> \
    --private-key-file <PATH-TO-PRIVATE-KEY.pem>
```

作成した profile を現在の profile に設定するには `--use` を指定します。

```console
$ skr config create <profile-name> \
    --service-principal-id <SERVICE-PRINCIPAL-ID> \
    --service-principal-key-id <KEY-ID> \
    --private-key-file <PATH-TO-PRIVATE-KEY.pem> \
    --use
```

端末で対話的に実行すると、profile 名、サービスプリンシパル ID、キーの KID、秘密鍵ファイルのパスを順に尋ねます。対話モードでは、既存の値を表示して変更するかを確認し、入力を省略した項目は変更しません。秘密鍵の内容は端末に入力せず、profile にも複製しません。

## 既存 profile の認証情報を編集する

`skr config edit` で既存の profile のサービスプリンシパルキー認証情報を更新します。profile 名を省略すると現在の profile を対象とします。`--service-principal-id`、`--service-principal-key-id`、`--private-key-file` のいずれかを指定すると、その項目だけを更新して他の認証情報や CLI/SDK 設定は保持します。指定がない場合は対話モードになります。

```console
$ skr config edit <profile-name> --service-principal-key-id <NEW-KEY-ID>
```

`edit` で現在の profile 以外を編集した場合、対話モードでは完了後に現在の profile へ切り替えるかを確認します。非対話利用では `--use` を指定した場合だけ切り替えます。

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

現在選択されているプロファイルの内容を JSON で表示します。プロファイル名を指定すると、そのプロファイルを表示します。アクセストークン、アクセストークンシークレット、秘密鍵の内容とパスは伏せて表示されます。

```console
$ skr config show <profile-name>
```

## API キーを使う場合

`skr` は API キー（`access_token` と `access_token_secret`）を対話的に入力・保存するフローを提供しません。API キーで認証する場合は、 sacloud-sdk-go が読み込める形式で profile ファイルを手動で編集してください。これは API キー認証そのものを禁止するものではありません。

## 関連項目

- [さくらのクラウド サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)
- [sacloud-sdk-go v0.3.0: プロファイル操作 `saclient.ProfileOp`](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/common/saclient#ProfileOp)
