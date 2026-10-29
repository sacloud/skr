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
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// createTimeout はルータ＋スイッチと Server の作成の期限です。
// 作成直後の404を最大30分リトライする SDK の待機を許容します。
const createTimeout = 35 * time.Minute

// TestScenarioInternet は internet コマンドのライブ E2E です。ルータ＋スイッチを作成し、
// 参照・名前検索・更新と復元・モニタリングを確認したあと、スイッチに接続する Server を
// cloud-init ディスク付きで作成して SSH ログインします。追加サブネットの追加・更新・削除と
// IPv6 の有効化・参照・無効化も確認します。後片て付けは Server と Disk、Internet の順に
// 識別情報の再照合つきで行います。テスト対象の操作は skr CLI で実行し、事前確認と
// 後片て付けの確認、起動待ちのポーリングは SDK で静かに実行します。
func TestScenarioInternet(t *testing.T) {
	h := helper.New(t, "internet-api", "ルータ＋スイッチの作成・参照・更新・削除と、スイッチ接続 Server の SSH ログイン、追加サブネットと IPv6 の操作が正しく動作することを確認する")

	// 事前確認と後片て付けの確認、起動待ちのポーリングは SDK で静かに実行します。
	// skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	internets := internetSDK.New(client)
	servers, disks := serverSDK.New(client), diskSDK.New(client)

	// cloud-init アーカイブとディスクプランを SDK で自動選択します
	params := helper.RequireServerDiskParams(t, client)

	name := internetNamePrefix + helper.RandomSuffix(t)

	// 作成前に Internet、Server、Disk の同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、既存リソースを変更しないまま終了する。
	requireNoInternetNameCollision(t, internets, name)
	helper.RequireNoServerNameCollision(t, servers, disks, name)

	// 後片て付け: Server と Disk、Internet の順に削除する。Server が残っている場合は
	// Internet を削除せず、手動確認を求める。
	var internetID, switchID, serverID types.ID
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, serverID, []helper.ServerIdentity{
			{Name: name, Description: serverDesc},
		}); err != nil {
			t.Errorf("server cleanup failed: %v", err)
		}
		// Server が残っている状態で Internet を削除しないため、不在を SDK で確認してから進む
		leftover, err := helper.LookupServers(t, servers, name)
		if err != nil {
			t.Errorf("cannot verify Server absence before Internet cleanup: %v", err)
			return
		}
		if len(leftover) > 0 {
			t.Errorf("Server %q remains; skipping Internet cleanup; manual cleanup required", name)
			return
		}
		if err := cleanupInternet(t, h, internets, name, internetID, switchID); err != nil {
			t.Errorf("Internet cleanup failed: %v", err)
		}
	}()

	// ルータ＋スイッチを作成します。作成直後の404を最大30分リトライする SDK の待機を
	// 許容して35分の期限を設けます
	data := h.CallWithTimeout(t, createTimeout, "ルータ＋スイッチを作成する",
		"iaas-api", "internet", "create",
		"--zone", internetZone, "--name", name, "--description", internetDesc,
		"--network-mask-len", "28", "--band-width-mbps", "100",
	)
	router, err := parseInternet(data)
	require.NoError(t, err, "decode created Internet")
	internetID, switchID = router.ID, router.Switch.ID
	require.NoError(t, checkInternetIdentity(router, internetID, name, switchID),
		"the create response should contain the created Internet")

	// 参照で作成内容を確認します
	read := readInternet(t, h, "作成したルータ＋スイッチを参照する", internetID)
	require.NoError(t, checkInternetIdentity(read, internetID, name, switchID),
		"read should return the created Internet")

	// internet find --name が作成した Internet を見つけられることを確認します。--name は部分一致検索です
	found := findInternetByCLI(t, h, name)
	require.Len(t, found, 1, "internet find --name should return only the created Internet")
	require.NoError(t, checkInternetIdentity(found[0], internetID, name, switchID),
		"find should return the created Internet")

	// 名前と説明を更新して復元します
	updateAndVerifyInternet(t, h, internetID, name, switchID)

	// 最近の期間のモニタリングを確認します
	monitorRouter(t, h, internetID)

	// スイッチに接続する Server を cloud-init ディスク付きで作成します
	created := helper.CreateCloudInitServer(t, h, disks, name, serverDesc, params, switchID.String())
	serverID = created.ID

	// 参照で NIC の接続先と MAC アドレスを確認します
	serverData := h.Call(t, "作成した Server を参照する",
		"iaas-api", "server", "read", "--zone", internetZone, "--id", serverID.String(),
		"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}")
	server, err := parseServerOutput(serverData)
	require.NoError(t, err, "decode created Server")
	require.NoError(t, checkServerIdentity(server, serverID, name, switchID),
		"read should return the created Server")

	// サブネットの一覧から Server の IPv4 設定を決めます。ルータ＋スイッチ接続では
	// Interfaces[].IPAddress が空のまま作成されるため、一覧に含まれるアドレスから選びます
	subnets := listSubnets(t, h, "ルータ＋スイッチのサブネットの一覧を取得する", internetID)
	nic, prefixLen, gateway, err := selectServerNIC(subnets, internetID, switchID, server)
	require.NoError(t, err, "select the Server IPv4 address from the subnet")

	// 一時的な SSH 鍵と、NIC 設定の runcmd を含む cloud-init の user-data を作成します。
	// user-data は MAC アドレスで NIC を特定し、選択したアドレスで IPv4 を設定します
	material := helper.WriteSSHMaterialWithRuncmd(t, helper.ServerSSHUser, false,
		nicConfigScript(nic.MACAddress, nic.IPAddress, prefixLen, gateway))

	// user-data を指定して起動します
	helper.BootServerWithUserData(t, h, servers, serverID, material.UserDataPath)

	// 起動を確認しながら SSH ログインできるまで待ちます。SSH の接続先は選択したアドレスです
	helper.WaitForServerSSHAt(t, h, servers, serverID, name, serverDesc, material, nic.IPAddress)

	// 追加サブネットの追加・更新・削除を確認します
	subnetScenario(t, h, internets, internetID, nic.IPAddress)

	// IPv6 の有効化・参照・無効化を確認します
	ipv6Scenario(t, h, internetID)
}

// readInternet は skr CLI でルータ＋スイッチを参照します。
func readInternet(t *testing.T, h *helper.Helper, description string, id types.ID) internetItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "internet", "read", "--zone", internetZone, "--id", id.String(),
		"--query", "{ID,Name,Description,NetworkMaskLen,BandWidthMbps,Switch}")
	item, err := parseInternet(data)
	require.NoError(t, err, "%s: decode Internet", description)
	return item
}

// findInternetByCLI は skr CLI の名前検索でルータ＋スイッチを検索します。
// --name は分割せず1要素の Names に変換する部分一致検索です。
func findInternetByCLI(t *testing.T, h *helper.Helper, name string) []internetItem {
	t.Helper()

	data := h.Call(t, "ルータ＋スイッチを名前で検索する",
		"iaas-api", "internet", "find", "--zone", internetZone, "--name", name,
		"--query", "map({ID,Name,Description,NetworkMaskLen,BandWidthMbps,Switch})")
	items, err := decodeInternetList(data)
	require.NoError(t, err, "decode Internet list")
	return items
}

// updatedInternetDescription は更新確認に使う説明です。更新後は元の値に復元するため、
// 後片て付けの識別には影響しません。
const updatedInternetDescription = internetDesc + " (updated)"

// updateAndVerifyInternet はルータ＋スイッチの名前と説明を更新し、参照で反映を確認したあと、
// 元の値に復元して確認します。名前と説明は後片て付けの識別に使われるため、復元します。
func updateAndVerifyInternet(t *testing.T, h *helper.Helper, id types.ID, name string, switchID types.ID) {
	t.Helper()

	updatedName := name + "-upd"
	h.Call(t, "ルータ＋スイッチの名前と説明を更新する",
		"iaas-api", "internet", "update", "--zone", internetZone, "--id", id.String(),
		"--name", updatedName, "--description", updatedInternetDescription)

	updated := readInternet(t, h, "更新した名前と説明を参照して確認する", id)
	require.Equal(t, updatedName, updated.Name, "update should change the name")
	require.Equal(t, updatedInternetDescription, updated.Description, "update should change the description")

	h.Call(t, "ルータ＋スイッチの名前と説明を元に戻す",
		"iaas-api", "internet", "update", "--zone", internetZone, "--id", id.String(),
		"--name", name, "--description", internetDesc)

	restored := readInternet(t, h, "復元した名前と説明を参照して確認する", id)
	require.NoError(t, checkInternetIdentity(restored, id, name, switchID),
		"the restored Internet should keep its identity")
}
