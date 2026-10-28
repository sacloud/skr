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
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// TestParseDiskSizeGB はディスクサイズの環境変数の値の解析を確認する。
func TestParseDiskSizeGB(t *testing.T) {
	size, err := ParseDiskSizeGB("")
	require.NoError(t, err)
	require.EqualValues(t, 20, size, "the disk size should default to 20GB")

	size, err = ParseDiskSizeGB("40")
	require.NoError(t, err)
	require.EqualValues(t, 40, size)

	for name, tc := range map[string]string{
		"not an integer": "abc",
		"zero":           "0",
		"negative":       "-20",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDiskSizeGB(tc)
			require.Error(t, err, "invalid disk size should be rejected")
		})
	}
}

// TestCheckServerIdentity は参照結果の識別確認を確認する。
func TestCheckServerIdentity(t *testing.T) {
	description := "Temporary Server for skr cloud-init E2E"
	updatedDescription := description + " (updated)"
	item := ServerItem{ID: 123, Name: "skr-e2e-server-test", Description: description}
	updated := ServerItem{ID: 123, Name: "skr-e2e-server-test-upd", Description: updatedDescription}

	// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認する
	require.NoError(t, CheckServerIdentity(item, 123, item.Name, item.Description))
	require.NoError(t, CheckServerIdentity(updated, 123, updated.Name, updated.Description))

	// ID・名前・説明のどれかが違えば拒否する
	require.Error(t, CheckServerIdentity(ServerItem{ID: 456, Name: item.Name, Description: item.Description}, 123, item.Name, item.Description), "mismatched resources should be refused")
	require.Error(t, CheckServerIdentity(ServerItem{ID: item.ID, Name: "changed", Description: item.Description}, 123, item.Name, item.Description), "mismatched resources should be refused")
	require.Error(t, CheckServerIdentity(ServerItem{ID: item.ID, Name: item.Name, Description: "changed"}, 123, item.Name, item.Description), "mismatched resources should be refused")

	// 更新が反映されていない古い内容は、更新後の期待値と一致しない
	require.Error(t, CheckServerIdentity(item, 123, updated.Name, updated.Description), "unapplied updates should be refused")
}

// TestMatchesCreatedDisk は孤立ディスクが今回作成した内容と一致することの確認を確かめる。
func TestMatchesCreatedDisk(t *testing.T) {
	params := ServerDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 20}
	disk := DiskItem{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024}
	require.True(t, matchesCreatedDisk(disk, 45, "skr-e2e-server-test", params))

	for name, tc := range map[string]DiskItem{
		"attached":      {ID: 45, Name: "skr-e2e-server-test", ServerID: 123, DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024},
		"other plan":    {ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 2, SourceArchiveID: 123, SizeMB: 20 * 1024},
		"other archive": {ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 999, SizeMB: 20 * 1024},
		"other size":    {ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 40 * 1024},
		"other name":    {ID: 45, Name: "changed", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, matchesCreatedDisk(tc, 45, "skr-e2e-server-test", params), "mismatched Disks should not be identified")
		})
	}
}

// TestMatchesDiskSettings は作成した Disk の指定内容の照合を確かめる。接続状態は含まない。
func TestMatchesDiskSettings(t *testing.T) {
	params := ServerDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 20}
	disk := DiskItem{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024}

	// 作成直後の接続中の Disk も、指定内容が一致していれば受け付ける
	require.True(t, MatchesDiskSettings(disk, "skr-e2e-server-test", params))
	require.True(t, MatchesDiskSettings(DiskItem{ServerID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024}, "skr-e2e-server-test", params))

	for name, tc := range map[string]DiskItem{
		"other name":    {Name: "changed", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024},
		"other plan":    {Name: "skr-e2e-server-test", DiskPlanID: 2, SourceArchiveID: 123, SizeMB: 20 * 1024},
		"other archive": {Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 999, SizeMB: 20 * 1024},
		"other size":    {Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 40 * 1024},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, MatchesDiskSettings(tc, "skr-e2e-server-test", params), "mismatched Disks should not be identified")
		})
	}
}

// TestDecodeServerList は Server の一括検索の応答の解析を確認する。
func TestDecodeServerList(t *testing.T) {
	servers, err := DecodeServerList([]byte(`[{"ID":123,"Name":"skr-e2e-server-test","Description":"d"}]`))
	require.NoError(t, err)
	require.Equal(t, []ServerItem{{ID: 123, Name: "skr-e2e-server-test", Description: "d"}}, servers)

	_, err = DecodeServerList([]byte(`null`))
	require.Error(t, err, "a null response should be refused")
	_, err = DecodeServerList([]byte(`{"ID":123}`))
	require.Error(t, err, "a non-array response should be refused")
}

// TestParseServerOutput は Server の参照応答の解析を確認する。
func TestParseServerOutput(t *testing.T) {
	server, err := ParseServerOutput([]byte(`{"ID":123,"Name":"skr-e2e-server-test"}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(123), server.ID)

	_, err = ParseServerOutput([]byte(`{}`))
	require.Error(t, err, "an empty Server ID should be refused")
}

// TestServerFromSDK は SDK の Server から参照結果の型への変換を確認する。
func TestServerFromSDK(t *testing.T) {
	description := "Temporary Server for skr cloud-init E2E"
	sdk := &iaas.Server{
		ID:             123,
		Name:           "skr-e2e-server-test",
		Description:    description,
		InstanceStatus: types.ServerInstanceStatuses.Up,
		Interfaces: []*iaas.InterfaceView{
			{ID: 200, IPAddress: "192.0.2.10", UpstreamType: types.UpstreamNetworkTypes.Shared},
			nil,
		},
		Disks: []*iaas.ServerConnectedDisk{
			{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SizeMB: 20480},
			nil,
		},
	}

	require.Equal(t, ServerItem{
		ID:             123,
		Name:           "skr-e2e-server-test",
		Description:    description,
		InstanceStatus: types.ServerInstanceStatuses.Up,
		Interfaces:     []ServerInterface{{IPAddress: "192.0.2.10", UpstreamType: types.UpstreamNetworkTypes.Shared}},
		Disks:          []DiskItem{{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SizeMB: 20480}},
	}, serverFromSDK(sdk), "the conversion should keep the verified fields and skip nil elements")
}

// TestDiskFromSDK は SDK の Disk から参照結果の型への変換を確認する。
func TestDiskFromSDK(t *testing.T) {
	sdk := &iaas.Disk{ID: 45, Name: "skr-e2e-server-test", ServerID: 123, DiskPlanID: 4, SourceArchiveID: 999, SizeMB: 20480}

	require.Equal(t, DiskItem{
		ID:              45,
		Name:            "skr-e2e-server-test",
		ServerID:        123,
		DiskPlanID:      4,
		SourceArchiveID: 999,
		SizeMB:          20480,
	}, diskFromSDK(sdk))
}

// TestFindReadsAllPages は一括検索が全ページを取得することを確認する。
func TestFindReadsAllPages(t *testing.T) {
	calls := 0
	items, err := paginateFind(t, func(ctx context.Context, from int) ([]DiskItem, error) {
		calls++
		if from != (calls-1)*findPageSize {
			return nil, fmt.Errorf("unexpected from %d", from)
		}
		if calls == 2 {
			return nil, nil
		}
		return make([]DiskItem, findPageSize), nil
	})

	require.NoError(t, err)
	require.Len(t, items, findPageSize, "the first page should be collected")
	require.Equal(t, 2, calls, "the second page should be requested")
}

// TestFindDoesNotTreatPageFailureAsMissing は途中ページの取得に失敗した場合に
// 不在として扱わないことを確認する。
func TestFindDoesNotTreatPageFailureAsMissing(t *testing.T) {
	calls := 0
	_, err := paginateFind(t, func(context.Context, int) ([]DiskItem, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("injected failure")
		}
		return make([]DiskItem, findPageSize), nil
	})

	require.ErrorContains(t, err, "injected failure", "a page failure should not be treated as no results")
}
