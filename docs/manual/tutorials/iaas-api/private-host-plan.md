# IaaS PrivateHostPlan API: 専有ホストプランを検索・参照する

専有ホストは、仮想サーバを収容する物理ホストの CPU とメモリを専有するサービスです。`iaas-api product privatehost` でゾーン内の専有ホストプランを検索し、CPU、CPU モデル、メモリ、利用可否を確認できます。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。
- 検索対象のゾーン名が分かること。

この API はプラン情報の検索・参照です。専有ホストの契約やサーバの割り当ては行いません。専有ホストの契約は利用料金を伴うため、実際の契約前に対象ゾーンと料金を確認してください。

## 手順

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api product --help
$ skr iaas-api product privatehost --help
$ skr iaas-api product privatehost find --help
$ skr iaas-api product privatehost read --help
```

### 2. 対象ゾーンのプランを検索する

```console
$ skr iaas-api product privatehost find --zone ZONE --query '.PrivateHostPlans | map({ID,Name,Class,CPU,CPUModel,Dedicated,MemoryMB,Availability})'
```

`ZONE` は対象ゾーン名に置き換えます。`MemoryMB` は API が返すメモリ値です。取得件数と開始位置は `--count`、`--from` で指定できます。

Filter や Sort などの API 検索条件は JSON で渡します。次を `private-host-plan-find.json` に保存し、ゾーンと名前を実際の値に置き換えます。

```json
{
  "Zone": "ZONE",
  "Filter": {
    "Name": "PLAN-NAME"
  }
}
```

Filter の文字列値は部分一致、配列値は OR 条件です。`--request` と個別フラグは併用できません。

```console
$ skr iaas-api product privatehost find --request @private-host-plan-find.json --query '.PrivateHostPlans | map({ID,Name,Class,CPU,CPUModel,Dedicated,MemoryMB,Availability})'
```

### 3. ID でプランを参照する

Find の結果で確認した ID と同じゾーンを指定します。

```console
$ skr iaas-api product privatehost read --zone ZONE --id PRIVATE-HOST-PLAN-ID
```

`ZONE` と `PRIVATE-HOST-PLAN-ID` は実際の値に置き換えます。Find と Read の結果から、プランの ID、名前、CPU、メモリ、利用可否を照合できます。

## 動作の確認

Find は `Total`、`From`、`Count`、`PrivateHostPlans` を含む JSON を出力し、Read は指定したプランを JSON で出力します。プラン情報は仕様の確認に使えますが、この API から専有ホストを契約したり、サーバを割り当てたりできません。

## クリーンアップ

この手順は読み取り専用です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: 専有ホスト](https://manual.sakura.ad.jp/cloud/server/private-host.html)
- [sacloud-sdk-go v0.3.1: PrivateHostPlan API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#PrivateHostPlanAPI)
- [sacloud-sdk-go v0.3.1: PrivateHostPlan model](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#PrivateHostPlan)
