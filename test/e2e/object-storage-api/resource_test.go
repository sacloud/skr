// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// objectStorageNamePrefix はシナリオが作成する一時バケット名の接頭辞です。
	objectStorageNamePrefix = "skr-e2e-"
	// objectStorageWaitInterval は状態の反映を待つ間隔です。
	objectStorageWaitInterval = 3 * time.Second
	// objectStorageWaitTimeout は状態の反映を待つ制限時間です。
	objectStorageWaitTimeout = 2 * time.Minute
)

// site はサイト一覧と詳細の応答から参照する項目です。
type site struct {
	ID string `json:"id"`
}

// plan は site plans の応答から参照する項目です。
type plan struct {
	Path      string `json:"service_class_path"`
	Type      string `json:"type"`
	ClusterID string `json:"cluster_id"`
}

// bucket はバケット一覧の応答から参照する項目です。
type bucket struct {
	Name       string          `json:"name"`
	ResourceID string          `json:"resource_id"`
	Plan       json.RawMessage `json:"plan"`
}

// createdBucket はバケット作成応答から参照する項目です。
type createdBucket struct {
	Name       string          `json:"name"`
	ClusterID  string          `json:"cluster_id"`
	ResourceID string          `json:"resource_id"`
	Plan       json.RawMessage `json:"plan"`
}

// account はサイトアカウントの応答から参照する項目です。
type account struct {
	ResourceID string `json:"resource_id"`
}

// bucketControl はパーミッションのバケット権限です。
type bucketControl struct {
	BucketName string `json:"bucket_name"`
	CanRead    bool   `json:"can_read"`
	CanWrite   bool   `json:"can_write"`
}

// permission はパーミッションの応答から参照する項目です。
type permission struct {
	ID          int64           `json:"id,omitempty"`
	DisplayName string          `json:"display_name"`
	Controls    []bucketControl `json:"bucket_controls"`
}

// accessKey はアクセスキーの応答から参照する項目です。
type accessKey struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// replication はレプリケーション設定の応答から参照する項目です。
type replication struct {
	Source createdBucket `json:"source_bucket"`
	Target createdBucket `json:"dest_bucket"`
	Status string        `json:"config_status"`
}

// bucketExtra はバケットの権限・キー操作に続くシナリオ固有の操作です。
// バケット設定の所有確認ができていない場合は cleanupVetoed を true にし、バケットの削除を中止します。
type bucketExtra func(t *testing.T, h *helper.Helper, siteID, bucket string, cleanupVetoed *bool)

// objectStorageNotFound はコマンドの失敗が対象不在の API 404 かどうかを判定します。
func objectStorageNotFound(stderr, method string) bool {
	return strings.HasPrefix(strings.TrimSpace(stderr), "object-storage: "+method+": API Error 404:")
}

// decodeArray は JSON 配列の応答を厳密にデコードします。
func decodeArray[T any](t *testing.T, output []byte, label string) []T {
	t.Helper()

	require.True(t, strings.HasPrefix(strings.TrimSpace(string(output)), "["), "%s must be a JSON array", label)
	var values []json.RawMessage
	require.NoError(t, json.Unmarshal(output, &values), "decode %s", label)
	items := make([]T, len(values))
	for i, value := range values {
		require.True(t, strings.HasPrefix(strings.TrimSpace(string(value)), "{"), "%s item %d must be a JSON object", label, i)
		require.NoError(t, json.Unmarshal(value, &items[i]), "decode %s item %d", label, i)
	}
	return items
}

// requireObjectFields は API 応答に必要なフィールドが存在することを確認します。
func requireObjectFields(t *testing.T, output []byte, label string, fields ...string) {
	t.Helper()

	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(output, &object), "decode %s", label)
	for _, field := range fields {
		value, ok := object[field]
		require.True(t, ok && string(value) != "null", "%s response is missing %s", label, field)
	}
}

// waitForObjectStorage は状態の反映を一定間隔で確認します。制限時間を過ぎるとテストを失敗させます。
func waitForObjectStorage(t *testing.T, h *helper.Helper, description string, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(objectStorageWaitTimeout)
	for !check() {
		require.Less(t, time.Now(), deadline, "timed out waiting for %s", description)
		h.Sleep(t, description, objectStorageWaitInterval)
	}
}

// objectStorageSitePlanPath はサイトとプランに対応する service_class_path を組み立てます。
func objectStorageSitePlanPath(siteID, planValue string) string {
	path := "objectstorage/" + siteID + "/bucket"
	if planValue != "" {
		path += "/" + planValue
	}
	return path
}

// confirmObjectStorageSite はサイト一覧と詳細を照合し、プラン指定があれば利用可能なプランであることを確認します。
func confirmObjectStorageSite(t *testing.T, h *helper.Helper, siteID, planValue string) {
	t.Helper()

	output := h.Call(t, "利用可能なオブジェクトストレージサイト一覧を取得する", "object-storage-api", "site", "list",
		"--query", "map({id,display_name,region,plan_family})")
	sites := decodeArray[site](t, output, "sites")
	matches := 0
	for _, item := range sites {
		if item.ID == siteID {
			matches++
		}
	}
	require.Equal(t, 1, matches, "expected exactly one site %s, got %d", siteID, matches)

	output = h.Call(t, "指定サイトの詳細を確認する", "object-storage-api", "site", "read", siteID)
	var selected site
	require.NoError(t, json.Unmarshal(output, &selected), "decode site")
	require.Equal(t, siteID, selected.ID, "site read identity does not match the requested site")

	if planValue == "" {
		return
	}
	output = h.Call(t, "指定サイトで利用可能なプラン一覧を取得する", "object-storage-api", "site", "plans", siteID,
		"--query", "map({service_class_path,type,cluster_id,capacity_gib,fee})")
	plans := decodeArray[plan](t, output, "plans")
	for _, item := range plans {
		if item.ClusterID == siteID && item.Type == "archive" && item.Path == objectStorageSitePlanPath(siteID, planValue) {
			return
		}
	}
	t.Fatalf("requested archive plan %s was not present in the site %s plan list", planValue, siteID)
}

// listObjectStorageBuckets はサイトのバケット一覧を取得します。
func listObjectStorageBuckets(t *testing.T, h *helper.Helper, description, siteID string) []bucket {
	t.Helper()

	output := h.Call(t, description, "object-storage-api", "bucket", "list", siteID,
		"--query", "map({name,resource_id,plan})")
	return decodeArray[bucket](t, output, "bucket list")
}

// bucketsNamed は指定した名前のバケットを一覧から抽出します。
func bucketsNamed(items []bucket, name string) []bucket {
	var matches []bucket
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches
}

// createObjectStorageBucket は一時バケットを作成し、応答の識別情報を確認します。
func createObjectStorageBucket(t *testing.T, h *helper.Helper, siteID, name, planValue string) createdBucket {
	t.Helper()

	args := []string{"object-storage-api", "bucket", "create", siteID, name}
	if planValue != "" {
		args = append(args, "--plan", planValue)
	}
	output := h.Call(t, "テスト用バケットを作成する", args...)
	var created createdBucket
	require.NoError(t, json.Unmarshal(output, &created),
		"decode created bucket identity; inspect site %s and bucket %s before cleanup", siteID, name)
	require.Equal(t, name, created.Name,
		"created bucket identity does not match the request; inspect site %s and bucket %s before cleanup", siteID, name)
	require.Equal(t, siteID, created.ClusterID,
		"created bucket identity does not match the request; inspect site %s and bucket %s before cleanup", siteID, name)
	return created
}

// checkCreatedBucketPlan は作成結果のバケットのプランがリクエストと一致することを確認します。
func checkCreatedBucketPlan(t *testing.T, item bucket, siteID, planValue string) {
	t.Helper()

	var actual plan
	require.NoError(t, json.Unmarshal(item.Plan, &actual), "decode created bucket plan")
	wantType := "standard"
	if planValue != "" {
		wantType = "archive"
	}
	require.Equal(t, objectStorageSitePlanPath(siteID, planValue), actual.Path,
		"created bucket plan does not match the requested plan")
	require.Equal(t, wantType, actual.Type, "created bucket plan does not match the requested plan")
}

// deleteObjectStorageBucket はリソース ID を再確認してバケットを削除し、一覧に残っていないことを確認します。
func deleteObjectStorageBucket(t *testing.T, h *helper.Helper, description, siteID string, created createdBucket) {
	t.Helper()

	items := listObjectStorageBuckets(t, h, description+"前に削除対象バケットが存在することを確認する", siteID)
	matches := bucketsNamed(items, created.Name)
	if len(matches) == 0 {
		return
	}
	require.Len(t, matches, 1, "bucket list returned ambiguous buckets named %s", created.Name)
	require.NotEmpty(t, created.ResourceID, "created bucket resource ID is unknown; refusing automatic deletion")
	require.Equal(t, created.ResourceID, matches[0].ResourceID,
		"refusing to delete bucket %s because its resource ID does not match the created bucket", created.Name)

	h.Call(t, "作成したテスト用バケットを削除する", "object-storage-api", "bucket", "delete", created.ClusterID, created.Name)
	items = listObjectStorageBuckets(t, h, description+"後にバケットが一覧に残っていないことを確認する", siteID)
	require.Empty(t, bucketsNamed(items, created.Name), "bucket %s remains after deletion", created.Name)
}

// listObjectStoragePermissions はサイトのパーミッション一覧を取得します。
func listObjectStoragePermissions(t *testing.T, h *helper.Helper, description, siteID string) []permission {
	t.Helper()

	output := h.Call(t, description, "object-storage-api", "permission", "list", siteID,
		"--query", "map({id,display_name,bucket_controls})")
	return decodeArray[permission](t, output, "permission list")
}

// controlsMatch はパーミッションの権限が期待値と一致することを確認します。期待しないバケットの権限は無視します。
func controlsMatch(actual, expected []bucketControl) bool {
	for _, want := range expected {
		count := 0
		for _, item := range actual {
			if item.BucketName == want.BucketName {
				count++
				if item != want {
					return false
				}
			}
		}
		if count != 1 {
			return false
		}
	}
	for _, item := range actual {
		known := false
		for _, want := range expected {
			if item.BucketName == want.BucketName {
				known = true
			}
		}
		if !known && (item.CanRead || item.CanWrite) {
			return false
		}
	}
	return true
}

// verifyObjectStoragePermission は一覧と詳細の両方でパーミッションの内容を確認します。
func verifyObjectStoragePermission(t *testing.T, h *helper.Helper, description, siteID string, expected permission) {
	t.Helper()

	permissionID := strconv.FormatInt(expected.ID, 10)
	items := listObjectStoragePermissions(t, h, description, siteID)
	matches := 0
	for _, item := range items {
		if item.ID == expected.ID {
			matches++
			require.Equal(t, expected.DisplayName, item.DisplayName,
				"permission ID matches but name changed; refusing modification")
			require.True(t, controlsMatch(item.Controls, expected.Controls),
				"permission ID matches but bucket controls changed; refusing modification")
		}
	}
	require.Equal(t, 1, matches, "created permission ID is missing or ambiguous")

	output := h.Call(t, description, "object-storage-api", "permission", "read", siteID, permissionID)
	var actual permission
	require.NoError(t, json.Unmarshal(output, &actual), "decode permission")
	require.Equal(t, expected.ID, actual.ID, "permission read does not match the created permission")
	require.Equal(t, expected.DisplayName, actual.DisplayName, "permission read does not match the created permission")
	require.True(t, controlsMatch(actual.Controls, expected.Controls),
		"permission read does not match the created permission")
}

// deleteObjectStoragePermission はパーミッションを、アクセスキーが残っていないことを確認して削除します。
// 対象が一覧に存在しない場合は削除済みとして扱います。
func deleteObjectStoragePermission(t *testing.T, h *helper.Helper, siteID string, expected permission) {
	t.Helper()

	items := listObjectStoragePermissions(t, h, "パーミッション削除前に対象が存在することを確認する", siteID)
	present := false
	for _, item := range items {
		if item.ID == expected.ID {
			present = true
			// 作成時の権限はこのシナリオで認可済みの状態である
			if len(expected.Controls) == 1 && controlsMatch(item.Controls, []bucketControl{{
				BucketName: expected.Controls[0].BucketName, CanRead: true, CanWrite: true,
			}}) {
				expected.Controls[0].CanWrite = true
			}
		}
	}
	if !present {
		return
	}
	verifyObjectStoragePermission(t, h, "パーミッション削除前の内容を確認する", siteID, expected)

	permissionID := strconv.FormatInt(expected.ID, 10)
	keys := objectStorageKeyList(t, h, "パーミッション削除前にアクセスキーが残っていないことを確認する",
		[]string{"permission", "access-key"}, []string{siteID, permissionID})
	require.Empty(t, keys, "permission still has access keys; refusing to delete it")

	h.Call(t, "一時的なパーミッションを削除する", "object-storage-api", "permission", "delete", siteID, permissionID)
	items = listObjectStoragePermissions(t, h, "パーミッション削除後に対象が一覧に残っていないことを確認する", siteID)
	for _, item := range items {
		require.NotEqual(t, expected.ID, item.ID, "permission remains after deletion")
	}
}

// runBucketPermissionExercise は一時パーミッションとそのアクセキーのライフサイクルを検証します。
func runBucketPermissionExercise(t *testing.T, h *helper.Helper, siteID, bucketName string) {
	t.Helper()

	expected := permission{
		DisplayName: bucketName,
		Controls:    []bucketControl{{BucketName: bucketName, CanRead: true, CanWrite: true}},
	}

	// 作成前に同名パーミッションが存在しないことを確認する
	items := listObjectStoragePermissions(t, h, "作成前に同名パーミッションが存在しないことを確認する", siteID)
	for _, item := range items {
		require.NotEqual(t, expected.DisplayName, item.DisplayName,
			"temporary permission name already exists; refusing to modify it")
	}

	// パーミッションを作成し、識別情報と権限を確認する
	requestPath := h.WriteRequestJSONFile(t, "permission-create", expected)
	output := h.Call(t, "テスト用パーミッションを作成する", "object-storage-api", "permission", "create", siteID,
		"--request", "@"+requestPath)
	var created permission
	require.NoError(t, json.Unmarshal(output, &created),
		"cannot identify created permission; inspect site %s and name %s manually", siteID, expected.DisplayName)
	require.Positive(t, created.ID, "created permission identity is unknown; manual inspection required")
	require.Equal(t, expected.DisplayName, created.DisplayName,
		"created permission identity or controls do not match; manual inspection required")
	require.True(t, controlsMatch(created.Controls, expected.Controls),
		"created permission identity or controls do not match; manual inspection required")
	expected.ID = created.ID

	deleted := false
	defer func() {
		if !deleted {
			deleteObjectStoragePermission(t, h, siteID, expected)
		}
	}()

	verifyObjectStoragePermission(t, h, "作成したパーミッションの内容を確認する", siteID, expected)

	// パーミッションのアクセスキーのライフサイクル
	permissionID := strconv.FormatInt(expected.ID, 10)
	runObjectStorageKeyLifecycle(t, h, []string{"permission", "access-key"}, []string{siteID, permissionID})

	// 書き込み許可を変更し、変更内容を確認する
	expected.Controls[0].CanWrite = false
	updatePath := h.WriteRequestJSONFile(t, "permission-update",
		permission{DisplayName: expected.DisplayName, Controls: expected.Controls})
	h.Call(t, "パーミッションの書き込み権限を変更する", "object-storage-api", "permission", "update", siteID, permissionID,
		"--request", "@"+updatePath)
	verifyObjectStoragePermission(t, h, "変更したパーミッションの内容を確認する", siteID, expected)

	deleteObjectStoragePermission(t, h, siteID, expected)
	deleted = true
}

// objectStorageKeyArgs はアクセキー系コマンドの引数を組み立てます。
func objectStorageKeyArgs(commandPath []string, operation string, commandTarget []string, id string) []string {
	args := append([]string{"object-storage-api"}, commandPath...)
	args = append(args, operation)
	args = append(args, commandTarget...)
	if id != "" {
		args = append(args, id)
	}
	return args
}

// objectStorageKeyList はアクセスキーの一覧を取得します。
func objectStorageKeyList(t *testing.T, h *helper.Helper, description string, commandPath, commandTarget []string) []accessKey {
	t.Helper()

	args := append(objectStorageKeyArgs(commandPath, "list", commandTarget, ""), "--query", "map({id})")
	output := h.Call(t, description, args...)
	return decodeArray[accessKey](t, output, "access key list")
}

// createObjectStorageKey はアクセスキーを作成します。作成応答にはシークレットが含まれるため、
// 実行記録から stdout を除いて取得します。
func createObjectStorageKey(t *testing.T, h *helper.Helper, commandPath, commandTarget []string) accessKey {
	t.Helper()

	output := h.CallHidingStdout(t, "アクセスキーを作成する", objectStorageKeyArgs(commandPath, "create", commandTarget, "")...)
	var created accessKey
	require.NoError(t, json.Unmarshal(output, &created),
		"cannot identify created access key; inspect the new permission or account manually")
	require.NotEmpty(t, created.ID,
		"cannot identify created access key; inspect the new permission or account manually")
	require.NotEmpty(t, created.Secret, "access key creation returned no secret")
	return created
}

// deleteObjectStorageKey はアクセスキーを削除し、一覧に残っていないことを確認します。
// 対象が一覧に存在しない場合は削除済みとして扱います。
func deleteObjectStorageKey(t *testing.T, h *helper.Helper, commandPath, commandTarget []string, id string) {
	t.Helper()

	items := objectStorageKeyList(t, h, "アクセスキー削除前に対象が存在することを確認する", commandPath, commandTarget)
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
		}
	}
	if !found {
		return
	}
	h.Call(t, "アクセスキーを削除する", objectStorageKeyArgs(commandPath, "delete", commandTarget, id)...)
	items = objectStorageKeyList(t, h, "アクセスキー削除後に対象が一覧に残っていないことを確認する", commandPath, commandTarget)
	for _, item := range items {
		require.NotEqual(t, id, item.ID, "access key remains after deletion")
	}
}

// runObjectStorageKeyLifecycle はアクセスキーの作成・参照・失効を検証します。
func runObjectStorageKeyLifecycle(t *testing.T, h *helper.Helper, commandPath, commandTarget []string) {
	t.Helper()

	items := objectStorageKeyList(t, h, "作成前に既存アクセスキーがないことを確認する", commandPath, commandTarget)
	require.Empty(t, items, "access keys already exist; refusing to replace them")

	created := createObjectStorageKey(t, h, commandPath, commandTarget)
	deleted := false
	defer func() {
		if !deleted {
			deleteObjectStorageKey(t, h, commandPath, commandTarget, created.ID)
		}
	}()

	// 作成したアクセキーの詳細を確認する。参照応答にシークレットは含まれません。
	output := h.Call(t, "作成したアクセキーの詳細を確認する", objectStorageKeyArgs(commandPath, "read", commandTarget, created.ID)...)
	var read accessKey
	require.NoError(t, json.Unmarshal(output, &read), "decode access key read response")
	require.Equal(t, created.ID, read.ID, "access key read ID mismatch or secret was unexpectedly returned")
	require.Empty(t, read.Secret, "access key read ID mismatch or secret was unexpectedly returned")

	deleteObjectStorageKey(t, h, commandPath, commandTarget, created.ID)
	deleted = true
}

// verifyObjectStorageAccount はサイトアカウントの ID が変わっていないことを確認します。
func verifyObjectStorageAccount(t *testing.T, h *helper.Helper, description, siteID string, expected account) {
	t.Helper()

	output := h.Call(t, description, "object-storage-api", "account", "read", siteID)
	var actual account
	require.NoError(t, json.Unmarshal(output, &actual), "decode site account")
	require.Equal(t, expected.ResourceID, actual.ResourceID, "site account ID changed; refusing modification")
}

// deleteObjectStorageAccount は作成したサイトアカウントを、残リソースがないことを確認して削除し、不在を確認します。
// 対象が不在の場合は削除済みとして扱います。
func deleteObjectStorageAccount(t *testing.T, h *helper.Helper, siteID string, expected account) {
	t.Helper()

	output, stderr, err := h.CallAllowingFailure(t, "アカウント削除前に作成したアカウントを再照合する",
		"object-storage-api", "account", "read", siteID)
	if err != nil {
		require.True(t, objectStorageNotFound(stderr, "Accounts.Read"),
			"failed to re-read the site account before deletion: %s", stderr)
		return
	}
	var actual account
	require.NoError(t, json.Unmarshal(output, &actual), "decode site account")
	require.Equal(t, expected.ResourceID, actual.ResourceID, "site account ID changed; refusing modification")

	require.Empty(t, listObjectStorageBuckets(t, h, "アカウント削除前にバケットが残っていないことを確認する", siteID),
		"site account contains buckets; refusing to delete it")
	require.Empty(t, listObjectStoragePermissions(t, h, "アカウント削除前にパーミッションが残っていないことを確認する", siteID),
		"site account contains permissions; refusing to delete it")
	require.Empty(t, objectStorageKeyList(t, h, "アカウント削除前にアクセスキーが残っていないことを確認する",
		[]string{"account", "access-key"}, []string{siteID}),
		"site account contains access keys; refusing to delete it")

	h.Call(t, "オブジェクトストレージアカウントを削除する", "object-storage-api", "account", "delete", siteID)
	_, stderr, err = h.CallAllowingFailure(t, "アカウント削除後に 404 になることを確認する",
		"object-storage-api", "account", "read", siteID)
	require.Error(t, err, "site account still exists after deletion")
	require.True(t, objectStorageNotFound(stderr, "Accounts.Read"),
		"account deletion was not confirmed by an Accounts.Read API 404: %s", stderr)
}

// ensureObjectStorageAccount はサイトアカウントが未作成なら作成し、後片付けで削除する関数を返します。
// 既存アカウントは変更・削除しません。
func ensureObjectStorageAccount(t *testing.T, h *helper.Helper, siteID string) func() {
	t.Helper()

	output, stderr, err := h.CallAllowingFailure(t, "サイトアカウントの有無を確認する",
		"object-storage-api", "account", "read", siteID)
	if err == nil {
		var existing account
		require.NoError(t, json.Unmarshal(output, &existing), "decode site account")
		require.NotEmpty(t, existing.ResourceID, "site account read returned no resource ID")
		return func() {}
	}
	require.True(t, objectStorageNotFound(stderr, "Accounts.Read"),
		"account absence was not confirmed by an Accounts.Read API 404: %s", stderr)

	output = h.Call(t, "オブジェクトストレージアカウントを作成する", "object-storage-api", "account", "create", siteID)
	var created account
	require.NoError(t, json.Unmarshal(output, &created), "decode created account")
	require.NotEmpty(t, created.ResourceID, "created account ID is unknown; manual inspection required")
	return func() { deleteObjectStorageAccount(t, h, siteID, created) }
}

// runBucketScenario は一時バケットのライフサイクルと、バケットを使う基本の権限・キー操作を検証します。
// サイトアカウントが未作成の場合は環境の依存不足としてスキップします。
func runBucketScenario(t *testing.T, h *helper.Helper, siteID, planValue string, extra bucketExtra) {
	t.Helper()

	confirmObjectStorageSite(t, h, siteID, planValue)

	// サイトアカウントが存在することを確認する
	output, stderr, err := h.CallAllowingFailure(t, "指定サイトのアカウント情報が取得できることを確認する",
		"object-storage-api", "account", "read", siteID)
	if err != nil {
		require.True(t, objectStorageNotFound(stderr, "Accounts.Read"),
			"site account must already exist: %s", stderr)
		t.Skipf("site account is absent on %s; create a site account to run this scenario", siteID)
	}
	var existing account
	require.NoError(t, json.Unmarshal(output, &existing), "decode site account")
	require.NotEmpty(t, existing.ResourceID, "site account read returned no resource ID")

	// パーミッション系 API が利用可能であることを確認する
	listObjectStoragePermissions(t, h, "パーミッション系 API が利用可能であることを確認する", siteID)

	name := objectStorageNamePrefix + helper.RandomSuffix(t)
	items := listObjectStorageBuckets(t, h, "作成前に同名バケットが存在しないことを確認する", siteID)
	require.Empty(t, bucketsNamed(items, name), "generated bucket name %s already exists; refusing to modify it", name)

	created := createObjectStorageBucket(t, h, siteID, name, planValue)
	deleted := false
	vetoed := false
	defer func() {
		if deleted {
			return
		}
		if vetoed {
			// バケット設定の所有確認ができていないため、バケットを削除せず手動確認を求める
			t.Logf("bucket deletion was vetoed; inspect site %s and bucket %s manually", siteID, name)
			return
		}
		deleteObjectStorageBucket(t, h, "クリーンアップとして作成したバケットを削除する", siteID, created)
	}()

	// 作成したバケットが一覧に含まれ、リソース ID とプランが一致することを確認する
	items = listObjectStorageBuckets(t, h, "作成したバケットが一覧に含まれることを確認する", siteID)
	matches := bucketsNamed(items, name)
	require.Len(t, matches, 1, "bucket list did not return exactly the created bucket %s", name)
	require.Equal(t, created.Name, matches[0].Name, "bucket list did not return exactly the created bucket %s", name)
	require.NotEmpty(t, matches[0].ResourceID,
		"bucket list returned no resource ID for %s; refusing automatic deletion", name)
	created.ResourceID = matches[0].ResourceID
	if len(matches[0].Plan) == 0 || string(matches[0].Plan) == "null" {
		matches[0].Plan = created.Plan
	}
	checkCreatedBucketPlan(t, matches[0], siteID, planValue)

	runBucketPermissionExercise(t, h, siteID, name)

	if extra != nil {
		extra(t, h, siteID, name, &vetoed)
	}
	if vetoed {
		// バケット設定の所有確認ができていないため、バケットを削除せず手動確認を求める
		t.Errorf("bucket deletion was vetoed; inspect site %s and bucket %s manually", siteID, name)
		return
	}

	deleteObjectStorageBucket(t, h, "作成したバケットを削除する", siteID, created)
	items = listObjectStorageBuckets(t, h, "削除後にバケットが一覧に残っていないことを確認する", siteID)
	require.Empty(t, bucketsNamed(items, name), "bucket %s remains after deletion", name)
	deleted = true
}

// encryptionKeyID は暗号化設定の KMS キー ID を取り出します。
func encryptionKeyID(output []byte) (string, error) {
	var state struct {
		KeyID string `json:"kms_key_id"`
	}
	if err := json.Unmarshal(output, &state); err != nil {
		return "", fmt.Errorf("decode encryption setting: %w", err)
	}
	if state.KeyID == "" {
		return "", errors.New("encryption setting has no key ID; refusing to modify it")
	}
	return state.KeyID, nil
}

// runEncryptionLifecycle は一時バケットの暗号化の有効化・キー ID の照合・無効化を検証します。
// 暗号化設定の所有確認ができていない場合は cleanupVetoed を true にし、バケットの削除を中止します。
func runEncryptionLifecycle(t *testing.T, h *helper.Helper, siteID, bucket, kmsKeyID string, cleanupVetoed *bool) {
	t.Helper()

	read := func(description string) ([]byte, string, error) {
		return h.CallAllowingFailure(t, description, "object-storage-api", "bucket", "encryption", "read", siteID, bucket)
	}
	// 設定が存在しない場合は true。読み取りの失敗が不在以外の場合はテストを失敗させます。
	absent := func(stderr string, err error) bool {
		if err == nil {
			return false
		}
		require.True(t, objectStorageNotFound(stderr, "BucketExtra.ReadEncryption"),
			"failed to read the encryption setting: %s", stderr)
		return true
	}

	// 無効化は現在のキー ID を再確認してから行う。所有確認ができない場合はバケット削除も中止する。
	disable := func() {
		output, stderr, err := read("バケットの暗号化設定を確認する")
		if absent(stderr, err) {
			return
		}
		keyID, keyErr := encryptionKeyID(output)
		if keyErr != nil || keyID != kmsKeyID {
			*cleanupVetoed = true
			t.Errorf("bucket encryption setting ownership is unverified; refusing to disable it or delete the bucket; manual inspection required")
			return
		}
		h.Call(t, "バケットの暗号化を無効化する", "object-storage-api", "bucket", "encryption", "disable", siteID, bucket)
		waitForObjectStorage(t, h, "暗号化の無効化を待つ", func() bool {
			output, stderr, err := read("バケットの暗号化設定を確認する")
			if absent(stderr, err) {
				return true
			}
			keyID, keyErr := encryptionKeyID(output)
			require.NoError(t, keyErr, "decode encryption setting while waiting for disable")
			require.Equal(t, kmsKeyID, keyID, "encryption key ID changed while waiting for disable")
			return false
		})
	}

	// 失敗した場合も無効化を試みる
	done := false
	defer func() {
		if !done {
			disable()
		}
	}()

	// 有効化前に新しいバケットに暗号化設定がないことを確認する
	output, stderr, err := read("有効化前にバケットの暗号化設定を確認する")
	if !absent(stderr, err) {
		if _, keyErr := encryptionKeyID(output); keyErr == nil {
			t.Errorf("new bucket unexpectedly has encryption configured")
		}
		*cleanupVetoed = true
		return
	}

	// 有効化と反映待ち
	h.Call(t, "バケットの暗号化を有効化する", "object-storage-api", "bucket", "encryption", "enable", siteID, bucket,
		"--kms-key-id", kmsKeyID)
	waitForObjectStorage(t, h, "暗号化の有効化を待つ", func() bool {
		output, stderr, err := read("バケットの暗号化設定を確認する")
		if absent(stderr, err) {
			return false
		}
		keyID, keyErr := encryptionKeyID(output)
		require.NoError(t, keyErr, "decode encryption setting while waiting for enable")
		require.Equal(t, kmsKeyID, keyID, "enabled encryption key ID does not match this scenario")
		return true
	})

	disable()
	done = true
}

// checkObjectStorageReplication はレプリケーション設定が作成したバケットと一致することを確認します。
func checkObjectStorageReplication(t *testing.T, output []byte, sourceSite, sourceBucket, targetSite, targetBucket string) {
	t.Helper()

	var actual replication
	require.NoError(t, json.Unmarshal(output, &actual), "decode replication")
	require.Equal(t, sourceBucket, actual.Source.Name,
		"replication source does not match the created bucket")
	require.Equal(t, sourceSite, actual.Source.ClusterID,
		"replication source does not match the created bucket")
	require.Equal(t, targetBucket, actual.Target.Name,
		"replication target does not match the created bucket")
	require.Equal(t, targetSite, actual.Target.ClusterID,
		"replication target does not match the created bucket")
	require.Contains(t, []string{"creating", "created"}, actual.Status,
		"replication status does not match the created buckets")
}

// runReplicationLifecycle は2バケット間のレプリケーション設定の有効化・照合・無効化を検証します。
func runReplicationLifecycle(t *testing.T, h *helper.Helper, sourceSite, sourceBucket, targetSite, targetBucket string) {
	t.Helper()

	read := func(description string) ([]byte, string, error) {
		return h.CallAllowingFailure(t, description, "object-storage-api", "bucket", "replication", "read", sourceSite, sourceBucket)
	}
	// 設定が存在しない場合は true。読み取りの失敗が不在以外の場合はテストを失敗させます。
	absent := func(stderr string, err error) bool {
		if err == nil {
			return false
		}
		require.True(t, objectStorageNotFound(stderr, "BucketExtra.ReadReplication"),
			"failed to read the replication setting: %s", stderr)
		return true
	}

	// 無効化は現在の設定を再確認してから行う
	disable := func() {
		output, stderr, err := read("バケットのレプリケーション設定を確認する")
		if absent(stderr, err) {
			return
		}
		checkObjectStorageReplication(t, output, sourceSite, sourceBucket, targetSite, targetBucket)
		h.Call(t, "バケットのレプリケーションを無効化する", "object-storage-api", "bucket", "replication", "disable", sourceSite, sourceBucket)
		waitForObjectStorage(t, h, "レプリケーションの無効化を待つ", func() bool {
			_, stderr, err := read("バケットのレプリケーション設定を確認する")
			return absent(stderr, err)
		})
	}

	// 失敗した場合も無効化を試みる
	done := false
	defer func() {
		if !done {
			disable()
		}
	}()

	// 有効化前に新しい送信元バケットにレプリケーション設定がないことを確認する
	_, stderr, err := read("新しいバケットのレプリケーション設定を確認する")
	require.True(t, absent(stderr, err), "new source bucket must have no replication setting")

	// 有効化と反映待ち
	output := h.Call(t, "バケットのレプリケーションを有効化する", "object-storage-api", "bucket", "replication", "enable",
		sourceSite, sourceBucket, "--target-bucket", targetBucket)
	checkObjectStorageReplication(t, output, sourceSite, sourceBucket, targetSite, targetBucket)
	waitForObjectStorage(t, h, "レプリケーションの作成を待つ", func() bool {
		output, stderr, err := read("バケットのレプリケーション設定を確認する")
		require.NoError(t, err, "failed to read the replication setting while waiting: %s", stderr)
		checkObjectStorageReplication(t, output, sourceSite, sourceBucket, targetSite, targetBucket)
		var actual replication
		require.NoError(t, json.Unmarshal(output, &actual), "decode replication")
		return actual.Status == "created"
	})

	disable()
	done = true
}
