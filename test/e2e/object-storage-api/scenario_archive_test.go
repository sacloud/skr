// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioObjectStorageArchive はアーカイブプランの一時バケットを作成し、
// ライフサイクルとプラン設定、権限・キー操作が正しく動作することを確認します。
func TestScenarioObjectStorageArchive(t *testing.T) {
	h := helper.New(t, "object-storage-archive",
		"アーカイブプランの一時バケットを作成し、ライフサイクルとプラン設定が正しく動作することを確認する")

	// arc02 のサイトアカウントが未作成なら作成し、後片付けで削除する。既存アカウントは変更しない。
	cleanupAccount := ensureObjectStorageAccount(t, h, "arc02")
	defer cleanupAccount()

	runBucketScenario(t, h, "arc02", "1t", nil)
}
