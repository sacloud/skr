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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	iammock "github.com/sacloud/sakumock/iam"
	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestRunIAMAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{
			args: []string{"iam-api", "--help"},
			want: []string{"user", "group", "policy", "auth", "folder", "iam-role", "id-policy", "id-role", "organization", "project", "project-api-key", "scim", "service-policy", "service-principal", "sso", "user-2fa", "SAKURA_ACCESS_TOKEN", "サービスプリンシパル"},
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
		{
			args: []string{"iam-api", "folder", "create", "--help"},
			want: []string{"--name", "--description", "--parent-id", "JSON"},
		},
		{
			args: []string{"iam-api", "folder", "move", "--help"},
			want: []string{"--ids", "--parent-id", "カンマ区切り"},
		},
		{
			args: []string{"iam-api", "project", "move", "--help"},
			want: []string{"--ids", "--parent-folder-id", "カンマ区切り"},
		},
		{
			args: []string{"iam-api", "auth", "update-auth-conditions", "--help"},
			want: []string{"--request", "認証条件", "組織全体"},
		},
		{
			args: []string{"iam-api", "auth", "update-password-policy", "--help"},
			want: []string{"--request", "--min-length", "--require-uppercase", "--require-lowercase", "--require-symbols", "4項目すべて"},
		},
		{
			args: []string{"iam-api", "organization", "read-service-policy", "--help"},
			want: []string{"--request", "--is-active", "--is-dry-run", "--name", "--code", "--type"},
		},
		{
			args: []string{"iam-api", "scim", "update", "--help"},
			want: []string{"--request", "--name", "name は必須"},
		},
		{
			args: []string{"iam-api", "service-principal", "update", "--help"},
			want: []string{"--request", "--name", "--description", "name と description は必須"},
		},
		{
			args: []string{"iam-api", "service-principal", "list-keys", "--help"},
			want: []string{"--request", "--page", "--per-page", "--ordering"},
		},
		{
			args: []string{"iam-api", "service-principal", "issue-token", "--help"},
			want: []string{"--assertion-file", "標準入力"},
		},
		{
			args:    []string{"iam-api", "project-api-key", "create", "--help"},
			want:    []string{"--request", "IAM ロール配列", "iam_roles", "@path.json", "秘密情報"},
			notWant: []string{"--project-id", "--name"},
		},
		{
			args: []string{"iam-api", "user-2fa", "clear-trusted-devices", "--help"},
			want: []string{"--user-id", "すべて削除"},
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
				require.NotContains(t, help, text, "help output unexpectedly contains %q", text)
			}
		})
	}
}

func TestIAMGeneratedCodeMatchesConfigs(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{"api/commands/iam-auth.json", "internal/iamapi/iam-auth_api_generated.go"},
		{"api/commands/iam-folder.json", "internal/iamapi/iam-folder_api_generated.go"},
		{"api/commands/iam-group.json", "internal/iamapi/iam_group_api_generated.go"},
		{"api/commands/iam-id-policy.json", "internal/iamapi/iam-id-policy_api_generated.go"},
		{"api/commands/iam-id-role.json", "internal/iamapi/iam-id-role_api_generated.go"},
		{"api/commands/iam-organization.json", "internal/iamapi/iam-organization_api_generated.go"},
		{"api/commands/iam-policy.json", "internal/iamapi/iam_policy_api_generated.go"},
		{"api/commands/iam-project-api-key.json", "internal/iamapi/iam-project-api-key_api_generated.go"},
		{"api/commands/iam-project.json", "internal/iamapi/iam-project_api_generated.go"},
		{"api/commands/iam-role.json", "internal/iamapi/iam-role_api_generated.go"},
		{"api/commands/iam-scim.json", "internal/iamapi/iam-scim_api_generated.go"},
		{"api/commands/iam-service-policy.json", "internal/iamapi/iam-service-policy_api_generated.go"},
		{"api/commands/iam-service-principal.json", "internal/iamapi/iam-service-principal_api_generated.go"},
		{"api/commands/iam-sso.json", "internal/iamapi/iam-sso_api_generated.go"},
		{"api/commands/iam-user-2fa.json", "internal/iamapi/iam-user-2fa_api_generated.go"},
		{"api/commands/iam-user.json", "internal/iamapi/iam_user_api_generated.go"},
	} {
		configData, err := os.ReadFile(repositoryPath(test.config))
		require.NoError(t, err)
		config, err := apigen.DecodeConfig(configData)
		require.NoError(t, err, "decode %s", test.config)
		want, err := apigen.Generate(config)
		require.NoError(t, err, "generate %s", test.config)
		got, err := os.ReadFile(repositoryPath(test.output))
		require.NoError(t, err)
		require.Equal(t, want, got, "generated IAM API command is stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
	}
}

func TestIAMAPIExtendedReadOperationsWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	err := os.WriteFile(passwordFile, []byte("Password-Test-1234"), 0o600)
	require.NoError(t, err)
	var user struct {
		ID int `json:"id"`
	}
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{
		"iam-api", "user", "create", "--name", "mock-2fa-user", "--code", "mock-2fa-user-code",
		"--description", "test 2FA user", "--password-file", passwordFile,
	}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("create test user exit code = %d; stderr: %s", exitCode, stderr.String())
	}
	err2 := json.Unmarshal(stdout.Bytes(), &user)
	require.NoError(t, err2)
	require.NotEqualValues(t, 0, user.ID, "created test user has no ID")
	t.Cleanup(func() {
		var cleanupOut, cleanupErr bytes.Buffer
		require.Equal(t, 0, run([]string{"iam-api", "user", "delete", strconv.Itoa(user.ID)}, &cleanupOut, &cleanupErr), "unexpected exit code")
	})

	for _, args := range [][]string{
		{"iam-api", "auth", "read-password-policy"},
		{"iam-api", "auth", "read-auth-context"},
		{"iam-api", "folder", "list"},
		{"iam-api", "iam-role", "list"},
		{"iam-api", "id-policy", "read-organization"},
		{"iam-api", "id-role", "list"},
		{"iam-api", "organization", "read"},
		{"iam-api", "organization", "read-service-policy"},
		{"iam-api", "project", "list"},
		{"iam-api", "project-api-key", "list"},
		{"iam-api", "scim", "list"},
		{"iam-api", "service-policy", "is-enabled"},
		{"iam-api", "service-policy", "list-rule-templates"},
		{"iam-api", "service-principal", "list"},
		{"iam-api", "sso", "list"},
		{"iam-api", "user-2fa", "list-trusted-devices", "--user-id", strconv.Itoa(user.ID)},
		{"iam-api", "user-2fa", "list-security-keys", "--user-id", strconv.Itoa(user.ID)},
	} {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	}
}

func TestIAMAPIFolderProjectAndServicePrincipalWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	var folder struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err := json.Unmarshal(runCommand("iam-api", "folder", "create", "--name", "mock-folder", "--description", "test folder"), &folder)
	require.NoError(t, err)
	require.NotEqualValues(t, 0, folder.ID, "folder create returned %#v", folder)
	require.Equal(t, "mock-folder", folder.Name, "unexpected value")
	var childFolder struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "folder", "create", "--name", "mock-child-folder", "--description", "child folder",
	), &childFolder))
	runCommand(
		"iam-api", "folder", "move", "--ids", strconv.Itoa(childFolder.ID), "--parent-id", strconv.Itoa(folder.ID),
	)
	var movedFolder struct {
		ParentID *int `json:"parent_id"`
	}
	err2 := json.Unmarshal(runCommand("iam-api", "folder", "read", strconv.Itoa(childFolder.ID)), &movedFolder)
	require.NoError(t, err2)
	if movedFolder.ParentID == nil || *movedFolder.ParentID != folder.ID {
		t.Fatalf("moved folder parent ID = %v, want %d", movedFolder.ParentID, folder.ID)
	}
	var projectedFolders []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "folder", "list", "--page", "1", "--query",
		".items | map({id,name,description})",
	), &projectedFolders))
	foundRootFolder := false
	for _, projected := range projectedFolders {
		if projected.ID == folder.ID && projected.Description == "test folder" {
			foundRootFolder = true
			break
		}
	}
	require.Len(t, projectedFolders, 2, "projected folder list = %#v, want both created folders including %d", projectedFolders, folder.ID)
	require.True(t, foundRootFolder, "unexpected value")
	var folderList struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
	}
	err3 := json.Unmarshal(runCommand("iam-api", "folder", "list", "--name", "mock-folder"), &folderList)
	require.NoError(t, err3)
	foundListedFolder := false
	for _, listed := range folderList.Items {
		if listed.ID == folder.ID {
			foundListedFolder = true
			break
		}
	}
	require.True(t, foundListedFolder, "folder list returned %#v, want folder %d", folderList, folder.ID)
	runCommand("iam-api", "folder", "update", strconv.Itoa(folder.ID), "--name", "mock-folder-updated", "--description", "updated")

	var project struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "project", "create", "--code", "mock-project-code", "--name", "mock-project", "--description", "test project",
	), &project))
	require.NotEqualValues(t, 0, project.ID, "project create returned %#v", project)
	require.Equal(t, "mock-project", project.Name, "unexpected value")
	runCommand(
		"iam-api", "project", "move", "--ids", strconv.Itoa(project.ID), "--parent-folder-id", strconv.Itoa(folder.ID),
	)
	var movedProject struct {
		ParentFolderID *int `json:"parent_folder_id"`
	}
	err4 := json.Unmarshal(runCommand("iam-api", "project", "read", strconv.Itoa(project.ID)), &movedProject)
	require.NoError(t, err4)
	if movedProject.ParentFolderID == nil || *movedProject.ParentFolderID != folder.ID {
		t.Fatalf("moved project parent folder ID = %v, want %d", movedProject.ParentFolderID, folder.ID)
	}
	var servicePrincipal struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "service-principal", "create", "--project-id", strconv.Itoa(project.ID),
		"--name", "mock-service-principal", "--description", "test principal",
	), &servicePrincipal))
	require.NotEqualValues(t, 0, servicePrincipal.ID, "service principal create returned %#v", servicePrincipal)
	require.Equal(t, "mock-service-principal", servicePrincipal.Name, "unexpected value")
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "service-principal", "update", strconv.Itoa(servicePrincipal.ID),
		"--name", "mock-service-principal-flag-updated",
		"--description", "updated principal",
	), &servicePrincipal))
	require.Equal(t, "mock-service-principal-flag-updated", servicePrincipal.Name, "service principal flag update returned %#v", servicePrincipal)
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "service-principal", "update", strconv.Itoa(servicePrincipal.ID), "--request",
		`{"name":"mock-service-principal-json-updated","description":"updated principal"}`,
	), &servicePrincipal))
	require.Equal(t, "mock-service-principal-json-updated", servicePrincipal.Name, "service principal JSON update returned %#v", servicePrincipal)
	require.Equal(t, "updated principal", servicePrincipal.Description, "unexpected value")

	runCommand("iam-api", "service-principal", "delete", strconv.Itoa(servicePrincipal.ID))
	runCommand("iam-api", "project", "delete", strconv.Itoa(project.ID))
	runCommand("iam-api", "folder", "delete", strconv.Itoa(childFolder.ID))
	runCommand("iam-api", "folder", "delete", strconv.Itoa(folder.ID))
}

func TestIAMAPIExtendedMutationsWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}
	postMock := func(path, body string) []byte {
		t.Helper()
		response, err := http.Post(server.TestURL()+path, "application/json", strings.NewReader(body)) //nolint:noctx // ローカルモックサーバーへのテスト準備。
		require.NoError(t, err)
		defer func() {
			err := response.Body.Close()
			require.NoError(t, err, "close mock response body: %v")
		}()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("POST %s status = %d", path, response.StatusCode)
		}
		var result bytes.Buffer
		if _, err := result.ReadFrom(response.Body); err != nil {
			t.Fatal(err)
		}
		return result.Bytes()
	}

	var passwordPolicy struct {
		MinLength        int  `json:"min_length"`
		RequireUppercase bool `json:"require_uppercase"`
		RequireLowercase bool `json:"require_lowercase"`
		RequireSymbols   bool `json:"require_symbols"`
	}
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "auth", "update-password-policy", "--request",
		`{"min_length":12,"require_uppercase":true,"require_lowercase":true,"require_symbols":false}`), &passwordPolicy))
	require.EqualValues(t, 12, passwordPolicy.MinLength, "updated password policy = %#v, want minimum length 12", passwordPolicy)
	require.True(t, passwordPolicy.RequireUppercase, "unexpected value")
	require.True(t, passwordPolicy.RequireLowercase, "unexpected value")
	require.False(t, passwordPolicy.RequireSymbols, "unexpected value")
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "auth", "update-password-policy", "--min-length", "14",
		"--require-uppercase", "--require-lowercase=false", "--require-symbols=false",
	), &passwordPolicy))
	require.EqualValues(t, 14, passwordPolicy.MinLength, "flag-updated password policy = %#v", passwordPolicy)
	require.True(t, passwordPolicy.RequireUppercase, "unexpected value")
	require.False(t, passwordPolicy.RequireLowercase, "unexpected value")
	require.False(t, passwordPolicy.RequireSymbols, "unexpected value")
	authConditions := `{"datetime_restriction":{"after":"2025-09-01T00:00:00+09:00","before":"2025-10-01T00:00:00+09:00"},"ip_restriction":{"mode":"allow_all"},"require_two_factor_auth":{"enabled":false}}`
	got := runCommand("iam-api", "auth", "update-auth-conditions", "--request", authConditions)
	require.NotEmpty(t, got, "update-auth-conditions returned an empty response")
	got2 := runCommand("iam-api", "id-policy", "update-organization", "--request", "[]")
	require.NotEmpty(t, got2, "update-organization ID policy returned an empty response")
	var organization struct {
		Name string `json:"name"`
	}
	err := json.Unmarshal(runCommand("iam-api", "organization", "update", "--name", "mock-organization-updated"), &organization)
	require.NoError(t, err)
	require.Equal(t, "mock-organization-updated", organization.Name, "organization update returned %#v", organization)
	got3 := runCommand("iam-api", "organization", "update-service-policy", "--request", "[]")
	require.NotEmpty(t, got3, "update-service-policy returned an empty response")
	runCommand("iam-api", "organization", "read-service-policy",
		"--is-active=false", "--is-dry-run=true", "--name", "mock-service", "--code", "mock-code", "--type", "bool")

	var apiKey struct {
		ID                int      `json:"id"`
		Name              string   `json:"name"`
		IAMRoles          []string `json:"iam_roles"`
		AccessTokenSecret string   `json:"access_token_secret"`
	}
	var invalidKeyOut, invalidKeyErr bytes.Buffer
	if exitCode := run([]string{"iam-api", "project-api-key", "create", "--request",
		`{"project_id":1,"name":"invalid-key","description":"missing roles"}`}, &invalidKeyOut, &invalidKeyErr); exitCode == 0 || !strings.Contains(invalidKeyErr.String(), "iam_roles") {
		t.Fatalf("API key create without IAM roles exit = %d, stderr = %q; want IAM role validation error", exitCode, invalidKeyErr.String())
	}
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "project-api-key", "create", "--request",
		`{"project_id":1,"name":"mock-api-key","description":"test key","iam_roles":["role-1"]}`), &apiKey))
	require.NotEqualValues(t, 0, apiKey.ID, "API key create returned %#v; expected ID, secret, and required IAM role", apiKey)
	require.NotEmpty(t, apiKey.AccessTokenSecret, "should not be empty")
	require.Len(t, apiKey.IAMRoles, 1, "unexpected length")
	require.Equal(t, "role-1", apiKey.IAMRoles[0], "unexpected value")
	var updatedAPIKey struct {
		Name     string   `json:"name"`
		IAMRoles []string `json:"iam_roles"`
	}
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "project-api-key", "update", strconv.Itoa(apiKey.ID), "--request",
		`{"name":"mock-api-key-updated","description":"updated key","iam_roles":["role-2"]}`), &updatedAPIKey))
	require.Equal(t, "mock-api-key-updated", updatedAPIKey.Name, "API key update returned %#v", updatedAPIKey)
	require.Len(t, updatedAPIKey.IAMRoles, 1, "unexpected length")
	require.Equal(t, "role-2", updatedAPIKey.IAMRoles[0], "unexpected value")
	runCommand("iam-api", "project-api-key", "delete", strconv.Itoa(apiKey.ID))

	var scim struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	err2 := json.Unmarshal(runCommand("iam-api", "scim", "create", "--name", "mock-scim"), &scim)
	require.NoError(t, err2)
	require.NotEmpty(t, scim.ID, "SCIM create returned %#v", scim)
	require.Equal(t, "mock-scim", scim.Name, "unexpected value")
	err3 := json.Unmarshal(runCommand("iam-api", "scim", "update", scim.ID, "--request", `{"name":"mock-scim-updated"}`), &scim)
	require.NoError(t, err3)
	require.Equal(t, "mock-scim-updated", scim.Name, "SCIM update returned %#v", scim)
	err4 := json.Unmarshal(runCommand("iam-api", "scim", "update", scim.ID, "--name", "mock-scim-flag-updated"), &scim)
	require.NoError(t, err4)
	require.Equal(t, "mock-scim-flag-updated", scim.Name, "SCIM flag update returned %#v", scim)
	var regeneratedToken struct {
		SecretToken string `json:"secret_token"`
	}
	err5 := json.Unmarshal(runCommand("iam-api", "scim", "regenerate-token", scim.ID), &regeneratedToken)
	require.NoError(t, err5)
	require.NotEmpty(t, regeneratedToken.SecretToken, "SCIM token regeneration returned an empty secret")
	runCommand("iam-api", "scim", "delete", scim.ID)

	got4 := strings.TrimSpace(string(runCommand("iam-api", "service-policy", "enable")))
	require.Empty(t, got4, "service-policy enable output = %q, want empty", got4)
	got5 := strings.TrimSpace(string(runCommand("iam-api", "service-policy", "is-enabled")))
	require.Equal(t, "true", got5, "service-policy enabled status = %q, want true", got5)
	runCommand("iam-api", "service-policy", "disable")
	got6 := strings.TrimSpace(string(runCommand("iam-api", "service-policy", "is-enabled")))
	require.Equal(t, "false", got6, "service-policy enabled status after disable = %q, want false", got6)

	ssoRequest := `{"name":"mock-sso","description":"test","idp_entity_id":"https://idp.example.test/entity","idp_login_url":"https://idp.example.test/login","idp_logout_url":"https://idp.example.test/logout","idp_certificate":"test-certificate"}`
	var ssoProfile struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Assigned bool   `json:"assigned"`
	}
	err6 := json.Unmarshal(runCommand("iam-api", "sso", "create", "--request", ssoRequest), &ssoProfile)
	require.NoError(t, err6)
	require.NotEqualValues(t, 0, ssoProfile.ID, "SSO create returned %#v", ssoProfile)
	require.Equal(t, "mock-sso", ssoProfile.Name, "unexpected value")
	ssoID := strconv.Itoa(ssoProfile.ID)
	ssoUpdate := `{"name":"mock-sso-updated","description":"updated","idp_entity_id":"https://idp.example.test/entity","idp_login_url":"https://idp.example.test/login","idp_logout_url":"https://idp.example.test/logout","idp_certificate":"test-certificate"}`
	err7 := json.Unmarshal(runCommand("iam-api", "sso", "update", ssoID, "--request", ssoUpdate), &ssoProfile)
	require.NoError(t, err7)
	runCommand("iam-api", "sso", "link", ssoID)
	err8 := json.Unmarshal(runCommand("iam-api", "sso", "read", ssoID), &ssoProfile)
	require.NoError(t, err8)
	require.True(t, ssoProfile.Assigned, "SSO profile was not linked")
	runCommand("iam-api", "sso", "unlink", ssoID)
	err9 := json.Unmarshal(runCommand("iam-api", "sso", "read", ssoID), &ssoProfile)
	require.NoError(t, err9)
	require.False(t, ssoProfile.Assigned, "SSO profile remained linked after unlink")
	runCommand("iam-api", "sso", "delete", ssoID)

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	err10 := os.WriteFile(passwordFile, []byte("Password-Test-2FA-1234"), 0o600)
	require.NoError(t, err10)
	var user struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "user", "create",
		"--name", "mock-2fa-mutation-user", "--code", "mock-2fa-mutation-user",
		"--description", "test", "--password-file", passwordFile), &user))
	require.NotEqualValues(t, 0, user.ID, "created test user has no ID")
	userID := strconv.Itoa(user.ID)
	var trustedDevice struct {
		ID int `json:"id"`
	}
	err11 := json.Unmarshal(postMock("/_sakumock/users/"+userID+"/trusted-devices", `{"name":"test-device"}`), &trustedDevice)
	require.NoError(t, err11)
	runCommand("iam-api", "user-2fa", "delete-trusted-device", "--user-id", userID, strconv.Itoa(trustedDevice.ID))
	err12 := json.Unmarshal(postMock("/_sakumock/users/"+userID+"/trusted-devices", `{"name":"test-device-to-clear"}`), &trustedDevice)
	require.NoError(t, err12)
	runCommand("iam-api", "user-2fa", "clear-trusted-devices", "--user-id", userID)
	var securityKey struct {
		ID int `json:"id"`
	}
	err13 := json.Unmarshal(postMock("/_sakumock/users/"+userID+"/security-keys", `{"name":"test-security-key"}`), &securityKey)
	require.NoError(t, err13)
	runCommand("iam-api", "user-2fa", "delete-security-key", "--user-id", userID, strconv.Itoa(securityKey.ID))
	runCommand("iam-api", "user-2fa", "deactivate-otp", "--user-id", userID)
	runCommand("iam-api", "user", "delete", userID)

	project, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&project.PublicKey)
	require.NoError(t, err)
	publicKeyFile := filepath.Join(t.TempDir(), "public-key.pem")
	err14 := os.WriteFile(publicKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyDER}), 0o600)
	require.NoError(t, err14)
	var servicePrincipal struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "service-principal", "create", "--project-id", "1",
		"--name", "mock-key-operations", "--description", "test"), &servicePrincipal))
	var servicePrincipalKey struct {
		ID string `json:"id"`
	}
	servicePrincipalID := strconv.Itoa(servicePrincipal.ID)
	require.NoError(t, json.Unmarshal(runCommand("iam-api", "service-principal", "upload-key", servicePrincipalID,
		"--public-key-file", publicKeyFile), &servicePrincipalKey))
	require.NotEmpty(t, servicePrincipalKey.ID, "uploaded service principal key has no ID")
	var listedKeys struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "service-principal", "list-keys", servicePrincipalID,
		"--page", "1", "--per-page", "10", "--ordering", "created_at",
	), &listedKeys))
	foundKey := false
	for _, key := range listedKeys.Items {
		if key.ID == servicePrincipalKey.ID {
			foundKey = true
		}
	}
	require.True(t, foundKey, "service principal key list returned %#v, want key %s", listedKeys, servicePrincipalKey.ID)
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "service-principal", "list-keys", servicePrincipalID,
		"--request", `{"page":1,"per_page":10,"ordering":"created_at"}`,
	), &listedKeys))
	foundKey = false
	for _, key := range listedKeys.Items {
		if key.ID == servicePrincipalKey.ID {
			foundKey = true
		}
	}
	require.True(t, foundKey, "service principal key JSON list returned %#v, want key %s", listedKeys, servicePrincipalKey.ID)
	runCommand("iam-api", "service-principal", "disable-key", servicePrincipalID, servicePrincipalKey.ID)
	runCommand("iam-api", "service-principal", "enable-key", servicePrincipalID, servicePrincipalKey.ID)
	runCommand("iam-api", "service-principal", "delete-key", servicePrincipalID, servicePrincipalKey.ID)
	runCommand("iam-api", "service-principal", "delete", servicePrincipalID)
}

func TestIAMAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	// プロジェクト/フォルダの IAM ポリシー操作には既存のプロジェクトとフォルダが必要なため、
	// モック内でテスト準備として作成する。
	createInMock := func(path, body string) struct {
		ID int `json:"id"`
	} {
		t.Helper()
		response, err := http.Post(server.TestURL()+path, "application/json", strings.NewReader(body)) //nolint:noctx // ローカルモックサーバーへのテスト準備。
		require.NoError(t, err)
		defer func() {
			err := response.Body.Close()
			require.NoError(t, err, "close mock response body: %v")
		}()
		require.Equal(t, http.StatusCreated, response.StatusCode, "POST %s status = %d", path, response.StatusCode)
		var result struct {
			ID int `json:"id"`
		}
		err = json.NewDecoder(response.Body).Decode(&result)
		require.NoError(t, err)
		return result
	}
	project := createInMock("/projects", `{"name":"mock-project","code":"mock-project-code","description":"test"}`)
	folder := createInMock("/folders", `{"name":"mock-folder","description":"test"}`)

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	err := os.WriteFile(passwordFile, []byte("Password-Test-1234\n"), 0o600)
	require.NoError(t, err)
	newPasswordFile := filepath.Join(t.TempDir(), "new-password.txt")
	err2 := os.WriteFile(newPasswordFile, []byte("Password-Test-5678"), 0o600)
	require.NoError(t, err2)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}

	// user の作成
	var created struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Code        string `json:"code"`
		Description string `json:"description"`
		Email       string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "user", "create",
		"--name", "mock-user", "--code", "mock-user-code", "--description", "test user",
		"--password-file", passwordFile,
	), &created))
	require.NotEqualValues(t, 0, created.ID, "create returned %#v, want user ID, name and code", created)
	require.Equal(t, "mock-user", created.Name, "unexpected value")
	require.Equal(t, "mock-user-code", created.Code, "unexpected value")
	userID := created.ID

	// user の一覧（配列出力）
	var listed []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err3 := json.Unmarshal(runCommand("iam-api", "user", "list"), &listed)
	require.NoError(t, err3)
	require.Len(t, listed, 1, "list returned %#v, want user %d", listed, userID)
	require.Equal(t, userID, listed[0].ID, "unexpected value")
	var projected []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err4 := json.Unmarshal(runCommand("iam-api", "user", "list", "--query", "map({id,name})"), &projected)
	require.NoError(t, err4)
	require.Len(t, projected, 1, "projected output = %+v, want created user ID and name", projected)
	require.Equal(t, userID, projected[0].ID, "unexpected value")
	require.Equal(t, "mock-user", projected[0].Name, "unexpected value")

	// user の読み込み
	var read struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	err5 := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &read)
	require.NoError(t, err5)
	require.Equal(t, userID, read.ID, "read returned %#v, want user %d", read, userID)
	require.Equal(t, "mock-user", read.Name, "unexpected value")

	// パスワードファイルなしの user 更新はパスワードを保持する
	var updated struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "user", "update", strconv.Itoa(userID), "--name", "renamed-user", "--description", "updated",
	), &updated))
	require.Equal(t, "renamed-user", updated.Name, "update returned %#v, want renamed-user and updated", updated)
	require.Equal(t, "updated", updated.Description, "unexpected value")

	// パスワードファイルありの user 更新
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "user", "update", strconv.Itoa(userID),
		"--name", "renamed-user", "--description", "updated", "--password-file", newPasswordFile,
	), &updated))

	// register-email / unregister-email の実行
	runCommand("iam-api", "user", "register-email", strconv.Itoa(userID), "--email", "user@example.com")
	var withEmail struct {
		Email string `json:"email"`
	}
	err6 := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &withEmail)
	require.NoError(t, err6)
	require.Equal(t, "user@example.com", withEmail.Email, "email = %q, want user@example.com", withEmail.Email)
	runCommand("iam-api", "user", "unregister-email", strconv.Itoa(userID))
	err7 := json.Unmarshal(runCommand("iam-api", "user", "read", strconv.Itoa(userID)), &withEmail)
	require.NoError(t, err7)
	require.Empty(t, withEmail.Email, "email after unregister = %q, want empty", withEmail.Email)

	// group の作成 / メンバーシップ
	var groupCreated struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "group", "create", "--name", "mock-group", "--description", "test group",
	), &groupCreated))
	require.NotEqualValues(t, 0, groupCreated.ID, "group create returned %#v, want group ID and name", groupCreated)
	require.Equal(t, "mock-group", groupCreated.Name, "unexpected value")
	groupID := groupCreated.ID

	var memberships []struct {
		ID int `json:"id"`
	}
	err8 := json.Unmarshal(runCommand("iam-api", "group", "update-memberships", strconv.Itoa(groupID), "--request", `[`+strconv.Itoa(userID)+`]`), &memberships)
	require.NoError(t, err8)
	require.Len(t, memberships, 1, "update-memberships returned %#v, want user %d", memberships, userID)
	require.Equal(t, userID, memberships[0].ID, "unexpected value")
	memberships = nil
	err9 := json.Unmarshal(runCommand("iam-api", "group", "read-memberships", strconv.Itoa(groupID)), &memberships)
	require.NoError(t, err9)
	require.Len(t, memberships, 1, "read-memberships returned %#v, want user %d", memberships, userID)
	require.Equal(t, userID, memberships[0].ID, "unexpected value")

	// user で絞り込んだ group の一覧
	var groupListed []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err10 := json.Unmarshal(runCommand("iam-api", "group", "list", "--user-id", strconv.Itoa(userID)), &groupListed)
	require.NoError(t, err10)
	require.Len(t, groupListed, 1, "group list returned %#v, want group %d", groupListed, groupID)
	require.Equal(t, groupID, groupListed[0].ID, "unexpected value")

	// group の更新 / 読み込み
	var groupUpdated struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"iam-api", "group", "update", strconv.Itoa(groupID), "--name", "renamed-group", "--description", "renamed",
	), &groupUpdated))
	require.Equal(t, "renamed-group", groupUpdated.Name, "group update returned %#v, want renamed-group and renamed", groupUpdated)
	require.Equal(t, "renamed", groupUpdated.Description, "unexpected value")

	// IAM ポリシーの binding
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
		err11 := json.Unmarshal(runCommand(args...), &updated)
		require.NoError(t, err11)
		require.Len(t, updated, 1, "%s returned %#v, want binding for iam-role-id and user 1", target.operation, updated)
		require.Equal(t, "iam-role-id", updated[0].Role.ID, "unexpected value")
		require.Len(t, updated[0].Principals, 1, "unexpected length")
		require.EqualValues(t, 1, updated[0].Principals[0].ID, "unexpected value")
	}

	var organizationBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	err12 := json.Unmarshal(runCommand("iam-api", "policy", "read-organization"), &organizationBindings)
	require.NoError(t, err12)
	require.Len(t, organizationBindings, 1, "read-organization returned %#v, want binding for iam-role-id", organizationBindings)
	require.Equal(t, "iam-role-id", organizationBindings[0].Role.ID, "unexpected value")
	var projectBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	err13 := json.Unmarshal(runCommand("iam-api", "policy", "read-project", strconv.Itoa(project.ID)), &projectBindings)
	require.NoError(t, err13)
	require.Len(t, projectBindings, 1, "read-project returned %#v, want binding for iam-role-id", projectBindings)
	require.Equal(t, "iam-role-id", projectBindings[0].Role.ID, "unexpected value")
	var folderBindings []struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	err14 := json.Unmarshal(runCommand("iam-api", "policy", "read-folder", strconv.Itoa(folder.ID)), &folderBindings)
	require.NoError(t, err14)
	require.Len(t, folderBindings, 1, "read-folder returned %#v, want binding for iam-role-id", folderBindings)
	require.Equal(t, "iam-role-id", folderBindings[0].Role.ID, "unexpected value")

	// user と group を削除し、一覧が空になることを確認する
	runCommand("iam-api", "user", "delete", strconv.Itoa(userID))
	runCommand("iam-api", "group", "delete", strconv.Itoa(groupID))
	listed = nil
	err15 := json.Unmarshal(runCommand("iam-api", "user", "list"), &listed)
	require.NoError(t, err15)
	require.Len(t, listed, 0, "user list after delete returned %#v, want empty", listed)
	emptyJSON := string(runCommand("iam-api", "user", "list", "--query", "map({id,name})"))
	want := "[]\n"
	require.Equal(t, want, emptyJSON, "empty projected output = %q, want %q", emptyJSON, want)
	groupListed = nil
	err16 := json.Unmarshal(runCommand("iam-api", "group", "list"), &groupListed)
	require.NoError(t, err16)
	require.Len(t, groupListed, 0, "group list after delete returned %#v, want empty", groupListed)
}

func TestIAMAPIPasswordFromStdin(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	// 標準入力をパスワードを流すパイプに置き換える。
	stdin := os.Stdin
	t.Cleanup(func() { os.Stdin = stdin })
	read, write, err := os.Pipe()
	require.NoError(t, err)
	if _, err := write.WriteString("Password-Stdin-1234\n"); err != nil {
		t.Fatal(err)
	}
	err = write.Close()
	require.NoError(t, err)
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
	err2 := json.Unmarshal(stdout.Bytes(), &created)
	require.NoError(t, err2)
	require.Equal(t, "stdin-user", created.Name, "create returned %#v, want stdin-user", created)
}

func TestIAMAPICommandInputErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := iammock.NewTestServer(iammock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_IAM", server.TestURL())

	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	err := os.WriteFile(passwordFile, []byte("Password-Test-1234"), 0o600)
	require.NoError(t, err)
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
		{"iam-api", "auth", "update-password-policy"},
		{"iam-api", "auth", "update-password-policy", "--min-length", "12", "--require-uppercase", "--require-lowercase"},
		{"iam-api", "auth", "update-auth-conditions"},
		{"iam-api", "organization", "read-service-policy", "--request", "{}", "--name", "service"},
		{"iam-api", "scim", "update", "1"},
		{"iam-api", "scim", "update", "1", "--request", `{"name":"from-json"}`, "--name", "from-flag"},
		{"iam-api", "service-principal", "update", "1"},
		{"iam-api", "service-principal", "update", "1", "--request", `{"name":"from-json"}`, "--name", "from-flag"},
		{"iam-api", "service-principal", "list-keys", "1", "--request", "{}", "--page", "1"},
		{"iam-api", "id-policy", "update-organization"},
		{"iam-api", "folder", "list", "--request", "{}", "--page", "1"},
		{"iam-api", "user-2fa", "list-trusted-devices"},
		{"iam-api", "service-principal", "issue-token"},
		{"iam-api", "service-principal", "issue-token", "--assertion-file", "/missing/assertion.txt"},
		{"iam-api", "service-principal", "upload-key", "1"},
		{"iam-api", "service-principal", "upload-key", "1", "--public-key-file", "/missing/key.pem"},
	} {
		var stdout, stderr bytes.Buffer
		exitCode := run(args, &stdout, &stderr)
		require.NotEqual(t, 0, exitCode, "run(%v) = %d stdout %q stderr %q; want an explicit error", args, exitCode, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
}

func TestIAMAPIUserCreateRequiresPasswordFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{
		"iam-api", "user", "create",
		"--name", "mock-user", "--code", "mock-user-code", "--description", "test",
	}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("user create without --password-file succeeded")
	}
	require.EqualValues(t, 0, stdout.Len(), "user create output = %q, error = %q; want an explicit --password-file error", stdout.String(), stderr.String())
	require.Contains(t, stderr.String(), "--password-file が必要です", "missing expected text %q", "--password-file が必要です")
}
