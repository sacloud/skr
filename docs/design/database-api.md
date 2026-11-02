# Database API コマンド設計

## 対象範囲

`iaas-api database` は SDK `github.com/sacloud/sacloud-sdk-go v0.3.1` のデータベースアプライアンスを操作します。データベースは MariaDB と PostgreSQL を提供する IaaS アプライアンスで、スイッチへの接続が必須です。単一のデータベースを対象とし、ルータ＋スイッチへの接続はレプリケーションなどの構成で利用できます。

公開する操作は次のとおりです。

| 操作 | 内容 |
| --- | --- |
| `find` | 一覧検索。`--name` による単一名の部分一致検索、`--tags`、件数・開始位置に対応 |
| `read` | ID で参照 |
| `create` | 作成。作成リクエストは起動の完了を待たずに応答を返す |
| `update` | 更新。送信した内容で設定が置き換わる。JSON のみを受け付ける |
| `delete` | 削除。`--force` と `--fail-if-not-found` に対応 |
| `boot` / `shutdown` / `reset` | 電源操作 |
| `monitor-cpu` / `monitor-disk` / `monitor-interface` / `monitor-database` | モニタリングデータの取得 |
| `list-parameter` | 設定パラメータの一覧取得 |

## パッケージと生成

操作の大半は `api/commands/iaas-database.json` で明示します。コマンドコードは `make generate-iaas-api` で `internal/iaas/databaseapi/database_api_generated.go` に生成します。生成コマンドの例: `make generate-iaas-api API_CONFIG=api/commands/iaas-database.json API_OUTPUT=internal/iaas/databaseapi/database_api_generated.go`。生成するのは `find`、`read`、`delete`、`boot`、`shutdown`、`reset`、`monitor-cpu`、`monitor-disk`、`monitor-interface`、`monitor-database`、`list-parameter` です。API interface、factory、Runtime も生成します。`internal/cli/` の配線は `make generate-wiring` から生成し、設定との一致を `internal/cli/wiring_test.go` で確認します。

`create` と `update` は手書きで `internal/iaas/databaseapi/commands.go` に実装し、api 層の操作 (`iaas.DatabaseOp` の `Create` / `Update`) を直接呼び出します。`create` は作成リクエストにパスワードという秘密情報を含むため、`--request` は保護したファイルまたは標準入力に限定する方針 ([ADR 0070](../adr/0070-service-endpoint-gateway-create-input.md)) に従い、個別フラグと `--request` JSON の両経路を設けるためです。`update` もパスワードを含む JSON を受け付けるため、同じ経路に限定します。

api 層の操作を直接呼び出す理由は次のとおりです。

- `iaas-api` の他の IaaS リソースコマンドと同じく、コマンド層は SDK の service 層に依存しません ([アーキテクチャ](cli-command-architecture.md))。service 層の create / update は builder によるオーケストレーション (起動待ち、設定のマージ) を行いますが、低レベルな `iaas-api` では SDK の api 層のリクエストをそのまま送ります。
- service 層の create / update には、入力をサイレントに置き換える挙動がありました。create はリクエストの `DatabaseType` を検証せずに既定値へ置き換え、update は読み取った現在値をそのまま更新リクエストに組み込みます。現在値のパスワードがマスク済み (`********`)、`DatabaseType` が表示用の値 (`MariaDB` など) で、どちらも API に受け付けられません ([sacloud-sdk-go #320](https://github.com/sacloud/sacloud-sdk-go/issues/320))。api 層の操作は入力をそのまま送るため、この種の置き換えは起こりません。
- service 層の `UpdateRequest.Validate()` は `Backupv2Weekdays` 未指定時に panic する不具合がありました ([sacloud-sdk-go #319](https://github.com/sacloud/sacloud-sdk-go/issues/319))。api 層の操作はこの検証を経由しないため、CLI が入力を検証します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/iaas/databaseapi/commands_test.go` で、CLI 登録とヘルプは `internal/cli/database_test.go` で確認します。

## 入力

### find

- `--zone` は必須です。`--name` は単一名の部分一致検索で、分割せず 1 要素の API `Names` に変換します ([ADR 0049](../adr/0049-internet-workflow-options.md) の拡張範囲)。
- `--tags` はカンマ区切りで API の `Tags` に渡します。`--count` と `--from` は件数と開始位置です。
- 名前配列 `Names`、ソート条件 `Sort` は JSON でのみ指定できます。

### create

- `--request` は `@path.json` 形式または `-` (標準入力) のみを受け付けます。インラインの JSON は秘密情報を含み得るため拒否します。`--request` と個別フラグは併用できません。併用の検出は、Kong が明示的に指定されたときだけ値を設定するポインタ型のフラグで行います。明示的な false・空文字列も指定とみなします (service-endpoint-gateway-api に同じ規約)。
- 個別フラグは次のとおりです。

| フラグ | 内容 |
| --- | --- |
| `--zone` (必須) | 対象ゾーン。`all` は不可 |
| `--name` (必須) | データベース名 |
| `--description` / `--tags` / `--icon-id` | 説明、タグ (カンマ区切り)、アイコン |
| `--plan-id` (必須) | データベースプラン ID。10: 10GB、30: 30GB、90: 90GB、240: 240GB、500: 500GB、1000: 1TB |
| `--switch-id` (必須) | 接続先スイッチ ID |
| `--ip-address` (必須、繰り返し) | 割り当てる IPv4 アドレス。1 〜 2 個 |
| `--network-mask-len` (必須) | ネットワークのプレフィックス長 (1 〜 32) |
| `--default-route` / `--port` / `--source-network` | 既定経路、ポート (1 〜 65535)、接続を許可する送信元ネットワーク |
| `--database-type` (必須) | `mariadb` または `postgres` |
| `--database-version` | バージョン |
| `--username` (必須) / `--password` (必須) | 管理ユーザー。パスワードは `@path` または `-` のみ |
| `--enable-replication` / `--replica-user-password` | 冗長化。レプリカ側パスワードは `@path` または `-` のみ |
| `--enable-web-ui` | Web UI の有効化 |
| `--enable-backup` / `--backup-weekday` / `--backup-start-time-hour` / `--backup-start-time-minute` | バックアップ設定 |
| `--enable-backup-v2` / `--backup-v2-*` | 新バックアップ設定。曜日と接続先が必須 |
| `--enable-monitoring-suite` | Monitoring Suite 連携 |
| `--disk-encryption-algorithm` / `--disk-encryption-kms-key` | ディスク暗号化。両方の指定が必要。アルゴリズムは `aes256_xts` |

- パスワードを引数の値として受け付けません。`--password @/path/to/file` または `--password -` (標準入力) で渡します。末尾の改行はトリムします。
- 冗長化を有効にする場合は `--replica-user-password` が必須で、IP アドレスを 2 個指定します。
- バックアップ (v2) を有効にする場合は曜日と接続先の指定が必須です。
- ディスク暗号化はアルゴリズムと KMS キー ID の両方の指定が必要です。アルゴリズムは API の値 `aes256_xts` のみ受け付けます。
- 曜日は `sun` から `sat` の値を検証してから API の値へ変換します。データベース種別は `mariadb` または `postgres` のみ受け付けます。
- 個別フラグから組み立てるリクエストは api 層の `DatabaseCreateRequest` に対応します。管理ユーザー名は `CommonSetting.DefaultUser`、パスワードは `CommonSetting.UserPassword`、RDBMS 種別は `Conf.DatabaseName` に対応します。レプリカユーザー名とバックアップの保存世代は、API の既定値として `replica` と 8 を設定します。

### update

- `--zone` と `--id` が必須で、更新内容は `--request` JSON (`@path.json` 形式または `-`) のみを受け付けます。
- 送信した内容で設定が置き換わります。api 層の更新リクエストをそのまま送るため、既存の設定を維持したい項目は JSON に再指定してください。読み取った現在の設定 (read の出力) を基に JSON を組み立てるのが確実です。更新リクエストの空の `Name` は送信されませんが、`Description` と `Tags` は常に送信されます。ネストした設定 (`CommonSetting`、`BackupSetting`、`Backupv2Setting`、`ReplicationSetting`、`MonitoringSuite`) は nil のとき省略されます。
- read の応答の `UserPassword` と `ReplicaPassword` はマスク済みの値で返ります。API はマスク済みの値を受け付けないため、設定を再指定する場合はパスワードに実値を指定してください。
- 冗長化を有効にする変更など、一度の変更で複数の操作 (設定変更と反映) を行う場合、変更はデータベースの再起動を伴うことがあります。

### delete / boot / shutdown / reset

- `delete` は `--force` と `--fail-if-not-found` に対応します。`--force` を付けない場合、起動中のデータベースはエラーになり、付けた場合は電源 OFF (強制終了) してから削除します。
- `boot` は `--no-wait`、`shutdown` は `--no-wait` と `--force` (強制終了) に対応します。いずれも省略時は完了を待ちます。

### monitor 系 / list-parameter

- `monitor-cpu`、`monitor-disk`、`monitor-interface`、`monitor-database` は `--zone` と `--id` が必須で、期間を `--start` と `--end` (RFC3339) で指定できます。
- `monitor-database` は未指定時に直近 1 時間の期間を秒精度で自動設定します (`SetMonitorDatabasePeriod`)。データベースのモニタリング API はナノ秒精度の時刻を受け付けないため、SDK 既定の `time.Now()` の値をそのまま送ると 400 エラーになります。`monitor-cpu` / `monitor-disk` / `monitor-interface` は SDK の既定値 (直近の期間) を使います。
- `list-parameter` は設定パラメータの一覧を返します。RDBMS 固有のパラメータの変更は、更新リクエスト (`DatabaseUpdateRequest`) に Parameters が含まれないため、この CLI の update では行えません。コントロールパネルで変更してください。

## 出力

API の応答フィールドは PascalCase です。`find`、`read`、`create`、`update`、`monitor` 系、`list-parameter` の出力は JSON で、`--query` による加工に対応します。`delete`、`boot`、`shutdown`、`reset` は応答本文を出力しません。

## ライブ E2E と後片付け

`test/e2e/database/` はビルド済み CLI を呼び出す testing.T ベースのシナリオテストです ([ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md))。

```console
$ SKR_E2E=1 go test ./test/e2e/database/ -run TestScenarioDatabase -timeout 60m
```

実行には `SKR_E2E` の設定と `skr-e2e` プロファイルが必要です。シナリオは接続先スイッチの作成から、データベース (MariaDB、10GB プラン) の作成、起動待ち合わせ、参照、パラメータ一覧、名前検索、モニタリング、説明の更新、電源操作 (shutdown / boot / reset)、削除と不在確認までを検証します。作成リクエストは起動の完了を待たないため、シナリオは作成後に SDK で起動状態を待ち合わせます。作成するのはスイッチ 1 件とデータベース 1 件のみで、テスト関数内の `defer` で削除と不在確認まで行います。データベースは作成から削除まで課金される可能性があります。

`is1b` と `is1c` はスイッチのゾーン内上限が 1/1 のため、シナリオは上限が緩い `is1c` を使います。プラン ID はゾーンによらず同じ ID を使えるため、シナリオは標準の 10GB プランの ID (10) を使います。データベースの作成・電源操作は完了まで時間がかかるため、コマンドのタイムアウトを長めに設定します。

update は送信した内容で設定が置き換わるため、シナリオは read の出力から現在の設定を含む更新リクエストを組み立てて JSON に渡します。read の応答のパスワードはマスク済みで返り、API はマスク済みの値を受け付けないため、シナリオは作成時に使った値で置き換えます。

パスワードはテスト実行時に生成し、一時ファイル経由で `--password @path` に渡します。実行記録にはファイルパスだけが残り、秘密情報は記録しません。

sakumock は v0.13.0 に database サーバーを含まないため、sakumock を使った CLI と SDK の統合テストはありません。手書き処理の単体テストは `internal/iaas/databaseapi/commands_test.go` で、CLI 登録とヘルプは `internal/cli/database_test.go` で確認します。

## 参考資料

- [データベース](https://manual.sakura.ad.jp/cloud/appliance/database/index.html)
- [MariaDB](https://manual.sakura.ad.jp/cloud/appliance/database/mariadb.html)
- [PostgreSQL](https://manual.sakura.ad.jp/cloud/appliance/database/postgresql.html)
- [データベース / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/service/iaas/database)
