# AppRun Dedicated API コマンド設計

- Status: Implemented
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.0`
- Related ADR: [ADR 0027](../adr/0027-apprun-dedicated-api-commands.md)

## コマンド構成

`skr apprun-dedicated-api` は AppRun 専有型の SDK 公開 API を直接呼び出す低レベルコマンドです。コマンド本体と SDK client の生成は `internal/apprundedicatedapi/`、CLI の登録と Runtime 接続は `internal/cli/` に置きます。認証は SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` を使います。

公開コマンドは `cluster`、`application`、`version`、`auto-scaling-group`、`worker-node`、`load-balancer`、`certificate`、`service-class` です。各操作は SDK の公開オペレーションに対応し、階層リソースの ID は名前付きフラグまたは位置引数で指定します。ページング結果は `items` と `nextCursor` を含む JSON 形状で出力します。

## 入力と安全な扱い

- 複雑な作成・更新リクエストは SDK の request model を `--request` で受け付けます。入力は直接 JSON、`@path.json`、標準入力 `-` です。
- 証明書、アプリケーションバージョンの入力は秘密情報を含み得るため、`@path.json` または標準入力のみ受け付けます。リクエスト内容をコマンドライン引数へ露出しません。
- Application の作成は `--name` と `--cluster-id` を受け付けます。バージョン有効化と解除は `--active-version` または `--deactivate` のどちらか一方を指定します。
- ワーカノードの draining 更新は `--draining true|false` を指定します。
- API エラーは Kong に返し、成功したような出力に変換しません。出力形式は共通の JSON、YAML、table と jq 処理を使います。

## テスト

`internal/cli/apprundedicated_test.go` は sakumock の AppRun Dedicated server に対して、クラスタ、オートスケーリンググループ、ワーカノード、ロードバランサ、証明書、アプリケーション、バージョンの実際の CLI パスを実行します。ヘルプ、ID の受け渡し、入力ファイル、更新と削除を検証します。

ライブ E2E は `test/e2e/apprun-dedicated-api` にあります。利用者が指定する cluster、auto-scaling-group、version JSON を受け取り、固有名と `is1b` を runner 側で設定します。安全性のためワーカノード数とアプリケーション複製数は各 1 に固定します。実行確認には `--confirm-is1b-live` が必要です。runner は選択中プロファイルを確認し、名前衝突があれば中止します。クラスタ、グループ、アプリケーションの作成結果を ID と内容で読み戻し、バージョンを有効化してアプリケーション配置を確認します。今回作成したバージョン、アプリケーション、グループ、クラスタをこの順に削除し、削除後に各一覧から消えたことを検証します。

実機実行には専用ワーカノード作成を伴い料金が発生します。ローカルの対象プロファイル、プロジェクト、サービスプリンシパル、リソース設定とイメージを確認し、承認を得てから実行します。実行前に `make build` と `./skr config current` を確認し、private evidence は `tmp/apprun-dedicated-api/` に保存します。証跡はアカウント固有の情報を含む可能性があるため共有・コミットしません。

```console
$ go test ./test/e2e/apprun-dedicated-api
$ make build
$ ./skr config current
$ go run ./test/e2e/apprun-dedicated-api --skr ./skr --cluster-request /secure/cluster.json --auto-scaling-group-request /secure/group.json --version-request /secure/version.json --confirm-is1b-live
```

## 根拠

- [AppRun 専有型の概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)
- [クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)
- [技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)
- [SDK v0.3.0 AppRun Dedicated API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0/api/apprun-dedicated)
