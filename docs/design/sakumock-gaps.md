# sakumock の既知の差分と未整備

## この文書について

sakumock を使うテスト (CLI 操作テスト、チュートリアル harness、ライブ E2E のドライラン) で判明した「実 API との差分」「モックに無い操作」「モックサーバー自体が無いドメイン」を、この 1 ファイルに集約して追跡する ([ADR 0090](../adr/0090-sakumock-gap-tracking.md))。

- 新たな差分や未整備を確認したら、対応する節に「現象」と「影響と現状の扱い」を追記する。
- sakumock の更新や実装追加で解消したら、確認した日付と内容を残して該当箇所を更新する。
- サービスごとの設計文書は「sakumock による検証の状況」だけを書き、差分の詳細はこの文書を参照する。
- チュートリアルの実行可否の現状は `test/tutorials/baseline.txt` の status と理由、対応表はこの文書の「チュートリアル harness と sakumock の接続状況」に置く。

## sakumock の構成

テストでは 2 種類のモックを併用する。

| 名称 | 場所 | 対象 API | 接続方法 |
| --- | --- | --- | --- |
| sakumock | `github.com/sacloud/sakumock` (外部ライブラリ) | Ogen 系 API (SimpleMQ、EventBus、Workflows など) | `NewTestServer` で起動し `SAKURA_ENDPOINTS_<SERVICE>` 環境変数で接続 |
| sakumockiaas | `internal/sakumock/iaas` (リポジトリ内) | IaaS API | `iaas.APICaller` として in-process 接続。チュートリアル harness は `internal/sakumock/iaas/http.go` の HTTP ブリッジ経由 |

## 差分の扱いの分類

| 分類 | 意味 |
| --- | --- |
| 許容 | その環境の制約であることを確認したうえで、エラーメッセージの一致など限定的な条件でテストを通す |
| 除外 | 対象操作をモックでのテストから外し、正常系をライブ E2E で検証する |
| 自前モック | sakumock に操作やサーバーが無い場合に、`httptest` または SDK interface の test double で補う |
| 未整備 | チュートリアル baseline を `deferred` にして、console ブロックの抽出と baseline 照合のみを行う |

## Ogen 系 API (sakumock v0.14.0)

### Workflows

| 操作 | 差分 | 影響と現状の扱い |
| --- | --- | --- |
| `list-suggestions` | sakumock は `Suggests` 要素を `{Id, Name}` で返すが、API 応答スキーマ上は要素に `Count` (1 以上の整数) が必須である。SDK (`ListSuggestionsOKSuggestsItem`) も `Name` と `Count` を必須としてデコードするため、応答のデコードが `Count (field required)` で失敗する。sakumock 自身の応答スキーマ検証 (`validate_gen.go`) も `Count` を必須としており、モック内部の不整合 | 許容。ライフサイクル E2E (`test/e2e/workflows/scenario_lifecycle_test.go` の `tryListWorkflowsSuggestions`) は、デコードエラーのメッセージが一致する場合だけ制約として通し、それ以外のエラーは失敗扱いにする。正常系はライブ E2E で検証する (2026-11-19 時点でライブ実行は未実施)。チュートリアル (`docs/manual/tutorials/workflows-api.md`) は候補検索を対象外にしている |

### Workflows の動作メモ (差分ではないがテスト設計に関わる事項)

- Runbook の実行エンジンは `Config.EnableDataPlane` を `true` にしたときだけ動作する。無効の場合、実行は作成と同時に `Succeeded` になり、`Result` は `"null"` のまま Runbook が解釈されない。ステップの実際の実行 (`sys.sleep` など) や `Queued`/`Running` の遷移、キャンセルを含むテストではデータプレーンを有効にする。E2E とチュートリアル harness の両方で有効にしている。
- ストアはサーバープロセス内のメモリのみで、プラン一覧は静的データ (ID 1 と 2)。状態はプロセスの生存期間で失われるため、テストごとにサーバーを起動する前提で作成したデータに依存しないこと。
- 実行にはワークフローの公開設定が必要で、非公開のワークフローの実行は 409 (`not executable`) になる。

### モックサーバーが存在しないドメイン

| ドメイン | 影響と現状の扱い |
| --- | --- |
| container-registry | sakumock にサーバーが無い。`internal/cli` のテストは SDK interface の test double で検証する。チュートリアル (`container-registry-api.md`) は baseline で `deferred` |
| networking-suite | 同上。`internal/cli` のテストはヘルプ出力の検証のみで、操作経路は未検証。チュートリアル (`networking-suite-api.md`) は baseline で `deferred` (手動置換プレースホルダーの撤去も別途必要) |
| gslb、proxylb、local-router | IaaS スコープの Ogen 系 API。sakumock にサーバーが無い。`internal/cli` のテストは SDK interface の test double で検証する。チュートリアルは baseline で `deferred` |

### 自前モックで代替しているドメイン

| ドメイン | 現状 | 移行候補 |
| --- | --- | --- |
| security-control | sakumock v0.13.0 にはサーバーが無かったため、`internal/cli/securitycontrol_test.go` は `httptest` と `SAKURA_ENDPOINTS_SECURITY_CONTROL` による自前モックで実装している | v0.14.0 (2026-11-07) で sakumock に `securitycontrol` モックが追加された。応答形式と操作範囲の整合を確認できたら自前モックから移行する (未確認) |
| simple-notification | sakumock には `simplenotification` モックが存在するが、`internal/cli/simplenotification_test.go` は `httptest` による自前モックで実装している | 同上。整合を確認できたら移行する (未確認) |

### SimpleMQ、EventBus、AppRun、AppRun Dedicated、IAM、KMS、Object Storage、Secret Manager、Service Endpoint Gateway

- いずれも sakumock のモックサーバーがあり、`internal/cli` の CLI 操作テストで使用している。既知の差分は未確認。見つけたらこの文書に節を追加する。

## IaaS API (sakumockiaas)

`internal/sakumock/iaas` は zone、icon、packetfilter、switch、database (appliance の一部操作: 参照・削除・電源・リセット・モニター、パラメータ一覧) のみ実装している。それ以外のリソースの URL は `unsupported IaaS request URL` エラーになる。

icon の検索は、SDK が `Names`/`Tags`/`Scope` を Filter (`Name`、`Tags.Name`、`Scope`) に変換した形式を受け付ける。検索条件は `Count`/`From`/`Sort`/`Filter` の一部キーのみを解釈し、FindCondition の `Include`/`Exclude` などそれ以外は無視する。ソートキーはリソースごとに限定的である (icon は ID/Name/Scope)。

database は create と update (POST/PUT の本体) に未対応で、テストは `AddDatabase` による事前シードで代替する。

チュートリアル baseline で `deferred` になっている IaaS 系チュートリアルと、sakumockiaas に起因する理由は次のとおり (`test/tutorials/baseline.txt` の理由と対応する)。

| チュートリアル | sakumockiaas に起因する理由 | その他の理由 |
| --- | --- | --- |
| `iaas-api/archive.md` | archive 未対応 | なし |
| `iaas-api/database.md` | create / update 未対応 | なし |
| `iaas-api/disk.md` | disk 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/disk-plan.md` | product disk 未対応 | なし |
| `iaas-api/icon.md` | 要確認 (モック自体は icon の CRUD と Filter 検索に対応済み。deferred の実原因は Phase 4 以降の実行確認で特定する) | 手動置換プレースホルダーの撤去 |
| `iaas-api/internet.md` | internet 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/internet-plan.md` | product internet 未対応 | なし |
| `iaas-api/ipaddress.md` | ipaddress 未対応 | なし |
| `iaas-api/ipv6addr.md` | ipv6addr 未対応 | なし |
| `iaas-api/ipv6net.md` | ipv6net 未対応 | なし |
| `iaas-api/license-info.md` | product license 未対応 | なし |
| `iaas-api/private-host-plan.md` | product privatehost 未対応 | なし |
| `iaas-api/region.md` | region 未対応 | なし |
| `iaas-api/server.md` | server 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/serviceclass.md` | service class 未対応 | なし |

- `iaas-api/packetfilter.md` と `iaas-api/switch.md` はモック対応済みで、理由は手動置換プレースホルダーの撤去のみ。
- `iaas-api/zone.md` は実行検証済み (`implemented`)。
- IaaS モックの拡充はチュートリアル harness の Phase 4 以降の作業 ([docs/design/tutorial-script-harness.md](tutorial-script-harness.md)) に含まれる。

## チュートリアル harness と sakumock の接続状況

| 状況 | チュートリアル |
| --- | --- |
| 接続済み (実行検証済み) | `eventbus.md` (SimpleMQ + EventBus)、`simplemq-api.md`、`iaas-api/zone.md`、`workflows-api.md` |
| sakumock にサーバーがあるが harness 未接続 | `apprun-api.md`、`apprun-dedicated.md`、`iam-api.md`、`kms-api.md`、`object-storage-api.md`、`secretmanager-api.md`、`security-control-api.md` (v0.14.0 追加分)、`service-endpoint-gateway-api.md`、`simple-notification-api.md` |
| sakumock にサーバーが無い | `container-registry-api.md`、`local-router-api.md`、`networking-suite-api.md`、IaaS 系は上の表を参照 |

- harness 未接続のチュートリアルは `internal/cli` では sakumock を使って検証済みである。「接続済み」にするには、fixture の作成とチュートリアル本文がモックの応答範囲で完結することの確認が必要である。

## 更新ルール

1. 差分・未整備を確認したら、対応する節に「現象」と「影響と現状の扱い」を書いて追記する。どのテストで許容・除外したか (ファイル名と関数名) まで書く。
2. sakumock のバージョン更新で解消したら、該当箇所に解消日と確認内容を残して更新する。節ごと削除する場合は、設計文書側の参照が残らないか確認する。
3. ライブ E2E で正常系を確認したら、「許容」「除外」の記載に実施日を添えて更新する。
4. `test/tutorials/baseline.txt` の status や理由を変えたら、この文書の対応表も更新する。
