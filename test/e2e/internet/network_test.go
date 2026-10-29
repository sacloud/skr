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
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// subnetItem はサブネットの参照結果です。
type subnetItem struct {
	ID             types.ID          `json:"ID"`
	SwitchID       types.ID          `json:"SwitchID"`
	InternetID     types.ID          `json:"InternetID"`
	DefaultRoute   string            `json:"DefaultRoute"`
	NextHop        string            `json:"NextHop"`
	StaticRoute    string            `json:"StaticRoute"`
	NetworkAddress string            `json:"NetworkAddress"`
	NetworkMaskLen int               `json:"NetworkMaskLen"`
	IPAddresses    []subnetIPAddress `json:"IPAddresses"`
}

// subnetIPAddress はサブネットに割り当て可能な IP アドレスの候補です。
type subnetIPAddress struct {
	HostName  string `json:"HostName"`
	IPAddress string `json:"IPAddress"`
}

// listSubnets は skr CLI でルータ＋スイッチのサブネットの一覧を取得します。
func listSubnets(t *testing.T, h *helper.Helper, description string, id types.ID) []subnetItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "internet", "list-subnet", "--zone", internetZone, "--id", id.String(),
		"--query", "map({ID,SwitchID,InternetID,DefaultRoute,NextHop,StaticRoute,NetworkAddress,NetworkMaskLen,IPAddresses})")
	var items []subnetItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("%s: decode subnet list: %v", description, err)
	}
	if items == nil {
		t.Fatalf("%s: expected a JSON array of subnet", description)
	}
	return items
}

// selectServerNIC はサブネットの一覧から Server の NIC 設定を決めます。
// ルータ＋スイッチ接続の Server は Interfaces[].IPAddress が空のまま作成されるため、
// 一覧に含まれる割り当て可能なアドレスから選びます。ネットワークアドレス、ゲートウェイ、
// ブロードキャスト、割り当て範囲外のアドレスは拒否し、NIC に既に割り当て済みの
// アドレスへのフォールバックも行いません。Server NIC の内容、プレフィクス長、
// デフォルトルートを返します。
func selectServerNIC(subnets []subnetItem, internetID, switchID types.ID, server serverItem) (serverInterface, int, string, error) {
	var primary *subnetItem
	for i := range subnets {
		subnet := &subnets[i]
		if subnet.InternetID != internetID || subnet.SwitchID != switchID {
			continue
		}
		if subnet.NextHop != "" {
			continue
		}
		if primary != nil {
			return serverInterface{}, 0, "", errors.New("expected exactly one primary subnet on the Router + Switch")
		}
		primary = subnet
	}
	if primary == nil {
		return serverInterface{}, 0, "", errors.New("expected one primary subnet on the Router + Switch")
	}
	if primary.NetworkMaskLen < 1 || primary.NetworkMaskLen > 32 {
		return serverInterface{}, 0, "", fmt.Errorf("primary subnet has an unexpected prefix length %d", primary.NetworkMaskLen)
	}
	network, err := netip.ParseAddr(primary.NetworkAddress)
	if err != nil {
		return serverInterface{}, 0, "", fmt.Errorf("primary subnet has an unexpected network address %q", primary.NetworkAddress)
	}
	gateway, err := netip.ParseAddr(primary.DefaultRoute)
	if err != nil {
		return serverInterface{}, 0, "", fmt.Errorf("primary subnet has an unexpected default route %q", primary.DefaultRoute)
	}
	prefix, err := network.Prefix(primary.NetworkMaskLen)
	if err != nil {
		return serverInterface{}, 0, "", fmt.Errorf("primary subnet has an unexpected prefix length %d", primary.NetworkMaskLen)
	}
	if !prefix.Contains(gateway) {
		return serverInterface{}, 0, "", fmt.Errorf("primary subnet default route %q is out of the network %q", primary.DefaultRoute, primary.NetworkAddress)
	}

	if len(server.Interfaces) != 1 {
		return serverInterface{}, 0, "", errors.New("Server has an unexpected number of NICs")
	}
	nic := server.Interfaces[0]
	// ルータ＋スイッチ接続の NIC は API で UpstreamType "router" として返る (2026-10-28 ライブ確認)。
	// 共有セグメント ("shared") と接続なし ("none") を接続済みと判定しないための確認である。
	if nic.UpstreamType != "switch" && nic.UpstreamType != "router" {
		return serverInterface{}, 0, "", fmt.Errorf("Server NIC is not connected to a Router + Switch: upstream type %q", nic.UpstreamType)
	}
	if nic.SwitchID != switchID {
		return serverInterface{}, 0, "", fmt.Errorf("Server NIC is not connected to the created switch %s", switchID)
	}
	if _, err := net.ParseMAC(nic.MACAddress); err != nil {
		return serverInterface{}, 0, "", fmt.Errorf("Server NIC has an unexpected MAC address %q", nic.MACAddress)
	}

	broadcast := broadcastOf(prefix)
	for _, candidate := range primary.IPAddresses {
		ip, err := netip.ParseAddr(candidate.IPAddress)
		if err != nil || !ip.Is4() || !prefix.Contains(ip) {
			continue
		}
		if ip == prefix.Addr() || ip == gateway || ip == broadcast {
			continue
		}
		return serverInterface{
			ID:           nic.ID,
			MACAddress:   nic.MACAddress,
			IPAddress:    candidate.IPAddress,
			SwitchID:     nic.SwitchID,
			UpstreamType: nic.UpstreamType,
		}, primary.NetworkMaskLen, primary.DefaultRoute, nil
	}
	return serverInterface{}, 0, "", errors.New("no usable Server IPv4 address in the primary subnet")
}

// broadcastOf は IPv4 ネットワークのブロードキャストアドレスを返します。
func broadcastOf(prefix netip.Prefix) netip.Addr {
	ip := prefix.Addr().As4()
	hostBits := 32 - prefix.Bits()
	// ホスト部のビットをすべて立てる (下位ビットから)
	for i := 0; i < hostBits; i++ {
		ip[3-i/8] |= 1 << (i % 8)
	}
	return netip.AddrFrom4(ip)
}

// nicConfigScript は cloud-init の runcmd で実行する NIC 設定スクリプトを返します。
// MAC アドレスで NIC を特定し、IPv4 アドレスとデフォルトルートを設定します。
// /sys/class/net/*/address で NIC を探す。MAC アドレスの表記は小文字にそろえる。
func nicConfigScript(macAddress, ipAddress string, prefixLen int, gateway string) string {
	return fmt.Sprintf(`set -eu
expected_mac=%s
address=%s
prefix=%d
gateway=%s
iface=
for path in /sys/class/net/*/address; do
  if [ "$(cat "$path")" = "$expected_mac" ]; then
    iface=${path%%/address}
    iface=${iface##*/}
    break
  fi
done
[ -n "$iface" ]
ip link set "$iface" up
ip address replace "$address/$prefix" dev "$iface"
ip route replace default via "$gateway" dev "$iface"
`,
		strconv.Quote(strings.ToLower(macAddress)), strconv.Quote(ipAddress), prefixLen, strconv.Quote(gateway))
}

// TestSelectServerNICPicksUsableAddress は割り当て可能なアドレスから NIC 設定を選ぶことを確認します。
func TestSelectServerNICPicksUsableAddress(t *testing.T) {
	subnets := []subnetItem{{
		ID: 14, SwitchID: 13, InternetID: 12, DefaultRoute: "192.0.2.1",
		NetworkAddress: "192.0.2.0", NetworkMaskLen: 28,
		IPAddresses: []subnetIPAddress{
			{HostName: "x", IPAddress: "192.0.2.2"},
			{HostName: "y", IPAddress: "192.0.2.1"},
		},
	}}
	server := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", UpstreamType: "router"}}}

	nic, prefixLen, gateway, err := selectServerNIC(subnets, 12, 13, server)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.2", nic.IPAddress, "the gateway address should be skipped")
	require.Equal(t, "02:00:00:00:00:01", nic.MACAddress)
	require.Equal(t, types.ID(13), nic.SwitchID)
	require.Equal(t, 28, prefixLen)
	require.Equal(t, "192.0.2.1", gateway)
}

// TestSelectServerNICSkipsNetworkAndBroadcastAddresses はネットワークアドレスとブロードキャストを
// 候補から外すことを確認します。
func TestSelectServerNICSkipsNetworkAndBroadcastAddresses(t *testing.T) {
	subnets := []subnetItem{{
		ID: 14, SwitchID: 13, InternetID: 12, DefaultRoute: "192.0.2.1",
		NetworkAddress: "192.0.2.0", NetworkMaskLen: 30,
		IPAddresses: []subnetIPAddress{
			{IPAddress: "192.0.2.0"},
			{IPAddress: "192.0.2.3"},
			{IPAddress: "192.0.2.2"},
		},
	}}
	server := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", UpstreamType: "router"}}}

	nic, _, _, err := selectServerNIC(subnets, 12, 13, server)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.2", nic.IPAddress, "the only usable address should be selected")
}

// TestSelectServerNICRejectsMissingOrUnusableSubnetAddresses は割り当て可能なアドレスがない場合に
// NIC IPAddress へフォールバックせず拒否することを確認します。
func TestSelectServerNICRejectsMissingOrUnusableSubnetAddresses(t *testing.T) {
	for _, addresses := range [][]subnetIPAddress{
		{},
		{{IPAddress: "192.0.2.0"}, {IPAddress: "192.0.2.1"}, {IPAddress: "192.0.2.15"}},
		{{IPAddress: "198.51.100.2"}, {IPAddress: "invalid"}, {IPAddress: "2001:db8::1"}},
	} {
		subnets := []subnetItem{{
			ID: 14, SwitchID: 13, InternetID: 12, DefaultRoute: "192.0.2.1",
			NetworkAddress: "192.0.2.0", NetworkMaskLen: 28,
			IPAddresses: addresses,
		}}
		server := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", IPAddress: "192.0.2.2", UpstreamType: "router"}}}

		_, _, _, err := selectServerNIC(subnets, 12, 13, server)
		require.Error(t, err, "addresses %v: want refusal without falling back to NIC IPAddress", addresses)
		require.Contains(t, err.Error(), "no usable Server IPv4 address")
	}
}

// TestSelectServerNICRejectsInvalidSubnets は主サブネットの内容が不正な場合に拒否することを確認します。
func TestSelectServerNICRejectsInvalidSubnets(t *testing.T) {
	primary := subnetItem{
		ID: 14, SwitchID: 13, InternetID: 12, DefaultRoute: "192.0.2.1",
		NetworkAddress: "192.0.2.0", NetworkMaskLen: 28,
		IPAddresses: []subnetIPAddress{{IPAddress: "192.0.2.2"}},
	}
	server := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", UpstreamType: "router"}}}

	// 主サブネットが複数ある
	_, _, _, err := selectServerNIC([]subnetItem{primary, primary}, 12, 13, server)
	require.Error(t, err, "multiple primary subnets should be refused")

	// 主サブネットがない (NextHop 付きのサブネットだけ)
	_, _, _, err = selectServerNIC([]subnetItem{{
		ID: 15, SwitchID: 13, InternetID: 12, NextHop: "192.0.2.9", DefaultRoute: "192.0.2.1",
		NetworkAddress: "192.0.2.8", NetworkMaskLen: 29,
	}}, 12, 13, server)
	require.Error(t, err, "a Router + Switch without a primary subnet should be refused")

	// サブネットがない
	_, _, _, err = selectServerNIC(nil, 12, 13, server)
	require.Error(t, err, "a Router + Switch without subnets should be refused")

	// プレフィクス長が不正
	invalidMaskLen := primary
	invalidMaskLen.NetworkMaskLen = 0
	_, _, _, err = selectServerNIC([]subnetItem{invalidMaskLen}, 12, 13, server)
	require.Error(t, err, "an invalid prefix length should be refused")

	// ネットワークアドレスが不正
	invalidNetwork := primary
	invalidNetwork.NetworkAddress = "invalid"
	_, _, _, err = selectServerNIC([]subnetItem{invalidNetwork}, 12, 13, server)
	require.Error(t, err, "an invalid network address should be refused")

	// デフォルトルートが不正
	invalidRoute := primary
	invalidRoute.DefaultRoute = "invalid"
	_, _, _, err = selectServerNIC([]subnetItem{invalidRoute}, 12, 13, server)
	require.Error(t, err, "an invalid default route should be refused")

	// デフォルトルートがネットワーク外
	outOfPrefix := primary
	outOfPrefix.DefaultRoute = "198.51.100.1"
	_, _, _, err = selectServerNIC([]subnetItem{outOfPrefix}, 12, 13, server)
	require.Error(t, err, "a default route out of the network should be refused")

	// Switch ID が異なる
	wrongSwitch := primary
	wrongSwitch.SwitchID = 99
	_, _, _, err = selectServerNIC([]subnetItem{wrongSwitch}, 12, 13, server)
	require.Error(t, err, "a subnet on a different Switch should be refused")
}

// TestSelectServerNICRejectsInvalidServerNIC は Server NIC の内容が不正な場合に拒否することを確認します。
func TestSelectServerNICRejectsInvalidServerNIC(t *testing.T) {
	subnets := []subnetItem{{
		ID: 14, SwitchID: 13, InternetID: 12, DefaultRoute: "192.0.2.1",
		NetworkAddress: "192.0.2.0", NetworkMaskLen: 28,
		IPAddresses: []subnetIPAddress{{IPAddress: "192.0.2.2"}},
	}}

	// NIC がない
	_, _, _, err := selectServerNIC(subnets, 12, 13, serverItem{ID: 34})
	require.Error(t, err, "a Server without NICs should be refused")

	// 共有セグメントに接続している
	shared := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", UpstreamType: "shared"}}}
	_, _, _, err = selectServerNIC(subnets, 12, 13, shared)
	require.Error(t, err, "a Server on the shared segment should be refused")

	// 接続なし
	none := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", UpstreamType: "none"}}}
	_, _, _, err = selectServerNIC(subnets, 12, 13, none)
	require.Error(t, err, "a disconnected Server NIC should be refused")

	// Switch ID が異なる
	wrongSwitch := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 99, MACAddress: "02:00:00:00:00:01", UpstreamType: "router"}}}
	_, _, _, err = selectServerNIC(subnets, 12, 13, wrongSwitch)
	require.Error(t, err, "a Server NIC on a different Switch should be refused")

	// MAC アドレスが不正
	invalidMAC := serverItem{ID: 34, Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "not-a-mac"}}}
	_, _, _, err = selectServerNIC(subnets, 12, 13, invalidMAC)
	require.Error(t, err, "an invalid MAC address should be refused")
}

// TestNICConfigScript は NIC 設定スクリプトの内容を確認します。
func TestNICConfigScript(t *testing.T) {
	script := nicConfigScript("9C:A3:BA:30:CA:96", "192.0.2.2", 28, "192.0.2.1")
	for _, expected := range []string{
		"expected_mac=\"9c:a3:ba:30:ca:96\"",
		"address=\"192.0.2.2\"",
		"prefix=28",
		"gateway=\"192.0.2.1\"",
		"/sys/class/net/*/address",
		"ip address replace",
		"ip route replace default",
	} {
		require.Contains(t, script, expected)
	}
}
