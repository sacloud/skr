// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// destinationIDEnv は既存通知先の ID を読み取る環境変数です。
const destinationIDEnv = "SKR_E2E_SIMPLE_NOTIFICATION_DESTINATION_ID"

// crudListQuery はリソース共通の一覧を射影する --query です。
const crudListQuery = ".CommonServiceItems | map({ID,Name,Description,Tags,Settings,Icon})"

// crudItem は --query で射影した CommonServiceItem です。
// Settings はリソースごとに構造が異なるため map で扱います。
type crudItem struct {
	ID          string         `json:"ID,omitempty"`
	Name        string         `json:"Name"`
	Description string         `json:"Description"`
	Tags        []string       `json:"Tags"`
	Settings    map[string]any `json:"Settings,omitempty"`
	Icon        any            `json:"Icon"`
}

// crudResponse は CLI の個別参照・作成・更新応答です。
type crudResponse struct {
	CommonServiceItem crudItem `json:"CommonServiceItem"`
}

// kindName は一時リソースの種類を日本語で返します。
func kindName(kind string) string {
	switch kind {
	case "destination":
		return "通知先"
	case "group":
		return "グループ"
	case "routing":
		return "ルーティング"
	}
	return kind
}

// destinationIDFromEnv は既存通知先の ID を環境変数から読み取ります。
// 未設定の場合はテストをスキップします。既存通知先は利用者が承認したものであり、テストは変更・削除しません。
func destinationIDFromEnv(t *testing.T) string {
	t.Helper()

	id := os.Getenv(destinationIDEnv)
	if id == "" {
		t.Skipf("%s not set, skipping E2E test", destinationIDEnv)
	}
	return id
}

// listCRUDItems は射影済みの一覧を取得します。取得に失敗した場合はテストを停止します。
func listCRUDItems(t *testing.T, h *helper.Helper, description, resource string) []crudItem {
	t.Helper()

	data := h.Call(t, description, "simple-notification-api", resource, "list", "--query", crudListQuery)
	var items []crudItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode list: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON resource array", description)
	return items
}

// listCRUDItemsAllowingFailure は後片付けで射影済みの一覧を取得します。
// 失敗してもテストを停止せず、その失敗をテストの結果へ反映します。
func listCRUDItemsAllowingFailure(t *testing.T, h *helper.Helper, description, resource string) []crudItem {
	t.Helper()

	data, stderr, err := h.CallAllowingFailure(t, description,
		"simple-notification-api", resource, "list", "--query", crudListQuery)
	if err != nil {
		t.Errorf("%s: %v\n%s", description, err, stderr)
		return nil
	}
	var items []crudItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Errorf("%s: decode list: %v", description, err)
		return nil
	}
	return items
}

// readCRUDItem は ID を指定してリソースを読み取ります。
func readCRUDItem(t *testing.T, h *helper.Helper, description, resource, id string) crudItem {
	t.Helper()

	data := h.Call(t, description, "simple-notification-api", resource, "read", id)
	var response crudResponse
	err := json.Unmarshal(data, &response)
	require.NoError(t, err, "%s: decode resource: %v", description, err)
	require.Equal(t, id, response.CommonServiceItem.ID, "%s: resource ID mismatch", description)
	return response.CommonServiceItem
}

// parseCRUDResponse は作成・更新応答をデコードします。
func parseCRUDResponse(t *testing.T, data []byte, description string) crudItem {
	t.Helper()

	var response crudResponse
	err := json.Unmarshal(data, &response)
	require.NoError(t, err, "%s: decode resource response: %v", description, err)
	return response.CommonServiceItem
}

// matchesCRUDItem は実際の項目が期待値と一致するかを判定します。
// タグは順序に依存せず比較し、Settings は期待値に含まれるキーだけ比較します。
func matchesCRUDItem(actual, expected crudItem) bool {
	actualTags, expectedTags := slices.Clone(actual.Tags), slices.Clone(expected.Tags)
	slices.Sort(actualTags)
	slices.Sort(expectedTags)
	if actual.ID != expected.ID || actual.Name != expected.Name || actual.Description != expected.Description ||
		!slices.Equal(actualTags, expectedTags) {
		return false
	}
	for key, value := range expected.Settings {
		if !reflect.DeepEqual(actual.Settings[key], value) {
			return false
		}
	}
	return true
}

// matchesExactCRUDItem は Settings と Icon も含めて完全一致するかを判定します。
func matchesExactCRUDItem(actual, expected crudItem) bool {
	return matchesCRUDItem(actual, expected) &&
		reflect.DeepEqual(actual.Settings, expected.Settings) &&
		reflect.DeepEqual(actual.Icon, expected.Icon)
}

func TestMatchesExactCRUDItemChecksFullStateAndIgnoresTagOrder(t *testing.T) {
	expected := crudItem{
		ID: "123", Name: "existing", Description: "baseline",
		Tags: []string{"one", "two"}, Settings: map[string]any{"Disabled": false}, Icon: "icon",
	}
	reorderedTags := expected
	reorderedTags.Tags = []string{"two", "one"}
	require.True(t, matchesExactCRUDItem(reorderedTags, expected),
		"tag order should not affect exact resource comparison")
	for _, mutate := range []func(*crudItem){
		func(actual *crudItem) { actual.Settings["Added"] = true },
		func(actual *crudItem) { delete(actual.Settings, "Disabled") },
		func(actual *crudItem) { actual.Icon = "changed" },
	} {
		actual := expected
		actual.Tags = slices.Clone(expected.Tags)
		actual.Settings = map[string]any{"Disabled": false}
		mutate(&actual)
		require.False(t, matchesExactCRUDItem(actual, expected), "accepted resource state change: %+v", actual)
	}
}

func TestMatchesCRUDItemChecksOnlyExpectedSettingKeys(t *testing.T) {
	expected := crudItem{
		ID: "123", Name: "item", Description: "desc",
		Tags: []string{"skr-e2e"}, Settings: map[string]any{"Disabled": true},
	}
	actual := crudItem{
		ID: "123", Name: "item", Description: "desc",
		Tags: []string{"skr-e2e"}, Settings: map[string]any{"Disabled": true, "Added": "x"},
	}
	require.True(t, matchesCRUDItem(actual, expected),
		"extra actual settings keys should not affect the comparison")
	actual.Settings["Disabled"] = false
	require.False(t, matchesCRUDItem(actual, expected),
		"changed expected setting keys should affect the comparison")
}
