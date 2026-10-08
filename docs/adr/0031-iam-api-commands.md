# ADR 0031: IAM API コマンドを SDK 操作に対応させる

- Status: Accepted
- Date: 2026-10-08

## Context

`sacloud-sdk-go v0.3.0` の `api/iam` は、さくらのクラウド IAM API（ユーザー、グループ、IAM ポリシーバインディングなど）を公開しています。IAM の操作を CLI から直接利用できるようにする必要があります。IAM API はリソース数が多く、すべてを一度に公開するのではなく、中核となる操作から段階的に公開します。テストには `sakumock/iam` が利用できます。

## Decision

- `skr iam-api` コマンドを追加し、まず中核の 3 リソースを公開します。
  - `skr iam-api user`: SDK の `UserAPI` が公開する list、create、read、update、delete、register-email、unregister-email。
  - `skr iam-api group`: SDK の `GroupAPI` が公開する list、create、read、update、delete、read-memberships、update-memberships。
  - `skr iam-api policy`: SDK の `IAMPolicyAPI` を使います。組織、プロジェクト、フォルダのポリシーバインディングを `read-organization`、`read-project`、`read-folder` で参照します。`update-organization`、`update-project`、`update-folder` で更新します。
- 定型操作は `cmd/apigen-api` の設定から生成し、パスワードを扱う user の create / update と一覧操作（list）は手書きコマンドにします。
- user の list と group の list は手書きにします。生成器のフラグ経路は「フラグが 1 つでも指定された場合のみ有効」という形になり、フラグがすべて任意の list では無指定の実行がエラーになるためです。list の入力はページングと並び順の独立した任意スカラーだけで、`--request` の JSON 経路を必要としないため、手書きでフラグのみを受け付けます。
- user の list / group の list は、SDK がページング情報（items、count、next、previous）を返すため、items 配列だけを出力するラッパーを手書きします。一覧出力は SimpleMQ や IaaS と同じ配列形式にし、`--query` の `map({ID,Name})` が利用できるようにします。
- user の create と update では、パスワードを個別フラグやインライン JSON に含めず、`--password-file` でファイルまたは標準入力（`-`）から受け付けます。名前・コード・説明は単一スカラーなのでフラグで受け付けます。作成・更新リクエストに配列や複雑なオブジェクトはないため、この 2 操作に `--request` の JSON 経路は追加しません。
- group の update-memberships はユーザー ID の配列を `--request` の JSON 配列で受け付けます。policy の update は `bindings` 配列（ロールとプリンシパルのオブジェクト）を `--request` の JSON で受け付けます。
- IAM ポリシーバインディングの更新は、指定したスコープ（組織・プロジェクト・フォルダ）の現在の割り当て全体を置き換えます。部分的な追加や削除は提供しません。
- 認証は sacloud-sdk-go の標準プロファイル／環境変数を使います。IAM の各機能を利用するには、対象機能の権限を付与したサービスプリンシパルが必要です。
- テストは `sakumock/iam` に対して実際の CLI コマンドパスを実行して検証します。

## Consequences

- IAM ユーザー、グループ、ポリシーバインディングを CLI から操作できます。残りの IAM リソース（organization、project、service-principal など）は段階的に追加します。
- list の出力は items 配列のみで、count、next、previous は出力しません。ページングは `--page` と `--per-page` で行います。
- パスワードは `--password-file` のみで受け付けるため、パスワードを引数に直接書く利用方法はありません。
- ポリシーバインディングの更新は全体置き換えです。`read` で取得した出力から `bindings` 配列を編集し、`update` に渡します。

## Alternatives considered

- IAM の全リソースを一度に公開する方法: リソース数が多く、操作ごとの入力設計と検証が 1 回の変更として大きくなりすぎるため採用しません。
- list も生成コードにする方法: 生成器のフラグ経路では任意フラグのみの操作で無指定実行がエラーになり、ページング情報を出力するかどうかも生成設定で表現できないため採用しません。
- パスワードを個別フラグで受け付ける方法: コマンドライン引数やシェル履歴に秘密情報が残るため採用しません。
- ポリシーバインディングの部分的な追加・削除を提供する方法: API が現在の割り当て全体を受け付ける形式のため、CLI 独自の差分計算を追加せず SDK の操作に対応させます。
