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

// このファイルは archive コマンド E2E のシナリオです。
// アーカイブの検索・参照を確認します。
package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// archiveZone はアーカイブを検索するゾーンです。
const archiveZone = "is1b"

// archiveQuery は Find の検索結果に含める項目を選択する jq 式です。
const archiveQuery = "map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})"

// archiveReadQuery は Read の参照結果に含める項目を選択する jq 式です。
const archiveReadQuery = "{ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags}"

// archiveSummary はアーカイブの検索・参照で確認する項目です。
type archiveSummary struct {
	ID           int64
	Name         string
	Description  string
	DiskPlanID   int64
	DiskPlanName string
	SizeMB       int
	Tags         []string
}

// TestScenarioArchive は archive コマンドのライブ E2E です。
// 選択中のプロファイルの確認と、指定ゾーンのアーカイブの検索・参照、
// Find の結果と Read の一致を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioArchive(t *testing.T) {
	h := helper.New(t, "archive-api",
		"IaaS API の archive コマンドで選択中のプロファイルの確認と、指定ゾーンのアーカイブの検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// 指定ゾーンのアーカイブを検索して ID と名前を確認する
	archives := findArchives(t, h, "指定ゾーンのアーカイブを検索する")
	require.NotEmpty(t, archives, "Archive Find returned no record")
	archive := archives[0]
	require.Positive(t, archive.ID, "archive has no positive ID")
	require.NotEmpty(t, archive.Name, "archive has an empty name")

	// 検索したアーカイブを ID で参照して検索結果と一致させる
	read := readArchive(t, h, "アーカイブを ID で参照する", archive.ID)
	require.Equal(t, archive.ID, read.ID, "read archive ID should match the find result")
	require.Equal(t, archive.Name, read.Name, "read archive name should match the find result")
	require.Equal(t, archive.Description, read.Description, "read archive description should match the find result")
	require.Equal(t, archive.DiskPlanID, read.DiskPlanID, "read archive disk plan ID should match the find result")
	require.Equal(t, archive.DiskPlanName, read.DiskPlanName, "read archive disk plan name should match the find result")
	require.Equal(t, archive.SizeMB, read.SizeMB, "read archive size should match the find result")
	// Tags の並び順は検索と参照で変わることがあるため、要素の一致だけを確認します
	require.ElementsMatch(t, archive.Tags, read.Tags, "read archive tags should match the find result")
}

// findArchives は指定ゾーンのアーカイブを検索して応答を返します。
func findArchives(t *testing.T, h *helper.Helper, description string) []archiveSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "archive", "find",
		"--zone", archiveZone, "--query", archiveQuery)
	require.NoError(t, err, "%s: archive find: %v", description, err)
	var archives []archiveSummary
	require.NoError(t, json.Unmarshal(data, &archives), "%s: decode archive list: %v", description, err)
	return archives
}

// readArchive は指定した ID のアーカイブを参照して応答を返します。
func readArchive(t *testing.T, h *helper.Helper, description string, id int64) archiveSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "archive", "read",
		"--zone", archiveZone, "--id", strconv.FormatInt(id, 10), "--query", archiveReadQuery)
	require.NoError(t, err, "%s: archive read %d: %v", description, id, err)
	var archive archiveSummary
	require.NoError(t, json.Unmarshal(data, &archive), "%s: decode archive: %v", description, err)
	return archive
}
