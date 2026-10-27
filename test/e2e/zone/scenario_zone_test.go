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
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// zoneItem はゾーンの一覧と参照で確認する項目です。
type zoneItem struct {
	ID          int64
	Name        string
	Description string
}

// TestScenarioZone は zone コマンドのライブ E2E です。
// ゾーン一覧の取得、jq 式での名前抽出、ID での参照を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioZone(t *testing.T) {
	h := helper.New(t, "zone-api",
		"IaaS API の zone コマンドでゾーン一覧の取得、名前の抽出、ID での参照が正しく動作することを確認する")

	// ゾーン一覧を取得して ID と名前を確認する
	zones := findZones(t, h, "ゾーン一覧を取得する")
	require.NotEmpty(t, zones, "zone list is empty")
	for i, z := range zones {
		require.NotZero(t, z.ID, "zone %d has no ID", i)
		require.NotEmpty(t, z.Name, "zone %d has an empty name", i)
	}

	// jq 式でゾーン名を抽出して一覧と一致させる
	expected := make([]string, 0, len(zones))
	for _, z := range zones {
		expected = append(expected, z.Name)
	}
	require.Equal(t, expected, findZoneNames(t, h, "jq 式でゾーン名を抽出する"),
		"zone names from --query output should match the zone list")

	// 一覧の先頭ゾーンを ID で参照して一覧と一致させる
	first := zones[0]
	read := readZone(t, h, "ゾーンを ID で参照する", first.ID)
	require.Equal(t, first.ID, read.ID, "read zone ID should match the zone list")
	require.Equal(t, first.Name, read.Name, "read zone name should match the zone list")
}

// findZones はゾーン一覧を取得して ID と名前を検証します。
func findZones(t *testing.T, h *helper.Helper, description string) []zoneItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "zone", "find")
	require.NoError(t, err, "%s: zone find: %v", description, err)
	var zones []zoneItem
	require.NoError(t, json.Unmarshal(data, &zones), "%s: decode zone list: %v", description, err)
	return zones
}

// findZoneNames は jq 式でゾーン名の配列を取得して検証します。
func findZoneNames(t *testing.T, h *helper.Helper, description string) []string {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "zone", "find", "--query", "map(.Name)")
	require.NoError(t, err, "%s: zone find --query: %v", description, err)
	var names []string
	require.NoError(t, json.Unmarshal(data, &names), "%s: decode zone names: %v", description, err)
	return names
}

// readZone は指定した ID のゾーンを参照して応答を検証します。
func readZone(t *testing.T, h *helper.Helper, description string, id int64) zoneItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "zone", "read",
		"--id", strconv.FormatInt(id, 10), "--query", "{ID,Name,Description}")
	require.NoError(t, err, "%s: zone read %d: %v", description, id, err)
	var item zoneItem
	require.NoError(t, json.Unmarshal(data, &item), "%s: decode zone: %v", description, err)
	return item
}
