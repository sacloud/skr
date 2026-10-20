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

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeGSLBCLI struct {
	item                *gslbItem
	failAt              string
	badCreate           bool
	changeOnCleanupRead bool
	server              *serverItem
	disk                *diskItem
	calls               []string
}

const testServerIP = "203.0.113.10"

func (f *fakeGSLBCLI) call(_ context.Context, step string, args []string, request any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if f.failAt == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "archive-preflight-read":
		return []byte(`{"ID":234,"Tags":["cloud-init"],"SizeMB":20480}`), nil
	case "server-before-find", "disk-before-find":
		return []byte("[]"), nil
	case "disk-cleanup-find-after", "disk-cleanup-find-final":
		if f.disk != nil {
			return json.Marshal([]diskItem{*f.disk})
		}
		return []byte("[]"), nil
	case "server-create":
		input := request.(map[string]any)
		f.server = &serverItem{
			ID: types.ID(456), Name: input["Name"].(string), Description: input["Description"].(string),
			InstanceStatus: "down",
			Interfaces:     []serverInterface{{IPAddress: testServerIP, UpstreamType: "shared"}},
		}
		f.disk = &diskItem{
			ID: 567, Name: f.server.Name, ServerID: f.server.ID,
			DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20480,
		}
		return json.Marshal(f.server)
	case "server-boot":
		if f.server == nil || argAfter(args, "--id") != f.server.ID.String() {
			return nil, errors.New("missing Server")
		}
		f.server.InstanceStatus = "up"
		return nil, nil
	case "server-read-boot", "server-read-up", "server-read-stopped", "server-cleanup-read":
		if f.server == nil || argAfter(args, "--id") != f.server.ID.String() {
			return nil, errors.New("missing Server")
		}
		return json.Marshal(f.server)
	case "server-cleanup-find", "server-cleanup-find-after":
		if f.server != nil {
			return json.Marshal([]serverItem{*f.server})
		}
		return []byte("[]"), nil
	case "server-cleanup-delete":
		for _, arg := range args {
			if arg == "--with-disks" {
				return nil, errors.New("must not delete unverified attached disks")
			}
		}
		if f.disk != nil {
			f.disk.ServerID = 0
		}
		f.server = nil
		return nil, nil
	case "disk-cleanup-read":
		if f.disk == nil || argAfter(args, "--id") != f.disk.ID.String() {
			return nil, errors.New("missing Disk")
		}
		return json.Marshal(f.disk)
	case "disk-cleanup-delete":
		if f.disk == nil || argAfter(args, "--id") != f.disk.ID.String() {
			return nil, errors.New("unsafe disk delete")
		}
		f.disk = nil
		return nil, nil
	case "before-find", "test-find", "cleanup-find", "cleanup-find-after":
		if f.item != nil && strings.Contains(strings.Join(args, " "), f.item.Name) {
			return json.Marshal([]gslbItem{*f.item})
		}
		return []byte("[]"), nil
	case "test-create":
		f.item = &gslbItem{
			ID:          types.ID(123),
			Name:        argAfter(args, "--name"),
			Description: argAfter(args, "--description"),
			FQDN:        "site-123.gslbN.sakura.ne.jp",
			DelayLoop:   10,
			HealthCheck: &iaas.GSLBHealthCheck{Protocol: types.GSLBHealthCheckProtocols.Ping},
			DestinationServers: iaas.GSLBServers{
				&iaas.GSLBServer{IPAddress: testServerIP},
			},
		}
		if f.badCreate {
			return []byte("{"), nil
		}
		return json.Marshal(f.item)
	case "test-read", "test-read-updated", "cleanup-read":
		if f.item == nil || argAfter(args, "--id") != f.item.ID.String() {
			return nil, errors.New("missing GSLB")
		}
		if step == "cleanup-read" && f.changeOnCleanupRead {
			f.item.Description = "changed by another actor"
		}
		return json.Marshal(f.item)
	case "test-update":
		if f.item == nil {
			return nil, errors.New("missing GSLB")
		}
		f.item.Description = argAfter(args, "--description")
		f.item.DelayLoop = 20
		return json.Marshal(f.item)
	case "cleanup-delete":
		if f.item == nil || argAfter(args, "--id") != f.item.ID.String() {
			return nil, errors.New("unsafe delete")
		}
		f.item = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected GSLB E2E step")
	}
}

func (f *fakeGSLBCLI) lookupDNS(_ context.Context, step, _ string) ([]string, error) {
	f.calls = append(f.calls, step)
	if f.server != nil && f.server.InstanceStatus.IsUp() {
		return []string{testServerIP}, nil
	}
	return nil, nil
}

func (f *fakeGSLBCLI) connectSSH(_ context.Context, step, _, _, _ string) error {
	f.calls = append(f.calls, step)
	return nil
}

func (f *fakeGSLBCLI) shutdownSSH(_ context.Context, step, _, _, _ string) error {
	f.calls = append(f.calls, step)
	if f.server == nil {
		return errors.New("missing Server")
	}
	f.server.InstanceStatus = "down"
	return nil
}

func (f *fakeGSLBCLI) testScenario() scenario {
	return scenario{
		call: f.call, lookupDNS: f.lookupDNS, connectSSH: f.connectSSH, shutdownSSH: f.shutdownSSH,
		serverName: namePrefix + "server-test", archiveID: 234, diskPlanID: 4, diskSizeGB: 20,
		userDataPath: "/tmp/user-data.yaml", privateKey: "/tmp/ssh-key", knownHosts: "/tmp/known-hosts",
		pollInterval: time.Millisecond, serverTimeout: time.Second, dnsTimeout: time.Second,
	}
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func TestScenarioCleansUpAfterSuccess(t *testing.T) {
	fake := &fakeGSLBCLI{}
	if err := fake.testScenario().run(context.Background(), namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil || fake.server != nil || fake.disk != nil {
		t.Fatal("temporary resources were not deleted")
	}
	if !containsStep(fake.calls, "test-create") || !containsStep(fake.calls, "test-update") ||
		!containsStep(fake.calls, "test-dns-healthy") || !containsStep(fake.calls, "server-shutdown") ||
		!containsStep(fake.calls, "test-dns-unhealthy") || !containsStep(fake.calls, "test-dns-recovered") ||
		!containsStep(fake.calls, "cleanup-delete") || !containsStep(fake.calls, "server-cleanup-delete") ||
		!containsStep(fake.calls, "disk-cleanup-read") || !containsStep(fake.calls, "disk-cleanup-delete") {
		t.Fatalf("scenario omitted required lifecycle steps: %v", fake.calls)
	}
}

func TestScenarioRefusesExistingGSLB(t *testing.T) {
	fake := &fakeGSLBCLI{item: &gslbItem{
		ID: types.ID(321), Name: namePrefix + "occupied", Description: "not owned",
	}}
	err := fake.testScenario().run(context.Background(), fake.item.Name)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want collision refusal", err)
	}
	if fake.item.ID != 321 || containsStep(fake.calls, "test-create") {
		t.Fatal("scenario changed an existing GSLB")
	}
}

func TestScenarioCleansUpAfterUpdateFailure(t *testing.T) {
	fake := &fakeGSLBCLI{failAt: "test-update"}
	err := fake.testScenario().run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("error = %v, want update failure", err)
	}
	if fake.item != nil || !containsStep(fake.calls, "cleanup-delete") {
		t.Fatal("scenario did not remove its created GSLB after failure")
	}
}

func TestScenarioRecoversWhenCreateOutputCannotBeDecoded(t *testing.T) {
	fake := &fakeGSLBCLI{badCreate: true}
	err := fake.testScenario().run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "decode GSLB") {
		t.Fatalf("error = %v, want decode failure", err)
	}
	if fake.item != nil || !containsStep(fake.calls, "cleanup-find") || !containsStep(fake.calls, "cleanup-delete") {
		t.Fatal("scenario did not recover and remove the created GSLB")
	}
}

func TestScenarioRefusesCleanupWithChangedIdentity(t *testing.T) {
	fake := &fakeGSLBCLI{failAt: "test-update", changeOnCleanupRead: true}
	err := fake.testScenario().run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("error = %v, want safe cleanup refusal", err)
	}
	if fake.item == nil || containsStep(fake.calls, "cleanup-delete") {
		t.Fatal("scenario deleted a GSLB with changed identity")
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

func TestScenarioRejectsNonCloudInitArchiveBeforeCreatingResources(t *testing.T) {
	fake := &fakeGSLBCLI{}
	test := fake.testScenario()
	test.call = func(ctx context.Context, step string, args []string, request any) ([]byte, error) {
		if step == "archive-preflight-read" {
			return []byte(`{"ID":234,"Tags":["os-linux"],"SizeMB":20480}`), nil
		}
		return fake.call(ctx, step, args, request)
	}
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "cloud-init tag") {
		t.Fatalf("expected cloud-init archive rejection, got %v", err)
	}
	if containsStep(fake.calls, "server-create") || containsStep(fake.calls, "test-create") {
		t.Fatal("created resources using a non-cloud-init archive")
	}
}

func TestParseDNSAnswers(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantIP  bool
		wantErr string
	}{
		{
			name: "healthy",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n" +
				"example.test. 10 IN A " + testServerIP + "\n",
			wantIP: true,
		},
		{
			name:   "no A records",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n",
		},
		{
			name:    "SERVFAIL",
			output:  ";; ->>HEADER<<- opcode: QUERY, status: SERVFAIL, id: 123\n",
			wantErr: "SERVFAIL",
		},
		{
			name:    "NXDOMAIN",
			output:  ";; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN, id: 123\n",
			wantErr: "NXDOMAIN",
		},
		{name: "empty output", wantErr: "no response header"},
		{
			name:    "short output is insufficient",
			output:  testServerIP + "\n",
			wantErr: "no response header",
		},
		{
			name:    "missing status",
			output:  ";; ->>HEADER<<- opcode: QUERY, id: 123\n",
			wantErr: "want NOERROR",
		},
		{
			name: "invalid A record",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n" +
				"example.test. 10 IN A invalid\n",
			wantErr: "invalid A record",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers, err := parseDNSAnswers(tt.output)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("answers = %v, error = %v, want %q", answers, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantIP {
				if len(answers) != 1 || answers[0] != testServerIP {
					t.Fatalf("answers = %v, want [%s]", answers, testServerIP)
				}
			} else if len(answers) != 0 {
				t.Fatalf("answers = %v, want no A records", answers)
			}
		})
	}
}

func TestScenarioRejectsDNSFailureAndCleansUp(t *testing.T) {
	fake := &fakeGSLBCLI{}
	test := fake.testScenario()
	test.lookupDNS = func(ctx context.Context, step, fqdn string) ([]string, error) {
		if step == "test-dns-unhealthy" {
			return parseDNSAnswers(";; ->>HEADER<<- opcode: QUERY, status: SERVFAIL, id: 123\n")
		}
		return fake.lookupDNS(ctx, step, fqdn)
	}
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "SERVFAIL") {
		t.Fatalf("error = %v, want DNS failure", err)
	}
	if fake.item != nil || fake.server != nil || fake.disk != nil {
		t.Fatal("temporary resources were not deleted after DNS failure")
	}
	if containsStep(fake.calls, "test-update") {
		t.Fatal("scenario continued after a failed DNS check")
	}
}

func TestCleanupRefusesUnverifiedDisk(t *testing.T) {
	tests := []struct {
		name   string
		change func(*diskItem)
	}{
		{"name", func(d *diskItem) { d.Name = "unrelated-disk" }},
		{"plan", func(d *diskItem) { d.DiskPlanID++ }},
		{"archive", func(d *diskItem) { d.SourceArchiveID++ }},
		{"size", func(d *diskItem) { d.SizeMB++ }},
		{"attached", func(d *diskItem) { d.ServerID = 999 }},
		{"missing ID", func(d *diskItem) { d.ID = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGSLBCLI{}
			test := fake.testScenario()
			test.call = func(ctx context.Context, step string, args []string, request any) ([]byte, error) {
				if step == "disk-cleanup-find-after" && fake.disk != nil {
					tt.change(fake.disk)
				}
				return fake.call(ctx, step, args, request)
			}
			err := test.run(context.Background(), namePrefix+"test")
			if err == nil || !strings.Contains(err.Error(), "cannot safely identify orphan Disk") {
				t.Fatalf("error = %v, want disk cleanup refusal", err)
			}
			if fake.disk == nil || containsStep(fake.calls, "disk-cleanup-delete") {
				t.Fatal("deleted an unverified disk")
			}
		})
	}
}

func TestCleanupRefusesDiskChangedBeforeDelete(t *testing.T) {
	fake := &fakeGSLBCLI{}
	test := fake.testScenario()
	test.call = func(ctx context.Context, step string, args []string, request any) ([]byte, error) {
		if step == "disk-cleanup-read" {
			fake.disk.ServerID = 999
		}
		return fake.call(ctx, step, args, request)
	}
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "Disk identity changed") {
		t.Fatalf("error = %v, want changed disk refusal", err)
	}
	if fake.disk == nil || containsStep(fake.calls, "disk-cleanup-delete") {
		t.Fatal("deleted a disk that changed before deletion")
	}
}

func TestDNSRunnerRecordsResponseFailures(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "dig"), []byte("#!/bin/sh\nprintf '%s\\n' \"$GSLB_TEST_DNS\"\n"), 0o700); err != nil { //nolint:gosec // The test helper must be executable and is created under t.TempDir.
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	for _, status := range []string{"NOERROR", "SERVFAIL", "NXDOMAIN"} {
		t.Run(status, func(t *testing.T) {
			t.Setenv("GSLB_TEST_DNS", ";; ->>HEADER<<- opcode: QUERY, status: "+status+", id: 123")
			recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			answers, err := (dnsRunner{evidence: recorder}).lookup(context.Background(), "dns-check", "example.test")
			if status == "NOERROR" {
				if err != nil || len(answers) != 0 {
					t.Fatalf("answers = %v, error = %v, want successful empty response", answers, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), status) {
				t.Fatalf("error = %v, want %s rejection", err, status)
			}
			data, readErr := os.ReadFile(filepath.Join(recorder.Dir(), "001-dns-check.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var recorded struct {
				Args  []string `json:"args"`
				Error string   `json:"error"`
			}
			if err := json.Unmarshal(data, &recorded); err != nil {
				t.Fatal(err)
			}
			if !containsStep(recorded.Args, "+comments") || !containsStep(recorded.Args, "+answer") ||
				containsStep(recorded.Args, "+short") {
				t.Fatalf("DNS arguments omit full response: %v", recorded.Args)
			}
			if (recorded.Error == "") != (status == "NOERROR") {
				t.Fatalf("evidence error = %q for status %s", recorded.Error, status)
			}
		})
	}
}
