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
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	"github.com/sacloud/skr/internal/apigen"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
	"github.com/stretchr/testify/require"
)

func TestSwitchGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-switch.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/switchapi/switch_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Switch commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-switch.json API_OUTPUT=internal/iaas/switchapi/switch_api_generated.go")
}

func TestRunIaaSAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{
			args: []string{"iaas-api", "--help"},
			want: []string{"switch", "SAKURA_ACCESS_TOKEN", "JSON"},
		},
		{
			args: []string{"iaas-api", "switch", "--help"},
			want: []string{"find", "read", "create", "update", "delete", "Zone"},
		},
		{
			args:    []string{"iaas-api", "switch", "find", "--help"},
			want:    []string{`"Zone":"tk1v"`, `"Names":["example"]`, "--tags", "カンマ区切り", "Sort", "有効な Key", "取得件数", "--zone", "併用不可"},
			notWant: []string{"--zone all"},
		},
		{
			args: []string{"iaas-api", "switch", "read", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, "作成結果", "@path.json", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "create", "--help"},
			want: []string{`"Zone":"tk1v"`, `"Name":"example"`, "--tags", "カンマ区切り", "必須", "NetworkMaskLen", "@path.json", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "update", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, "--tags", "カンマ区切り", "省略した項目は変更しません", "NetworkMaskLen", "--zone", "--id", "--name", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "delete", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, `"FailIfNotFound":true`, "対象が見つからない場合", "WaitForReleaseTimeout", "秒", "併用不可"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())

			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
			}
			for _, text := range test.notWant {
				require.NotContains(t, help, text, "help output contains removed text %q", text)
			}
		})
	}
}

func TestIaaSSwitchAPIWithLocalSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
			return swytch.New(server), nil
		})
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}

	var created iaas.Switch
	create := `{"Zone":"test-zone","Name":"TEST-SWITCH-NAME","Description":"Temporary switch for skr API tutorial"}`
	err := json.Unmarshal(runCommand("iaas-api", "switch", "create", "--request", create), &created)
	require.NoError(t, err)
	require.NotEqualValues(t, 0, created.ID, "create returned %#v, want created switch fields", created)
	require.Equal(t, "TEST-SWITCH-NAME", created.Name, "unexpected value")
	require.Equal(t, "Temporary switch for skr API tutorial", created.Description, "unexpected value")

	var found []*iaas.Switch
	err2 := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`), &found)
	require.NoError(t, err2)
	require.Len(t, found, 1, "find returned %#v, want switch %d", found, created.ID)
	require.Equal(t, created.ID, found[0].ID, "unexpected value")
	var projected []struct {
		ID   json.Number
		Name string
	}
	err3 := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`, "--query", "map({ID,Name})"), &projected)
	require.NoError(t, err3)
	require.Len(t, projected, 1, "projected output = %+v, want created switch ID and name", projected)
	require.Equal(t, created.ID.String(), projected[0].ID.String(), "unexpected value")
	require.Equal(t, created.Name, projected[0].Name, "unexpected value")

	var read iaas.Switch
	readRequestBytes, err := json.Marshal(map[string]any{"Zone": "test-zone", "ID": created.ID})
	require.NoError(t, err)
	readRequest := string(readRequestBytes)
	err4 := json.Unmarshal(runCommand("iaas-api", "switch", "read", "--request", readRequest), &read)
	require.NoError(t, err4)
	require.Equal(t, created.ID, read.ID, "read returned %#v, want switch %d", read, created.ID)
	require.Equal(t, created.Name, read.Name, "unexpected value")

	updateRequestBytes, err := json.Marshal(map[string]any{"Zone": "test-zone", "ID": created.ID, "Name": "updated-switch"})
	require.NoError(t, err)
	updateRequest := string(updateRequestBytes)
	var updated iaas.Switch
	err5 := json.Unmarshal(runCommand("iaas-api", "switch", "update", "--request", updateRequest), &updated)
	require.NoError(t, err5)
	require.Equal(t, "updated-switch", updated.Name, "update returned %#v, want updated name and unchanged description", updated)
	require.Equal(t, "Temporary switch for skr API tutorial", updated.Description, "unexpected value")

	output := runCommand("iaas-api", "switch", "delete", "--request", readRequest)
	require.Len(t, output, 0, "delete output = %s, want empty", output)
	found = nil
	err6 := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`), &found)
	require.NoError(t, err6)
	require.Len(t, found, 0, "find after delete returned %#v, want no switches", found)
	emptyJSON := string(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`, "--query", "map({ID,Name})"))
	want := "[]\n"
	require.Equal(t, want, emptyJSON, "empty projected output = %q, want %q", emptyJSON, want)
}

type switchFindRecordAPI struct {
	switchapi.API
	requests []swytch.FindRequest
}

func (api *switchFindRecordAPI) FindWithContext(_ context.Context, request *swytch.FindRequest) ([]*iaas.Switch, error) {
	api.requests = append(api.requests, *request)
	return []*iaas.Switch{{Name: request.Zone}}, nil
}

func TestGeneratedFindRejectsInvalidZoneBeforeCallingAPI(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "flag", args: []string{"--zone", ""}},
		{name: "all flag", args: []string{"--zone", "all"}},
		{name: "request", args: []string{"--request", `{"Zone":""}`}},
		{name: "all request", args: []string{"--request", `{"Zone":"all"}`}},
		{name: "missing JSON zone", args: []string{"--request", `{}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &switchFindRecordAPI{}
			factoryCalled := false
			commandLine := newCLI()
			setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
				factoryCalled = true
				return api, nil
			})

			var stdout, stderr bytes.Buffer
			args := append([]string{"iaas-api", "switch", "find"}, test.args...)
			exitCode := runCLI(args, &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, exitCode, "find with invalid zone: code %d, stdout %q, stderr %q; want validation error", exitCode, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
			require.False(t, factoryCalled, "invalid zone reached API setup: factory called=%t, requests=%+v", factoryCalled, api.requests)
			require.Len(t, api.requests, 0, "unexpected length")
		})
	}
}

func TestSwitchFlagInputs(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	call := func(args ...string) ([]byte, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return swytch.New(server), nil })
		code := runCLI(append([]string{"iaas-api", "switch"}, args...), &stdout, &stderr, commandLine)
		return stdout.Bytes(), stderr.String(), code
	}
	mustRun := func(args ...string) []byte {
		t.Helper()
		out, stderr, code := call(args...)
		require.Equal(t, 0, code, "%v: code %d, stderr %s", args, code, stderr)
		require.Empty(t, stderr, "should be empty")
		return out
	}
	var created iaas.Switch
	err := json.Unmarshal(mustRun("create", "--zone", "test-zone", "--name", "flag-switch", "--description", "created", "--tags", "production,web"), &created)
	require.NoError(t, err)
	if created.Name != "flag-switch" || created.Description != "created" ||
		len(created.Tags) != 2 || created.Tags[0] != "production" || created.Tags[1] != "web" {
		t.Fatalf("unexpected create result: %+v", created)
	}
	id := created.ID.String()
	var found []*iaas.Switch
	err2 := json.Unmarshal(mustRun("find", "--zone", "test-zone", "--tags", "production,web", "--count", "0", "--from", "0"), &found)
	require.NoError(t, err2)
	require.Len(t, found, 1, "unexpected find result: %+v", found)
	require.Equal(t, created.ID, found[0].ID, "unexpected value")
	var read iaas.Switch
	err3 := json.Unmarshal(mustRun("read", "--zone", "test-zone", "--id", id), &read)
	require.NoError(t, err3)
	require.Equal(t, created.ID, read.ID, "unexpected read result: %+v", read)
	var updated iaas.Switch
	err4 := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--name", "renamed", "--tags", "updated,cli"), &updated)
	require.NoError(t, err4)
	if updated.Name != "renamed" || updated.Description != "created" ||
		len(updated.Tags) != 2 || updated.Tags[0] != "cli" || updated.Tags[1] != "updated" {
		t.Fatalf("update lost an omitted value: %+v", updated)
	}
	err5 := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--description", ""), &updated)
	require.NoError(t, err5)
	if updated.Description != "" || updated.Name != "renamed" ||
		len(updated.Tags) != 2 || updated.Tags[0] != "cli" || updated.Tags[1] != "updated" {
		t.Fatalf("explicit empty description not applied: %+v", updated)
	}
	err6 := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--network-mask-len", "0"), &updated)
	require.NoError(t, err6)
	if updated.NetworkMaskLen != 0 || updated.Name != "renamed" || updated.Description != "" ||
		len(updated.Tags) != 2 || updated.Tags[0] != "cli" || updated.Tags[1] != "updated" {
		t.Fatalf("explicit zero network mask or omitted fields were not preserved: %+v", updated)
	}
	mustRun("delete", "--zone", "test-zone", "--id", id, "--fail-if-not-found")
	found = nil
	err7 := json.Unmarshal(mustRun("find", "--zone", "test-zone"), &found)
	require.NoError(t, err7)
	require.Len(t, found, 0, "deleted Switch still listed: %+v", found)
	for _, args := range [][]string{
		{"create", "--name", "missing-zone"},
		{"create", "--zone", "test-zone"},
		{"update", "--zone", "test-zone"},
		{"read", "--zone", "test-zone", "--id", "0"},
		{"delete", "--zone", "test-zone", "--id", "not-a-number"},
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":"test-zone"}`, "--count", "0"},
		{"find", "--request", `{"Zone":"test-zone"}`, "--tags", "test"},
		{"create", "--request", `{"Zone":"test-zone","Name":"json"}`, "--description", ""},
		{"create", "--request", `{"Zone":"test-zone","Name":"json"}`, "--tags", "test"},
		{"update", "--request", `{"Zone":"test-zone","ID":123}`, "--network-mask-len", "0"},
		{"update", "--request", `{"Zone":"test-zone","ID":123}`, "--tags", "test"},
		{"delete", "--request", `{"Zone":"test-zone","ID":123}`, "--fail-if-not-found=false"},
		{"read", "--zone", "all", "--id", "123"},
		{"create", "--zone", "all", "--name", "switch"},
		{"update", "--zone", "all", "--id", "123", "--name", "switch"},
		{"delete", "--zone", "all", "--id", "123"},
	} {
		out, stderr, code := call(args...)
		require.NotEqual(t, 0, code, "%v: code %d stdout %s stderr %s; want explicit error", args, code, out, stderr)
		require.Len(t, out, 0, "unexpected length")
		require.NotEmpty(t, stderr, "should not be empty")
	}
}

func TestSwitchCreateRequiresZoneAndName(t *testing.T) {
	for _, request := range []string{
		`{"Name":"missing-zone"}`,
		`{"Zone":"test-zone"}`,
	} {
		var stdout, stderr bytes.Buffer
		exitCode := run([]string{"iaas-api", "switch", "create", "--request", request}, &stdout, &stderr)
		require.NotEqual(t, 0, exitCode, "create with request %s succeeded, want validation error", request)

		require.EqualValues(t, 0, stdout.Len(), "create with request %s wrote stdout %q, want empty", request, stdout.String())
	}
}

func TestSwitchHelpJSONExamplesDecode(t *testing.T) {
	var find swytch.FindRequest
	err := decodeRequest(`{"Zone":"tk1v","Names":["example"],"Sort":[{"Key":"Name","Order":0}]}`, &find)
	require.NoError(t, err)
	if find.Zone != "tk1v" || len(find.Names) != 1 || find.Names[0] != "example" ||
		len(find.Sort) != 1 || find.Sort[0].Key != "Name" || find.Sort[0].Order != 0 {
		t.Fatalf("unexpected find request: %+v", find)
	}
	var create swytch.CreateRequest
	err2 := decodeRequest(`{"Zone":"tk1v","Name":"example","Tags":["test"]}`, &create)
	require.NoError(t, err2)
	require.Equal(t, "tk1v", create.Zone, "unexpected create request: %+v", create)
	require.Equal(t, "example", create.Name, "unexpected value")
	require.Len(t, create.Tags, 1, "unexpected length")
	require.Equal(t, "test", create.Tags[0], "unexpected value")
	var update swytch.UpdateRequest
	err3 := decodeRequest(`{"Zone":"tk1v","ID":123456789012,"Tags":["test"]}`, &update)
	require.NoError(t, err3)
	if update.Zone != "tk1v" || update.ID != 123456789012 || update.Tags == nil ||
		len(*update.Tags) != 1 || (*update.Tags)[0] != "test" {
		t.Fatalf("unexpected update request: %+v", update)
	}
}
