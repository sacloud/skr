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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	eventbusmock "github.com/sacloud/sakumock/eventbus"
	"github.com/sacloud/skr/version"
	"github.com/stretchr/testify/require"
)

func repositoryPath(path string) string {
	return filepath.Join("..", "..", path)
}

func TestRunVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
			if got, want := stdout.String(), version.FullVersion()+"\n"; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
			got := stderr.String()
			require.Empty(t, got, "stderr = %q, want empty", got)
		})
	}
}

func TestRunWithoutArgumentsShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(nil, &stdout, &stderr), "stderr: %s", stderr.String())
	for _, text := range []string{"Usage: skr <command> [flags]", "Commands:", "version", "config"} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}

func TestRunIncompleteCommandShowsHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iam-api"}, want: []string{"Usage: skr iam-api", "user", "group"}},
		{args: []string{"iaas-api"}, want: []string{"Usage: skr iaas-api", "switch", "server"}},
		{args: []string{"iaas-api", "switch"}, want: []string{"Usage: skr iaas-api switch", "find", "create"}},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			for _, text := range test.want {
				require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
			}
			got := stderr.String()
			require.Empty(t, got, "stderr = %q, want empty", got)
		})
	}
}

func TestRunUnknownCommandDoesNotShowHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"iam-api", "unknown"}, &stdout, &stderr), "run should fail")
	require.NotContains(t, stdout.String(), "Usage:", "stdout = %q, stderr = %q; want an unknown-command error without help", stdout.String(), stderr.String())
	require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
}

func TestRunConfigCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "production",
		Attributes: map[string]any{},
	}))
	err = profileOp.SetCurrentName("production")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "current"}, &stdout, &stderr), "stderr: %s", stderr.String())
	if got, want := stdout.String(), "production\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}

func TestRunConfigCurrentWithoutSelection(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "current"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Contains(t, got2, "current", "stderr = %q, want an error mentioning the current profile", got2)
}

func TestRunTraceHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"--trace", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())
	for _, text := range []string{"--trace", "認証情報"} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
}

func TestRunEventbusAPIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"eventbus-api", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())
	for _, text := range []string{
		"process-configuration list",
		"process-configuration update-secret",
		"schedule list",
		"trigger list",
		"ベストエフォート型",
	} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
}

func TestRunEventbusResourceHelpExplainsRequest(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"eventbus-api", "process-configuration", "create", "--help"},
			want: []string{"Destination", "simplemq", "Parameters", "Provider"},
		},
		{
			args: []string{"eventbus-api", "schedule", "create", "--help"},
			want: []string{"ProcessConfigurationID", "StartsAt", "RecurringUnit", "RecurringStep", "1893456000000"},
		},
		{
			args: []string{"eventbus-api", "trigger", "create", "--help"},
			want: []string{"Source", "Types", "ProcessConfigurationID", "EVENT-SOURCE", "EVENT-TYPE"},
		},
		{
			args: []string{"eventbus-api", "process-configuration", "update-secret", "--help"},
			want: []string{"--secret-file", "APIKey", "AccessTokenSecret"},
		},
		{
			args: []string{"eventbus-api", "process-configuration", "update", "--help"},
			want: []string{"Destination", "Parameters", "指定", "null"},
		},
		{
			args: []string{"eventbus-api", "schedule", "update", "--help"},
			want: []string{"部分更新", "Settings", "Description"},
		},
		{
			args: []string{"eventbus-api", "trigger", "update", "--help"},
			want: []string{"Source", "Types", "ProcessConfigurationID"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			for _, text := range test.want {
				require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
			}
		})
	}
}

func TestRunEventbusResourceHelpExplainsWorkflow(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"eventbus-api", "--help"},
			want: []string{"process-configuration list", "schedule list", "trigger list", "SAKURA_ACCESS_TOKEN", "JSON"},
		},
		{
			args: []string{"eventbus-api", "process-configuration", "--help"},
			want: []string{"実行先サービス", "schedule または trigger"},
		},
		{
			args: []string{"eventbus-api", "schedule", "--help"},
			want: []string{"process-configuration を作成", "StartsAt", "Unix epoch"},
		},
		{
			args: []string{"eventbus-api", "trigger", "--help"},
			want: []string{"イベントソース", "process-configuration", "EVENT-SOURCE"},
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

func TestDecodeRequest(t *testing.T) {
	type request struct {
		Name string
	}

	var inline request
	err := decodeRequest(`{"Name":"inline"}`, &inline)
	require.NoError(t, err)
	require.Equal(t, "inline", inline.Name, "inline Name = %q, want inline", inline.Name)

	path := filepath.Join(t.TempDir(), "request.json")
	err2 := os.WriteFile(path, []byte(`{"Name":"file"}`), 0o600)
	require.NoError(t, err2)
	var fromFile request
	err3 := decodeRequest("@"+path, &fromFile)
	require.NoError(t, err3)
	require.Equal(t, "file", fromFile.Name, "file Name = %q, want file", fromFile.Name)
}

func TestDecodeRequestInvalidJSON(t *testing.T) {
	var got struct{}
	err := decodeRequest("{", &got)
	require.Error(t, err, "decodeRequest() error = %v, want JSON decoding error", err)
	require.Contains(t, err.Error(), "decode request JSON", "missing expected text %q", "decode request JSON")
}

func TestEventbusAPIProcessConfigurationWithSakumock(t *testing.T) {
	server := eventbusmock.NewTestServer(eventbusmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_EVENTBUS", server.TestURL()+"/")

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	checkProjection := func(resource, id, name string) {
		t.Helper()
		output := runCommand("eventbus-api", resource, "list", "--query", "map({ID,Name})")
		var projected []struct {
			ID   string
			Name string
		}
		err := json.Unmarshal(output, &projected)
		require.NoError(t, err)
		require.Len(t, projected, 1, "%s projection = %s, want ID %s and name %s", resource, output, id, name)
		require.Equal(t, id, projected[0].ID, "unexpected value")
		require.Equal(t, name, projected[0].Name, "unexpected value")
	}

	createJSON := `{"CommonServiceItem":{"Name":"sakumock-process-configuration","Settings":{"Destination":"simplemq","Parameters":"{\"queue_name\":\"sakumock\",\"content\":\"created\"}"}}}`
	var created v1.CommonServiceItem
	err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "create", "--request", createJSON), &created)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID, "create returned an empty ID")
	if got, want := created.Provider.Class, v1.ProviderClassEventbusprocessconfiguration; got != want {
		t.Errorf("created process configuration provider class = %q, want %q", got, want)
	}

	var items []v1.CommonServiceItem
	err2 := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "list"), &items)
	require.NoError(t, err2)
	require.Len(t, items, 1, "list returned %#v, want the created process configuration", items)
	require.Equal(t, created.ID, items[0].ID, "unexpected value")
	checkProjection("process-configuration", created.ID, created.Name)

	for _, resource := range []string{"schedule", "trigger"} {
		items = nil
		err3 := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &items)
		require.NoError(t, err3, "decode %s list", resource)
		require.Len(t, items, 0, "%s list returned %#v, want no resources", resource, items)
		got := string(runCommand("eventbus-api", resource, "list", "--query", "map({ID,Name})"))
		require.Equal(t, "[]\n", got, "%s empty projection = %q, want []", resource, got)
	}

	exerciseResource := func(resource, name, updatedName string, providerClass v1.ProviderClass, settings any) {
		t.Helper()
		requestJSON, err := json.Marshal(map[string]any{
			"CommonServiceItem": map[string]any{
				"Name":     name,
				"Settings": settings,
			},
		})
		require.NoError(t, err)
		var item v1.CommonServiceItem
		err4 := json.Unmarshal(runCommand("eventbus-api", resource, "create", "--request", string(requestJSON)), &item)
		require.NoError(t, err4, "create %s", resource)
		require.NotEmpty(t, item.ID, "create %s returned an empty ID", resource)
		if got, want := item.Provider.Class, providerClass; got != want {
			t.Errorf("created %s provider class = %q, want %q", resource, got, want)
		}

		var listed []v1.CommonServiceItem
		err5 := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &listed)
		require.NoError(t, err5, "list %s", resource)
		require.Len(t, listed, 1, "%s list returned %#v, want created resource %q", resource, listed, item.ID)
		require.Equal(t, item.ID, listed[0].ID, "unexpected value")
		checkProjection(resource, item.ID, item.Name)

		var read v1.CommonServiceItem
		err6 := json.Unmarshal(runCommand("eventbus-api", resource, "read", item.ID), &read)
		require.NoError(t, err6, "read %s", resource)
		require.Equal(t, item.ID, read.ID, "%s read ID = %q, want %q", resource, read.ID, item.ID)

		updateJSON, err := json.Marshal(map[string]any{
			"CommonServiceItem": map[string]any{"Name": updatedName},
		})
		require.NoError(t, err)
		var updated v1.CommonServiceItem
		err7 := json.Unmarshal(runCommand("eventbus-api", resource, "update", item.ID, "--request", string(updateJSON)), &updated)
		require.NoError(t, err7, "update %s", resource)
		require.Equal(t, updatedName, updated.Name, "%s update Name = %q, want %q", resource, updated.Name, updatedName)

		runCommand("eventbus-api", resource, "delete", item.ID)
		listed = nil
		err8 := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &listed)
		require.NoError(t, err8, "list %s after delete", resource)
		require.Len(t, listed, 0, "%s list after delete returned %#v, want no resources", resource, listed)
	}

	exerciseResource("schedule", "sakumock-schedule", "updated-sakumock-schedule", v1.ProviderClassEventbusschedule, map[string]any{
		"ProcessConfigurationID": created.ID,
		"StartsAt":               1700000000000,
		"RecurringStep":          1,
		"RecurringUnit":          "day",
	})
	exerciseResource("trigger", "sakumock-trigger", "updated-sakumock-trigger", v1.ProviderClassEventbustrigger, map[string]any{
		"ProcessConfigurationID": created.ID,
		"Source":                 "//sakumock/source",
		"Types":                  []string{"sakumock.created"},
	})

	var read v1.CommonServiceItem
	err9 := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "read", created.ID), &read)
	require.NoError(t, err9)
	require.Equal(t, created.ID, read.ID, "read ID = %q, want %q", read.ID, created.ID)

	updateJSON := `{"CommonServiceItem":{"Name":"updated-process-configuration"}}`
	var updated v1.CommonServiceItem
	err10 := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "update", created.ID, "--request", updateJSON), &updated)
	require.NoError(t, err10)
	require.Equal(t, "updated-process-configuration", updated.Name, "updated Name = %q, want updated-process-configuration", updated.Name)

	secretPath := filepath.Join(t.TempDir(), "secret.json")
	err11 := os.WriteFile(secretPath, []byte(`{"AccessToken":"mock-token","AccessTokenSecret":"mock-secret"}`), 0o600)
	require.NoError(t, err11)
	runCommand("eventbus-api", "process-configuration", "update-secret", created.ID, "--secret-file", secretPath)
	storedSecret, ok := server.Secret(created.ID)
	if !ok || !bytes.Contains(storedSecret, []byte("mock-token")) {
		t.Errorf("stored secret = %s, want mock-token", storedSecret)
	}

	runCommand("eventbus-api", "process-configuration", "delete", created.ID)
	items = nil
	err12 := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "list"), &items)
	require.NoError(t, err12)
	require.Len(t, items, 0, "list after delete returned %#v, want no resources", items)
}
