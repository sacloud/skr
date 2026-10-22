# IaaS LicenseInfo API: ライセンス情報を検索する

`iaas-api product license` でライセンス情報を検索し、名称や利用条件を確認できます。結果は `Total`、`From`、`Count`、`LicenseInfo` を含む JSON です。LicenseInfo API が公開する操作は Find のみで、ゾーン指定や Read はありません。

## 前提条件

- `skr` がビルド済みであること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- 選択中のプロファイルまたは API キーで IaaS API を利用できること。

この手順はクラウドリソースを作成、変更、削除しません。

## 手順

### 1. 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api product --help
$ skr iaas-api product license --help
$ skr iaas-api product license find --help
```

### 2. ライセンス情報を検索する

```console
$ skr iaas-api product license find --query '.LicenseInfo | map({ID,Name})'
```

検索条件を追加しない場合は、一覧を取得します。取得件数と開始位置は `--count`、`--from` で指定できます。`TermsOfUse` を含めて確認する場合は、結果全体を表示するか、必要な項目を `--query` に指定してください。

Filter や Sort などの API 検索条件は JSON で渡します。次を `license-info-find.json` に保存します。

```json
{
  "Filter": {
    "Name": "LICENSE-NAME"
  }
}
```

`LICENSE-NAME` は検索する文字列に置き換えます。Filter の文字列値は部分一致、配列値は OR 条件です。`--request` と `--count`、`--from` は併用できません。

```console
$ skr iaas-api product license find --request @license-info-find.json --query '.LicenseInfo | map({ID,Name})'
```

## 動作の確認

Find の JSON にはライセンス情報の識別子、名前、利用条件などが含まれます。SDK の `LicenseInfo` モデルは `ID`、`Name`、`CreatedAt`、`ModifiedAt`、`TermsOfUse` を公開しています。`--query` を使うと必要な項目だけを表示できます。

利用条件の適用可否はライセンスの種類や利用形態によって異なります。実際の利用条件と適用範囲は、対象ライセンスの情報および[ライセンスに関する FAQ](https://manual.sakura.ad.jp/cloud/support/license/ms-license.html)で確認してください。

## クリーンアップ

この手順は読み取り専用です。削除するリソースはありません。

## 参考資料

- [さくらのクラウドマニュアル: マイクロソフト製品のライセンスに関する FAQ](https://manual.sakura.ad.jp/cloud/support/license/ms-license.html)
- [sacloud-sdk-go v0.3.1: LicenseInfo API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#LicenseInfoAPI)
- [sacloud-sdk-go v0.3.1: LicenseInfo model](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#LicenseInfo)
