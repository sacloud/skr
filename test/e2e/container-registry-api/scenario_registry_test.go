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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
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
)

func TestScenarioRegistry(t *testing.T) {
	h := helper.New(t, "container-registry-api", "コンテナレジストリとそのユーザーの作成/更新/確認が可能であることを確認する")

	suffix := randomSuffix(t)
	name := registryPrefix + suffix
	description := descriptionPrefix + suffix
	updatedDescription := description + "/updated"
	passwordPath := writePasswordFile(t)

	findRequest := h.WriteRequestJSONFile(t, "find", map[string]any{"Names": []string{name}})

	// 作成前に同名レジストリが存在しないことを確認する
	before := findRegistries(t, h, "作成前に同名レジストリが存在しないことを確認する", findRequest)
	require.Empty(t, before, "registry name %q already exists; refusing to modify it", name)

	// レジストリを作成する
	data := h.Call(t, "コンテナレジストリを作成する",
		"container-registry-api", "registry", "create", "--name", name, "--description", description)
	createdRegistry := parseRegistry(t, data)
	require.Equal(t, name, createdRegistry.Name, "created registry name should match the requested name")
	require.Equal(t, description, createdRegistry.Description, "created registry description should match the requested description")
	defer func() {
		// 作成したレジストリを削除し、一覧に残っていないことを確認する
		_ = h.Call(t, "作成したレジストリを削除する", "container-registry-api", "registry", "delete", createdRegistry.ID.String())
		remaining := findRegistries(t, h, "削除後にレジストリが一覧に残っていないことを確認する", findRequest)
		require.Empty(t, remaining, "registry %s should not remain after deletion", name)
	}()

	// 作成したレジストリが一覧に含まれることを確認する
	listed := findRegistries(t, h, "作成したレジストリが一覧に含まれることを確認する", findRequest)
	require.Len(t, listed, 1, "registry list should contain exactly one match for %q", name)
	requireRegistryIdentity(t, *listed[0], createdRegistry.ID, name, description)

	// レジストリの説明を更新する
	updateRequest := h.WriteRequestJSONFile(t, "update", map[string]any{"Description": updatedDescription})
	data = h.Call(t, "レジストリの説明を更新する",
		"container-registry-api", "registry", "update", createdRegistry.ID.String(), "--request", "@"+updateRequest)
	requireRegistryIdentity(t, parseRegistry(t, data), createdRegistry.ID, name, updatedDescription)

	// 更新後のレジストリを参照する
	data = h.Call(t, "更新後のレジストリを参照する", "container-registry-api", "registry", "read", createdRegistry.ID.String())
	requireRegistryIdentity(t, parseRegistry(t, data), createdRegistry.ID, name, updatedDescription)

	// レジストリユーザーを追加する
	_ = h.Call(t, "レジストリユーザーを追加する",
		"container-registry-api", "registry", "user", "add", createdRegistry.ID.String(),
		"--user-name", userName, "--password-file", passwordPath, "--permission", "readwrite")
	defer func() {
		// ユーザーを削除し、残っていないことを確認する
		_ = h.Call(t, "ユーザーを削除する",
			"container-registry-api", "registry", "user", "delete", createdRegistry.ID.String(), userName)
		users := listUsers(t, h, "ユーザー削除後にユーザーが残っていないことを確認する", createdRegistry.ID.String())
		require.Empty(t, users, "user should not remain after deletion")
	}()

	// 追加したユーザーが一覧に含まれることを確認する
	users := listUsers(t, h, "追加したユーザーが一覧に含まれることを確認する", createdRegistry.ID.String())
	require.Len(t, users, 1, "user list should contain exactly the user created by this run")
	require.Equal(t, userName, users[0].UserName)
	require.Equal(t, types.ContainerRegistryPermissions.ReadWrite, users[0].Permission)

	// ユーザーの権限を変更する
	_ = h.Call(t, "ユーザーの権限を変更する",
		"container-registry-api", "registry", "user", "update", createdRegistry.ID.String(), userName, "--permission", "readonly")

	// 権限の変更がユーザー一覧に反映されていることを確認する
	users = listUsers(t, h, "権限の変更がユーザー一覧に反映されていることを確認する", createdRegistry.ID.String())
	require.Len(t, users, 1, "user list should contain exactly the user created by this run")
	require.Equal(t, userName, users[0].UserName)
	require.Equal(t, types.ContainerRegistryPermissions.ReadOnly, users[0].Permission)
}

// requireRegistryIdentity は参照結果の ID・名前と説明が、今回の実行で設定した値と一致することを確認します。
func requireRegistryIdentity(t *testing.T, item iaas.ContainerRegistry, id types.ID, name string, descriptions ...string) {
	t.Helper()

	require.Equal(t, id, item.ID, "registry ID should match the registry created by this run")
	require.Equal(t, name, item.Name, "registry name should match the name requested by this run")
	require.Equal(t, name, item.SubDomainLabel, "registry connection name should match the name requested by this run")
	require.Contains(t, descriptions, item.Description, "registry description should be a value set by this run")
}

// findRegistries は Names で絞り込んだレジストリ一覧を取得します。
func findRegistries(t *testing.T, h *helper.Helper, description, requestPath string) []*iaas.ContainerRegistry {
	t.Helper()

	data := h.Call(t, description,
		"container-registry-api", "registry", "list", "--request", "@"+requestPath)
	var registries []*iaas.ContainerRegistry
	err := json.Unmarshal(data, &registries)
	require.NoError(t, err, "%s: decode registries: %v", description, err)
	require.NotNil(t, registries, "%s: expected a JSON registry array", description)
	return registries
}

// listUsers はレジストリのユーザー一覧を取得します。
func listUsers(t *testing.T, h *helper.Helper, description, registryID string) []iaas.ContainerRegistryUser {
	t.Helper()

	data := h.Call(t, description,
		"container-registry-api", "registry", "user", "list", registryID)
	var users []iaas.ContainerRegistryUser
	err := json.Unmarshal(data, &users)
	require.NoError(t, err, "%s: decode registry users: %v", description, err)
	require.NotNil(t, users, "%s: expected a JSON user array", description)
	return users
}

// parseRegistry は CLI の JSON 出力をレジストリとして解釈します。
func parseRegistry(t *testing.T, data []byte) iaas.ContainerRegistry {
	t.Helper()

	var registry iaas.ContainerRegistry
	err := json.Unmarshal(data, &registry)
	require.NoError(t, err, "decode container registry: %v", err)
	require.NotZero(t, registry.ID, "container registry ID is missing")
	return registry
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
