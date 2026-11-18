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

//go:build skr_tutorial_harness

// このファイルはチュートリアル実行 harness (test/tutorials) 専用のビルドです。
// 通常のビルドには含まれず、製品の挙動には影響しません。
// docs/design/tutorial-script-harness.md を参照してください。
package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	sakumockiaas "github.com/sacloud/skr/internal/sakumock/iaas"
)

// harnessZonesEnvVar はチュートリアル harness がゾーン一覧をシードするための環境変数です。
// カンマ区切りのゾーン名を受け付けます。
const harnessZonesEnvVar = "SKR_TUTORIAL_HARNESS_ZONES"

// harnessIaaSURLEnvVar はチュートリアル harness が起動した IaaS モックの
// HTTP ブリッジの URL を伝える環境変数です。skr はサブプロセスで実行されるため、
// モックの状態を共有するには runner 側の 1 つのサーバーに接続します。
const harnessIaaSURLEnvVar = "SKR_TUTORIAL_HARNESS_IAAS_URL"

// New はチュートリアル harness 用の IaaS API クライアントを返します。
// 環境変数にモックの URL があれば HTTP 経由で接続し、なければ in-process の
// sakumock サーバーを返します。
func New(trace bool) (iaas.APICaller, error) {
	if url := os.Getenv(harnessIaaSURLEnvVar); url != "" {
		return sakumockiaas.NewRemoteCaller(url)
	}
	names := os.Getenv(harnessZonesEnvVar)
	if names == "" {
		return nil, fmt.Errorf("%s is not set", harnessZonesEnvVar)
	}
	var zones []string
	for _, name := range strings.Split(names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			zones = append(zones, name)
		}
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("%s must contain at least one zone", harnessZonesEnvVar)
	}
	server := sakumockiaas.NewTestServer(sakumockiaas.Config{Zones: zones})
	return server, nil
}
