# AppRun Dedicated API コマンド設計

- Status: API commands implemented; live E2E and tutorial tracked in [#59](https://github.com/sacloud/skr/issues/59)
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

is1b のライブ E2E と利用者向けチュートリアルは未実装です。課金を伴うライブ検証の入力、実行、安全なクリーンアップを整え、確認済みの結果に基づいてチュートリアルを作成する作業を [#59](https://github.com/sacloud/skr/issues/59) で管理します。

## 根拠

- [AppRun 専有型の概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)
- [クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)
- [技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)
- [SDK v0.3.0 AppRun Dedicated API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0/api/apprun-dedicated)
