// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioObjectStorageEncryption は既存 KMS キーを指定し、一時バケットの暗号化の有効化・
// キー ID の照合・無効化を検証します。KMS キー自体は変更・削除しません。
// KMS キー作成が未実装のため (#141)、既存キー ID を環境変数で指定します。
func TestScenarioObjectStorageEncryption(t *testing.T) {
	h := helper.New(t, "object-storage-encryption",
		"一時バケットの暗号化を有効化・無効化し、KMS キー ID が正しく反映されることを確認する")

	kmsKeyID := os.Getenv("SKR_E2E_OBJECT_STORAGE_KMS_KEY_ID")
	if kmsKeyID == "" {
		t.Skip("SKR_E2E_OBJECT_STORAGE_KMS_KEY_ID is not set; the encryption scenario requires an existing KMS key ID (#141)")
	}

	runBucketScenario(t, h, "tky01", "", func(t *testing.T, h *helper.Helper, siteID, bucket string, cleanupVetoed *bool) {
		runEncryptionLifecycle(t, h, siteID, bucket, kmsKeyID, cleanupVetoed)
	})
}
