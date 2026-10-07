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
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	"github.com/sacloud/skr/internal/apigen"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	"github.com/sacloud/skr/internal/iaas/zones"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

func TestSwitchGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-switch.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/switchapi/switch_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Switch commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-switch.json API_OUTPUT=internal/iaas/switchapi/switch_api_generated.go")
	}
}

func TestRunIaaSAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"iaas-api", "--help"},
			want: []string{"switch", "SAKURA_ACCESS_TOKEN", "JSON"},
		},
		{
			args: []string{"iaas-api", "switch", "--help"},
			want: []string{"find", "read", "create", "update", "delete", "Zone", "全ゾーン"},
		},
		{
			args: []string{"iaas-api", "switch", "find", "--help"},
			want: []string{`"Zone":"tk1v"`, `"Names":["example"]`, "Tags", "Sort", "有効な Key", "取得件数", "--zone all", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "read", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, "作成結果", "@path.json", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "create", "--help"},
			want: []string{`"Zone":"tk1v"`, `"Name":"example"`, `"Tags":["test"]`, "必須", "NetworkMaskLen", "@path.json", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "update", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, `"Tags":["test"]`, "省略した項目は変更しません", "NetworkMaskLen", "--zone", "--id", "--name", "併用不可"},
		},
		{
			args: []string{"iaas-api", "switch", "delete", "--help"},
			want: []string{`"Zone":"tk1v"`, `"ID":123456789012`, `"FailIfNotFound":true`, "対象が見つからない場合", "WaitForReleaseTimeout", "秒", "併用不可"},
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

func TestIaaSSwitchAPIWithLocalSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
			return swytch.New(server), nil
		})
		if exitCode := runCLI(args, &stdout, &stderr, commandLine); exitCode != 0 {
			t.Fatalf("run(%v) exit code = %d; stderr: %s", args, exitCode, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.Bytes()
	}

	var created iaas.Switch
	create := `{"Zone":"test-zone","Name":"TEST-SWITCH-NAME","Description":"Temporary switch for skr API tutorial"}`
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "create", "--request", create), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "TEST-SWITCH-NAME" || created.Description != "Temporary switch for skr API tutorial" {
		t.Fatalf("create returned %#v, want created switch fields", created)
	}

	var found []*iaas.Switch
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("find returned %#v, want switch %d", found, created.ID)
	}
	var projected []struct {
		ID   json.Number
		Name string
	}
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`, "--query", "map({ID,Name})"), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID.String() != created.ID.String() || projected[0].Name != created.Name {
		t.Fatalf("projected output = %+v, want created switch ID and name", projected)
	}

	var read iaas.Switch
	readRequestBytes, err := json.Marshal(map[string]any{"Zone": "test-zone", "ID": created.ID})
	if err != nil {
		t.Fatal(err)
	}
	readRequest := string(readRequestBytes)
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "read", "--request", readRequest), &read); err != nil {
		t.Fatal(err)
	}
	if read.ID != created.ID || read.Name != created.Name {
		t.Errorf("read returned %#v, want switch %d", read, created.ID)
	}

	updateRequestBytes, err := json.Marshal(map[string]any{"Zone": "test-zone", "ID": created.ID, "Name": "updated-switch"})
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := string(updateRequestBytes)
	var updated iaas.Switch
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "update", "--request", updateRequest), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "updated-switch" || updated.Description != "Temporary switch for skr API tutorial" {
		t.Errorf("update returned %#v, want updated name and unchanged description", updated)
	}

	if output := runCommand("iaas-api", "switch", "delete", "--request", readRequest); len(output) != 0 {
		t.Errorf("delete output = %s, want empty", output)
	}
	found = nil
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("find after delete returned %#v, want no switches", found)
	}
	emptyJSON := string(runCommand("iaas-api", "switch", "find", "--request", `{"Zone":"test-zone"}`, "--query", "map({ID,Name})"))
	if want := "[]\n"; emptyJSON != want {
		t.Errorf("empty projected output = %q, want %q", emptyJSON, want)
	}
}

func TestIaaSSwitchFindAllZones(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
			return swytch.New(server), nil
		})
		commandLine.IaaSAPI.Switch.SetZoneFactory(func() (zones.API, error) {
			return zone.New(server), nil
		})
		if exitCode := runCLI(args, &stdout, &stderr, commandLine); exitCode != 0 {
			t.Fatalf("run(%v) exit code = %d; stderr: %s", args, exitCode, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.Bytes()
	}

	for _, entry := range []struct {
		zone string
		name string
	}{
		{zone: "test-zone-a", name: "switch-a"},
		{zone: "test-zone-b", name: "switch-b"},
	} {
		runCommand("iaas-api", "switch", "create", "--zone", entry.zone, "--name", entry.name)
	}

	var found []*iaas.Switch
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--zone", "all"), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].Name != "switch-a" || found[1].Name != "switch-b" {
		t.Errorf("find all returned %#v, want one result from each configured zone", found)
	}
	var projected []map[string]json.RawMessage
	if err := json.Unmarshal(runCommand("iaas-api", "switch", "find", "--zone", "all", "--query", "map({ID,Name})"), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 2 {
		t.Fatalf("projected all-zone output = %+v, want two results", projected)
	}
	for i, item := range projected {
		if len(item) != 2 || string(item["Name"]) != `"`+found[i].Name+`"` {
			t.Errorf("projected result %d = %+v, want only ID and Name", i, item)
		}
	}
}

type switchFindRecordAPI struct {
	switchapi.API
	requests []swytch.FindRequest
	failZone string
}

func (api *switchFindRecordAPI) FindWithContext(_ context.Context, request *swytch.FindRequest) ([]*iaas.Switch, error) {
	api.requests = append(api.requests, *request)
	if request.Zone == api.failZone {
		return nil, errors.New("mock search failure")
	}
	return []*iaas.Switch{{Name: request.Zone}}, nil
}

func TestGeneratedFindAllZonesAppliesFiltersAndAvoidsPartialOutput(t *testing.T) {
	run := func(api *switchFindRecordAPI, args ...string) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) { return api, nil })
		commandLine.IaaSAPI.Switch.SetZoneFactory(func() (zones.API, error) {
			return testIaaSZoneAPI{zones: []*iaas.Zone{{Name: "zone-a"}, {Name: "zone-b"}}}, nil
		})
		commandArgs := append([]string{"iaas-api", "switch", "find"}, args...)
		exitCode := runCLI(commandArgs, &stdout, &stderr, commandLine)
		return exitCode, stdout.String(), stderr.String()
	}

	api := &switchFindRecordAPI{}
	exitCode, stdout, stderr := run(api, "--zone", "all", "--count", "3", "--from", "2")
	if exitCode != 0 || stderr != "" {
		t.Fatalf("all-zone search exit code = %d, stderr = %q", exitCode, stderr)
	}
	if len(api.requests) != 2 {
		t.Fatalf("all-zone search made %d requests, want 2", len(api.requests))
	}
	for i, zone := range []string{"zone-a", "zone-b"} {
		request := api.requests[i]
		if request.Zone != zone || request.Count != 3 || request.From != 2 {
			t.Errorf("request %d = %+v, want zone %s, count 3, from 2", i, request, zone)
		}
	}
	if !strings.Contains(stdout, `"Name": "zone-a"`) || !strings.Contains(stdout, `"Name": "zone-b"`) {
		t.Errorf("all-zone output = %s, want results from both zones", stdout)
	}

	api = &switchFindRecordAPI{failZone: "zone-b"}
	exitCode, stdout, stderr = run(api, "--zone", "all")
	if exitCode == 0 || stdout != "" || !strings.Contains(stderr, `zone "zone-b"`) {
		t.Fatalf("failed all-zone search: code %d, stdout %q, stderr %q; want error without partial output", exitCode, stdout, stderr)
	}
}

func TestGeneratedFindJSONDoesNotExpandAllZones(t *testing.T) {
	api := &switchFindRecordAPI{}
	zoneFactoryCalled := false
	var stdout, stderr bytes.Buffer
	commandLine := newCLI()
	commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) { return api, nil })
	commandLine.IaaSAPI.Switch.SetZoneFactory(func() (zones.API, error) {
		zoneFactoryCalled = true
		return testIaaSZoneAPI{zones: []*iaas.Zone{{Name: "zone-a"}}}, nil
	})
	exitCode := runCLI(
		[]string{"iaas-api", "switch", "find", "--request", `{"Zone":"all"}`},
		&stdout, &stderr, commandLine,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON search exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if zoneFactoryCalled || len(api.requests) != 1 || api.requests[0].Zone != "all" {
		t.Fatalf("JSON request was altered: zone factory called=%t, requests=%+v", zoneFactoryCalled, api.requests)
	}
}

func TestGeneratedFindRejectsEmptyZoneBeforeCallingAPI(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "flag", args: []string{"--zone", ""}},
		{name: "request", args: []string{"--request", `{"Zone":""}`}},
		{name: "missing JSON zone", args: []string{"--request", `{}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &switchFindRecordAPI{}
			factoryCalled := false
			commandLine := newCLI()
			commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
				factoryCalled = true
				return api, nil
			})

			var stdout, stderr bytes.Buffer
			args := append([]string{"iaas-api", "switch", "find"}, test.args...)
			exitCode := runCLI(args, &stdout, &stderr, commandLine)
			if exitCode == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("find with empty zone: code %d, stdout %q, stderr %q; want validation error", exitCode, stdout.String(), stderr.String())
			}
			if factoryCalled || len(api.requests) != 0 {
				t.Fatalf("invalid zone reached API setup: factory called=%t, requests=%+v", factoryCalled, api.requests)
			}
		})
	}
}

type testIaaSZoneAPI struct {
	zones []*iaas.Zone
}

func (api testIaaSZoneAPI) FindWithContext(context.Context, *zone.FindRequest) ([]*iaas.Zone, error) {
	return api.zones, nil
}

func TestSwitchFlagInputs(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	call := func(args ...string) ([]byte, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) { return swytch.New(server), nil })
		code := runCLI(append([]string{"iaas-api", "switch"}, args...), &stdout, &stderr, commandLine)
		return stdout.Bytes(), stderr.String(), code
	}
	mustRun := func(args ...string) []byte {
		t.Helper()
		out, stderr, code := call(args...)
		if code != 0 || stderr != "" {
			t.Fatalf("%v: code %d, stderr %s", args, code, stderr)
		}
		return out
	}
	var created iaas.Switch
	if err := json.Unmarshal(mustRun("create", "--zone", "test-zone", "--name", "flag-switch", "--description", "created"), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "flag-switch" || created.Description != "created" {
		t.Fatalf("unexpected create result: %+v", created)
	}
	id := created.ID.String()
	var found []*iaas.Switch
	if err := json.Unmarshal(mustRun("find", "--zone", "test-zone", "--count", "0", "--from", "0"), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("unexpected find result: %+v", found)
	}
	var read iaas.Switch
	if err := json.Unmarshal(mustRun("read", "--zone", "test-zone", "--id", id), &read); err != nil {
		t.Fatal(err)
	}
	if read.ID != created.ID {
		t.Fatalf("unexpected read result: %+v", read)
	}
	var updated iaas.Switch
	if err := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--name", "renamed"), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed" || updated.Description != "created" {
		t.Fatalf("update lost an omitted value: %+v", updated)
	}
	if err := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--description", ""), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Description != "" || updated.Name != "renamed" {
		t.Fatalf("explicit empty description not applied: %+v", updated)
	}
	if err := json.Unmarshal(mustRun("update", "--zone", "test-zone", "--id", id, "--network-mask-len", "0"), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.NetworkMaskLen != 0 || updated.Name != "renamed" || updated.Description != "" {
		t.Fatalf("explicit zero network mask or omitted fields were not preserved: %+v", updated)
	}
	mustRun("delete", "--zone", "test-zone", "--id", id, "--fail-if-not-found")
	found = nil
	if err := json.Unmarshal(mustRun("find", "--zone", "test-zone"), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("deleted Switch still listed: %+v", found)
	}
	for _, args := range [][]string{
		{"create", "--name", "missing-zone"},
		{"create", "--zone", "test-zone"},
		{"update", "--zone", "test-zone"},
		{"read", "--zone", "test-zone", "--id", "0"},
		{"delete", "--zone", "test-zone", "--id", "not-a-number"},
		{"find"},
		{"find", "--request", `{"Zone":"test-zone"}`, "--count", "0"},
		{"create", "--request", `{"Zone":"test-zone","Name":"json"}`, "--description", ""},
		{"update", "--request", `{"Zone":"test-zone","ID":123}`, "--network-mask-len", "0"},
		{"delete", "--request", `{"Zone":"test-zone","ID":123}`, "--fail-if-not-found=false"},
		{"read", "--zone", "all", "--id", "123"},
		{"create", "--zone", "all", "--name", "switch"},
		{"update", "--zone", "all", "--id", "123", "--name", "switch"},
		{"delete", "--zone", "all", "--id", "123"},
	} {
		out, stderr, code := call(args...)
		if code == 0 || len(out) != 0 || stderr == "" {
			t.Errorf("%v: code %d stdout %s stderr %s; want explicit error", args, code, out, stderr)
		}
	}
}

func TestSwitchCreateRequiresZoneAndName(t *testing.T) {
	for _, request := range []string{
		`{"Name":"missing-zone"}`,
		`{"Zone":"test-zone"}`,
	} {
		var stdout, stderr bytes.Buffer
		exitCode := run([]string{"iaas-api", "switch", "create", "--request", request}, &stdout, &stderr)
		if exitCode == 0 {
			t.Errorf("create with request %s succeeded, want validation error", request)
		}

		if stdout.Len() != 0 {
			t.Errorf("create with request %s wrote stdout %q, want empty", request, stdout.String())
		}
	}
}

func TestSwitchHelpJSONExamplesDecode(t *testing.T) {
	var find swytch.FindRequest
	if err := decodeRequest(`{"Zone":"tk1v","Names":["example"],"Sort":[{"Key":"Name","Order":0}]}`, &find); err != nil {
		t.Fatal(err)
	}
	if find.Zone != "tk1v" || len(find.Names) != 1 || find.Names[0] != "example" ||
		len(find.Sort) != 1 || find.Sort[0].Key != "Name" || find.Sort[0].Order != 0 {
		t.Fatalf("unexpected find request: %+v", find)
	}
	var create swytch.CreateRequest
	if err := decodeRequest(`{"Zone":"tk1v","Name":"example","Tags":["test"]}`, &create); err != nil {
		t.Fatal(err)
	}
	if create.Zone != "tk1v" || create.Name != "example" || len(create.Tags) != 1 || create.Tags[0] != "test" {
		t.Fatalf("unexpected create request: %+v", create)
	}
	var update swytch.UpdateRequest
	if err := decodeRequest(`{"Zone":"tk1v","ID":123456789012,"Tags":["test"]}`, &update); err != nil {
		t.Fatal(err)
	}
	if update.Zone != "tk1v" || update.ID != 123456789012 || update.Tags == nil ||
		len(*update.Tags) != 1 || (*update.Tags)[0] != "test" {
		t.Fatalf("unexpected update request: %+v", update)
	}
}
