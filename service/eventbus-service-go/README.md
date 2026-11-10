# eventbus-service-go

さくらのクラウド EventBus の実行設定 (process-configuration)、スケジュール (schedule)、トリガー (trigger) を操作する高レベルライブラリです。`sacloud-sdk-go` の [`api/eventbus`](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0/api/eventbus) が公開する低レベルの CommonServiceItem 操作を隠蔽し、ライブラリのコードを読むだけで使える doc comment と例を提供します。

## 提供する機能

- `Service` 構造体が、実行設定・スケジュール・トリガーごとの操作を提供します。
- 実行設定は宛先別の個別入力 (`simplenotification`、`simplemq`、`autoscale`) を受け付け、宛先とパラメーターの組み合わせを検証して API リクエストへ変換します。
- 更新時は現状を API から読み取り、未指定 (nil) の項目を既存値で保持するマージを行います。宛先を切り替えた場合は旧宛先のパラメーターを引き継ぎません。
- read・update・delete・set-secret は変更前に API から対象を再取得し、ID とリソース種別、Settings の種別が一致しなければ変更 API を呼び出しません。読み取りと変更の間に context がキャンセルされた場合も変更 API を呼び出しません。
- 出力射影 (`ProjectProcessConfiguration`、`ProjectSchedule`、`ProjectTrigger`) が、API の応答を利用者向けの構造体へ変換します。secret の値を出力に含めません。
- スケジュールの `StartsAt` は、API が保持するエポックミリ秒 (整数と数値文字列の両方) をミリ秒精度の UTC の `time.Time` へ変換します。リクエストの `StartsAt` も `time.Time` で受け付け、ミリ秒未満の小数秒は指定できず、エラーを返します。

## 使い方

```go
import (
	"os"

	eventbussdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"

	eventbus "github.com/sacloud/skr/service/eventbus-service-go"
)

// 認証情報は環境変数またはプロファイルから読み込みます。
// 秘密情報をコードや引数に直接書きません。
var client saclient.Client
if err := client.SetEnviron(os.Environ()); err != nil {
	return err
}
v1Client, err := eventbussdk.NewClient(&client)
if err != nil {
	return err
}
service := eventbus.New(v1Client)

// ptr は例を簡潔にするためのヘルパーです (例: func ptr[T any](v T) *T { return &v })。
created, err := service.CreateProcessConfiguration(ctx, &eventbus.CreateProcessConfigurationRequest{
	Name:        "example",
	Description: "説明",
	Settings: eventbus.ProcessConfigurationSettings{
		Destination: "simplemq",
		QueueName:   ptr("queue-name"),
		Content:     ptr("hello"),
	},
})
if err != nil {
	return err
}
output, err := eventbus.ProjectProcessConfiguration(created)
if err != nil {
	return err
}
```

実行設定の更新は、nil のフィールドを現状維持として扱います。次の例では名前だけを変更し、宛先のパラメーターは現状のまま保持します。

```go
updated, err := service.UpdateProcessConfiguration(ctx, &eventbus.UpdateProcessConfigurationRequest{
	ID:   output.ID,
	Name: ptr("renamed"),
})
```

コンパイルが通る追加の例は `example_test.go` にあります。Go の doc 形式 (`go doc`、pkg.go.dev) でも参照できます。

## 開発

このモジュールは `sacloud/skr` リポジトリ内で暫定管理しています。`sacloud/skr` のルートからは `replace` ディレクティブで参照されます。将来は `sacloud-sdk-go` の `service/eventbus` として提案することを想定しており、その際はモジュールパスを変更します。

```sh
cd service/eventbus-service-go
go test ./...
golangci-lint run ./...
```

## 関連ドキュメント

- [EventBus 高レベルコマンドの設計](../../docs/design/eventbus.md) — 入力の詳細、更新のマージ規則、出力設計。判断の背景 (service 層の切り出し、トリガー条件の指定方法) もこの設計文書からたどれます。
