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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// regionSummary はリージョンの検索・参照で確認する項目です。
type regionSummary struct {
	ID          int64
	Name        string
	Description string
	NameServers []string
}

// TestScenarioRegion は region コマンドのライブ E2E です。
// 選択中のプロファイルの確認と、リージョンの検索・参照、
// Find の結果と Read の一致を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioRegion(t *testing.T) {
	h := helper.New(t, "region-api",
		"IaaS API の region コマンドで選択中のプロファイルの確認と、リージョンの検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// リージョンを検索して ID と名前を確認する
	regions := findRegions(t, h, "リージョンを検索する")
	require.NotEmpty(t, regions, "region list is empty")
	for i, r := range regions {
		require.Positive(t, r.ID, "region %d has no positive ID", i)
		require.NotEmpty(t, r.Name, "region %d has an empty name", i)
	}

	// 検索結果の先頭リージョンを ID で参照して検索結果と一致させる
	first := regions[0]
	read := readRegion(t, h, "リージョンを ID で参照する", first.ID)
	require.Equal(t, first.ID, read.ID, "read region ID should match the find result")
	require.Equal(t, first.Name, read.Name, "read region name should match the find result")
	require.Equal(t, first.Description, read.Description, "read region description should match the find result")
	require.Equal(t, first.NameServers, read.NameServers, "read region name servers should match the find result")
}

// findRegions はリージョンを検索して応答を返します。
func findRegions(t *testing.T, h *helper.Helper, description string) []regionSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "region", "find",
		"--query", "map({ID,Name,Description,NameServers})")
	require.NoError(t, err, "%s: region find: %v", description, err)
	var regions []regionSummary
	require.NoError(t, json.Unmarshal(data, &regions), "%s: decode region list: %v", description, err)
	return regions
}

// readRegion は指定した ID のリージョンを参照して応答を返します。
func readRegion(t *testing.T, h *helper.Helper, description string, id int64) regionSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "region", "read",
		"--id", strconv.FormatInt(id, 10), "--query", "{ID,Name,Description,NameServers}")
	require.NoError(t, err, "%s: region read %d: %v", description, id, err)
	var region regionSummary
	require.NoError(t, json.Unmarshal(data, &region), "%s: decode region: %v", description, err)
	return region
}
