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

// このファイルは product disk コマンド E2E のシナリオです。
// ディスク製品プランの検索・参照を確認します。
package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// diskPlanZone はディスク製品プランを検索するゾーンです。
const diskPlanZone = "is1b"

// diskPlanQuery は検索結果に含める項目を選択する jq 式です。
const diskPlanQuery = `{Total,From,Count,DiskPlans:(.DiskPlans | map({ID,Name,StorageClass,Availability,Size}))}`

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

// TestScenarioDiskPlan は product disk コマンドのライブ E2E です。
// 選択中のプロファイルの確認と、指定ゾーンのディスク製品プランの検索・参照、
// Find の結果と Read の一致、ページング指定の確認を行います。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioDiskPlan(t *testing.T) {
	h := helper.New(t, "diskplan-api",
		"IaaS API の product disk コマンドで選択中のプロファイルの確認と、指定ゾーンのディスク製品プランの検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// 指定ゾーンのディスク製品プランを検索してページングと ID を確認する
	plans := findDiskPlans(t, h, "指定ゾーンのディスク製品プランを検索する")
	require.Len(t, plans, 1, "product disk find should return one record")
	plan := plans[0]
	require.Positive(t, plan.ID, "disk plan has no positive ID")
	require.NotEmpty(t, plan.Name, "disk plan has an empty name")
	require.NotEmpty(t, plan.Size, "disk plan has no size data")

	// 検索したディスク製品プランを ID で参照して検索結果と一致させる
	read := readDiskPlan(t, h, "ディスク製品プランを ID で参照する", plan.ID)
	require.Equal(t, plan.ID, read.ID, "read disk plan ID should match the find result")
	require.Equal(t, plan.Name, read.Name, "read disk plan name should match the find result")
	require.Equal(t, plan.StorageClass, read.StorageClass, "read disk plan storage class should match the find result")
	require.Equal(t, plan.Availability, read.Availability, "read disk plan availability should match the find result")
	require.Equal(t, plan.Size, read.Size, "read disk plan size data should match the find result")
}

// findDiskPlans は指定ゾーンのディスク製品プランを検索して応答を返します。
// Find 応答のページング情報が要求 (--count 1 --from 0) と一致することを確認します。
// この確認で --count と --from が API まで伝わっていることを検証します。
func findDiskPlans(t *testing.T, h *helper.Helper, description string) []diskPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "disk", "find",
		"--zone", diskPlanZone, "--count", "1", "--from", "0", "--query", diskPlanQuery)
	require.NoError(t, err, "%s: product disk find: %v", description, err)
	var find diskPlanFindSummary
	require.NoError(t, json.Unmarshal(data, &find), "%s: decode disk plan list: %v", description, err)
	require.NotNil(t, find.Total, "Find response is missing its Total")
	require.NotNil(t, find.From, "Find response is missing its From")
	require.NotNil(t, find.Count, "Find response is missing its Count")
	require.Equal(t, 0, *find.From, "Find response From should match the requested --from 0")
	require.LessOrEqual(t, *find.Count, 1, "Find response Count should honor the requested --count 1")
	require.Equal(t, *find.Count, len(find.DiskPlans), "Find response Count should match the result items")
	return find.DiskPlans
}

// readDiskPlan は指定した ID のディスク製品プランを参照して応答を返します。
func readDiskPlan(t *testing.T, h *helper.Helper, description string, id int64) diskPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "disk", "read",
		"--zone", diskPlanZone, "--id", strconv.FormatInt(id, 10),
		"--query", "{ID,Name,StorageClass,Availability,Size}")
	require.NoError(t, err, "%s: product disk read %d: %v", description, id, err)
	var plan diskPlanSummary
	require.NoError(t, json.Unmarshal(data, &plan), "%s: decode disk plan: %v", description, err)
	return plan
}
