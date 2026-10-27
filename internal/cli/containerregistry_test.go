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
	"github.com/stretchr/testify/require"
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
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, item := range test.want {
				require.Contains(t, help, item, "help does not contain %q: %s", item, help)
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
	setContainerRegistryAPIFactory(commandLine, containerRegistryRuntime(), func() (containerregistryapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	listed := run("container-registry-api", "registry", "list", "--request", `{"Names":["registry"]}`, "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	err := json.Unmarshal(listed, &projection)
	require.NoError(t, err)
	if len(projection) != 1 || string(projection[0]["Name"]) != `"registry"` {
		t.Fatalf("list output = %s", listed)
	}
	require.Len(t, api.findRequest.Names, 1, "Find request = %+v", api.findRequest)
	require.Equal(t, "registry", api.findRequest.Names[0], "unexpected value")

	created := run("container-registry-api", "registry", "create", "--request", `{"Name":"registry","Description":"test"}`)
	var createResult iaas.ContainerRegistry
	err2 := json.Unmarshal(created, &createResult)
	require.NoError(t, err2)
	if createResult.ID != 123 || api.createRequest.Name != "registry" ||
		api.createRequest.SubDomainLabel != "registry" || api.createRequest.Description != "test" {
		t.Fatalf("Create result/request = %+v / %+v", createResult, api.createRequest)
	}
	jsonRequest := *api.createRequest
	run("container-registry-api", "registry", "create", "--name", "registry", "--description", "test")
	require.Equal(t, *api.createRequest, jsonRequest, "flag and JSON requests differ: %+v / %+v", jsonRequest, api.createRequest)
	run("container-registry-api", "registry", "create", "--request", `{"Name":"registry","Description":"","IconID":0,"VirtualDomain":""}`)
	jsonRequest = *api.createRequest
	run("container-registry-api", "registry", "create", "--name", "registry", "--description", "", "--icon-id", "0", "--virtual-domain", "")
	require.Equal(t, *api.createRequest, jsonRequest, "explicit zero-value requests differ: %+v / %+v", jsonRequest, api.createRequest)

	run("container-registry-api", "registry", "read", "123")
	require.NotNil(t, api.readRequest, "Read request = %+v", api.readRequest)
	require.EqualValues(t, 123, api.readRequest.ID, "unexpected value")

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
	err3 := os.WriteFile(passwordFile, []byte(password+"\n"), 0o600)
	require.NoError(t, err3)
	if output := run("container-registry-api", "registry", "user", "add", "123",
		"--user-name", "builder",
		"--password-file", passwordFile,
		"--permission", "readwrite"); len(output) != 0 {
		t.Fatalf("user add output = %q, want empty", output)
	}
	require.NotNil(t, api.addUser, "AddUser request = %+v", api.addUser)
	require.Equal(t, password, api.addUser.Password, "unexpected value")
	require.Equal(t, "builder", api.addUser.UserName, "unexpected value")
	require.Equal(t, types.ContainerRegistryPermissions.ReadWrite, api.addUser.Permission, "unexpected value")

	run("container-registry-api", "registry", "user", "update", "123", "builder", "--permission", "readonly")
	require.NotNil(t, api.updateUser, "UpdateUser request = %+v", api.updateUser)
	require.Equal(t, types.ContainerRegistryPermissions.ReadOnly, api.updateUser.Permission, "unexpected value")
	require.Empty(t, api.updateUser.Password, "should be empty")
	run("container-registry-api", "registry", "user", "update", "123", "builder", "--password-file", passwordFile, "--permission", "all")
	require.NotNil(t, api.updateUser, "UpdateUser password request = %+v", api.updateUser)
	require.Equal(t, types.ContainerRegistryPermissions.All, api.updateUser.Permission, "unexpected value")
	require.Equal(t, password, api.updateUser.Password, "unexpected value")
	run("container-registry-api", "registry", "user", "update", "123", "builder", "--password-file", passwordFile)
	require.Equal(t, types.ContainerRegistryPermissions.ReadWrite, api.updateUser.Permission, "password-only update did not preserve permission: %+v", api.updateUser)
	require.Equal(t, password, api.updateUser.Password, "unexpected value")

	run("container-registry-api", "registry", "user", "delete", "123", "builder")
	require.Equal(t, "builder", api.deleteUser, "DeleteUser user = %q", api.deleteUser)

	run("container-registry-api", "registry", "delete", "123")
	require.NotNil(t, api.deleteRequest, "Delete request = %+v", api.deleteRequest)
	require.EqualValues(t, 123, api.deleteRequest.ID, "unexpected value")
}

func TestContainerRegistryRequestAndUserValidationBeforeAPICall(t *testing.T) {
	commandLine := newCLI()
	setContainerRegistryAPIFactory(commandLine, containerRegistryRuntime(), func() (containerregistryapi.API, error) {
		t.Fatal("API factory called for invalid input")
		return nil, nil
	})

	for _, args := range [][]string{
		{"container-registry-api", "registry", "create"},
		{"container-registry-api", "registry", "create", "--request", `{}`},
		{"container-registry-api", "registry", "create", "--request", `{"Name":""}`},
		{"container-registry-api", "registry", "create", "--name", ""},
		{"container-registry-api", "registry", "create", "--description", "test"},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry"}`, "--name", "registry"},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry"}`, "--description", ""},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry"}`, "--icon-id", "0"},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry"}`, "--virtual-domain", ""},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"registry","Users":[{"Password":"secret"}]}`},
		{"container-registry-api", "registry", "create", "--request", `{"Name":"Invalid_Name"}`},
		{"container-registry-api", "registry", "user", "add", "123", "--user-name", "builder", "--password-file", "missing", "--permission", "invalid"},
		{"container-registry-api", "registry", "user", "update", "123", "builder"},
	} {
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "run(%v) = code %d, stdout %q, want validation error", args, code, stdout.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	}
}

func TestContainerRegistryPasswordUpdateRejectsMissingOrInvalidUser(t *testing.T) {
	for _, users := range [][]containerregistryapi.User{
		nil,
		{{UserName: "builder", Permission: ""}},
		{{UserName: "builder", Permission: "readonly"}, {UserName: "builder", Permission: "readonly"}},
	} {
		api := &containerRegistryAPIMock{users: users}
		commandLine := newCLI()
		setContainerRegistryAPIFactory(commandLine, containerregistryapi.Runtime{
			ReadPassword: func(string) (string, error) { return "test-password", nil },
		}, func() (containerregistryapi.API, error) { return api, nil })
		var stdout, stderr bytes.Buffer
		if code := runCLI([]string{
			"container-registry-api", "registry", "user", "update", "123", "builder", "--password-file", "password",
		}, &stdout, &stderr, commandLine); code == 0 || api.updateUser != nil {
			t.Fatalf("invalid user allowed update: code=%d request=%+v stderr=%s", code, api.updateUser, stderr.String())
		}
	}
}
