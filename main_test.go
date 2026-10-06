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

package main

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
)

func TestRunConfigCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name:       "production",
		Attributes: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profileOp.SetCurrentName("production"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "current"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "production\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigCurrentWithoutSelection(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "current"}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "current") {
		t.Errorf("stderr = %q, want an error mentioning the current profile", got)
	}
}

func TestRunTraceHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"--trace", "--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	for _, text := range []string{"--trace", "認証情報"} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("help output does not contain %q", text)
		}
	}
}

func TestRunEventbusAPIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"eventbus-api", "--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	for _, text := range []string{
		"process-configuration list",
		"process-configuration update-secret",
		"schedule list",
		"trigger list",
		"ベストエフォート型",
	} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("help output does not contain %q", text)
		}
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
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("run(%v) exit code = %d; stderr: %s", test.args, exitCode, stderr.String())
			}
			for _, text := range test.want {
				if !strings.Contains(stdout.String(), text) {
					t.Errorf("help output does not contain %q", text)
				}
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

func TestDecodeRequest(t *testing.T) {
	type request struct {
		Name string
	}

	var inline request
	if err := decodeRequest(`{"Name":"inline"}`, &inline); err != nil {
		t.Fatal(err)
	}
	if inline.Name != "inline" {
		t.Errorf("inline Name = %q, want inline", inline.Name)
	}

	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(`{"Name":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var fromFile request
	if err := decodeRequest("@"+path, &fromFile); err != nil {
		t.Fatal(err)
	}
	if fromFile.Name != "file" {
		t.Errorf("file Name = %q, want file", fromFile.Name)
	}
}

func TestDecodeRequestInvalidJSON(t *testing.T) {
	var got struct{}
	if err := decodeRequest("{", &got); err == nil || !strings.Contains(err.Error(), "decode request JSON") {
		t.Fatalf("decodeRequest() error = %v, want JSON decoding error", err)
	}
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
		if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("run(%v) exit code = %d; stderr: %s", args, exitCode, stderr.String())
		}
		return stdout.Bytes()
	}

	createJSON := `{"CommonServiceItem":{"Name":"sakumock-process-configuration","Settings":{"Destination":"simplemq","Parameters":"{\"queue_name\":\"sakumock\",\"content\":\"created\"}"}}}`
	var created v1.CommonServiceItem
	if err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "create", "--request", createJSON), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("create returned an empty ID")
	}
	if got, want := created.Provider.Class, v1.ProviderClassEventbusprocessconfiguration; got != want {
		t.Errorf("created process configuration provider class = %q, want %q", got, want)
	}

	var items []v1.CommonServiceItem
	if err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "list"), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("list returned %#v, want the created process configuration", items)
	}

	for _, resource := range []string{"schedule", "trigger"} {
		items = nil
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &items); err != nil {
			t.Fatalf("decode %s list: %v", resource, err)
		}
		if len(items) != 0 {
			t.Errorf("%s list returned %#v, want no resources", resource, items)
		}
	}

	exerciseResource := func(resource, name, updatedName string, providerClass v1.ProviderClass, settings any) {
		t.Helper()
		requestJSON, err := json.Marshal(map[string]any{
			"CommonServiceItem": map[string]any{
				"Name":     name,
				"Settings": settings,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		var item v1.CommonServiceItem
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "create", "--request", string(requestJSON)), &item); err != nil {
			t.Fatalf("create %s: %v", resource, err)
		}
		if item.ID == "" {
			t.Fatalf("create %s returned an empty ID", resource)
		}
		if got, want := item.Provider.Class, providerClass; got != want {
			t.Errorf("created %s provider class = %q, want %q", resource, got, want)
		}

		var listed []v1.CommonServiceItem
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &listed); err != nil {
			t.Fatalf("list %s: %v", resource, err)
		}
		if len(listed) != 1 || listed[0].ID != item.ID {
			t.Fatalf("%s list returned %#v, want created resource %q", resource, listed, item.ID)
		}

		var read v1.CommonServiceItem
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "read", item.ID), &read); err != nil {
			t.Fatalf("read %s: %v", resource, err)
		}
		if read.ID != item.ID {
			t.Errorf("%s read ID = %q, want %q", resource, read.ID, item.ID)
		}

		updateJSON, err := json.Marshal(map[string]any{
			"CommonServiceItem": map[string]any{"Name": updatedName},
		})
		if err != nil {
			t.Fatal(err)
		}
		var updated v1.CommonServiceItem
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "update", item.ID, "--request", string(updateJSON)), &updated); err != nil {
			t.Fatalf("update %s: %v", resource, err)
		}
		if updated.Name != updatedName {
			t.Errorf("%s update Name = %q, want %q", resource, updated.Name, updatedName)
		}

		runCommand("eventbus-api", resource, "delete", item.ID)
		listed = nil
		if err := json.Unmarshal(runCommand("eventbus-api", resource, "list"), &listed); err != nil {
			t.Fatalf("list %s after delete: %v", resource, err)
		}
		if len(listed) != 0 {
			t.Errorf("%s list after delete returned %#v, want no resources", resource, listed)
		}
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
	if err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "read", created.ID), &read); err != nil {
		t.Fatal(err)
	}
	if read.ID != created.ID {
		t.Errorf("read ID = %q, want %q", read.ID, created.ID)
	}

	updateJSON := `{"CommonServiceItem":{"Name":"updated-process-configuration"}}`
	var updated v1.CommonServiceItem
	if err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "update", created.ID, "--request", updateJSON), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "updated-process-configuration" {
		t.Errorf("updated Name = %q, want updated-process-configuration", updated.Name)
	}

	secretPath := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"AccessToken":"mock-token","AccessTokenSecret":"mock-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommand("eventbus-api", "process-configuration", "update-secret", created.ID, "--secret-file", secretPath)
	storedSecret, ok := server.Secret(created.ID)
	if !ok || !bytes.Contains(storedSecret, []byte("mock-token")) {
		t.Errorf("stored secret = %s, want mock-token", storedSecret)
	}

	runCommand("eventbus-api", "process-configuration", "delete", created.ID)
	items = nil
	if err := json.Unmarshal(runCommand("eventbus-api", "process-configuration", "list"), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("list after delete returned %#v, want no resources", items)
	}
}
