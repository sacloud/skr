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
	"os/exec"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	gslbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/gslb"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// gslbNamePrefix はテスト用リソースの名前の接頭辞です。ランダムな接尾辞とともに使い、
// 過去の実行のリソースと衝突しないようにします。
const gslbNamePrefix = "skr-e2e-gslb-"

// serverDescription はヘルスチェック用のテスト Server に付ける固定の説明です。後片付けの識別にも使います。
const serverDescription = "Temporary GSLB health-check server created by skr live E2E"

// TestScenarioGSLB は GSLB E2E です。ヘルスチェック用の Server を2台 (実サーバーとソーリーサーバー) 作成し、
// GSLB の作成・参照・更新・削除と、DNS への実際の名前解決とヘルスチェックの切り替えを確認します。
// 実サーバーを SSH で停止するとソーリーサーバーへ切り替わり、再起動すると元へ戻ることを確認します。
// テスト対象の操作は skr CLI と dig で実行し、事前確認と後片付けの確認と起動待ちのポーリングは
// SDK で静かに実行します。途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioGSLB(t *testing.T) {
	h := helper.New(t, "gslb-api", "GSLB の作成・参照・更新・削除と、サーバーへの実際の名前解決とヘルスチェック動作が正しく動作することを確認する")

	// dig は DNS の確認に必要です
	if _, err := exec.LookPath("dig"); err != nil {
		require.NoError(t, err, "dig is required for the GSLB live E2E")
	}

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	servers, disks := serverSDK.New(client), diskSDK.New(client)
	gslbs := gslbSDK.New(client)

	// cloud-init アーカイブとディスクプランを SDK で自動選択します
	params := helper.RequireServerDiskParams(t, client)

	// 一時的な SSH 鍵と cloud-init の user-data を作成します。実サーバーの停止に sudo を使うため付与します
	material := helper.WriteSSHMaterial(t, helper.ServerSSHUser, true)

	name := gslbNamePrefix + helper.RandomSuffix(t)
	primaryName, sorryName := name+"-server", name+"-sorry"

	// 作成前に Server、Disk、GSLB の同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	helper.RequireNoServerNameCollision(t, servers, disks, primaryName)
	helper.RequireNoServerNameCollision(t, servers, disks, sorryName)
	requireNoGSLBNameCollision(t, gslbs, name)

	// 後片付け: 両方の Server と GSLB を識別情報の再照合つきで削除する
	var primaryID, sorryID, gslbID types.ID
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, primaryID, []helper.ServerIdentity{
			{Name: primaryName, Description: serverDescription},
		}); err != nil {
			t.Errorf("primary server cleanup failed: %v", err)
		}
	}()
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, sorryID, []helper.ServerIdentity{
			{Name: sorryName, Description: serverDescription},
		}); err != nil {
			t.Errorf("sorry server cleanup failed: %v", err)
		}
	}()
	defer func() {
		if err := cleanupGSLB(t, h, gslbs, name, gslbID); err != nil {
			t.Errorf("GSLB cleanup failed: %v", err)
		}
	}()

	// 実サーバーを cloud-init ディスク付きで作成し、user-data を指定して起動して SSH ログインを確認します
	primary := helper.CreateCloudInitServer(t, h, disks, primaryName, serverDescription, params)
	primaryID = primary.ID
	helper.BootServerWithUserData(t, h, primaryID, material.UserDataPath)
	helper.WaitForServerSSH(t, h, servers, primaryID, primaryName, serverDescription, material)
	primaryIP := sharedIPOfRunningServer(t, servers, primaryID)

	// ソーリーサーバーも同じ手順で作成して起動します。全実サーバー停止時の誘導先になります
	sorry := helper.CreateCloudInitServer(t, h, disks, sorryName, serverDescription, params)
	sorryID = sorry.ID
	helper.BootServerWithUserData(t, h, sorryID, material.UserDataPath)
	helper.WaitForServerSSH(t, h, servers, sorryID, sorryName, serverDescription, material)
	sorryIP := sharedIPOfRunningServer(t, servers, sorryID)

	require.NotEqual(t, primaryIP, sorryIP, "primary and sorry server must have different IP addresses")

	// テスト用の GSLB を作成します。実サーバーを宛先に設定し、ソーリーサーバーを誘導先に設定します
	data, _, err := h.CallAllowingFailure(t, "テスト用の GSLB を作成する",
		"gslb-api", "create",
		"--name", name, "--description", gslbDescription,
		"--health-check-protocol", "ping", "--delay-loop", "10", "--tags", "skr-e2e",
		"--destination-server", "ip-address="+primaryIP,
		"--sorry-server", sorryIP,
	)
	require.NoError(t, err, "create GSLB")
	created, err := parseGSLB(data)
	require.NoError(t, err)
	gslbID = created.ID
	require.NoError(t, checkGSLBIdentity(created, gslbID, name, gslbDescription),
		"the create response should contain the created GSLB")
	verifyGSLBSettings(t, created, gslbDescription, 10, primaryIP, sorryIP)

	// ヘルスチェックが通るまで DNS の反映を待ちます
	waitForDNS(t, h, created.FQDN, primaryIP)

	// gslb find が作成した GSLB を見つけられることを確認します
	found := findGSLBs(t, h, name)
	require.Len(t, found, 1, "gslb find --name should return only the created GSLB")
	require.NoError(t, checkGSLBIdentity(found[0], gslbID, name, gslbDescription),
		"gslb find should return the created GSLB")
	require.Equal(t, created.FQDN, found[0].FQDN, "gslb find should return the same FQDN")

	// gslb read が作成済みの GSLB を返すことを確認します
	read := readGSLB(t, h, gslbID)
	require.NoError(t, checkGSLBIdentity(read, gslbID, name, gslbDescription),
		"gslb read should return the created GSLB")
	verifyGSLBSettings(t, read, gslbDescription, 10, primaryIP, sorryIP)
	require.Equal(t, created.FQDN, read.FQDN, "gslb read should return the same FQDN")

	// 実サーバーを SSH で停止します。全実サーバーが監視エラーになるとソーリーサーバーへ切り替わります
	_, err = h.RunSSHCommand(t, material, helper.ServerSSHUser, primaryIP, "実サーバーを SSH で停止する",
		"sudo -n systemctl poweroff --no-block")
	require.NoError(t, err, "shut down the primary server")
	helper.WaitForServerStop(t, servers, primaryID, primaryName, serverDescription)
	waitForDNS(t, h, created.FQDN, sorryIP)

	// 実サーバーを起動します。ヘルスチェックが再び通ると元の宛先へ戻ります
	helper.BootServer(t, h, primaryID)
	helper.WaitForServerSSH(t, h, servers, primaryID, primaryName, serverDescription, material)
	waitForDNS(t, h, created.FQDN, primaryIP)

	// GSLB の説明とチェック間隔を更新して、応答で反映を確認します。宛先や監視設定は置き換わりません
	data, _, err = h.CallAllowingFailure(t, "GSLB の説明とチェック間隔を更新する",
		"gslb-api", "update", "--id", gslbID.String(),
		"--description", updatedGSLBDescription, "--delay-loop", "20",
	)
	require.NoError(t, err, "update GSLB")
	updated, err := parseGSLB(data)
	require.NoError(t, err)
	require.NoError(t, checkGSLBIdentity(updated, gslbID, name, updatedGSLBDescription),
		"the update should be reflected in the response")
	verifyGSLBSettings(t, updated, updatedGSLBDescription, 20, primaryIP, sorryIP)
	require.Equal(t, created.FQDN, updated.FQDN, "the update should not change the FQDN")

	// gslb read が更新済みの GSLB を返すことを確認します
	read = readGSLB(t, h, gslbID)
	require.NoError(t, checkGSLBIdentity(read, gslbID, name, updatedGSLBDescription),
		"gslb read should return the updated GSLB")
	verifyGSLBSettings(t, read, updatedGSLBDescription, 20, primaryIP, sorryIP)
}

// sharedIPOfRunningServer は SDK で Server を参照し、共有 NIC の IPv4 アドレスを返します。
func sharedIPOfRunningServer(t *testing.T, servers *serverSDK.Service, id types.ID) string {
	t.Helper()

	item, err := helper.FetchServer(t, servers, id)
	require.NoError(t, err, "read the running Server")
	ip := helper.SharedIPOfServer(item)
	require.NotEmpty(t, ip, "the running Server should have a shared IPv4 address")
	return ip
}

// verifyGSLBSettings は GSLB の応答が期待する設定 (説明・チェック間隔・宛先・ソーリーサーバー) と
// 一致することを確認します。FQDN は設定時に割り当てられるため、空でないことだけを確認します。
func verifyGSLBSettings(t *testing.T, item gslbItem, description string, delayLoop int, primaryIP, sorryIP string) {
	t.Helper()

	require.NotEmpty(t, item.FQDN, "the GSLB should have an FQDN")
	require.Equal(t, description, item.Description, "the GSLB description should match")
	require.Equal(t, delayLoop, item.DelayLoop, "the GSLB should use the requested delay loop")
	require.NotNil(t, item.HealthCheck, "the GSLB should have a health check")
	require.Equal(t, types.GSLBHealthCheckProtocols.Ping, item.HealthCheck.Protocol,
		"the GSLB should use the ping health check")
	require.Len(t, item.DestinationServers, 1, "the GSLB should have exactly one destination server")
	require.Equal(t, primaryIP, item.DestinationServers[0].IPAddress,
		"the GSLB destination should be the primary server")
	require.Equal(t, sorryIP, item.SorryServer, "the GSLB sorry server should be the sorry server IP")
}
