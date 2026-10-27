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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	"github.com/sacloud/skr/internal/apigen"
	diskapi "github.com/sacloud/skr/internal/iaas/diskapi"
	"github.com/stretchr/testify/require"
)

func TestDiskGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-disk.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/diskapi/disk_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Disk commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-disk.json API_OUTPUT=internal/iaas/diskapi/disk_api_generated.go")
}

func TestRunIaaSDiskHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"disk", "server", "switch"}},
		{args: []string{"iaas-api", "disk", "--help"}, want: []string{"find", "read", "create", "update", "delete"}},
		{args: []string{"iaas-api", "disk", "find", "--help"}, want: []string{"--zone", "--name", "--tags", "カンマ区切り", "Count", "From", "Names", "--request", "部分一致", "併用不可"}, notWant: []string{"--zone all"}},
		{args: []string{"iaas-api", "disk", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
		{args: []string{"iaas-api", "disk", "create", "--help"}, want: []string{"--zone", "--name", "--disk-plan-id", "--connection", "--size-gb", "--tags", "--request", "DistantFrom", "OSType", "API フィールドではありません", "EditParameter", "@path.json", "併用不可"}},
		{args: []string{"iaas-api", "disk", "update", "--help"}, want: []string{"--zone", "--id", "--name", "--description", "--tags", "--icon-id", "--connection", "--no-wait", "--request", "省略した項目", "EditParameter", "@path.json", "併用不可"}},
		{args: []string{"iaas-api", "disk", "delete", "--help"}, want: []string{"--fail-if-not-found", "--wait-for-release", "接続中", "復旧できません"}},
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

func TestIaaSDiskCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &diskAPITestDouble{}
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "disk"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	create := `{"Zone":"test-zone","Name":"test-disk","Description":"test description","DiskPlanID":4,"Connection":"virtio","SizeGB":20,"Tags":["test"],"IconID":11,"NoWait":true}`
	output, stderr, code := run("create", "--request", create)
	require.Equal(t, 0, code, "create: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"Name": "test-disk"`, "missing expected text %q", `"Name": "test-disk"`)
	if api.createRequest == nil || api.createRequest.Zone != "test-zone" ||
		api.createRequest.Name != "test-disk" || api.createRequest.Description != "test description" ||
		api.createRequest.DiskPlanID != 4 || api.createRequest.Connection != "virtio" ||
		api.createRequest.SizeGB != 20 || api.createRequest.IconID != 11 || !api.createRequest.NoWait ||
		len(api.createRequest.Tags) != 1 || api.createRequest.Tags[0] != "test" {
		t.Fatalf("Create request = %+v, want JSON values", api.createRequest)
	}
	jsonCreateRequest := *api.createRequest

	_, stderr, code = run("create",
		"--zone", "test-zone", "--name", "test-disk", "--description", "test description",
		"--disk-plan-id", "4", "--connection", "virtio", "--size-gb", "20",
		"--tags", "test", "--icon-id", "11", "--no-wait",
	)
	require.Equal(t, 0, code, "flag create: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.NotNil(t, api.createRequest, "flag Create request = %+v, want JSON request %+v", api.createRequest, jsonCreateRequest)
	require.Equal(t, jsonCreateRequest, *api.createRequest, "unexpected value")

	_, stderr, code = run("create",
		"--zone", "test-zone", "--name", "test-disk", "--disk-plan-id", "4",
		"--connection", "virtio", "--size-gb", "20", "--no-wait=false",
	)
	require.Equal(t, 0, code, "create with explicit false boolean: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.NotNil(t, api.createRequest, "Create request NoWait = %v, want explicit false", api.createRequest)
	require.False(t, api.createRequest.NoWait, "unexpected value")

	_, stderr, code = run("find", "--zone", "test-zone", "--tags", "production,web", "--count", "7", "--from", "2")
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.findRequests, 1, "Find called %d times, want one zone", len(api.findRequests))
	if request := api.findRequests[0]; request.Zone != "test-zone" || request.Count != 7 || request.From != 2 ||
		len(request.Tags) != 2 || request.Tags[0] != "production" || request.Tags[1] != "web" {
		t.Errorf("Find request = %+v, want test-zone, Count 7, From 2", request)
	}

	output, stderr, code = run("find", "--zone", "test-zone", "--query", "map({ID,Name,SizeMB})")
	require.Equal(t, 0, code, "projected find: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	var projected []struct {
		ID     json.Number
		Name   string
		SizeMB int
	}
	err := json.Unmarshal([]byte(output), &projected)
	require.NoError(t, err)
	require.Len(t, projected, 1, "projected Disk = %s, want ID, name and size", output)
	require.Equal(t, "123", projected[0].ID.String(), "unexpected value")
	require.Equal(t, "test-zone", projected[0].Name, "unexpected value")
	require.EqualValues(t, 20480, projected[0].SizeMB, "unexpected value")

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "123")
	require.Equal(t, 0, code, "read: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"ID": 123`, "missing expected text %q", `"ID": 123`)
	require.NotNil(t, api.readRequest, "Read request = %+v, want zone and ID", api.readRequest)
	require.Equal(t, "test-zone", api.readRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.readRequest.ID, "unexpected value")

	update := `{"Zone":"test-zone","ID":123,"Name":"updated-disk","Description":"","Tags":["test"],"IconID":0,"Connection":"virtio","NoWait":true}`
	_, stderr, code = run("update", "--request", update)
	require.Equal(t, 0, code, "update: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if api.updateRequest == nil || api.updateRequest.ID != 123 ||
		api.updateRequest.Name == nil || *api.updateRequest.Name != "updated-disk" ||
		api.updateRequest.Description == nil || *api.updateRequest.Description != "" ||
		api.updateRequest.Tags == nil || len(*api.updateRequest.Tags) != 1 || (*api.updateRequest.Tags)[0] != "test" ||
		api.updateRequest.IconID == nil || *api.updateRequest.IconID != 0 ||
		api.updateRequest.Connection == nil || *api.updateRequest.Connection != "virtio" ||
		!api.updateRequest.NoWait {
		t.Fatalf("Update request = %+v, want ID and Name", api.updateRequest)
	}
	jsonUpdateRequest := *api.updateRequest

	_, stderr, code = run("update",
		"--zone", "test-zone", "--id", "123", "--name", "updated-disk",
		"--description", "", "--tags", "test", "--icon-id", "0",
		"--connection", "virtio", "--no-wait",
	)
	require.Equal(t, 0, code, "flag update: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.NotNil(t, api.updateRequest, "flag Update request = %+v, want JSON request %+v", api.updateRequest, jsonUpdateRequest)
	require.Equal(t, jsonUpdateRequest, *api.updateRequest, "unexpected value")

	_, stderr, code = run("update", "--zone", "test-zone", "--id", "123", "--name", "partial-update")
	require.Equal(t, 0, code, "partial flag update: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if api.updateRequest == nil || api.updateRequest.Name == nil || *api.updateRequest.Name != "partial-update" ||
		api.updateRequest.Description != nil || api.updateRequest.Tags != nil ||
		api.updateRequest.IconID != nil || api.updateRequest.Connection != nil {
		t.Fatalf("partial Update request = %+v, want only Name set", api.updateRequest)
	}

	_, stderr, code = run("delete", "--zone", "test-zone", "--id", "123", "--fail-if-not-found", "--wait-for-release")
	require.Equal(t, 0, code, "delete: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if api.deleteRequest == nil || api.deleteRequest.ID != 123 ||
		!api.deleteRequest.FailIfNotFound || !api.deleteRequest.WaitForRelease {
		t.Fatalf("Delete request = %+v, want ID, FailIfNotFound, and WaitForRelease", api.deleteRequest)
	}
}

func TestIaaSDiskRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &diskAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"read", "--zone", "all", "--id", "123"},
		{"create"},
		{"create", "--zone", "test-zone", "--name", "disk", "--disk-plan-id", "4", "--connection", "virtio"},
		{"create", "--request", `{"Zone":"test-zone","Name":"disk","DiskPlanID":4,"Connection":"virtio","SizeGB":20}`, "--zone", "test-zone"},
		{"update", "--request", `{"Zone":"test-zone","ID":123}`, "--zone", "test-zone"},
		{"update", "--id", "123", "--name", "disk"},
		{"delete", "--zone", "all", "--id", "123"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "disk"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	if len(api.findRequests) != 0 || api.readRequest != nil || api.createRequest != nil ||
		api.updateRequest != nil || api.deleteRequest != nil {
		t.Fatalf("invalid request reached API: %+v", api)
	}
}

type diskAPITestDouble struct {
	findRequests  []disk.FindRequest
	readRequest   *disk.ReadRequest
	createRequest *disk.CreateRequest
	updateRequest *disk.UpdateRequest
	deleteRequest *disk.DeleteRequest
}

func (api *diskAPITestDouble) FindWithContext(_ context.Context, request *disk.FindRequest) ([]*iaas.Disk, error) {
	api.findRequests = append(api.findRequests, *request)
	return []*iaas.Disk{{ID: 123, Name: request.Zone, SizeMB: 20480}}, nil
}

func (api *diskAPITestDouble) ReadWithContext(_ context.Context, request *disk.ReadRequest) (*iaas.Disk, error) {
	api.readRequest = request
	return &iaas.Disk{ID: request.ID, Name: "test-disk"}, nil
}

func (api *diskAPITestDouble) CreateWithContext(_ context.Context, request *disk.CreateRequest) (*iaas.Disk, error) {
	api.createRequest = request
	return &iaas.Disk{ID: 123, Name: request.Name}, nil
}

func (api *diskAPITestDouble) UpdateWithContext(_ context.Context, request *disk.UpdateRequest) (*iaas.Disk, error) {
	api.updateRequest = request
	name := "test-disk"
	if request.Name != nil {
		name = *request.Name
	}
	return &iaas.Disk{ID: request.ID, Name: name}, nil
}

func (api *diskAPITestDouble) DeleteWithContext(_ context.Context, request *disk.DeleteRequest) error {
	api.deleteRequest = request
	return nil
}
