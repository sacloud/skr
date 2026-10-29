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

// このファイルは IaaS 検索 E2E のシナリオです。
// アーカイブとディスク製品プランの検索・参照を確認します。
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioIaasLookup はアーカイブとディスク製品プランの検索・参照のライブ E2E です。
// 選択中のプロファイルの確認、is1b のアーカイブの検索・参照と Find 結果と Read の一致、
// ディスク製品プランの検索・参照と Find 結果と Read の一致、ページング指定の確認を行います。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioIaasLookup(t *testing.T) {
	h := helper.New(t, "iaas-lookup-api",
		"IaaS API でアーカイブとディスク製品プランの検索・参照が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// is1b のアーカイブを検索して ID と名前を確認する
	archives := findArchives(t, h, "is1b のアーカイブを検索する")
	require.NotEmpty(t, archives, "Archive Find returned no record")
	require.Positive(t, archives[0].ID, "archive has no positive ID")
	require.NotEmpty(t, archives[0].Name, "archive has an empty name")

	// 検索したアーカイブを ID で参照して検索結果と一致させる
	archiveRead := readArchive(t, h, "検索したアーカイブを ID で参照する", archives[0].ID)
	require.Equal(t, archives[0].ID, archiveRead.ID, "Archive Read did not match the Find result for ID %d", archives[0].ID)
	require.Equal(t, archives[0].Name, archiveRead.Name, "Archive Read did not match the Find result for name")
	require.Equal(t, archives[0].Description, archiveRead.Description, "Archive Read did not match the Find result for description")
	require.Equal(t, archives[0].DiskPlanID, archiveRead.DiskPlanID, "Archive Read did not match the Find result for disk plan ID")
	require.Equal(t, archives[0].DiskPlanName, archiveRead.DiskPlanName, "Archive Read did not match the Find result for disk plan name")
	require.Equal(t, archives[0].SizeMB, archiveRead.SizeMB, "Archive Read did not match the Find result for size")
	// Tags の並び順は検索と参照で変わることがあるため、要素の一致だけを確認します
	require.ElementsMatch(t, archives[0].Tags, archiveRead.Tags, "Archive Read did not match the Find result for tags")

	// is1b のディスク製品プランを検索してページングと ID を確認する
	diskPlans := findDiskPlans(t, h, "is1b のディスク製品プランを検索する")
	require.Len(t, diskPlans, 1, "DiskPlan Find returned %d records, want one plan", len(diskPlans))
	require.Positive(t, diskPlans[0].ID, "disk plan has no positive ID")
	require.NotEmpty(t, diskPlans[0].Name, "disk plan has an empty name")
	require.NotEmpty(t, diskPlans[0].Size, "disk plan has no size data")

	// 検索したディスク製品プランを ID で参照して検索結果と一致させる
	diskPlanRead := readDiskPlan(t, h, "検索したディスク製品プランを ID で参照する", diskPlans[0].ID)
	require.Equal(t, diskPlans[0], diskPlanRead, "DiskPlan Read did not match the Find result for ID %d", diskPlans[0].ID)
}
