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

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// registryPrefix は今回の実行が作成するレジストリ名の接頭辞です。
	registryPrefix = "skr-e2e-cr-"
	// descriptionPrefix は今回の実行が設定する説明の接頭辞です。
	descriptionPrefix = "skr-e2e-container-registry/"
	// userName は今回の実行が作成するレジストリユーザー名です。
	userName = "skr-e2e"
	// queryRegistries / queryUsers は --query で必要な項目だけを抽出する式です。
	queryRegistries = "map({ID,Name,SubDomainLabel,Description})"
	queryUsers      = "map({UserName,Permission})"
)

func TestScenarioRegistry(t *testing.T) {
	h := helper.New(t, "container-registry-api", "コンテナレジストリとそのユーザーの作成/更新/確認が可能であることを確認する")

	suffix := randomSuffix(t)
	name := registryPrefix + suffix
	description := descriptionPrefix + suffix
	updatedDescription := description + "/updated"
	passwordPath := writePasswordFile(t)

	findRequest := h.WriteRequestJSONFile(t, "find", map[string]any{"Names": []string{name}})

	// 1. 作成前に同名レジストリが存在しないことを確認する
	before := findRegistries(t, h, "作成前に同名レジストリが存在しないことを確認する", findRequest)
	require.Empty(t, before, "registry name %q already exists; refusing to modify it", name)

	// 2. レジストリを作成する
	data := h.Call(t, "コンテナレジストリを作成する",
		"container-registry-api", "registry", "create", "--name", name, "--description", description)
	createdRegistry := parseRegistry(t, data)
	require.Equal(t, name, createdRegistry.Name, "created registry name should match the requested name")
	require.Equal(t, description, createdRegistry.Description, "created registry description should match the requested description")
	defer func() {
		// 11. 作成したレジストリを削除し、一覧に残っていないことを確認する
		_ = h.Call(t, "作成したレジストリを削除する", "container-registry-api", "registry", "delete", createdRegistry.id(t))
		remaining := findRegistries(t, h, "削除後にレジストリが一覧に残っていないことを確認する", findRequest)
		require.Empty(t, remaining, "registry %s should not remain after deletion", name)
	}()

	// 3. 作成したレジストリが一覧に含まれることを確認する
	listed := findRegistries(t, h, "作成したレジストリが一覧に含まれることを確認する", findRequest)
	require.Len(t, listed, 1, "registry list should contain exactly one match for %q", name)
	requireRegistryIdentity(t, listed[0], createdRegistry.id(t), name, description)

	// 4. レジストリの説明を更新する
	updateRequest := h.WriteRequestJSONFile(t, "update", map[string]any{"Description": updatedDescription})
	data = h.Call(t, "レジストリの説明を更新する",
		"container-registry-api", "registry", "update", createdRegistry.id(t), "--request", "@"+updateRequest)
	requireRegistryIdentity(t, parseRegistry(t, data), createdRegistry.id(t), name, updatedDescription)

	// 5. 更新後のレジストリを参照する
	data = h.Call(t, "更新後のレジストリを参照する", "container-registry-api", "registry", "read", createdRegistry.id(t))
	requireRegistryIdentity(t, parseRegistry(t, data), createdRegistry.id(t), name, updatedDescription)

	// 6. レジストリユーザーを追加する
	_ = h.Call(t, "レジストリユーザーを追加する",
		"container-registry-api", "registry", "user", "add", createdRegistry.id(t),
		"--user-name", userName, "--password-file", passwordPath, "--permission", "readwrite")
	defer func() {
		// 10. ユーザーを削除し、残っていないことを確認する
		_ = h.Call(t, "ユーザーを削除する",
			"container-registry-api", "registry", "user", "delete", createdRegistry.id(t), userName)
		users := listUsers(t, h, "ユーザー削除後にユーザーが残っていないことを確認する", createdRegistry.id(t))
		require.Empty(t, users, "user should not remain after deletion")
	}()

	// 7. 追加したユーザーが一覧に含まれることを確認する
	users := listUsers(t, h, "追加したユーザーが一覧に含まれることを確認する", createdRegistry.id(t))
	require.Len(t, users, 1, "user list should contain exactly the user created by this run")
	require.Equal(t, userName, users[0].UserName)
	require.Equal(t, "readwrite", users[0].Permission)

	// 8. ユーザーの権限を変更する
	_ = h.Call(t, "ユーザーの権限を変更する",
		"container-registry-api", "registry", "user", "update", createdRegistry.id(t), userName, "--permission", "readonly")

	// 9. 権限の変更がユーザー一覧に反映されていることを確認する
	users = listUsers(t, h, "権限の変更がユーザー一覧に反映されていることを確認する", createdRegistry.id(t))
	require.Len(t, users, 1, "user list should contain exactly the user created by this run")
	require.Equal(t, userName, users[0].UserName)
	require.Equal(t, "readonly", users[0].Permission)
}

type registryItem struct {
	ID           json.RawMessage `json:"ID"`
	Name         string          `json:"Name"`
	RegistryName string          `json:"SubDomainLabel"`
	Description  string          `json:"Description"`
}

// id はレジストリ ID を文字列として返します。API の ID は数値または文字列として出力されるため、両方に対応します。
func (item registryItem) id(t *testing.T) string {
	t.Helper()

	if len(item.ID) == 0 {
		t.Fatal("container registry ID is missing")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(item.ID))
	decoder.UseNumber()
	err := decoder.Decode(&value)
	require.NoError(t, err, "decode container registry ID")
	switch value := value.(type) {
	case string:
		require.NotEmpty(t, value, "container registry ID is empty")
		return value
	case json.Number:
		require.NotEqual(t, "0", value.String(), "container registry ID is empty")
		return value.String()
	default:
		t.Fatalf("container registry ID has unsupported JSON type %T", value)
		return ""
	}
}

type userItem struct {
	UserName   string `json:"UserName"`
	Permission string `json:"Permission"`
}

// requireRegistryIdentity は参照結果の ID・名前と説明が、今回の実行で設定した値と一致することを確認します。
func requireRegistryIdentity(t *testing.T, item registryItem, id, name string, descriptions ...string) {
	t.Helper()

	require.Equal(t, id, item.id(t), "registry ID should match the registry created by this run")
	require.Equal(t, name, item.Name, "registry name should match the name requested by this run")
	require.Equal(t, name, item.RegistryName, "registry connection name should match the name requested by this run")
	require.Contains(t, descriptions, item.Description, "registry description should be a value set by this run")
}

// findRegistries は Names で絞り込んだレジストリ一覧を取得します。
func findRegistries(t *testing.T, h *helper.Helper, description, requestPath string) []registryItem {
	t.Helper()

	data := h.Call(t, description,
		"container-registry-api", "registry", "list", "--request", "@"+requestPath, "--query", queryRegistries)
	var items []registryItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode projected registries: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON registry array", description)
	return items
}

// listUsers はレジストリのユーザー一覧を取得します。
func listUsers(t *testing.T, h *helper.Helper, description, registryID string) []userItem {
	t.Helper()

	data := h.Call(t, description,
		"container-registry-api", "registry", "user", "list", registryID, "--query", queryUsers)
	var users []userItem
	err := json.Unmarshal(data, &users)
	require.NoError(t, err, "%s: decode projected users: %v", description, err)
	require.NotNil(t, users, "%s: expected a JSON user array", description)
	return users
}

// parseRegistry は CLI の JSON 出力をレジストリとして解釈します。
func parseRegistry(t *testing.T, data []byte) registryItem {
	t.Helper()

	var item registryItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode container registry: %v", err)
	item.id(t)
	return item
}

// randomSuffix はリソース名の衝突回避用サフィックスを生成します。
func randomSuffix(t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	require.NoError(t, err, "generate random suffix: %v", err)
	return hex.EncodeToString(buf)
}

// writePasswordFile は今回の実行専用のパスワードが入った保護されたファイルを作成し、パスを返します。
func writePasswordFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "registry-password")
	err := os.WriteFile(path, []byte(randomSuffix(t)), 0o600)
	require.NoError(t, err, "write private E2E password file: %v", err)
	return path
}
