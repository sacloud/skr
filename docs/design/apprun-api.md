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

生成された各リソースの `Commands`、API interface、factory、Runtime は、`internal/apprunapi/commands.go` の `NewCommands` で一括して CLI に注入します。SDK クライアントは共通の認証設定から作成し、ドメイン固有の request decoder と共通出力処理を Runtime に渡します。

## リクエスト入力と秘密情報

- アプリケーションの Create / Update は `CreateApplicationBody` / `PatchApplicationBody` を `--request` で受け付けます。レジストリ認証情報や Secret 値を含む可能性があるため、`@path.json` または `-`（標準入力）に限定し、インライン JSON を拒否します。
- PacketFilter の Update と Traffic の Update は SDK の request model を `--request` で受け付けます。これらは直接 JSON、`@path.json`、標準入力 `-` を利用できます。
- Application / Version の一覧ではページ番号、ページサイズ、並び替えを個別フラグで指定できます。`--request` と個別フラグの同時指定はできません。
- `DecodeRequest` は標準の JSON unmarshal を使います。構造体にない JSON フィールドは無視され、型違い、不正な JSON、末尾の余分な JSON 値はエラーになります。更新リクエストの省略フィールドと明示したゼロ値は SDK の Optional 型で区別します。
- AppRun の出力と API エラーは共通の JSON 出力・エラー伝播経路に接続します。認証情報をレスポンスやエラーに追加する処理は行いません。

## テスト

`internal/apprunapi/` の単体テストは、JSON 入力のファイル・標準入力・インライン形式、読み込みエラー、JSON 型エラー、更新フィールドの省略と明示値を検証します。`internal/cli/apprun_test.go` は help と秘密情報のインライン拒否、sakumock AppRun サーバーを使った CLI 操作、出力に Secret 値を含めないこと、削除後の不在を検証します。

`test/e2e/apprun/` は一意名の Application を1件作成し、一覧・詳細・ステータス・公開 URL の応答を検証してから、その実行で作成した ID と設定が一致する場合だけ削除します。ステータスが `Healthy` になるまで期限付きで待機し、`UnHealthy` は失敗として扱います。実行には `--confirm-apprun-live` が必要です。公開 URL へのリクエストが課金対象のインスタンスを起動する可能性があるため、ライブ環境のプロファイルとプロジェクトを確認してから実行してください。

ライブ E2E で Application の作成・一覧・参照、`Deploying` から `Healthy` への遷移、公開 URL の HTTP 200 と本文 `Hello from AppRun!` を確認済みです。作成した ID と設定を削除直前に再照合して削除し、削除後の空一覧も確認しました。クラウドリソースとローカル一時ファイルの後片付けを含め、runner の終了コードは0でした。

実行証跡は Git 管理外の `tmp/apprun/202610221719/` に保存しています。プロファイル名やリソース ID を含むため、証跡をリポジトリに追加・共有しません。先行した実行では一覧取得が API 403 で停止し、リソースを作成していません。権限変更後の再実行で上記のシナリオが成功しました。

Application の Update、Version、Traffic、PacketFilter、User の操作は今回のライブ E2E の対象外です。これらのコマンドの検証は sakumock によるもので、今回の成功を全操作のライブ検証完了とは扱いません。

## 根拠

- [AppRun 共用型の基本情報](https://manual.sakura.ad.jp/cloud/apprun/about.html)
- [AppRun 共用型クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)
- [AppRun 共用型 API](https://manual.sakura.ad.jp/api/cloud/apprun/)
- [sacloud-sdk-go v0.3.1 AppRun API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/apprun)
