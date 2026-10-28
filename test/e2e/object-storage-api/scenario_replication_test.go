// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioObjectStorageReplication は異なる通常サイトの2バケット間でレプリケーションを設定し、
// 送信元・宛先の名前とサイト、作成状態、無効化後の設定不在を確認します。
func TestScenarioObjectStorageReplication(t *testing.T) {
	h := helper.New(t, "object-storage-replication",
		"異なる通常サイトの2バケット間でレプリケーションを設定し、設定と後片付けが正しく動作することを確認する")

	// 送信元サイトのバケットを作成し、そのフローの中で宛先サイトのバケットとレプリケーションを設定する
	runBucketScenario(t, h, "isk01", "", func(t *testing.T, h *helper.Helper, sourceSite, sourceBucket string, cleanupVetoed *bool) {
		// 宛先サイトのアカウントが未作成なら作成し、後片付けで削除する。既存アカウントは変更しない。
		cleanupTargetAccount := ensureObjectStorageAccount(t, h, "tky01")
		defer cleanupTargetAccount()

		runBucketScenario(t, h, "tky01", "", func(t *testing.T, h *helper.Helper, targetSite, targetBucket string, _ *bool) {
			runReplicationLifecycle(t, h, sourceSite, sourceBucket, targetSite, targetBucket)
		})
	})
}
