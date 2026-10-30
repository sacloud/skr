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
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipAddressNamePrefix はテスト用サーバーの名前の接頭辞です。ランダムな接尾辞とともに使い、
// 過去の実行のリソースと衝突しないようにします。
const ipAddressNamePrefix = "skr-e2e-ipaddress-"

// ipAddressServerDesc はテスト用サーバーに付ける固定の説明です。後片て付けの識別にも使います。
const ipAddressServerDesc = "Temporary Server for skr ipaddress E2E"

// TestScenarioIPAddressUpdateHostName は ipaddress update-host-name のライブ E2E です。
// 共有セグメントにサーバーを作成し、割り当てられたグローバル IP アドレスに対して
// update-host-name で逆引きホスト名を設定できること、--host-name を省略すると
// 逆引きホスト名が空に設定される (登録が解除される) ことを確認します。
// テスト対象の操作は skr CLI で実行し、事前確認と後片て付けの確認は SDK で静かに実行します。
func TestScenarioIPAddressUpdateHostName(t *testing.T) {
	h := helper.New(t, "ipaddress-api", "ipaddress update-host-name が、割り当て済みのグローバル IP アドレスの逆引きホスト名を正しく変更できることを確認する")

	// 事前確認と後片て付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	servers := serverSDK.New(client)
	disks := diskSDK.New(client)

	name := ipAddressNamePrefix + helper.RandomSuffix(t)

	// クラウドディスクの作成元アーカイブとディスクプランを SDK で自動選択します
	params := helper.RequireServerDiskParams(t, client)
	helper.RequireNoServerNameCollision(t, servers, disks, name)

	var serverID types.ID
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, serverID,
			[]helper.ServerIdentity{{Name: name, Description: ipAddressServerDesc}}); err != nil {
			t.Errorf("Server cleanup failed: %v", err)
		}
	}()

	// 共有セグメントにテスト用のサーバーを作成します
	server := helper.CreateCloudInitServer(t, h, disks, name, ipAddressServerDesc, params, "shared")
	serverID = server.ID

	// サーバーを起動して、起動の完了を待ちます
	helper.BootServer(t, h, server.ID)
	item := waitForServerUp(t, servers, server.ID, name, ipAddressServerDesc)

	// 共有セグメントに割り当てられた IPv4 アドレスを取得します
	assignedIP := helper.SharedIPOfServer(item)
	require.NotEmpty(t, assignedIP,
		"the started Server should have a shared segment IPv4 address")

	// ipaddress list で割り当てられたアドレスが一覧に含まれることを確認します
	addrs := listIPAddresses(t, h)
	found := findIPAddressByAddr(addrs, assignedIP)
	require.NotNil(t, found,
		"ipaddress list should contain the address assigned to the test Server (%s)", assignedIP)

	// ipaddress read で割り当てられたアドレスを参照します
	read := readIPAddress(t, h, assignedIP)
	require.Equal(t, assignedIP, read.IPAddress,
		"ipaddress read should return the assigned address")

	// 逆引き設定には、対象のアドレスへの正引き (A レコード) が必要です
	// (https://manual.sakura.ad.jp/cloud/network/switch/reverse-dns.html)。
	// テストのアドレスは実行ごとに変わるため、ホスト名に含まれるアドレスを応答する
	// ワイルドカード DNS (sslip.io) で正引きを表現します。
	hostName := assignedIP + ".sslip.io"

	// ipaddress update-host-name で逆引きホスト名を設定します
	updateIPAddressHostName(t, h, assignedIP, hostName)

	// ipaddress read で設定後の逆引きホスト名を確認します
	require.Equal(t, hostName, readIPAddress(t, h, assignedIP).HostName,
		"ipaddress read should return the registered host name")

	// --host-name を省略した update-host-name は、逆引きホスト名を空に設定します (登録の解除)
	updateIPAddressHostName(t, h, assignedIP, "")
	require.Empty(t, readIPAddress(t, h, assignedIP).HostName,
		"the host name should be cleared")
}

// waitForServerUp は SDK でサーバーが起動するまで期限付きで待ちます。
// ポーリングの間もサーバーが識別情報を保っていることを確認します。
// 起動待ちのポーリングは証跡に残さない安全確認です。
func waitForServerUp(t *testing.T, servers *serverSDK.Service, id types.ID, name, description string) helper.ServerItem {
	t.Helper()

	deadline := time.Now().Add(helper.ServerSSHWaitTimeout)
	for {
		item, err := helper.FetchServer(t, servers, id)
		require.NoError(t, err, "read Server while waiting for boot")
		require.NoError(t, helper.CheckServerIdentity(item, id, name, description),
			"the Server should keep its identity while booting")
		if item.InstanceStatus.IsUp() {
			return item
		}
		if time.Now().Add(helper.ServerSSHPolling).After(deadline) {
			t.Fatalf("Server %s did not start within %s", id, helper.ServerSSHWaitTimeout)
		}
		time.Sleep(helper.ServerSSHPolling)
	}
}
