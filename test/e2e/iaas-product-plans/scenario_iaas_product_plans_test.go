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

// このファイルは IaaS 製品プラン E2E のシナリオです。
// ライセンス情報の検索と、インターネット・プライベートホストプランの検索・参照を確認します。
package main

import (
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioIaasProductPlans は IaaS 製品プランコマンドのライブ E2E です。
// 選択中のプロファイルの確認と、is1b のインターネットプランとプライベートホストプランの
// 検索・参照、Find の結果と Read の一致、ライセンス情報の検索を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioIaasProductPlans(t *testing.T) {
	h := helper.New(t, "iaas-product-plans-api",
		"IaaS API でインターネット、ライセンス、プライベートホストの製品プラン検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// is1b のインターネットプランを検索して ID と名前を確認する
	internetPlans := findInternetPlans(t, h, "is1b のインターネットプランを検索する")
	require.Len(t, internetPlans, 1, "InternetPlan Find returned %d records, want one plan", len(internetPlans))
	require.Positive(t, internetPlans[0].ID, "internet plan has no positive ID")
	require.NotEmpty(t, internetPlans[0].Name, "internet plan has an empty name")

	// 検索したインターネットプランを ID で参照して検索結果と一致させる
	internetRead := readInternetPlan(t, h, "検索したインターネットプランを ID で参照する", internetPlans[0].ID)
	require.Equal(t, internetPlans[0], internetRead, "InternetPlan Read did not match the Find result for ID %d", internetPlans[0].ID)

	// ライセンス情報を検索して ID と名前を確認する
	licenses := findLicenseInfos(t, h, "ライセンス情報を検索する")
	for i, license := range licenses {
		require.Positive(t, license.ID, "license %d has no positive ID", i)
		require.NotEmpty(t, license.Name, "license %d has an empty name", i)
	}

	// is1b のプライベートホストプランを検索して ID と名前を確認する
	hostPlans := findPrivateHostPlans(t, h, "is1b のプライベートホストプランを検索する")
	require.Len(t, hostPlans, 1, "PrivateHostPlan Find returned %d records, want one plan", len(hostPlans))
	require.Positive(t, hostPlans[0].ID, "private host plan has no positive ID")
	require.NotEmpty(t, hostPlans[0].Name, "private host plan has an empty name")

	// 検索したプライベートホストプランを ID で参照して検索結果と一致させる
	hostRead := readPrivateHostPlan(t, h, "検索したプライベートホストプランを ID で参照する", hostPlans[0].ID)
	require.Equal(t, hostPlans[0], hostRead, "PrivateHostPlan Read did not match the Find result for ID %d", hostPlans[0].ID)
}
