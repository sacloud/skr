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
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
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
	// findPageSize は Server・Disk の一括検索で使う1ページの件数です。
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
	// ArchiveID は is1b で利用可能な cloud-init アーカイブの ID です。
	ArchiveID int64
	// DiskPlanID は is1b で利用可能なディスクプランの ID です。
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
	// material は SSH 接続確認に使う一時的な鍵と cloud-init の user-data です。
	material helper.SSHMaterial
	// sshTimeout は SSH ログイン確認の期限です。ライブ実行では sshWaitTimeout を使います。
	sshTimeout time.Duration
	// serverID は作成に成功した Server の ID です。作成前は空です。
	serverID types.ID
}

// requireServerParams はライブ実行に必要な cloud-init アーカイブとディスクプランの設定を読み取ります。
// 環境変数は SKR_E2E_CLOUDIMG_ARCHIVE_ID と SKR_E2E_DISK_PLAN_ID、SKR_E2E_DISK_SIZE_GB (既定 20) です。
// アーカイブかプランが未設定の場合はテストをスキップします。
func requireServerParams(t *testing.T) serverDiskParams {
	t.Helper()

	params, err := parseServerParams(
		os.Getenv("SKR_E2E_CLOUDIMG_ARCHIVE_ID"),
		os.Getenv("SKR_E2E_DISK_PLAN_ID"),
		os.Getenv("SKR_E2E_DISK_SIZE_GB"),
	)
	if errors.Is(err, errMissingServerParams) {
		t.Skipf("%v, skipping E2E test", err)
	}
	require.NoError(t, err, "parse Server E2E parameters")
	return params
}

// errMissingServerParams は cloud-init アーカイブかディスクプランの環境変数が未設定であることを示します。
var errMissingServerParams = errors.New("SKR_E2E_CLOUDIMG_ARCHIVE_ID and SKR_E2E_DISK_PLAN_ID are required")

// parseServerParams は cloud-init アーカイブとディスクプランの設定値を解析します。
// アーカイブとプランは正の整数、ディスクサイズは正の整数 (既定 20) です。
func parseServerParams(archiveID, diskPlanID, sizeGB string) (serverDiskParams, error) {
	if archiveID == "" || diskPlanID == "" {
		return serverDiskParams{}, errMissingServerParams
	}
	archive, err := strconv.ParseInt(archiveID, 10, 64)
	if err != nil || archive <= 0 {
		return serverDiskParams{}, errors.New("SKR_E2E_CLOUDIMG_ARCHIVE_ID must be a positive integer")
	}
	plan, err := strconv.ParseInt(diskPlanID, 10, 64)
	if err != nil || plan <= 0 {
		return serverDiskParams{}, errors.New("SKR_E2E_DISK_PLAN_ID must be a positive integer")
	}
	size := int64(20)
	if sizeGB != "" {
		size, err = strconv.ParseInt(sizeGB, 10, 64)
		if err != nil || size <= 0 {
			return serverDiskParams{}, errors.New("SKR_E2E_DISK_SIZE_GB must be a positive integer")
		}
	}
	return serverDiskParams{ArchiveID: archive, DiskPlanID: plan, SizeGB: size}, nil
}

// findServerArgs は名前で Server を検索する引数を組み立てます。--name は部分一致検索です。
func findServerArgs(name string, from int) []string {
	return []string{
		"iaas-api", "server", "find",
		"--zone", zone, "--name", name,
		"--count", strconv.Itoa(findPageSize), "--from", strconv.Itoa(from),
		"--query", "map({ID,Name,Description})",
	}
}

// findDiskArgs は名前で Disk を検索する引数を組み立てます。--name は部分一致検索です。
func findDiskArgs(name string, from int) []string {
	return []string{
		"iaas-api", "disk", "find",
		"--zone", zone, "--name", name,
		"--count", strconv.Itoa(findPageSize), "--from", strconv.Itoa(from),
		"--query", "map({ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB})",
	}
}

// createServerArgs は cloud-init ディスクと共有 NIC を持つ Server 作成の引数を組み立てます。
// ディスクと NIC は ADR 0041 の反復可能オプションで指定します。
// user-data は起動時に渡すため、作成引数には含めません。
func createServerArgs(name string, params serverDiskParams) []string {
	return []string{
		"iaas-api", "server", "create",
		"--zone", zone, "--name", name, "--description", serverDescription,
		"--cpu", "1", "--memory-gb", "1",
		"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", params.ArchiveID, params.DiskPlanID, params.SizeGB),
		"--nic", "upstream=shared",
	}
}

// bootServerArgs は cloud-init の user-data を指定して Server を起動する引数を組み立てます。
// user-data は @path 形式でのみ渡し、内容を引数に含めません。
func bootServerArgs(id types.ID, userDataPath string) []string {
	return []string{
		"iaas-api", "server", "boot",
		"--zone", zone, "--id", id.String(),
		"--user-data", "@" + userDataPath,
	}
}

// readServerArgs は Server を参照する引数を組み立てます。
func readServerArgs(id types.ID) []string {
	return []string{
		"iaas-api", "server", "read", "--zone", zone, "--id", id.String(),
		"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}",
	}
}

// updateServerArgs は Server の名前と説明を更新する引数を組み立てます。
func updateServerArgs(id types.ID, name, desc string) []string {
	return []string{
		"iaas-api", "server", "update",
		"--zone", zone, "--id", id.String(),
		"--name", name, "--description", desc,
	}
}

// deleteServerArgs は Server をディスクとともに削除する引数を組み立てます。
// --with-disks で接続中の Disk も削除し、--force で電源を止めてから削除します。
func deleteServerArgs(id types.ID) []string {
	return []string{
		"iaas-api", "server", "delete",
		"--zone", zone, "--id", id.String(),
		"--with-disks", "--force", "--fail-if-not-found",
	}
}

// readDiskArgs は Disk を参照する引数を組み立てます。
func readDiskArgs(id types.ID) []string {
	return []string{
		"iaas-api", "disk", "read", "--zone", zone, "--id", id.String(),
		"--query", "{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB}",
	}
}

// deleteDiskArgs は孤立した Disk を削除する引数を組み立てます。
func deleteDiskArgs(id types.ID) []string {
	return []string{
		"iaas-api", "disk", "delete",
		"--zone", zone, "--id", id.String(),
		"--fail-if-not-found",
	}
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

// decodeDiskList は Disk の一括検索の応答を解析します。
func decodeDiskList(data []byte) ([]diskItem, error) {
	var items []diskItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode Disk list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of Disks")
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

// parseDisk は Disk の参照応答を解析します。ID が空の場合はエラーを返します。
func parseDisk(data []byte) (diskItem, error) {
	var item diskItem
	if err := json.Unmarshal(data, &item); err != nil {
		return diskItem{}, fmt.Errorf("decode Disk: %w", err)
	}
	if item.ID.IsEmpty() {
		return diskItem{}, errors.New("Disk ID is empty")
	}
	return item, nil
}

// checkServer は参照結果の ID・名前・説明が、今回の実行で作成した内容と一致することを確認します。
func checkServer(item serverItem, id types.ID, name string) error {
	if item.ID != id || item.Name != name || item.Description != serverDescription {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	return nil
}

// checkUpdatedServer は更新後の名前と説明が反映されていることを確認します。
func checkUpdatedServer(item serverItem, id types.ID, name string) error {
	if item.ID != id || item.Name != updatedServerName(name) || item.Description != updatedServerDescription {
		return fmt.Errorf("updated Server identity mismatch for ID %s (expected updated name and description)", id)
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

// TestParseServerParams は cloud-init アーカイブとディスクプランの設定値の解析を確認する。
func TestParseServerParams(t *testing.T) {
	params, err := parseServerParams("123", "4", "")
	require.NoError(t, err)
	require.Equal(t, serverDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 20}, params, "the disk size should default to 20GB")

	params, err = parseServerParams("123", "4", "40")
	require.NoError(t, err)
	require.Equal(t, serverDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 40}, params)

	for name, tc := range map[string]struct {
		archiveID, diskPlanID, sizeGB string
	}{
		"missing archive":   {archiveID: "", diskPlanID: "4"},
		"missing disk plan": {archiveID: "123", diskPlanID: ""},
		"invalid archive":   {archiveID: "abc", diskPlanID: "4"},
		"negative archive":  {archiveID: "-1", diskPlanID: "4"},
		"invalid disk plan": {archiveID: "123", diskPlanID: "abc"},
		"invalid disk size": {archiveID: "123", diskPlanID: "4", sizeGB: "abc"},
		"zero disk size":    {archiveID: "123", diskPlanID: "4", sizeGB: "0"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseServerParams(tc.archiveID, tc.diskPlanID, tc.sizeGB)
			require.Error(t, err, "invalid parameters should be rejected")
		})
	}
}

// TestFindArgsPaginateByName は名前検索の引数がページングと絞り込みを含むことを確認する。
func TestFindArgsPaginateByName(t *testing.T) {
	require.Equal(t, []string{
		"iaas-api", "server", "find",
		"--zone", "is1b", "--name", "skr-e2e-server-test",
		"--count", "100", "--from", "0",
		"--query", "map({ID,Name,Description})",
	}, findServerArgs("skr-e2e-server-test", 0), "the server find should limit fields with --query")

	require.Equal(t, []string{
		"iaas-api", "disk", "find",
		"--zone", "is1b", "--name", "skr-e2e-server-test",
		"--count", "100", "--from", "100",
		"--query", "map({ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB})",
	}, findDiskArgs("skr-e2e-server-test", 100), "the disk find should request the second page")
}

// TestCreateServerArgsUseDedicatedOptions は Server 作成の引数が ADR 0041 の
// 反復可能オプションと共有 NIC を使うことを確認する。
func TestCreateServerArgsUseDedicatedOptions(t *testing.T) {
	args := createServerArgs("skr-e2e-server-test", serverDiskParams{ArchiveID: 123, DiskPlanID: 4, SizeGB: 20})

	require.Equal(t, []string{
		"iaas-api", "server", "create",
		"--zone", "is1b", "--name", "skr-e2e-server-test", "--description", serverDescription,
		"--cpu", "1", "--memory-gb", "1",
		"--disk", "archive-id=123,plan-id=4,size-gb=20",
		"--nic", "upstream=shared",
	}, args)
}

// TestBootServerArgsOnlyReferenceUserDataPath は起動の引数が user-data を
// @path 形式でのみ参照し、内容を含まないことを確認する。
func TestBootServerArgsOnlyReferenceUserDataPath(t *testing.T) {
	args := bootServerArgs(123, "/tmp/user-data.yaml")

	require.Equal(t, []string{
		"iaas-api", "server", "boot",
		"--zone", "is1b", "--id", "123",
		"--user-data", "@/tmp/user-data.yaml",
	}, args)
	require.NotContains(t, strings.Join(args, " "), "ssh_authorized_keys", "the boot args should not contain the user-data content")
}

// TestServerMutationArgsMatchAPIOperations は更新と削除の引数を確認する。
func TestServerMutationArgsMatchAPIOperations(t *testing.T) {
	require.Equal(t, []string{
		"iaas-api", "server", "update",
		"--zone", "is1b", "--id", "123",
		"--name", "skr-e2e-server-test-upd", "--description", "updated description",
	}, updateServerArgs(123, "skr-e2e-server-test-upd", "updated description"))

	require.Equal(t, []string{
		"iaas-api", "server", "delete",
		"--zone", "is1b", "--id", "123",
		"--with-disks", "--force", "--fail-if-not-found",
	}, deleteServerArgs(123), "the server delete should also delete connected Disks")

	require.Equal(t, []string{
		"iaas-api", "disk", "delete",
		"--zone", "is1b", "--id", "45",
		"--fail-if-not-found",
	}, deleteDiskArgs(45))
}

// TestCheckServerIdentity は参照結果の識別確認を確認する。
func TestCheckServerIdentity(t *testing.T) {
	item := serverItem{ID: 123, Name: "skr-e2e-server-test", Description: serverDescription}
	require.NoError(t, checkServer(item, 123, "skr-e2e-server-test"))

	for name, tc := range map[string]serverItem{
		"identity":    {ID: 456, Name: "skr-e2e-server-test", Description: serverDescription},
		"name":        {ID: 123, Name: "changed", Description: serverDescription},
		"description": {ID: 123, Name: "skr-e2e-server-test", Description: "changed"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, checkServer(tc, 123, "skr-e2e-server-test"), "mismatched resources should be refused")
		})
	}
}

// TestCheckUpdatedServerIdentity は更新後の識別確認を確認する。
func TestCheckUpdatedServerIdentity(t *testing.T) {
	item := serverItem{ID: 123, Name: "skr-e2e-server-test-upd", Description: updatedServerDescription}
	require.NoError(t, checkUpdatedServer(item, 123, "skr-e2e-server-test"))

	stale := serverItem{ID: 123, Name: "skr-e2e-server-test", Description: serverDescription}
	require.Error(t, checkUpdatedServer(stale, 123, "skr-e2e-server-test"), "unapplied updates should be refused")
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

// TestDecodeListResponses は一括検索の応答の解析を確認する。
func TestDecodeListResponses(t *testing.T) {
	servers, err := decodeServerList([]byte(`[{"ID":123,"Name":"skr-e2e-server-test","Description":"d"}]`))
	require.NoError(t, err)
	require.Equal(t, []serverItem{{ID: 123, Name: "skr-e2e-server-test", Description: "d"}}, servers)

	disks, err := decodeDiskList([]byte(`[{"ID":45,"Name":"skr-e2e-server-test","SizeMB":20480}]`))
	require.NoError(t, err)
	require.Equal(t, []diskItem{{ID: 45, Name: "skr-e2e-server-test", SizeMB: 20480}}, disks)

	_, err = decodeServerList([]byte(`null`))
	require.Error(t, err, "a null response should be refused")
	_, err = decodeServerList([]byte(`{"ID":123}`))
	require.Error(t, err, "a non-array response should be refused")
	_, err = decodeDiskList([]byte(`null`))
	require.Error(t, err, "a null response should be refused")
}

// TestParseResponses は参照応答の解析を確認する。
func TestParseResponses(t *testing.T) {
	server, err := parseServer([]byte(`{"ID":123,"Name":"skr-e2e-server-test"}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(123), server.ID)

	_, err = parseServer([]byte(`{}`))
	require.Error(t, err, "an empty Server ID should be refused")
	_, err = parseDisk([]byte(`{}`))
	require.Error(t, err, "an empty Disk ID should be refused")
}
