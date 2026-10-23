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

	apprunmock "github.com/sacloud/sakumock/apprun"
)

func TestRunAppRunAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"apprun-api", "--help"},
			want: []string{"application", "version", "packet-filter", "traffic", "user", "AppRun 共用型", "保護したファイル", "manual.sakura.ad.jp/cloud/apprun/about.html"},
		},
		{
			args: []string{"apprun-api", "application", "create", "--help"},
			want: []string{"CreateApplicationBody", "--request", "@path.json", "標準入力", "認証情報"},
		},
		{
			args: []string{"apprun-api", "application", "list", "--help"},
			want: []string{"--page-num", "--page-size", "--sort-field", "--sort-order"},
		},
		{
			args: []string{"apprun-api", "traffic", "update", "--help"},
			want: []string{"UpdateTrafficBody", "version_name", "percent", "最新バージョン"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) code = %d; stderr: %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				if !strings.Contains(help, expected) {
					t.Errorf("help does not contain %q:\n%s", expected, help)
				}
			}
		})
	}
}

func TestRunAppRunAPIRejectsInlineSensitiveCreateRequest(t *testing.T) {
	const secret = "test-only-apprun-secret"
	args := []string{"apprun-api", "application", "create", "--request",
		`{"components":[{"secret":[{"key":"TOKEN","value":"` + secret + `"}]}]}`}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("inline application request succeeded")
	}
	if strings.Contains(stderr.String(), secret) || !strings.Contains(stderr.String(), "@path.json") {
		t.Fatalf("error = %q, want safe file-input guidance without secret", stderr.String())
	}
}

func TestRunAppRunAPIRejectsInlineSensitiveUpdateRequest(t *testing.T) {
	const secret = "test-only-apprun-secret"
	args := []string{"apprun-api", "application", "update", "app-id", "--request",
		`{"components":[{"secret":[{"key":"TOKEN","value":"` + secret + `"}]}]}`}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("inline application update succeeded")
	}
	if strings.Contains(stderr.String(), secret) || !strings.Contains(stderr.String(), "@path.json") {
		t.Fatalf("error = %q, want safe file-input guidance without secret", stderr.String())
	}
}

func TestRunAppRunAPIRejectsInvalidSortOrder(t *testing.T) {
	for _, args := range [][]string{
		{"apprun-api", "application", "list", "--sort-order", "invalid"},
		{"apprun-api", "version", "list", "--application-id", "app-id", "--sort-order", "invalid"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 {
			t.Errorf("run(%v) succeeded with invalid sort order", args)
		}
		if !strings.Contains(stderr.String(), "sort-order must be asc or desc") {
			t.Errorf("run(%v) error = %q, want sort-order validation", args, stderr.String())
		}
	}
}

func TestAppRunAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_SHARED", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) code = %d; stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() > 0 {
			t.Errorf("run(%v) stderr = %q", args, stderr.String())
		}
		return stdout.Bytes()
	}

	requestFile := filepath.Join(t.TempDir(), "create.json")
	request := `{"name":"mock-apprun","timeout_seconds":60,"port":8080,"min_scale":0,"max_scale":1,"scale_target_concurrency":100,"components":[{"name":"web","max_cpu":"0.5","max_memory":"1Gi","deploy_source":{"container_registry":{"image":"ghcr.io/sacloud/apprun-samples/tutorial:latest"}},"secret":[{"key":"TOKEN","value":"test-only-apprun-secret"}]}]}`
	if err := os.WriteFile(requestFile, []byte(request), 0o600); err != nil {
		t.Fatal(err)
	}
	createdData := runCommand("apprun-api", "application", "create", "--request", "@"+requestFile)
	if bytes.Contains(createdData, []byte("test-only-apprun-secret")) {
		t.Fatalf("application create output exposed a secret: %s", createdData)
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "mock-apprun" {
		t.Fatalf("created application = %+v", created)
	}

	var listed struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runCommand("apprun-api", "application", "list", "--page-num", "1", "--page-size", "10"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.ID || listed.Data[0].Name != created.Name {
		t.Fatalf("application list = %+v, want created application", listed)
	}
	runCommand("apprun-api", "application", "read", created.ID)
	runCommand("apprun-api", "application", "status", created.ID)
	runCommand("apprun-api", "traffic", "list", created.ID)
	runCommand("apprun-api", "packet-filter", "read", created.ID)

	updateFile := filepath.Join(t.TempDir(), "update.json")
	if err := os.WriteFile(updateFile, []byte(`{"max_scale":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommand("apprun-api", "application", "update", created.ID, "--request", "@"+updateFile)
	if err := os.WriteFile(updateFile, []byte(`{"min_scale":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommand("apprun-api", "application", "update", created.ID, "--request", "@"+updateFile)

	var versions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runCommand("apprun-api", "version", "list", "--application-id", created.ID,
		"--sort-field", "created_at", "--sort-order", "asc"), &versions); err != nil {
		t.Fatal(err)
	}
	if len(versions.Data) < 2 || versions.Data[0].ID == "" {
		t.Fatalf("version list = %+v, want two versions created by application updates", versions)
	}
	versionID := versions.Data[0].ID
	runCommand("apprun-api", "version", "read", "--application-id", created.ID, "--version-id", versionID)
	runCommand("apprun-api", "version", "status", "--application-id", created.ID, "--version-id", versionID)
	runCommand("apprun-api", "traffic", "update", created.ID, "--request", `[{"is_latest_version":true,"percent":100}]`)
	runCommand("apprun-api", "version", "delete", "--application-id", created.ID, "--version-id", versionID)
	runCommand("apprun-api", "packet-filter", "update", created.ID, "--request", `{"is_enabled":false,"settings":[]}`)

	runCommand("apprun-api", "user", "create")
	runCommand("apprun-api", "user", "read")
	runCommand("apprun-api", "application", "delete", created.ID)
	var remaining []any
	if err := json.Unmarshal(runCommand("apprun-api", "application", "list",
		"--query", ".data | map(select(.id == \""+created.ID+"\"))"), &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("deleted application remains in list: %+v", remaining)
	}
}
