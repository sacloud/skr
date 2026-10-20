# IaaS Region API コマンド設計

## 対象操作

`github.com/sacloud/sacloud-sdk-go v0.3.1` の `service/iaas/region` が公開する Find / Read を `iaas-api region` に接続します。Region の API モデルは ID、名前、説明、ネームサーバーを持ちます。SDK リクエストに Zone は含まれないため、コマンドにもゾーン指定を設けません。

## 入力と出力

- Find は任意の `--count` と `--from` を受け付けます。検索名 (`Names`) と Sort は SDK `FindRequest` JSON を使います。Sort は `Key` と `Order` を持つオブジェクトの配列です。JSON は直接入力または `@path.json` で指定でき、個別フラグとの併用はできません。
- Read は Region ID を必須とし、`--id` または `ReadRequest` JSON を受け付けます。JSON と `--id` は併用できません。
- 結果は他の IaaS コマンドと同じ JSON 出力処理に渡します。`--query` で必要な項目に絞れます。

## CLI とテストの配置

生成設定は `api/commands/iaas-region.json`、生成コードは `internal/iaas/regionapi/region_api_generated.go` に置きます。SDK factory とルートコマンドは `internal/cli/iaas.go` で接続します。CLI 統合テストは `internal/cli/region_test.go`、確認フラグ付きの読み取り専用ライブ E2E とシナリオテストは `test/e2e/region/` に置きます。

## 参照

- [ADR 0052: IaaS Region API コマンドを追加する](../adr/0052-iaas-region-api-commands.md)
- [SDK Region API v0.3.1 の検索操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/find_service.go)
- [SDK Region API v0.3.1 の参照操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/read_service.go)
- [SDK Region request v0.3.1](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/region/find_request.go)
- [SDK Region model v0.3.1](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/api/iaas/zz_models.go#L3625)
- [さくらのクラウド API v1.1 設備関連 API](https://manual.sakura.ad.jp/cloud-api/1.1/facility/index.html)
