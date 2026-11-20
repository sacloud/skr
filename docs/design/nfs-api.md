# NFS API コマンド設計

## 対象範囲

`nfs-api` は SDK `github.com/sacloud/sacloud-sdk-go v0.7.0` の NFS アプライアンスを操作します。NFS はスイッチ配下のローカルネットワークに設置するファイルサーバアプライアンスで、プランは標準 (HDD) と SSD のディスクプランから選びます。プラン ID は共通仕様ノート `sys-nfs` のプラン一覧から解決します。接続先スイッチ、プラン、IP アドレス、ネットマスク長は作成後の変更ができません。

公開する操作は次のとおりです。

| 操作 | 内容 |
| --- | --- |
| `find` | 一覧検索。`--name` による単一名の部分一致検索、`--tags`、件数・開始位置に対応 |
| `read` | ID で参照 |
| `create` | 作成。作成リクエストは起動の完了を待たずに応答を返す |
| `update` | 更新。未指定項目は空の値として送信する |
| `delete` | 削除。電源状態を考慮したワークフロー。`--force` と `--fail-if-not-found` に対応 |
| `boot` / `shutdown` / `reset` | 電源操作。boot と shutdown は `--no-wait` に対応 |
| `monitor-cpu` / `monitor-free-disk-size` / `monitor-interface` | モニタリングデータの取得 |

## パッケージと生成

操作の大半は `api/commands/nfs.json` で明示します。コマンドコードは `make generate-iaas-api` で `internal/iaas/nfsapi/nfs_api_generated.go` に生成します。生成するのは `find`、`read`、`delete`、`boot`、`shutdown`、`reset`、`monitor-cpu`、`monitor-free-disk-size`、`monitor-interface` です。API interface、factory、Runtime も生成します。`internal/cli/` の配線は `make generate-wiring` から生成し (`internal/cli/nfs_wiring_generated.go`)、設定との一致を `internal/cli/wiring_test.go` で確認します。

`create` と `update` は手書きで `internal/iaas/nfsapi/commands.go` と `requests.go` に実装し、api 層の操作 (`iaas.NFSOp` の `Create` / `Update`) を直接呼び出します。

- create は SDK の作成リクエスト型にゾーンフィールドがなく、CLI がゾーンを別途管理するため、手書きの `CreateRequest` (Zone を含む) から `iaas.NFSCreateRequest` へ変換します。変換時に Class (`nfs`) を補完します。
- update は SDK の更新リクエストが未指定項目を空の値として送る挙動のため、個別フラグはポインタ型で未指定を区別します。

`delete` は生成コマンドですが、`TransformDeleteRequest` で手書きのワークフロー (電源状態を考慮した停止と削除) を呼び出します。

生成コードを直接編集せず、設定を変更して再生成します。手書き処理と SDK 呼び出しは `internal/iaas/nfsapi/commands_test.go` と `sdk_api_test.go` で確認します。CLI 登録とヘルプは `internal/cli/nfs_test.go` で確認します (高レベル `skr nfs` の service 層の検証は `internal/nfs/` のテストで行います)。

## 入力

### find

- `--zone` は必須です。`--name` は単一名の部分一致検索で、分割せず 1 要素の API `Names` に変換します ([ADR 0049](../adr/0049-internet-workflow-options.md) の拡張範囲)。
- `--tags` はカンマ区切りで複数要素の API `Tags` に渡します ([ADR 0035](../adr/0035-iaas-tags-flag.md))。`--count` と `--from` は件数と開始位置です。
- 名前配列 `Names`、ソート条件 `Sort` は JSON でのみ指定できます。

### create

- `--request` は直接指定または `@path.json` 形式を受け付けます。NFS の作成リクエストに秘密情報は含まれないため、インラインの JSON も許可します。`--request` と個別フラグは併用できません。
- 個別フラグは次のとおりです。

| フラグ | 内容 |
| --- | --- |
| `--zone` (必須) | 対象ゾーン。`all` は不可 |
| `--name` (必須) | NFS の名前 |
| `--description` / `--tags` / `--icon-id` | 説明、タグ (カンマ区切り)、アイコン |
| `--plan-id` (必須) | NFS プラン ID。Note (sys-nfs) のプラン一覧にある、プラン種別 (標準/HDD、SSD) とサイズに対応する ID |
| `--switch-id` (必須) | 接続先スイッチ ID。作成後の変更はできません |
| `--ip-address` (必須、繰り返し) | 割り当てる IPv4 アドレス。1〜2 個。作成後の変更はできません |
| `--network-mask-len` (必須) | ネットワークのプレフィックス長 |
| `--default-route` | 既定経路の IP アドレス |

- `--ip-address` は IPv4 アドレスだけを受け付け、3 個以上の指定と IPv6 アドレス、不正な形式を拒否します。
- 作成応答は起動の完了を待ちません。起動後の操作の前に、利用者側で起動状態を確認してください。

### update

- `--zone` と `--id` が必須で、更新内容は個別フラグ (`--name`、`--description`、`--tags`、`--icon-id`) または `--request` JSON で指定します。両経路は併用できません。
- SDK の更新リクエストは未指定項目を空の値として送ります。既存の値を維持する項目は、変更しない項目も含めてあわせて指定してください。
- プランとスイッチは更新リクエストに含まれません。

### delete

- `--zone` と `--id` が必須です。
- 電源が ON の場合、`--force` が無ければエラーとします。`--force` を指定すると、強制終了と停止待ちのうえ削除します。
- 元の状態が不明 (Unknown) の場合は状態確認をせずに削除します。
- `--fail-if-not-found` を指定すると、対象が存在しない場合はエラーとします。省略時は、対象が存在しない場合も削除済みとして成功します。

### boot / shutdown / reset

- `boot` は `--no-wait` に対応します。省略時は起動の完了を待ちます。
- `shutdown` は `--no-wait` と `--force-shutdown` に対応します。省略時は停止の完了を待ちます。
- `reset` はリクエストのパラメータを持ちません。

### monitor-*

- `--zone` と `--id` が必須です。`--start` と `--end` は取得期間で、`--end` を省略すると現在時刻、`--start` を省略すると `--end` の 1 時間前になります。

## 高レベルコマンド (`skr nfs plans`)

プラン ID の解決は高レベルコマンド `skr nfs plans` ([ADR 0096](../adr/0096-nfs-high-level-plans.md)) が担います。service 層と CLI コマンドを `internal/nfs/` に手書きで置き、service 層は Note (`sys-nfs`) の検索と Content の解析を担います ([アーキテクチャ](cli-command-architecture.md) の高レベル層)。

- 検索条件は Name `sys-nfs`、Class `json`、件数 100 です。結果からタグ `@zone=<ゾーン>` が一致する Note を取り出します。0 件、複数件、Note が利用可能な状態でない場合はエラーにします。
- Content の `plans` オブジェクトから既知のストレージ種別 (SSD、HDD) の配列を読み、各要素の `size`、`availability`、`planId`、`cloneable` を出力へ変換します。出力は SSD、HDD の順、同じ種別内はサイズ昇順です。プラン ID は数値として復元します (10 桁のプラン ID が float の丸めで壊れないよう、構造体にデコードします)。
- 出力フィールドは `Storage`、`Size`、`PlanID` (types.ID)、`Availability`、`Cloneable` です。省略時は `Availability` が `available` のプランのみを出力し、`--all` で `disabled` も含めます。利用不可のプランが Note の配列に含まれるため、デフォルトの絞り込みが最小サイズ選択の手順を短くします。
- service 層の Note 操作は `iaas.NoteAPI` を interface で受け、単体テスト (`internal/nfs/plans_test.go`) で検索条件、ゾーン照合、Content 解析、並べ替え、フィルタ、異常系を固定します。
- チュートリアルのプラン解決手順は `curl` と `jq` による生 API から `skr nfs plans` へ置き換わりました ([ADR 0095](../adr/0095-nfs-api-commands.md) 時点の課題の解消)。

## E2E

実行には `SKR_E2E` の設定と `skr-e2e` プロファイルが必要です。シナリオは接続先スイッチの作成から、NFS (Note から解決した SSD プランの最小サイズ) の作成、起動待ち合わせ、参照、名前・タグ検索、モニタリング、更新を検証します。電源操作 (shutdown / boot / reset) と削除後の不在確認までを (`test/e2e/nfs/scenario_nfs_test.go`) で検証します。
作成リクエストは起動の完了を待たないため、シナリオは作成後に SDK で起動状態を待ち合わせます。プラン解決は安全確認として SDK 直呼びで行い ([ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md))、解決したプラン ID が `skr nfs plans` の出力に含まれることを照合します。`--all` の出力はデフォルト出力のプランをすべて含むことも確認します。作成するのはスイッチ 1 件と NFS 1 件のみです。テスト関数内の `defer` で ID と識別情報を再照合してから削除し、不在確認まで行います。NFS は作成から削除まで課金されます。

2026-11-20 に tk1a で全ステップをライブ検証しました (プランは Note から解決した SSD 20GB)。ライブ検証で確認した API の挙動は次のとおりです。

- 空きディスク容量のモニタリング API はデータベースと共通の route を使う日単位の API です。`--start` / `--end` に正時 (00:00:00) でない時刻を指定すると API エラー (400 `time is incorrect`) になります。ヘルプに正時指定を案内し、シナリオでは正時の期間を指定します。作成日のデータは日が変わるまで確定しないため、新規作成直後の取得結果は空配列になります。
- update で送信したタグの順序は API 側で並べ替えられて返ります。シナリオはタグの順序を要素の一致 (ElementsMatch) で照合します。
- update の反映は非同期で、直後の参照が旧値を返すことがあります。シナリオは説明が変わるまで参照を再試行します。

## sakumock との差分

sakumock の IaaS モックは NFS を含みません (`docs/design/sakumockiaas-gaps.md` を参照)。そのため、チュートリアルの CLI 操作ハーネステストは sakumock の NFS 対応まで延期します。単体テストは fake API で、ライブ検証は E2E シナリオで行います。
