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
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioServer は Server E2E です。cloud-init ディスク付きの Server を is1b に作成し、
// user-data で起動して SSH 接続を確認し、名前と説明の更新・復元と後片付けまで行います。
// cloud-init アーカイブとディスクプランは SDK で自動選択します。
// テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認と起動待ちのポーリングは
// SDK で静かに実行します。途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioServer(t *testing.T) {
	h := helper.New(t, "server-api", "IaaS サーバーを cloud-init ディスク付きで作成・起動し、SSH 接続・更新・削除と後片付けが正しく動作することを確認する")

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	servers, disks := serverSDK.New(client), diskSDK.New(client)

	// cloud-init アーカイブとディスクプランを SDK で自動選択します。
	// アーカイブは Ubuntu Server 24.04 LTS (cloudimg) に限定して選択します
	params := requireServerParams(t, client)

	// 一時的な SSH 鍵と cloud-init の user-data を作成します。秘密鍵の内容は実行記録に残りません
	material := helper.WriteSSHMaterial(t, sshUser, false)

	run := &serverRun{name: namePrefix + helper.RandomSuffix(t), params: params}

	// 作成前に Server と Disk の両方で同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	requireNoServerNameCollision(t, servers, disks, run.name)

	// 後片付け: Server を識別情報の再照合つきで削除し、孤立ディスクの回収と不在確認まで行う
	defer func() {
		if err := cleanupServerResources(t, h, servers, disks, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// テスト用の Server を cloud-init ディスクと共有 NIC 付きで作成し、参照と名前検索で確認する
	createServer(t, h, run)

	// cloud-init の user-data を指定して起動する。user-data は @path 形式でのみ渡し、内容を引数に含めません
	_, _, err = h.CallAllowingFailure(t, "cloud-init の user-data を指定してサーバーを起動する",
		"iaas-api", "server", "boot",
		"--zone", zone, "--id", run.serverID.String(),
		"--user-data", "@"+material.UserDataPath,
	)
	require.NoError(t, err)

	// 起動を確認しながら SSH ログインできるまで待つ
	waitForServerSSH(t, h, servers, material, run)

	// サーバーの名前と説明を更新して元に戻す
	updateAndVerifyServer(t, h, run.serverID, run.name)
}

// requireNoServerNameCollision は作成前に Server と Disk の両方で同名リソースが存在しないことを
// SDK で確認します。確認の実行は証跡に残しません。既存リソースは変更しません。
func requireNoServerNameCollision(t *testing.T, servers *serverSDK.Service, disks *diskSDK.Service, name string) {
	t.Helper()

	srvMatches, err := lookupServers(t, servers, name)
	require.NoError(t, err, "find existing servers")
	require.Empty(t, srvMatches, "test server name %q already exists; refusing to modify it", name)
	diskMatches, err := lookupDisks(t, disks, name)
	require.NoError(t, err, "find existing disks")
	require.Empty(t, diskMatches, "test disk name %q already exists; refusing to modify it", name)
}

// createServer は cloud-init ディスクと共有 NIC を持つテスト用 Server を skr CLI で作成し、
// 参照と名前検索で作成結果を確認します。作成に成功した ID を run に記録し、
// 後片付けが確実に回収できるようにします。
func createServer(t *testing.T, h *helper.Helper, run *serverRun) {
	t.Helper()

	// 作成応答に作成結果が含まれる。ディスクと NIC は ADR 0041 の反復可能オプションで指定します
	data, _, err := h.CallAllowingFailure(t, "テスト用のサーバーを cloud-init ディスク付きで作成する",
		"iaas-api", "server", "create",
		"--zone", zone, "--name", run.name, "--description", serverDescription,
		"--cpu", "1", "--memory-gb", "1",
		"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", run.params.ArchiveID, run.params.DiskPlanID, run.params.SizeGB),
		"--nic", "upstream=shared",
	)
	require.NoError(t, err, "create Server with cloud-init disk")
	item, err := parseServer(data)
	require.NoError(t, err)
	require.NoError(t, checkServerIdentity(item, item.ID, run.name, serverDescription), "the create response should contain the created Server")
	run.serverID = item.ID

	// server read が作成済みの Server を返すことを確認する。--query で確認に必要な項目へ絞り込みます
	data, _, err = h.CallAllowingFailure(t, "作成したサーバーを参照する",
		"iaas-api", "server", "read", "--zone", zone, "--id", run.serverID.String(),
		"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}",
	)
	require.NoError(t, err, "read the created Server")
	item, err = parseServer(data)
	require.NoError(t, err)
	require.NoError(t, checkServerIdentity(item, run.serverID, run.name, serverDescription), "server read should return the created Server")

	// server find --name が作成した Server を見つけられることを確認します。--name は部分一致検索です
	data, _, err = h.CallAllowingFailure(t, "作成したサーバーを名前で検索する",
		"iaas-api", "server", "find",
		"--zone", zone, "--name", run.name,
		"--query", "map({ID,Name,Description})",
	)
	require.NoError(t, err, "find the created Server by name")
	matches, err := decodeServerList(data)
	require.NoError(t, err)
	require.Len(t, matches, 1, "server find --name should return only the created Server")
	require.Equal(t, run.serverID, matches[0].ID, "server find --name should return the created Server")
}

// waitForServerSSH は SDK で起動と共有 NIC の IP アドレスを確認し、SSH ログインが
// 成功するまで期限付きで待ちます。起動待ちのポーリングは証跡に残りません。
// cloud-init の反映はベストエフォートで待ち、期限を過ぎたらテストを失敗させます。
// SSH の接続試行は実行記録に残り、成功の判断は hostname の取得で行います。
func waitForServerSSH(t *testing.T, h *helper.Helper, servers *serverSDK.Service, material helper.SSHMaterial, run *serverRun) {
	t.Helper()

	deadline := time.Now().Add(sshWaitTimeout)
	for {
		item, err := fetchServer(t, servers, run.serverID)
		require.NoError(t, err, "read Server while waiting for SSH login")
		require.NoError(t, checkServerIdentity(item, run.serverID, run.name, serverDescription), "the Server should keep its identity while booting")
		if item.InstanceStatus.IsUp() {
			if ip := sharedIPOfServer(item); ip != "" {
				if err := h.ConnectSSH(t, material, sshUser, ip, "SSH ログインを確認する"); err == nil {
					return
				} else {
					t.Logf("SSH login attempt failed, retrying: %v", err)
				}
			}
		}
		if time.Now().Add(sshPolling).After(deadline) {
			t.Fatalf("SSH login check timed out after %s", sshWaitTimeout)
		}
		time.Sleep(sshPolling)
	}
}

// sharedIPOfServer は Server の参照結果から共有 NIC の IPv4 アドレスを返します。ない場合は空文字列を返します。
func sharedIPOfServer(item serverItem) string {
	for _, iface := range item.Interfaces {
		if iface.UpstreamType != "shared" || net.ParseIP(iface.IPAddress) == nil {
			continue
		}
		return iface.IPAddress
	}
	return ""
}

// updatedServerDescription は更新確認中に使用する一時的な説明です。
const updatedServerDescription = serverDescription + " (updated)"

// updatedServerName は更新確認中に使用する一時的な名前です。
func updatedServerName(name string) string {
	return name + "-upd"
}

// updateAndVerifyServer は Server の名前と説明を skr CLI で更新し、応答で反映を確認してから
// 元の値に戻し、復元を確認します。復元により後片付けが Server を再び識別できるようになります。
func updateAndVerifyServer(t *testing.T, h *helper.Helper, id types.ID, name string) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "サーバーの名前と説明を更新する",
		"iaas-api", "server", "update",
		"--zone", zone, "--id", id.String(),
		"--name", updatedServerName(name), "--description", updatedServerDescription,
		// 応答は単一オブジェクトのため、--query はオブジェクト構築で確認に必要な項目へ絞り込みます
		"--query", "{ID,Name,Description}",
	)
	require.NoError(t, err, "update Server Name and Description")
	item, err := parseServer(data)
	require.NoError(t, err)
	require.NoError(t, checkServerIdentity(item, id, updatedServerName(name), updatedServerDescription), "the update should be reflected in the response")

	data, _, err = h.CallAllowingFailure(t, "サーバーの名前と説明を元に戻す",
		"iaas-api", "server", "update",
		"--zone", zone, "--id", id.String(),
		"--name", name, "--description", serverDescription,
		"--query", "{ID,Name,Description}",
	)
	require.NoError(t, err, "restore Server Name and Description")
	item, err = parseServer(data)
	require.NoError(t, err)
	require.NoError(t, checkServerIdentity(item, id, name, serverDescription), "the restore should be reflected in the response")
}

// cleanupServerResources は後片付けです。作成に成功した Server の ID を優先して SDK で再照合し、
// ID がない場合は名前検索で回収を試みます。識別できない場合は削除しません。
// 削除は skr CLI で行い、削除後は SDK で同名リソースの不在を確認し、孤立した Disk の回収まで行います。
func cleanupServerResources(t *testing.T, h *helper.Helper, servers *serverSDK.Service, disks *diskSDK.Service, run *serverRun) error {
	t.Helper()

	id := run.serverID
	if id.IsEmpty() {
		matches, err := lookupServers(t, servers, run.name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Server: %w", err)
		}
		if len(matches) == 0 {
			return cleanupOrphanDisks(t, h, disks, run, nil)
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != serverDescription {
			return errors.New("cannot safely identify created Server; manual cleanup required")
		}
		id = matches[0].ID
	}
	// 削除前に SDK で再照合し、保持した ID と今回設定した識別情報 (作成時または更新時) が
	// 一致する場合だけ削除する。更新フェーズで復元が完了していない場合は更新後の値と一致する。
	current, err := fetchServer(t, servers, id)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	identityErr := checkServerIdentity(current, id, run.name, serverDescription)
	if identityErr != nil {
		identityErr = checkServerIdentity(current, id, updatedServerName(run.name), updatedServerDescription)
	}
	if identityErr != nil {
		return fmt.Errorf("cleanup refused: Server identity changed; manual cleanup required: %w", identityErr)
	}
	// --with-disks で接続中の Disk も削除し、--force で電源を止めてから削除します
	_, _, deleteErr := h.CallAllowingFailure(t, "サーバーをディスクとともに削除する",
		"iaas-api", "server", "delete",
		"--zone", zone, "--id", id.String(),
		"--with-disks", "--force", "--fail-if-not-found",
	)
	if err := requireServerAbsent(t, servers, run); err != nil {
		return errors.Join(deleteErr, err)
	}
	return errors.Join(deleteErr, cleanupOrphanDisks(t, h, disks, run, current.Disks))
}

// requireServerAbsent は Server が元の名前と更新フェーズの名前のどちらでも残っていないことを
// SDK で確認します。更新フェーズで復元が完了していない場合も、更新後の名前での不在を確認します。
func requireServerAbsent(t *testing.T, servers *serverSDK.Service, run *serverRun) error {
	t.Helper()

	for _, name := range []string{run.name, updatedServerName(run.name)} {
		matches, err := lookupServers(t, servers, name)
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
func cleanupOrphanDisks(t *testing.T, h *helper.Helper, disks *diskSDK.Service, run *serverRun, knownDisks []diskItem) error {
	t.Helper()

	matches, err := lookupDisks(t, disks, run.name)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 || len(knownDisks) != 1 || matches[0].ID.IsEmpty() ||
		matches[0].ID != knownDisks[0].ID || matches[0].Name != run.name {
		return errors.New("cannot safely identify orphan Disk; manual cleanup required")
	}
	id := matches[0].ID
	disk, err := disks.ReadWithContext(t.Context(), &diskSDK.ReadRequest{Zone: zone, ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify orphan Disk: read Disk %s: %w", id, err)
	}
	if !matchesCreatedDisk(diskFromSDK(disk), id, run.name, run.params) {
		return errors.New("orphan Disk identity or attachment changed; manual cleanup required")
	}
	if _, _, err := h.CallAllowingFailure(t, "孤立ディスクを削除する",
		"iaas-api", "disk", "delete",
		"--zone", zone, "--id", id.String(),
		"--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete orphan Disk %s: %w", id, err)
	}
	matches, err = lookupDisks(t, disks, run.name)
	if err != nil {
		return err
	}
	if len(matches) != 0 {
		return errors.New("Disk remains after cleanup; manual cleanup required")
	}
	return nil
}
