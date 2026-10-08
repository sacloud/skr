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
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	iammock "github.com/sacloud/sakumock/iam"
	"github.com/sacloud/skr/internal/iamapi"
)

func TestRunIAMAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"iam-api", "--help"},
			want: []string{"user", "group", "policy", "SAKURA_ACCESS_TOKEN", "サービスプリンシパル"},
		},
		{
			args: []string{"iam-api", "user", "--help"},
			want: []string{"list", "create", "read", "update", "delete", "register-email", "unregister-email"},
		},
		{
			args: []string{"iam-api", "user", "list", "--help"},
			want: []string{"--page", "--per-page", "--ordering", "code", "-code"},
		},
		{
			args: []string{"iam-api", "user", "create", "--help"},
			want: []string{"--name", "--code", "--description", "--email", "--password-file", "標準入力", "register-email"},
		},
		{
			args: []string{"iam-api", "user", "update", "--help"},
			want: []string{"--name", "--description", "--password-file", "変更しません"},
		},
		{
			args: []string{"iam-api", "user", "register-email", "--help"},
			want: []string{"--email", "SSO"},
		},
		{
			args: []string{"iam-api", "group", "list", "--help"},
			want: []string{"--user-id", "--ordering", "name", "-name"},
		},
		{
			args: []string{"iam-api", "group", "update-memberships", "--help"},
			want: []string{"--request", "[1,2,3]", "上書き", "read-memberships"},
		},
		{
			args: []string{"iam-api", "policy", "update-organization", "--help"},
			want: []string{"--request", "ポリシーバインディング", "read-organization", "置き換え"},
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

func TestIAMAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	// The project/folder IAM policy operations require an existing project and
	// folder; create them in the mock as test setup.
	createInMock := func(path, body string) struct {
		ID int `json:"id"`
	} {
		t.Helper()
		response, err := http.Post(server.TestURL()+path, "application/json", strings.NewReader(body)) //nolint:noctx // Test setup against the local mock server.
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := response.Body.Close(); err != nil {
				t.Errorf("close mock response body: %v", err)
			}
		}()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("POST %s status = %d", path, response.StatusCode)
		}
		var result struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	project := createInMock("/projects", `{"name":"mock-project","code":"mock-project-code","description":"test"}`)
	folder := createInMock("/folders", `{"name":"mock-folder","description":"test"}`)

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(passwordFile, []byte("Password-Test-1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newPasswordFile := filepath.Join(t.TempDir(), "new-password.txt")
	if err := os.WriteFile(newPasswordFile, []byte("Password-Test-5678"), 0o600); err != nil {
		t.Fatal(err)
	}

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("run(%v) exit code = %d; stderr: %s", args, exitCode, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.Bytes()
	}

	// user create
	var created struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Code        string `json:"code"`
		Description string `json:"description"`
		Email       string `json:"email"`
	}
	if err := json.Unmarshal(runCommand(
		"iam-api", "user", "create",
		"--name", "mock-user", "--code", "mock-user-code", "--description", "test user",
		"--password-file", passwordFile,
	), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "mock-user" || created.Code != "mock-user-code" {
		t.Fatalf("create returned %#v, want user ID, name and code", created)
	}
	userID := created.ID

	// user list (array output)
	var listed []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "user", "list"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != userID {
		t.Fatalf("list returned %#v, want user %d", listed, userID)
	}
	var projected []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "user", "list", "--query", "map({id,name})"), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != userID || projected[0].Name != "mock-user" {
		t.Fatalf("projected output = %+v, want created user ID and name", projected)
	}

	// user read
	var read struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &read); err != nil {
		t.Fatal(err)
	}
	if read.ID != userID || read.Name != "mock-user" {
		t.Fatalf("read returned %#v, want user %d", read, userID)
	}

	// user update without password file keeps the password
	var updated struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(runCommand(
		"iam-api", "user", "update", strconv.Itoa(userID), "--name", "renamed-user", "--description", "updated",
	), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed-user" || updated.Description != "updated" {
		t.Fatalf("update returned %#v, want renamed-user and updated", updated)
	}

	// user update with password file
	if err := json.Unmarshal(runCommand(
		"iam-api", "user", "update", strconv.Itoa(userID),
		"--name", "renamed-user", "--description", "updated", "--password-file", newPasswordFile,
	), &updated); err != nil {
		t.Fatal(err)
	}

	// register-email / unregister-email
	runCommand("iam-api", "user", "register-email", strconv.Itoa(userID), "--email", "user@example.com")
	var withEmail struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &withEmail); err != nil {
		t.Fatal(err)
	}
	if withEmail.Email != "user@example.com" {
		t.Fatalf("email = %q, want user@example.com", withEmail.Email)
	}
	runCommand("iam-api", "user", "unregister-email", strconv.Itoa(userID))
	if err := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &withEmail); err != nil {
		t.Fatal(err)
	}
	if withEmail.Email != "" {
		t.Fatalf("email after unregister = %q, want empty", withEmail.Email)
	}

	// group create / memberships
	var groupCreated struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(runCommand(
		"iam-api", "group", "create", "--name", "mock-group", "--description", "test group",
	), &groupCreated); err != nil {
		t.Fatal(err)
	}
	if groupCreated.ID == 0 || groupCreated.Name != "mock-group" {
		t.Fatalf("group create returned %#v, want group ID and name", groupCreated)
	}
	groupID := groupCreated.ID

	var memberships []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "group", "update-memberships", strconv.Itoa(groupID), "--request", `[`+strconv.Itoa(userID)+`]`), &memberships); err != nil {
		t.Fatal(err)
	}
	if len(memberships) != 1 || memberships[0].ID != userID {
		t.Fatalf("update-memberships returned %#v, want user %d", memberships, userID)
	}
	memberships = nil
	if err := json.Unmarshal(runCommand("iam-api", "group", "read-memberships", strconv.Itoa(groupID)), &memberships); err != nil {
		t.Fatal(err)
	}
	if len(memberships) != 1 || memberships[0].ID != userID {
		t.Fatalf("read-memberships returned %#v, want user %d", memberships, userID)
	}

	// group list filtered by user
	var groupListed []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "group", "list", "--user-id", strconv.Itoa(userID)), &groupListed); err != nil {
		t.Fatal(err)
	}
	if len(groupListed) != 1 || groupListed[0].ID != groupID {
		t.Fatalf("group list returned %#v, want group %d", groupListed, groupID)
	}

	// group update / read
	var groupUpdated struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(runCommand(
		"iam-api", "group", "update", strconv.Itoa(groupID), "--name", "renamed-group", "--description", "renamed",
	), &groupUpdated); err != nil {
		t.Fatal(err)
	}
	if groupUpdated.Name != "renamed-group" || groupUpdated.Description != "renamed" {
		t.Fatalf("group update returned %#v, want renamed-group and renamed", groupUpdated)
	}

	// IAM policy bindings
	bindingsRequest := `[{"role":{"type":"preset","id":"iam-role-id"},"principals":[{"type":"user","id":1}]}]`
	for _, target := range []struct {
		operation string
		id        string
	}{
		{operation: "update-organization"},
		{operation: "update-project", id: strconv.Itoa(project.ID)},
		{operation: "update-folder", id: strconv.Itoa(folder.ID)},
	} {
		args := []string{"iam-api", "policy", target.operation}
		if target.id != "" {
			args = append(args, target.id)
		}
		args = append(args, "--request", bindingsRequest)
		var updated []struct {
			Role struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"role"`
			Principals []struct {
				Type string `json:"type"`
				ID   int    `json:"id"`
			} `json:"principals"`
		}
		if err := json.Unmarshal(runCommand(args...), &updated); err != nil {
			t.Fatal(err)
		}
		if len(updated) != 1 || updated[0].Role.ID != "iam-role-id" || len(updated[0].Principals) != 1 || updated[0].Principals[0].ID != 1 {
			t.Fatalf("%s returned %#v, want binding for iam-role-id and user 1", target.operation, updated)
		}
	}

	var organizationBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "policy", "read-organization"), &organizationBindings); err != nil {
		t.Fatal(err)
	}
	if len(organizationBindings) != 1 || organizationBindings[0].Role.ID != "iam-role-id" {
		t.Fatalf("read-organization returned %#v, want binding for iam-role-id", organizationBindings)
	}
	var projectBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "policy", "read-project", strconv.Itoa(project.ID)), &projectBindings); err != nil {
		t.Fatal(err)
	}
	if len(projectBindings) != 1 || projectBindings[0].Role.ID != "iam-role-id" {
		t.Fatalf("read-project returned %#v, want binding for iam-role-id", projectBindings)
	}
	var folderBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	if err := json.Unmarshal(runCommand("iam-api", "policy", "read-folder", strconv.Itoa(folder.ID)), &folderBindings); err != nil {
		t.Fatal(err)
	}
	if len(folderBindings) != 1 || folderBindings[0].Role.ID != "iam-role-id" {
		t.Fatalf("read-folder returned %#v, want binding for iam-role-id", folderBindings)
	}

	// delete user and group, then verify the lists are empty
	runCommand("iam-api", "user", "delete", strconv.Itoa(userID))
	runCommand("iam-api", "group", "delete", strconv.Itoa(groupID))
	listed = nil
	if err := json.Unmarshal(runCommand("iam-api", "user", "list"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("user list after delete returned %#v, want empty", listed)
	}
	emptyJSON := string(runCommand("iam-api", "user", "list", "--query", "map({id,name})"))
	if want := "[]\n"; emptyJSON != want {
		t.Errorf("empty projected output = %q, want %q", emptyJSON, want)
	}
	groupListed = nil
	if err := json.Unmarshal(runCommand("iam-api", "group", "list"), &groupListed); err != nil {
		t.Fatal(err)
	}
	if len(groupListed) != 0 {
		t.Errorf("group list after delete returned %#v, want empty", groupListed)
	}
}

func TestIAMAPIPasswordFromStdin(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	// Replace standard input with a pipe carrying the password.
	stdin := os.Stdin
	t.Cleanup(func() { os.Stdin = stdin })
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.WriteString("Password-Stdin-1234\n"); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = read

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{
		"iam-api", "user", "create",
		"--name", "stdin-user", "--code", "stdin-user-code", "--description", "test",
		"--password-file", "-",
	}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run exit code = %d; stderr: %s", exitCode, stderr.String())
	}
	var created struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "stdin-user" {
		t.Fatalf("create returned %#v, want stdin-user", created)
	}
}

func TestIAMAPICommandInputErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(passwordFile, []byte("Password-Test-1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"iam-api", "user", "create", "--name", "mock-user", "--code", "mock-user-code", "--description", "test"},
		{"iam-api", "user", "create", "--name", "mock-user", "--description", "test", "--password-file", passwordFile},
		{"iam-api", "user", "create", "--code", "mock-user-code", "--description", "test", "--password-file", passwordFile},
		{"iam-api", "user", "create", "--name", "mock-user", "--code", "mock-user-code", "--description", "test", "--password-file", "/missing/password.txt"},
		{"iam-api", "user", "update", "1", "--name", "renamed-user"},
		{"iam-api", "user", "update", "1", "--description", "updated"},
		{"iam-api", "group", "create", "--name", "mock-group"},
		{"iam-api", "group", "create", "--description", "test"},
		{"iam-api", "group", "update", "1", "--name", "renamed-group"},
		{"iam-api", "group", "update-memberships", "1"},
		{"iam-api", "policy", "update-organization"},
		{"iam-api", "policy", "update-project", "1"},
		{"iam-api", "user", "register-email", "1"},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("run(%v) = %d stdout %q stderr %q; want an explicit error", args, exitCode, stdout.String(), stderr.String())
		}
	}
}

func TestIAMAPIDecodeRequestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memberships.json")
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o600); err != nil {
		t.Fatal(err)
	}
	var value []int
	if err := iamapi.DecodeRequest("@"+path, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 3 || value[0] != 1 || value[2] != 3 {
		t.Fatalf("decoded %#v, want [1 2 3]", value)
	}
	if err := iamapi.DecodeRequest("@/missing/memberships.json", &value); err == nil {
		t.Error("DecodeRequest(@missing) succeeded, want an error")
	}
	if err := iamapi.DecodeRequest("{invalid", &value); err == nil {
		t.Error("DecodeRequest(invalid JSON) succeeded, want an error")
	}
}
