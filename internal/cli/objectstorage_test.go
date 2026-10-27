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
		"object-storage-api site list",
		"object-storage-api account access-key create",
		"object-storage-api bucket encryption enable",
		"object-storage-api permission access-key create",
		"オブジェクト本体の転送ではなく",
		"作成完了後からバケットの基本料金が発生します",
	} {
		require.Contains(t, stdout.String(), text, "help output does not contain %q", text)
	}
}

func TestRunObjectStoragePermissionCreateHelpDocumentsJSONInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"object-storage-api", "permission", "create", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())

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
	sites := decodeObjectStorageResult[[]identity](t, runCommand("site", "list"))
	require.Len(t, sites, 3, "site count = %d, want 3", len(sites))
	accounts := map[string]string{}
	for _, site := range []string{"tky01", "isk01", "arc02"} {
		got := decodeObjectStorageResult[identity](t, runCommand("site", "read", site))
		require.Equal(t, site, got.ID, "site read returned ID %q, want %q", got.ID, site)
		requireAbsent("account", "read", site)
		created := decodeObjectStorageResult[identity](t, runCommand("account", "create", site))
		read := decodeObjectStorageResult[identity](t, runCommand("account", "read", site))
		require.NotEmpty(t, created.ResourceID, "account resource ID was missing or changed")
		require.Equal(t, created.ResourceID, read.ResourceID, "unexpected value")
		accounts[site] = created.ResourceID
		plans := decodeObjectStorageResult[[]struct {
			ClusterID string `json:"cluster_id"`
			Path      string `json:"service_class_path"`
		}](t, runCommand("site", "plans", site))
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
	checkKeys := func(path []string, target ...string) {
		t.Helper()
		args := func(operation string, id ...string) []string {
			result := append(append([]string{}, path...), operation)
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
	checkKeys([]string{"account", "access-key"}, "tky01")

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
		args := []string{"bucket", "create", item.site, item.name}
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
		list := decodeObjectStorageResult[[]bucket](t, runCommand("bucket", "list", item.site, "--query", "map({name,resource_id})"))
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
	created := decodeObjectStorageResult[permission](t, runCommand("permission", "create", "tky01", "--request", "@"+request))
	if created.ID <= 0 || created.DisplayName != "skr-test" || len(created.Controls) != 1 ||
		created.Controls[0] != (control{BucketName: "skr-test-source", CanRead: true, CanWrite: true}) {
		t.Fatal("permission creation did not preserve bucket controls")
	}
	permissionID := strconv.FormatInt(created.ID, 10)
	listed := decodeObjectStorageResult[[]permission](t, runCommand("permission", "list", "tky01"))
	require.Len(t, listed, 1, "permission list omitted the created ID")
	require.Equal(t, created.ID, listed[0].ID, "unexpected value")
	read := decodeObjectStorageResult[permission](t, runCommand("permission", "read", "tky01", permissionID))
	require.Equal(t, created.ID, read.ID, "permission read did not preserve identity")
	require.Equal(t, created.DisplayName, read.DisplayName, "unexpected value")
	checkKeys([]string{"permission", "access-key"}, "tky01", permissionID)
	updatedBody := strings.Replace(body, `"can_write":true`, `"can_write":false`, 1)
	updated := decodeObjectStorageResult[permission](t, runCommand("permission", "update", "tky01", permissionID, "--request", updatedBody))
	read = decodeObjectStorageResult[permission](t, runCommand("permission", "read", "tky01", permissionID))
	for _, value := range []permission{updated, read} {
		if value.ID != created.ID || value.DisplayName != created.DisplayName || len(value.Controls) != 1 ||
			value.Controls[0] != (control{BucketName: "skr-test-source", CanRead: true, CanWrite: false}) {
			t.Fatal("permission update did not change only the write permission")
		}
	}
	for _, args := range [][]string{
		{"site", "status", "read", "tky01"},
		{"site", "status", "quota", "tky01"},
		{"bucket", "penalty", "tky01", "skr-test-source"},
		{"bucket", "usage", "tky01", "skr-test-source"},
		{"bucket", "quota", "tky01", "skr-test-source"},
	} {
		result := decodeObjectStorageResult[map[string]json.RawMessage](t, runCommand(args...))
		require.NotEmpty(t, result, "%v returned no fields", args)
	}
	metering := decodeObjectStorageResult[[]json.RawMessage](t, runCommand("site", "status", "bucket-metering", "tky01", "skr-test-source",
		"--from", "2026-09-01T00:00:00Z", "--to", "2026-10-01T00:00:00Z"))
	require.Len(t, metering, 0, "sakumock returned unexpected usage history")

	const keyID = "12345678-1234-4234-9234-123456789abc"
	runCommand("bucket", "encryption", "enable", "tky01", "skr-test-source", "--kms-key-id", keyID)
	encryption := decodeObjectStorageResult[struct {
		KeyID string `json:"kms_key_id"`
	}](t, runCommand("bucket", "encryption", "read", "tky01", "skr-test-source"))
	require.Equal(t, keyID, encryption.KeyID, "encryption read did not preserve the selected key ID")
	runCommand("bucket", "encryption", "disable", "tky01", "skr-test-source")
	requireAbsent("bucket", "encryption", "read", "tky01", "skr-test-source")
	type replication struct {
		Source bucket `json:"source_bucket"`
		Target bucket `json:"dest_bucket"`
		Status string `json:"config_status"`
	}
	for _, operation := range []string{"enable", "read"} {
		args := []string{"bucket", "replication", operation, "tky01", "skr-test-source"}
		if operation == "enable" {
			args = append(args, "--target-bucket", "skr-test-target")
		}
		setting := decodeObjectStorageResult[replication](t, runCommand(args...))
		if setting.Source.Name != "skr-test-source" || setting.Source.ClusterID != "tky01" ||
			setting.Target.Name != "skr-test-target" || setting.Target.ClusterID != "isk01" || setting.Status != "created" {
			t.Fatal("replication source, destination, or status did not match")
		}
	}
	runCommand("bucket", "replication", "disable", "tky01", "skr-test-source")
	requireAbsent("bucket", "replication", "read", "tky01", "skr-test-source")
	runCommand("permission", "delete", "tky01", permissionID)
	list := decodeObjectStorageResult[[]permission](t, runCommand("permission", "list", "tky01"))
	require.Len(t, list, 0, "permission remains after deletion")
	for _, item := range buckets {
		list := decodeObjectStorageResult[[]bucket](t, runCommand("bucket", "list", item.site))
		require.Len(t, list, 1, "bucket identity changed before deletion")
		require.Equal(t, item.id, list[0].ResourceID, "unexpected value")
		runCommand("bucket", "delete", item.site, item.name)
		list2 := decodeObjectStorageResult[[]bucket](t, runCommand("bucket", "list", item.site))
		require.Len(t, list2, 0, "bucket remains after deletion")
		account := decodeObjectStorageResult[identity](t, runCommand("account", "read", item.site))
		if account.ResourceID != accounts[item.site] {
			t.Fatal("account identity changed before deletion")
		}
		runCommand("account", "delete", item.site)
		requireAbsent("account", "read", item.site)
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
	require.NotEqual(t, 0, run([]string{"object-storage-api", "site", "list"}, &stdout, &stderr), "run should fail")

	require.EqualValues(t, 0, stdout.Len(), "API error was hidden or produced success-shaped output")
	require.Contains(t, stderr.String(), "403", "missing expected text %q", "403")
}

func TestObjectStorageInvalidQueryBeforeAPIFactory(t *testing.T) {
	var commands [][]string
	for _, operation := range []string{"list", "read", "plans"} {
		args := []string{"site", operation}
		if operation != "list" {
			args = append(args, "tky01")
		}
		commands = append(commands, args)
	}
	for _, operation := range []string{"read", "quota", "bucket-metering"} {
		args := []string{"site", "status", operation, "tky01"}
		if operation == "bucket-metering" {
			args = append(args, "test-bucket", "--from", "2026-09-01T00:00:00Z", "--to", "2026-10-01T00:00:00Z")
		}
		commands = append(commands, args)
	}
	for _, operation := range []string{"create", "read", "delete"} {
		commands = append(commands, []string{"account", operation, "tky01"})
	}
	for _, operation := range []string{"list", "create", "delete", "penalty", "usage", "quota"} {
		args := []string{"bucket", operation, "tky01"}
		if operation != "list" {
			args = append(args, "test-bucket")
		}
		commands = append(commands, args)
	}
	for _, resource := range []string{"encryption", "replication"} {
		for _, operation := range []string{"read", "enable", "disable"} {
			args := []string{"bucket", resource, operation, "tky01", "test-bucket"}
			if operation == "enable" {
				flag := "--kms-key-id"
				if resource == "replication" {
					flag = "--target-bucket"
				}
				args = append(args, flag, "test-target")
			}
			commands = append(commands, args)
		}
	}
	for _, operation := range []string{"list", "create", "read", "update", "delete"} {
		args := []string{"permission", operation, "tky01"}
		if operation == "read" || operation == "update" || operation == "delete" {
			args = append(args, "1001")
		}
		if operation == "create" || operation == "update" {
			args = append(args, "--request", `{"display_name":"test","bucket_controls":[]}`)
		}
		commands = append(commands, args)
	}
	for _, resource := range []string{"account", "permission"} {
		for _, operation := range []string{"list", "create", "read", "delete"} {
			args := []string{resource, "access-key", operation, "tky01"}
			if resource == "permission" {
				args = append(args, "1001")
			}
			if operation == "read" || operation == "delete" {
				args = append(args, "test-key")
			}
			commands = append(commands, args)
		}
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
		call(true, "account", "create", site)
	}
	call(true, "bucket", "create", "isk01", "skr-site-source")
	call(true, "bucket", "create", "tky01", "skr-site-target")
	call(true, "bucket", "replication", "enable", "isk01", "skr-site-source", "--target-bucket", "skr-site-target")
	for _, operation := range []string{"read", "enable", "disable"} {
		args := []string{"bucket", "replication", operation, "tky01", "skr-site-source"}
		if operation == "enable" {
			args = append(args, "--target-bucket", "skr-site-target")
		}
		call(false, args...)
	}
	call(false, "bucket", "delete", "tky01", "skr-site-source")
	setting := decodeObjectStorageResult[struct {
		Target struct {
			Name string `json:"name"`
		} `json:"dest_bucket"`
	}](t, call(true, "bucket", "replication", "read", "isk01", "skr-site-source"))
	if setting.Target.Name != "skr-site-target" {
		t.Fatal("wrong-site operation changed replication")
	}
	buckets := decodeObjectStorageResult[[]struct {
		Name string `json:"name"`
	}](t, call(true, "bucket", "list", "isk01"))
	require.Len(t, buckets, 1, "wrong-site operation deleted the source bucket")
	require.Equal(t, "skr-site-source", buckets[0].Name, "unexpected value")
}
