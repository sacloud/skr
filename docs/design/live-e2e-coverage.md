# ライブ E2E のコマンド網羅状況

- 調査日: 2026-10-23
- 根拠: [ADR 0058](../adr/0058-live-e2e-command-coverage.md)

## 調査方法

[ADR 0058](../adr/0058-live-e2e-command-coverage.md) で公開するすべての末端サブコマンドにライブ E2E が必須とされたため、`skr --help` から抽出した末端コマンド一覧と `test/e2e/` 配下の runner 実装を突合しました。runner が実行する CLI 引数を精査し、末端コマンドごとに runner の対象範囲を判定しています。ライブ実行の成否は `PLAN.md` と各設計文書の記録に従い、この文書では runner が対象とするコマンドの範囲を基準にします。全末端コマンドは311個で、タスク1の完了後は未検証が118個です。

## 網羅済みの runner

| runner | 対象末端コマンド | 状況 |
| --- | --- | --- |
| `test/e2e/c0nfig` | config の current / list / show / create / edit / use(6) | 全コマンド実装済み |
| `test/e2e/http` | http(1) | 全コマンド実装済み |
| `test/e2e/simplemq` | queue 8操作、message 4操作(12) | 全コマンド実装済み。2026-10-27 に testing.T ベースのシナリオテストへ移行し、ライブで正常応答を確認済み |
| `test/e2e/simple-notification` | destination 6、group 6、history 2、routing 7(21) | 全コマンド実装済み。2026-10-27 に testing.T ベースのシナリオテストへ移行し、ライブで正常応答を確認済み |
| `test/e2e/container-registry-api` | registry 5操作、user 4操作(9) | 全コマンド実装済み |
| `test/e2e/networking-suite` | subnet-group 5、subnet 5、interface-connection 2、address 2(14) | 全コマンド実装済み |
| `test/e2e/gslb` | find / read / create / update / delete(5) | 全コマンド実装済み |
| `test/e2e/icon`、`test/e2e/packetfilter` | 各 CRUD(5) | 全コマンド実装済み |
| `test/e2e/disk`、`test/e2e/switch` | 各 CRUD(5) | 全コマンド実装済み。2026-10-27 に testing.T ベースのシナリオテストへ移行し、ライブで正常応答を確認済み |
| `test/e2e/region` | find / read(2) | 全コマンド実装済み |
| `test/e2e/serverplan` | find / read(2) | 全コマンド実装済み。2026-10-27 に testing.T ベースのシナリオテストへ移行し、ライブで正常応答を確認済み |
| `test/e2e/serviceclass` | find(1) | 全コマンド実装済み。2026-10-27 に testing.T ベースのシナリオテストへ移行し、ライブで正常応答を確認済み |
| `test/e2e/zone` | find / find --query / read(3) | 全コマンド実装済み。ライブで正常応答を確認済み |
| `test/e2e/version` | version(1) | 全コマンド実装済み。ライブで正常応答を確認済み |
| `test/e2e/iaas-lookup` | archive find / read、product disk find / read(4) | 全コマンド実装済み |
| `test/e2e/iaas-product-plans` | product internet find / read、product license find、product privatehost find / read(5) | 全コマンド実装済み |
| `test/e2e/object-storage-api` | 6シナリオで34コマンド | runner 実装済み。ライブは29コマンドの正常応答を確認済みで、`bucket penalty` と `site status bucket-metering` は API エラー、暗号化3操作は KMS キー作成未実装のため延期。状況は `PLAN.md` を参照 |
| `test/e2e/iam-api` | 8シナリオで59コマンド(読み取り15、リソーススコープ29、アカウント系15) | runner 実装済み。ライブは57コマンドの正常実行を確認済みで、sso link/unlink の2操作は [#144](https://github.com/sacloud/skr/issues/144) で管理 |

`config` は CLI のメタコマンドとして上表に含めます。`version` は `test/e2e/version` の testing.T ベースのシナリオテストで検証します。

## 未検証コマンド

タスク3の完了後、未検証は35コマンドです。

| サービス | 未検証コマンド | 件数 |
| --- | --- | --- |
| apprun-dedicated-api | 次節の35コマンド | 35 |

### apprun-dedicated-api の未検証35コマンド

cluster 5操作、application 6操作、version 4操作、auto-scaling-group 4操作、worker-node 3操作です。load-balancer 6操作(load-balancer node 2操作を含む)、certificate 5操作、service-class 2操作です。

## タスク

未検証コマンドを次の段階で整備します。各タスクの実装前に、シナリオ、成功条件、作成・変更・削除するリソースと費用の範囲を提示して承認を得ます。

### タスク1: 追加リソースなし・費用なしの拡張(完了)

- `server update`(1コマンド): 完了。`test/e2e/server/` の E2E に作成済みサーバーの名前と説明を更新して応答で反映を確認し、元に戻すフローとして組み込みました。restore に失敗した場合、削除フローは名前の不一致を検出して削除を拒否します。2026-10-23 にライブで正常実行を確認し、2026-10-27 に [ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) に従う testing.T ベースのシナリオテストへ移行しました。
- `internet update` と `internet monitor-router`(2コマンド): 完了。`test/e2e/internet/` の既存 runner に `scenario_update.go` と `scenario_monitor_router.go` として追加します。作成済みルータ＋スイッチの名前・説明の更新と復元を Read で確認し、直近 10 分間の `monitor-router` を `--request` で実行します。トラフィックのない新規ルータに対する空配列は整形式の応答として扱います。2026-10-23 にライブで正常実行を確認しました。
- `version`(1コマンド): 完了。`skr version` が `v<semver> <os>/<arch>, build <revision>` 形式で出力して正常終了することを確認します。2026-10-23 にライブで正常実行を確認しました。2026-10-27 に [ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) に従う testing.T ベースのシナリオテスト (`test/e2e/version/scenario_version_test.go`) へ移行し、同日に移行後のシナリオをライブ再実行しました。

### タスク2: 同一実行リソース上の拡張(完了)

- `internet` の add-subnet / update-subnet / delete-subnet(3コマンド): 完了。`test/e2e/internet/` の既存 runner に `scenario_subnet.go` として追加し、作成済みルータ＋スイッチに追加スイッチを作成してサブネットを追加・NextHop 更新・削除を Read で確認します。2026-10-23 にライブで正常実行を確認しました。
- `internet` の enable-ipv6 / read-ipv6 / disable-ipv6(3コマンド): 完了。`scenario_ipv6.go` として同じ runner に追加し、同一ルータ＋スイッチで IPv6 の有効化・参照・無効化を Read で確認します。2026-10-23 にライブで正常実行を確認しました。
- `eventbus-api` の schedule 5コマンドと process-configuration update、trigger update(7コマンド): 完了。2026-10-23 にライブで正常実行を確認しました。2026-10-27 に `test/e2e/eventbus-api/` を [ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行し、同じシナリオ内で再実行しました。schedule は現在時刻の1年後に1日周期で発火する設定を作成し、テスト時間内に発火しません。process-configuration の update はメッセージ内容を更新し、その内容を SimpleMQ 待ち受けが受信することでライブ更新を証明します。trigger の update は説明のみを更新して復元します。
- `apprun` の application update と version の list / read / status / delete(5コマンド): 完了。`test/e2e/apprun/` に application update で 1 vCPU / 2Gi の新バージョンを作成して Healthy 待ち、traffic を最新バージョンに100%割り当ててから前バージョンを削除するシナリオを追加します。2026-10-23 にライブで正常実行を確認しました。
- application update は新しいバージョンを作るため、version 操作と組み合わせて作成・参照・削除まで確認します。
- `apprun` の traffic と packet-filter、user の create / read(6コマンド): 完了。application のライフサイクルと同じシナリオに追加します。packet-filter は有効化＋設定を確認してから無効化に戻します。2026-10-23 にライブで正常実行を確認しました。
- `apprun user create`(1コマンド): 完了。サインアップはアカウント単位で取り消せないため、create の失敗を「サインアップ済み」として許容し、Read で `limit.application_count` を確認します。2026-10-23 にライブで正常実行を確認しました。
- vCPU とメモリの組み合わせは API 側で検証されます(0.5/1Gi、1/2Gi、2/4Gi)。update では 1 vCPU / 2Gi を使用します。

### タスク3: iam-api の段階的整備(完了)

- 読み取り系(15): 完了。`scenario_read_only.go` で一括参照し、2026-10-26 にライブで正常応答を確認しました。
- リソーススコープの書き込み(29): 完了。6つのシナリオで、runner が作成する一時リソースの作成・変更・削除と後片付けを1シナリオで確認し、2026-10-26 にライブで正常実行を確認しました。sso link/unlink は全組織ユーザーのログイン方法に影響するため、[#144](https://github.com/sacloud/skr/issues/144) で対応します。
- アカウント全体やログインユーザーへ影響する書き込み(15): 完了。`scenario_account.go` と `scenario_user2fa.go` で確認し、2026-10-26 にライブで正常実行を確認しました。
    - パスワードポリシー・認証条件・ID ポリシー・組織ポリシーの各更新は、読み取った値をそのまま送り返す同一値の往復で、組織設定を変更しません。
    - service-policy は無効状態での update が 409 で拒否されるため、有効化してから同一ルールで更新し、無効化に戻します。
    - user-2fa は使い捨ての一時ユーザーに対して、空ページの参照と `deactivate-otp`・`clear-trusted-devices` の正常終了、存在しない ID の操作が 404 で分類されることを確認します。
- 期待どおりの失敗を確認するステップは、`test/e2e/internal/evidence` の `RecordExpectedError` で記録します。ORDER.txt では `expected-error`、レポートでは「期待どおりの失敗」と表示され、CLI の終了コード 1 が期待値であるステップを予期しない失敗と区別します。
- ライブ実行では一時的なネットワークエラーが頻発したため、`SAKURACLOUD_RETRY_MAX=3` と `SAKURACLOUD_RETRY_WAIT_MIN=1`、`SAKURACLOUD_RETRY_WAIT_MAX=30` を実行時に指定しました。プロファイルの `RetryMax = 0` は変更していません。

### タスク4: apprun-dedicated-api

35コマンドがクラスタ作成を前提とし、実行コストが大きいため、[#59](https://github.com/sacloud/skr/issues/59) の範囲で管理します。

### 横断タスク

- コマンドツリーとシナリオの対応を検査する網羅テストを追加し、runner 追加時の網羅漏れを検出します。要求は [ADR 0058](../adr/0058-live-e2e-command-coverage.md) に記載しています。
- 各タスク完了後に、この文書の網羅表と未検証コマンド、`PLAN.md` の対応状況を更新します。

## 命名ルールの適用状況

- 既存 runner のディレクトリ名と証跡識別名は kebab-case で規則に適合しています。既存 runner の一括改名は行いません。
- タスク1で追加した `scenario_update.go`、`scenario_monitor_router.go` と対応する `_test.go` は命名規則に従います。証跡ステップは `test-update`、`test-monitor-router` のような kebab-case です。version の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/version/scenario_version_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`) は削除しました。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- タスク2で追加した `scenario_subnet.go` と `scenario_ipv6.go` は `test/e2e/internet/` にあります。eventbus-api のシナリオは `test/e2e/eventbus-api/` にあり、2026-10-27 に [ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) に従い testing.T ベースのシナリオテストへ移行しました。apprun のシナリオは `test/e2e/apprun/` にあり、同日に移行しました。
- タスク2の internet runner の証跡ステップは `test-add-subnet`、`test-update-subnet`、`test-delete-subnet`、`test-enable-ipv6`、`test-read-ipv6` などの kebab-case です。
- タスク2の eventbus runner の証跡ステップは `update-configuration`、`update-trigger`、`schedule-create`、`schedule-read`、`schedule-update` などの kebab-case です。2026-10-27 に testing.T ベースのシナリオテストへ移行しました。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- タスク2の apprun は 2026-10-27 に testing.T ベースのシナリオテストへ移行しました。旧 runner の証跡ステップは `update-application`、`list-versions`、`read-version`、`list-traffic`、`update-traffic`、`read-packet-filter` などの kebab-case です。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- server の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/server/scenario_server_test.go` ほか) へ移行しました。旧 runner (`main.go`、`scenario_update.go`) は削除し、SSH 鍵の生成と SSH 接続の確認は `test/e2e/internal/helper` に移しました ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。同日に作成前の同名確認、後片付けの再照合と不在確認、起動待ちのポーリングを SDK で直接実行するよう変更しました ([ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md))。実行記録にはテスト対象の CLI 操作だけが残ります。同日に偽の操作 (`fakeServerCommands`) によるシナリオ経路の単体テストを廃止し、実ロジックの単体テストだけを残しました。テストファイルはシナリオ・共有ロジック・コマンド操作の3つに整理し、CLI の引数は操作のメソッドに直接書いて、引数を組み立てるだけの関数とインターフェースも削除しました。
- タスク3の iam-api runner は `test/e2e/iam-api/` 配下に次のシナリオと、対応する `_test.go` を追加しました。証跡ステップは `account-read-password-policy`、`account-update-service-policy`、`u2fa-read-missing-security-key` などの kebab-case です。
    - `scenario_read_only.go`、`scenario_policy.go`、`scenario_hierarchy.go`、`scenario_serviceprincipal.go`、`scenario_projectapikey.go`。
    - `scenario_scim.go`、`scenario_sso.go`、`scenario_account.go`、`scenario_user2fa.go`。
- 任意の改善候補として、`test/e2e/simple-notification/crud.go` を `scenario_crud_only.go` に改名できます。対応する `crud_test.go` も合わせます。`test/e2e/gslb/server.go` は役割を表す名前のため変更不要です。2026-10-27 の simple-notification の移行で `crud.go` は削除済みです。
- switch の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/switch/scenario_switch_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`) は削除しました。旧 runner の固定名での事前後片付けはランダムな接尾辞付きの名前に置き換え、作成前の同名確認は [ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md) に従い実行記録を残さない SDK で直接実行し、後片付けの削除前再参照と削除後の不在確認はシナリオ内で行います。検証ゾーンは旧 runner の is1b モードに合わせて is1b の単一シナリオに絞り、旧 runner の Sandbox (tk1v) モードは削除しました。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- simplemq の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/simplemq/scenario_simplemq_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`) は削除しました。旧 runner の `evidence` 機構による実行記録は共有 helper の実行記録に置き換わり、API キーの発行は stdout を実行記録に残さない `CallHidingStdout` で行います。移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- simple-notification の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`scenario_notification_test.go`、`scenario_crud_only_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`、`crud.go`、`crud_test.go`) は `test/e2e/simple-notification/` から削除しました。既存通知先の ID は `SKR_E2E_SIMPLE_NOTIFICATION_DESTINATION_ID` 環境変数で渡し、未設定の場合はテストをスキップします。一時リソースの作成・更新・並べ替えの反映待ちは `helper.Sleep` で実行記録に残します。後片付けの削除と不在確認は `helper.Call` / `CallAllowingFailure` で行い、移行後はシナリオの CLI 呼び出しの説明が実行記録の見出しになります。
- serviceclass の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/serviceclass/scenario_serviceclass_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`) は削除しました。旧 runner の `--confirm-read-only-live` フラグによる実行確認は共有 helper の `SKR_E2E` ゲートに置き換わり、旧 runner の `evidence` 機構による実行記録は共有 helper の実行記録に置き換わります。シナリオは読み取り専用で、選択中のプロファイルの確認と `is1b` のサービスクラス検索を検証します。
- serverplan の E2E は 2026-10-27 に testing.T ベースのシナリオテスト (`test/e2e/serverplan/scenario_serverplan_test.go`) へ移行しました。旧 runner (`main.go`、`main_test.go`) は削除しました。旧 runner の `--confirm-read-only-live` フラグによる実行確認は共有 helper の `SKR_E2E` ゲートに置き換わり、旧 runner の `evidence` 機構による実行記録は共有 helper の実行記録に置き換わります。シナリオは読み取り専用で、選択中のプロファイルの確認と `is1b` のサーバープランの検索・参照、Find の結果と Read の一致を検証します。

## 参照

- [ADR 0058: 全サブコマンドをライブ E2E の検証対象にする](../adr/0058-live-e2e-command-coverage.md)
- [`PLAN.md`](../../PLAN.md)
- 各 runner の設計文書(`docs/design/` 配下)
