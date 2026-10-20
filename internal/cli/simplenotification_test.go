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
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
)

func TestSimpleNotificationGeneratedCodeMatchesConfig(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{"api/commands/simple-notification-destination.json", "internal/simplenotificationapi/simple_notification_destination_api_generated.go"},
		{"api/commands/simple-notification-group.json", "internal/simplenotificationapi/simple_notification_group_api_generated.go"},
		{"api/commands/simple-notification-routing.json", "internal/simplenotificationapi/simple_notification_routing_api_generated.go"},
		{"api/commands/simple-notification-history.json", "internal/simplenotificationapi/simple_notification_history_api_generated.go"},
	} {
		configData, err := os.ReadFile(repositoryPath(test.config))
		if err != nil {
			t.Fatal(err)
		}
		config, err := apigen.DecodeConfig(configData)
		if err != nil {
			t.Fatal(err)
		}
		want, err := apigen.Generate(config)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(repositoryPath(test.output))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("generated API commands are stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
		}
	}
}

func TestRunSimpleNotificationAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"simple-notification-api", "--help"},
			want: []string{"destination", "group", "routing", "history", "通知の到達は保証されず"},
		},
		{
			args: []string{"simple-notification-api", "destination", "--help"},
			want: []string{"list", "read", "create", "update", "delete", "read-status"},
		},
		{
			args: []string{"simple-notification-api", "destination", "create", "--help"},
			want: []string{"--request", "@path.json", "Type", "email", "webhook", "ServiceClass"},
		},
		{
			args: []string{"simple-notification-api", "group", "create", "--help"},
			want: []string{"Destinations", "通知先 ID の配列", "Provider.Class"},
		},
		{
			args: []string{"simple-notification-api", "group", "send-message", "--help"},
			want: []string{"GROUP-ID", "--message", "--request", "併用できません", "到達を保証しません"},
		},
		{
			args: []string{"simple-notification-api", "routing", "--help"},
			want: []string{"reorder", "list-source"},
		},
		{
			args: []string{"simple-notification-api", "history", "--help"},
			want: []string{"list", "read"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("run(%v) exit code = %d; stderr: %s", test.args, exitCode, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help output does not contain %q", text)
				}
			}
		})
	}
}
