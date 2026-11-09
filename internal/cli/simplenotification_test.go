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
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestSimpleNotificationGeneratedCodeMatchesConfig(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{"api/commands/simple-notification.json", "internal/simplenotificationapi/simple_notification_common_service_item_api_generated.go"},
		{"api/commands/simple-notification-history.json", "internal/simplenotificationapi/simple_notification_history_api_generated.go"},
	} {
		configData, err := os.ReadFile(repositoryPath(test.config))
		require.NoError(t, err)
		config, err := apigen.DecodeConfig(configData, apigen.ModeOgen)
		require.NoError(t, err)
		want, err := apigen.Generate(config, apigen.ModeOgen)
		require.NoError(t, err)
		got, err := os.ReadFile(repositoryPath(test.output))
		require.NoError(t, err)
		require.Equal(t, want, got, "generated API commands are stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
	}
}

func TestRunSimpleNotificationAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"simple-notification-api", "--help"},
			want: []string{
				"list-common-service-items", "read-common-service-item", "create-common-service-item",
				"update-common-service-item", "delete-common-service-item",
				"check-notification-destination-status", "send-notification", "reorder-notification-routing",
				"list-notification-sources", "list-notification-histories", "read-notification-history",
				"通知の到達は保証されず",
			},
		},
		{
			args: []string{"simple-notification-api", "list-common-service-items", "--help"},
			want: []string{"--provider-class", "saknoticedestination", "saknoticegroup", "saknoticerouting"},
		},
		{
			args: []string{"simple-notification-api", "create-common-service-item", "--help"},
			want: []string{"--provider-class", "--request", "@path.json", "Type", "email", "webhook", "Provider.Class", "ServiceClass"},
		},
		{
			args: []string{"simple-notification-api", "update-common-service-item", "--help"},
			want: []string{"--provider-class", "--request", "@path.json", "Name、Description、Tags、Icon", "通知先アドレスは変更できません"},
		},
		{
			args: []string{"simple-notification-api", "send-notification", "--help"},
			want: []string{"--message", "--request", "併用できません", "到達を保証しません", "--provider-class は不要"},
		},
		{
			args: []string{"simple-notification-api", "check-notification-destination-status", "--help"},
			want: []string{"通知先専用の操作", "--provider-class は不要"},
		},
		{
			args: []string{"simple-notification-api", "reorder-notification-routing", "--help"},
			want: []string{"--request", "@path.json", "ルーティング設定専用の操作"},
		},
		{
			args: []string{"simple-notification-api", "list-notification-histories", "--help"},
			want: []string{"通知履歴を一覧表示します"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())

			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
			}
		})
	}
}

func TestSimpleNotificationTraceUsesParsedFlag(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")
	t.Setenv("SAKURA_TRACE_MODE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"From":0,"Count":0,"Total":0,"CommonServiceItems":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_SIMPLE_NOTIFICATION", server.URL+"/")
	previous := log.Writer()
	t.Cleanup(func() { log.SetOutput(previous) })
	for _, trace := range []bool{false, true} {
		var output, stderr, logs bytes.Buffer
		log.SetOutput(&logs)
		args := []string{"simple-notification-api", "list-common-service-items", "--provider-class", "saknoticedestination"}
		if trace {
			args = append([]string{"--trace"}, args...)
		}
		require.Equal(t, 0, run(args, &output, &stderr), "stderr: %s", stderr.String())
		if strings.Contains(logs.String(), "[TRACE]") != trace {
			t.Fatalf("trace output enabled = %v, want %v", strings.Contains(logs.String(), "[TRACE]"), trace)
		}
	}
}

func TestDestinationSDKValidationDoesNotExposeFileValues(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")
	t.Setenv("SAKURA_TRACE_MODE", "")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_SIMPLE_NOTIFICATION", server.URL+"/")
	const webhookURL = "https://example.invalid/private-webhook-value"
	path := filepath.Join(t.TempDir(), "request.json")
	for _, test := range []struct {
		name string
		args []string
		data string
	}{
		{
			name: "create",
			args: []string{"simple-notification-api", "create-common-service-item", "--provider-class", "saknoticedestination"},
			data: `{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"` + webhookURL + `","Value":"webhook"}}}`,
		},
		{
			name: "update",
			args: []string{"simple-notification-api", "update-common-service-item", "--provider-class", "saknoticedestination", "123456789012"},
			data: `{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"` + webhookURL + `","Value":"webhook"}}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := os.WriteFile(path, []byte(test.data), 0o600)
			require.NoError(t, err)
			args := append(test.args, "--request", "@"+path)
			var output, stderr bytes.Buffer
			code := run(args, &output, &stderr)
			require.EqualValues(t, 1, code, "exit = %d, want 1", code)
			require.NotContains(t, stderr.String()+output.String(), webhookURL, "error must not contain the protected address")
			require.NotEmpty(t, stderr.String(), "error output is empty")
		})
	}
	require.EqualValues(t, 0, calls, "invalid request reached the server %d times", calls)
}

func TestDestinationInlineRequestsAreRejected(t *testing.T) {
	const webhookURL = "https://example.invalid/private-webhook-value"
	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "create",
			args: []string{"simple-notification-api", "create-common-service-item", "--provider-class", "saknoticedestination",
				"--request", `{"CommonServiceItem":{"Name":"test","Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}}`},
		},
		{
			name: "update with settings",
			args: []string{"simple-notification-api", "update-common-service-item", "--provider-class", "saknoticedestination", "123456789012",
				"--request", `{"CommonServiceItem":{"Name":"test","Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}}`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(test.args, &stdout, &stderr)
			require.EqualValues(t, 1, code, "exit = %d, want 1", code)
			combined := stderr.String() + stdout.String()
			require.NotContains(t, combined, webhookURL, "error must not contain the protected address")
			require.Contains(t, combined, "@path.json")
		})
	}
}

func TestCommonServiceItemRequiresProviderClass(t *testing.T) {
	for _, args := range [][]string{
		{"simple-notification-api", "list-common-service-items"},
		{"simple-notification-api", "create-common-service-item", "--request", "{}"},
		{"simple-notification-api", "update-common-service-item", "123456789012", "--request", "{}"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		require.EqualValues(t, 1, code, "args = %v, exit = %d, want 1", args, code)
		combined := stderr.String() + stdout.String()
		require.Contains(t, combined, "--provider-class")
	}
}
