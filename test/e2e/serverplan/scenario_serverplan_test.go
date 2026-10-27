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

// serverPlanZone はサーバープランを検索するゾーンです。
const serverPlanZone = "is1b"

// serverPlanQuery は検索結果に含める項目を選択する jq 式です。
const serverPlanQuery = ".ServerPlans | map({ID,Name,CPU,MemoryMB,GPU,CPUModel,GPUModel,Commitment,Generation,ConfidentialVM,Availability})"

// serverPlanSummary はサーバープランの検索・参照で確認する項目です。
type serverPlanSummary struct {
	ID             int64
	Name           string
	CPU            int
	MemoryMB       int
	GPU            int
	CPUModel       string
	GPUModel       string
	Commitment     string
	Generation     int
	ConfidentialVM bool
	Availability   string
}

// TestScenarioServerPlan は product server コマンドのライブ E2E です。
// 選択中のプロファイルの確認と、指定ゾーンのサーバープランの検索・参照、
// Find の結果と Read の ID・プラン情報の一致を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioServerPlan(t *testing.T) {
	h := helper.New(t, "serverplan-api",
		"IaaS API の product server コマンドで選択中のプロファイルの確認と、指定ゾーンのサーバープランの検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// 指定ゾーンのサーバープランを検索する
	plans := findServerPlans(t, h, "指定ゾーンのサーバープランを検索する")
	require.Len(t, plans, 1, "product server find should return one record")
	plan := plans[0]
	require.Positive(t, plan.ID, "server plan has no positive ID")
	require.NotEmpty(t, plan.Name, "server plan has an empty name")

	// 検索したサーバープランを ID で参照して検索結果と一致させる
	read := readServerPlan(t, h, "サーバープランを ID で参照する", plan.ID)
	require.Equal(t, plan.ID, read.ID, "read plan ID should match the find result")
	require.Equal(t, plan.Name, read.Name, "read plan name should match the find result")
	require.Equal(t, plan.CPU, read.CPU, "read plan CPU should match the find result")
	require.Equal(t, plan.MemoryMB, read.MemoryMB, "read plan memory should match the find result")
	require.Equal(t, plan.GPU, read.GPU, "read plan GPU should match the find result")
}

// findServerPlans は指定ゾーンのサーバープランを検索して応答を返します。
func findServerPlans(t *testing.T, h *helper.Helper, description string) []serverPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "server", "find",
		"--zone", serverPlanZone, "--count", "1", "--from", "0", "--query", serverPlanQuery)
	require.NoError(t, err, "%s: product server find: %v", description, err)
	var plans []serverPlanSummary
	require.NoError(t, json.Unmarshal(data, &plans), "%s: decode server plan list: %v", description, err)
	return plans
}

// readServerPlan は指定した ID のサーバープランを参照して応答を返します。
func readServerPlan(t *testing.T, h *helper.Helper, description string, id int64) serverPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "server", "read",
		"--zone", serverPlanZone, "--id", strconv.FormatInt(id, 10))
	require.NoError(t, err, "%s: product server read %d: %v", description, id, err)
	var plan serverPlanSummary
	require.NoError(t, json.Unmarshal(data, &plan), "%s: decode server plan: %v", description, err)
	return plan
}
