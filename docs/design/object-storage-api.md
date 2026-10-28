# Object Storage API コマンド設計

## 対象範囲

`object-storage-api` は `sacloud-sdk-go v0.3.1` の `api/object-storage` が公開する管理 API を利用します。対象は次の操作です。

| SDK interface | CLI 操作 |
| --- | --- |
| `SiteAPI` | サイトの List / Read、サイト別プランの List |
| `AccountAPI` | サイトアカウントの Create / Read / Delete、アクセスキーの List / Create / Read / Delete |
| `BucketAPI` | バケットの List / Create / Delete |
| `BucketExtraAPI` | 暗号化・レプリケーションの Read / Enable / Disable、ペナルティ・利用量・クォータの Read |
| `PermissionsAPI` | パーミッションの List / Create / Read / Update / Delete、アクセスキーの List / Create / Read / Delete |
| `SiteStatusAPI` | サイト状態・クォータ・期間指定バケット利用量の Read |

オブジェクトストレージ管理 API と Amazon S3 互換 API は別の API です。S3 互換 API によるオブジェクトの一覧、アップロード、取得、削除はこのコマンドでは扱いません。

## クライアントと認証

`internal/objectstorageapi.NewAPI` は `internal/saclient.New` から SDK クライアントを作成し、公開 `object-storage` パッケージの `FedClient` とサイト ID ごとの `SiteClient` を使います。SDK の既定エンドポイント設定を尊重し、HTTP 通信を独自に実装しません。

サイト一覧はサイト非依存の API を呼び出します。サイトを対象にするその他の操作では、サイト ID を引数から取得して `SiteClient` を作成します。バケット固有の暗号化・レプリケーション・利用量操作は、サイトクライアントと対象バケット名から SDK の操作 interface を作成します。

バケット削除とレプリケーションの参照・有効化・無効化は、SDK 内では `FedClient` を使うサイト非依存の操作です。CLI は実行直前に、指定サイトのバケット一覧で対象名が一意に存在することを確認します。不一致・空一覧・所属照会失敗では後続の操作を拒否し、指定サイト外へ作用することを防ぎます。

## 入力と出力

- サイト ID、バケット名、権限 ID、アクセスキー ID は対象を指定する位置引数です。
- パーミッションの Create / Update は SDK の `PermissionBucketControlsBody` JSON を `--request` で受け取ります。`display_name` と `bucket_controls` は単純な独立値ではなく、バケットごとの権限を含む構造化入力です。直接 JSON と `@path.json` に対応します。
- `bucket create` の `--plan` は SDK がアーカイブ用サイトのプラン経路を組み立てる補助入力です。`site plans` の `service_class_path` を参照して値を指定します。通常サイトでは指定を拒否します。
- `bucket-metering` の期間は RFC3339 形式の `--from` と `--to` で指定し、SDK に `time.Time` として渡します。
- アクセスキーのシークレットは Create の SDK 応答にだけ含まれ、標準 JSON 出力へ表示します。作成応答をログに残したり、Git 管理下へ保存したりしないよう注意をヘルプに記載します。
- API がエラーを返した場合は出力せず、エラーを呼び出し元へ返します。戻り値を持たない変更操作は成功時も JSON を出力しません。
- 全コマンドで出力式を API factory の呼び出し前に検証します。不正な `--query` でキー発行や設定変更が先に実行されることを防ぎます。

## 副作用と検証

バケット作成は料金が発生し、削除はバケット内のオブジェクトとフォルダが空の場合だけ実行できます。SDK の操作 interface を差し替える単体テストで主要なリクエスト構築、JSON デコード、エラー伝播、対象バケットの選択を確認します。

`internal/cli/objectstorage_test.go` では、利用中の sakumock v0.13.0 に含まれる `objectstorage.NewTestServer` を起動し、実際の CLI → 公開 SDK → HTTP サーバーの経路で全34末端コマンドを検証します。サイトごとのアカウント、通常・アーカイブバケット、権限更新、両種類のアクセスキー、暗号化、レプリケーション、テレメトリーと削除後の不在が対象です。コマンドツリーとの照合によって網羅漏れを検出し、403のエラー伝播とモックの OpenAPI 応答検証結果も確認します。

このテストはモックの制御プレーンの検証です。メータリングはモックに利用履歴がなく空配列を返すため、実サービスの月次レコード検証とは区別します。KMS キーの実在性や実サービスでの暗号化を保証せず、ライブ暗号化の延期やテレメトリーの失敗を成功扱いにしません。ドメインの単体テストも維持します。

## ライブ E2E

[ADR 0058](../adr/0058-live-e2e-command-coverage.md) と [ADR 0062](../adr/0062-e2e-testing-t-scenarios.md) に従い、チュートリアルへの掲載有無にかかわらず、全34末端コマンドを `test/e2e/object-storage-api/` の testing.T ベースの6シナリオに割り当てます。全末端コマンドの網羅状況は [ADR 0065](../adr/0065-e2e-command-coverage-baseline.md) の baseline (`test/e2e/command-coverage/baseline.txt`) で追跡し、実行記録から抽出した末端コマンドが baseline と一致することを検証します。この検査は実サービスの正常動作を保証するものではなく、ライブ実行結果は別に管理します。

### シナリオと副作用

| シナリオ | 対象と成功条件 | 作成・削除するリソース |
| --- | --- | --- |
| `TestScenarioObjectStorageBucket` | 石狩第1サイトでサイト確認、通常バケット作成・一覧、パーミッションの作成・一覧・参照・権限変更、アクセキーの作成・一覧・参照・失効、削除後の不在を確認します | 通常バケット1個、パーミッション1個、パーミッションのキー1個を扱います |
| `TestScenarioObjectStorageAccount` | 未作成サイトでアカウント ID、プラン一覧、アカウントキーの全操作と削除後の不在を確認します | サイトアカウント1個とアカウントキー1個を扱います |
| `TestScenarioObjectStorageArchive` | プラン一覧と作成結果のプランを照合し、`Bucket` シナリオと同じ権限・キー操作を実行します | アーカイブバケット1個、パーミッション1個とキー1個を扱います |
| `TestScenarioObjectStorageMetrics` | サイトの状態・クォータ、バケットの利用量・クォータを照合します | 一時通常バケット1個、パーミッション1個とキー1個を扱います |
| `TestScenarioObjectStorageEncryption` | 東京第1サイトで既存 KMS キーを指定し、暗号化の有効化・キー ID の照合・無効化を確認します | 一時通常バケットとその権限・キーを扱います。KMS キー自体は変更・削除しません |
| `TestScenarioObjectStorageReplication` | 異なる通常サイトの2バケット間で設定し、送信元・宛先の名前とサイト、`created` 状態、無効化後の設定不在を確認します | 通常バケット2個と各バケットのパーミッション・キーを扱います |

バケットを使う各シナリオは基本の権限・キー操作も実行します。サイトアカウントが未作成である場合は `Account` と `Replication` のシナリオでアカウントを作成し、後片付けで削除します。既存アカウントの変更・削除や再作成は拒否します。`Encryption` は KMS キー作成が未実装のため ([#141](https://github.com/sacloud/skr/issues/141))、既存キー ID を環境変数 `SKR_E2E_OBJECT_STORAGE_KMS_KEY_ID` で指定した場合だけ実行します。

通常バケットとアーカイブバケットの作成には料金が発生し、レプリケーションは通常バケット2個を作成します。実行前に選択中のプロファイル、対象サイト、作成するリソースと料金を確認します。今回の実行では、利用者から料金を理由に範囲を制限しない承認を得ています。料金とサービス条件は[基本情報](https://manual.sakura.ad.jp/cloud/objectstorage/about.html)、[レプリケーション](https://manual.sakura.ad.jp/cloud/objectstorage/crr.html)、[暗号化](https://manual.sakura.ad.jp/cloud/objectstorage/encryption.html)を参照します。

### 再実行

SDK の認証プロファイルと Object Storage 管理権限を用意し、`make build` で CLI をビルドします。`SKR_E2E=1` を設定して以下のテストを実行します。未設定の場合はテストをスキップします。サイトアカウントが未作成の場合は各シナリオが作成し、後片付けで削除します。

```sh
# 石狩第1サイトで通常バケット、パーミッション、アクセキーの一連の操作を検証します。
SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageBucket -v

# 未作成のアーカイブサイトでアカウントとアカウントキーの一連の操作を検証します。
SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageAccount -v

# アーカイブプランのバケットを作成し、プラン設定を検証します。
SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageArchive -v

# サイトの状態・クォータとバケットの利用量・クォータを検証します。
SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageMetrics -v

# 異なる通常サイトの2バケット間でレプリケーションを検証します。
SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageReplication -v

# KEY-ID に既存 KMS キーの識別子を指定して暗号化を検証します。
SKR_E2E_OBJECT_STORAGE_KMS_KEY_ID=KEY-ID \
  SKR_E2E=1 go test ./test/e2e/object-storage-api/ -run TestScenarioObjectStorageEncryption -v
```

すべてのシナリオをまとめて実行する場合は `-run TestScenarioObjectStorage` を使います。

### コマンド対応とライブ実行状況

2026-10-28 の実行結果です。全34コマンドにシナリオがあり、このうち29コマンドで正常応答と期待内容をライブ確認しました。暗号化3操作は合意により延期し、残る2操作は API エラーのため成功未確認です。

| `object-storage-api` 以下のコマンド | 個数 | 主なシナリオ | ライブ結果 |
| --- | --- | --- | --- |
| `site list / read / plans` | 3 | `Bucket`、`Archive`、`Account` | 成功しました |
| `site status read / quota` | 2 | `Metrics` | 東京第1サイトで成功しました |
| `site status bucket-metering` | 1 | deferred | HTTP 400 が返り、成功未確認です。#142 で追跡します |
| `account create / read / delete` | 3 | `Account` | アーカイブ・東京第1サイトで成功しました |
| `account access-key list / create / read / delete` | 4 | `Account` | 成功しました |
| `bucket list / create / delete` | 3 | `Bucket`、`Archive`、`Replication` | 通常・アーカイブ両プランで成功しました |
| `permission list / create / read / update / delete` | 5 | `Bucket` | 書き込み許可の変更を含め成功しました |
| `permission access-key list / create / read / delete` | 4 | `Bucket` | 成功しました |
| `bucket usage / quota` | 2 | `Metrics` | 東京第1サイトで成功しました |
| `bucket penalty` | 1 | deferred | HTTP 404 が返り、成功未確認です。#142 で追跡します |
| `bucket encryption read / enable / disable` | 3 | `Encryption` | KMS キー作成が未実装のため、#141 で追跡します |
| `bucket replication read / enable / disable` | 3 | `Replication` | 石狩第1サイトから東京第1サイトへの設定で成功しました |

暗号化は [#141](https://github.com/sacloud/skr/issues/141)、テレメトリーの失敗は [#142](https://github.com/sacloud/skr/issues/142) で追跡します。`penalty` の404をペナルティなしと解釈したり、`bucket-metering` の400を空利用量で置き換えたりしません。CLI は公開 SDK にサイト、バケット名、期間を渡しており、失敗の原因が CLI・SDK・サービスのどこにあるかは未確定です。両操作は baseline で deferred として扱い、正常系のシナリオでは呼び出しません。

### 安全性と証跡

バケットは作成結果のサイト・名前と一覧の `resource_id` を保持し、削除直前に再照合します。一覧の `plan` が `null` の場合は作成応答のプランを照合します。権限は数値 ID・表示名・バケット制御を確認し、許可が増えた別の権限は削除しません。アカウントは作成 ID が不変で、バケット・権限・キーがすべて空の場合だけ削除します。403、通信失敗、不明な404、`null` や非配列の一覧を不在とは扱いません。

アクセスキーの作成応答にはシークレットが含まれるため、シナリオは実行記録から標準出力を除いて取得します。キーのシークレットは実行記録へ残さず、作成したキーの ID を参照応答と照合し、参照応答にシークレットが含まれないことを確認します。パーミッションとアカウントの JSON リクエストは `--request @path` の一時ファイルで渡し、後片付けで削除します。

レプリケーションは設定を無効化してから両バケットを削除します。失敗・キャンセル時も独立した期限付きコンテキストで後片付けし、削除失敗を最終終了コードへ反映します。所有を確認できない対象は自動削除を拒否します。

暗号化も無効化直前に設定を再取得し、現在の `kms_key_id` が今回指定したキー ID と一致する場合だけ無効化します。明確な404は設定不在として変更せず終了し、不一致・不正応答・キー ID 欠落・照会失敗は変更を拒否します。キャンセル後の片付けにも同じ条件を適用します。設定の所有を確認できない場合は親バケットも自動削除せず、手動確認を求めるエラーを返します。

証跡は共有ヘルパーが Git 管理外の `test/e2e/object-storage-api/e2e-output/<識別名>-<実行日時>.md` に Markdown で保存します。2026-10-28 の実行では `object-storage-bucket`、`object-storage-account`、`object-storage-archive`、`object-storage-metrics`、`object-storage-replication` の記録を残しました。作成したバケット・権限・キー・サイトアカウントの削除と不在確認は完了しています。
