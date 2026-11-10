// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// このファイルはパッケージの利用例です。Output コメントを持たない Example 関数は
// コンパイルだけが検証され、実行されません。
package eventbus_test

import (
	"context"
	"fmt"
	"os"
	"time"

	sdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"

	eventbus "github.com/sacloud/skr/service/eventbus-service-go"
)

func ptr[T any](value T) *T { return &value }

// newExampleService は例で使う Service を構築します。
// 認証情報は環境変数または SAKURA_PROFILE_DIR 配下のプロファイルから読み込みます。
func newExampleService() (*eventbus.Service, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	caller, err := sdk.NewClient(&client)
	if err != nil {
		return nil, err
	}
	return eventbus.New(caller), nil
}

// ExampleNew は Service の構築方法を示します。
func ExampleNew() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}
	_ = service
}

// ExampleService_CreateProcessConfiguration は実行設定を作成し、出力へ変換します。
func ExampleService_CreateProcessConfiguration() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
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
		return // エラー処理
	}
	output, err := eventbus.ProjectProcessConfiguration(created)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(output.ID, output.Destination, output.Parameters["queue_name"])
}

// ExampleService_UpdateProcessConfiguration は名前だけを変更します。
// nil のフィールドは現状維持のため、宛先のパラメーターは保持されます。
func ExampleService_UpdateProcessConfiguration() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	updated, err := service.UpdateProcessConfiguration(ctx, &eventbus.UpdateProcessConfigurationRequest{
		ID:   "113800000000",
		Name: ptr("renamed"),
	})
	if err != nil {
		return // エラー処理
	}
	output, err := eventbus.ProjectProcessConfiguration(updated)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(output.Name, output.Parameters["queue_name"])
}

// ExampleService_SetProcessConfigurationSecret は SimpleMQ 宛先の secret を登録します。
// secret の値は引数に渡さず、保護したファイルから読み込むなど、値の出所を
// 呼び出し側で保護してください。
func ExampleService_SetProcessConfigurationSecret() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	// secret の値は保護したファイル (0600 など) から読み込みます。
	apiKey, err := os.ReadFile("path/to/apikey")
	if err != nil {
		return // エラー処理
	}
	err = service.SetProcessConfigurationSecret(ctx, &eventbus.SetProcessConfigurationSecretRequest{
		ID: "113800000000",
		Secret: v1.NewSimpleMQSecretSetSecretRequestSecret(
			v1.SimpleMQSecret{APIKey: string(apiKey)},
		),
	})
	if err != nil {
		return // エラー処理
	}
}

// ExampleService_CreateSchedule は間隔指定のスケジュールを作成します。
func ExampleService_CreateSchedule() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	created, err := service.CreateSchedule(ctx, &eventbus.CreateScheduleRequest{
		Name: "example",
		Settings: eventbus.ScheduleSettings{
			ProcessConfigurationID: "113800000000",
			StartsAt:               time.Date(2030, 1, 10, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
			RecurringStep:          ptr(5),
			RecurringUnit:          ptr("hour"),
		},
	})
	if err != nil {
		return // エラー処理
	}
	output, err := eventbus.ProjectSchedule(created)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(output.ID, output.StartsAt)
}

// ExampleService_CreateTrigger は等号と包含の条件を持つトリガーを作成します。
// 条件キーには、モニタリングスイートのアラートで利用できる severity と subject を使います。
func ExampleService_CreateTrigger() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	created, err := service.CreateTrigger(ctx, &eventbus.CreateTriggerRequest{
		Name: "example",
		Settings: eventbus.TriggerSettings{
			Source: "//eventbus.sakura.ad.jp/monitoringsuite/alert",
			Types:  []string{"jp.ad.sakura.eventbus.alert.firing"},
			Conditions: []eventbus.TriggerCondition{
				{Key: "severity", Op: "eq", Values: []string{"critical"}},
				{Key: "subject", Op: "in", Values: []string{"AlertRule/113800000000", "AlertRule/113800000001"}},
			},
			ProcessConfigurationID: "113800000000",
		},
	})
	if err != nil {
		return // エラー処理
	}
	output, err := eventbus.ProjectTrigger(created)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(output.ID, output.Conditions[0].Key, output.Conditions[0].Op)
}
