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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/sacloud/skr/internal/apigen"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
)

func TestServerGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-server.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/serverapi/server_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Server commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-server.json API_OUTPUT=internal/iaas/serverapi/server_api_generated.go")
	}
}

func TestRunIaaSServerHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"server", "switch"}},
		{args: []string{"iaas-api", "server", "--help"}, want: []string{"find", "read", "create", "boot", "update", "delete"}},
		{args: []string{"iaas-api", "server", "find", "--help"}, want: []string{"--zone", "--tags", "カンマ区切り", "Count", "From", "Names", "併用不可"}, notWant: []string{"--zone all"}},
		{args: []string{"iaas-api", "server", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
		{args: []string{"iaas-api", "server", "create", "--help"}, want: []string{"--zone", "--name", "--cpu", "--memory-gb", "--tags", "--request", "--disk", "--nic", "archive-id", "plan-id", "size-gb", "繰り返し", "shared", "CPU", "MemoryGB", "Disks", "NetworkInterfaces", "併用不可", "必須: Zone", "文字列配列", "NoWait=true", "BootAfterCreate=true", "NoWait=true とは併用できません", `"Tags":["tutorial"]`}, notWant: []string{"--cloudimg-archive-id", "--disk-plan-id", "--disk-size-gb", "--network-interface"}},
		{args: []string{"iaas-api", "server", "boot", "--help"}, want: []string{"--zone", "--id", "--user-data", "@path.yaml", "標準入力", "インライン値は指定できません"}},
		{args: []string{"iaas-api", "server", "update", "--help"}, want: []string{"--name", "--description", "--tags", "--cpu", "--memory-gb", "省略", "併用不可", "任意フラグは Name、Description、Tags", "CPUModel です。JSON 経路", "IconID、Commitment、Generation、InterfaceDriver、CDROMID、PrivateHostID、Disks、NetworkInterfaces", "ID の設定・クリアには JSON"}, notWant: []string{"--icon-id", "--commitment", "--generation", "--interface-driver", "--cdrom-id", "--private-host-id"}},
		{args: []string{"iaas-api", "server", "delete", "--help"}, want: []string{"--with-disks", "--force", "ディスク", "強制停止", "併用不可"}},
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

func TestIaaSServerCommandsBuildSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &serverAPITestDouble{}
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setServerAPIFactory(commandLine, func() (serverapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "server"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	requestPath := filepath.Join(t.TempDir(), "server-create.json")
	createRequestJSON := `{"Zone":"test-zone","Name":"server","CPU":1,"MemoryGB":1,"Description":"test description","IconID":41,"GPU":0,"GPUModel":"gpu-model","CPUModel":"cpu-model","Commitment":"standard","Generation":100,"ConfidentialVM":false,"InterfaceDriver":"virtio","BootAfterCreate":false,"CDROMID":42,"PrivateHostID":43,"Tags":["tutorial"]}`
	if err := os.WriteFile(requestPath, []byte(createRequestJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	output, stderr, code := run("create", "--request", "@"+requestPath)
	if code != 0 || stderr != "" || !strings.Contains(output, `"Name": "server"`) {
		t.Fatalf("create: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.createRequest == nil || api.createRequest.Zone != "test-zone" ||
		api.createRequest.Name != "server" || api.createRequest.CPU != 1 || api.createRequest.MemoryGB != 1 {
		t.Fatalf("Create request = %+v, want JSON values", api.createRequest)
	}
	jsonCreateRequest := *api.createRequest
	if len(jsonCreateRequest.Tags) != 1 || jsonCreateRequest.Tags[0] != "tutorial" {
		t.Fatalf("Create JSON Tags = %v, want [tutorial]", jsonCreateRequest.Tags)
	}
	jsonCreateRequest.Tags = nil
	_, stderr, code = run(
		"create",
		"--zone", "test-zone",
		"--name", "server",
		"--cpu", "1",
		"--memory-gb", "1",
		"--tags", "production,web",
		"--description", "test description",
		"--icon-id", "41",
		"--gpu", "0",
		"--gpu-model", "gpu-model",
		"--cpu-model", "cpu-model",
		"--commitment", "standard",
		"--generation", "100",
		"--confidential-vm=false",
		"--interface-driver", "virtio",
		"--boot-after-create=false",
		"--cdrom-id", "42",
		"--private-host-id", "43",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("create flags: code %d, stderr %q", code, stderr)
	}
	if api.createRequest == nil {
		t.Fatal("Create flag request is nil")
	}
	wantTags := []string{"production", "web"}
	if !reflect.DeepEqual([]string(api.createRequest.Tags), wantTags) {
		t.Fatalf("Create flag Tags = %v, want %v", api.createRequest.Tags, wantTags)
	}
	api.createRequest.Tags = nil
	if !reflect.DeepEqual(*api.createRequest, jsonCreateRequest) {
		t.Fatalf("Create flag request = %+v, want JSON request %+v", api.createRequest, jsonCreateRequest)
	}

	_, stderr, code = run(
		"create",
		"--zone", "test-zone",
		"--name", "cloudimg-server",
		"--cpu", "2",
		"--memory-gb", "4",
		"--disk", "archive-id=456,plan-id=7,size-gb=20",
		"--disk", "plan-id=4,size-gb=100",
		"--nic", "upstream=shared",
		"--nic", "upstream=disconnected",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("create cloud-init options: code %d, stderr %q", code, stderr)
	}
	if api.createRequest == nil || len(api.createRequest.Disks) != 2 {
		t.Fatalf("Create request disks = %+v, want two disks", api.createRequest)
	}
	createdDisk := api.createRequest.Disks[0]
	if createdDisk.Name != "cloudimg-server" || createdDisk.DiskPlanID != 7 ||
		createdDisk.Connection != "virtio" || createdDisk.SourceArchiveID != 456 || createdDisk.SizeGB != 20 {
		t.Errorf("cloudimg disk = %+v, want name, plan 7, virtio, archive 456, size 20", createdDisk)
	}
	if second := api.createRequest.Disks[1]; second.DiskPlanID != 4 || second.SizeGB != 100 || second.SourceArchiveID != 0 {
		t.Errorf("second disk = %+v, want empty 100 GB disk", second)
	}
	if len(api.createRequest.NetworkInterfaces) != 2 || api.createRequest.NetworkInterfaces[0].Upstream != "shared" || api.createRequest.NetworkInterfaces[1].Upstream != "disconnected" {
		t.Errorf("Create network interfaces = %+v, want shared then disconnected", api.createRequest.NetworkInterfaces)
	}

	userDataPath := filepath.Join(t.TempDir(), "cloud-c0nfig.yaml")
	userData := "#cloud-config\nssh_authorized_keys:\n  - ssh-ed25519 AAAA-example\n"
	if err := os.WriteFile(userDataPath, []byte(userData), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = run("boot", "--zone", "test-zone", "--id", "123", "--user-data", "@"+userDataPath)
	if code != 0 || stderr != "" {
		t.Fatalf("boot: code %d, stderr %q", code, stderr)
	}
	if api.bootRequest == nil || api.bootRequest.Zone != "test-zone" ||
		api.bootRequest.ID != 123 || api.bootRequest.UserData != userData {
		t.Fatalf("Boot request = %+v, want zone, ID, and unmodified user-data", api.bootRequest)
	}

	_, stderr, code = run("boot", "--zone", "test-zone", "--id", "123")
	if code != 0 || stderr != "" {
		t.Fatalf("boot without user-data: code %d, stderr %q", code, stderr)
	}
	if api.bootRequest == nil || api.bootRequest.Zone != "test-zone" ||
		api.bootRequest.ID != 123 || api.bootRequest.UserData != "" {
		t.Fatalf("Boot request = %+v, want zone and ID without user-data", api.bootRequest)
	}

	_, stderr, code = run("find", "--zone", "test-zone", "--tags", "production,web", "--count", "4", "--from", "2")
	if code != 0 || stderr != "" {
		t.Fatalf("find: code %d, stderr %q", code, stderr)
	}
	if len(api.findRequests) != 1 {
		t.Fatalf("Find called %d times, want one zone", len(api.findRequests))
	}
	if request := api.findRequests[0]; request.Zone != "test-zone" || request.Count != 4 || request.From != 2 ||
		!reflect.DeepEqual(request.Tags, wantTags) {
		t.Errorf("Find request = %+v, want test-zone, Count 4, From 2", request)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "123")
	if code != 0 || stderr != "" || !strings.Contains(output, `"ID": 123`) {
		t.Fatalf("read: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.readRequest == nil || api.readRequest.Zone != "test-zone" || api.readRequest.ID != 123 {
		t.Fatalf("Read request = %+v, want zone and ID flags", api.readRequest)
	}

	_, stderr, code = run("update", "--zone", "test-zone", "--id", "123", "--name", "renamed", "--tags", "production,web", "--memory-gb", "0")
	if code != 0 || stderr != "" {
		t.Fatalf("update: code %d, stderr %q", code, stderr)
	}
	if api.updateRequest == nil || api.updateRequest.Name == nil || *api.updateRequest.Name != "renamed" ||
		api.updateRequest.MemoryGB == nil || *api.updateRequest.MemoryGB != 0 ||
		api.updateRequest.Tags == nil || !reflect.DeepEqual([]string(*api.updateRequest.Tags), wantTags) {
		t.Fatalf("Update request = %+v, want explicit name and zero memory", api.updateRequest)
	}

	_, stderr, code = run("delete", "--zone", "test-zone", "--id", "123", "--with-disks", "--force")
	if code != 0 || stderr != "" {
		t.Fatalf("delete: code %d, stderr %q", code, stderr)
	}
	if api.deleteRequest == nil || api.deleteRequest.ID != 123 || !api.deleteRequest.WithDisks || !api.deleteRequest.Force {
		t.Fatalf("Delete request = %+v, want ID, WithDisks, and Force", api.deleteRequest)
	}
}

func TestIaaSServerRejectsInvalidInputBeforeCallingAPI(t *testing.T) {
	api := &serverAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"read", "--zone", "all", "--id", "123"},
		{"create", "--request", `{"Zone":"test-zone","Name":"server"}`, "--zone", "test-zone"},
		{"create", "--zone", "test-zone", "--name", "server", "--memory-gb", "1"},
		{"create", "--request", `{"Zone":"test-zone","Name":"server","CPU":1,"MemoryGB":1}`, "--boot-after-create=false"},
		{"create", "--request", `{"Zone":"test-zone","Name":"server","CPU":1,"MemoryGB":1}`, "--disk", "plan-id=4,size-gb=20"},
		{"create", "--request", `{"Zone":"test-zone","Name":"server","CPU":1,"MemoryGB":1}`, "--nic", "upstream=shared"},
		{"create", "--zone", "test-zone", "--name", "server", "--cpu", "1", "--memory-gb", "1", "--disk", "archive-id=123"},
		{"create", "--zone", "test-zone", "--name", "server", "--cpu", "1", "--memory-gb", "1", "--disk", "archive-id=123,plan-id=7,size-gb=20", "--boot-after-create"},
		{"boot", "--zone", "test-zone", "--id", "123", "--user-data", "inline-user-data"},
		{"delete", "--zone", "all", "--id", "123"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setServerAPIFactory(commandLine, func() (serverapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "server"}, args...), &stdout, &stderr, commandLine)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want an error before API call", args, code, stdout.String(), stderr.String())
		}
	}
	if len(api.findRequests) != 0 || api.readRequest != nil || api.createRequest != nil || api.bootRequest != nil ||
		api.updateRequest != nil || api.deleteRequest != nil {
		t.Fatalf("invalid input reached API: %+v", api)
	}
}

func TestIaaSServerPassesNoWaitWithBootAfterCreateToSDK(t *testing.T) {
	var stdout, stderr bytes.Buffer
	factoryCalled := false
	commandLine := newCLI()
	api := &serverAPITestDouble{}
	setServerAPIFactory(commandLine, func() (serverapi.API, error) {
		factoryCalled = true
		return api, nil
	})
	code := runCLI(
		[]string{"iaas-api", "server", "create", "--request", `{"Zone":"test-zone","Name":"server","CPU":1,"MemoryGB":1,"NoWait":true,"BootAfterCreate":true}`},
		&stdout,
		&stderr,
		commandLine,
	)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("create: code %d, stdout %q, stderr %q; want request forwarded", code, stdout.String(), stderr.String())
	}
	if !factoryCalled || api.createRequest == nil || !api.createRequest.NoWait || !api.createRequest.BootAfterCreate {
		t.Fatalf("request was not forwarded unchanged: %+v", api.createRequest)
	}
}

func TestIaaSServerFindUsesSDKCaller(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverAPICaller{}
	var stdout, stderr bytes.Buffer
	commandLine := newCLI()
	setServerAPIFactory(commandLine, func() (serverapi.API, error) {
		return server.New(caller), nil
	})
	code := runCLI([]string{"iaas-api", "server", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("find: code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"Name": "sdk-server"`) {
		t.Fatalf("find output = %q, want SDK-decoded server", stdout.String())
	}
	if len(caller.calls) != 1 || caller.calls[0].method != "GET" {
		t.Fatalf("SDK caller requests = %+v, want one GET", caller.calls)
	}
	parsed, err := url.Parse(caller.calls[0].uri)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(parsed.Path, "/zone/test-zone/api/cloud/1.1/server") {
		t.Errorf("SDK request path = %q, want Server endpoint", parsed.Path)
	}
}

type serverAPITestDouble struct {
	findRequests  []server.FindRequest
	readRequest   *server.ReadRequest
	createRequest *server.CreateRequest
	bootRequest   *server.BootRequest
	updateRequest *server.UpdateRequest
	deleteRequest *server.DeleteRequest
}

func (api *serverAPITestDouble) FindWithContext(_ context.Context, request *server.FindRequest) ([]*iaas.Server, error) {
	api.findRequests = append(api.findRequests, *request)
	return []*iaas.Server{{ID: 123, Name: request.Zone}}, nil
}

func (api *serverAPITestDouble) ReadWithContext(_ context.Context, request *server.ReadRequest) (*iaas.Server, error) {
	api.readRequest = request
	return &iaas.Server{ID: request.ID, Name: "server"}, nil
}

func (api *serverAPITestDouble) CreateWithContext(_ context.Context, request *server.CreateRequest) (*iaas.Server, error) {
	api.createRequest = request
	return &iaas.Server{ID: 123, Name: request.Name}, nil
}

func (api *serverAPITestDouble) BootWithContext(_ context.Context, request *server.BootRequest) error {
	api.bootRequest = request
	return nil
}

func (api *serverAPITestDouble) UpdateWithContext(_ context.Context, request *server.UpdateRequest) (*iaas.Server, error) {
	api.updateRequest = request
	return &iaas.Server{ID: request.ID, Name: "renamed"}, nil
}

func (api *serverAPITestDouble) DeleteWithContext(_ context.Context, request *server.DeleteRequest) error {
	api.deleteRequest = request
	return nil
}

type serverAPICaller struct {
	calls []serverAPICall
}

type serverAPICall struct {
	method string
	uri    string
}

func (caller *serverAPICaller) Do(_ context.Context, method, uri string, _ any) ([]byte, error) {
	caller.calls = append(caller.calls, serverAPICall{method: method, uri: uri})
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse SDK request URL: %w", err)
	}
	if method != "GET" || !strings.HasSuffix(parsed.Path, "/server") {
		return nil, fmt.Errorf("unexpected SDK request: %s %s", method, uri)
	}
	return json.Marshal(map[string]any{
		"Total": 1,
		"Count": 1,
		"Servers": []map[string]any{
			{"ID": 123, "Name": "sdk-server"},
		},
	})
}
