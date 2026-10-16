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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type fakeCLI struct {
	groups        []groupItem
	deleted       []string
	calls         []string
	subnetsRemain bool
}

func (f *fakeCLI) call(_ context.Context, step string, args []string, _ any) ([]byte, error) {
	f.calls = append(f.calls, step)
	switch step {
	case "cleanup-read-subnet-group":
		if len(f.groups) != 1 || len(args) < 4 || args[3] != f.groups[0].SRN {
			return nil, errors.New("unexpected group read")
		}
		return json.Marshal(f.groups[0])
	case "cleanup-delete-subnet-group":
		if len(f.groups) != 1 || len(args) != 4 || args[3] != f.groups[0].SRN {
			return nil, errors.New("unexpected group delete")
		}
		f.deleted = append(f.deleted, args[3])
		f.groups = []groupItem{}
		return nil, nil
	case "cleanup-find-subnet-group", "cleanup-list-groups-after":
		if f.groups == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(f.groups)
	case "cleanup-list-subnets-after":
		if f.subnetsRemain {
			return []byte(`[{"SRN":"remaining"}]`), nil
		}
		return []byte("[]"), nil
	case "cleanup-find-subnet-0", "cleanup-find-subnet-1", "cleanup-find-server-0", "cleanup-find-server-1":
		return []byte("[]"), nil
	default:
		return nil, errors.New("unexpected CLI call: " + step)
	}
}

func TestInterfaceSRNUsesZoneAndInterfaceID(t *testing.T) {
	if got, want := interfaceSRN(types.ID(3456789012)), "srnv1:sakura-is1b:sakura.iaas.interface:3456789012"; got != want {
		t.Fatalf("interfaceSRN() = %q, want %q", got, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 3 {
		t.Fatalf("NIC ID = %s, want 3", got.ID)
	}
	item.Interfaces = append(item.Interfaces, serverInterface{ID: 4, UpstreamType: "none"})
	if _, err := findInterface(item, "disconnected"); err == nil {
		t.Fatal("findInterface accepted multiple disconnected NICs")
	}
}

func TestCheckCloudimgArchive(t *testing.T) {
	item := archiveItem{ID: 113701786718, Name: "Ubuntu Server 24.04.2 LTS 64bit (cloudimg)"}
	if err := checkCloudimgArchive(item, 113701786718); err != nil {
		t.Fatalf("checkCloudimgArchive rejected cloudimg Archive: %v", err)
	}

	tests := []struct {
		name string
		item archiveItem
	}{
		{name: "wrong id", item: archiveItem{ID: 1, Name: "Ubuntu cloudimg"}},
		{name: "not cloudimg", item: archiveItem{ID: 113701786718, Name: "Ubuntu Server 24.04.2 LTS 64bit"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := checkCloudimgArchive(test.item, 113701786718); err == nil {
				t.Fatal("checkCloudimgArchive accepted an invalid Archive")
			}
		})
	}
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
			if got := serverNICArgs(test.entry); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("serverNICArgs(%t) = %v, want %v", test.entry, got, test.want)
			}
		})
	}
}

func TestEntryUserDataConfiguresDefaultUserAndNetworkingSuiteDHCP(t *testing.T) {
	userData, err := entryUserData("ssh-rsa test-public-key", "9C:A3:BA:30:04:23")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"hostname: nstest6",
		"  - default",
		"ssh_authorized_keys:\n  - ssh-rsa test-public-key",
		"macaddress: \"9c:a3:ba:30:04:23\"",
		"dhcp4: true",
		"route-metric: 200",
		"- [netplan, apply]",
	} {
		if !strings.Contains(userData, want) {
			t.Errorf("entryUserData() does not contain %q", want)
		}
	}
	if strings.Contains(userData, "name: skr-e2e") {
		t.Fatal("entryUserData() should install the key for the Archive default user")
	}
}

func TestEntryUserDataRejectsInvalidNICMAC(t *testing.T) {
	if _, err := entryUserData("ssh-rsa test-public-key", "not-a-mac"); err == nil {
		t.Fatal("entryUserData() accepted an invalid NIC MAC address")
	}
}

func TestCheckDiskMatchesRequestedServerAndDisk(t *testing.T) {
	disk := diskItem{
		ID: 13, Name: "entry", ServerID: 11, DiskPlanID: 4,
		SourceArchiveID: 12, SizeMB: 20 * 1024,
	}
	if err := checkDisk(disk, 11, "entry", 12, 4, 20); err != nil {
		t.Fatalf("checkDisk rejected matching Disk: %v", err)
	}
	disk.SourceArchiveID = 14
	if err := checkDisk(disk, 11, "entry", 12, 4, 20); err == nil {
		t.Fatal("checkDisk accepted a Disk from another Archive")
	}
}

func TestCleanupLeavesUnmarkedGroupUntouched(t *testing.T) {
	fake := &fakeCLI{groups: []groupItem{{
		SRN: "temporary-srn", Name: namePrefix + "different",
		Description: "user-owned resource", IPv4AddressRangeCIDR: "10.0.0.0/16",
		Region: struct{ Code string }{Code: "is1"},
	}}}
	test := &scenario{client: fake}
	if err := test.cleanup(context.Background(), namePrefix+"run"); err != nil {
		t.Fatal(err)
	}
	if len(fake.deleted) != 0 || len(fake.groups) != 1 {
		t.Fatalf("cleanup modified unrelated group: deleted=%v remaining=%v", fake.deleted, fake.groups)
	}
}

func TestCleanupDeletesOnlyMatchingTemporaryGroup(t *testing.T) {
	group := groupItem{
		SRN: "temporary-srn", Name: namePrefix + "run-updated",
		Description: description, IPv4AddressRangeCIDR: "10.0.0.0/16",
		Region: struct{ Code string }{Code: "is1"},
	}
	fake := &fakeCLI{groups: []groupItem{group}}
	test := &scenario{client: fake, group: group}
	if err := test.cleanup(context.Background(), namePrefix+"run"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.deleted, []string{group.SRN}) || len(fake.groups) != 0 {
		t.Fatalf("cleanup deleted=%v remaining=%v", fake.deleted, fake.groups)
	}
	if strings.Contains(strings.Join(fake.calls, " "), "cleanup-delete-subnet") == false {
		t.Fatalf("cleanup did not record group deletion: %v", fake.calls)
	}
}

func TestCleanupDoesNotDeleteGroupWithRemainingSubnets(t *testing.T) {
	group := groupItem{
		SRN: "temporary-srn", Name: namePrefix + "run-updated",
		Description: description, IPv4AddressRangeCIDR: "10.0.0.0/16",
		Region: struct{ Code string }{Code: "is1"},
	}
	fake := &fakeCLI{groups: []groupItem{group}, subnetsRemain: true}
	test := &scenario{client: fake, group: group}
	if err := test.cleanup(context.Background(), namePrefix+"run"); err == nil {
		t.Fatal("cleanup succeeded while subnets remained")
	}
	if len(fake.deleted) != 0 {
		t.Fatalf("cleanup deleted a group with remaining subnets: %v", fake.deleted)
	}
}

func TestPythonTCPCommandQuotesScript(t *testing.T) {
	command := pythonTCPCommand("10.0.2.10", "ping")
	for _, text := range []string{
		"python3 -c ", "10.0.2.10", "ping", "38172", `.encode()`, "makefile(", "rb", `.decode().strip()`,
	} {
		if !strings.Contains(command, text) {
			t.Errorf("TCP command %q does not contain %q", command, text)
		}
	}
}

func TestPythonTCPCommandExchangesBytes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close TCP listener: %v", err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("python3", "-c", pythonTCPCode(host, port, "ping")) //nolint:gosec // The generated code targets only a test-local listener.
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pythonTCPCommand failed: %v\n%s", err, output)
	}
	if got, want := string(output), "PONG networking-suite\n"; got != want {
		t.Fatalf("pythonTCPCommand output = %q, want %q", got, want)
	}
	select {
	case got := <-received:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.line != "ping\n" {
			t.Fatalf("server received %q, want %q", got.line, "ping\n")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for TCP listener")
	}
}
