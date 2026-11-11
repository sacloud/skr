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
// 秘密鍵の値は発生しません。後片付けで作成した KMS キーの削除を予約し、既存の KMS キーは変更しません (#141)。
// 東京第1サイトのアカウントが未作成の場合は Metrics シナリオと同じく作成し、後片付けで削除します。
func TestScenarioObjectStorageEncryption(t *testing.T) {
	h := helper.New(t, "object-storage-encryption",
		"一時バケットの暗号化を有効化・無効化し、KMS キー ID が正しく反映されることを確認する")

	// テスト専用の KMS キーを作成する
	key := createEncryptionKmsKey(t, h)
	// 後片付け: バケットとサイトアカウントの後片付けが先に実行されるよう、KMS キーの削除予約を defer する
	defer scheduleEncryptionKmsKeyDestruction(t, h, key)

	// 東京第1サイトのアカウントが未作成なら作成し、後片付けで削除する。既存アカウントは変更しない。
	cleanupAccount := ensureObjectStorageAccount(t, h, "tky01")
	defer cleanupAccount()

	runBucketScenario(t, h, "tky01", "", func(t *testing.T, h *helper.Helper, siteID, bucket string, cleanupVetoed *bool) {
		runEncryptionLifecycle(t, h, siteID, bucket, key.ID, cleanupVetoed)
	})
}

// encryptionKmsKey は暗号化シナリオで作成した KMS キーの参照に使うフィールドです。
type encryptionKmsKey struct {
	ID                     string `json:"ID"`
	Name                   string `json:"Name"`
	Status                 string `json:"Status"`
	DeletionScheduledAfter string `json:"DeletionScheduledAfter"`
}

// createEncryptionKmsKey は暗号化シナリオ専用の KMS キーを生成モードで作成します。
// 生成モードのため秘密鍵の値は発生せず、実行記録に秘密情報は残りません。
func createEncryptionKmsKey(t *testing.T, h *helper.Helper) encryptionKmsKey {
	t.Helper()

	name := "skr-e2e-object-storage-encryption-" + helper.RandomSuffix(t)

	// 作成前に同名キーが存在しないことを確認する
	output := h.Call(t, "作成前に同名 KMS キーが存在しないことを確認する",
		"kms-api", "list-keys", "--query", `map(select(.Name == "`+name+`") | {ID,Name})`)
	var existing []encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &existing), "decode KMS key list")
	require.Empty(t, existing, "KMS key %s already exists; refusing to modify it", name)

	output = h.Call(t, "テスト専用の KMS キーを作成する",
		"kms-api", "create-key", "--name", name,
		"--description", "object-storage-api encryption scenario")
	var created encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &created), "decode created KMS key")
	require.NotEmpty(t, created.ID, "created KMS key should have an ID")
	return created
}

// scheduleEncryptionKmsKeyDestruction は所有を再確認してから作成した KMS キーの削除を予約します。
func scheduleEncryptionKmsKeyDestruction(t *testing.T, h *helper.Helper, key encryptionKmsKey) {
	t.Helper()

	currentOutput := h.Call(t, "後片付け前に KMS キーの所有と状態を再確認する",
		"kms-api", "read-key", key.ID,
		"--query", "{ID,Name,Status,DeletionScheduledAfter}")
	var current encryptionKmsKey
	require.NoError(t, json.Unmarshal(currentOutput, &current), "decode KMS key before cleanup")
	require.Equal(t, key.ID, current.ID, "refusing to change an unexpected KMS key ID")
	require.Equal(t, key.Name, current.Name, "refusing to change an unexpected KMS key name")
	switch current.Status {
	case "active":
		output := h.Call(t, "作成した KMS キーの削除を予約する",
			"kms-api", "schedule-key-destruction", key.ID, "--pending-days", "7")
		var scheduled encryptionKmsKey
		require.NoError(t, json.Unmarshal(output, &scheduled), "decode KMS key schedule-destruction response")
		require.Equal(t, "pending_destruction", scheduled.Status)
		require.NotEmpty(t, scheduled.DeletionScheduledAfter)
	case "pending_destruction":
		require.NotEmpty(t, current.DeletionScheduledAfter)
	default:
		require.Contains(t, []string{"active", "pending_destruction"}, current.Status,
			"refusing to change a KMS key in an unexpected state")
	}
	output := h.Call(t, "削除予約後の KMS キーの状態を確認する",
		"kms-api", "read-key", key.ID,
		"--query", "{ID,Name,Status,DeletionScheduledAfter}")
	var after encryptionKmsKey
	require.NoError(t, json.Unmarshal(output, &after), "decode KMS key after cleanup")
	require.Equal(t, key.ID, after.ID)
	require.Equal(t, key.Name, after.Name)
	require.Equal(t, "pending_destruction", after.Status)
	require.NotEmpty(t, after.DeletionScheduledAfter)
}
