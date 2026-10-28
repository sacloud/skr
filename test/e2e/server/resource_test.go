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
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// zone は Server E2E で使用するゾーンです。
	zone = "is1b"
	// namePrefix はテスト用 Server と Disk の名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	namePrefix = "skr-e2e-server-"
	// serverDescription はテスト用 Server に付ける固定の説明です。後片付けの識別にも使います。
	serverDescription = "Temporary Server for skr cloud-init E2E"
	// sshUser は cloud-init で登録する SSH ログイン用ユーザーです。
	sshUser = "skr-e2e"
	// sshWaitTimeout は SSH ログイン確認の期限です。cloud-init の反映を待つため長めに取ります。
	sshWaitTimeout = 20 * time.Minute
	// sshPolling は SSH ログインの再確認間隔です。
	sshPolling = 10 * time.Second
	// findPageSize は SDK の一括検索で使う1ページの件数です。
	findPageSize = 100
)

// serverInterface は Server の NIC の参照結果です。
type serverInterface struct {
	IPAddress    string                     `json:"IPAddress"`
	UpstreamType types.EUpstreamNetworkType `json:"UpstreamType"`
}

// serverItem は Server の参照結果です。
type serverItem struct {
	ID             types.ID                    `json:"ID"`
	Name           string                      `json:"Name"`
	Description    string                      `json:"Description"`
	InstanceStatus types.EServerInstanceStatus `json:"InstanceStatus"`
	Interfaces     []serverInterface           `json:"Interfaces"`
	Disks          []diskItem                  `json:"Disks"`
}

// diskItem は Disk の参照結果です。
type diskItem struct {
	ID              types.ID `json:"ID"`
	Name            string   `json:"Name"`
	ServerID        types.ID `json:"ServerID"`
	DiskPlanID      types.ID `json:"DiskPlanID"`
	SourceArchiveID types.ID `json:"SourceArchiveID"`
	SizeMB          int64    `json:"SizeMB"`
}

// serverDiskParams は cloud-init ディスクの作成に使うアーカイブとディスクプランの設定です。
type serverDiskParams struct {
	// ArchiveID は自動選択した Ubuntu Server 24.04 LTS (cloudimg) アーカイブの ID です。
	ArchiveID int64
	// DiskPlanID は自動選択したディスクプランの ID です。
	DiskPlanID int64
	// SizeGB はクラウドディスクのサイズ (GB) です。
	SizeGB int64
}

// serverRun は1回の Server E2E 実行の状態と設定です。
type serverRun struct {
	// name は今回の実行が作成する一意な Server 名です。Disk も同名で作成します。
	name string
	// params は cloud-init アーカイブとディスクプランの設定です。
	params serverDiskParams
	// serverID は作成に成功した Server の ID です。作成前は空です。
	serverID types.ID
}

// requireServerParams はディスクサイズの環境変数を読み取り、
// cloud-init アーカイブとディスクプランを SDK で自動選択します。
// 環境変数は SKR_E2E_DISK_SIZE_GB (既定 20) です。
// アーカイブとディスクプランの選択は E2E の事前確認のため SDK で静かに実行します。
func requireServerParams(t *testing.T, caller iaas.APICaller) serverDiskParams {
	t.Helper()

	sizeGB, err := parseDiskSizeGB(os.Getenv("SKR_E2E_DISK_SIZE_GB"))
	require.NoError(t, err, "parse SKR_E2E_DISK_SIZE_GB")
	archive := helper.FindCloudimgArchive(t, caller, zone)
	plan := helper.FindDiskPlan(t, caller, zone, sizeGB)
	return serverDiskParams{
		ArchiveID:  int64(archive.ID),
		DiskPlanID: int64(plan.ID),
		SizeGB:     sizeGB,
	}
}

// parseDiskSizeGB はディスクサイズ (GB) の環境変数の値を解析します。未設定なら既定の 20 を返します。
func parseDiskSizeGB(sizeGB string) (int64, error) {
	if sizeGB == "" {
		return 20, nil
	}
	size, err := strconv.ParseInt(sizeGB, 10, 64)
	if err != nil || size <= 0 {
		return 0, errors.New("SKR_E2E_DISK_SIZE_GB must be a positive integer")
	}
	return size, nil
}

// decodeServerList は Server の一括検索の応答を解析します。
func decodeServerList(data []byte) ([]serverItem, error) {
	var items []serverItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode Server list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of Servers")
	}
	return items, nil
}

// parseServer は Server の参照応答を解析します。ID が空の場合はエラーを返します。
func parseServer(data []byte) (serverItem, error) {
	var item serverItem
	if err := json.Unmarshal(data, &item); err != nil {
		return serverItem{}, fmt.Errorf("decode Server: %w", err)
	}
	if item.ID.IsEmpty() {
		return serverItem{}, errors.New("Server ID is empty")
	}
	return item, nil
}

// serverFromSDK は SDK の Server を参照結果の型へ変換します。
// 接続中 Disk の情報は Server の応答に含まれる項目へ写します。
func serverFromSDK(s *iaas.Server) serverItem {
	item := serverItem{
		ID:             s.ID,
		Name:           s.Name,
		Description:    s.Description,
		InstanceStatus: s.InstanceStatus,
	}
	for _, iface := range s.Interfaces {
		if iface == nil {
			continue
		}
		item.Interfaces = append(item.Interfaces, serverInterface{
			IPAddress:    iface.IPAddress,
			UpstreamType: iface.UpstreamType,
		})
	}
	for _, disk := range s.Disks {
		if disk == nil {
			continue
		}
		item.Disks = append(item.Disks, connectedDiskFromSDK(disk))
	}
	return item
}

// connectedDiskFromSDK は SDK の接続中 Disk 情報を Disk の参照結果の型へ変換します。
func connectedDiskFromSDK(d *iaas.ServerConnectedDisk) diskItem {
	return diskItem{
		ID:         d.ID,
		Name:       d.Name,
		DiskPlanID: d.DiskPlanID,
		SizeMB:     int64(d.SizeMB),
	}
}

// diskFromSDK は SDK の Disk を Disk の参照結果の型へ変換します。
func diskFromSDK(d *iaas.Disk) diskItem {
	return diskItem{
		ID:              d.ID,
		Name:            d.Name,
		ServerID:        d.ServerID,
		DiskPlanID:      d.DiskPlanID,
		SourceArchiveID: d.SourceArchiveID,
		SizeMB:          int64(d.SizeMB),
	}
}

// checkServerIdentity は参照結果が期待する Server (ID・名前・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func checkServerIdentity(item serverItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// matchesCreatedDisk は Disk が、今回の実行で作成したクラウドディスクと一致することを確認します。
// サイズは MB 単位の応答を GB に換算して照合し、Server に接続されていないことも確認します。
func matchesCreatedDisk(disk diskItem, id types.ID, name string, params serverDiskParams) bool {
	return disk.ID == id && disk.Name == name && disk.ServerID.IsEmpty() &&
		disk.DiskPlanID == types.ID(params.DiskPlanID) && disk.SourceArchiveID == types.ID(params.ArchiveID) &&
		params.SizeGB > 0 && disk.SizeMB/1024 == params.SizeGB && disk.SizeMB%1024 == 0
}

// TestParseDiskSizeGB はディスクサイズの環境変数の値の解析を確認する。
func TestParseDiskSizeGB(t *testing.T) {
	size, err := parseDiskSizeGB("")
	require.NoError(t, err)
	require.EqualValues(t, 20, size, "the disk size should default to 20GB")

	size, err = parseDiskSizeGB("40")
	require.NoError(t, err)
	require.EqualValues(t, 40, size)

	for name, tc := range map[string]string{
		"not an integer": "abc",
		"zero":           "0",
		"negative":       "-20",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseDiskSizeGB(tc)
			require.Error(t, err, "invalid disk size should be rejected")
		})
	}
}

// TestCheckServerIdentity は参照結果の識別確認を確認する。
func TestCheckServerIdentity(t *testing.T) {
	item := serverItem{ID: 123, Name: "skr-e2e-server-test", Description: serverDescription}
	updated := serverItem{ID: 123, Name: "skr-e2e-server-test-upd", Description: updatedServerDescription}

	// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認する
	require.NoError(t, checkServerIdentity(item, 123, item.Name, item.Description))
	require.NoError(t, checkServerIdentity(updated, 123, updated.Name, updated.Description))

	// ID・名前・説明のどれかが違えば拒否する
	require.Error(t, checkServerIdentity(serverItem{ID: 456, Name: item.Name, Description: item.Description}, 123, item.Name, item.Description), "mismatched resources should be refused")
	require.Error(t, checkServerIdentity(serverItem{ID: item.ID, Name: "changed", Description: item.Description}, 123, item.Name, item.Description), "mismatched resources should be refused")
	require.Error(t, checkServerIdentity(serverItem{ID: item.ID, Name: item.Name, Description: "changed"}, 123, item.Name, item.Description), "mismatched resources should be refused")

	// 更新が反映されていない古い内容は、更新後の期待値と一致しない
	require.Error(t, checkServerIdentity(item, 123, updated.Name, updated.Description), "unapplied updates should be refused")
}

// TestMatchesCreatedDisk は孤立ディスクが今回作成した内容と一致することの確認を確かめる。
func TestMatchesCreatedDisk(t *testing.T) {
	params := serverDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 20}
	disk := diskItem{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SourceArchiveID: 123, SizeMB: 20 * 1024}
	require.True(t, matchesCreatedDisk(disk, 45, "skr-e2e-server-test", params))

	for name, tc := range map[string]diskItem{
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

// TestDecodeServerList は Server の一括検索の応答の解析を確認する。
func TestDecodeServerList(t *testing.T) {
	servers, err := decodeServerList([]byte(`[{"ID":123,"Name":"skr-e2e-server-test","Description":"d"}]`))
	require.NoError(t, err)
	require.Equal(t, []serverItem{{ID: 123, Name: "skr-e2e-server-test", Description: "d"}}, servers)

	_, err = decodeServerList([]byte(`null`))
	require.Error(t, err, "a null response should be refused")
	_, err = decodeServerList([]byte(`{"ID":123}`))
	require.Error(t, err, "a non-array response should be refused")
}

// TestParseServerResponse は Server の参照応答の解析を確認する。
func TestParseServerResponse(t *testing.T) {
	server, err := parseServer([]byte(`{"ID":123,"Name":"skr-e2e-server-test"}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(123), server.ID)

	_, err = parseServer([]byte(`{}`))
	require.Error(t, err, "an empty Server ID should be refused")
}

// TestServerFromSDK は SDK の Server から参照結果の型への変換を確認する。
func TestServerFromSDK(t *testing.T) {
	sdk := &iaas.Server{
		ID:             123,
		Name:           "skr-e2e-server-test",
		Description:    serverDescription,
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

	require.Equal(t, serverItem{
		ID:             123,
		Name:           "skr-e2e-server-test",
		Description:    serverDescription,
		InstanceStatus: types.ServerInstanceStatuses.Up,
		Interfaces:     []serverInterface{{IPAddress: "192.0.2.10", UpstreamType: types.UpstreamNetworkTypes.Shared}},
		Disks:          []diskItem{{ID: 45, Name: "skr-e2e-server-test", DiskPlanID: 4, SizeMB: 20480}},
	}, serverFromSDK(sdk), "the conversion should keep the verified fields and skip nil elements")
}

// TestDiskFromSDK は SDK の Disk から参照結果の型への変換を確認する。
func TestDiskFromSDK(t *testing.T) {
	sdk := &iaas.Disk{ID: 45, Name: "skr-e2e-server-test", ServerID: 123, DiskPlanID: 4, SourceArchiveID: 999, SizeMB: 20480}

	require.Equal(t, diskItem{
		ID:              45,
		Name:            "skr-e2e-server-test",
		ServerID:        123,
		DiskPlanID:      4,
		SourceArchiveID: 999,
		SizeMB:          20480,
	}, diskFromSDK(sdk))
}
