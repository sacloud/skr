// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioSecretManager は secretmanager-api コマンドのライブ E2E です。
// テスト専用の KMS キーを作成し、Vault 作成から参照、一覧、更新、シークレットの
// 作成 (upsert によるバージョン増加を含む)、unveil、削除までの一連の操作を検証します。
// KMS キーは生成モードで作成するため、秘密鍵の値は発生しません。後片付けで作成した
// Vault、シークレット、KMS キーを削除し、既存の KMS キーは変更・削除しません。
// `secret unveil` の出力には秘密情報が含まれるため、実行記録からは非表示にします。
func TestScenarioSecretManager(t *testing.T) {
	h := helper.New(t, "secretmanager-api",
		"シークレット保管庫 (Vault) とシークレットの作成・参照・一覧・更新・取得・削除が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	vaultName := "skr-e2e-secretmanager-" + suffix
	renamedVaultName := vaultName + "-renamed"
	secretName := "skr-e2e-secret-" + suffix
	secretValue1 := "secret-manager-e2e-value-1"
	secretValue2 := "secret-manager-e2e-value-2"

	// テスト専用の KMS キーを作成する (Vault 作成に必要)
	key := createScenarioKmsKey(t, h)
	// 後片付け: Vault とシークレットの後片付けが先に実行されるよう、テスト関数の defer で KMS キーを削除する
	defer deleteScenarioKmsKey(t, h, key.ID)

	// 作成前に同名 Vault が存在しないことを確認する
	before := listSecretManagerVaults(t, h, "作成前に同名 Vault が存在しないことを確認する")
	for _, item := range before {
		require.NotEqual(t, vaultName, item.Name, "vault name %q already exists; refusing to modify it", vaultName)
		require.NotEqual(t, renamedVaultName, item.Name, "vault name %q already exists; refusing to modify it", renamedVaultName)
	}

	// Vault を作成する
	created := createSecretManagerVault(t, h, vaultName, "secretmanager-api scenario", key.ID)
	require.NotEmpty(t, created.ID, "created vault should have an ID: %+v", created)
	require.Equal(t, vaultName, created.Name, "created vault name should match the requested name")
	require.Equal(t, key.ID, created.KmsKeyID, "created vault should reference the scenario KMS key")
	vaultID := created.ID

	// 後片付け: シークレットを削除してから Vault を削除し、一覧に残っていないことを確認する
	defer func() {
		deleteSecretManagerSecret(t, h, vaultID, secretName)
		deleteSecretManagerVault(t, h, vaultID)
		remaining := listSecretManagerVaults(t, h, "削除後に Vault が一覧に残っていないことを確認する")
		for _, item := range remaining {
			require.NotEqual(t, vaultID, item.ID, "vault %q should not remain after deletion", vaultID)
		}
	}()

	// 作成した Vault を参照する
	read := readSecretManagerVault(t, h, vaultID)
	require.Equal(t, vaultID, read.ID, "read should return the vault created by this run")
	require.Equal(t, vaultName, read.Name, "read vault name should match the requested name")
	require.Equal(t, key.ID, read.KmsKeyID, "read vault should reference the scenario KMS key")

	// 作成した Vault が一覧に含まれることを確認する
	listed := listSecretManagerVaults(t, h, "作成した Vault が一覧に含まれることを確認する")
	matches := 0
	for _, item := range listed {
		if item.ID == vaultID {
			require.Equal(t, vaultName, item.Name, "listed vault %q should be the vault created by this run", vaultID)
			matches++
		}
	}
	require.Equal(t, 1, matches, "vault list should contain exactly one match for %q", vaultID)

	// Vault の名前と説明を更新する
	updated := updateSecretManagerVault(t, h, vaultID, renamedVaultName, "secretmanager-api scenario updated")
	require.Equal(t, vaultID, updated.ID, "updated vault should be the vault created by this run")
	require.Equal(t, renamedVaultName, updated.Name, "updated vault name should match the requested name")

	// 更新後の名前が参照でも返ることを確認する
	reread := readSecretManagerVault(t, h, vaultID)
	require.Equal(t, renamedVaultName, reread.Name, "read after update should return the updated name")

	// シークレットを作成する (1 回目)
	valueFile1 := writeSecretValueFile(t, secretValue1)
	secretCreated := createSecretManagerSecret(t, h, vaultID, secretName, valueFile1)
	require.Equal(t, secretName, secretCreated.Name, "created secret name should match the requested name")
	require.Equal(t, 1, secretCreated.LatestVersion, "first create should register version 1: %+v", secretCreated)

	// 後片付け: 作成したシークレットを削除し、一覧に残っていないことを確認する
	defer func() {
		deleteSecretManagerSecret(t, h, vaultID, secretName)
		remaining := listSecretManagerSecrets(t, h, vaultID, "削除後にシークレットが一覧に残っていないことを確認する")
		for _, item := range remaining {
			require.NotEqual(t, secretName, item.Name, "secret %q should not remain after deletion", secretName)
		}
	}()

	// 同名シークレットに再作成すると新しいバージョンとして登録される (upsert)
	valueFile2 := writeSecretValueFile(t, secretValue2)
	secretUpdated := createSecretManagerSecret(t, h, vaultID, secretName, valueFile2)
	require.Equal(t, 2, secretUpdated.LatestVersion, "upsert should bump the latest version: %+v", secretUpdated)

	// シークレットが一覧に含まれることを確認する
	secretListed := listSecretManagerSecrets(t, h, vaultID, "作成したシークレットが一覧に含まれることを確認する")
	secretMatches := 0
	for _, item := range secretListed {
		if item.Name == secretName {
			require.Equal(t, 2, item.LatestVersion, "listed secret %q should report the latest version", secretName)
			secretMatches++
		}
	}
	require.Equal(t, 1, secretMatches, "secret list should contain exactly one match for %q", secretName)

	// シークレットの値を取得する (最新バージョン)
	// stdout に秘密情報が含まれるため CallHidingStdout で実行記録から非表示にする
	unveiled := unveilSecretManagerSecret(t, h, vaultID, secretName, "")
	require.Equal(t, secretValue2, unveiled, "unveiled value should match the latest secret value")

	// 過去のバージョンも取得できることを確認する
	unveiledV1 := unveilSecretManagerSecret(t, h, vaultID, secretName, "1")
	require.Equal(t, secretValue1, unveiledV1, "unveiled value should match version 1")
}

// scenarioKmsKey はシナリオで作成した KMS キーの参照に使うフィールドです。
type scenarioKmsKey struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

// createScenarioKmsKey はシナリオ専用の KMS キーを生成モードで作成します。
// 生成モードのため秘密鍵の値は発生せず、実行記録に秘密情報は残りません。
func createScenarioKmsKey(t *testing.T, h *helper.Helper) scenarioKmsKey {
	t.Helper()

	name := "skr-e2e-secretmanager-kms-" + helper.RandomSuffix(t)

	// 作成前に同名キーが存在しないことを確認する
	output := h.Call(t, "作成前に同名 KMS キーが存在しないことを確認する",
		"kms-api", "key", "list", "--query", `map(select(.Name == "`+name+`") | {ID,Name})`)
	var existing []scenarioKmsKey
	require.NoError(t, json.Unmarshal(output, &existing), "decode KMS key list")
	require.Empty(t, existing, "KMS key %s already exists; refusing to modify it", name)

	output = h.Call(t, "テスト専用の KMS キーを作成する",
		"kms-api", "key", "create", "--name", name,
		"--description", "secretmanager-api scenario")
	var created scenarioKmsKey
	require.NoError(t, json.Unmarshal(output, &created), "decode created KMS key")
	require.NotEmpty(t, created.ID, "created KMS key should have an ID")
	return created
}

// deleteScenarioKmsKey は作成した KMS キーを削除し、一覧に残っていないことを確認します。
func deleteScenarioKmsKey(t *testing.T, h *helper.Helper, keyID string) {
	t.Helper()

	h.Call(t, "作成した KMS キーを削除する", "kms-api", "key", "delete", keyID)
	output := h.Call(t, "削除後に KMS キーが一覧に残っていないことを確認する",
		"kms-api", "key", "list", "--query", `map(select(.ID == "`+keyID+`") | {ID,Name})`)
	var remaining []scenarioKmsKey
	require.NoError(t, json.Unmarshal(output, &remaining), "decode KMS key list")
	require.Empty(t, remaining, "KMS key %s remains after deletion", keyID)
}
