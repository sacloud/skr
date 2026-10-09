# ADR 0034: Container Registry API コマンドを追加する

- Status: Accepted
- Date: 2026-10-09

## Context

`sacloud-sdk-go v0.3.0` は `service/iaas/containerregistry` と `api/iaas.ContainerRegistryAPI` で Container Registry API を公開しています。これは IaaS の CommonServiceItem を使うグローバルリソースであり、通常の `skr iaas-api <resource>` に追加するゾーン指定型リソースとは形が異なります。レジストリ API にはユーザーのパスワードも含まれるため、リクエスト JSON に秘密情報を混在させない入力設計が必要です。

## Decision

- 作成では `--name` と任意のスカラーフラグを JSON 入力と排他的に提供します。タグなどの複雑な値は JSON 入力に残します。パスワード単独更新は現在の権限を読み取って保持し、対象ユーザーを一意に確認できない場合は更新しません。

- `skr container-registry-api registry` を専用コマンドとして追加し、レジストリの検索、作成、参照、更新、削除と、レジストリ配下のユーザー一覧・追加・更新・削除を公開します。実装は `internal/containerregistryapi` に置き、SDK の `containerregistry.Service` と `iaas.ContainerRegistryAPI` を利用します。
- レジストリの作成・更新では SDK request 全体を受け付けず、ユーザーやパスワードを含まない入力型へ JSON を厳密にデコードします。レジストリ名は作成時だけ受け付け、作成後の変更を許可しません。廃止された公開設定も入力として公開しません。
- ユーザーのパスワードは `--password-file` でファイルまたは標準入力からのみ受け付けます。ユーザー一覧は SDK が返すユーザー名と権限だけを出力し、パスワードの値は出力しません。
- sakumock に IaaS API の Container Registry 実装がないため、SDK の公開 `iaas.APICaller` interface を実装するテスト caller で SDK の API 操作経路を検証します。CLI 操作自体はドメイン API mock と再実行可能なライブ E2E runner でも検証します。

## Consequences

- Container Registry の管理操作を IaaS の他リソースと異なる共通サービス項目 API として公開できます。コマンドに `--zone` は追加しません。
- JSON リクエストは許可フィールドを限定するため、SDK が対応する全 Container Registry API フィールドを CLI から指定できるわけではありません。新しい項目を公開する場合は、仕様と秘密情報への影響を確認して入力型・ヘルプ・テストを更新します。
- コンテナイメージの push / pull や repository 操作は Container Registry 管理 API コマンドの範囲に含めず、サービスマニュアルに沿ったコンテナクライアントで行います。
- ライブ E2E はレジストリとユーザーを作成・更新・削除するため、明示確認なしでは実行しません。

## Alternatives considered

- `iaas-api containerregistry` に追加する方法: グローバルな CommonServiceItem API でゾーン指定がなく、既存の IaaS リソース用コマンドと異なるため採用しません。
- SDK request 型をそのまま `--request` で受け付ける方法: request にユーザーパスワードを含む経路を作り得るため採用しません。
- `container-registry-api` の生成コードで全操作を扱う方法: 作成・更新入力の限定、秘密情報のファイル入力、ユーザー一覧の出力制御が必要なため、明示的な手書きハンドラーを採用します。
