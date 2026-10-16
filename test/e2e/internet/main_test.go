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
	internet   *internetItem
	server     *serverItem
	disk       bool
	failStep   string
	calls      []string
	subnetJSON string
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
		return []byte(`[{"ID":14,"InternetID":12,"DefaultRoute":"192.0.2.1","NetworkAddress":"192.0.2.0","NetworkMaskLen":28,"IPAddresses":[{"IPAddress":"192.0.2.0"},{"IPAddress":"192.0.2.1"},{"IPAddress":"192.0.2.15"},{"IPAddress":"192.0.2.2"}]}]`), nil
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
		pollInterval: time.Millisecond,
		sshTimeout:   20 * time.Millisecond,
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
		"test-create-internet", "test-create-server", "test-list-subnet", "test-boot-server",
		"test-read-server", "cleanup-read-server", "cleanup-delete-server",
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
