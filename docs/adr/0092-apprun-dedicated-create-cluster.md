# ADR 0092: AppRun Dedicated 高レベルコマンドに create-cluster を追加する

- Status: Accepted
- Date: 2026-11-19

## 背景

[#266](https://github.com/sacloud/skr/issues/266) で、`skr apprun-dedicated` に `create-cluster` がほしいという要望がありました。[ADR 0083](0083-apprun-dedicated-infrastructure-lifecycle-commands.md) で `create-auto-scaling-group`、`delete-auto-scaling-group`、`delete-cluster` を追加したときは、クラスタ作成は「設定が複雑で `--request` JSON が必要になり、高レベル層で簡潔に表現できる部分が少ない」として対象外にしました。

しかしクラスタ作成の必須入力を改めて確認すると、SDK の `CreateClusterRequest` は名前、サービスプリンシパル ID、ロードバランサの待受ポート (`port` と `protocol` の組)、任意の Let's Encrypt 用メールアドレスだけです。待受ポートは [ADR 0078](0078-shared-text-to-json-notation.md) で採用した `key=value` カンマ区切りの反復可能フラグで簡潔に表現でき、JSON を必須にする根拠はなくなりました。削除系の高レベルコマンドが揃うなか、作成だけが低レベル JSON という不対称も解消できます。

## 決定

- `skr apprun-dedicated create-cluster` を追加します。

  ```text
  skr apprun-dedicated create-cluster <name> --service-principal-id=ID [--port 'port=PORT,protocol=PROTOCOL' ...] [--lets-encrypt-email=EMAIL]
  ```

- `<name>` は位置引数、`--service-principal-id` は必須フラグです ([ADR 0075](0075-high-level-command-tier.md) の位置引数の方針に従います)。
- `--port` は反復可能な `key=value` カンマ区切りフラグです ([ADR 0041](0041-server-repeatable-disk-and-nic-options.md) と同じ構文)。1 指定がロードバランサの待受ポート 1 個に対応し、`port` は 1 ～ 65535 の整数、`protocol` は `http`、`https`、`tcp` のいずれかです。省略時は待受ポートなし (空配列) で作成します。kong がカンマで分割しないよう `sep:"none"` を指定します。
- `--lets-encrypt-email` は Let's Encrypt の証明書発行に使うメールアドレスの任意フラグです。省略時は Let's Encrypt を設定しません。
- service 層 `Service.CreateCluster` は、同名クラスタの不在確認 (ページングを含む List 走査) → 作成 → `Read` による読み戻しと ID・名前の照合を行い、結果を JSON で出力します。
- 待機オプションは設けません。クラスタ作成 API は同期完了を表す `ClusterIdentifier` を返し、非同期の遷移待ち (healthy 待ちなど) は存在しません。ワーカノードの healthy 待ちは `create-auto-scaling-group` ([ADR 0083](0083-apprun-dedicated-infrastructure-lifecycle-commands.md)) が担います。
- [ADR 0041](0041-server-repeatable-disk-and-nic-options.md) の `key=value` 反復可能フラグは Server 限定としましたが、本 ADR は別サービスでの限定的な再採用です。他の配列項目への自動的な一般化は行いません。個別に設計根拠を示して採否を判断します。
- [ADR 0083](0083-apprun-dedicated-infrastructure-lifecycle-commands.md) の「クラスタ作成は高レベル層の対象外」とする判断を本 ADR が置き換えます。低レベル `apprun-dedicated-api create-cluster` (`--request` JSON) は仕様変更せず、JSON が必要な構成の経路として維持します。

## 影響

- `service/apprun-dedicated-service-go` に `Service.CreateCluster` と fake の作成操作を追加します。
- `internal/apprundedicated` に `CreateClusterCommand` と `--port` の構文解析・検証を追加します。
- ライブ E2E (`test/e2e/apprun-dedicated/scenario_infrastructure_lifecycle_test.go`) のクラスタ作成を高レベルコマンドに切り替えます。低レベル `create-cluster` のライブ検証は sakumock テスト (`TestRunAppRunDedicatedCommandsWithSakumock`) が JSON 経路を検証します。課金 (ワーカノード 1 台の時間割料金) は変わりません ([#59](https://github.com/sacloud/skr/issues/59))。
- ユーザーズマニュアル、チュートリアル、設計文書、`PLAN.md` を更新します。チュートリアルのクラスタ作成手順を低レベル JSON から高レベルコマンドに置き換えます ([ADR 0081](0081-high-level-command-users-manual.md))。

## 検討した選択肢

- 待受ポートを JSON 専用にする方式: 名前とサービスプリンシパル ID だけの入力には JSON ファイルが過剰で、`create-cluster` の高レベル化の利益が失われるため採用しません。
- 待受ポートを `--port 80=http` のような独自構文にする方式: リポジトリで採用済みの `key=value` カンマ区切り ([ADR 0041](0041-server-repeatable-disk-and-nic-options.md)) と構文が分かれ、利用者の学習コストが増えるため採用しません。
- 作成後の healthy 待ちを `--wait` で行う方式: クラスタ作成 API はワーカノードを持たず、待つべき非同期遷移が存在しないため採用しません ([ADR 0082](0082-high-level-command-wait-options.md) の対象は反映・削除後の不在確認などの待機が必要な操作)。
