# sakumockiaas の対応範囲と未対応

## この文書について

リポジトリ内実装の IaaS モック sakumockiaas (`internal/sakumock/iaas`) の対応範囲と未対応を、リポジトリ側の作業として追跡する ([ADR 0091](../adr/0091-split-sakumock-and-sakumockiaas-gap-tracking.md))。upstream issue は存在せず、対応が必要な作業はこの文書と [チュートリアル script harness の設計](tutorial-script-harness.md) の Phase 計画で管理する。

- 操作やリソースの対応を進めたら、「対応範囲と制限」と「チュートリアル baseline との対応」を更新する。
- `test/tutorials/baseline.txt` の status や理由を変えたら、対応表も更新する。
- 差分の扱いの分類 (許容、除外、自前モック、未整備) は [sakumock の既知の差分と未整備](sakumock-gaps.md) の定義に従う。

外部ライブラリの sakumock (`github.com/sacloud/sakumock`、Ogen 系 API のモック) は別物で、その差分は [sakumock の既知の差分と未整備](sakumock-gaps.md) で管理する。

## 位置づけ

| 名称 | 場所 | 対象 API | 接続方法 |
| --- | --- | --- | --- |
| sakumockiaas | `internal/sakumock/iaas` (リポジトリ内) | IaaS API | `iaas.APICaller` として in-process 接続。チュートリアル harness は `internal/sakumock/iaas/http.go` の HTTP ブリッジ経由 |

## 対応範囲と制限

zone、icon、packetfilter、switch、database (appliance の一部操作: 参照・削除・電源・リセット・モニター、パラメータ一覧) のみ実装している。それ以外のリソースの URL は `unsupported IaaS request URL` エラーになる。

icon の検索は、SDK が `Names`/`Tags`/`Scope` を Filter (`Name`、`Tags.Name`、`Scope`) に変換した形式を受け付ける。検索条件は `Count`/`From`/`Sort`/`Filter` の一部キーのみを解釈し、FindCondition の `Include`/`Exclude` などそれ以外は無視する。ソートキーはリソースごとに限定的である (icon は ID/Name/Scope)。

database は create と update (POST/PUT の本体) に未対応で、テストは `AddDatabase` による事前シードで代替する。

## チュートリアル baseline との対応

チュートリアル baseline で `deferred` になっているチュートリアルのうち、sakumockiaas に起因する理由を含むものと、その理由は次のとおり (`test/tutorials/baseline.txt` の理由と対応する)。

| チュートリアル | sakumockiaas に起因する理由 | その他の理由 |
| --- | --- | --- |
| `gslb-api.md` | appliance 拡充が必要 | sakumock に GSLB モックサーバーが無い (Ogen 系)、手動置換プレースホルダーの撤去 |
| `proxylb-api.md` | 拡充が必要 | sakumock に ProxyLB モックサーバーが無い (Ogen 系)、手動置換プレースホルダーの撤去 |
| `iaas-api/archive.md` | archive 未対応 | なし |
| `iaas-api/database.md` | create / update 未対応 | なし |
| `iaas-api/disk.md` | disk 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/disk-plan.md` | product disk 未対応 | なし |
| `iaas-api/icon.md` | なし | 手動置換プレースホルダーの撤去 |
| `iaas-api/internet.md` | internet 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/internet-plan.md` | product internet 未対応 | なし |
| `iaas-api/ipaddress.md` | ipaddress 未対応 | なし |
| `iaas-api/ipv6addr.md` | ipv6addr 未対応 | なし |
| `iaas-api/ipv6net.md` | ipv6net 未対応 | なし |
| `iaas-api/license-info.md` | product license 未対応 | なし |
| `iaas-api/private-host-plan.md` | product privatehost 未対応 | なし |
| `iaas-api/region.md` | region 未対応 | なし |
| `iaas-api/server-plan.md` | server plan 未対応 | なし |
| `iaas-api/server.md` | server 未対応 | 手動置換プレースホルダーの撤去 |
| `iaas-api/serviceclass.md` | service class 未対応 | なし |

- `iaas-api/packetfilter.md`、`iaas-api/switch.md`、`iaas-api/icon.md` はモック対応済み (icon は CRUD と Filter 検索に対応) で、baseline の理由は手動置換プレースホルダーの撤去のみである。3 件とも実行確認は未済で、Phase 4 以降に実行して `deferred` 解除を判断する。
- `iaas-api/zone.md` は実行検証済み (`implemented`)。
- IaaS モックの拡充はチュートリアル harness の Phase 4 以降の作業 ([docs/design/tutorial-script-harness.md](tutorial-script-harness.md)) に含まれる。

## 更新ルール

1. 対応を進めたら「対応範囲と制限」と対応表を更新する。`internal/sakumock/iaas` の単体テストと、harness 経由の実行確認が通ってから更新する。
2. チュートリアルの実行検証が通ったら、対応表の行を更新し、`test/tutorials/baseline.txt` の status も `implemented` に変える。
3. この文書の対象はリポジトリ内実装のため、upstream への報告は不要である。IaaS モックの拡充の進捗は [docs/design/tutorial-script-harness.md](tutorial-script-harness.md) の Phase 計画と `PLAN.md` でも追跡する。
