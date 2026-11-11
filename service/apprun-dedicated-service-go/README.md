# apprun-dedicated-service-go

さくらのクラウド AppRun Dedicated のアプリケーションを、デプロイから削除まで一連の操作として扱う高レベルライブラリです。`sacloud-sdk-go` の [`api/apprun-dedicated`](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0/api/apprun-dedicated) が公開する低レベルなアプリケーション・バージョン操作を隠蔽し、ライブラリのコードを読むだけで使える doc comment と例を提供します。

## 提供する機能

- `Service` 構造体が、deploy、rollback、deactivate、delete、status、versions、containers、init、render、diff の操作を提供します。
- `ApplicationDefinition` が、アプリケーションの定義をクラスタ名とアプリケーション名、バージョンの内容 (CPU、メモリー、スケーリング、イメージ、公開ポート、環境変数) で表現します。
- deploy は同名のアプリケーションがなければ作成し、バージョンを作成して有効化します。registryPassword のアクション (new/keep/remove) は定義と現在の状態から自動で決定します。
- rollback はアクティブバージョンより前のバージョンへの切り替えを行い、バージョン指定による切り戻しもできます。
- deactivate はアクティブバージョンを解除して停止します。delete はアクティブバージョンの解除、停止待ち、削除、不在確認をします。
- 変更の直前に定義の cluster、name と対象のクラスタ名・アプリケーション名を再照合し、一致しない場合や context がキャンセルされた場合は変更 API を呼び出しません。
- 待ち処理 (起動待ち、停止待ち、削除の再試行) はタイムアウト付きで行い、タイムアウト時は変更が完了していることを結果に含めて返します。
- render は定義を API へ送る JSON 形式に変換し、diff はアクティブバージョンとの差分を比較します。registryPassword と secret=true の環境変数の値は、出力と比較の対象から除かれます。
- init は既存のアプリケーションから定義を復元します。registryPassword と secret=true の環境変数の値は API から取得できないため、復元した定義には含まれません。

## 使い方

```go
import (
	"context"
	"os"

	apprunsdk "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"

	apprundedicated "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

// 認証情報は環境変数またはプロファイルから読み込みます。
// 秘密情報をコードや引数に直接書きません。
var client saclient.Client
if err := client.SetEnviron(os.Environ()); err != nil {
	return err
}
v1Client, err := apprunsdk.NewClient(&client)
if err != nil {
	return err
}
service := apprundedicated.New(v1Client)

result, err := service.Deploy(ctx, &apprundedicated.ApplicationDefinition{
	Cluster:     "example-cluster",
	Name:        "example-app",
	CPU:         1000,
	Memory:      1024,
	ScalingMode: v1.ScalingModeManual,
	FixedScale:  ptr(int32(1)),
	Image:       "ghcr.io/example/app:v1",
}, nil)
if err != nil {
	return err
}
```

デプロイ後の状態は status で確認できます。

```go
status, err := service.Status(ctx, &apprundedicated.ApplicationDefinition{
	Cluster: "example-cluster",
	Name:    "example-app",
})
```

コンパイルが通る追加の例は `example_test.go` にあります。Go の doc 形式 (`go doc`、pkg.go.dev) でも参照できます。

## 開発

このモジュールは `sacloud/skr` リポジトリ内で暫定管理しています。`sacloud/skr` のルートからは `replace` ディレクティブで参照されます。将来は `sacloud-sdk-go` の `service/apprun-dedicated` として提案することを想定しており、その際はモジュールパスを変更します。

```sh
cd service/apprun-dedicated-service-go
go test ./...
golangci-lint run ./...
```

## 関連ドキュメント

- [AppRun Dedicated 高レベルコマンドの設計](../../docs/design/apprun-dedicated.md) — 入力の詳細、registryPassword の扱い、待ち処理の規則。判断の背景 (service 層の切り出し、対応するコマンド構成) もこの設計文書からたどれます。
