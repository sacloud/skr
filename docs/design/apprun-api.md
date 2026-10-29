# AppRun 共用型 API コマンド設計

- Status: Implemented
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`
- Related ADR: [ADR 0056](../adr/0056-apprun-api-commands.md)

## コマンド構成

`skr apprun-api` は、コンテナイメージからアプリケーションを実行する AppRun 共用型の API 操作を提供します。AppRun Dedicated は別の API として `skr apprun-dedicated-api` に分けます。実装は `internal/apprunapi/`、CLI 登録と SDK クライアントの注入は `internal/cli/` に置きます。

コマンドは SDK v0.3.1 の公開メソッドに対応します。対象は Application、Version、PacketFilter、Traffic、User です。

| リソース | 操作 |
| --- | --- |
| Application | List / Read / Create / Update / Delete / Status |
| Version | List / Read / Delete / Status |
| PacketFilter | Read / Update |
| Traffic | List / Update |
| User | Read / Create |

Version の作成は Application の更新で新しいバージョンが作られるため、独立した Create コマンドはありません。コマンド名、引数、フラグ、ヘルプは `api/commands/apprun-*.json` で定義し、`cmd/apigen-api` から生成します。

Application / Version の一覧で指定する `sort-order` は、SDK の enum 検証を SDK 呼び出し前に実行します。受け付ける値は `asc` と `desc` です。

生成された各リソースの `Commands`、API interface、factory、Runtime に加え、パッケージはリソースごとの constructor (`NewApplicationAPI` など) を公開します。`internal/cli/` の配線は `make generate-wiring` から生成し、各リソースの factory が SDK クライアントを共通の認証設定から作成し、ドメイン固有の request decoder と共通出力処理を Runtime に接続します。

## リクエスト入力と秘密情報

- アプリケーションの Create / Update は `CreateApplicationBody` / `PatchApplicationBody` を `--request` で受け付けます。レジストリ認証情報や Secret 値を含む可能性があるため、`@path.json` または `-`（標準入力）に限定し、インライン JSON を拒否します。
- PacketFilter の Update と Traffic の Update は SDK の request model を `--request` で受け付けます。これらは直接 JSON、`@path.json`、標準入力 `-` を利用できます。
- Application / Version の一覧ではページ番号、ページサイズ、並び替えを個別フラグで指定できます。`--request` と個別フラグの同時指定はできません。
- `DecodeRequest` は標準の JSON unmarshal を使います。構造体にない JSON フィールドは無視され、型違い、不正な JSON、末尾の余分な JSON 値はエラーになります。更新リクエストの省略フィールドと明示したゼロ値は SDK の Optional 型で区別します。
- AppRun の出力と API エラーは共通の JSON 出力・エラー伝播経路に接続します。認証情報をレスポンスやエラーに追加する処理は行いません。

## テスト

`internal/apprunapi/` の単体テストは、JSON 入力のファイル・標準入力・インライン形式、読み込みエラー、JSON 型エラー、更新フィールドの省略と明示値を検証します。`internal/cli/apprun_test.go` は help と秘密情報のインライン拒否、sakumock AppRun サーバーを使った CLI 操作、出力に Secret 値を含めないこと、削除後の不在を検証します。

`test/e2e/apprun/` は testing.T ベースのシナリオテストです。`SKR_E2E` を設定して `go test` を実行すると `skr-e2e` プロファイルでライブ実行し、通常の `go test` では実行環境を変更しません。シナリオは一意名の Application を1件作成し、一覧・詳細・ステータス・公開 URL の応答を検証してから、その実行で作成した ID と設定が一致する場合だけ削除します。ステータスが `Healthy` になるまで期限付きで待機し、`UnHealthy` は失敗として扱います。公開 URL へのリクエストが課金対象のインスタンスを起動する可能性があるため、実行前にライブ環境のプロファイルとプロジェクトを確認してください。

Application の Update、Version、Traffic、PacketFilter、User の各操作も同じシナリオ内で検証します。Update は 1 vCPU / 2Gi の新しいバージョンを作成し、Version の一覧・参照・ステータス・削除と Traffic の一覧・更新を確認します。PacketFilter は有効化と設定を確認してから無効状態に戻します。User はサインアップがアカウント単位で取り消せないため、create の失敗を「サインアップ済み」として許容し、Read で `limit.application_count` を確認します。

ライブ E2E で Application の作成・一覧・参照、`Deploying` から `Healthy` への遷移、公開 URL の HTTP 200 と本文 `Hello from AppRun!` を確認済みです。Update、Version、Traffic、PacketFilter、User の各操作も 2026-10-27 に testing.T ベースのシナリオとして全工程を実行しました。作成した ID と設定を削除直前に再照合して削除し、削除後の一覧に対象 ID がないことも確認しました。

実行記録は Git 管理外の `test/e2e/apprun/e2e-output/` 配下に Markdown で保存します。リソース ID や応答を含むため、記録をリポジトリに追加・共有しません。一覧照合は全ページ取得で行い、途中ページの取得失敗は不在として扱いません。公開 URL の確認は期限付きの再試行で行い、各試行の結果はテストログに出力します。

## 根拠

- [AppRun 共用型の基本情報](https://manual.sakura.ad.jp/cloud/apprun/about.html)
- [AppRun 共用型クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)
- [AppRun 共用型 API](https://manual.sakura.ad.jp/api/cloud/apprun/)
- [sacloud-sdk-go v0.3.1 AppRun API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/apprun)
