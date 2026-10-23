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
| `test/e2e/simplemq` | queue 8操作、message 4操作(12) | 全コマンド実装済み |
| `test/e2e/simple-notification` | destination 6、group 6、history 2、routing 7(21) | 全コマンド実装済み |
| `test/e2e/container-registry-api` | registry 5操作、user 4操作(9) | 全コマンド実装済み |
| `test/e2e/networking-suite` | subnet-group 5、subnet 5、interface-connection 2、address 2(14) | 全コマンド実装済み |
| `test/e2e/gslb` | find / read / create / update / delete(5) | 全コマンド実装済み |
| `test/e2e/icon`、`test/e2e/disk`、`test/e2e/packetfilter`、`test/e2e/switch` | 各 CRUD(5) | 全コマンド実装済み |
| `test/e2e/region` | find / read(2) | 全コマンド実装済み |
| `test/e2e/serverplan` | find / read(2) | 全コマンド実装済み |
| `test/e2e/serviceclass` | find(1) | 全コマンド実装済み |
| `test/e2e/zone` | find(1) | 全コマンド実装済み |
| `test/e2e/version` | version(1) | 全コマンド実装済み。ライブで正常応答を確認済み |
| `test/e2e/iaas-lookup` | archive find / read、product disk find / read(4) | 全コマンド実装済み |
| `test/e2e/iaas-product-plans` | product internet find / read、product license find、product privatehost find / read(5) | 全コマンド実装済み |
| `test/e2e/object-storage-api` | 6シナリオで34コマンド | runner 実装済み。ライブは29コマンドの正常応答を確認済みで、`bucket penalty` と `site status bucket-metering` は API エラー、暗号化3操作は KMS キー作成未実装のため延期。状況は `PLAN.md` を参照 |

`config` は CLI のメタコマンドとして上表に含めます。`version` は `test/e2e/version` の専用 runner で検証します。

## 未検証コマンド

タスク2の完了後、未検証は59コマンドです。

| サービス | 未検証コマンド | 件数 |
| --- | --- | --- |
| iam-api | 次節の59コマンド | 59 |
| apprun-dedicated-api | 次節の35コマンド | 35 |

### iam-api の未検証59コマンド

分類は実装前の暫定であり、実装時に公式マニュアルと SDK で確認します。

- 読み取り系(15)
    - iam-role の list / read、id-role の list / read、id-policy read-organization。
    - auth の read-auth-context / read-auth-conditions / read-password-policy。
    - policy の read-folder / read-organization / read-project。
    - organization の read / read-service-policy。
    - service-policy の is-enabled / list-rule-templates。
- リソーススコープの書き込み(29)
    - policy の update-folder / update-project、folder move、project move。
    - scim の create / delete / list / read / regenerate-token / update。
    - sso の create / delete / link / list / read / unlink / update。
    - project-api-key の create / delete / list / read / update。
    - service-principal の update / list-keys / upload-key / delete-key / disable-key / enable-key / issue-token。
- アカウント全体やログインユーザーへ影響する書き込み(15)
    - auth の update-auth-conditions / update-password-policy。
    - service-policy の enable / disable、organization の update / update-service-policy。
    - policy update-organization、id-policy update-organization。
    - user-2fa の clear-trusted-devices / deactivate-otp / delete-security-key / delete-trusted-device。
    - user-2fa の list-security-keys / list-trusted-devices / read-security-key。

### apprun-dedicated-api の未検証35コマンド

cluster 5操作、application 6操作、version 4操作、auto-scaling-group 4操作、worker-node 3操作です。load-balancer 6操作(load-balancer node 2操作を含む)、certificate 5操作、service-class 2操作です。

## タスク

未検証コマンドを次の段階で整備します。各タスクの実装前に、シナリオ、成功条件、作成・変更・削除するリソースと費用の範囲を提示して承認を得ます。

### タスク1: 追加リソースなし・費用なしの拡張(完了)

- `server update`(1コマンド): 完了。`test/e2e/server/` の既存 runner に `scenario_update.go` として追加し、作成済みサーバーの名前と説明を更新して Read で反映を確認し、元の値に戻してから既存の削除フローへ進みます。restore に失敗した場合、削除フローは名前の不一致を検出して削除を拒否します。2026-10-23 にライブで正常実行を確認しました。
- `internet update` と `internet monitor-router`(2コマンド): 完了。`test/e2e/internet/` の既存 runner に `scenario_update.go` と `scenario_monitor_router.go` として追加します。作成済みルータ＋スイッチの名前・説明の更新と復元を Read で確認し、直近 10 分間の `monitor-router` を `--request` で実行します。トラフィックのない新規ルータに対する空配列は整形式の応答として扱います。2026-10-23 にライブで正常実行を確認しました。
- `version`(1コマンド): 完了。軽量な専用 runner `test/e2e/version/`(`--confirm-version-live`)を新規作成し、`skr version` が `v<semver> <os>/<arch>, build <revision>` 形式で出力して正常終了することを確認します。2026-10-23 にライブで正常実行を確認しました。

### タスク2: 同一実行リソース上の拡張(完了)

- `internet` の add-subnet / update-subnet / delete-subnet(3コマンド): 完了。`test/e2e/internet/` の既存 runner に `scenario_subnet.go` として追加し、作成済みルータ＋スイッチに追加スイッチを作成してサブネットを追加・NextHop 更新・削除を Read で確認します。2026-10-23 にライブで正常実行を確認しました。
- `internet` の enable-ipv6 / read-ipv6 / disable-ipv6(3コマンド): 完了。`scenario_ipv6.go` として同じ runner に追加し、同一ルータ＋スイッチで IPv6 の有効化・参照・無効化を Read で確認します。2026-10-23 にライブで正常実行を確認しました。
- `eventbus-api` の schedule 5コマンドと process-configuration update、trigger update(7コマンド): 完了。`scenario_schedule.go` と `scenario_update.go` を `test/e2e/eventbus-api/` に追加します。schedule は現在時刻の1年後に1日周期で発火する設定を作成し、テスト時間内に発火しません。process-configuration の update はメッセージ内容を更新し、その内容を SimpleMQ 待ち受けが受信することでライブ更新を証明します。trigger の update は説明のみを更新して復元します。2026-10-23 にライブで正常実行を確認しました。
- `apprun` の application update と version の list / read / status / delete(5コマンド): 完了。`test/e2e/apprun/` に `scenario_version.go` を追加し、application update で 1 vCPU / 2Gi の新バージョンを作成して Healthy 待ち、traffic を最新バージョンに100%割り当ててから前バージョンを削除します。2026-10-23 にライブで正常実行を確認しました。
- application update は新しいバージョンを作るため、version 操作と組み合わせて作成・参照・削除まで確認します。
- `apprun` の traffic と packet-filter、user の create / read(6コマンド): 完了。`scenario_packetfilter.go` と `scenario_user.go` を同じ runner に追加します。packet-filter は有効化＋設定を確認してから無効化に戻します。2026-10-23 にライブで正常実行を確認しました。
- `apprun user create`(1コマンド): 完了。サインアップはアカウント単位で取り消せないため、create の失敗を「サインアップ済み」として許容し、Read で `limit.application_count` を確認します。2026-10-23 にライブで正常実行を確認しました。
- vCPU とメモリの組み合わせは API 側で検証されます(0.5/1Gi、1/2Gi、2/4Gi)。update では 1 vCPU / 2Gi を使用します。

### タスク3: iam-api の段階的整備

未検証59コマンドを前節の暫定分類に従って整備します。

- 読み取り系(15): 費用と副作用がないため、最初に整備します。
- リソーススコープの書き込み(29): runner が作成する一時リソースに対する操作です。作成・変更・削除と後片付けを1シナリオで確認します。
- アカウント全体やログインユーザーへ影響する書き込み(15): 実行前の合意が必要です。延期する場合は追跡 Issue を作成します。

### タスク4: apprun-dedicated-api

35コマンドがクラスタ作成を前提とし、実行コストが大きいため、[#59](https://github.com/sacloud/skr/issues/59) の範囲で管理します。

### 横断タスク

- コマンドツリーとシナリオの対応を検査する網羅テストを追加し、runner 追加時の網羅漏れを検出します。要求は [ADR 0058](../adr/0058-live-e2e-command-coverage.md) に記載しています。
- 各タスク完了後に、この文書の網羅表と未検証コマンド、`PLAN.md` の対応状況を更新します。

## 命名ルールの適用状況

- 既存 runner のディレクトリ名と証跡識別名は kebab-case で規則に適合しています。既存 runner の一括改名は行いません。
- タスク1で追加した `scenario_update.go`、`scenario_monitor_router.go` と対応する `_test.go`、`test/e2e/version/`(`main.go` + `main_test.go`)は命名規則に従います。証跡ステップは `test-update`、`test-monitor-router`、`version-print` のような kebab-case です。
- タスク2で追加した `scenario_subnet.go` と `scenario_ipv6.go` は `test/e2e/internet/` にあります。`scenario_update.go` と `scenario_schedule.go` は `test/e2e/eventbus-api/` にあります。`scenario_version.go`、`scenario_packetfilter.go`、`scenario_user.go` は `test/e2e/apprun/` にあります。対応する `_test.go` も命名規則に従います。
- タスク2の internet runner の証跡ステップは `test-add-subnet`、`test-update-subnet`、`test-delete-subnet`、`test-enable-ipv6`、`test-read-ipv6` などの kebab-case です。
- タスク2の eventbus runner の証跡ステップは `update-configuration`、`update-trigger`、`schedule-create`、`schedule-read`、`schedule-update` などの kebab-case です。
- タスク2の apprun runner の証跡ステップは `update-application`、`list-versions`、`read-version`、`list-traffic`、`update-traffic`、`read-packet-filter` などの kebab-case です。
- 任意の改善候補として、`test/e2e/simple-notification/crud.go` を `scenario_crud_only.go` に改名できます。対応する `crud_test.go` も合わせます。`test/e2e/gslb/server.go` は役割を表す名前のため変更不要です。

## 参照

- [ADR 0058: 全サブコマンドをライブ E2E の検証対象にする](../adr/0058-live-e2e-command-coverage.md)
- [`PLAN.md`](../../PLAN.md)
- 各 runner の設計文書(`docs/design/` 配下)
