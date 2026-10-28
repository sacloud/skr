// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioObjectStorageAccount は未作成サイトでオブジェクトストレージアカウントとテスト用アクセキーを作成し、
// アカウント ID、プラン一覧、キーのライフサイクルと削除後の不在を確認します。
// 既存アカウントの削除や再作成は拒否します。
func TestScenarioObjectStorageAccount(t *testing.T) {
	h := helper.New(t, "object-storage-account",
		"未作成サイトでオブジェクトストレージアカウントとテスト用アクセキーを作成し、ライフサイクルと後片付けが正しく動作することを確認する")

	siteID := "arc02"
	confirmObjectStorageSite(t, h, siteID, "")

	// アカウントが未作成であることを確認する
	_, stderr, err := h.CallAllowingFailure(t, "アカウントが未作成であることを確認する",
		"object-storage-api", "account", "read", siteID)
	if err == nil {
		t.Skipf("site account already exists on %s; refusing to create or delete an existing account", siteID)
	}
	require.True(t, objectStorageNotFound(stderr, "Accounts.Read"),
		"account absence was not confirmed by an Accounts.Read API 404: %s", stderr)

	output := h.Call(t, "オブジェクトストレージアカウントを作成する", "object-storage-api", "account", "create", siteID)
	var created account
	require.NoError(t, json.Unmarshal(output, &created), "decode created account")
	require.NotEmpty(t, created.ResourceID, "created account ID is unknown; manual inspection required")

	deleted := false
	defer func() {
		if !deleted {
			deleteObjectStorageAccount(t, h, siteID, created)
		}
	}()

	verifyObjectStorageAccount(t, h, "作成したアカウントの ID が変更されていないことを確認する", siteID, created)

	// サイトで利用可能なプラン一覧を取得する
	plans := h.Call(t, "サイトで利用可能なプラン一覧を取得する", "object-storage-api", "site", "plans", siteID,
		"--query", "map({service_class_path,type,cluster_id,capacity_gib,fee})")
	available := decodeArray[plan](t, plans, "account site plans")
	require.NotEmpty(t, available, "site returned no available plans")

	// アカウントアクセキーのライフサイクル
	runObjectStorageKeyLifecycle(t, h, []string{"account", "access-key"}, []string{siteID})

	deleteObjectStorageAccount(t, h, siteID, created)
	deleted = true
}
