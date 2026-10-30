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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// secretManagerVaultItem は secretmanager-api vault コマンドの参照結果です。
// SecretManager API の応答フィールドは PascalCase です。
type secretManagerVaultItem struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	KmsKeyID    string `json:"KmsKeyID"`
}

// secretManagerSecretItem は secretmanager-api secret コマンドの参照結果です。
type secretManagerSecretItem struct {
	Name          string `json:"Name"`
	LatestVersion int    `json:"LatestVersion"`
}

// createSecretManagerVault は skr CLI でシークレット保管庫 (Vault) を作成し、必要な項目に絞って返します。
func createSecretManagerVault(t *testing.T, h *helper.Helper, name, description, kmsKeyID string) secretManagerVaultItem {
	t.Helper()

	data := h.Call(t, "シークレット保管庫 (Vault) を作成する",
		"secretmanager-api", "vault", "create",
		"--name", name, "--description", description, "--kms-key-id", kmsKeyID,
		"--query", "{ID,Name,Description,KmsKeyID}",
	)
	var item secretManagerVaultItem
	require.NoError(t, json.Unmarshal(data, &item), "decode secretmanager-api vault create response")
	return item
}

// readSecretManagerVault は skr CLI で指定した Vault を参照します。
func readSecretManagerVault(t *testing.T, h *helper.Helper, id string) secretManagerVaultItem {
	t.Helper()

	data := h.Call(t, "シークレット保管庫 (Vault) を参照する",
		"secretmanager-api", "vault", "read", id,
		"--query", "{ID,Name,Description,KmsKeyID}",
	)
	var item secretManagerVaultItem
	require.NoError(t, json.Unmarshal(data, &item), "decode secretmanager-api vault read response")
	return item
}

// listSecretManagerVaults は skr CLI で Vault を一覧します。
func listSecretManagerVaults(t *testing.T, h *helper.Helper, description string) []secretManagerVaultItem {
	t.Helper()

	data := h.Call(t, description,
		"secretmanager-api", "vault", "list",
		"--query", "map({ID,Name,Description,KmsKeyID})",
	)
	var items []secretManagerVaultItem
	require.NoError(t, json.Unmarshal(data, &items), "decode secretmanager-api vault list response")
	return items
}

// updateSecretManagerVault は skr CLI で Vault の名前と説明を更新します。
func updateSecretManagerVault(t *testing.T, h *helper.Helper, id, name, description string) secretManagerVaultItem {
	t.Helper()

	data := h.Call(t, "シークレット保管庫 (Vault) の名前と説明を更新する",
		"secretmanager-api", "vault", "update", id,
		"--name", name, "--description", description,
		"--query", "{ID,Name,Description,KmsKeyID}",
	)
	var item secretManagerVaultItem
	require.NoError(t, json.Unmarshal(data, &item), "decode secretmanager-api vault update response")
	return item
}

// deleteSecretManagerVault は skr CLI で指定した Vault を削除します。
func deleteSecretManagerVault(t *testing.T, h *helper.Helper, id string) {
	t.Helper()

	h.Call(t, "シークレット保管庫 (Vault) を削除する", "secretmanager-api", "vault", "delete", id)
}

// createSecretManagerSecret は skr CLI で Vault にシークレットを作成し、必要な項目に絞って返します。
func createSecretManagerSecret(t *testing.T, h *helper.Helper, vaultID, name, valueFile string) secretManagerSecretItem {
	t.Helper()

	data := h.Call(t, "Vault にシークレットを作成する",
		"secretmanager-api", "secret", "create", vaultID,
		"--name", name, "--value-file", valueFile,
		"--query", "{Name,LatestVersion}",
	)
	var item secretManagerSecretItem
	require.NoError(t, json.Unmarshal(data, &item), "decode secretmanager-api secret create response")
	return item
}

// listSecretManagerSecrets は skr CLI で Vault 内のシークレットを一覧します。
func listSecretManagerSecrets(t *testing.T, h *helper.Helper, vaultID, description string) []secretManagerSecretItem {
	t.Helper()

	data := h.Call(t, description,
		"secretmanager-api", "secret", "list", vaultID,
		"--query", "map({Name,LatestVersion})",
	)
	var items []secretManagerSecretItem
	require.NoError(t, json.Unmarshal(data, &items), "decode secretmanager-api secret list response")
	return items
}

// unveilSecretManagerSecret は skr CLI でシークレットの値を取得します。
// stdout に秘密情報が含まれるため、実行記録からは非表示にします。
func unveilSecretManagerSecret(t *testing.T, h *helper.Helper, vaultID, name string, version string) string {
	t.Helper()

	args := []string{"secretmanager-api", "secret", "unveil", vaultID, name}
	if version != "" {
		args = append(args, "--secret-version", version)
	}
	data := h.CallHidingStdout(t, "シークレットの値を取得する (stdout は非表示)", args...)
	return string(data)
}

// deleteSecretManagerSecret は skr CLI で Vault からシークレットを削除します。
func deleteSecretManagerSecret(t *testing.T, h *helper.Helper, vaultID, name string) {
	t.Helper()

	h.Call(t, "Vault からシークレットを削除する",
		"secretmanager-api", "secret", "delete", vaultID, name)
}

// writeSecretValueFile はシークレットの値を一時ファイルに書き込みます。
func writeSecretValueFile(t *testing.T, value string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secret-value.txt")
	require.NoError(t, os.WriteFile(path, []byte(value), 0o600), "write secret value file")
	return path
}
