# skr

`skr` は、さくらのクラウドを操作する CLI ツールです。

> [!WARNING]
> 🚧 **開発中バージョンです。** `skr` は現在開発中の CLI であり、コマンド、オプション、設定、入出力は変更されることがあります。利用前やアップデート前に、対象コマンドの `--help` と関連ドキュメントを確認してください。既存の `usacloud` との互換性は保証しません。

## リリースの入手

[GitHub Releases](https://github.com/sacloud/skr/releases) から、対象 OS・CPU 向けの ZIP ファイルと SHA-256 チェックサムを取得できます。
バイナリは Linux（amd64、386、arm、arm64）、macOS（amd64、arm64）、Windows（amd64、386）向けに公開します。
ZIP ファイルを展開し、`skr`（Windows では `skr.exe`）を PATH の通ったディレクトリに配置してください。

コンテナイメージは `ghcr.io/sacloud/skr` で公開します。Linux の amd64 と arm64 に対応しています。
バージョンを固定する場合は `v` 付きのリリースタグを指定してください。`latest` は最新の正式リリース、`dev` は `main` ブランチの開発版です。
プレリリースでは `latest` を更新しません。Homebrew 向けの配布は行いません。

```console
$ docker run --rm ghcr.io/sacloud/skr:latest --help
```

## コマンド

現在利用できる主なコマンドは次のとおりです。

| コマンド | 操作できる API |
| --- | --- |
| `skr iaas-api disk` | ディスクの検索、参照、作成、更新、削除 |
| `skr iaas-api server` | サーバの検索、参照、作成、更新、削除 |
| `skr iaas-api switch` | スイッチの検索、参照、作成、更新、削除 |
| `skr iaas-api zone` | ゾーン一覧の取得 |
| `skr simplemq-api queue` | キューの管理、メッセージ数の確認、キュー内メッセージの削除 |
| `skr simplemq-api message` | メッセージの送信、受信、タイムアウト延長、削除 |
| `skr eventbus-api process-configuration` | 実行設定の管理、実行先サービス用シークレットの登録 |
| `skr eventbus-api schedule` | スケジュールの管理 |
| `skr eventbus-api trigger` | イベントトリガーの管理 |
| `skr apprun-dedicated-api` | AppRun 専有型のクラスタ、アプリケーション、ワーカノードなどの管理 |
| `skr container-registry-api registry` | コンテナレジストリと認証ユーザーの管理 |
| `skr iam-api user` | IAM ユーザーの作成、参照、更新、削除、メールアドレスの登録と解除 |
| `skr iam-api group` | IAM グループの管理とメンバーシップの更新 |
| `skr iam-api policy` | 組織・プロジェクト・フォルダの IAM ポリシーバインディングの参照と更新 |
| `skr iam-api auth` | 組織の認証条件とパスワードポリシーの参照・更新 |
| `skr iam-api folder` / `project` | IAM フォルダとプロジェクトの管理・移動 |
| `skr iam-api organization` | 組織情報とサービス利用ポリシーの管理 |
| `skr iam-api iam-role` / `id-role` | IAM ロールと ID ロールの参照 |
| `skr iam-api id-policy` | 組織の ID ポリシーの参照・更新 |
| `skr iam-api project-api-key` | プロジェクト API キーの管理 |
| `skr iam-api service-principal` | サービスプリンシパルと公開鍵の管理、アクセストークンの発行 |
| `skr iam-api service-policy` | サービス利用ポリシーの状態とルールテンプレートの操作 |
| `skr iam-api scim` | ユーザープロビジョニング設定の管理 |
| `skr iam-api sso` | SSO プロファイルの管理 |
| `skr iam-api user-2fa` | ユーザーの OTP、信頼済みデバイス、セキュリティキーの管理 |
| `skr http <url>` | SDK の認証情報を使った任意の HTTPS エンドポイントへのリクエスト |
| `skr config current` | 現在選択されているプロファイル名の確認 |

コマンドの一覧と使い方は、`skr --help` または各コマンドの `--help` で確認できます。

```console
$ skr --help
$ skr iaas-api disk --help
$ skr iaas-api switch --help
$ skr simplemq-api --help
$ skr eventbus-api --help
$ skr apprun-dedicated-api --help
$ skr container-registry-api --help
$ skr http --help
```

Server creation requires a CPU/memory combination available in the target zone. Use a JSON `--request` file for array-based configuration. Confirm the target environment and pricing before creating a server ([server creation and deletion](https://manual.sakura.ad.jp/cloud/server/create-delete.html)).

```console
$ skr iaas-api server create --zone ZONE --name SERVER-NAME --cpu 1 --memory-gb 1
```

## 認証と出力

IaaS API、EventBus API、SimpleMQ のキュー管理 API、AppRun Dedicated API は SDK のプロファイルか
`SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で認証します。Container Registry API と IAM API も同じ認証方法です。

SimpleMQ のメッセージ API では対象キューの API キーが必要です。キーは `--api-key-file` でファイルから読み込みます。コマンドライン引数に直接含めないでください。

IAM のユーザー作成・更新で指定するパスワードは `--password-file` でファイルまたは標準入力から読み込みます。値をコマンドライン引数に直接含めないでください。

Container Registry のユーザーパスワードも `--password-file` で保護されたファイルまたは標準入力から読み込み、引数に直接含めないでください。
Container Registry API の手順は[チュートリアル](docs/manual/tutorials/container-registry-api.md)を参照してください。
IAM の各機能を利用するには、対象機能の権限を付与したサービスプリンシパルが必要です
（[サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)、[IAM ポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。
IAM の認証条件、SSO、ユーザープロビジョニング、サービス利用ポリシー、ID ポリシーは組織やユーザーの認証・操作に影響します。ポリシー更新は対象範囲の設定全体を置き換える操作があるため、更新前に現在値を読み取り、内容を確認してください。プロジェクト API キーや SCIM トークン、サービスプリンシパルのアクセストークンなどの秘密情報は、作成・再発行時の出力を安全に保管してください。詳しくは [IAM API チュートリアル](docs/manual/tutorials/iam-api.md)を参照してください。

API コマンドの結果は JSON で出力します。
`--query` に jq 式を指定すると、API の結果を加工して JSON で出力できます。詳細は
[API 出力の jq 加工ガイド](docs/manual/query.md)を参照してください。

SDK の HTTP リクエストとレスポンスは、コマンドの前に `--trace` を指定するとトレースできます。
トレースには認証情報などが含まれる場合があるため、出力の取り扱いに注意してください。

`skr http` は SDK のプロファイルまたは同じ環境変数を使って認証し、指定 URL のホストへリクエストを送ります。
HTTPS URL のみ指定でき、レスポンス本文は加工せずに出力します。認証情報を送るため、信頼できる接続先だけを指定してください。
使い方は[HTTP リクエストのガイド](docs/manual/http-request.md)を参照してください。

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

リリースは tagpr のリリース PR を `main` にマージすると開始します。
GoReleaser によるバイナリ公開とコンテナイメージ公開の構成は、[リリース設計](docs/design/release.md)を参照してください。

## ライセンス

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
