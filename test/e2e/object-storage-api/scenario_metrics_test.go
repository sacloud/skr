// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioObjectStorageMetrics は一時バケットでサイトの状態・クォータとバケットの利用量・クォータ、
// 月次のメトリクス参照を検証します。
func TestScenarioObjectStorageMetrics(t *testing.T) {
	h := helper.New(t, "object-storage-metrics",
		"一時バケットでサイトの状態・クォータとバケットの利用量・クォータの取得が正しく動作することを確認する")

	// tky01 のサイトアカウントが未作成なら作成し、後片付けで削除する。既存アカウントは変更しない。
	cleanupAccount := ensureObjectStorageAccount(t, h, "tky01")
	defer cleanupAccount()

	runBucketScenario(t, h, "tky01", "", readObjectStorageMetrics)
}

// readObjectStorageMetrics はサイトの状態・クォータとバケットのメトリクスを参照します。
// `bucket penalty` と `site status bucket-metering` は現時点で API エラー (#142) となるため deferred とし、
// このシナリオでは呼び出しません。
func readObjectStorageMetrics(t *testing.T, h *helper.Helper, siteID, bucket string, cleanupVetoed *bool) {
	t.Helper()

	output := h.Call(t, "サイトの稼働状態を確認する", "object-storage-api", "site", "status", "read", siteID)
	requireObjectFields(t, output, "サイトの稼働状態", "accept_new", "status_code")

	output = h.Call(t, "サイトのクォータ情報を確認する", "object-storage-api", "site", "status", "quota", siteID)
	requireObjectFields(t, output, "サイトのクォータ情報", "num_buckets", "num_permissions")

	output = h.Call(t, "バケットの使用量を確認する", "object-storage-api", "bucket", "usage", siteID, bucket)
	requireObjectFields(t, output, "バケットの使用量", "num_objects_per_bucket", "amount_gib_per_bucket")
	var usage struct {
		Objects *int     `json:"num_objects_per_bucket"`
		Amount  *float64 `json:"amount_gib_per_bucket"`
	}
	require.NoError(t, json.Unmarshal(output, &usage), "decode bucket usage")
	require.NotNil(t, usage.Objects, "bucket has invalid storage usage")
	require.NotNil(t, usage.Amount, "bucket has invalid storage usage")
	require.GreaterOrEqual(t, *usage.Objects, 0, "bucket has invalid storage usage")
	require.GreaterOrEqual(t, *usage.Amount, 0.0, "bucket has invalid storage usage")

	output = h.Call(t, "バケットのクォータを確認する", "object-storage-api", "bucket", "quota", siteID, bucket)
	requireObjectFields(t, output, "バケットのクォータ", "num_objects_per_bucket", "amount_gib_per_bucket")
}
