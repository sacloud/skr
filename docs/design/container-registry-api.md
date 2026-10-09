# Container Registry API コマンド設計

## 範囲と配置

`container-registry-api` は `service/iaas/containerregistry` の high-level service と `api/iaas.ContainerRegistryAPI` を組み合わせます。Container Registry は CommonServiceItem を使うグローバルリソースのため、`iaas-api` のゾーン指定型リソースとは別のルートコマンドにします。パッケージは `internal/containerregistryapi`、CLI の SDK client factory と Runtime 接続は `internal/cli/containerregistry.go` に置きます。設計判断は [ADR 0034](../adr/0034-container-registry-api-commands.md) に記録しています。

## コマンド構成

| コマンド | SDK 操作 | 入力・出力 |
| --- | --- | --- |
| `registry list` | `FindWithContext` | 任意の `FindRequest` JSON。結果はレジストリ配列です |
| `registry create` | `CreateWithContext` | 限定した作成 JSON。結果はレジストリです |
| `registry read <id>` | `ReadWithContext` | レジストリ ID。結果はレジストリです |
| `registry update <id>` | `UpdateWithContext` | 限定した部分更新 JSON。結果はレジストリです |
| `registry delete <id>` | `DeleteWithContext` | レジストリ ID。成功時の出力はありません |
| `registry user list <registry-id>` | `ContainerRegistryAPI.ListUsers` | ユーザー名と権限の配列。パスワードは出力しません |
| `registry user add <registry-id>` | `ContainerRegistryAPI.AddUser` | ユーザー名、権限、`--password-file` |
| `registry user update <registry-id> <user-name>` | `ContainerRegistryAPI.UpdateUser` | `--permission` および／または `--password-file` |
| `registry user delete <registry-id> <user-name>` | `ContainerRegistryAPI.DeleteUser` | 識別子。成功時の出力はありません |

## リクエストと秘密情報

レジストリ作成では、SDK `CreateRequest` のうち `Name`、`Description`、`Tags`、`IconID`、`VirtualDomain` のみを受け付けます。`Name` は共通リソース名とレジストリ接続名の両方に設定します。更新では `Description`、`Tags`、`IconID`、`VirtualDomain` のポインター項目のみを受け付けます。どちらも `--request` にインライン JSON または `@path.json` で指定します。未知フィールドを拒否し、ユーザーやパスワードをリクエスト JSON から受け付けません。`AccessLevel` は SDK で deprecated とされ、現行マニュアルの公開設定の変更時期も踏まえて公開しません。

ユーザー追加・更新は独立したスカラー値と秘密情報ファイルで構成します。パスワード値は CLI 引数に含めず、ファイルまたは `-` による標準入力から読みます。ユーザー一覧の出力型はユーザー名・権限だけを含み、パスワードの値を含みません。

## 検証

`internal/containerregistryapi` の SDK caller test は公開 `iaas.APICaller` を通じて SDK の Container Registry API 操作とレスポンス変換を検証します。`internal/cli` のテストは実際の Kong コマンドパス、JSON の厳密なデコード、パスワードファイル、出力および入力エラーを確認します。`test/e2e/container-registry-api` は確認フラグ必須のライブシナリオで、ユニークなレジストリ・ユーザーを作成して照合し、今回作成した対象だけを削除します。

ビルド済み CLI と対象プロファイルを確認し、ライブ API に接続する操作を承認した場合のみ、次のコマンドを実行します。

```console
$ go run ./test/e2e/container-registry-api --skr ./skr --confirm-container-registry-live
```

runner はランダム名のレジストリとユーザーを作成し、レジストリの説明とユーザー権限を更新してから両方を削除します。既存名との衝突を検出した場合や識別情報が一致しない場合は変更せず停止します。コマンドの入出力証跡と `--request @path.json` で渡した JSON の内容は Git 管理外の `tmp/container-registry-api/` にアクセス制限付きで保存し、`REPORT.md` に表示します。パスワードは request JSON に含めず、証跡には記録しません。
