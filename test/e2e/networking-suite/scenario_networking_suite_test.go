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

// このファイルは Networking Suite の E2E シナリオです。
// サブネットグループ、サブネット、インターフェース接続、IP アドレスの操作を検証し、
// 別サブネットに接続した Server 間の TCP 通信を確認します。
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/stretchr/testify/require"

	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioNetworkingSuite は Networking Suite の E2E です。
// ライブ実行には is1b の cloudimg Archive とディスクプランを SDK で自動選択し、
// SAKURA_ENDPOINTS_NETWORKING_SUITE が is1b を指すことが必要です。
// 事前確認と起動待ち・停止待ちのポーリングは SDK で静かに実行し、
// テスト対象の操作は skr CLI で実行して記録に残します。
func TestScenarioNetworkingSuite(t *testing.T) {
	h := helper.New(t, "networking-suite-api",
		"Networking Suite API でサブネットグループ・サブネット・インターフェース接続・IP アドレスの操作と、別サブネット間の TCP 通信が正しく動作することを確認する")

	requireIs1bEndpoint(t)

	// 検証用の IaaS API クライアント。事前確認と起動待ち・停止待ちのポーリングと
	// 削除後の不在確認に使い、証跡には残しません
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create the IaaS API client for verification")
	servers, disks := serverSDK.New(client), diskSDK.New(client)

	// cloudimg Archive とディスクプランを SDK で自動選択する。
	// Archive は Ubuntu Server 24.04 LTS (cloudimg) に限定して選択します
	params := requireNetworkingSuiteParams(t, client)

	run := &networkingRun{
		name:   namePrefix + helper.RandomSuffix(t),
		params: params,
	}

	// 作成前に同名のリソースが存在しないことを確認する。
	// subnet-group list は確認対象の操作でもあるため CLI で実行して記録に残します
	requireNoNameCollision(t, h, run.name)

	// 後片付け: 停止・削除・不在確認まで行い、途中の失敗はテストの結果に含めます
	defer func() {
		if err := cleanupNetworkingSuite(t, h, servers, disks, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	createSubnetGroupAndSubnets(t, h, run)
	createServersAndConnections(t, h, servers, disks, run)
	verifyAddresses(t, h, run)
	bootAndVerifyTCP(t, h, servers, run)
}

// requireNoNameCollision は作成前に同名のリソースが存在しないことを確認します。
// 既存の同名リソースは変更も削除もしません。
func requireNoNameCollision(t *testing.T, h *helper.Helper, name string) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "同名のサブネットグループが存在しないことを確認する",
		"networking-suite-api", "subnet-group", "list",
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Region})",
	)
	require.NoError(t, err, "list subnet groups")
	groups, err := decodeList[groupItem](data, "subnet-group list")
	require.NoError(t, err)
	for _, group := range groups {
		require.NotEqual(t, name, group.Name,
			"test subnet group %q already exists; refusing to modify it", name)
	}
	// is1 で利用できるサブネットグループの上限は3個のため、作成前に空きを確認します
	regionGroups := 0
	for _, group := range groups {
		if group.Region.Code == regionCode {
			regionGroups++
		}
	}
	require.Less(t, regionGroups, 3,
		"is1 already has the maximum of three subnet groups; refusing to create another")

	for i := 0; i < 2; i++ {
		server := serverName(name, i)
		data, _, err := h.CallAllowingFailure(t, "同名のテスト用サーバーが存在しないことを確認する",
			"iaas-api", "server", "find", "--zone", zone, "--name", server,
			"--query", "map({ID,Name,Description})",
		)
		require.NoError(t, err, "find Servers by name")
		items, err := decodeList[serverItem](data, "server find")
		require.NoError(t, err)
		for _, item := range items {
			require.NotEqual(t, server, item.Name,
				"test Server %q already exists; refusing to modify it", server)
		}
	}
}

// createSubnetGroupAndSubnets はサブネットグループとサブネットを作成し、
// 更新と参照を確認します。
func createSubnetGroupAndSubnets(t *testing.T, h *helper.Helper, run *networkingRun) {
	t.Helper()

	// サブネットグループを作成する
	data, _, err := h.CallAllowingFailure(t, "テスト用のサブネットグループを作成する",
		"networking-suite-api", "subnet-group", "create",
		"--name", run.name, "--description", description,
		"--ipv4-address-range-cidr", ipv4GroupCIDR, "--region", regionCode,
	)
	require.NoError(t, err, "create the test subnet group")
	require.NoError(t, json.Unmarshal(data, &run.group), "decode the created subnet group")
	require.NoError(t, checkGroup(run.group, run.name),
		"the subnet group create response should match its create request")

	// サブネットグループの名前を更新する。Update は Name と Description の両方が必須です
	_, _, err = h.CallAllowingFailure(t, "サブネットグループの名前を更新する",
		"networking-suite-api", "subnet-group", "update", run.group.SRN,
		"--name", updatedGroupName(run.name), "--description", description,
	)
	require.NoError(t, err, "update the subnet group")
	run.group.Name = updatedGroupName(run.name)

	// 更新したサブネットグループを参照する
	current, err := readGroupViaCLI(t, h, run.group.SRN)
	require.NoError(t, err, "read the updated subnet group")
	require.Equal(t, run.group.SRN, current.SRN, "subnet-group read should return the same SRN")
	require.NoError(t, checkGroup(current, updatedGroupName(run.name)),
		"subnet-group read should return the updated name")

	// サブネットを2個作成する
	for i := 0; i < 2; i++ {
		subnetName := fmt.Sprintf("%s-subnet-%d", run.name, i+1)
		data, _, err := h.CallAllowingFailure(t,
			fmt.Sprintf("テスト用のサブネット%dを作成する", i+1),
			"networking-suite-api", "subnet", "create",
			"--name", subnetName, "--description", description,
			"--ipv4-address-range-cidr", subnetCIDRs[i], "--zone", zone,
			"--subnet-group-srn", run.group.SRN,
		)
		require.NoError(t, err, "create the test subnet")
		require.NoError(t, json.Unmarshal(data, &run.subnets[i]), "decode the created subnet")
		require.NoError(t, checkSubnet(run.subnets[i], subnetName, subnetCIDRs[i], run.group.SRN),
			"the subnet create response should match its create request")
	}

	// 1つ目のサブネットの名前を更新する
	_, _, err = h.CallAllowingFailure(t, "1つ目のサブネットの名前を更新する",
		"networking-suite-api", "subnet", "update", run.subnets[0].SRN,
		"--name", updatedSubnetName(run.name), "--description", description,
	)
	require.NoError(t, err, "update the subnet")
	run.subnets[0].Name = updatedSubnetName(run.name)

	// サブネット一覧を参照する
	data, _, err = h.CallAllowingFailure(t, "サブネットグループのサブネット一覧を参照する",
		"networking-suite-api", "subnet", "list", run.group.SRN,
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
	)
	require.NoError(t, err, "list subnets")
	subnets, err := decodeList[subnetItem](data, "subnet list")
	require.NoError(t, err)
	require.True(t, containsSubnet(subnets, run.subnets[0]),
		"subnet list should return the updated subnet")
	require.True(t, containsSubnet(subnets, run.subnets[1]),
		"subnet list should return the second subnet")

	// 更新したサブネットを参照する
	currentSubnet, err := readSubnetViaCLI(t, h, run.subnets[0].SRN)
	require.NoError(t, err, "read the updated subnet")
	require.NoError(t, checkSubnet(currentSubnet, updatedSubnetName(run.name), subnetCIDRs[0], run.group.SRN),
		"subnet read should return the updated name")
}

// createServersAndConnections は Server A (共有 NIC + 未接続 NIC) と Server B (未接続 NIC) を作成し、
// 未接続 NIC をそれぞれのサブネットへ接続します。
func createServersAndConnections(t *testing.T, h *helper.Helper, servers *serverSDK.Service, disks *diskSDK.Service, run *networkingRun) {
	t.Helper()

	for i := 0; i < 2; i++ {
		name := serverName(run.name, i)
		// Server を cloudimg Archive から作成する。
		// NIC は ADR 0041 の反復可能オプションで指定します
		args := []string{
			"iaas-api", "server", "create", "--zone", zone,
			"--name", name, "--description", description,
			"--cpu", "1", "--memory-gb", "1",
			"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d",
				run.params.ArchiveID, run.params.DiskPlanID, run.params.SizeGB),
		}
		args = append(args, serverNICArgs(i == 0)...)
		data := h.CallWithTimeout(t, serverCallTimeout,
			"テスト用のサーバーを cloudimg Archive から作成する", args...)
		require.NoError(t, json.Unmarshal(data, &run.servers[i]), "decode the created Server")
		require.False(t, run.servers[i].ID.IsEmpty(),
			"the create response should contain the created Server")
		require.Equal(t, name, run.servers[i].Name,
			"the create response should match the temporary name")
		require.Equal(t, description, run.servers[i].Description)
		require.Len(t, run.servers[i].Disks, 1,
			"the create response should identify its Disk")
		require.False(t, run.servers[i].Disks[0].ID.IsEmpty(),
			"the create response should identify its Disk ID")
		// Disk の ID は作成応答から保存し、後続の参照では上書きしません
		run.diskIDs[i] = run.servers[i].Disks[0].ID

		// server read で作成結果と NIC を確認する。
		// NIC の MAC アドレスは user-data の netplan 設定に使います
		data = h.Call(t, "作成したサーバーを参照する",
			"iaas-api", "server", "read", "--zone", zone, "--id", run.servers[i].ID.String(),
			"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}",
		)
		var current serverItem
		require.NoError(t, json.Unmarshal(data, &current), "decode the created Server")
		require.NoError(t, checkServerIdentity(current, run.servers[i].ID, name, description),
			"server read should return the created Server")
		require.NotEmpty(t, current.Interfaces, "the Server should have its NICs")
		run.servers[i] = current

		// Disk が cloudimg Archive・DiskPlan・Server・サイズと一致することを照合しながら
		// 利用可能になるまで待つ
		_, err := waitForDiskAvailable(t, disks, run.diskIDs[i], run.servers[i].ID, name, run.params)
		require.NoError(t, err, "wait for the Disk to become available")

		// 未接続 NIC をサブネットへ接続する
		nic, err := findInterface(run.servers[i], "disconnected")
		require.NoError(t, err, "find the disconnected NIC")
		data = h.Call(t, fmt.Sprintf("未接続 NIC をサブネット%dへ接続する", i+1),
			"networking-suite-api", "interface-connection", "create",
			"--interface-srn", interfaceSRN(zone, nic.ID), "--subnet-srn", run.subnets[i].SRN,
		)
		require.NoError(t, json.Unmarshal(data, &run.connects[i]), "decode the interface connection")
		require.NotEmpty(t, run.connects[i].SRN,
			"the interface connection should return its SRN")
		require.NotNil(t, net.ParseIP(run.connects[i].EphemeralIPv4Address),
			"the interface connection should return its IPv4 address")
	}
	run.bAddress = run.connects[1].EphemeralIPv4Address
}

// verifyAddresses は Address API で割り当てられた IPv4 アドレスを確認します。
func verifyAddresses(t *testing.T, h *helper.Helper, run *networkingRun) {
	t.Helper()

	for i := 0; i < 2; i++ {
		data := h.Call(t, fmt.Sprintf("サブネット%dに割り当てられた IP アドレスを検索する", i+1),
			"networking-suite-api", "address", "list", run.subnets[i].SRN,
			"--interface-connection-srn", run.connects[i].SRN,
			"--query", "map({SRN,IPAddress,Subnet})",
		)
		addresses, err := decodeList[addressItem](data, "address list")
		require.NoError(t, err)
		require.Len(t, addresses, 1,
			"the subnet should have exactly one assigned address")
		require.True(t, containsAddress(addresses, run.connects[i], run.subnets[i].SRN),
			"address list should return the interface connection address")

		data = h.Call(t, "割り当てられた IP アドレスを参照する",
			"networking-suite-api", "address", "read", addresses[0].SRN,
			"--query", "{SRN,IPAddress,Subnet}",
		)
		var address addressItem
		require.NoError(t, json.Unmarshal(data, &address), "decode the address")
		require.Equal(t, run.connects[i].EphemeralIPv4Address, address.IPAddress,
			"the address should match the interface connection address")
	}
}

// bootAndVerifyTCP は user-data を指定して両 Server を起動し、Server A の共有側 IP へ SSH 接続して、
// 別サブネットの Server B へ TCP 接続します。
func bootAndVerifyTCP(t *testing.T, h *helper.Helper, servers *serverSDK.Service, run *networkingRun) {
	t.Helper()

	// 一時的な SSH 鍵を作成する。user-data は Networking Suite NIC の netplan 設定が必要なため
	// 専用の内容を書き込み、helper が生成した user-data は使いません
	material := helper.WriteSSHMaterial(t, sshUser, false)
	run.material = material
	tempDir := filepath.Dir(material.PrivateKeyPath)
	publicKeyData, err := os.ReadFile(material.PrivateKeyPath + ".pub")
	require.NoError(t, err, "read the temporary public SSH key")
	publicKey := strings.TrimSpace(string(publicKeyData))
	entryNIC, err := findInterface(run.servers[0], "disconnected")
	require.NoError(t, err, "find the entry Server's Networking Suite NIC")
	entryUserDataContent, err := entryUserData(publicKey, entryNIC.MACAddress)
	require.NoError(t, err, "build the entry Server user-data")
	entryPath := filepath.Join(tempDir, "entry-user-data.yaml")
	peerPath := filepath.Join(tempDir, "peer-user-data.yaml")
	require.NoError(t, os.WriteFile(entryPath, []byte(entryUserDataContent), 0o600),
		"write the entry Server user-data")
	require.NoError(t, os.WriteFile(peerPath, []byte(peerUserData), 0o600),
		"write the peer Server user-data")

	// user-data を指定して両 Server を起動する
	for i, path := range []string{entryPath, peerPath} {
		h.Call(t, fmt.Sprintf("user-data を指定してサーバー%dを起動する", i+1),
			"iaas-api", "server", "boot", "--zone", zone, "--id", run.servers[i].ID.String(),
			"--user-data", "@"+path,
		)
		run.booted[i] = true
	}

	// 起動を確認しながら SSH ログインできるまで待つ
	run.publicIP = waitEntrySSH(t, h, servers, run)

	// 別サブネットの Server B の listener へ TCP 接続する。
	// cloud-init の反映を待つため再試行します
	var response string
	for attempt := 0; attempt < tcpRetryAttempts; attempt++ {
		stdout, err := h.RunSSHCommand(t, material, sshUser, run.publicIP,
			"Server A から別サブネットの Server B へ TCP 接続する",
			pythonTCPCommand(run.bAddress, "ping"))
		if err == nil {
			response = strings.TrimSpace(stdout)
			break
		}
		t.Logf("TCP connection attempt %d failed, retrying: %v", attempt+1, err)
		time.Sleep(tcpRetryInterval)
	}
	require.Equal(t, "PONG networking-suite", response,
		"the peer Server should respond over the cross-subnet TCP connection")
}

// waitEntrySSH は SSH ログインできるまで Server の起動を確認し続けます。
// NIC が変更された場合は SSH 接続を拒否します。ポーリングは証跡に残しません。
func waitEntrySSH(t *testing.T, h *helper.Helper, servers *serverSDK.Service, run *networkingRun) string {
	t.Helper()

	deadline := time.Now().Add(sshWaitTimeout)
	for {
		item, err := fetchServer(t, servers, run.servers[0].ID)
		require.NoError(t, err, "read the entry Server while waiting for SSH login")
		require.NoError(t, checkServerIdentity(item, run.servers[0].ID, serverName(run.name, 0), description),
			"the entry Server should keep its identity while booting")
		require.True(t, sameInterfaceIDs(run.servers[0].Interfaces, item.Interfaces),
			"refusing SSH access after the entry Server's NICs changed")
		if item.InstanceStatus.IsUp() {
			if ip := sharedIPOfServer(item); ip != "" {
				if err := h.ConnectSSH(t, run.material, sshUser, ip, "SSH ログインを確認する"); err == nil {
					return ip
				}
				t.Logf("SSH login attempt to %s failed, retrying: %v", ip, err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("SSH login check timed out after %s", sshWaitTimeout)
		}
		time.Sleep(sshPollingInterval)
	}
}
