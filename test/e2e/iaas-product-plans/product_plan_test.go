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

// このファイルは IaaS 製品プラン E2E の共有ヘルパーです。
// 製品プランの検索・参照とページング情報の検証をまとめています。
package main

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// productPlanZone はゾーン指定が必要な製品プランを検索するゾーンです。
const productPlanZone = "is1b"

// internetPlanSummary はインターネットプランの検索・参照で確認する項目です。
type internetPlanSummary struct {
	ID            int64
	Name          string
	BandWidthMbps int
	Availability  string
}

// licenseInfoSummary はライセンス情報の検索で確認する項目です。
type licenseInfoSummary struct {
	ID   int64
	Name string
}

// privateHostPlanSummary はプライベートホストプランの検索・参照で確認する項目です。
type privateHostPlanSummary struct {
	ID           int64
	Name         string
	Class        string
	CPU          int
	CPUModel     string
	Dedicated    bool
	MemoryMB     int
	Availability string
}

type internetPlanFindSummary struct {
	Total         *int                  `json:"Total"`
	From          *int                  `json:"From"`
	Count         *int                  `json:"Count"`
	InternetPlans []internetPlanSummary `json:"InternetPlans"`
}

type licenseInfoFindSummary struct {
	Total       *int                 `json:"Total"`
	From        *int                 `json:"From"`
	Count       *int                 `json:"Count"`
	LicenseInfo []licenseInfoSummary `json:"LicenseInfo"`
}

type privateHostPlanFindSummary struct {
	Total            *int                     `json:"Total"`
	From             *int                     `json:"From"`
	Count            *int                     `json:"Count"`
	PrivateHostPlans []privateHostPlanSummary `json:"PrivateHostPlans"`
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

// findInternetPlans は指定ゾーンのインターネットプランを検索して応答を返します。
func findInternetPlans(t *testing.T, h *helper.Helper, description string) []internetPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "internet", "find",
		"--zone", productPlanZone, "--count", "1", "--from", "0",
		"--query", `{Total,From,Count,InternetPlans:(if (.InternetPlans | type) == "array" then (.InternetPlans | map({ID,Name,BandWidthMbps,Availability})) else error("InternetPlans must be an array") end)}`)
	require.NoError(t, err, "%s: product internet find: %v", description, err)
	var find internetPlanFindSummary
	require.NoError(t, json.Unmarshal(data, &find), "%s: decode InternetPlan find output: %v", description, err)
	checkFindPage(t, find.Total, find.From, find.Count, len(find.InternetPlans))
	return find.InternetPlans
}

// readInternetPlan は指定した ID のインターネットプランを参照して応答を返します。
func readInternetPlan(t *testing.T, h *helper.Helper, description string, id int64) internetPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "internet", "read",
		"--zone", productPlanZone, "--id", strconv.FormatInt(id, 10))
	require.NoError(t, err, "%s: product internet read %d: %v", description, id, err)
	var plan internetPlanSummary
	require.NoError(t, json.Unmarshal(data, &plan), "%s: decode InternetPlan read output: %v", description, err)
	return plan
}

// findLicenseInfos はライセンス情報を検索して応答を返します。
func findLicenseInfos(t *testing.T, h *helper.Helper, description string) []licenseInfoSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "license", "find",
		"--count", "1", "--from", "0",
		"--query", `{Total,From,Count,LicenseInfo:(if (.LicenseInfo | type) == "array" then (.LicenseInfo | map({ID,Name})) else error("LicenseInfo must be an array") end)}`)
	require.NoError(t, err, "%s: product license find: %v", description, err)
	var find licenseInfoFindSummary
	require.NoError(t, json.Unmarshal(data, &find), "%s: decode LicenseInfo find output: %v", description, err)
	checkFindPage(t, find.Total, find.From, find.Count, len(find.LicenseInfo))
	return find.LicenseInfo
}

// findPrivateHostPlans は指定ゾーンのプライベートホストプランを検索して応答を返します。
func findPrivateHostPlans(t *testing.T, h *helper.Helper, description string) []privateHostPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "privatehost", "find",
		"--zone", productPlanZone, "--count", "1", "--from", "0",
		"--query", `{Total,From,Count,PrivateHostPlans:(if (.PrivateHostPlans | type) == "array" then (.PrivateHostPlans | map({ID,Name,Class,CPU,CPUModel,Dedicated,MemoryMB,Availability})) else error("PrivateHostPlans must be an array") end)}`)
	require.NoError(t, err, "%s: product privatehost find: %v", description, err)
	var find privateHostPlanFindSummary
	require.NoError(t, json.Unmarshal(data, &find), "%s: decode PrivateHostPlan find output: %v", description, err)
	checkFindPage(t, find.Total, find.From, find.Count, len(find.PrivateHostPlans))
	return find.PrivateHostPlans
}

// readPrivateHostPlan は指定した ID のプライベートホストプランを参照して応答を返します。
func readPrivateHostPlan(t *testing.T, h *helper.Helper, description string, id int64) privateHostPlanSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "product", "privatehost", "read",
		"--zone", productPlanZone, "--id", strconv.FormatInt(id, 10))
	require.NoError(t, err, "%s: product privatehost read %d: %v", description, id, err)
	var plan privateHostPlanSummary
	require.NoError(t, json.Unmarshal(data, &plan), "%s: decode PrivateHostPlan read output: %v", description, err)
	return plan
}
