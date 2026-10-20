# ADR 0052: IaaS Region API コマンドを追加する

- Status: Accepted
- Date: 2026-10-20
- SDK: `github.com/sacloud/sacloud-sdk-go v0.3.1`

## Context

`PLAN.md` で未対応となっている IaaS 補助サービスの Region には、SDK で Find / Read が公開されています。Region のリクエストにはゾーン指定がなく、共通 IaaS リソースのゾーン別操作とは入力と API の性質が異なります。

## Decision

- `iaas-api region` に SDK の Find / Read のみを公開します。SDK にない作成・更新・削除操作は追加しません。
- Find は任意の `--count` / `--from` と `--request` JSON を受け付けます。検索名と Sort は JSON リクエストで指定し、入力経路は混在させません。
- Read は必須の Region ID を `--id` または ReadRequest JSON で受け付けます。
- Zone フラグやゾーン横断処理は追加しません。SDK の Region リクエストに Zone フィールドがなく、Region API 操作は SDK に注入された API caller で実行されます。
- コマンドは `cmd/apigen-iaas` の設定から生成し、SDK factory と CLI 登録は `internal/cli` に接続します。
- 自動テストでは CLI 経由の入力、SDK request、出力、エラー伝播を確認します。別途、実環境を変更しない確認フラグ付きの読み取り専用 Find / Read E2E runner を用意します。

## Consequences

- 利用者は Region の一覧と ID による参照を CLI から実行できます。
- SDK が提供しない操作や、ゾーンを指定する入力は CLI に存在しません。
- ライブ E2E は認証済み SDK プロファイルを使いますが、Find / Read のみを実行し、クラウドリソースを変更しません。

## Alternatives considered

- `--zone` を Region の各操作に追加する案: SDK のリクエスト型に Zone がなく、API 入力として説明できないため採用しません。
- SDK 操作を全て同じ CRUD として公開する案: SDK は Find / Read のみを提供するため採用しません。
- リソース共通の zone コマンドへ追加する案: Region は独立した SDK サービスであり、`iaas-api <resource>` の候補として管理する方針に従いません。
