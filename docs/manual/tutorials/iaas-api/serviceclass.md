# IaaS ServiceClass API: ゾーン内のサービスクラスを検索する

ServiceClass API で、指定したゾーンのサービスクラス情報を検索できます。検索結果にはサービスクラス名、パス、表示名、公開状態、料金情報が含まれます。

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
$ skr iaas-api service-class --help
$ skr iaas-api service-class find --help
```

### 2. ゾーン内のサービスクラスを検索する

```console
$ skr iaas-api service-class find --zone is1b --query 'map({ID,ServiceClassName,ServiceClassPath,DisplayName,IsPublic,Price})'
```

`is1b` は検索対象のゾーン名に置き換えます。検索結果は JSON の配列です。`--query` では必要な項目だけを選べます。取得件数や開始位置を指定する場合は `--count` と `--from` を追加します。

```console
$ skr iaas-api service-class find --zone ZONE --count 10 --from 0 --query 'map({ID,ServiceClassName,ServiceClassPath,DisplayName})'
```

検索条件の `Zone` は必須です。`Sort` は `FindRequest` JSON で指定します。`--request` と個別フラグは併用できません。

## クリーンアップ

この手順は読み取り専用の検索です。削除するリソースはありません。

## 参考資料

- [さくらのクラウド API v1.1](https://manual.sakura.ad.jp/cloud-api/1.1/)
- [さくらのクラウド API ポータル](https://manual.sakura.ad.jp/api/cloud/portal/)
- [sacloud-sdk-go v0.3.1: ServiceClass Find 操作](https://github.com/sacloud/sacloud-sdk-go/blob/v0.3.1/service/iaas/serviceclass/find_service.go)
- [sacloud-sdk-go v0.3.1: ServiceClass FindRequest](https://github.com/sacloud/sacloud-sdk-go/blob/v0.3.1/service/iaas/serviceclass/find_request.go)
- [sacloud-sdk-go v0.3.1: ServiceClass model](https://github.com/sacloud/sacloud-sdk-go/blob/v0.3.1/api/iaas/zz_models.go#L25516)
