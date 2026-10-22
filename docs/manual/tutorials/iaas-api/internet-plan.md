# IaaS InternetPlan API: インターネットプランを検索・参照する

`iaas-api product internet` で対象ゾーンのインターネットプランを検索し、帯域幅と利用可否を確認できます。Find で取得した ID を使うと、プランを個別に参照できます。この API はプラン情報の読み取り専用です。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。
- 検索対象のゾーン名が分かること。

この手順はクラウドリソースを作成、変更、削除しません。

## 手順

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api product --help
$ skr iaas-api product internet --help
$ skr iaas-api product internet find --help
$ skr iaas-api product internet read --help
```

### 2. 対象ゾーンのプランを検索する

```console
$ skr iaas-api product internet find --zone ZONE --query '.InternetPlans | map({ID,Name,BandWidthMbps,Availability})'
```

`ZONE` は対象ゾーン名に置き換えます。`BandWidthMbps` は API が返す帯域幅、`Availability` は利用可否です。取得件数と開始位置は `--count`、`--from` で指定できます。

Filter や Sort などの API 検索条件は JSON で渡します。たとえば次の内容を `internet-plan-find.json` に保存し、ゾーンと名前を実際の値に置き換えます。

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
$ skr iaas-api product internet find --request @internet-plan-find.json --query '.InternetPlans | map({ID,Name,BandWidthMbps,Availability})'
```

### 3. ID でプランを参照する

Find の結果で確認した ID と同じゾーンを指定します。

```console
$ skr iaas-api product internet read --zone ZONE --id INTERNET-PLAN-ID
```

`ZONE` と `INTERNET-PLAN-ID` は実際の値に置き換えます。必要な項目だけを表示する場合は `--query` を指定できます。

## 動作の確認

Find は `Total`、`From`、`Count`、`InternetPlans` を含む JSON を出力し、Read は指定したプランを JSON で出力します。Find の先頭 ID を Read に渡し、ID、名前、帯域幅、利用可否を照合できます。

プラン情報の参照と、ルータ＋スイッチの帯域変更は別の操作です。帯域変更ではルータ＋スイッチの新規作成、設定引き継ぎ、旧リソース削除が行われ、変更後の課金が発生します。変更方法と料金上の注意は[回線帯域の変更](https://manual.sakura.ad.jp/cloud/network/switch/bandwidth-change.html)を確認してください。

## クリーンアップ

この手順は読み取り専用です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: 回線帯域の変更](https://manual.sakura.ad.jp/cloud/network/switch/bandwidth-change.html)
- [sacloud-sdk-go v0.3.1: InternetPlan API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#InternetPlanAPI)
- [sacloud-sdk-go v0.3.1: InternetPlan model](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#InternetPlan)
