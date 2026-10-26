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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

func TestInternetArgsBuildScopedRequests(t *testing.T) {
	request := map[string]any{"Zone": zone, "Name": "unique-router"}
	args, err := internetArgs("preflight-find-internet", request, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet find --zone is1b --name unique-router" {
		t.Fatalf("Internet find args = %v, error = %v", args, err)
	}
	args, err = internetArgs("test-create-internet", request, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet create --zone is1b --name unique-router --description "+internetDesc+" --network-mask-len 28 --band-width-mbps 100" {
		t.Fatalf("Internet create args = %v, error = %v", args, err)
	}
	args, err = internetArgs("cleanup-delete-internet", map[string]any{"Zone": zone, "ID": types.ID(12)}, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet delete --zone is1b --id 12 --force --fail-if-not-found" {
		t.Fatalf("Internet delete args = %v, error = %v", args, err)
	}
}

func TestServerCreateArgsUseRouterSwitchAndCloudImageDisk(t *testing.T) {
	args, err := internetArgs("test-create-server", map[string]any{
		"Zone": zone, "Name": "unique-router", "CloudimgArchiveID": int64(234),
		"DiskPlanID": int64(4), "DiskSizeGB": int64(20), "SwitchID": int64(13),
	}, "")
	want := "iaas-api server create --zone is1b --name unique-router --description " + serverDesc +
		" --cpu 1 --memory-gb 1 --disk archive-id=234,plan-id=4,size-gb=20 --nic upstream=13"
	if err != nil || strings.Join(args, " ") != want {
		t.Fatalf("Server create args = %v, error = %v", args, err)
	}
	for _, resource := range []string{"server", "disk"} {
		args, err := findArgs(resource, map[string]any{"Name": "name,with comma"})
		if err != nil || len(args) != 7 || args[6] != "name,with comma" {
			t.Fatalf("%s find args = %v, error = %v", resource, args, err)
		}
	}
}

func TestWriteUserDataConfiguresServerRouterNIC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-data.yaml")
	if err := writeUserData(path, "ssh-rsa AAAA temporary", "02:00:00:00:00:01", "192.0.2.2", 28, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("user-data permissions = %o, want 600", info.Mode().Perm())
	}
	//nolint:gosec // path is a fixed filename in this test's temporary directory.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"ssh-rsa AAAA temporary", "02:00:00:00:00:01", "192.0.2.2", "prefix=28", "192.0.2.1", "ip route replace default"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("user-data does not contain %q: %s", expected, data)
		}
	}
}

func TestWriteUserDataRejectsInvalidNetworkValues(t *testing.T) {
	for _, test := range []struct {
		publicKey string
		mac       string
		address   string
		prefix    int
		gateway   string
	}{
		{"not-a-key", "02:00:00:00:00:01", "192.0.2.2", 28, "192.0.2.1"},
		{"ssh-rsa AAAA temporary", "bad", "192.0.2.2", 28, "192.0.2.1"},
		{"ssh-rsa AAAA temporary", "02:00:00:00:00:01", "2001:db8::1", 64, "192.0.2.1"},
		{"ssh-rsa AAAA temporary", "02:00:00:00:00:01", "192.0.2.2", 33, "192.0.2.1"},
		{"ssh-rsa AAAA temporary", "02:00:00:00:00:01", "192.0.2.2", 28, "bad"},
	} {
		err := writeUserData(filepath.Join(t.TempDir(), "user-data.yaml"), test.publicKey, test.mac, test.address, test.prefix, test.gateway)
		if err == nil {
			t.Errorf("accepted invalid network values: %+v", test)
		}
	}
}

type fakeCLI struct {
	internet    *internetItem
	server      *serverItem
	disk        bool
	failStep    string
	calls       []string
	subnetJSON  string
	addedSubnet *subnetResponse
	ipv6Net     *ipv6NetItem
}

func (f *fakeCLI) call(_ context.Context, step string, request map[string]any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failStep {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "preflight-find-internet", "cleanup-find-internet", "cleanup-find-internet-after":
		if f.internet != nil && f.internet.Name == request["Name"] {
			return json.Marshal([]internetItem{*f.internet})
		}
		return []byte("[]"), nil
	case "test-create-internet":
		f.internet = &internetItem{
			ID: 12, Name: request["Name"].(string), Description: internetDesc,
			NetworkMaskLen: 28, BandWidthMbps: 100,
		}
		f.internet.Switch.ID = 13
		return json.Marshal(f.internet)
	case "test-read-internet", "cleanup-read-internet":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		return json.Marshal(f.internet)
	case "test-list-subnet":
		if f.subnetJSON != "" {
			return []byte(f.subnetJSON), nil
		}
		return f.subnetListJSON()
	case "test-add-subnet":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		maskLen, err := requestInt64(request, "NetworkMaskLen")
		if err != nil || maskLen != addedSubnetMaskLen {
			return nil, errors.New("add-subnet requires NetworkMaskLen 28")
		}
		nextHop, err := requestNextHop(request)
		if err != nil {
			return nil, err
		}
		if f.addedSubnet != nil {
			return nil, errors.New("an added Subnet already exists")
		}
		f.addedSubnet = &subnetResponse{ID: 15, NextHop: nextHop, NetworkAddress: "198.51.100.0", NetworkMaskLen: addedSubnetMaskLen}
		return json.Marshal(f.addedSubnet)
	case "test-update-subnet":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		subnetID, err := requestInt64(request, "SubnetID")
		if err != nil || f.addedSubnet == nil || subnetID != int64(f.addedSubnet.ID) {
			return nil, errors.New("missing added Subnet")
		}
		nextHop, err := requestNextHop(request)
		if err != nil {
			return nil, err
		}
		f.addedSubnet.NextHop = nextHop
		return json.Marshal(f.addedSubnet)
	case "test-delete-subnet":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		subnetID, err := requestInt64(request, "SubnetID")
		if err != nil || f.addedSubnet == nil || subnetID != int64(f.addedSubnet.ID) {
			return nil, errors.New("missing added Subnet")
		}
		f.addedSubnet = nil
		return nil, nil
	case "test-enable-ipv6", "test-read-ipv6", "test-disable-ipv6":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		switch step {
		case "test-enable-ipv6":
			if f.ipv6Net != nil {
				return nil, errors.New("IPv6Net is already enabled")
			}
			f.ipv6Net = &ipv6NetItem{ID: 16, IPv6Prefix: "2001:db8::", IPv6PrefixLen: 64}
			return json.Marshal(f.ipv6Net)
		case "test-read-ipv6":
			if f.ipv6Net == nil {
				return nil, errors.New("IPv6Net is not enabled")
			}
			return json.Marshal(f.ipv6Net)
		default:
			if f.ipv6Net == nil {
				return nil, errors.New("IPv6Net is not enabled")
			}
			f.ipv6Net = nil
			return nil, nil
		}
	case "preflight-find-server", "preflight-find-disk", "cleanup-find-server", "cleanup-find-server-after", "cleanup-find-disk-after":
		if (step == "preflight-find-server" || step == "cleanup-find-server" || step == "cleanup-find-server-after") &&
			f.server != nil && f.server.Name == request["Name"] {
			return json.Marshal([]serverItem{*f.server})
		}
		if (step == "preflight-find-disk" || step == "cleanup-find-disk-after") && f.disk {
			return json.Marshal([]struct {
				ID   types.ID `json:"ID"`
				Name string   `json:"Name"`
			}{{ID: 45, Name: request["Name"].(string)}})
		}
		return []byte("[]"), nil
	case "test-create-server":
		f.server = &serverItem{
			ID: 34, Name: request["Name"].(string), Description: serverDesc,
			Interfaces: []serverInterface{{
				ID: 35, MACAddress: "02:00:00:00:00:01", SwitchID: 13,
			}},
			Disks: []struct {
				ID types.ID `json:"ID"`
			}{{ID: 45}},
		}
		f.disk = true
		return json.Marshal(f.server)
	case "test-boot-server":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		f.server.InstanceStatus = "up"
		data, err := os.ReadFile(request["UserDataPath"].(string))
		if err != nil || !strings.Contains(string(data), "192.0.2.2") {
			return nil, errors.New("missing Router + Switch network configuration")
		}
		return nil, nil
	case "test-read-server", "cleanup-read-server":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		return json.Marshal(f.server)
	case "cleanup-delete-server":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		f.server = nil
		f.disk = false
		return nil, nil
	case "test-update-internet", "test-update-internet-restore":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		name, ok := request["Name"].(string)
		if !ok || name == "" {
			return nil, errors.New("missing Name")
		}
		desc, ok := request["Description"].(string)
		if !ok || desc == "" {
			return nil, errors.New("missing Description")
		}
		f.internet.Name = name
		f.internet.Description = desc
		return nil, nil
	case "test-monitor-router":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		return []byte(`[{"Time":"2026-01-01T00:00:00Z","In":0,"Out":0}]`), nil
	case "cleanup-delete-internet":
		if f.internet == nil || request["ID"] != f.internet.ID {
			return nil, errors.New("missing Internet")
		}
		f.internet = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func (f *fakeCLI) subnetListJSON() ([]byte, error) {
	subnets := []subnetItem{{
		ID: 14, InternetID: 12, DefaultRoute: "192.0.2.1", NetworkAddress: "192.0.2.0",
		NetworkMaskLen: 28,
		IPAddresses: []struct {
			IPAddress string `json:"IPAddress"`
		}{{IPAddress: "192.0.2.2"}, {IPAddress: "192.0.2.1"}},
	}}
	if f.addedSubnet != nil {
		subnets = append(subnets, subnetItem{
			ID: f.addedSubnet.ID, InternetID: 12, NextHop: f.addedSubnet.NextHop,
			NetworkAddress: f.addedSubnet.NetworkAddress, NetworkMaskLen: f.addedSubnet.NetworkMaskLen,
		})
	}
	return json.Marshal(subnets)
}

func testScenario(fake *fakeCLI, dir string) scenario {
	return scenario{
		client: fake, archiveID: 234, diskPlanID: 4, diskSizeGB: 20,
		userDataPath: filepath.Join(dir, "user-data.yaml"),
		publicKey:    "ssh-rsa AAAA temporary",
		privateKey:   filepath.Join(dir, "ssh-key"),
		knownHosts:   filepath.Join(dir, "known_hosts"),
		connect: func(context.Context, string, string, string) error {
			return nil
		},
		pollInterval:    time.Millisecond,
		sshTimeout:      20 * time.Millisecond,
		recoveryTimeout: 100 * time.Millisecond,
	}
}

type cliFunc func(context.Context, string, map[string]any) ([]byte, error)

func (f cliFunc) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	return f(ctx, step, request)
}

func TestOperationTimeoutBudgets(t *testing.T) {
	for _, step := range []string{"test-create-internet", "test-create-server"} {
		if got := timeoutForStep(step); got <= 30*time.Minute {
			t.Fatalf("%s timeout = %s, want more than SDK's 30 minute visibility retry window", step, got)
		}
	}
	if timeoutForStep("test-read-server") != commandTimeout {
		t.Fatal("normal operations must retain their short timeout")
	}
	if scenarioTimeout <= 2*createTimeout+sshWaitTimeout {
		t.Fatal("scenario timeout must allow both creates, SSH waiting, and other operations")
	}
}

func TestScenarioRecoversInterruptedInternetCreation(t *testing.T) {
	for _, mode := range []string{"cancelled", "invalid output"} {
		t.Run(mode, func(t *testing.T) {
			fake := &fakeCLI{}
			test := testScenario(fake, t.TempDir())
			test.recoveryTimeout = time.Second
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			searches, reads := 0, 0
			test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
				switch step {
				case "test-create-internet":
					data, err := fake.call(ctx, step, request)
					if err != nil {
						return nil, err
					}
					if mode == "invalid output" {
						return []byte("{"), nil
					}
					cancel()
					return data, context.Canceled
				case "cleanup-find-internet":
					if ctx.Err() != nil {
						t.Fatal("recovery reused cancelled scenario context")
					}
					searches++
					if searches == 1 {
						return nil, errors.New("temporary search failure")
					}
					if searches == 2 {
						return []byte("[]"), nil
					}
				case "cleanup-read-internet":
					reads++
					if reads == 1 {
						return nil, errors.New("created Internet temporarily unreadable")
					}
				}
				return fake.call(ctx, step, request)
			})
			err := test.run(ctx, namePrefix+"test")
			if err == nil || (mode == "cancelled" && !errors.Is(err, context.Canceled)) {
				t.Fatalf("original failure was lost: %v", err)
			}
			if strings.Contains(err.Error(), "cleanup failed") || searches < 4 || reads < 2 {
				t.Fatalf("recovery did not complete: searches=%d reads=%d error=%v", searches, reads, err)
			}
			if fake.internet != nil || fake.server != nil || fake.disk || !containsStep(fake.calls, "cleanup-delete-internet") {
				t.Fatalf("resources not cleaned: %+v", fake)
			}
		})
	}
}

func TestInternetRecoveryRefusesUnsafeMatches(t *testing.T) {
	for _, mode := range []string{"ambiguous", "identity", "read identity"} {
		t.Run(mode, func(t *testing.T) {
			fake := &fakeCLI{}
			test := testScenario(fake, t.TempDir())
			test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
				data, err := fake.call(ctx, step, request)
				if step == "test-create-internet" {
					return nil, context.DeadlineExceeded
				}
				if step == "cleanup-find-internet" {
					switch mode {
					case "ambiguous":
						return json.Marshal([]internetItem{*fake.internet, *fake.internet})
					case "identity":
						item := *fake.internet
						item.Description = "unrelated"
						return json.Marshal([]internetItem{item})
					}
				}
				if step == "cleanup-read-internet" && mode == "read identity" {
					item := *fake.internet
					item.Name = "unrelated"
					return json.Marshal(item)
				}
				return data, err
			})
			err := test.run(context.Background(), namePrefix+"test")
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "manual cleanup required") {
				t.Fatalf("error = %v", err)
			}
			if fake.internet == nil || containsStep(fake.calls, "cleanup-delete-internet") {
				t.Fatal("unsafe match was deleted")
			}
		})
	}
}

func TestInternetRecoveryExpiresWithExplicitFailure(t *testing.T) {
	for _, mode := range []string{"empty", "search error", "read error"} {
		t.Run(mode, func(t *testing.T) {
			fake := &fakeCLI{}
			test := testScenario(fake, t.TempDir())
			attempts := 0
			test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
				if step == "test-create-internet" {
					if mode == "read error" {
						if _, err := fake.call(ctx, step, request); err != nil {
							return nil, err
						}
					}
					return nil, context.Canceled
				}
				if step == "cleanup-find-internet" {
					attempts++
					if mode == "search error" {
						return nil, errors.New("search unavailable")
					}
				}
				if step == "cleanup-read-internet" && mode == "read error" {
					return nil, errors.New("read unavailable")
				}
				return fake.call(ctx, step, request)
			})
			err := test.run(context.Background(), namePrefix+"test")
			if !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) ||
				!strings.Contains(err.Error(), "manual cleanup required") || attempts < 2 {
				t.Fatalf("attempts = %d, error = %v", attempts, err)
			}
			if containsStep(fake.calls, "cleanup-delete-internet") {
				t.Fatal("deleted an unverified resource")
			}
		})
	}
}

func TestCleanupContinuesAfterServerDeletionResponseFailure(t *testing.T) {
	for _, internetFails := range []bool{false, true} {
		t.Run(fmt.Sprint(internetFails), func(t *testing.T) {
			fake := &fakeCLI{}
			test := testScenario(fake, t.TempDir())
			deleteErr := errors.New("Server deletion response or evidence failed")
			internetErr := errors.New("Internet deletion failed")
			test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
				if step == "cleanup-delete-internet" && internetFails {
					return nil, internetErr
				}
				data, err := fake.call(ctx, step, request)
				if step == "cleanup-delete-server" && err == nil {
					return nil, deleteErr
				}
				return data, err
			})
			err := test.run(context.Background(), namePrefix+"test")
			if !errors.Is(err, deleteErr) || errors.Is(err, internetErr) != internetFails {
				t.Fatalf("cleanup errors were not preserved: %v", err)
			}
			if fake.server != nil || fake.disk || (fake.internet != nil) != internetFails ||
				!containsStep(fake.calls, "cleanup-read-internet") {
				t.Fatalf("unexpected cleanup state: %+v", fake)
			}
		})
	}
}

func TestScenarioCreatesRouterServerAndCleansEverything(t *testing.T) {
	fake := &fakeCLI{}
	test := testScenario(fake, t.TempDir())
	connected := false
	test.connect = func(_ context.Context, address, _, _ string) error {
		if address != "192.0.2.2" {
			t.Fatalf("SSH address = %q, want usable subnet address", address)
		}
		connected = true
		return nil
	}
	if err := test.run(context.Background(), namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	if !connected {
		t.Fatal("SSH was not attempted")
	}
	if fake.internet != nil || fake.server != nil || fake.disk {
		t.Fatalf("resources after E2E: Internet=%+v Server=%+v Disk=%v", fake.internet, fake.server, fake.disk)
	}
	for _, step := range []string{
		"preflight-find-internet", "preflight-find-server", "preflight-find-disk",
		"test-create-internet", "test-update-internet", "test-update-internet-restore",
		"test-monitor-router", "test-create-server", "test-list-subnet", "test-boot-server",
		"test-read-server", "test-add-subnet", "test-update-subnet", "test-delete-subnet",
		"test-enable-ipv6", "test-read-ipv6", "test-disable-ipv6",
		"cleanup-read-server", "cleanup-delete-server",
		"cleanup-find-server-after", "cleanup-find-disk-after", "cleanup-read-internet",
		"cleanup-delete-internet", "cleanup-find-internet-after",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestScenarioCleansInternetAfterServerCreateFailure(t *testing.T) {
	fake := &fakeCLI{failStep: "test-create-server"}
	test := testScenario(fake, t.TempDir())
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want injected CLI failure", err)
	}
	if fake.internet != nil || fake.server != nil || fake.disk {
		t.Fatalf("resources after failed E2E: Internet=%+v Server=%+v Disk=%v", fake.internet, fake.server, fake.disk)
	}
	if !containsStep(fake.calls, "cleanup-delete-internet") {
		t.Fatal("Internet was not deleted after Server creation failed")
	}
}

func TestNetworkRejectsMissingOrUnusableSubnetAddresses(t *testing.T) {
	for _, addresses := range []string{
		`[]`,
		`[{"IPAddress":"192.0.2.0"},{"IPAddress":"192.0.2.1"},{"IPAddress":"192.0.2.15"}]`,
		`[{"IPAddress":"198.51.100.2"},{"IPAddress":"invalid"},{"IPAddress":"2001:db8::1"}]`,
	} {
		fake := &fakeCLI{subnetJSON: `[{"InternetID":12,"DefaultRoute":"192.0.2.1","NetworkAddress":"192.0.2.0","NetworkMaskLen":28,"IPAddresses":` + addresses + `}]`}
		test := testScenario(fake, t.TempDir())
		_, _, _, err := test.network(context.Background(), 12, 13, serverItem{
			Interfaces: []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01", IPAddress: "192.0.2.2"}},
		})
		if err == nil || !strings.Contains(err.Error(), "no usable Server IPv4 address") {
			t.Fatalf("addresses %s: error = %v, want refusal without falling back to NIC IPAddress", addresses, err)
		}
	}
}

func TestScenarioRefusesExistingInternet(t *testing.T) {
	fake := &fakeCLI{internet: &internetItem{
		ID: 9, Name: namePrefix + "test", Description: "unrelated Internet",
	}}
	fake.internet.Switch.ID = 10
	err := testScenario(fake, t.TempDir()).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want name collision refusal", err)
	}
	if len(fake.calls) != 2 || fake.internet.Description != "unrelated Internet" {
		t.Fatalf("existing Internet was modified: calls=%v Internet=%+v", fake.calls, fake.internet)
	}
}

func TestScenarioDoesNotDeleteInternetIfServerCleanupFails(t *testing.T) {
	fake := &fakeCLI{failStep: "cleanup-delete-server"}
	test := testScenario(fake, t.TempDir())
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want cleanup failure", err)
	}
	if fake.server == nil || fake.internet == nil || containsStep(fake.calls, "cleanup-delete-internet") {
		t.Fatalf("unsafe cleanup state: Internet=%+v Server=%+v calls=%v", fake.internet, fake.server, fake.calls)
	}
}

func containsStep(steps []string, want string) bool {
	for _, step := range steps {
		if step == want {
			return true
		}
	}
	return false
}

func TestFinalizeRunRecordsFailureWhenCleanupFails(t *testing.T) {
	code, err := finalizeRun(0, func() error { return errors.New("cleanup failed") }, func(result string) error {
		if result != "failed" {
			t.Fatalf("result = %q, want failed", result)
		}
		return nil
	})
	if code != 1 || err == nil {
		t.Fatalf("code=%d error=%v, want failure", code, err)
	}
}

func TestScenarioWaitsForFailedSSHAndCleansResources(t *testing.T) {
	fake := &fakeCLI{}
	test := testScenario(fake, t.TempDir())
	test.connect = func(context.Context, string, string, string) error {
		return errors.New("SSH unavailable")
	}
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "SSH login check timed out") {
		t.Fatalf("scenario error = %v, want SSH timeout", err)
	}
	if fake.internet != nil || fake.server != nil || fake.disk {
		t.Fatalf("resources after failed SSH were not cleaned up: Internet=%+v Server=%+v Disk=%v", fake.internet, fake.server, fake.disk)
	}
}
