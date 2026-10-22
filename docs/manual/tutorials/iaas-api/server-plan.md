# IaaS ServerPlan API: サーバプランを検索・参照する

ServerPlan API でゾーン内のサーバプランを検索し、ID を指定して個別のプラン情報を参照できます。サーバプランには CPU、メモリ、GPU、CPU モデル、利用可否などの情報が含まれます。サーバの作成やプラン変更に使うプランを確認するときに利用できます。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。
- 検索対象のゾーン名が分かること。

この手順は読み取り専用です。クラウドリソースを作成、変更、削除しません。

## 手順

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api product server --help
$ skr iaas-api product server find --help
$ skr iaas-api product server read --help
```

### 2. 対象ゾーンのプランを検索する

```console
$ skr iaas-api product server find --zone ZONE --query '.ServerPlans | map({ID,Name,CPU,MemoryMB,GPU,CPUModel,GPUModel,Commitment,Generation,ConfidentialVM,Availability})'
```

`ZONE` は対象ゾーン名に置き換えます。Find の JSON は API 応答の `Total`、`From`、`Count`、`ServerPlans` を含みます。`MemoryMB` は API が返すメモリ値です。取得件数と開始位置は `--count` と `--from` で指定できます。`--query` で `ServerPlans` 配列を取り出して表示しています。

```console
$ skr iaas-api product server find --zone ZONE --count 10 --from 0 --query '.ServerPlans | map({ID,Name,CPU,MemoryMB,Availability})'
```

名前で絞り込む場合は API の `Filter` を JSON で指定します。`server-plan-find.json` に対象ゾーンと検索条件を記述します。

```json
{
  "Zone": "ZONE",
  "Filter": {
    "Name": "PLAN-NAME"
  }
}
```

`Filter` の文字列値は部分一致で検索し、配列値は OR 条件として検索します。

```console
$ skr iaas-api product server find --request @server-plan-find.json --query '.ServerPlans | map({ID,Name,CPU,MemoryMB,Availability})'
```

`Filter` と `Sort` は API の検索条件として JSON で指定します。`--request` は個別フラグと併用できません。

### 3. ID でプランを参照する

検索結果から確認した ID と同じゾーンを指定します。

```console
$ skr iaas-api product server read --zone ZONE --id SERVER-PLAN-ID
```

`SERVER-PLAN-ID` は検索結果で確認した数値 ID に置き換えます。Find と Read の結果は JSON で出力されます。表示項目を限定するには、たとえば次のように指定します。

```console
$ skr iaas-api product server read --zone ZONE --id SERVER-PLAN-ID --query '{ID,Name,CPU,MemoryMB,GPU,CPUModel,GPUModel,Commitment,Generation,ConfidentialVM,Availability}'
```

コア専有プランはサーバの CPU プランの一部として選択できます。通常プランとコア専有プランの違いや、プラン変更時の条件は、[コア専有プラン](https://manual.sakura.ad.jp/cloud/server/dedicated-cpu-plan.html)と[サーバのプラン変更](https://manual.sakura.ad.jp/cloud/server/plan-update.html)を参照してください。プランの仕様を確認したうえで、サーバを作成またはプラン変更してください。

## クリーンアップ

この手順は読み取り専用の検索・参照です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: コア専有プラン](https://manual.sakura.ad.jp/cloud/server/dedicated-cpu-plan.html)
- [さくらのクラウドマニュアル: サーバのプラン変更](https://manual.sakura.ad.jp/cloud/server/plan-update.html)
- [sacloud-sdk-go v0.3.1: ServerPlan API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#ServerPlanAPI)
- [sacloud-sdk-go v0.3.1: ServerPlan Find response](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#ServerPlanFindResult)
- [sacloud-sdk-go v0.3.1: ServerPlan model](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#ServerPlan)
