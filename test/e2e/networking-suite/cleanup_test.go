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

// このファイルは Networking Suite E2E の後片付けです。
// 作成したリソースを識別情報の再照合つきで削除し、不在確認まで行います。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// cleanupNetworkingSuite は後片付けです。
// 起動した Server を停止し、作成したリソースを識別情報の再照合つきで削除します。
// 途中の失敗でも残りの後片付けを試み、その失敗を結果に含めます。
// 一致しないリソースは自動削除を拒否し、手動確認が必要なエラーを返します。
func cleanupNetworkingSuite(t *testing.T, h *helper.Helper, servers *serverSDK.Service, disks *diskSDK.Service, run *networkingRun) (result error) {
	t.Helper()

	// 作成応答を失った可能性があるリソースを回収する。名前と属性で厳格に照合します
	if run.group.SRN == "" {
		if err := findTemporaryGroup(t, h, run); err != nil {
			result = errors.Join(result, err)
		}
	}
	for i := 0; i < 2; i++ {
		if run.subnets[i].SRN == "" {
			recovered, err := findTemporarySubnet(t, h, i, run)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			run.subnets[i] = recovered
		}
	}

	// 起動した Server を識別情報の再確認つきで停止する
	stopErr := stopServers(t, h, servers, run)
	if stopErr != nil {
		result = errors.Join(result,
			fmt.Errorf("graceful Server shutdown failed; force-deleting only verified temporary Servers: %w", stopErr))
	} else {
		// 停止に成功した場合だけインターフェース接続を解除する
		for i := 0; i < 2; i++ {
			if run.connects[i].SRN == "" {
				continue
			}
			if _, _, err := h.CallAllowingFailure(t,
				fmt.Sprintf("インターフェース接続%dを解除する", i+1),
				"networking-suite-api", "interface-connection", "delete", run.connects[i].SRN); err != nil {
				result = errors.Join(result, err)
			}
		}
	}

	// Server と接続中の Disk を削除する
	result = errors.Join(result, deleteServers(t, h, servers, disks, run))

	// サブネットとサブネットグループを削除する
	result = errors.Join(result, deleteSubnetsAndGroup(t, h, run))

	return result
}

// stopServers は起動した Server を停止します。
// 停止前に識別情報と NIC を再確認し、変更を検知した場合は停止も接続解除も拒否します。
// 停止の依頼は SSH で行い、停止状態は SDK で確認します。
// SSH の停止依頼が失敗した場合はそのエラーを返し、
// 呼び出し元が識別情報を再確認した上で強制削除に進みます。
func stopServers(t *testing.T, h *helper.Helper, servers *serverSDK.Service, run *networkingRun) error {
	t.Helper()

	// 停止前に識別情報と NIC を再確認する
	for i := 0; i < 2; i++ {
		if run.servers[i].ID.IsEmpty() {
			continue
		}
		item, err := fetchServer(t, servers, run.servers[i].ID)
		if err != nil {
			return fmt.Errorf("cannot verify Server before cleanup: %w", err)
		}
		if err := checkServerIdentity(item, run.servers[i].ID, serverName(run.name, i), description); err != nil {
			return fmt.Errorf("cleanup refused: %w", err)
		}
		if !sameInterfaceIDs(run.servers[i].Interfaces, item.Interfaces) {
			return fmt.Errorf("refusing to stop Server %s after its NICs changed", item.ID)
		}
		run.booted[i] = run.booted[i] || item.InstanceStatus.IsUp()
		if i == 0 && run.publicIP == "" {
			run.publicIP = sharedIPOfServer(item)
		}
	}
	if !run.booted[0] && !run.booted[1] {
		return nil
	}

	// 停止を依頼する。Server B は listener の shutdown を受け取り、Server A は poweroff します
	var shutdownErr error
	if run.booted[1] && run.bAddress != "" && run.publicIP != "" {
		if _, err := h.RunSSHCommand(t, run.material, sshUser, run.publicIP,
			"Server B に停止を依頼する", pythonTCPCommand(run.bAddress, "shutdown")); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("request peer Server shutdown: %w", err))
		}
	}
	if run.booted[0] && run.publicIP != "" {
		if _, err := h.RunSSHCommand(t, run.material, sshUser, run.publicIP,
			"Server A に停止を依頼する",
			"sudo sh -c 'sleep 2; systemctl poweroff' >/dev/null 2>&1 </dev/null &"); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("request entry Server shutdown: %w", err))
		}
	}
	if shutdownErr != nil {
		return shutdownErr
	}

	// 両 Server の停止を待つ。NIC が変更された場合は接続の解除を拒否します
	deadline := time.Now().Add(stopWaitTimeout)
	for time.Now().Before(deadline) {
		allStopped := true
		for i := 0; i < 2; i++ {
			if !run.booted[i] || run.servers[i].ID.IsEmpty() {
				continue
			}
			item, err := fetchServer(t, servers, run.servers[i].ID)
			if err != nil {
				return err
			}
			if !sameInterfaceIDs(run.servers[i].Interfaces, item.Interfaces) {
				return fmt.Errorf("refusing connection cleanup for Server %s after its NICs changed", item.ID)
			}
			if !item.InstanceStatus.IsDown() {
				allStopped = false
			}
		}
		if allStopped {
			run.booted = [2]bool{}
			return nil
		}
		time.Sleep(stopPollingInterval)
	}
	return errors.Join(shutdownErr, errors.New("timed out waiting for temporary Servers to stop"))
}

// deleteServers は Server と接続中の Disk を削除します。
// 削除前に SDK で識別情報・NIC・Disk を再照合し、一致する場合だけ削除します。
// Disk の ID が作成応答と一致しない場合は手動確認が必要なエラーを返し、自動削除を拒否します。
func deleteServers(t *testing.T, h *helper.Helper, servers *serverSDK.Service, disks *diskSDK.Service, run *networkingRun) (result error) {
	t.Helper()

	for i := 0; i < 2; i++ {
		item := run.servers[i]
		if item.ID.IsEmpty() {
			// 作成応答を失った場合は名前と説明で回収する
			recovered, err := findTemporaryServer(t, h, i, run)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			if recovered.ID.IsEmpty() {
				continue // 作成されていなかった
			}
			run.servers[i] = recovered
			item = recovered
		}
		// 削除前に SDK で再確認する
		current, err := fetchServer(t, servers, item.ID)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("cannot verify Server before cleanup: %w", err))
			continue
		}
		if err := checkServerIdentity(current, item.ID, serverName(run.name, i), description); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup refused: Server identity changed: %w", err))
			continue
		}
		if len(current.Disks) != 0 {
			if run.diskIDs[i].IsEmpty() || len(current.Disks) != 1 || current.Disks[0].ID != run.diskIDs[i] {
				result = errors.Join(result, fmt.Errorf(
					"refusing to delete Server %s: Disk ID does not match its create response; manual cleanup required", item.ID))
				continue
			}
			// 接続中 Disk の参照結果が保存した ID と一致することを確認してから削除します
			disk, err := waitForDiskAvailable(t, disks, run.diskIDs[i], current.ID, serverName(run.name, i), run.params)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			if disk.ID != run.diskIDs[i] {
				result = errors.Join(result, fmt.Errorf(
					"refusing to delete Server %s: Disk read returned a different ID", item.ID))
				continue
			}
		} else if !run.diskIDs[i].IsEmpty() {
			result = errors.Join(result, fmt.Errorf(
				"refusing to delete Server %s: created Disk %s is no longer attached; manual cleanup required",
				item.ID, run.diskIDs[i]))
			continue
		}
		if !sameInterfaceIDs(item.Interfaces, current.Interfaces) {
			result = errors.Join(result, fmt.Errorf(
				"refusing to delete Server %s after its NICs changed", item.ID))
			continue
		}
		// --with-disks で接続中の Disk も削除し、--force で電源を止めてから削除します
		if _, _, err := h.CallAllowingFailure(t,
			fmt.Sprintf("テスト用のサーバー%dをディスクとともに削除する", i+1),
			"iaas-api", "server", "delete", "--zone", zone, "--id", item.ID.String(),
			"--with-disks", "--force", "--fail-if-not-found"); err != nil {
			result = errors.Join(result, err)
			continue
		}
		// 削除後の不在を SDK で確認する。他の Server と Disk は削除しません
		matches, err := lookupServersByName(t, servers, serverName(run.name, i))
		if err != nil {
			result = errors.Join(result, fmt.Errorf("verify Server absence after cleanup: %w", err))
			continue
		}
		for _, found := range matches {
			if found.ID == item.ID {
				result = errors.Join(result, fmt.Errorf("temporary Server %s remains after deletion", item.ID))
			}
		}
		diskMatches, err := lookupDisksByName(t, disks, serverName(run.name, i))
		if err != nil {
			result = errors.Join(result, fmt.Errorf("verify Disk absence after cleanup: %w", err))
			continue
		}
		for _, found := range diskMatches {
			if found.Name == serverName(run.name, i) {
				result = errors.Join(result, fmt.Errorf("temporary Disk %s remains after deletion", found.ID))
			}
		}
	}
	return result
}

// deleteSubnetsAndGroup はサブネットとサブネットグループを削除します。
// 削除前に割り当て済みアドレスが残っていないことと識別情報を確認し、
// 一致する場合だけ削除します。
func deleteSubnetsAndGroup(t *testing.T, h *helper.Helper, run *networkingRun) (result error) {
	t.Helper()

	// 依存関係のためサブネットを逆順に削除する
	for i := 1; i >= 0; i-- {
		item := run.subnets[i]
		if item.SRN == "" {
			continue // 回収できなかった場合は後続のグループ削除も拒否されます
		}
		// 割り当て済みアドレスが残っていれば削除しない
		data, _, err := h.CallAllowingFailure(t,
			fmt.Sprintf("サブネット%dの IP アドレスが残っていないことを確認する", i+1),
			"networking-suite-api", "address", "list", item.SRN,
			"--query", "map({SRN,IPAddress,Subnet})",
		)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("list addresses for subnet %s; refusing cleanup: %w", item.SRN, err))
			continue
		}
		addresses, err := decodeList[addressItem](data, "address list")
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if len(addresses) != 0 {
			result = errors.Join(result, fmt.Errorf("subnet %s still has assigned addresses; refusing cleanup", item.SRN))
			continue
		}
		// 識別情報を再確認する。更新後の名前は1つ目のサブネットだけ許可します
		current, err := readSubnetViaCLI(t, h, item.SRN)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if current.SRN != item.SRN {
			result = errors.Join(result, errors.New("refusing to delete subnet after its SRN changed"))
			continue
		}
		expectedName := fmt.Sprintf("%s-subnet-%d", run.name, i+1)
		if i == 0 && current.Name == updatedSubnetName(run.name) {
			expectedName = current.Name
		}
		if err := checkSubnet(current, expectedName, item.IPv4AddressRangeCIDR, run.group.SRN); err != nil {
			result = errors.Join(result, fmt.Errorf("refusing to delete subnet: %w", err))
			continue
		}
		if _, _, err := h.CallAllowingFailure(t,
			fmt.Sprintf("テスト用のサブネット%dを削除する", i+1),
			"networking-suite-api", "subnet", "delete", item.SRN); err != nil {
			result = errors.Join(result, err)
		}
	}

	// グループ配下のサブネットが空になったことを確認してからグループを削除する
	if run.group.SRN == "" {
		return result
	}
	data, _, err := h.CallAllowingFailure(t,
		"サブネットグループのサブネット一覧が空になったことを確認する",
		"networking-suite-api", "subnet", "list", run.group.SRN,
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
	)
	if err != nil {
		return errors.Join(result, err)
	}
	remaining, err := decodeList[subnetItem](data, "subnet list")
	if err != nil {
		return errors.Join(result, err)
	}
	if len(remaining) != 0 {
		return errors.Join(result, errors.New("temporary subnet group still has subnets"))
	}
	current, err := readGroupViaCLI(t, h, run.group.SRN)
	if err != nil {
		return errors.Join(result, err)
	}
	if current.SRN != run.group.SRN {
		return errors.Join(result, errors.New("refusing to delete subnet group after its SRN changed"))
	}
	// 更新後の名前も許可する。更新が反映されていない場合は元の名前と一致します
	if err := checkGroup(current, run.name); err != nil {
		if err := checkGroup(current, updatedGroupName(run.name)); err != nil {
			return errors.Join(result, fmt.Errorf("refusing to delete subnet group: %w", err))
		}
	}
	if _, _, err := h.CallAllowingFailure(t, "テスト用のサブネットグループを削除する",
		"networking-suite-api", "subnet-group", "delete", run.group.SRN); err != nil {
		return errors.Join(result, err)
	}
	// 削除後の不在を確認する。他のグループは削除しません
	groups, err := listGroupsViaCLI(t, h, "削除後のサブネットグループ一覧を確認する")
	if err != nil {
		return errors.Join(result, err)
	}
	for _, found := range groups {
		if found.SRN == run.group.SRN {
			result = errors.Join(result, fmt.Errorf("temporary subnet group %s remains after deletion", run.group.SRN))
		}
	}
	return result
}

// findTemporaryGroup は作成応答を失った可能性があるサブネットグループを
// 名前・説明・CIDR・リージョンの一致で回収します。
// 複数一致する場合は手動確認が必要なエラーを返します。
func findTemporaryGroup(t *testing.T, h *helper.Helper, run *networkingRun) error {
	t.Helper()

	groups, err := listGroupsViaCLI(t, h, "応答を失った可能性があるテスト用サブネットグループを検索する")
	if err != nil {
		return err
	}
	for _, item := range groups {
		nameMatches := item.Name == run.name || item.Name == updatedGroupName(run.name)
		if nameMatches && item.Description == description &&
			item.IPv4AddressRangeCIDR == ipv4GroupCIDR && item.Region.Code == regionCode {
			if run.group.SRN != "" {
				return errors.New("multiple matching temporary subnet groups; manual cleanup required")
			}
			run.group = item
		}
	}
	return nil
}

// findTemporarySubnet は作成応答を失った可能性があるサブネットを属性一致で回収します。
// 1つ目のサブネットは更新後の名前も許可します。
// 複数一致する場合は手動確認が必要なエラーを返します。
func findTemporarySubnet(t *testing.T, h *helper.Helper, index int, run *networkingRun) (subnetItem, error) {
	t.Helper()

	if run.group.SRN == "" {
		return subnetItem{}, nil
	}
	data, _, err := h.CallAllowingFailure(t,
		"応答を失った可能性があるテスト用サブネットを検索する",
		"networking-suite-api", "subnet", "list", run.group.SRN,
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
	)
	if err != nil {
		return subnetItem{}, err
	}
	items, err := decodeList[subnetItem](data, "subnet list")
	if err != nil {
		return subnetItem{}, err
	}
	expectedNames := []string{fmt.Sprintf("%s-subnet-%d", run.name, index+1)}
	if index == 0 {
		expectedNames = append(expectedNames, updatedSubnetName(run.name))
	}
	expectedCIDR := subnetCIDRs[index]
	var found subnetItem
	for _, item := range items {
		nameMatches := false
		for _, name := range expectedNames {
			if item.Name == name {
				nameMatches = true
				break
			}
		}
		if nameMatches && item.Description == description &&
			item.IPv4AddressRangeCIDR == expectedCIDR && item.Zone.Code == zone &&
			item.SubnetGroup.SRN == run.group.SRN {
			if found.SRN != "" {
				return subnetItem{}, errors.New("multiple matching temporary subnets; manual cleanup required")
			}
			found = item
		}
	}
	return found, nil
}

// findTemporaryServer は作成応答を失った可能性がある Server を名前と説明で回収します。
// 複数一致する場合は手動確認が必要なエラーを返します。
func findTemporaryServer(t *testing.T, h *helper.Helper, index int, run *networkingRun) (serverItem, error) {
	t.Helper()

	name := serverName(run.name, index)
	data, _, err := h.CallAllowingFailure(t,
		"応答を失った可能性があるテスト用サーバーを名前で検索する",
		"iaas-api", "server", "find", "--zone", zone, "--name", name,
		"--query", "map({ID,Name,Description,Interfaces})",
	)
	if err != nil {
		return serverItem{}, err
	}
	items, err := decodeList[serverItem](data, "server find")
	if err != nil {
		return serverItem{}, err
	}
	var found serverItem
	for _, item := range items {
		if item.Name == name && item.Description == description && !item.ID.IsEmpty() {
			if !found.ID.IsEmpty() {
				return serverItem{}, errors.New("multiple matching temporary Servers; manual cleanup required")
			}
			found = item
		}
	}
	return found, nil
}

// listGroupsViaCLI は subnet-group list を CLI で実行し、一覧を返します。
func listGroupsViaCLI(t *testing.T, h *helper.Helper, step string) ([]groupItem, error) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, step,
		"networking-suite-api", "subnet-group", "list",
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Region})",
	)
	if err != nil {
		return nil, err
	}
	return decodeList[groupItem](data, "subnet-group list")
}

// readGroupViaCLI は subnet-group read を CLI で実行し、参照結果を返します。
func readGroupViaCLI(t *testing.T, h *helper.Helper, srn string) (groupItem, error) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "サブネットグループを参照して識別情報を確認する",
		"networking-suite-api", "subnet-group", "read", srn,
		"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Region}",
	)
	if err != nil {
		return groupItem{}, err
	}
	var item groupItem
	if err := json.Unmarshal(data, &item); err != nil {
		return groupItem{}, fmt.Errorf("decode subnet-group read: %w", err)
	}
	return item, nil
}

// readSubnetViaCLI は subnet read を CLI で実行し、参照結果を返します。
func readSubnetViaCLI(t *testing.T, h *helper.Helper, srn string) (subnetItem, error) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "サブネットを参照して識別情報を確認する",
		"networking-suite-api", "subnet", "read", srn,
		"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup}",
	)
	if err != nil {
		return subnetItem{}, err
	}
	var item subnetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return subnetItem{}, fmt.Errorf("decode subnet read: %w", err)
	}
	return item, nil
}
