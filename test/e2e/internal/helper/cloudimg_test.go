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

package helper

import (
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// TestIsUbuntu2404Cloudimg は cloudimg Archive 名の判定を確認する。
func TestIsUbuntu2404Cloudimg(t *testing.T) {
	tests := map[string]struct {
		name string
		want bool
	}{
		"Ubuntu 24.04 cloudimg":         {name: "Ubuntu Server 24.04.2 LTS 64bit (cloudimg)", want: true},
		"Ubuntu 24.04.1 cloudimg":       {name: "Ubuntu Server 24.04.1 LTS 64bit (cloudimg)", want: true},
		"other Ubuntu release":          {name: "Ubuntu Server 22.04.5 LTS 64bit (cloudimg)", want: false},
		"Ubuntu 26.04":                  {name: "Ubuntu Server 26.04 LTS 64bit (cloudimg)", want: false},
		"not cloudimg":                  {name: "Ubuntu Server 24.04.2 LTS 64bit", want: false},
		"other OS":                      {name: "Debian GNU/Linux 12.7.0 64bit (cloudimg)", want: false},
		"Ubuntu only in description":    {name: "CData Sync v25.1(Ubuntu 24.04.2 LTS 64bit)", want: false},
		"cloudimg without Ubuntu 24.04": {name: "Debian GNU/Linux 13.5.0 64bit (cloudimg)", want: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, IsUbuntu2404Cloudimg(tc.name))
		})
	}
}

// TestLatestCloudimgArchive は Ubuntu Server 24.04 LTS (cloudimg) Archive の選択を確認する。
func TestLatestCloudimgArchive(t *testing.T) {
	archives := []*iaas.Archive{
		{ID: 113601947034, Name: "Ubuntu Server 24.04.1 LTS 64bit (cloudimg)"},
		{ID: 113701786718, Name: "Ubuntu Server 24.04.2 LTS 64bit (cloudimg)"},
		{ID: 113601947037, Name: "Ubuntu Server 22.04.5 LTS 64bit (cloudimg)"},
		{ID: 113801341549, Name: "Ubuntu Server 26.04 LTS 64bit (cloudimg)"},
	}
	latest := LatestCloudimgArchive(archives)
	require.NotNil(t, latest)
	require.Equal(t, types.ID(113701786718), latest.ID,
		"the latest Ubuntu Server 24.04 cloudimg Archive should be selected")

	require.Nil(t, LatestCloudimgArchive([]*iaas.Archive{
		{ID: 1, Name: "Debian GNU/Linux 12.7.0 64bit (cloudimg)"},
	}), "a zone without Ubuntu Server 24.04 cloudimg should have no selection")
}

// TestAvailableDiskPlan は指定サイズが利用可能なディスクプランの選択を確認する。
func TestAvailableDiskPlan(t *testing.T) {
	available := func(sizeMB int) *iaas.DiskPlanSizeInfo {
		return &iaas.DiskPlanSizeInfo{SizeMB: sizeMB, Availability: types.Availabilities.Available}
	}
	plans := []*iaas.DiskPlan{
		{ID: 2, Name: "標準プラン", Availability: types.Availabilities.Available,
			Size: []*iaas.DiskPlanSizeInfo{available(40960)}},
		{ID: 4, Name: "SSDプラン", Availability: types.Availabilities.Available,
			Size: []*iaas.DiskPlanSizeInfo{available(20480), available(40960)}},
		{ID: 6, Name: "引退プラン", Availability: types.Availabilities.Discontinued,
			Size: []*iaas.DiskPlanSizeInfo{available(20480)}},
	}
	plan := AvailableDiskPlan(plans, 20)
	require.NotNil(t, plan)
	require.Equal(t, types.ID(4), plan.ID,
		"the available plan supporting the requested size should be selected")

	require.NotNil(t, AvailableDiskPlan(plans, 40), "both plans support 40GB")
	require.Nil(t, AvailableDiskPlan(plans, 100),
		"a size no plan supports should have no selection")

	// 利用可能なサイズが stopped のプランは選ばない
	stoppedPlan := &iaas.DiskPlan{ID: 8, Name: "停止サイズプラン",
		Availability: types.Availabilities.Available,
		Size: []*iaas.DiskPlanSizeInfo{
			{SizeMB: 20480, Availability: types.Availabilities.Failed},
		}}
	require.Nil(t, AvailableDiskPlan([]*iaas.DiskPlan{stoppedPlan}, 20),
		"a plan whose size is not available should not be selected")
}

// TestDiskPlanHasSize はディスクプランのサイズ確認を確認する。
func TestDiskPlanHasSize(t *testing.T) {
	plan := &iaas.DiskPlan{ID: 4, Availability: types.Availabilities.Available,
		Size: []*iaas.DiskPlanSizeInfo{
			{SizeMB: 20480, Availability: types.Availabilities.Available},
			{SizeMB: 40960, Availability: types.Availabilities.Available},
		}}
	require.True(t, diskPlanHasSize(plan, 20))
	require.True(t, diskPlanHasSize(plan, 40))
	require.False(t, diskPlanHasSize(plan, 100))
}
