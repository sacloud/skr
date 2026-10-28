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

// このファイルは Networking Suite E2E の共有ヘルパーの単体テストです。
// シナリオの経路はライブ実行で検証します。
package main

import (
	"bufio"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

func TestInterfaceSRNUsesZoneAndInterfaceID(t *testing.T) {
	require.Equal(t, "srnv1:sakura-is1b:sakura.iaas.interface:3456789012",
		interfaceSRN("is1b", types.ID(3456789012)))
}

func TestFindInterfaceRequiresUniqueDisconnectedNIC(t *testing.T) {
	item := serverItem{
		ID: 1,
		Interfaces: []serverInterface{
			{ID: 2, UpstreamType: "shared"},
			{ID: 3, UpstreamType: "none"},
		},
	}
	got, err := findInterface(item, "disconnected")
	require.NoError(t, err)
	require.EqualValues(t, 3, got.ID)

	item.Interfaces = append(item.Interfaces, serverInterface{ID: 4, UpstreamType: "none"})
	_, err = findInterface(item, "disconnected")
	require.Error(t, err, "findInterface accepted multiple disconnected NICs")
}

func TestServerNICArgs(t *testing.T) {
	tests := []struct {
		name  string
		entry bool
		want  []string
	}{
		{name: "entry server", entry: true, want: []string{"--nic", "upstream=shared", "--nic", "upstream=disconnected"}},
		{name: "peer server", want: []string{"--nic", "upstream=disconnected"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, serverNICArgs(test.entry))
		})
	}
}

func TestEntryUserDataConfiguresDefaultUserAndNetworkingSuiteDHCP(t *testing.T) {
	userData, err := entryUserData("ssh-rsa test-public-key", "9C:A3:BA:30:04:23")
	require.NoError(t, err)
	for _, want := range []string{
		"hostname: nstest6",
		"  - default",
		"ssh_authorized_keys:\n  - ssh-rsa test-public-key",
		`macaddress: "9c:a3:ba:30:04:23"`,
		"dhcp4: true",
		"route-metric: 200",
		"- [netplan, apply]",
	} {
		require.Contains(t, userData, want)
	}
	require.NotContains(t, userData, "name: skr-e2e",
		"entryUserData should install the key for the Archive default user")
}

func TestEntryUserDataRejectsInvalidNICMAC(t *testing.T) {
	_, err := entryUserData("ssh-rsa test-public-key", "not-a-mac")
	require.Error(t, err, "entryUserData accepted an invalid NIC MAC address")
}

func TestCheckDiskMatchesRequestedServerAndDisk(t *testing.T) {
	params := networkingSuiteParams{ArchiveID: 12, DiskPlanID: 4, SizeGB: 20}
	disk := diskItem{
		ID: 13, Name: "entry", ServerID: 11, DiskPlanID: 4,
		SourceArchiveID: 12, SizeMB: 20 * 1024,
	}
	require.NoError(t, checkDisk(disk, 11, "entry", params))

	disk.SourceArchiveID = 14
	require.Error(t, checkDisk(disk, 11, "entry", params),
		"checkDisk accepted a Disk from another Archive")
}

func TestSameInterfaceIDs(t *testing.T) {
	expected := []serverInterface{{ID: 456, UpstreamType: "shared"}, {ID: 789, UpstreamType: "none"}}
	require.True(t, sameInterfaceIDs(expected, expected))
	require.True(t, sameInterfaceIDs(expected,
		[]serverInterface{{ID: 456, UpstreamType: "shared"}, {ID: 789, UpstreamType: "none", IPAddress: "10.0.1.4"}}),
		"sameInterfaceIDs should compare only the NIC IDs")
	require.False(t, sameInterfaceIDs(expected,
		[]serverInterface{{ID: 456, UpstreamType: "shared"}, {ID: 999, UpstreamType: "none"}}),
		"sameInterfaceIDs should reject a changed NIC")
	require.False(t, sameInterfaceIDs(expected, expected[:1]),
		"sameInterfaceIDs should reject a missing NIC")
}

func TestParseDiskSizeGB(t *testing.T) {
	size, err := parseDiskSizeGB("")
	require.NoError(t, err)
	require.EqualValues(t, 20, size, "the disk size should default to 20GB")

	size, err = parseDiskSizeGB("40")
	require.NoError(t, err)
	require.EqualValues(t, 40, size)

	for _, test := range []struct {
		name   string
		sizeGB string
	}{
		{name: "not an integer", sizeGB: "abc"},
		{name: "zero", sizeGB: "0"},
		{name: "negative", sizeGB: "-20"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseDiskSizeGB(test.sizeGB)
			require.Error(t, err)
		})
	}
}

func TestPythonTCPCommandQuotesScript(t *testing.T) {
	command := pythonTCPCommand("10.0.2.10", "ping")
	for _, text := range []string{
		"python3 -c ", "10.0.2.10", "ping", "38172", `.encode()`, "makefile(", "rb", `.decode().strip()`,
	} {
		require.Contains(t, command, text)
	}
}

func TestPythonTCPCommandExchangesBytes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, listener.Close(), "close TCP listener")
	}()

	type result struct {
		line string
		err  error
	}
	received := make(chan result, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			received <- result{err: err}
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close TCP connection: %v", err)
			}
		}()
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			received <- result{err: err}
			return
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			received <- result{err: err}
			return
		}
		if _, err := conn.Write([]byte("PONG networking-suite\n")); err != nil {
			received <- result{err: err}
			return
		}
		received <- result{line: line}
	}()

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	command := exec.Command("python3", "-c", pythonTCPCode(host, port, "ping")) //nolint:gosec // 生成されたコードはテスト用のローカルリスナーのみを対象とする。
	output, err := command.CombinedOutput()
	require.NoError(t, err, "pythonTCPCommand failed: %s", output)
	require.Equal(t, "PONG networking-suite\n", string(output))

	select {
	case got := <-received:
		require.NoError(t, got.err)
		require.Equal(t, "ping\n", got.line)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for TCP listener")
	}
}

func TestDecodeListRejectsNullAndNonArray(t *testing.T) {
	items, err := decodeList[groupItem]([]byte("[]"), "subnet-group list")
	require.NoError(t, err)
	require.Empty(t, items)

	_, err = decodeList[groupItem]([]byte("null"), "subnet-group list")
	require.Error(t, err, "decodeList accepted a null response")
	_, err = decodeList[groupItem]([]byte(`{"SRN":"x"}`), "subnet-group list")
	require.Error(t, err, "decodeList accepted a non-array response")
}

func TestContainsAddressAndSubnet(t *testing.T) {
	connection := connectionItem{SRN: "connection-srn", EphemeralIPv4Address: "10.0.1.4"}
	subnet := subnetItem{SRN: "subnet-srn", Name: "subnet"}
	address := addressItem{SRN: "address-srn", IPAddress: "10.0.1.4"}
	address.Subnet.SRN = "subnet-srn"
	require.True(t, containsAddress([]addressItem{address}, connection, "subnet-srn"))
	require.False(t, containsAddress(nil, connection, "subnet-srn"))
	require.False(t, containsAddress([]addressItem{address}, connection, "other-srn"),
		"containsAddress should reject a different subnet")
	require.True(t, containsSubnet([]subnetItem{subnet}, subnet))
	require.False(t, containsSubnet(nil, subnet))
}

func TestSharedIPOfServer(t *testing.T) {
	server := serverItem{
		Interfaces: []serverInterface{
			{ID: 1, UpstreamType: "none"},
			{ID: 2, UpstreamType: "shared", IPAddress: "192.0.2.1"},
		},
	}
	require.Equal(t, "192.0.2.1", sharedIPOfServer(server))
	server.Interfaces = append(server.Interfaces, serverInterface{ID: 3, UpstreamType: "shared", IPAddress: "not-an-ip"})
	require.Equal(t, "192.0.2.1", sharedIPOfServer(server))
	require.Empty(t, sharedIPOfServer(serverItem{Interfaces: []serverInterface{{ID: 1, UpstreamType: "none"}}}),
		"sharedIPOfServer should return empty without a shared NIC")
	require.True(t, strings.Contains(shellQuote("a'b"), "'\"'\"'"),
		"shellQuote should escape single quotes")
}
