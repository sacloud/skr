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

package cli

import (
	apprunsdk "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/skr/internal/apprundedicated"
	"github.com/sacloud/skr/internal/apprundedicatedapi"
	clientconfig "github.com/sacloud/skr/internal/saclient"
	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

func (c *cli) initAppRunDedicatedAPI() {
	c.AppRunDedicatedAPI = apprundedicatedapi.NewCommands(apprundedicatedapi.Runtime{
		NewClient: func() (*v1.Client, error) {
			return apprundedicatedapi.NewClient(c.Trace)
		},
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	})
}

// initAppRunDedicated は手書きの高レベル apprun-dedicated コマンド群を配線します (ADR 0079)。
// 出力処理は CLI 共通の実装を、API クライアントは factory 関数で注入します。
func (c *cli) initAppRunDedicated() {
	c.AppRunDedicated = apprundedicated.NewCommands(
		apprundedicated.Runtime{
			ValidateOutput: validateOutput,
			WriteOutput:    writeOutput,
		},
		func() (*apprundedicatedservice.Service, error) {
			return newAppRunDedicatedService(c.Trace)
		},
		func() (*apprundedicatedservice.Service, error) {
			return newAppRunDedicatedDeployService(c.Trace)
		},
	)
}

// newAppRunDedicatedService は高レベルの apprun-dedicated コマンド用に、
// service/apprun-dedicated-service-go の Service を構築します。
func newAppRunDedicatedService(trace bool) (*apprundedicatedservice.Service, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	v1Client, err := apprunsdk.NewClient(client)
	if err != nil {
		return nil, err
	}
	return apprundedicatedservice.New(v1Client), nil
}

// newAppRunDedicatedDeployService は deploy 用の Service を構築します。
// deploy は registryPassword や secret=true の環境変数の値をリクエスト本文に
// 含め得るため、実行前に実効トレース設定を検査し、トレースが有効な場合は
// 秘密情報が記録されるおそれがあるため実行しません。
func newAppRunDedicatedDeployService(trace bool) (*apprundedicatedservice.Service, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	if err := apprundedicated.CheckTraceSettings(client); err != nil {
		return nil, err
	}
	v1Client, err := apprunsdk.NewClient(client)
	if err != nil {
		return nil, err
	}
	return apprundedicatedservice.New(v1Client), nil
}
