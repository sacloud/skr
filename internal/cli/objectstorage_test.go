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
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	objectstoragemock "github.com/sacloud/sakumock/objectstorage"
	"github.com/sacloud/skr/internal/objectstorageapi"
	"github.com/stretchr/testify/require"
)

func TestRunObjectStorageAPIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"object-storage-api", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())
	for _, text := range []string{
		"object-storage-api get-clusters",
		"object-storage-api create-account-key",
		"object-storage-api put-bucket-encryption",
		"object-storage-api create-permission-key",
		"オブジェクト本体の転送ではなく",
		"作成完了後からバケットの基本料金が発生します",
	} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
}

func TestRunObjectStoragePermissionCreateHelpDocumentsJSONInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"object-storage-api", "create-permission", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())

	for _, text := range []string{"--request", "display_name", "bucket_controls", "@path.json"} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
}

func newObjectStorageMock(t *testing.T, cfg objectstoragemock.Config) *objectstoragemock.Server {
	t.Helper()
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_TRACE_MODE", "")
	server := objectstoragemock.NewTestServer(cfg)
	t.Cleanup(func() {
		violations := server.SpecViolations()
		require.Len(t, violations, 0, "sakumock returned %d OpenAPI specification violations", len(violations))
		server.Close()
	})
	t.Setenv("SAKURA_ENDPOINTS_OBJECT_STORAGE", server.TestURL())
	return server
}

func decodeObjectStorageResult[T any](t *testing.T, output []byte) T {
	t.Helper()
	var result T
	err := json.Unmarshal(output, &result)
	require.NoError(t, err, "decode Object Storage response: %v")
	return result
}

func TestObjectStorageAPIWithSakumock(t *testing.T) {
	newObjectStorageMock(t, objectstoragemock.Config{})
	var calls []string
	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(append([]string{"object-storage-api"}, args...), &stdout, &stderr), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "object-storage-api %v: unexpected stderr: %s", args, stderr.String())
		calls = append(calls, strings.Join(args, " "))
		return stdout.Bytes()
	}
	requireAbsent := func(args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"object-storage-api"}, args...), &stdout, &stderr)
		require.NotEqual(t, 0, code, "object-storage-api %v: expected API 404, got exit %d: %s", args, code, stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.Contains(t, stderr.String(), "API Error 404:", "missing expected text %q", "API Error 404:")
	}
	type identity struct {
		ID         string `json:"id"`
		ResourceID string `json:"resource_id"`
	}
	sites := decodeObjectStorageResult[[]identity](t, runCommand("get-clusters"))
	require.Len(t, sites, 3, "site count = %d, want 3", len(sites))
	accounts := map[string]string{}
	for _, site := range []string{"tky01", "isk01", "arc02"} {
		got := decodeObjectStorageResult[identity](t, runCommand("get-cluster", site))
		require.Equal(t, site, got.ID, "site read returned ID %q, want %q", got.ID, site)
		requireAbsent("get-account", site)
		created := decodeObjectStorageResult[identity](t, runCommand("create-account", site))
		read := decodeObjectStorageResult[identity](t, runCommand("get-account", site))
		require.NotEmpty(t, created.ResourceID, "account resource ID was missing or changed")
		require.Equal(t, created.ResourceID, read.ResourceID, "unexpected value")
		accounts[site] = created.ResourceID
		plans := decodeObjectStorageResult[[]struct {
			ClusterID string `json:"cluster_id"`
			Path      string `json:"service_class_path"`
		}](t, runCommand("get-plans", site))
		require.NotEmpty(t, plans, "site returned no plans")
		for _, plan := range plans {
			if plan.ClusterID != site || !strings.HasPrefix(plan.Path, "objectstorage/"+site+"/bucket") {
				t.Fatal("plan did not belong to the requested site")
			}
		}
	}
	type key struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	checkKeys := func(names map[string]string, target ...string) {
		t.Helper()
		args := func(operation string, id ...string) []string {
			result := append([]string{}, names[operation])
			result = append(result, target...)
			return append(result, id...)
		}
		keys := decodeObjectStorageResult[[]key](t, runCommand(args("list")...))
		require.Len(t, keys, 0, "new owner already has access keys")
		created := decodeObjectStorageResult[key](t, runCommand(args("create")...))
		require.NotEmpty(t, created.ID, "key creation omitted ID or secret")
		require.NotEmpty(t, created.Secret, "should not be empty")
		read := decodeObjectStorageResult[key](t, runCommand(args("read", created.ID)...))
		require.Equal(t, created.ID, read.ID, "key read changed ID or exposed the secret again")
		require.Empty(t, read.Secret, "should be empty")
		keys = decodeObjectStorageResult[[]key](t, runCommand(args("list")...))
		require.Len(t, keys, 1, "key list did not match the new key or exposed its secret")
		require.Equal(t, created.ID, keys[0].ID, "unexpected value")
		require.Empty(t, keys[0].Secret, "should be empty")
		runCommand(args("delete", created.ID)...)
		keys2 := decodeObjectStorageResult[[]key](t, runCommand(args("list")...))
		require.Len(t, keys2, 0, "key remains after deletion")
	}
	checkKeys(map[string]string{"list": "get-account-keys", "create": "create-account-key", "read": "get-account-key", "delete": "delete-account-key"}, "tky01")

	type bucket struct {
		Name       string `json:"name"`
		ClusterID  string `json:"cluster_id"`
		ResourceID string `json:"resource_id"`
		Plan       struct {
			Path string `json:"service_class_path"`
			Type string `json:"type"`
		} `json:"plan"`
	}
	buckets := []struct {
		site, name, plan, id string
	}{
		{site: "tky01", name: "skr-test-source"},
		{site: "isk01", name: "skr-test-target"},
		{site: "arc02", name: "skr-test-archive", plan: "1t"},
	}
	for i := range buckets {
		item := &buckets[i]
		args := []string{"create-bucket", item.site, item.name}
		wantPath, wantType := "objectstorage/"+item.site+"/bucket", "standard"
		if item.plan != "" {
			args = append(args, "--plan", item.plan)
			wantPath, wantType = wantPath+"/"+item.plan, "archive"
		}
		created := decodeObjectStorageResult[bucket](t, runCommand(args...))
		require.Equal(t, item.name, created.Name, "created bucket site, name, or plan did not match")
		require.Equal(t, item.site, created.ClusterID, "unexpected value")
		require.Equal(t, wantPath, created.Plan.Path, "unexpected value")
		require.Equal(t, wantType, created.Plan.Type, "unexpected value")
		list := decodeObjectStorageResult[[]bucket](t, runCommand("list-buckets", item.site, "--query", "map({name,resource_id})"))
		require.Len(t, list, 1, "bucket list omitted the new bucket identity")
		require.Equal(t, item.name, list[0].Name, "unexpected value")
		require.NotEmpty(t, list[0].ResourceID, "should not be empty")
		item.id = list[0].ResourceID
	}

	type control struct {
		BucketName string `json:"bucket_name"`
		CanRead    bool   `json:"can_read"`
		CanWrite   bool   `json:"can_write"`
	}
	type permission struct {
		ID          int64     `json:"id"`
		DisplayName string    `json:"display_name"`
		Controls    []control `json:"bucket_controls"`
	}
	request := filepath.Join(t.TempDir(), "permission.json")
	body := `{"display_name":"skr-test","bucket_controls":[{"bucket_name":"skr-test-source","can_read":true,"can_write":true}]}`
	err := os.WriteFile(request, []byte(body), 0o600)
	require.NoError(t, err)
	created := decodeObjectStorageResult[permission](t, runCommand("create-permission", "tky01", "--request", "@"+request))
	if created.ID <= 0 || created.DisplayName != "skr-test" || len(created.Controls) != 1 ||
		created.Controls[0] != (control{BucketName: "skr-test-source", CanRead: true, CanWrite: true}) {
		t.Fatal("permission creation did not preserve bucket controls")
	}
	permissionID := strconv.FormatInt(created.ID, 10)
	listed := decodeObjectStorageResult[[]permission](t, runCommand("get-permissions", "tky01"))
	require.Len(t, listed, 1, "permission list omitted the created ID")
	require.Equal(t, created.ID, listed[0].ID, "unexpected value")
	read := decodeObjectStorageResult[permission](t, runCommand("get-permission", "tky01", permissionID))
	require.Equal(t, created.ID, read.ID, "permission read did not preserve identity")
	require.Equal(t, created.DisplayName, read.DisplayName, "unexpected value")
	checkKeys(map[string]string{"list": "get-permission-keys", "create": "create-permission-key", "read": "get-permission-key", "delete": "delete-permission-key"}, "tky01", permissionID)
	updatedBody := strings.Replace(body, `"can_write":true`, `"can_write":false`, 1)
	updated := decodeObjectStorageResult[permission](t, runCommand("update-permission", "tky01", permissionID, "--request", updatedBody))
	read = decodeObjectStorageResult[permission](t, runCommand("get-permission", "tky01", permissionID))
	for _, value := range []permission{updated, read} {
		if value.ID != created.ID || value.DisplayName != created.DisplayName || len(value.Controls) != 1 ||
			value.Controls[0] != (control{BucketName: "skr-test-source", CanRead: true, CanWrite: false}) {
			t.Fatal("permission update did not change only the write permission")
		}
	}
	for _, args := range [][]string{
		{"get-status", "tky01"},
		{"get-quota", "tky01"},
		{"get-bucket-penalty", "tky01", "skr-test-source"},
		{"get-bucket-usage", "tky01", "skr-test-source"},
		{"get-bucket-quota", "tky01", "skr-test-source"},
	} {
		result := decodeObjectStorageResult[map[string]json.RawMessage](t, runCommand(args...))
		require.NotEmpty(t, result, "%v returned no fields", args)
	}
	metering := decodeObjectStorageResult[[]json.RawMessage](t, runCommand("get-bucket-metering", "tky01", "skr-test-source",
		"--from", "2026-09-01T00:00:00Z", "--to", "2026-10-01T00:00:00Z"))
	require.Len(t, metering, 0, "sakumock returned unexpected usage history")

	// kms_key_id は Object Storage API の ResourceID (数値) 形式に従う
	const keyID = "123456789012"
	runCommand("put-bucket-encryption", "tky01", "skr-test-source", "--kms-key-id", keyID)
	encryption := decodeObjectStorageResult[struct {
		KeyID string `json:"kms_key_id"`
	}](t, runCommand("get-bucket-encryption", "tky01", "skr-test-source"))
	require.Equal(t, keyID, encryption.KeyID, "encryption read did not preserve the selected key ID")
	runCommand("delete-bucket-encryption", "tky01", "skr-test-source")
	requireAbsent("get-bucket-encryption", "tky01", "skr-test-source")
	type replication struct {
		Source bucket `json:"source_bucket"`
		Target bucket `json:"dest_bucket"`
		Status string `json:"config_status"`
	}
	for _, operation := range []string{"post-bucket-replication", "get-bucket-replication"} {
		args := []string{operation, "tky01", "skr-test-source"}
		if operation == "post-bucket-replication" {
			args = append(args, "--target-bucket", "skr-test-target")
		}
		setting := decodeObjectStorageResult[replication](t, runCommand(args...))
		if setting.Source.Name != "skr-test-source" || setting.Source.ClusterID != "tky01" ||
			setting.Target.Name != "skr-test-target" || setting.Target.ClusterID != "isk01" || setting.Status != "created" {
			t.Fatal("replication source, destination, or status did not match")
		}
	}
	runCommand("delete-bucket-replication", "tky01", "skr-test-source")
	requireAbsent("get-bucket-replication", "tky01", "skr-test-source")
	runCommand("delete-permission", "tky01", permissionID)
	list := decodeObjectStorageResult[[]permission](t, runCommand("get-permissions", "tky01"))
	require.Len(t, list, 0, "permission remains after deletion")
	for _, item := range buckets {
		list := decodeObjectStorageResult[[]bucket](t, runCommand("list-buckets", item.site))
		require.Len(t, list, 1, "bucket identity changed before deletion")
		require.Equal(t, item.id, list[0].ResourceID, "unexpected value")
		runCommand("delete-bucket", item.site, item.name)
		list2 := decodeObjectStorageResult[[]bucket](t, runCommand("list-buckets", item.site))
		require.Len(t, list2, 0, "bucket remains after deletion")
		account := decodeObjectStorageResult[identity](t, runCommand("get-account", item.site))
		if account.ResourceID != accounts[item.site] {
			t.Fatal("account identity changed before deletion")
		}
		runCommand("delete-account", item.site)
		requireAbsent("get-account", item.site)
	}

	commands := objectstorageapi.NewCommands(objectstorageapi.Runtime{}, nil)
	parser, err := kong.New(&commands)
	require.NoError(t, err)
	var verifyCoverage func(*kong.Node, []string)
	verifyCoverage = func(node *kong.Node, path []string) {
		for _, child := range node.Children {
			if child.Type == kong.CommandNode {
				verifyCoverage(child, append(append([]string{}, path...), child.Name))
			}
		}
		if !node.Leaf() || node.Type != kong.CommandNode {
			return
		}
		prefix := strings.Join(path, " ") + " "
		for _, call := range calls {
			if strings.HasPrefix(call+" ", prefix) {
				return
			}
		}
		t.Errorf("no successful sakumock call for object-storage-api %s", strings.Join(path, " "))
	}
	verifyCoverage(parser.Model.Node, nil)
}

func TestObjectStorageSakumockErrorPropagation(t *testing.T) {
	newObjectStorageMock(t, objectstoragemock.Config{Fault: []string{"403:1"}})
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"object-storage-api", "get-clusters"}, &stdout, &stderr), "run should fail")

	require.EqualValues(t, 0, stdout.Len(), "API error was hidden or produced success-shaped output")
	require.Contains(t, stderr.String(), "403", "missing expected text %q", "403")
}

func TestObjectStorageInvalidQueryBeforeAPIFactory(t *testing.T) {
	var commands [][]string
	// 34 末端コマンドそれぞれについて、API ファクトリを呼ぶ前の --query 検証を確認します
	siteCommands := []string{"get-clusters", "get-cluster", "get-plans", "get-status", "get-quota", "get-bucket-metering"}
	for _, name := range siteCommands {
		args := []string{name}
		if name != "get-clusters" {
			args = append(args, "tky01")
		}
		if name == "get-bucket-metering" {
			args = append(args, "test-bucket", "--from", "2026-09-01T00:00:00Z", "--to", "2026-10-01T00:00:00Z")
		}
		commands = append(commands, args)
	}
	for _, name := range []string{"create-account", "get-account", "delete-account"} {
		commands = append(commands, []string{name, "tky01"})
	}
	for _, name := range []string{"list-buckets", "create-bucket", "delete-bucket", "get-bucket-penalty", "get-bucket-usage", "get-bucket-quota"} {
		args := []string{name, "tky01"}
		if name != "list-buckets" {
			args = append(args, "test-bucket")
		}
		commands = append(commands, args)
	}
	encryptionReplication := map[string]string{
		"get-bucket-encryption":     "--kms-key-id",
		"put-bucket-encryption":     "--kms-key-id",
		"delete-bucket-encryption":  "--kms-key-id",
		"get-bucket-replication":    "--target-bucket",
		"post-bucket-replication":   "--target-bucket",
		"delete-bucket-replication": "--target-bucket",
	}
	for _, name := range []string{"get-bucket-encryption", "put-bucket-encryption", "delete-bucket-encryption", "get-bucket-replication", "post-bucket-replication", "delete-bucket-replication"} {
		args := []string{name, "tky01", "test-bucket"}
		if flag := encryptionReplication[name]; name == "put-bucket-encryption" || name == "post-bucket-replication" {
			args = append(args, flag, "test-target")
		}
		commands = append(commands, args)
	}
	for _, name := range []string{"get-permissions", "create-permission", "get-permission", "update-permission", "delete-permission"} {
		args := []string{name, "tky01"}
		if name == "get-permission" || name == "update-permission" || name == "delete-permission" {
			args = append(args, "1001")
		}
		if name == "create-permission" || name == "update-permission" {
			args = append(args, "--request", `{"display_name":"test","bucket_controls":[]}`)
		}
		commands = append(commands, args)
	}
	accountKeys := map[string][]string{
		"get-account-keys":      nil,
		"create-account-key":    nil,
		"get-account-key":       {"test-key"},
		"delete-account-key":    {"test-key"},
		"get-permission-keys":   {"1001"},
		"create-permission-key": {"1001"},
		"get-permission-key":    {"1001", "test-key"},
		"delete-permission-key": {"1001", "test-key"},
	}
	for _, name := range []string{"get-account-keys", "create-account-key", "get-account-key", "delete-account-key", "get-permission-keys", "create-permission-key", "get-permission-key", "delete-permission-key"} {
		args := append([]string{name, "tky01"}, accountKeys[name]...)
		commands = append(commands, args)
	}
	require.Len(t, commands, 34, "query regression coverage = %d, want 34", len(commands))
	for _, args := range commands {
		for _, expression := range []string{"(", "unknown_function"} {
			t.Run(strings.Join(args, " ")+"/"+expression, func(t *testing.T) {
				commandLine := newCLI()
				commandLine.ObjectStorageAPI = objectstorageapi.NewCommands(objectstorageapi.Runtime{
					DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
				}, func(string) (objectstorageapi.API, error) {
					t.Fatal("API factory called with invalid query")
					return objectstorageapi.API{}, nil
				})
				var stdout, stderr bytes.Buffer
				input := append([]string{"object-storage-api"}, args...)
				input = append(input, "--query", expression)
				code := runCLI(input, &stdout, &stderr, commandLine)
				require.NotEqual(t, 0, code, "invalid query: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
				require.EqualValues(t, 0, stdout.Len(), "unexpected value")
				require.Contains(t, stderr.String(), "--query expression", "missing expected text %q", "--query expression")
			})
		}
	}
}

func TestObjectStorageSakumockRejectsWrongSite(t *testing.T) {
	newObjectStorageMock(t, objectstoragemock.Config{})
	call := func(wantSuccess bool, args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"object-storage-api"}, args...), &stdout, &stderr)
		if wantSuccess {
			require.Equal(t, 0, code, "%v: %s", args, stderr.String())
		} else if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "操作を拒否") {
			t.Fatalf("wrong-site operation %v: exit %d, stderr %s", args, code, stderr.String())
		}
		return stdout.Bytes()
	}
	for _, site := range []string{"isk01", "tky01"} {
		call(true, "create-account", site)
	}
	call(true, "create-bucket", "isk01", "skr-site-source")
	call(true, "create-bucket", "tky01", "skr-site-target")
	call(true, "post-bucket-replication", "isk01", "skr-site-source", "--target-bucket", "skr-site-target")
	for _, operation := range []string{"get-bucket-replication", "post-bucket-replication", "delete-bucket-replication"} {
		args := []string{operation, "tky01", "skr-site-source"}
		if operation == "post-bucket-replication" {
			args = append(args, "--target-bucket", "skr-site-target")
		}
		call(false, args...)
	}
	call(false, "delete-bucket", "tky01", "skr-site-source")
	setting := decodeObjectStorageResult[struct {
		Target struct {
			Name string `json:"name"`
		} `json:"dest_bucket"`
	}](t, call(true, "get-bucket-replication", "isk01", "skr-site-source"))
	if setting.Target.Name != "skr-site-target" {
		t.Fatal("wrong-site operation changed replication")
	}
	buckets := decodeObjectStorageResult[[]struct {
		Name string `json:"name"`
	}](t, call(true, "list-buckets", "isk01"))
	require.Len(t, buckets, 1, "wrong-site operation deleted the source bucket")
	require.Equal(t, "skr-site-source", buckets[0].Name, "unexpected value")
}
