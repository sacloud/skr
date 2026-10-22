# ADR 0055: IaaS 商品プラン API コマンド

- Status: Accepted
- Date: 2026-10-22

## Context

`sacloud-sdk-go v0.3.1` は、InternetPlan、LicenseInfo、PrivateHostPlan の低レベル API を公開しています。API route と SDK 操作の違いを CLI に反映し、既存のプラン情報コマンドと同じく読み取り専用で利用できるようにする必要があります。

## Decision

- InternetPlan は `/api/cloud/1.1/product/internet` に対応する `iaas-api product internet` とし、Find / Read を公開します。
- LicenseInfo は `/api/cloud/1.1/product/license` に対応する `iaas-api product license` とし、SDK が公開する Find のみを公開します。ゾーン指定や Read は追加しません。
- PrivateHostPlan は `/api/cloud/1.1/product/privatehost` に対応する `iaas-api product privatehost` とし、Find / Read を公開します。
- IaaS 用の既存生成器を使用します。Find の応答 envelope を JSON 出力に保ち、ゾーンが必要な操作は単一ゾーンの共通検証を有効にします。LicenseInfo の Find はゾーンなしで実行でき、条件を省略した一覧取得を許可します。
- 3 API のライブ E2E は読み取り専用にします。InternetPlan と PrivateHostPlan は is1b で Find 結果と Read 結果を照合し、LicenseInfo は Find の応答を検証します。

## Consequences

- 3 API のユーザー向けコマンド、SDK 呼び出し、ヘルプ、チュートリアルを個別に保守します。SDK にない操作を補いません。
- E2E はクラウドリソースを作成・変更・削除しません。選択中の認証プロファイルで商品情報の読み取り権限が必要です。
- LicenseInfo の `TermsOfUse` は検索応答に含まれ得るため、E2E の出力投影から除外し、private evidence に記録しません。

## Alternatives considered

- SDK のサービスパッケージ名をそのまま CLI 階層にする案は採用しません。既存の IaaS 商品プランコマンドに倣い、CLI の階層は API route を反映した `product` とします。
- LicenseInfo に Read やゾーンを追加する案は採用しません。SDK v0.3.1 の公開 API にこれらの操作や引数がないためです。
- ライブ E2E でプランを契約・変更する案は採用しません。検索・参照だけで CLI と SDK の通信および応答内容を検証できます。
