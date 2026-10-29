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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/stretchr/testify/require"
)

const (
	// ServerZone は Server を扱う E2E で使用するゾーンです。GSLB 自体はゾーンを持ちませんが、
	// ヘルスチェック用の Server はこのゾーンに作成します。
	ServerZone = "is1b"
	// ServerNamePrefix はテスト用 Server と Disk の名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	ServerNamePrefix = "skr-e2e-server-"
	// ServerDescription はテスト用 Server に付ける固定の説明です。後片付けの識別にも使います。
	ServerDescription = "Temporary Server for skr cloud-init E2E"
	// ServerSSHUser は cloud-init で登録する SSH ログイン用ユーザーです。
	ServerSSHUser = "skr-e2e"
	// ServerSSHWaitTimeout は SSH ログイン確認と起動・停止待ちの期限です。
	// cloud-init の反映を待つため長めに取ります。
	ServerSSHWaitTimeout = 20 * time.Minute
	// ServerSSHPolling は SSH ログインと起動・停止の再確認間隔です。
	ServerSSHPolling = 10 * time.Second
	// BootAcceptWaitTimeout は起動 API のエラー後に起動の受理を確認する期限です。
	BootAcceptWaitTimeout = 3 * time.Minute
	// findPageSize は SDK の一括検索で使う1ページの件数です。
	findPageSize = 100
)

// ServerInterface は Server の NIC の参照結果です。
type ServerInterface struct {
	IPAddress    string                     `json:"IPAddress"`
	UpstreamType types.EUpstreamNetworkType `json:"UpstreamType"`
}

// ServerItem は Server の参照結果です。
type ServerItem struct {
	ID             types.ID                    `json:"ID"`
	Name           string                      `json:"Name"`
	Description    string                      `json:"Description"`
	InstanceStatus types.EServerInstanceStatus `json:"InstanceStatus"`
	Interfaces     []ServerInterface           `json:"Interfaces"`
	Disks          []DiskItem                  `json:"Disks"`
}

// DiskItem は Disk の参照結果です。
type DiskItem struct {
	ID              types.ID `json:"ID"`
	Name            string   `json:"Name"`
	ServerID        types.ID `json:"ServerID"`
	DiskPlanID      types.ID `json:"DiskPlanID"`
	SourceArchiveID types.ID `json:"SourceArchiveID"`
	SizeMB          int64    `json:"SizeMB"`
}

// ServerDiskParams は cloud-init ディスクの作成に使うアーカイブとディスクプランの設定です。
type ServerDiskParams struct {
	// ArchiveID は自動選択した Ubuntu Server 24.04 LTS (cloudimg) アーカイブの ID です。
	ArchiveID int64
	// DiskPlanID は自動選択したディスクプランの ID です。
	DiskPlanID int64
	// SizeGB はクラウドディスクのサイズ (GB) です。
	SizeGB int64
}

// ServerIdentity は後片付けの再照合で許容する Server の識別情報です。
// 更新確認で名前と説明を変える場合は、更新後の識別情報も許容する値として含めます。
type ServerIdentity struct {
	// Name は許容する Server 名です。
	Name string
	// Description は許容する Server の説明です。
	Description string
}

// ParseDiskSizeGB はディスクサイズ (GB) の環境変数の値を解析します。未設定なら既定の 20 を返します。
func ParseDiskSizeGB(sizeGB string) (int64, error) {
	if sizeGB == "" {
		return 20, nil
	}
	size, err := strconv.ParseInt(sizeGB, 10, 64)
	if err != nil || size <= 0 {
		return 0, errors.New("SKR_E2E_DISK_SIZE_GB must be a positive integer")
	}
	return size, nil
}

// RequireServerDiskParams はディスクサイズの環境変数を読み取り、
// cloud-init アーカイブとディスクプランを SDK で自動選択します。
// 環境変数は SKR_E2E_DISK_SIZE_GB (既定 20) です。
// アーカイブとディスクプランの選択は E2E の事前確認のため SDK で静かに実行します。
func RequireServerDiskParams(t *testing.T, caller iaas.APICaller) ServerDiskParams {
	t.Helper()

	sizeGB, err := ParseDiskSizeGB(os.Getenv("SKR_E2E_DISK_SIZE_GB"))
	require.NoError(t, err, "parse SKR_E2E_DISK_SIZE_GB")
	archive := FindCloudimgArchive(t, caller, ServerZone)
	plan := FindDiskPlan(t, caller, ServerZone, sizeGB)
	return ServerDiskParams{
		ArchiveID:  int64(archive.ID),
		DiskPlanID: int64(plan.ID),
		SizeGB:     sizeGB,
	}
}

// DecodeServerList は Server の一括検索の応答を解析します。
func DecodeServerList(data []byte) ([]ServerItem, error) {
	var items []ServerItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode Server list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of Servers")
	}
	return items, nil
}

// ParseServerOutput は Server の参照応答を解析します。ID が空の場合はエラーを返します。
func ParseServerOutput(data []byte) (ServerItem, error) {
	var item ServerItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ServerItem{}, fmt.Errorf("decode Server: %w", err)
	}
	if item.ID.IsEmpty() {
		return ServerItem{}, errors.New("Server ID is empty")
	}
	return item, nil
}

// CheckServerIdentity は参照結果が期待する Server (ID・名前・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func CheckServerIdentity(item ServerItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// SharedIPOfServer は Server の参照結果から共有 NIC の IPv4 アドレスを返します。ない場合は空文字列を返します。
func SharedIPOfServer(item ServerItem) string {
	for _, iface := range item.Interfaces {
		if iface.UpstreamType != "shared" || net.ParseIP(iface.IPAddress) == nil {
			continue
		}
		return iface.IPAddress
	}
	return ""
}

// RequireNoServerNameCollision は作成前に Server と Disk の両方で同名リソースが存在しないことを
// SDK で確認します。確認の実行は証跡に残しません。既存リソースは変更しません。
func RequireNoServerNameCollision(t *testing.T, servers *serverSDK.Service, disks *diskSDK.Service, name string) {
	t.Helper()

	srvMatches, err := LookupServers(t, servers, name)
	require.NoError(t, err, "find existing servers")
	require.Empty(t, srvMatches, "test server name %q already exists; refusing to modify it", name)
	diskMatches, err := lookupDisks(t, disks, name)
	require.NoError(t, err, "find existing disks")
	require.Empty(t, diskMatches, "test disk name %q already exists; refusing to modify it", name)
}

// CreateCloudInitServer は cloud-init ディスクと指定した接続先の NIC を持つテスト用 Server を
// skr CLI で作成し、作成応答と作成した Disk が期待する内容と一致することを SDK で確認します。
// upstream は NIC の接続先で、共有セグメントは "shared"、ルータ＋スイッチの Switch はその ID です。
// 作成に成功した Server を返します。
func CreateCloudInitServer(t *testing.T, h *Helper, disks *diskSDK.Service, name, description string, params ServerDiskParams, upstream string) ServerItem {
	t.Helper()

	// 作成応答に作成結果が含まれる。ディスクと NIC は ADR 0041 の反復可能オプションで指定します
	data, _, err := h.CallAllowingFailure(t, "テスト用のサーバーを cloud-init ディスク付きで作成する",
		"iaas-api", "server", "create",
		"--zone", ServerZone, "--name", name, "--description", description,
		"--cpu", "1", "--memory-gb", "1",
		"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", params.ArchiveID, params.DiskPlanID, params.SizeGB),
		"--nic", "upstream="+upstream,
	)
	require.NoError(t, err, "create Server with cloud-init disk")
	item, err := ParseServerOutput(data)
	require.NoError(t, err)
	require.NoError(t, CheckServerIdentity(item, item.ID, name, description),
		"the create response should contain the created Server")

	// 依存リソースの Disk も作成直後に所属関係と指定内容を SDK で照合します
	diskMatches, err := lookupDisks(t, disks, name)
	require.NoError(t, err, "find the created Disk")
	require.Len(t, diskMatches, 1, "disk find by name should return only the created Disk")
	require.True(t, MatchesDiskSettings(diskMatches[0], name, params),
		"the created Disk should match the requested settings")
	require.Equal(t, item.ID, diskMatches[0].ServerID,
		"the created Disk should be attached to the created Server")
	return item
}

// BootServer は Server を起動します。
func BootServer(t *testing.T, h *Helper, id types.ID) {
	t.Helper()

	_, _, err := h.CallAllowingFailure(t, "サーバーを起動する",
		"iaas-api", "server", "boot", "--zone", ServerZone, "--id", id.String(),
	)
	require.NoError(t, err, "boot Server")
}

// BootServerWithUserData は cloud-init の user-data を指定して Server を起動します。
// user-data は @path 形式でのみ渡し、内容を引数に含めません。
// 起動 API は受理後に応答を失い、リトライが 409 になることがあります。
// この場合も起動は進行するため、SDK で Server の状態を確認し、
// down のままの場合だけ失敗として扱います。
func BootServerWithUserData(t *testing.T, h *Helper, servers *serverSDK.Service, id types.ID, userDataPath string) {
	t.Helper()

	_, _, err := h.CallAllowingFailure(t, "cloud-init の user-data を指定してサーバーを起動する",
		"iaas-api", "server", "boot",
		"--zone", ServerZone, "--id", id.String(),
		"--user-data", "@"+userDataPath,
	)
	if err == nil {
		return
	}
	if !bootInProgress(t, servers, id) {
		require.NoError(t, err, "boot Server with user-data")
	}
}

// bootInProgress は起動 API のエラー後に Server の状態を確認し、起動の受理を待ちます。
// down 以外の状態に変われば起動が受理されたとみなします。証跡に残さない安全確認です。
func bootInProgress(t *testing.T, servers *serverSDK.Service, id types.ID) bool {
	t.Helper()

	deadline := time.Now().Add(BootAcceptWaitTimeout)
	for {
		item, err := FetchServer(t, servers, id)
		if err != nil {
			return false
		}
		if !item.InstanceStatus.IsDown() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(ServerSSHPolling)
	}
}

// WaitForServerSSH は SDK で起動と共有 NIC の IP アドレスを確認し、SSH ログインが
// 成功するまで期限付きで待ちます。起動待ちのポーリングは証跡に残りません。
// cloud-init の反映はベストエフォートで待ち、期限を過ぎたらテストを失敗させます。
// SSH の接続試行は実行記録に残り、成功の判断は hostname の取得で行います。
func WaitForServerSSH(t *testing.T, h *Helper, servers *serverSDK.Service, id types.ID, name, description string, material SSHMaterial) {
	waitForServerSSH(t, h, servers, id, name, description, material, func(item ServerItem) (string, bool) {
		ip := SharedIPOfServer(item)
		return ip, ip != ""
	})
}

// WaitForServerSSHAt は SDK で起動を確認しながら、指定した IPv4 アドレスの Server に
// SSH ログインが成功するまで期限付きで待ちます。ルータ＋スイッチ接続の Server のように、
// SSH 接続先が起動前に決まっている場合に使います。
// 起動待ちのポーリングは証跡に残りません。SSH の接続試行は実行記録に残り、
// 成功の判断は hostname の取得で行います。
func WaitForServerSSHAt(t *testing.T, h *Helper, servers *serverSDK.Service, id types.ID, name, description string, material SSHMaterial, ip string) {
	waitForServerSSH(t, h, servers, id, name, description, material, func(ServerItem) (string, bool) {
		return ip, ip != ""
	})
}

// waitForServerSSH は起動待ちのポーリングを共通化した本体です。ipOf は各ポーリングで
// SSH の接続先を返し、接続先を取得できるまで起動の反映を待ちます。
func waitForServerSSH(t *testing.T, h *Helper, servers *serverSDK.Service, id types.ID, name, description string, material SSHMaterial, ipOf func(ServerItem) (string, bool)) {
	t.Helper()

	deadline := time.Now().Add(ServerSSHWaitTimeout)
	for {
		item, err := FetchServer(t, servers, id)
		require.NoError(t, err, "read Server while waiting for SSH login")
		require.NoError(t, CheckServerIdentity(item, id, name, description),
			"the Server should keep its identity while booting")
		if item.InstanceStatus.IsUp() {
			if ip, ok := ipOf(item); ok {
				if err := h.ConnectSSH(t, material, ServerSSHUser, ip, "SSH ログインを確認する"); err == nil {
					return
				} else {
					t.Logf("SSH login attempt failed, retrying: %v", err)
				}
			}
		}
		if time.Now().Add(ServerSSHPolling).After(deadline) {
			t.Fatalf("SSH login check timed out after %s", ServerSSHWaitTimeout)
		}
		time.Sleep(ServerSSHPolling)
	}
}

// WaitForServerStop は SDK で Server が停止するまで期限付きで待ちます。
// ポーリングの間も Server が識別情報を保っていることを確認し、期限を過ぎたらテストを失敗させます。
// 停止待ちのポーリングは証跡に残りません。
func WaitForServerStop(t *testing.T, servers *serverSDK.Service, id types.ID, name, description string) {
	t.Helper()

	deadline := time.Now().Add(ServerSSHWaitTimeout)
	for {
		item, err := FetchServer(t, servers, id)
		require.NoError(t, err, "read Server while waiting for shutdown")
		require.NoError(t, CheckServerIdentity(item, id, name, description),
			"the Server should keep its identity while stopping")
		if item.InstanceStatus.IsDown() {
			return
		}
		if time.Now().Add(ServerSSHPolling).After(deadline) {
			t.Fatalf("Server %s did not stop within %s", id, ServerSSHWaitTimeout)
		}
		time.Sleep(ServerSSHPolling)
	}
}

// DeleteServerWithDisks は Server を接続中の Disk も含めて削除します。
// --with-disks で接続中の Disk も削除し、--force で電源を止めてから削除します。
// 削除の成否は呼び出し元が後片付けの続きと合わせて判断するため、エラーを返します。
func DeleteServerWithDisks(t *testing.T, h *Helper, id types.ID) error {
	t.Helper()

	_, _, deleteErr := h.CallAllowingFailure(t, "サーバーをディスクとともに削除する",
		"iaas-api", "server", "delete",
		"--zone", ServerZone, "--id", id.String(),
		"--with-disks", "--force", "--fail-if-not-found",
	)
	return deleteErr
}

// CleanupServerResources は後片付けです。作成に成功した Server の ID を優先して SDK で再照合し、
// ID がない場合は許容する識別情報の名前で検索して回収を試みます。識別できない場合は削除しません。
// 削除は skr CLI で --with-disks --force を付けて行い、削除後は SDK で不在を確認し、
// 孤立した Disk の回収まで行います。identities は許容する識別情報で、最初の要素の名前は
// Disk の名前にも使います。削除後の確認も含め、後片付けの失敗は結合したエラーで返します。
func CleanupServerResources(t *testing.T, h *Helper, servers *serverSDK.Service, disks *diskSDK.Service, params ServerDiskParams, id types.ID, identities []ServerIdentity) error {
	t.Helper()

	if len(identities) == 0 {
		return errors.New("cleanup requires at least one allowed Server identity")
	}
	primaryName := identities[0].Name

	serverID := id
	if serverID.IsEmpty() {
		for _, identity := range identities {
			matches, err := LookupServers(t, servers, identity.Name)
			if err != nil {
				return fmt.Errorf("cannot locate possibly created Server: %w", err)
			}
			if len(matches) == 0 {
				continue
			}
			if len(matches) != 1 || matches[0].ID.IsEmpty() ||
				matches[0].Name != identity.Name || matches[0].Description != identity.Description {
				return errors.New("cannot safely identify created Server; manual cleanup required")
			}
			serverID = matches[0].ID
			break
		}
		if serverID.IsEmpty() {
			return cleanupOrphanDisks(t, h, disks, primaryName, params, nil)
		}
	}

	// 削除前に SDK で再照合し、保持した ID と許容する識別情報が一致する場合だけ削除する。
	// 更新フェーズで復元が完了していない場合は更新後の識別情報と一致する。
	current, err := FetchServer(t, servers, serverID)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	allowed := false
	for _, identity := range identities {
		if CheckServerIdentity(current, serverID, identity.Name, identity.Description) == nil {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("cleanup refused: Server identity changed; manual cleanup required")
	}

	deleteErr := DeleteServerWithDisks(t, h, serverID)

	names := make([]string, 0, len(identities))
	for _, identity := range identities {
		names = append(names, identity.Name)
	}
	absentErr := requireServerAbsent(t, servers, names...)
	return errors.Join(deleteErr, absentErr, cleanupOrphanDisks(t, h, disks, primaryName, params, current.Disks))
}

// FetchServer は SDK で Server を参照します。起動待ちや後片付けの確認など、
// 証跡に残さない安全確認で使います。
func FetchServer(t *testing.T, servers *serverSDK.Service, id types.ID) (ServerItem, error) {
	t.Helper()

	server, err := servers.ReadWithContext(t.Context(), &serverSDK.ReadRequest{Zone: ServerZone, ID: id})
	if err != nil {
		return ServerItem{}, fmt.Errorf("read Server %s: %w", id, err)
	}
	return serverFromSDK(server), nil
}

// LookupServers は SDK で名前部分一致検索を行い、全ページの結果を返します。
// 後片て付けでの残存確認など、証跡に残さない安全確認で使います。
func LookupServers(t *testing.T, servers *serverSDK.Service, name string) ([]ServerItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]ServerItem, error) {
		found, err := servers.FindWithContext(ctx, &serverSDK.FindRequest{
			Zone:  ServerZone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]ServerItem, 0, len(found))
		for _, s := range found {
			if s == nil {
				continue
			}
			items = append(items, serverFromSDK(s))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Servers by name: %w", err)
	}
	return items, nil
}

// lookupDisks は SDK で名前部分一致検索を行い、全ページの結果を返します。
func lookupDisks(t *testing.T, disks *diskSDK.Service, name string) ([]DiskItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]DiskItem, error) {
		found, err := disks.FindWithContext(ctx, &diskSDK.FindRequest{
			Zone:  ServerZone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]DiskItem, 0, len(found))
		for _, d := range found {
			if d == nil {
				continue
			}
			items = append(items, diskFromSDK(d))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Disks by name: %w", err)
	}
	return items, nil
}

// paginateFind は Count/From のページングで全ページを取得します。
// 途中ページの取得に失敗した場合はエラーを返し、不在として扱いません。
func paginateFind[T any](t *testing.T, fetch func(ctx context.Context, from int) ([]T, error)) ([]T, error) {
	t.Helper()

	var all []T
	for from := 0; ; from += findPageSize {
		items, err := fetch(t.Context(), from)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < findPageSize {
			return all, nil
		}
	}
}

// requireServerAbsent は Server が指定した名前のどれでも残っていないことを SDK で確認します。
func requireServerAbsent(t *testing.T, servers *serverSDK.Service, names ...string) error {
	t.Helper()

	for _, name := range names {
		matches, err := LookupServers(t, servers, name)
		if err != nil {
			return fmt.Errorf("verify Server absence after cleanup: %w", err)
		}
		if len(matches) != 0 {
			return fmt.Errorf("Server %q remains after cleanup; manual cleanup required", name)
		}
	}
	return nil
}

// cleanupOrphanDisks は Server 削除後に残った同名 Disk を回収します。
// SDK で検索と参照を行い、既知の Disk と一致する場合のみ skr CLI で削除し、
// 削除後は SDK で不在を確認します。識別できない Disk は変更せず、手動での回収が必要なことを報告します。
func cleanupOrphanDisks(t *testing.T, h *Helper, disks *diskSDK.Service, name string, params ServerDiskParams, knownDisks []DiskItem) error {
	t.Helper()

	matches, err := lookupDisks(t, disks, name)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 || len(knownDisks) != 1 || matches[0].ID.IsEmpty() ||
		matches[0].ID != knownDisks[0].ID || matches[0].Name != name {
		return errors.New("cannot safely identify orphan Disk; manual cleanup required")
	}
	id := matches[0].ID
	disk, err := disks.ReadWithContext(t.Context(), &diskSDK.ReadRequest{Zone: ServerZone, ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify orphan Disk: read Disk %s: %w", id, err)
	}
	if !matchesCreatedDisk(diskFromSDK(disk), id, name, params) {
		return errors.New("orphan Disk identity or attachment changed; manual cleanup required")
	}
	if _, _, err := h.CallAllowingFailure(t, "孤立ディスクを削除する",
		"iaas-api", "disk", "delete",
		"--zone", ServerZone, "--id", id.String(),
		"--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete orphan Disk %s: %w", id, err)
	}
	matches, err = lookupDisks(t, disks, name)
	if err != nil {
		return err
	}
	if len(matches) != 0 {
		return errors.New("Disk remains after cleanup; manual cleanup required")
	}
	return nil
}

// serverFromSDK は SDK の Server を参照結果の型へ変換します。
// 接続中 Disk の情報は Server の応答に含まれる項目へ写します。
func serverFromSDK(s *iaas.Server) ServerItem {
	item := ServerItem{
		ID:             s.ID,
		Name:           s.Name,
		Description:    s.Description,
		InstanceStatus: s.InstanceStatus,
	}
	for _, iface := range s.Interfaces {
		if iface == nil {
			continue
		}
		item.Interfaces = append(item.Interfaces, ServerInterface{
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
func connectedDiskFromSDK(d *iaas.ServerConnectedDisk) DiskItem {
	return DiskItem{
		ID:         d.ID,
		Name:       d.Name,
		DiskPlanID: d.DiskPlanID,
		SizeMB:     int64(d.SizeMB),
	}
}

// diskFromSDK は SDK の Disk を Disk の参照結果の型へ変換します。
func diskFromSDK(d *iaas.Disk) DiskItem {
	return DiskItem{
		ID:              d.ID,
		Name:            d.Name,
		ServerID:        d.ServerID,
		DiskPlanID:      d.DiskPlanID,
		SourceArchiveID: d.SourceArchiveID,
		SizeMB:          int64(d.SizeMB),
	}
}

// MatchesDiskSettings は Disk が、今回の実行で作成した指定内容 (名前・プラン・作成元アーカイブ・サイズ) と
// 一致することを確認します。サイズは MB 単位の応答を GB に換算して照合します。
func MatchesDiskSettings(disk DiskItem, name string, params ServerDiskParams) bool {
	return disk.Name == name &&
		disk.DiskPlanID == types.ID(params.DiskPlanID) && disk.SourceArchiveID == types.ID(params.ArchiveID) &&
		params.SizeGB > 0 && disk.SizeMB/1024 == params.SizeGB && disk.SizeMB%1024 == 0
}

// matchesCreatedDisk は孤立 Disk が、今回の実行で作成したクラウドディスクと一致することを確認します。
// 作成時の指定内容に加え、ID が一致し Server に接続されていないことも確認します。
func matchesCreatedDisk(disk DiskItem, id types.ID, name string, params ServerDiskParams) bool {
	return disk.ID == id && disk.ServerID.IsEmpty() && MatchesDiskSettings(disk, name, params)
}
