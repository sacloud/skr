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
		{"api/commands/simple-notification-destination.json", "internal/simplenotificationapi/simple_notification_destination_api_generated.go"},
		{"api/commands/simple-notification-group.json", "internal/simplenotificationapi/simple_notification_group_api_generated.go"},
		{"api/commands/simple-notification-routing.json", "internal/simplenotificationapi/simple_notification_routing_api_generated.go"},
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
		args := []string{"simple-notification-api", "destination", "list"}
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
	for _, operation := range []string{"create", "update"} {
		data := `{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"` + webhookURL + `","Value":"webhook"}}}`
		err := os.WriteFile(path, []byte(data), 0o600)
		require.NoError(t, err)
		args := []string{"simple-notification-api", "destination", operation}
		if operation == "update" {
			args = append(args, "123456789012")
		}
		args = append(args, "--request", "@"+path)
		var output, stderr bytes.Buffer
		code := run(args, &output, &stderr)
		require.EqualValues(t, 1, code, "%s exit = %d, want 1", operation, code)
		if strings.Contains(stderr.String()+output.String(), webhookURL) || stderr.Len() == 0 {
			t.Fatalf("%s did not return a nonempty safe error", operation)
		}
	}
	require.EqualValues(t, 0, calls, "invalid request reached the server %d times", calls)
}
