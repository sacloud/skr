// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioObjectStorageBucket は既存サイトアカウントで一時バケット、パーミッション、アクセキーを作成し、
// CRUD 操作と後片付けが正しく動作することを確認します。既存リソースは変更しません。
func TestScenarioObjectStorageBucket(t *testing.T) {
	h := helper.New(t, "object-storage-bucket",
		"一時バケット、パーミッション、アクセキーを作成し、CRUD 操作と後片付けが正しく動作することを確認する")

	runBucketScenario(t, h, "isk01", "", nil)
}
