// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioObjectStorageEncryption はテスト専用の KMS キーを作成し、一時バケットの暗号化の
// 有効化・キー ID の照合・無効化を検証します。KMS キーは生成モードで作成するため、
// 秘密鍵の値は発生しません。後片付けで作成した KMS キーを削除し、既存の KMS キーは変更・削除しません (#141)。
func TestScenarioObjectStorageEncryption(t *testing.T) {
	h := helper.New(t, "object-storage-encryption",
		"一時バケットの暗号化を有効化・無効化し、KMS キー ID が正しく反映されることを確認する")

	// テスト専用の KMS キーを作成する
	key := createEncryptionKmsKey(t, h)
	// 後片付け: バケットの削除が先に実行されるよう、テスト関数の defer で KMS キーを削除する
	defer deleteEncryptionKmsKey(t, h, key.ID)

	runBucketScenario(t, h, "tky01", "", func(t *testing.T, h *helper.Helper, siteID, bucket string, cleanupVetoed *bool) {
		runEncryptionLifecycle(t, h, siteID, bucket, key.ID, cleanupVetoed)
	})
}

// encryptionKmsKey は暗号化シナリオで作成した KMS キーの参照に使うフィールドです。
type encryptionKmsKey struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

// createEncryptionKmsKey は暗号化シナリオ専用の KMS キーを生成モードで作成します。
// 生成モードのため秘密鍵の値は発生せず、実行記録に秘密情報は残りません。
func createEncryptionKmsKey(t *testing.T, h *helper.Helper) encryptionKmsKey {
	t.Helper()

	name := "skr-e2e-object-storage-encryption-" + helper.RandomSuffix(t)

	// 作成前に同名キーが存在しないことを確認する
	output := h.Call(t, "作成前に同名 KMS キーが存在しないことを確認する",
		"kms-api", "key", "list", "--query", `map(select(.Name == "`+name+`") | {ID,Name})`)
	var existing []encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &existing), "decode KMS key list")
	require.Empty(t, existing, "KMS key %s already exists; refusing to modify it", name)

	output = h.Call(t, "テスト専用の KMS キーを作成する",
		"kms-api", "key", "create", "--name", name,
		"--description", "object-storage-api encryption scenario")
	var created encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &created), "decode created KMS key")
	require.NotEmpty(t, created.ID, "created KMS key should have an ID")
	return created
}

// deleteEncryptionKmsKey は作成した KMS キーを削除し、一覧に残っていないことを確認します。
func deleteEncryptionKmsKey(t *testing.T, h *helper.Helper, keyID string) {
	t.Helper()

	h.Call(t, "作成した KMS キーを削除する", "kms-api", "key", "delete", keyID)
	output := h.Call(t, "削除後に KMS キーが一覧に残っていないことを確認する",
		"kms-api", "key", "list", "--query", `map(select(.ID == "`+keyID+`") | {ID,Name})`)
	var remaining []encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &remaining), "decode KMS key list")
	require.Empty(t, remaining, "KMS key %s remains after deletion", keyID)
}
