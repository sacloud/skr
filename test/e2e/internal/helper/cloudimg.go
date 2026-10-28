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
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	archiveSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
	diskplanSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
	"github.com/stretchr/testify/require"
)

// cloudimgArchiveFilter は cloudimg Archive を検索する部分一致の名前条件です。
var cloudimgArchiveFilter = "cloudimg"

// FindCloudimgArchive は指定ゾーンで Ubuntu Server 24.04 LTS (cloudimg) Archive を検索し、
// 最新のもの (ID が最大のもの) を返します。対応する Archive がない場合はテストを失敗させます。
// 検索は E2E の事前確認のため SDK で静かに実行し、実行記録には残しません。
func FindCloudimgArchive(t *testing.T, caller iaas.APICaller, zoneName string) *iaas.Archive {
	t.Helper()

	archives, err := archiveSDK.New(caller).FindWithContext(t.Context(), &archiveSDK.FindRequest{
		Zone:  zoneName,
		Names: []string{cloudimgArchiveFilter},
	})
	require.NoError(t, err, "find the cloudimg Archive in zone %s", zoneName)
	latest := LatestCloudimgArchive(archives)
	require.NotNil(t, latest,
		"zone %s has no Ubuntu Server 24.04 LTS (cloudimg) Archive", zoneName)
	return latest
}

// LatestCloudimgArchive は Ubuntu Server 24.04 LTS (cloudimg) Archive のうち
// 最新のもの (ID が最大のもの) を返します。対応する Archive がない場合は nil を返します。
func LatestCloudimgArchive(archives []*iaas.Archive) *iaas.Archive {
	var latest *iaas.Archive
	for _, archive := range archives {
		if !IsUbuntu2404Cloudimg(archive.Name) {
			continue
		}
		if latest == nil || archive.ID > latest.ID {
			latest = archive
		}
	}
	return latest
}

// IsUbuntu2404Cloudimg は Archive 名が Ubuntu Server 24.04 LTS の (cloudimg) かどうかを返します。
// E2E は ubuntu ユーザーと cloud-init の netplan 設定を使うため、他の OS やリリースは受け付けません。
func IsUbuntu2404Cloudimg(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "ubuntu server 24.04") && strings.Contains(lower, "(cloudimg)")
}

// FindDiskPlan は指定ゾーンで指定サイズ (GB) が利用可能なディスクプランを検索して返します。
// 複数のプランが対応する場合は ID が最小のものを安定して選びます。
// 対応するプランがない場合はテストを失敗させます。
func FindDiskPlan(t *testing.T, caller iaas.APICaller, zoneName string, sizeGB int64) *iaas.DiskPlan {
	t.Helper()

	plans, err := diskplanSDK.New(caller).FindWithContext(t.Context(),
		&diskplanSDK.FindRequest{Zone: zoneName})
	require.NoError(t, err, "find the disk plan in zone %s", zoneName)
	plan := AvailableDiskPlan(plans, sizeGB)
	require.NotNil(t, plan,
		"zone %s has no available disk plan for %d GB", zoneName, sizeGB)
	return plan
}

// AvailableDiskPlan は指定サイズ (GB) が利用可能なディスクプランのうち
// ID が最小のものを返します。対応するプランがない場合は nil を返します。
func AvailableDiskPlan(plans []*iaas.DiskPlan, sizeGB int64) *iaas.DiskPlan {
	var found *iaas.DiskPlan
	for _, plan := range plans {
		if plan.Availability != types.Availabilities.Available || !diskPlanHasSize(plan, sizeGB) {
			continue
		}
		if found == nil || plan.ID < found.ID {
			found = plan
		}
	}
	return found
}

// diskPlanHasSize はディスクプランが指定サイズ (GB) を利用可能なサイズとして持つかどうかを返します。
func diskPlanHasSize(plan *iaas.DiskPlan, sizeGB int64) bool {
	// GB を MB に換算して照合します。int64 のまま扱い、解析済みの値を狭める変換を行いません
	sizeMB := sizeGB * 1024
	for _, size := range plan.Size {
		if int64(size.SizeMB) == sizeMB && size.Availability == types.Availabilities.Available {
			return true
		}
	}
	return false
}
