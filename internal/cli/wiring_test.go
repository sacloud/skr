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
	"os"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

// TestGeneratedWiringMatchesConfig は、生成済みの配線ファイルが設定から再生成した結果と一致することを確認します。
// 設定を変更した場合は make generate-wiring で再生成してください。
func TestGeneratedWiringMatchesConfig(t *testing.T) {
	// 生成ヘッダーの設定パスを Makefile と同じリポジトリルート起点に揃えます
	t.Chdir(repositoryPath(""))
	tests := []struct {
		name     string
		configs  []string
		mode     apigen.Mode
		expected string
	}{
		{
			name: "IaaS",
			configs: []string{
				"iaas-archive.json",
				"iaas-disk.json",
				"iaas-icon.json",
				"iaas-internet.json",
				"iaas-ipaddress.json",
				"iaas-ipv6addr.json",
				"iaas-ipv6net.json",
				"iaas-packetfilter.json",
				"iaas-product-disk.json",
				"iaas-product-internet.json",
				"iaas-product-license.json",
				"iaas-product-privatehost.json",
				"iaas-product-server.json",
				"iaas-region.json",
				"iaas-server.json",
				"iaas-serviceclass.json",
				"iaas-switch.json",
				"iaas-zone.json",
			},
			mode:     apigen.ModeIaaS,
			expected: "internal/cli/iaas_wiring_generated.go",
		},
		{
			name: "Eventbus",
			configs: []string{
				"eventbus-process-c0nfiguration.json",
				"eventbus-schedule.json",
				"eventbus-trigger.json",
			},
			mode:     apigen.ModeOgen,
			expected: "internal/cli/eventbus_wiring_generated.go",
		},
		{
			name: "IAM",
			configs: []string{
				"iam-auth.json",
				"iam-folder.json",
				"iam-group.json",
				"iam-id-policy.json",
				"iam-id-role.json",
				"iam-organization.json",
				"iam-policy.json",
				"iam-project-api-key.json",
				"iam-project.json",
				"iam-role.json",
				"iam-scim.json",
				"iam-service-policy.json",
				"iam-service-principal.json",
				"iam-sso.json",
				"iam-user-2fa.json",
				"iam-user.json",
			},
			mode:     apigen.ModeOgen,
			expected: "internal/cli/iam_wiring_generated.go",
		},
		{
			name: "SimpleMQ",
			configs: []string{
				"simplemq-queue.json",
			},
			mode:     apigen.ModeOgen,
			expected: "internal/cli/simplemq_wiring_generated.go",
		},
		{
			name: "SimpleNotification",
			configs: []string{
				"simple-notification-destination.json",
				"simple-notification-group.json",
				"simple-notification-history.json",
				"simple-notification-routing.json",
			},
			mode:     apigen.ModeOgen,
			expected: "internal/cli/simple_notification_wiring_generated.go",
		},
		{
			name: "AppRun",
			configs: []string{
				"apprun-application.json",
				"apprun-packet-filter.json",
				"apprun-traffic.json",
				"apprun-user.json",
				"apprun-version.json",
			},
			mode:     apigen.ModeOgen,
			expected: "internal/cli/apprun_wiring_generated.go",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := make([]string, 0, len(test.configs))
			for _, name := range test.configs {
				paths = append(paths, "api/commands/"+name)
			}
			want, err := apigen.GenerateWiring(paths, test.mode)
			require.NoError(t, err)
			got, err := os.ReadFile(test.expected)
			require.NoError(t, err)
			require.Equal(t, string(want), string(got), "generated wiring is stale; run make generate-wiring")
		})
	}
}
