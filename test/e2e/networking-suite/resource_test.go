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

// このファイルは Networking Suite E2E の共有ヘルパーです。
// 参照結果の型、識別情報の照合、環境変数の解析、SSH 接続の補助を提供します。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// zone は Networking Suite E2E で使用するゾーンです。
	zone = "is1b"

	// namePrefix はテスト用リソースの名前の接頭辞です。
	// ランダムな接尾辞とともに使い、既存のリソースと区別できるようにします。
	namePrefix = "skr-e2e-networking-suite-"

	// description はテスト用リソースに付ける固定の説明です。
	// 後片付けで今回の実行が作成したリソースを識別するために使います。
	description = "Temporary Networking Suite E2E resource"

	// sshUser は cloudimg Archive の既定ユーザーです。
	sshUser = "ubuntu"

	// tcpPort は Server B の TCP listener のポート番号です。
	tcpPort = 38172

	// ipv4GroupCIDR はテスト用サブネットグループの IPv4 アドレス範囲です。
	ipv4GroupCIDR = "10.0.0.0/16"

	// regionCode はテスト用サブネットグループのリージョンです。
	regionCode = "is1"
)

// subnetCIDRs はテスト用サブネット2個の IPv4 アドレス範囲です。
// グループの範囲と重ならず、既存のネットワークとも重ならない値を使います。
var subnetCIDRs = [2]string{"10.0.1.0/24", "10.0.2.0/24"}

const (
	// findPageSize は名前検索の1回の取得件数です。
	findPageSize = 100

	// diskWaitTimeout は Disk が利用可能になるまでの期限です。
	diskWaitTimeout = 10 * time.Minute

	// diskPollingInterval は Disk の参照間隔です。
	diskPollingInterval = 10 * time.Second

	// sshWaitTimeout は SSH ログイン確認の期限です。
	// cloud-init の反映を待つため、runbook の SLA より長めに取ります。
	sshWaitTimeout = 20 * time.Minute

	// sshPollingInterval は起動と SSH ログインの再確認間隔です。
	sshPollingInterval = 10 * time.Second

	// tcpRetryAttempts は別サブネット間 TCP 接続の試行回数です。
	tcpRetryAttempts = 30

	// tcpRetryInterval は別サブネット間 TCP 接続の再試行間隔です。
	tcpRetryInterval = 5 * time.Second

	// stopWaitTimeout は Server の停止を待つ期限です。
	stopWaitTimeout = 5 * time.Minute

	// stopPollingInterval は Server の停止を確認する間隔です。
	stopPollingInterval = 5 * time.Second

	// serverCallTimeout は Server の作成と削除に許す CLI タイムアウトです。
	serverCallTimeout = 15 * time.Minute
)

// networkingSuiteParams は Networking Suite E2E で自動選択した cloudimg Archive とディスクプランの設定です。
type networkingSuiteParams struct {
	// ArchiveID は自動選択した Ubuntu Server 24.04 LTS (cloudimg) Archive の ID です。
	ArchiveID int64

	// DiskPlanID は自動選択したディスクプランの ID です。
	DiskPlanID int64

	// SizeGB はクラウドディスクのサイズ (GB) です。
	SizeGB int64
}

// requireNetworkingSuiteParams はディスクサイズの環境変数を読み取り、
// cloudimg Archive とディスクプランを SDK で自動選択します。
// 環境変数は SKR_E2E_NETWORKING_SUITE_DISK_SIZE_GB (既定 20) です。
// Archive とディスクプランの選択は E2E の事前確認のため SDK で静かに実行します。
func requireNetworkingSuiteParams(t *testing.T, caller iaas.APICaller) networkingSuiteParams {
	t.Helper()

	sizeGB, err := parseDiskSizeGB(os.Getenv("SKR_E2E_NETWORKING_SUITE_DISK_SIZE_GB"))
	require.NoError(t, err, "parse SKR_E2E_NETWORKING_SUITE_DISK_SIZE_GB")
	archive := helper.FindCloudimgArchive(t, caller, zone)
	plan := helper.FindDiskPlan(t, caller, zone, sizeGB)
	return networkingSuiteParams{
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
		return 0, errors.New("SKR_E2E_NETWORKING_SUITE_DISK_SIZE_GB must be a positive integer")
	}
	return size, nil
}

// requireIs1bEndpoint は Networking Suite の endpoint が is1b を指すことを確認します。
// 既定 endpoint は is1c のため、SAKURA_ENDPOINTS_NETWORKING_SUITE が未設定の場合はテストをスキップします。
func requireIs1bEndpoint(t *testing.T) {
	t.Helper()

	endpoint := os.Getenv("SAKURA_ENDPOINTS_NETWORKING_SUITE")
	if endpoint == "" {
		t.Skipf("SAKURA_ENDPOINTS_NETWORKING_SUITE is not set, skipping E2E test")
	}
	require.Contains(t, endpoint, "/zone/is1b/",
		"SAKURA_ENDPOINTS_NETWORKING_SUITE must point to the is1b API endpoint")
}

// groupItem はサブネットグループの参照結果です。
type groupItem struct {
	SRN                  string `json:"SRN"`
	Name                 string `json:"Name"`
	Description          string `json:"Description"`
	IPv4AddressRangeCIDR string `json:"IPv4AddressRangeCIDR"`
	Region               struct {
		Code string `json:"Code"`
	} `json:"Region"`
}

// subnetItem はサブネットの参照結果です。
type subnetItem struct {
	SRN                  string `json:"SRN"`
	Name                 string `json:"Name"`
	Description          string `json:"Description"`
	IPv4AddressRangeCIDR string `json:"IPv4AddressRangeCIDR"`
	Zone                 struct {
		Code string `json:"Code"`
	} `json:"Zone"`
	SubnetGroup struct {
		SRN string `json:"SRN"`
	} `json:"SubnetGroup"`
}

// connectionItem はインターフェース接続の作成結果です。
type connectionItem struct {
	SRN                  string `json:"SRN"`
	EphemeralIPv4Address string `json:"EphemeralIPv4Address"`
}

// addressItem は割り当てられた IPv4 アドレスの参照結果です。
type addressItem struct {
	SRN       string `json:"SRN"`
	IPAddress string `json:"IPAddress"`
	Subnet    struct {
		SRN string `json:"SRN"`
	} `json:"Subnet"`
}

// serverInterface は Server の NIC の参照結果です。
type serverInterface struct {
	ID           types.ID `json:"ID"`
	IPAddress    string   `json:"IPAddress"`
	MACAddress   string   `json:"MACAddress"`
	UpstreamType string   `json:"UpstreamType"`
}

// diskItem は接続中 Disk の参照結果です。Server の作成応答と Disk の参照の両方で使います。
type diskItem struct {
	ID              types.ID `json:"ID"`
	Name            string   `json:"Name"`
	ServerID        types.ID `json:"ServerID"`
	DiskPlanID      types.ID `json:"DiskPlanID"`
	SourceArchiveID types.ID `json:"SourceArchiveID"`
	SizeMB          int64    `json:"SizeMB"`
	Availability    string   `json:"Availability"`
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

// networkingRun は1回の Networking Suite E2E 実行の状態と設定です。
type networkingRun struct {
	// name は今回の実行が作成する一意なサブネットグループの名前です。
	name string

	// params は cloudimg Archive とディスクプランの設定です。
	params networkingSuiteParams

	// material は SSH 接続確認に使う一時的な鍵です。起動後に設定されます。
	material helper.SSHMaterial

	// group は作成したサブネットグループです。
	group groupItem

	// subnets は作成した2個のサブネットです。
	subnets [2]subnetItem

	// servers は作成した Server (entry, peer) です。
	servers [2]serverItem

	// diskIDs は Server の作成応答から保存した Disk の ID です。
	// 後続の参照では上書きしないため、参照応答の欠損を検出できます。
	diskIDs [2]types.ID

	// connects は作成したインターフェース接続です。
	connects [2]connectionItem

	// booted は Server を起動したかどうかです。停止を確認できたら偽に戻します。
	booted [2]bool

	// publicIP は Server A の共有 NIC の IPv4 アドレスです。SSH 接続に使います。
	publicIP string

	// bAddress は Server B の Networking Suite 側 IPv4 アドレスです。TCP 接続先に使います。
	bAddress string
}

// serverName はインデックスに対応するテスト用 Server の名前を返します。
func serverName(name string, index int) string {
	if index == 0 {
		return name + "-entry"
	}
	return name + "-peer"
}

// updatedGroupName は更新確認に使うサブネットグループの名前です。
func updatedGroupName(name string) string {
	return name + "-updated"
}

// updatedSubnetName は更新確認に使う1つ目のサブネットの名前です。
func updatedSubnetName(name string) string {
	return name + "-subnet-1-updated"
}

// interfaceSRN は未接続 NIC の SRN を返します。
// Networking Suite はこの SRN を操作対象の NIC として扱います。
func interfaceSRN(zoneName string, interfaceID types.ID) string {
	return fmt.Sprintf("srnv1:sakura-%s:sakura.iaas.interface:%s", zoneName, interfaceID)
}

// checkGroup はサブネットグループの参照結果が期待する内容と一致することを確認します。
func checkGroup(group groupItem, name string) error {
	if group.SRN == "" {
		return errors.New("subnet group response did not contain its SRN")
	}
	if group.Name != name {
		return fmt.Errorf("subnet group response returned Name %q, want %q", group.Name, name)
	}
	if group.Description != description {
		return fmt.Errorf("subnet group response returned Description %q, want %q", group.Description, description)
	}
	if group.IPv4AddressRangeCIDR != ipv4GroupCIDR {
		return fmt.Errorf("subnet group response returned IPv4AddressRangeCIDR %q, want %q", group.IPv4AddressRangeCIDR, ipv4GroupCIDR)
	}
	if group.Region.Code != regionCode {
		return fmt.Errorf("subnet group response returned Region %q, want %q", group.Region.Code, regionCode)
	}
	return nil
}

// checkSubnet はサブネットの参照結果が期待する内容と一致することを確認します。
func checkSubnet(subnet subnetItem, name, cidr, groupSRN string) error {
	if subnet.SRN == "" {
		return errors.New("subnet response did not contain its SRN")
	}
	if subnet.Name != name {
		return fmt.Errorf("subnet response returned Name %q, want %q", subnet.Name, name)
	}
	if subnet.Description != description {
		return fmt.Errorf("subnet response returned Description %q, want %q", subnet.Description, description)
	}
	if subnet.IPv4AddressRangeCIDR != cidr {
		return fmt.Errorf("subnet response returned IPv4AddressRangeCIDR %q, want %q", subnet.IPv4AddressRangeCIDR, cidr)
	}
	if subnet.Zone.Code != zone {
		return fmt.Errorf("subnet response returned Zone %q, want %q", subnet.Zone.Code, zone)
	}
	if groupSRN != "" && subnet.SubnetGroup.SRN != groupSRN {
		return fmt.Errorf("subnet response returned SubnetGroup %q, want %q", subnet.SubnetGroup.SRN, groupSRN)
	}
	return nil
}

// checkServerIdentity は Server の参照結果が期待する Server であることを確認します。
func checkServerIdentity(server serverItem, id types.ID, name, expectedDescription string) error {
	if server.ID.IsEmpty() {
		return errors.New("server response did not contain its ID")
	}
	if server.ID != id {
		return fmt.Errorf("server response returned ID %s, want %s", server.ID, id)
	}
	if server.Name != name {
		return fmt.Errorf("server response returned Name %q, want %q", server.Name, name)
	}
	if server.Description != expectedDescription {
		return fmt.Errorf("server response returned Description %q, want %q", server.Description, expectedDescription)
	}
	return nil
}

// checkDisk は Disk が期待する cloudimg Archive、DiskPlan、Server、サイズと一致することを確認します。
func checkDisk(disk diskItem, serverID types.ID, serverName string, params networkingSuiteParams) error {
	if disk.ID.IsEmpty() {
		return errors.New("disk response did not contain its ID")
	}
	if disk.Name != serverName {
		return fmt.Errorf("disk response returned Name %q, want %q", disk.Name, serverName)
	}
	if disk.ServerID != serverID {
		return fmt.Errorf("disk response returned ServerID %s, want %s", disk.ServerID, serverID)
	}
	if disk.DiskPlanID != types.ID(params.DiskPlanID) {
		return fmt.Errorf("disk response returned DiskPlanID %s, want %d", disk.DiskPlanID, params.DiskPlanID)
	}
	if disk.SourceArchiveID != types.ID(params.ArchiveID) {
		return fmt.Errorf("disk response returned SourceArchiveID %s, want %d", disk.SourceArchiveID, params.ArchiveID)
	}
	if disk.SizeMB != params.SizeGB*1024 {
		return fmt.Errorf("disk response returned SizeMB %d, want %d", disk.SizeMB, params.SizeGB*1024)
	}
	return nil
}

// sameInterfaceIDs は NIC 一覧の ID が一致することを確認します。
// 順序も含めて比較し、1個でも欠けている場合は偽を返します。
func sameInterfaceIDs(expected, actual []serverInterface) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i, iface := range expected {
		if iface.ID != actual[i].ID {
			return false
		}
	}
	return true
}

// findInterface は NIC の種類に対応する未接続 NIC を探します。
// "disconnected" は UpstreamType "none" の NIC を表します。
func findInterface(server serverItem, upstreamType string) (serverInterface, error) {
	expected := ""
	switch upstreamType {
	case "none", "disconnected":
		expected = "none"
	case "shared":
		expected = "shared"
	default:
		return serverInterface{}, fmt.Errorf("unknown NIC upstream type %q", upstreamType)
	}
	var found *serverInterface
	for i, iface := range server.Interfaces {
		if !strings.EqualFold(iface.UpstreamType, expected) {
			continue
		}
		if found != nil {
			return serverInterface{}, fmt.Errorf("expected one %s NIC, found multiple", upstreamType)
		}
		found = &server.Interfaces[i]
	}
	if found == nil {
		return serverInterface{}, fmt.Errorf("expected one %s NIC, found none", upstreamType)
	}
	return *found, nil
}

// sharedIPOfServer は共有 NIC に割り当てられた IPv4 アドレスを返します。
// 共有 NIC がなければ空文字を返します。
func sharedIPOfServer(server serverItem) string {
	for _, iface := range server.Interfaces {
		if !strings.EqualFold(iface.UpstreamType, "shared") {
			continue
		}
		if net.ParseIP(iface.IPAddress) != nil {
			return iface.IPAddress
		}
	}
	return ""
}

// containsAddress は割り当てられたアドレス一覧に期待するアドレスが含まれるかどうかを返します。
func containsAddress(addresses []addressItem, connection connectionItem, subnetSRN string) bool {
	for _, address := range addresses {
		if address.IPAddress == connection.EphemeralIPv4Address && address.Subnet.SRN == subnetSRN {
			return true
		}
	}
	return false
}

// containsSubnet はサブネット一覧に期待するサブネットが含まれるかどうかを返します。
func containsSubnet(subnets []subnetItem, subnet subnetItem) bool {
	for _, item := range subnets {
		if item.SRN == subnet.SRN && item.Name == subnet.Name {
			return true
		}
	}
	return false
}

// decodeList は CLI の一覧応答を配列として解析します。
// null や非配列の応答はエラーにします。
func decodeList[T any](data []byte, what string) ([]T, error) {
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode %s: %w", what, err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array for " + what)
	}
	return items, nil
}

// serverNICArgs は Server の作成に使う --nic オプションを返します。
// Server A は共有 NIC と未接続 NIC、Server B は未接続 NIC だけを持ちます。
func serverNICArgs(withSharedNIC bool) []string {
	if withSharedNIC {
		return []string{"--nic", "upstream=shared", "--nic", "upstream=disconnected"}
	}
	return []string{"--nic", "upstream=disconnected"}
}

// entryUserData は Server A の cloud-init user-data を返します。
// 既定ユーザーに公開鍵を置き、Networking Suite NIC を MAC アドレスで DHCP に固定します。
// cloudimg Archive はユーザーを作り直さないため、users は default のままにします。
// DHCP の既定経路が共有 NIC の既定経路より優先されないよう、メトリックを 200 にします。
func entryUserData(publicKey, nwsNICMAC string) (string, error) {
	mac, err := net.ParseMAC(nwsNICMAC)
	if err != nil {
		return "", fmt.Errorf("invalid Networking Suite NIC MAC address %q: %w", nwsNICMAC, err)
	}
	return fmt.Sprintf(`#cloud-config
hostname: nstest6
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
packages:
  - openssh-server
users:
  - default
ssh_authorized_keys:
  - %s
write_files:
  - path: /etc/netplan/60-networking-suite.yaml
    permissions: "0600"
    content: |
      network:
        version: 2
        ethernets:
          networking-suite:
            match:
              macaddress: "%s"
            dhcp4: true
            dhcp4-overrides:
              route-metric: 200
runcmd:
  - [netplan, generate]
  - [netplan, apply]
`, publicKey, strings.ToLower(mac.String())), nil
}

// peerUserData は Server B の cloud-init user-data です。
// TCP `tcpPort` で待ち受ける listener を systemd サービスとして起動し、
// "ping" を受け取ると "PONG networking-suite" を返します。
// "shutdown" を受け取ると Server を停止します。
const peerUserData = `#cloud-config
hostname: nstest6-peer
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
write_files:
  - path: /usr/local/bin/skr-networking-suite-listener.py
    permissions: "0755"
    content: |
      #!/usr/bin/python3
      import socket
      import subprocess

      with socket.socket() as listener:
          listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
          listener.bind(("0.0.0.0", 38172))
          listener.listen()
          while True:
              conn, _ = listener.accept()
              with conn:
                  command = conn.makefile("r").readline().strip()
                  if command == "ping":
                      conn.sendall(b"PONG networking-suite\n")
                  elif command == "shutdown":
                      conn.sendall(b"stopping\n")
                      subprocess.Popen(["/usr/bin/systemctl", "poweroff"])
                      break
  - path: /etc/systemd/system/skr-networking-suite-listener.service
    permissions: "0644"
    content: |
      [Unit]
      Description=Temporary Networking Suite E2E listener
      After=network-online.target
      [Service]
      ExecStart=/usr/bin/python3 /usr/local/bin/skr-networking-suite-listener.py
      Restart=always
      [Install]
      WantedBy=multi-user.target
runcmd:
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, skr-networking-suite-listener.service]
`

// pythonTCPCode は SSH で実行する Python のコードを返します。
// TCP 接続先は Networking Suite の IPv4 アドレス `ip` のポート `port` です。
// `command` を送り、1行の応答を標準出力に書き出します。接続は 10 秒でタイムアウトします。
func pythonTCPCode(ip string, port int, command string) string {
	return fmt.Sprintf(
		"import socket; s=socket.create_connection((%q,%d),timeout=10); s.sendall(%q.encode()); r=s.makefile('rb'); print(r.readline().decode().strip()); s.close()",
		ip, port, command+"\n",
	)
}

// pythonTCPCommandAt は SSH で実行するコマンドを返します。
// ポートとデータを指定して Python で TCP 接続します。
func pythonTCPCommandAt(ip string, port int, command string) string {
	return "python3 -c " + shellQuote(pythonTCPCode(ip, port, command))
}

// pythonTCPCommand は SSH で実行するコマンドを返します。
// TCP 接続先は Networking Suite の IPv4 アドレス `ip` の `tcpPort` です。
func pythonTCPCommand(ip, command string) string {
	return pythonTCPCommandAt(ip, tcpPort, command)
}

// shellQuote はシェルの単一引用符で囲んだ文字列にします。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// fetchServer は SDK で Server を参照します。
// 起動待ち・停止待ちのポーリングと後片付けの事前確認に使い、証跡には残しません。
func fetchServer(t *testing.T, servers *serverSDK.Service, id types.ID) (serverItem, error) {
	t.Helper()

	server, err := servers.ReadWithContext(t.Context(), &serverSDK.ReadRequest{Zone: zone, ID: id})
	if err != nil {
		return serverItem{}, fmt.Errorf("read Server %s: %w", id, err)
	}
	return serverFromSDK(server), nil
}

// lookupServersByName は SDK で名前検索を行い、結果を参照結果の型で返します。
// 削除後の不在確認に使い、証跡には残しません。
func lookupServersByName(t *testing.T, servers *serverSDK.Service, name string) ([]serverItem, error) {
	t.Helper()

	items, err := paginateFind(func(ctx context.Context, from int) ([]serverItem, error) {
		found, err := servers.FindWithContext(ctx, &serverSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]serverItem, 0, len(found))
		for _, server := range found {
			if server == nil {
				continue
			}
			items = append(items, serverFromSDK(server))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Servers by name: %w", err)
	}
	return items, nil
}

// lookupDisksByName は SDK で名前検索を行い、結果を参照結果の型で返します。
// 削除後の不在確認に使い、証跡には残しません。
func lookupDisksByName(t *testing.T, disks *diskSDK.Service, name string) ([]diskItem, error) {
	t.Helper()

	items, err := paginateFind(func(ctx context.Context, from int) ([]diskItem, error) {
		found, err := disks.FindWithContext(ctx, &diskSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]diskItem, 0, len(found))
		for _, disk := range found {
			if disk == nil {
				continue
			}
			items = append(items, diskFromSDK(disk))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Disks by name: %w", err)
	}
	return items, nil
}

// waitForDiskAvailable は SDK で Disk を参照し、
// cloudimg Archive、DiskPlan、Server、サイズを照合しながら利用可能になるまで待ちます。
// ポーリングは証跡に残しません。
func waitForDiskAvailable(t *testing.T, disks *diskSDK.Service, diskID, serverID types.ID, name string, params networkingSuiteParams) (diskItem, error) {
	t.Helper()

	deadline := time.Now().Add(diskWaitTimeout)
	for {
		disk, err := disks.ReadWithContext(t.Context(), &diskSDK.ReadRequest{Zone: zone, ID: diskID})
		if err != nil {
			return diskItem{}, fmt.Errorf("read Disk %s: %w", diskID, err)
		}
		item := diskFromSDK(disk)
		if err := checkDisk(item, serverID, name, params); err != nil {
			return diskItem{}, err
		}
		if item.Availability == "available" {
			return item, nil
		}
		if time.Now().After(deadline) {
			return diskItem{}, fmt.Errorf("Disk %s did not become available within %s", diskID, diskWaitTimeout)
		}
		select {
		case <-t.Context().Done():
			return diskItem{}, t.Context().Err()
		case <-time.After(diskPollingInterval):
		}
	}
}

// serverFromSDK は SDK の Server を参照結果の型へ変換します。
func serverFromSDK(server *iaas.Server) serverItem {
	item := serverItem{
		ID:             server.ID,
		Name:           server.Name,
		Description:    server.Description,
		InstanceStatus: server.InstanceStatus,
	}
	for _, iface := range server.Interfaces {
		if iface == nil {
			continue
		}
		item.Interfaces = append(item.Interfaces, serverInterface{
			ID:           iface.ID,
			IPAddress:    iface.IPAddress,
			MACAddress:   iface.MACAddress,
			UpstreamType: string(iface.UpstreamType),
		})
	}
	for _, disk := range server.Disks {
		if disk == nil {
			continue
		}
		item.Disks = append(item.Disks, diskItem{ID: disk.ID, Name: disk.Name})
	}
	return item
}

// diskFromSDK は SDK の Disk を Disk の参照結果の型へ変換します。
func diskFromSDK(disk *iaas.Disk) diskItem {
	return diskItem{
		ID:              disk.ID,
		Name:            disk.Name,
		ServerID:        disk.ServerID,
		DiskPlanID:      disk.DiskPlanID,
		SourceArchiveID: disk.SourceArchiveID,
		SizeMB:          int64(disk.SizeMB),
		Availability:    string(disk.Availability),
	}
}

// paginateFind は SDK の検索をページネーションで続け、全結果を返します。
func paginateFind[T any](fetch func(ctx context.Context, from int) ([]T, error)) ([]T, error) {
	var all []T
	for from := 0; ; from += findPageSize {
		items, err := fetch(context.Background(), from)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if len(items) < findPageSize {
			break
		}
	}
	return all, nil
}
