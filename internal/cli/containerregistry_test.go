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
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	sdk "github.com/sacloud/sacloud-sdk-go/service/iaas/containerregistry"
	"github.com/sacloud/skr/internal/containerregistryapi"
)

type containerRegistryAPIMock struct {
	findRequest   *sdk.FindRequest
	createRequest *sdk.CreateRequest
	readRequest   *sdk.ReadRequest
	updateRequest *sdk.UpdateRequest
	deleteRequest *sdk.DeleteRequest
	users         []containerregistryapi.User
	addUser       *iaas.ContainerRegistryUserCreateRequest
	updateUser    *iaas.ContainerRegistryUserUpdateRequest
	deleteUser    string
}

func (m *containerRegistryAPIMock) Find(_ context.Context, request *sdk.FindRequest) ([]*iaas.ContainerRegistry, error) {
	m.findRequest = request
	return []*iaas.ContainerRegistry{{ID: 123, Name: "registry"}}, nil
}

func (m *containerRegistryAPIMock) Read(_ context.Context, request *sdk.ReadRequest) (*iaas.ContainerRegistry, error) {
	m.readRequest = request
	return &iaas.ContainerRegistry{ID: request.ID, Name: "registry"}, nil
}

func (m *containerRegistryAPIMock) Create(_ context.Context, request *sdk.CreateRequest) (*iaas.ContainerRegistry, error) {
	m.createRequest = request
	return &iaas.ContainerRegistry{
		ID:             123,
		Name:           request.Name,
		SubDomainLabel: request.SubDomainLabel,
		Description:    request.Description,
	}, nil
}

func (m *containerRegistryAPIMock) Update(_ context.Context, request *sdk.UpdateRequest) (*iaas.ContainerRegistry, error) {
	m.updateRequest = request
	return &iaas.ContainerRegistry{ID: request.ID, Name: "registry"}, nil
}

func (m *containerRegistryAPIMock) Delete(_ context.Context, request *sdk.DeleteRequest) error {
	m.deleteRequest = request
	return nil
}

func (m *containerRegistryAPIMock) ListUsers(_ context.Context, _ types.ID) ([]containerregistryapi.User, error) {
	return m.users, nil
}

func (m *containerRegistryAPIMock) AddUser(_ context.Context, _ types.ID, request *iaas.ContainerRegistryUserCreateRequest) error {
	m.addUser = request
	return nil
}

func (m *containerRegistryAPIMock) UpdateUser(_ context.Context, _ types.ID, userName string, request *iaas.ContainerRegistryUserUpdateRequest) error {
	m.deleteUser = userName
	m.updateUser = request
	return nil
}

func (m *containerRegistryAPIMock) DeleteUser(_ context.Context, _ types.ID, userName string) error {
	m.deleteUser = userName
	return nil
}

func TestRunContainerRegistryAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"container-registry-api", "--help"}, []string{"registry", "SAKURA_ACCESS_TOKEN", "password", "JSON"}},
		{[]string{"container-registry-api", "registry", "--help"}, []string{"list", "create", "read", "update", "delete", "user"}},
		{[]string{"container-registry-api", "registry", "create", "--help"}, []string{"--request", "Name", "レジストリ接続名", "Users", "公開設定", "@path.json"}},
		{[]string{"container-registry-api", "registry", "update", "--help"}, []string{"Description", "Tags", "IconID", "VirtualDomain", "Name", "変更できません"}},
		{[]string{"container-registry-api", "registry", "user", "add", "--help"}, []string{"--user-name", "--password-file", "--permission", "標準入力", "秘密情報"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v): code %d, stderr %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, item := range test.want {
				if !strings.Contains(help, item) {
					t.Errorf("help does not contain %q: %s", item, help)
				}
			}
		})
	}
}

func TestContainerRegistryCommandOperations(t *testing.T) {
	api := &containerRegistryAPIMock{users: []containerregistryapi.User{{
		UserName:   "builder",
		Permission: types.ContainerRegistryPermissions.ReadWrite,
	}}}
	commandLine := newCLI()
	commandLine.ContainerRegistryAPI.SetFactory(func() (containerregistryapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
			t.Fatalf("run(%v): code %d, stderr %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("run(%v) stderr = %q", args, stderr.String())
		}
		return stdout.Bytes()
	}

	listed := run("container-registry-api", "registry", "list", "--request", `{"Names":["registry"]}`, "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	if err := json.Unmarshal(listed, &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection) != 1 || string(projection[0]["Name"]) != `"registry"` {
		t.Fatalf("list output = %s", listed)
	}
	if len(api.findRequest.Names) != 1 || api.findRequest.Names[0] != "registry" {
		t.Fatalf("Find request = %+v", api.findRequest)
	}

	created := run("container-registry-api", "registry", "create", "--request", `{"Name":"registry","Description":"test"}`)
	var createResult iaas.ContainerRegistry
	if err := json.Unmarshal(created, &createResult); err != nil {
		t.Fatal(err)
	}
	if createResult.ID != 123 || api.createRequest.Name != "registry" ||
		api.createRequest.SubDomainLabel != "registry" || api.createRequest.Description != "test" {
		t.Fatalf("Create result/request = %+v / %+v", createResult, api.createRequest)
	}

	run("container-registry-api", "registry", "read", "123")
	if api.readRequest == nil || api.readRequest.ID != 123 {
		t.Fatalf("Read request = %+v", api.readRequest)
	}

	run("container-registry-api", "registry", "update", "123", "--request", `{"Description":"updated"}`)
	if api.updateRequest == nil || api.updateRequest.ID != 123 || api.updateRequest.Description == nil || *api.updateRequest.Description != "updated" || api.updateRequest.Name != nil {
		t.Fatalf("Update request = %+v", api.updateRequest)
	}

	users := run("container-registry-api", "registry", "user", "list", "123", "--query", "map({UserName,Permission})")
	if strings.Contains(string(users), "Password") || !strings.Contains(string(users), "builder") {
		t.Fatalf("user output = %s", users)
	}

	passwordFile := filepath.Join(t.TempDir(), "registry-password")
	const password = "pass.word-123"
	if err := os.WriteFile(passwordFile, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output := run("container-registry-api", "registry", "user", "add", "123",
		"--user-name", "builder",
		"--password-file", passwordFile,
		"--permission", "readwrite"); len(output) != 0 {
		t.Fatalf("user add output = %q, want empty", output)
	}
	if api.addUser == nil || api.addUser.Password != password || api.addUser.UserName != "builder" || api.addUser.Permission != types.ContainerRegistryPermissions.ReadWrite {
		t.Fatalf("AddUser request = %+v", api.addUser)
	}

	run("container-registry-api", "registry", "user", "update", "123", "builder", "--permission", "readonly")
	if api.updateUser == nil || api.updateUser.Permission != types.ContainerRegistryPermissions.ReadOnly || api.updateUser.Password != "" {
		t.Fatalf("UpdateUser request = %+v", api.updateUser)
	}
	run("container-registry-api", "registry", "user", "update", "123", "builder", "--password-file", passwordFile, "--permission", "all")
	if api.updateUser == nil || api.updateUser.Permission != types.ContainerRegistryPermissions.All || api.updateUser.Password != password {
		t.Fatalf("UpdateUser password request = %+v", api.updateUser)
	}

	run("container-registry-api", "registry", "user", "delete", "123", "builder")
	if api.deleteUser != "builder" {
		t.Fatalf("DeleteUser user = %q", api.deleteUser)
	}

	run("container-registry-api", "registry", "delete", "123")
	if api.deleteRequest == nil || api.deleteRequest.ID != 123 {
		t.Fatalf("Delete request = %+v", api.deleteRequest)
	}
}

func TestContainerRegistryRequestAndUserValidationBeforeAPICall(t *testing.T) {
	commandLine := newCLI()
	commandLine.ContainerRegistryAPI.SetFactory(func() (containerregistryapi.API, error) {
		t.Fatal("API factory called for invalid input")
		return nil, nil
	})

	for _, args := range [][]string{
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry","Users":[{"Password":"secret"}]}`},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"Invalid_Name"}`},
		{"container-registry-api", "registry", "user", "add", "123", "--user-name", "builder", "--password-file", "missing", "--permission", "invalid"},
		{"container-registry-api", "registry", "user", "update", "123", "builder"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr, commandLine); code == 0 || stdout.Len() != 0 {
			t.Fatalf("run(%v) = code %d, stdout %q, want validation error", args, code, stdout.String())
		}
	}
}
