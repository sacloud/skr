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
)

func TestDiskGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-disk.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/diskapi/disk_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Disk commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-disk.json API_OUTPUT=internal/iaas/diskapi/disk_api_generated.go")
	}
}

func TestRunIaaSDiskHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"disk", "server", "switch"}},
		{args: []string{"iaas-api", "disk", "--help"}, want: []string{"find", "read", "create", "update", "delete"}},
		{args: []string{"iaas-api", "disk", "find", "--help"}, want: []string{"--zone", "--tags", "カンマ区切り", "Count", "From", "Names", "--request", "JSON 例", `"Names":["example"]`, "併用不可"}, notWant: []string{"--zone all"}},
		{args: []string{"iaas-api", "disk", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
		{args: []string{"iaas-api", "disk", "create", "--help"}, want: []string{"JSON 専用", "DiskPlanID", "Connection", "SizeGB", "@path.json"}},
		{args: []string{"iaas-api", "disk", "update", "--help"}, want: []string{"JSON 専用", "省略した項目", "EditParameter"}},
		{args: []string{"iaas-api", "disk", "delete", "--help"}, want: []string{"--fail-if-not-found", "--wait-for-release", "接続中", "復旧できません"}},
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
			for _, text := range test.notWant {
				if strings.Contains(help, text) {
					t.Errorf("help output contains removed text %q", text)
				}
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
		commandLine.IaaSAPI.Disk.SetFactory(func() (diskapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "disk"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	create := `{"Zone":"test-zone","Name":"test-disk","DiskPlanID":4,"Connection":"virtio","SizeGB":20,"Tags":["test"]}`
	output, stderr, code := run("create", "--request", create)
	if code != 0 || stderr != "" || !strings.Contains(output, `"Name": "test-disk"`) {
		t.Fatalf("create: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.createRequest == nil || api.createRequest.Zone != "test-zone" ||
		api.createRequest.Name != "test-disk" || api.createRequest.DiskPlanID != 4 ||
		api.createRequest.Connection != "virtio" || api.createRequest.SizeGB != 20 ||
		len(api.createRequest.Tags) != 1 || api.createRequest.Tags[0] != "test" {
		t.Fatalf("Create request = %+v, want JSON values", api.createRequest)
	}

	_, stderr, code = run("find", "--zone", "test-zone", "--tags", "production,web", "--count", "7", "--from", "2")
	if code != 0 || stderr != "" {
		t.Fatalf("find: code %d, stderr %q", code, stderr)
	}
	if len(api.findRequests) != 1 {
		t.Fatalf("Find called %d times, want one zone", len(api.findRequests))
	}
	if request := api.findRequests[0]; request.Zone != "test-zone" || request.Count != 7 || request.From != 2 ||
		len(request.Tags) != 2 || request.Tags[0] != "production" || request.Tags[1] != "web" {
		t.Errorf("Find request = %+v, want test-zone, Count 7, From 2", request)
	}

	output, stderr, code = run("find", "--zone", "test-zone", "--query", "map({ID,Name,SizeMB})")
	if code != 0 || stderr != "" {
		t.Fatalf("projected find: code %d, stderr %q", code, stderr)
	}
	var projected []struct {
		ID     json.Number
		Name   string
		SizeMB int
	}
	if err := json.Unmarshal([]byte(output), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID.String() != "123" || projected[0].Name != "test-zone" || projected[0].SizeMB != 20480 {
		t.Fatalf("projected Disk = %s, want ID, name and size", output)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "123")
	if code != 0 || stderr != "" || !strings.Contains(output, `"ID": 123`) {
		t.Fatalf("read: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.readRequest == nil || api.readRequest.Zone != "test-zone" || api.readRequest.ID != 123 {
		t.Fatalf("Read request = %+v, want zone and ID", api.readRequest)
	}

	update := `{"Zone":"test-zone","ID":123,"Name":"updated-disk"}`
	_, stderr, code = run("update", "--request", update)
	if code != 0 || stderr != "" {
		t.Fatalf("update: code %d, stderr %q", code, stderr)
	}
	if api.updateRequest == nil || api.updateRequest.ID != 123 ||
		api.updateRequest.Name == nil || *api.updateRequest.Name != "updated-disk" {
		t.Fatalf("Update request = %+v, want ID and Name", api.updateRequest)
	}

	_, stderr, code = run("delete", "--zone", "test-zone", "--id", "123", "--fail-if-not-found", "--wait-for-release")
	if code != 0 || stderr != "" {
		t.Fatalf("delete: code %d, stderr %q", code, stderr)
	}
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
		{"create", "--request", `{"Zone":"test-zone","Name":"disk","DiskPlanID":4,"Connection":"virtio","SizeGB":20}`, "--zone", "test-zone"},
		{"create", "--request", `{"Zone":"test-zone","Name":"disk","DiskPlanID":4,"Connection":"virtio"}`},
		{"update", "--request", `{"Zone":"test-zone","ID":123}`},
		{"delete", "--zone", "all", "--id", "123"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Disk.SetFactory(func() (diskapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "disk"}, args...), &stdout, &stderr, commandLine)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
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
