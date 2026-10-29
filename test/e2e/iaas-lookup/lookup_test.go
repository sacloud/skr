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

// このファイルは IaaS 検索 E2E の共有ヘルパーです。
// アーカイブとディスク製品プランの検索・参照をまとめています。
package main

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// lookupZone はアーカイブとディスク製品プランを検索するゾーンです。
const lookupZone = "is1b"

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

// diskPlanSize はディスク製品プランのサイズ情報で確認する項目です。
type diskPlanSize struct {
	Availability  string
	DisplaySize   int
	DisplaySuffix string
	SizeMB        int
}

// diskPlanSummary はディスク製品プランの検索・参照で確認する項目です。
type diskPlanSummary struct {
	ID           int64
	Name         string
	StorageClass string
	Availability string
	Size         []diskPlanSize
}

// diskPlanFindSummary はディスク製品プランの Find 応答で確認する項目です。
type diskPlanFindSummary struct {
	Total     *int              `json:"Total"`
	From      *int              `json:"From"`
	Count     *int              `json:"Count"`
	DiskPlans []diskPlanSummary `json:"DiskPlans"`
}

// checkFindPage は Find 応答のページング情報が要求 (--count 1 --from 0) と一致することを確認します。
// この確認で --count と --from が API まで伝わっていることを検証します。
func checkFindPage(t *testing.T, total, from, count *int, itemCount int) {
	t.Helper()
	require.NotNil(t, total, "Find response is missing its Total")
	require.NotNil(t, from, "Find response is missing its From")
	require.NotNil(t, count, "Find response is missing its Count")
	require.Equal(t, 0, *from, "Find response From should match the requested --from 0")
	require.LessOrEqual(t, *count, 1, "Find response Count should honor the requested --count 1")
	require.Equal(t, *count, itemCount, "Find response Count should match the result items")
}

// findArchives は指定ゾーンのアーカイブを検索して応答を返します。
func findArchives(t *testing.T, h *helper.Helper, description string) []archiveSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "archive", "find",
		"--zone", lookupZone,
		"--query", "map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})")
	require.NoError(t, err, "%s: archive find: %v", description, err)
	var archives []archiveSummary
	require.NoError(t, json.Unmarshal(data, &archives), "%s: decode Archive find output: %v", description, err)
	return archives
}

// readArchive は指定した ID のアーカイブを参照して応答を返します。
func readArchive(t *testing.T, h *helper.Helper, description string, id int64) archiveSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "archive", "read",
		"--zone", lookupZone, "--id", strconv.FormatInt(id, 10),
		"--query", "{ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags}")
	require.NoError(t, err, "%s: archive read %d: %v", description, id, err)
	var archive archiveSummary
	require.NoError(t, json.Unmarshal(data, &archive), "%s: decode Archive read output: %v", description, err)
	return archive
}

// findDiskPlans は指定ゾーンのディスク製品プランを検索して応答を返します。
func findDiskPlans(t *testing.T, h *helper.Helper, description string) []diskPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "disk", "find",
		"--zone", lookupZone, "--count", "1", "--from", "0",
		"--query", `{Total,From,Count,DiskPlans:(.DiskPlans | map({ID,Name,StorageClass,Availability,Size}))}`)
	require.NoError(t, err, "%s: product disk find: %v", description, err)
	var find diskPlanFindSummary
	require.NoError(t, json.Unmarshal(data, &find), "%s: decode DiskPlan find output: %v", description, err)
	checkFindPage(t, find.Total, find.From, find.Count, len(find.DiskPlans))
	return find.DiskPlans
}

// readDiskPlan は指定した ID のディスク製品プランを参照して応答を返します。
func readDiskPlan(t *testing.T, h *helper.Helper, description string, id int64) diskPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "disk", "read",
		"--zone", lookupZone, "--id", strconv.FormatInt(id, 10),
		"--query", "{ID,Name,StorageClass,Availability,Size}")
	require.NoError(t, err, "%s: product disk read %d: %v", description, id, err)
	var plan diskPlanSummary
	require.NoError(t, json.Unmarshal(data, &plan), "%s: decode DiskPlan read output: %v", description, err)
	return plan
}
