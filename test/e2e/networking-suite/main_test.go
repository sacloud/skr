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
	"fmt"
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
	groups           []groupItem
	subnets          []subnetItem
	foundServers     map[int][]serverItem
	addresses        map[string][]addressItem
	addressListError map[string]error
	args             map[string][]string
	deleted          []string
	deletedSubnets   []string
	calls            []string
	subnetsRemain    bool
	failStep         string
	operationError   error
	deletedServers   []string
	disks            map[types.ID]diskItem
}

type cliFunc func(context.Context, string, []string, any) ([]byte, error)

func (f cliFunc) call(ctx context.Context, step string, args []string, request any) ([]byte, error) {
	return f(ctx, step, args, request)
}

func (f *fakeCLI) call(_ context.Context, step string, args []string, _ any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = make(map[string][]string)
	}
	f.args[step] = append([]string(nil), args...)
	switch step {
	case "profile-current":
		return nil, nil
	case "preflight-read-cloudimg-archive":
		return json.Marshal(archiveItem{ID: 12, Name: "Ubuntu Server 24.04.2 LTS 64bit (cloudimg)"})
	case "preflight-list-groups", "preflight-find-entry", "preflight-find-peer":
		return []byte("[]"), nil
	case "create-subnet-group", "read-subnet-group":
		return json.Marshal(f.groups[0])
	case "update-subnet-group":
		f.groups[0].Name += "-updated"
		if f.failStep == step {
			return nil, f.operationError
		}
		return nil, nil
	case "create-subnet-0":
		return json.Marshal(f.subnets[0])
	case "create-subnet-1":
		return json.Marshal(f.subnets[1])
	case "update-subnet":
		f.subnets[0].Name += "-updated"
		if f.failStep == step {
			return nil, f.operationError
		}
		return nil, nil
	case "list-subnets":
		return json.Marshal(f.subnets)
	case "read-subnet":
		return json.Marshal(f.subnets[0])
	case "create-server-0":
		if f.failStep == "read-server-0" {
			return json.Marshal(f.foundServers[0][0])
		}
		return nil, f.operationError
	case "read-server-0":
		return nil, f.operationError
	case "cleanup-check-server-0", "cleanup-read-server-0":
		return json.Marshal(f.foundServers[0][0])
	case "cleanup-wait-server-disk-0":
		if disk, ok := f.disks[types.ID(300)]; ok {
			return json.Marshal(disk)
		}
		return json.Marshal(diskItem{
			ID: 999, Name: namePrefix + "run-entry", ServerID: 123,
			DiskPlanID: 4, SourceArchiveID: 14, SizeMB: 20 * 1024, Availability: "available",
		})
	case "cleanup-delete-server-0":
		f.deletedServers = append(f.deletedServers, args[6])
		f.foundServers[0] = nil
		return nil, nil
	case "cleanup-find-server-after-0", "cleanup-find-disk-after-0":
		return []byte("[]"), nil
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
		if f.subnets == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(f.subnets)
	case "cleanup-find-subnet-0", "cleanup-find-subnet-1":
		return []byte("[]"), nil
	case "cleanup-find-server-0":
		return json.Marshal(f.foundServers[0])
	case "cleanup-find-server-1":
		return json.Marshal(f.foundServers[1])
	case "cleanup-list-addresses-0", "cleanup-list-addresses-1":
		if err := f.addressListError[step]; err != nil {
			return nil, err
		}
		return json.Marshal(f.addresses[step])
	case "cleanup-read-subnet-0", "cleanup-read-subnet-1":
		for _, subnet := range f.subnets {
			if subnet.SRN == args[3] {
				return json.Marshal(subnet)
			}
		}
		return nil, errors.New("unexpected subnet read")
	case "cleanup-delete-subnet-0", "cleanup-delete-subnet-1":
		f.deletedSubnets = append(f.deletedSubnets, args[3])
		for i := range f.subnets {
			if f.subnets[i].SRN == args[3] {
				f.subnets = append(f.subnets[:i], f.subnets[i+1:]...)
				break
			}
		}
		return nil, nil
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
		{name: "other OS", item: archiveItem{ID: 113701786718, Name: "Debian 12 (cloudimg)"}},
		{name: "other Ubuntu release", item: archiveItem{ID: 113701786718, Name: "Ubuntu Server 22.04 LTS (cloudimg)"}},
		{name: "Ubuntu only in description", item: archiveItem{ID: 113701786718, Name: "Debian (cloudimg)", Description: "Ubuntu Server 24.04 LTS (cloudimg)"}},
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

func TestFindTemporaryServerRetainsNICIdentityForCleanup(t *testing.T) {
	expected := serverItem{
		ID:          123,
		Name:        namePrefix + "run-entry",
		Description: description,
		Interfaces: []serverInterface{
			{ID: 456, UpstreamType: "shared"},
			{ID: 789, UpstreamType: "none"},
		},
	}
	fake := &fakeCLI{foundServers: map[int][]serverItem{0: {expected}}}
	test := &scenario{client: fake}
	got, err := test.findTemporaryServer(context.Background(), 0, expected.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Interfaces, expected.Interfaces) {
		t.Fatalf("recovered interfaces = %v, want %v", got.Interfaces, expected.Interfaces)
	}
	if !sameInterfaceIDs(got.Interfaces, expected.Interfaces) {
		t.Fatal("recovered NIC identity cannot authorize cleanup of the created Server")
	}
	if args := strings.Join(fake.args["cleanup-find-server-0"], " "); !strings.Contains(args, "map({ID,Name,Description,Interfaces})") {
		t.Fatalf("server recovery query omitted Interfaces: %q", args)
	}
}

func TestRunCleansUpAfterPartiallyAppliedOperations(t *testing.T) {
	for _, failStep := range []string{"update-subnet-group", "update-subnet", "create-server-0", "read-server-0", "lost create response with Disk"} {
		t.Run(failStep, func(t *testing.T) {
			t.Setenv("SAKURA_ENDPOINTS_NETWORKING_SUITE", "https://secure.sakura.ad.jp/cloud/zone/is1b/api/cloud/1.1/")
			name := namePrefix + "run"
			operationError := errors.New("operation applied but response failed")
			group := groupItem{
				SRN: "group-srn", Name: name, Description: description,
				IPv4AddressRangeCIDR: "10.0.0.0/16",
				Region:               struct{ Code string }{Code: "is1"},
			}
			fake := &fakeCLI{
				groups: []groupItem{group}, failStep: failStep, operationError: operationError,
			}
			if failStep != "update-subnet-group" {
				for i, cidr := range []string{"10.0.1.0/24", "10.0.2.0/24"} {
					fake.subnets = append(fake.subnets, subnetItem{
						SRN: fmt.Sprintf("subnet-%d", i), Name: fmt.Sprintf("%s-subnet-%d", name, i+1),
						Description: description, IPv4AddressRangeCIDR: cidr,
						Zone:        struct{ Code string }{Code: zone},
						SubnetGroup: struct{ SRN string }{SRN: group.SRN},
					})
				}
			}
			if failStep == "create-server-0" || failStep == "read-server-0" || failStep == "lost create response with Disk" {
				fake.foundServers = map[int][]serverItem{0: {{
					ID: 123, Name: name + "-entry", Description: description, InstanceStatus: "down",
					Interfaces: []serverInterface{{ID: 456, UpstreamType: "shared"}, {ID: 789, UpstreamType: "none"}},
				}}}
			}
			if failStep == "read-server-0" || failStep == "lost create response with Disk" {
				fake.foundServers[0][0].Disks = []diskItem{{ID: 300}}
				fake.disks = map[types.ID]diskItem{300: {
					ID: 300, Name: name + "-entry", ServerID: 123, DiskPlanID: 4,
					SourceArchiveID: 12, SizeMB: 20 * 1024, Availability: "available",
				}}
			}
			test := &scenario{client: fake, archiveID: 12, diskPlanID: 4, diskSizeGB: 20}
			err := test.run(context.Background(), name)
			if !errors.Is(err, operationError) {
				t.Fatalf("run error = %v, want original operation error", err)
			}
			if failStep == "lost create response with Disk" {
				if !strings.Contains(err.Error(), "manual cleanup required") || len(fake.deletedServers) != 0 {
					t.Fatalf("unknown Disk must require manual cleanup without deletion: err=%v deleted=%v", err, fake.deletedServers)
				}
				return
			}
			if strings.Contains(err.Error(), "cleanup failed") {
				t.Fatalf("cleanup after partial operation failed: %v", err)
			}
			if len(fake.groups) != 0 || len(fake.subnets) != 0 {
				t.Fatalf("resources remain: groups=%v subnets=%v", fake.groups, fake.subnets)
			}
			if failStep == "create-server-0" || failStep == "read-server-0" {
				if !reflect.DeepEqual(fake.deletedServers, []string{"123"}) {
					t.Fatalf("created Server was not deleted: %v", fake.deletedServers)
				}
				if _, called := fake.args["cleanup-wait-server-disk-0"]; called && failStep == "create-server-0" {
					t.Fatal("cleanup tried to wait for a missing Disk")
				}
				if failStep == "read-server-0" && test.diskIDs[0] != 300 {
					t.Fatalf("Disk ID from create response was lost: %v", test.diskIDs)
				}
			}
		})
	}
}

func TestCleanupDoesNotDeleteSubnetWhenAddressCheckFails(t *testing.T) {
	assertSubnetCleanupBlocked(t, true)
}

func TestCleanupRejectsUnexpectedNamesAfterUpdate(t *testing.T) {
	for _, resource := range []string{"group", "first subnet", "second subnet"} {
		t.Run(resource, func(t *testing.T) {
			name := namePrefix + "run"
			group := groupItem{
				SRN: "group-srn", Name: name, Description: description,
				IPv4AddressRangeCIDR: "10.0.0.0/16",
				Region:               struct{ Code string }{Code: "is1"},
			}
			subnets := [2]subnetItem{}
			for i, cidr := range []string{"10.0.1.0/24", "10.0.2.0/24"} {
				subnets[i] = subnetItem{
					SRN: fmt.Sprintf("subnet-%d", i), Name: fmt.Sprintf("%s-subnet-%d", name, i+1),
					Description: description, IPv4AddressRangeCIDR: cidr,
					Zone:        struct{ Code string }{Code: zone},
					SubnetGroup: struct{ SRN string }{SRN: group.SRN},
				}
			}
			fake := &fakeCLI{groups: []groupItem{group}, subnets: []subnetItem{subnets[0], subnets[1]}}
			switch resource {
			case "group":
				fake.groups[0].Name = name + "-unplanned"
			case "first subnet":
				fake.subnets[0].Name = subnets[0].Name + "-unplanned"
			case "second subnet":
				fake.subnets[1].Name += "-updated"
			}
			test := &scenario{client: fake, group: group, subnets: subnets}
			if err := test.cleanup(context.Background(), name); err == nil {
				t.Fatal("cleanup accepted an unplanned name")
			}
			if len(fake.deleted) != 0 {
				t.Fatalf("cleanup deleted group: %v", fake.deleted)
			}
			if resource != "group" && len(fake.subnets) != 1 {
				t.Fatalf("cleanup modified renamed subnet: %v", fake.subnets)
			}
		})
	}
}

func TestCleanupDisklessServerStillRequiresIdentity(t *testing.T) {
	for _, field := range []string{"name", "description", "NICs", "Disk"} {
		t.Run(field, func(t *testing.T) {
			name := namePrefix + "run"
			expected := serverItem{
				ID: 123, Name: name + "-entry", Description: description, InstanceStatus: "down",
				Interfaces: []serverInterface{{ID: 456, UpstreamType: "shared"}},
			}
			current := expected
			switch field {
			case "name":
				current.Name += "-unplanned"
			case "description":
				current.Description = "not the temporary Server"
			case "NICs":
				current.Interfaces = []serverInterface{{ID: 789, UpstreamType: "shared"}}
			case "Disk":
				current.Disks = []diskItem{{ID: 999}}
			}
			fake := &fakeCLI{foundServers: map[int][]serverItem{0: {current}}}
			test := &scenario{client: fake, archiveID: 12, diskPlanID: 4, diskSizeGB: 20}
			test.servers[0] = expected
			if err := test.cleanup(context.Background(), name); err == nil {
				t.Fatalf("cleanup accepted a Server with changed %s", field)
			}
			if len(fake.deletedServers) != 0 {
				t.Fatalf("cleanup deleted changed Server: %v", fake.deletedServers)
			}
		})
	}
}

func TestCleanupRunningServers(t *testing.T) {
	for _, outcome := range []string{
		"stopped", "SSH failure", "wait failure", "NIC mismatch", "Disk ID mismatch",
		"Disk attributes mismatch", "unknown Disk ID", "Disk read ID mismatch", "detached Disk",
		"SSH failure with NIC mismatch", "SSH failure with Disk ID mismatch",
		"pre-stop NIC mismatch", "stop-poll NIC mismatch",
	} {
		t.Run(outcome, func(t *testing.T) {
			name := namePrefix + "run"
			stopError := errors.New("shutdown failed")
			var calls []string
			sshCalls := 0
			sshFailed := strings.HasPrefix(outcome, "SSH failure")
			test := &scenario{archiveID: 12, diskPlanID: 4, diskSizeGB: 20, publicIP: "192.0.2.1", bAddress: "10.0.2.4"}
			for i, suffix := range []string{"entry", "peer"} {
				test.servers[i] = serverItem{
					ID: types.ID(100 + i), Name: name + "-" + suffix, Description: description,
					Interfaces: []serverInterface{{ID: types.ID(200 + i)}},
					Disks:      []diskItem{{ID: types.ID(300 + i)}},
				}
				test.diskIDs[i] = types.ID(300 + i)
				test.connects[i] = connectionItem{SRN: fmt.Sprintf("connection-%d", i)}
			}
			if outcome == "unknown Disk ID" {
				test.diskIDs[0] = 0
			}
			test.sshCommand = func(_ context.Context, step, _, _ string) (string, string, error) {
				calls = append(calls, step)
				sshCalls++
				if sshFailed {
					return "", "", stopError
				}
				return "", "", nil
			}
			test.client = cliFunc(func(_ context.Context, step string, args []string, _ any) ([]byte, error) {
				calls = append(calls, step)
				switch {
				case step == "cleanup-find-subnet-group":
					return []byte("[]"), nil
				case strings.HasPrefix(step, "cleanup-check-server-"), strings.HasPrefix(step, "cleanup-read-server-"):
					i := int(step[len(step)-1] - '0')
					current := test.servers[i]
					current.InstanceStatus = "up"
					if sshCalls == 2 && !sshFailed {
						current.InstanceStatus = "down"
					}
					if i == 0 && (outcome == "pre-stop NIC mismatch" || outcome == "stop-poll NIC mismatch" && strings.HasPrefix(step, "cleanup-read-server-")) {
						current.Interfaces = []serverInterface{{ID: 999}}
					}
					if strings.HasPrefix(step, "cleanup-read-server-") {
						// 最初の stop-poll read を失敗させるが、その後の削除 identity read は許可する。
						if outcome == "wait failure" && sshCalls == 2 {
							sshCalls++
							return nil, stopError
						}
						if i == 0 {
							switch outcome {
							case "NIC mismatch", "SSH failure with NIC mismatch":
								current.Interfaces = []serverInterface{{ID: 999}}
							case "Disk ID mismatch", "SSH failure with Disk ID mismatch":
								current.Disks = []diskItem{{ID: 999}}
							case "detached Disk":
								current.Disks = nil
							}
						}
					}
					return json.Marshal(current)
				case strings.HasPrefix(step, "cleanup-wait-server-disk-"):
					i := int(step[len(step)-1] - '0')
					disk := diskItem{
						ID: types.ID(300 + i), Name: test.servers[i].Name, ServerID: test.servers[i].ID,
						DiskPlanID: 4, SourceArchiveID: 12, SizeMB: 20 * 1024, Availability: "available",
					}
					if i == 0 && outcome == "Disk attributes mismatch" {
						disk.SourceArchiveID = 999
					}
					if i == 0 && outcome == "Disk read ID mismatch" {
						disk.ID = 999
					}
					return json.Marshal(disk)
				case strings.HasPrefix(step, "cleanup-delete-connection-"):
					if sshFailed || outcome == "wait failure" || outcome == "NIC mismatch" || outcome == "pre-stop NIC mismatch" || outcome == "stop-poll NIC mismatch" {
						t.Fatal("connection detached after shutdown failure")
					}
					return nil, nil
				case strings.HasPrefix(step, "cleanup-delete-server-"):
					if !strings.Contains(strings.Join(args, " "), "--with-disks --force --fail-if-not-found") {
						t.Fatalf("missing forced cleanup options: %v", args)
					}
					return nil, nil
				case strings.HasPrefix(step, "cleanup-find-server-after-"), strings.HasPrefix(step, "cleanup-find-disk-after-"):
					return []byte("[]"), nil
				default:
					return nil, fmt.Errorf("unexpected call %s", step)
				}
			})
			err := test.cleanup(context.Background(), name)
			if outcome == "stopped" && err != nil {
				t.Fatal(err)
			}
			if outcome != "stopped" && err == nil {
				t.Fatal("cleanup failure was not propagated")
			}
			stopFailed := sshFailed || outcome == "wait failure" || outcome == "NIC mismatch" || outcome == "pre-stop NIC mismatch" || outcome == "stop-poll NIC mismatch"
			if (sshFailed || outcome == "wait failure") && !errors.Is(err, stopError) {
				t.Fatalf("shutdown error was lost: %v", err)
			}
			deletedEntry := false
			deletedPeer := false
			lastConnection := -1
			connectionCount := 0
			firstDeletion := -1
			for i, step := range calls {
				if strings.HasPrefix(step, "cleanup-delete-connection-") {
					lastConnection = i
					connectionCount++
				}
				if strings.HasPrefix(step, "cleanup-delete-server-") && firstDeletion == -1 {
					firstDeletion = i
				}
				deletedEntry = deletedEntry || step == "cleanup-delete-server-0"
				deletedPeer = deletedPeer || step == "cleanup-delete-server-1"
			}
			expectEntryDeleted := outcome == "stopped" || outcome == "SSH failure" || outcome == "wait failure"
			if deletedEntry != expectEntryDeleted || !deletedPeer {
				t.Fatalf("unexpected Server deletions: %v", calls)
			}
			if !stopFailed && (connectionCount != 2 || lastConnection >= firstDeletion) {
				t.Fatalf("connections must be detached before Server deletion: %v", calls)
			}
			if outcome == "pre-stop NIC mismatch" && sshCalls != 0 {
				t.Fatalf("SSH attempted after NIC identity changed: %v", calls)
			}
		})
	}
}

func TestWaitForEntrySSHRetainsNICIdentity(t *testing.T) {
	for _, change := range []string{"exchange", "addition"} {
		t.Run(change, func(t *testing.T) {
			name := namePrefix + "run"
			expected := serverItem{
				ID: 123, Name: name + "-entry", Description: description, InstanceStatus: "up",
				Interfaces: []serverInterface{{ID: 456, UpstreamType: "shared", IPAddress: "192.0.2.1"}},
			}
			current := expected
			current.Interfaces = []serverInterface{{ID: 789, UpstreamType: "shared", IPAddress: "192.0.2.2"}}
			if change == "addition" {
				current.Interfaces = append(current.Interfaces, expected.Interfaces...)
			}
			fake := &fakeCLI{foundServers: map[int][]serverItem{0: {current}}}
			test := &scenario{client: cliFunc(func(ctx context.Context, step string, args []string, request any) ([]byte, error) {
				if step == "poll-entry-server" {
					return json.Marshal(current)
				}
				return fake.call(ctx, step, args, request)
			})}
			test.servers[0] = expected
			test.sshCommand = func(context.Context, string, string, string) (string, string, error) {
				t.Fatal("SSH attempted after NIC identity changed")
				return "", "", nil
			}
			if _, err := test.waitForEntrySSH(context.Background()); err == nil {
				t.Fatal("SSH wait accepted changed NICs")
			}
			if !reflect.DeepEqual(test.servers[0], expected) {
				t.Fatalf("saved Server identity was overwritten: %+v", test.servers[0])
			}
			if err := test.cleanup(context.Background(), name); err == nil {
				t.Fatal("cleanup accepted changed NICs after SSH wait")
			}
			if len(fake.deletedServers) != 0 {
				t.Fatalf("cleanup deleted changed Server: %v", fake.deletedServers)
			}
		})
	}
}

func TestCleanupDoesNotDeleteSubnetWithAssignedAddresses(t *testing.T) {
	assertSubnetCleanupBlocked(t, false)
}

func assertSubnetCleanupBlocked(t *testing.T, failAddressList bool) {
	t.Helper()
	group := groupItem{
		SRN: "temporary-srn", Name: namePrefix + "run-updated",
		Description: description, IPv4AddressRangeCIDR: "10.0.0.0/16",
		Region: struct{ Code string }{Code: "is1"},
	}
	subnets := [2]subnetItem{
		{
			SRN: "entry-subnet-srn", Name: namePrefix + "run-subnet-1-updated",
			Description: description, IPv4AddressRangeCIDR: "10.0.1.0/24",
			Zone:        struct{ Code string }{Code: zone},
			SubnetGroup: struct{ SRN string }{SRN: group.SRN},
		},
		{
			SRN: "peer-subnet-srn", Name: namePrefix + "run-subnet-2",
			Description: description, IPv4AddressRangeCIDR: "10.0.2.0/24",
			Zone:        struct{ Code string }{Code: zone},
			SubnetGroup: struct{ SRN string }{SRN: group.SRN},
		},
	}
	fake := &fakeCLI{
		groups:  []groupItem{group},
		subnets: []subnetItem{subnets[0], subnets[1]},
		addressListError: map[string]error{
			"cleanup-list-addresses-0": func() error {
				if failAddressList {
					return errors.New("address list unavailable")
				}
				return nil
			}(),
		},
		addresses: map[string][]addressItem{
			"cleanup-list-addresses-0": {
				{SRN: "assigned-address-srn", IPAddress: "10.0.1.4"},
			},
		},
	}
	test := &scenario{client: fake, group: group, subnets: subnets}
	if err := test.cleanup(context.Background(), namePrefix+"run"); err == nil {
		t.Fatal("cleanup succeeded while the address safety check did not pass")
	}
	if !reflect.DeepEqual(fake.deletedSubnets, []string{subnets[1].SRN}) {
		t.Fatalf("deleted subnets = %v, want only the verified empty subnet %q", fake.deletedSubnets, subnets[1].SRN)
	}
	if len(fake.groups) != 1 {
		t.Fatal("cleanup deleted the subnet group while a subnet remained")
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
	command := exec.Command("python3", "-c", pythonTCPCode(host, port, "ping")) //nolint:gosec // 生成されたコードはテスト用のローカルリスナーのみを対象とする。
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
