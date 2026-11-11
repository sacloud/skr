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
package apprundedicated_test

import (
	"context"
	"fmt"
	"os"

	sdk "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"

	apprundedicated "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

func ptr[T any](value T) *T { return &value }

// newExampleService は例で使う Service を構築します。
// 認証情報は環境変数または SAKURA_PROFILE_DIR 配下のプロファイルから読み込みます。
func newExampleService() (*apprundedicated.Service, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	caller, err := sdk.NewClient(&client)
	if err != nil {
		return nil, err
	}
	return apprundedicated.New(caller), nil
}

// ExampleNew は Service の構築方法を示します。
func ExampleNew() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}
	_ = service
}

// ExampleService_Deploy はアプリケーションを新規作成または更新します。
// registryPassword の値は引数に渡さず、保護したファイルから読み込むなど、値の出所を
// 呼び出し側で保護してください。
func ExampleService_Deploy() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	// registryPassword の値は保護したファイル (0600 など) から読み込みます。
	password, err := os.ReadFile("path/to/registry-password")
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	result, err := service.Deploy(ctx, &apprundedicated.ApplicationDefinition{
		Cluster:          "example-cluster",
		Name:             "example-app",
		CPU:              1000,
		Memory:           1024,
		ScalingMode:      v1.ScalingModeManual,
		FixedScale:       ptr(int32(1)),
		Image:            "ghcr.io/example/app:v1",
		RegistryUsername: ptr("example-user"),
		RegistryPassword: ptr(string(password)),
		ExposedPorts: []version.ExposedPort{
			{
				TargetPort:       8080,
				LoadBalancerPort: ptr(v1.Port(443)),
				UseLetsEncrypt:   true,
				Host:             []string{"app.example.com"},
			},
		},
	}, nil)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(result.Cluster, result.Application, result.Version, result.ApplicationCreated)
}

// ExampleService_Render は定義を API へ送る JSON 形式に変換します。
// 秘密情報は出力から除かれます。
func ExampleService_Render() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	rendered, err := service.Render(&apprundedicated.ApplicationDefinition{
		Cluster:     "example-cluster",
		Name:        "example-app",
		CPU:         1000,
		Memory:      1024,
		ScalingMode: v1.ScalingModeManual,
		FixedScale:  ptr(int32(1)),
		Image:       "ghcr.io/example/app:v1",
	})
	if err != nil {
		return // エラー処理
	}
	// rendered は map[string]any です。JSON にするなどして利用します。
	_ = rendered
}

// ExampleService_Status はアプリケーションの現在の状態を取得します。
func ExampleService_Status() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	status, err := service.Status(ctx, &apprundedicated.ApplicationDefinition{
		Cluster: "example-cluster",
		Name:    "example-app",
	})
	if err != nil {
		return // エラー処理
	}
	fmt.Println(status.Cluster, status.Application.Name, status.Application.ActiveVersion)
}

// ExampleService_Rollback は前のバージョンへ切り替えます。
func ExampleService_Rollback() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	result, err := service.Rollback(ctx, &apprundedicated.ApplicationDefinition{
		Cluster: "example-cluster",
		Name:    "example-app",
	}, nil)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(result.Version)
}

// ExampleService_Delete はアプリケーションを削除します。
func ExampleService_Delete() {
	service, err := newExampleService()
	if err != nil {
		return // エラー処理
	}

	ctx := context.Background()
	result, err := service.Delete(ctx, &apprundedicated.ApplicationDefinition{
		Cluster: "example-cluster",
		Name:    "example-app",
	}, nil)
	if err != nil {
		return // エラー処理
	}
	fmt.Println(result.Cluster, result.Application)
}
