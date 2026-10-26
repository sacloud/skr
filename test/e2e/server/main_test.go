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
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

func TestFinalizeRunCleansUpBeforeRecordingResult(t *testing.T) {
	cleanupFailure := errors.New("cleanup failed")
	recordFailure := errors.New("record failed")
	for _, test := range []struct {
		name       string
		exitCode   int
		cleanupErr error
		recordErr  error
		wantCode   int
		wantResult string
	}{
		{"success", 0, nil, nil, 0, "passed"},
		{"scenario failure", 1, nil, nil, 1, "failed"},
		{"cleanup failure", 0, cleanupFailure, nil, 1, "failed"},
		{"record failure", 0, nil, recordFailure, 1, "passed"},
		{"both failures", 0, cleanupFailure, recordFailure, 1, "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleaned := false
			code, err := finalizeRun(test.exitCode, func() error {
				cleaned = true
				return test.cleanupErr
			}, func(result string) error {
				if !cleaned || result != test.wantResult {
					t.Fatalf("cleanup completed=%v, result=%q; want %q", cleaned, result, test.wantResult)
				}
				return test.recordErr
			})
			if code != test.wantCode || (err != nil) != (test.cleanupErr != nil || test.recordErr != nil) {
				t.Fatalf("code=%d error=%v", code, err)
			}
			for _, expected := range []error{test.cleanupErr, test.recordErr} {
				if expected != nil && !errors.Is(err, expected) {
					t.Fatalf("error does not preserve %v: %v", expected, err)
				}
			}
		})
	}
}

func TestFinalizeRunBeforeRecorderCreation(t *testing.T) {
	cleaned := false
	code, err := finalizeRun(1, func() error {
		cleaned = true
		return nil
	}, nil)
	if !cleaned || code != 1 || err != nil {
		t.Fatalf("cleaned=%v code=%d error=%v", cleaned, code, err)
	}
}

func TestSSHRunnerRecordsLoginResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		stdout string
		stderr string
		err    error
	}{
		{name: "success", stdout: "test-server\n"},
		{name: "connection-failure", stderr: "Connection refused", err: errors.New("SSH exited")},
		{name: "empty-hostname"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder, err := evidence.CreateAt(t.TempDir(), time.Now(), evidence.Scenario{})
			if err != nil {
				t.Fatal(err)
			}
			runner := sshRunner{
				evidence: recorder,
				run: func(_ context.Context, args []string) (string, string, error) {
					if args[len(args)-1] != "hostname" {
						t.Fatalf("unexpected SSH arguments: %v", args)
					}
					return test.stdout, test.stderr, test.err
				},
			}
			err = runner.connect(context.Background(), sshUser, "203.0.113.1", "/tmp/key", "/tmp/known_hosts")
			wantFailure := test.err != nil || test.stdout == ""
			if (err != nil) != wantFailure {
				t.Fatalf("connect error = %v, wantFailure = %v", err, wantFailure)
			}
			data, err := os.ReadFile(filepath.Join(recorder.Dir(), "001-test-ssh-login.json"))
			if err != nil {
				t.Fatal(err)
			}
			var entry struct {
				Step       string   `json:"step"`
				Args       []string `json:"args"`
				Stdout     string   `json:"stdout"`
				Stderr     string   `json:"stderr"`
				Error      string   `json:"error"`
				ExitStatus *int     `json:"exit_status"`
			}
			if err := json.Unmarshal(data, &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Step != "test-ssh-login" || entry.Stdout != test.stdout || entry.Stderr != test.stderr ||
				(entry.Error != "") != wantFailure {
				t.Fatalf("unexpected evidence: %+v", entry)
			}
			if !wantFailure && (entry.ExitStatus == nil || *entry.ExitStatus != 0) {
				t.Fatalf("successful SSH exit status = %v", entry.ExitStatus)
			}
			if err := recorder.SetResult("passed", time.Now()); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"ORDER.txt", "REPORT.md"} {
				//nolint:gosec // Fixed evidence filenames inside this test's temporary directory.
				data, err := os.ReadFile(filepath.Join(recorder.Dir(), file))
				if err != nil || !strings.Contains(string(data), "test-ssh-login") {
					t.Fatalf("%s does not contain SSH evidence: %v", file, err)
				}
			}
		})
	}
}

func TestSSHEvidenceFailureStopsRetriesAndCleansUp(t *testing.T) {
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now(), evidence.Scenario{})
	if err != nil {
		t.Fatal(err)
	}
	// Replace the expected evidence file with a directory to force a write failure.
	if err := os.Mkdir(filepath.Join(recorder.Dir(), "001-test-ssh-login.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	runner := sshRunner{
		evidence: recorder,
		run: func(context.Context, []string) (string, string, error) {
			attempts++
			return "test-server\n", "", nil
		},
	}
	fake := &fakeCLI{}
	test := testScenario(fake)
	test.connect = runner.connect
	err = test.run(context.Background(), namePrefix+"test")
	if !errors.Is(err, errSSHEvidence) || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d; want evidence failure without retry", err, attempts)
	}
	if fake.server != nil || fake.disk || len(fake.deleted) != 1 {
		t.Fatal("resources were not cleaned up after evidence failure")
	}
}

type fakeCLI struct {
	server                    *serverItem
	disk                      bool
	failStep                  string
	failCreateAfterSideEffect bool
	changeIdentityAtCleanup   bool
	deleted                   []types.ID
	calls                     []string
	leaveDisk                 bool
	createDiskOnly            bool
	orphanDisk                *diskItem
	deletedDisks              []types.ID
	multipleOrphans           bool
	deleteServerError         bool
	keepOrphan                bool
}

func (f *fakeCLI) call(_ context.Context, step string, request map[string]any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failStep && !(step == "test-create" && f.failCreateAfterSideEffect) {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "preflight-find-server", "preflight-find-disk", "cleanup-find-server", "cleanup-find-server-after", "cleanup-find-disk-after", "cleanup-find-disk-final":
		if step == "preflight-find-server" || step == "cleanup-find-server" || step == "cleanup-find-server-after" {
			if f.server != nil && f.server.Name == request["Name"] {
				return json.Marshal([]serverItem{*f.server})
			}
		} else if f.disk {
			if f.multipleOrphans && step == "cleanup-find-disk-after" {
				return json.Marshal([]diskItem{{ID: 45, Name: request["Name"].(string)}, {ID: 46, Name: request["Name"].(string)}})
			}
			return json.Marshal([]struct {
				ID   types.ID `json:"ID"`
				Name string   `json:"Name"`
			}{{ID: 45, Name: request["Name"].(string)}})
		}
		return []byte("[]"), nil
	case "test-create":
		f.server = &serverItem{
			ID: 123, Name: request["Name"].(string), Description: description,
			Disks: []diskItem{{ID: 45}},
		}
		f.disk = true
		if f.createDiskOnly {
			f.server = nil
			return nil, errors.New("injected partial create failure")
		}
		output, err := json.Marshal(f.server)
		if f.failStep == step {
			return nil, errors.New("injected CLI failure")
		}
		return output, err
	case "test-boot":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		f.server.InstanceStatus = "up"
		f.server.Interfaces = []serverInterface{{IPAddress: "203.0.113.1", UpstreamType: "shared"}}
		return nil, nil
	case "test-read", "cleanup-read":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		item := *f.server
		if step == "cleanup-read" && f.changeIdentityAtCleanup {
			item.Description = "changed identity"
		}
		return json.Marshal(item)
	case "cleanup-delete":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		f.deleted = append(f.deleted, f.server.ID)
		f.server = nil
		if !f.leaveDisk {
			f.disk = false
		}
		if f.deleteServerError {
			return nil, errors.New("injected partial delete failure")
		}
		return nil, nil
	case "test-update", "test-update-restore":
		if f.server == nil || request["ID"] != f.server.ID {
			return nil, errors.New("missing Server")
		}
		name, ok := request["Name"].(string)
		if !ok || name == "" {
			return nil, errors.New("missing Name")
		}
		desc, ok := request["Description"].(string)
		if !ok || desc == "" {
			return nil, errors.New("missing Description")
		}
		f.server.Name = name
		f.server.Description = desc
		return nil, nil
	case "cleanup-read-disk":
		disk := diskItem{ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024}
		if f.orphanDisk != nil {
			disk = *f.orphanDisk
		}
		return json.Marshal(disk)
	case "cleanup-delete-disk":
		f.deletedDisks = append(f.deletedDisks, request["ID"].(types.ID))
		if !f.keepOrphan {
			f.disk = false
		}
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func testScenario(fake *fakeCLI) scenario {
	return scenario{
		client: fake, archiveID: 234, diskPlanID: 4, diskSizeGB: 20,
		userDataPath: "/tmp/user-data.yaml", privateKey: "/tmp/ssh-key",
		knownHosts: "/tmp/known_hosts",
		connect: func(context.Context, string, string, string, string) error {
			return nil
		},
		pollInterval: time.Millisecond,
		sshTimeout:   20 * time.Millisecond,
	}
}

func TestScenarioCreatesBootsSSHAndCleansServerAndDisk(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(fake).run(context.Background(), namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	if fake.server != nil || fake.disk || len(fake.deleted) != 1 || fake.deleted[0] != 123 {
		t.Fatalf("resources after E2E: server=%+v disk=%v deleted=%v", fake.server, fake.disk, fake.deleted)
	}
	for _, step := range []string{
		"preflight-find-server", "preflight-find-disk", "test-create", "test-boot",
		"test-read", "test-update", "test-update-restore",
		"cleanup-read", "cleanup-delete", "cleanup-find-server-after", "cleanup-find-disk-after",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestScenarioCleansUpAfterCreateReturnsError(t *testing.T) {
	fake := &fakeCLI{failStep: "test-create", failCreateAfterSideEffect: true}
	err := testScenario(fake).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want injected CLI failure", err)
	}
	if fake.server != nil || fake.disk || len(fake.deleted) != 1 {
		t.Fatalf("partially created resources not cleaned up: server=%+v disk=%v deleted=%v", fake.server, fake.disk, fake.deleted)
	}
}

func TestScenarioDeletesVerifiedOrphanDisk(t *testing.T) {
	fake := &fakeCLI{leaveDisk: true}
	if err := testScenario(fake).run(context.Background(), namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	if fake.disk || len(fake.deletedDisks) != 1 || fake.deletedDisks[0] != 45 {
		t.Fatalf("orphan cleanup failed: %+v", fake)
	}
	for _, step := range []string{"cleanup-read-disk", "cleanup-delete-disk", "cleanup-find-disk-final"} {
		if !containsStep(fake.calls, step) {
			t.Errorf("missing cleanup step %s", step)
		}
	}
}

func TestScenarioRefusesUnverifiedOrphanDisk(t *testing.T) {
	for _, disk := range []diskItem{
		{ID: 46, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		{ID: 45, Name: "different", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		{ID: 45, Name: namePrefix + "test", ServerID: 999, DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		{ID: 45, Name: namePrefix + "test", DiskPlanID: 5, SourceArchiveID: 234, SizeMB: 20 * 1024},
		{ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 235, SizeMB: 20 * 1024},
		{ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 100 * 1024},
	} {
		fake := &fakeCLI{leaveDisk: true, orphanDisk: &disk}
		err := testScenario(fake).run(context.Background(), namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "manual cleanup required") || !fake.disk || len(fake.deletedDisks) != 0 {
			t.Fatalf("unsafe orphan deletion: disk=%+v error=%v", disk, err)
		}
	}
}

func TestScenarioReportsDiskWhenPartialCreateHasNoServer(t *testing.T) {
	fake := &fakeCLI{createDiskOnly: true}
	err := testScenario(fake).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "cannot safely identify orphan Disk") ||
		!containsStep(fake.calls, "cleanup-find-disk-after") || !fake.disk || len(fake.deletedDisks) != 0 {
		t.Fatalf("unidentified disk was ignored or deleted: error=%v calls=%v", err, fake.calls)
	}
}

func TestScenarioReportsOrphanCleanupFailures(t *testing.T) {
	for _, step := range []string{"cleanup-find-disk-after", "cleanup-read-disk", "cleanup-delete-disk", "cleanup-find-disk-final"} {
		fake := &fakeCLI{leaveDisk: true, failStep: step}
		err := testScenario(fake).run(context.Background(), namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("%s: error=%v", step, err)
		}
	}
}

func TestScenarioRejectsAmbiguousOrRemainingOrphans(t *testing.T) {
	for _, fake := range []*fakeCLI{
		{leaveDisk: true, multipleOrphans: true},
		{leaveDisk: true, keepOrphan: true},
	} {
		err := testScenario(fake).run(context.Background(), namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "manual cleanup required") || !fake.disk {
			t.Fatalf("orphan not reported: error=%v", err)
		}
		if fake.multipleOrphans && len(fake.deletedDisks) != 0 {
			t.Fatal("ambiguous disk was deleted")
		}
	}
}

func TestScenarioCleansDiskAfterPartialServerDelete(t *testing.T) {
	fake := &fakeCLI{leaveDisk: true, deleteServerError: true}
	err := testScenario(fake).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "injected partial delete failure") ||
		fake.disk || len(fake.deletedDisks) != 1 {
		t.Fatalf("partial delete cleanup: error=%v deleted=%v", err, fake.deletedDisks)
	}
}

type paginatedCLI struct {
	calls int
}

func (f *paginatedCLI) call(_ context.Context, _ string, request map[string]any) ([]byte, error) {
	f.calls++
	if request["Count"] != 100 || request["From"] != (f.calls-1)*100 {
		return nil, errors.New("unexpected pagination")
	}
	if f.calls == 2 {
		return []byte("[]"), nil
	}
	return json.Marshal(make([]diskItem, 100))
}

func TestFindReadsAllPages(t *testing.T) {
	fake := &paginatedCLI{}
	items, err := (scenario{client: fake}).find(context.Background(), "preflight-find-disk", "disk", namePrefix+"test")
	if err != nil || len(items) != 100 || fake.calls != 2 {
		t.Fatalf("pagination: items=%d calls=%d error=%v", len(items), fake.calls, err)
	}
}

func TestScenarioRefusesExistingServer(t *testing.T) {
	fake := &fakeCLI{server: &serverItem{
		ID: 456, Name: namePrefix + "test", Description: "unrelated Server",
	}}
	err := testScenario(fake).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want name collision refusal", err)
	}
	if len(fake.calls) != 2 || fake.server.Description != "unrelated Server" || len(fake.deleted) != 0 {
		t.Fatalf("existing Server was modified: calls=%v server=%+v", fake.calls, fake.server)
	}
}

func TestScenarioRefusesCleanupAfterIdentityChange(t *testing.T) {
	fake := &fakeCLI{changeIdentityAtCleanup: true}
	err := testScenario(fake).run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup refusal", err)
	}
	if fake.server == nil || len(fake.deleted) != 0 {
		t.Fatalf("Server with changed identity was deleted: server=%+v deleted=%v", fake.server, fake.deleted)
	}
}

func TestScenarioTimesOutWhenSSHLoginFailsAndCleansUp(t *testing.T) {
	fake := &fakeCLI{}
	test := testScenario(fake)
	test.connect = func(context.Context, string, string, string, string) error {
		return errors.New("SSH unavailable")
	}
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "SSH login check timed out") {
		t.Fatalf("scenario error = %v, want SSH timeout", err)
	}
	if fake.server != nil || fake.disk || len(fake.deleted) != 1 {
		t.Fatalf("resources after failed SSH were not cleaned up: server=%+v disk=%v deleted=%v", fake.server, fake.disk, fake.deleted)
	}
}

func TestServerArgsUseDedicatedOptionsAndDoNotIncludeUserData(t *testing.T) {
	args, err := serverArgs("test-create", map[string]any{
		"Zone": zone, "Name": "server-name", "CloudimgArchiveID": int64(234),
		"DiskPlanID": int64(4), "DiskSizeGB": int64(20),
	}, "/tmp/user-data.yaml")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"--disk archive-id=234,plan-id=4,size-gb=20", "--nic upstream=shared",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("Create args %q do not include %q", joined, expected)
		}
	}
	args, err = serverArgs("test-boot", map[string]any{"Zone": zone, "ID": types.ID(123), "UserDataPath": "/tmp/user-data.yaml"}, "/tmp/user-data.yaml")
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--user-data @/tmp/user-data.yaml") || strings.Contains(joined, "ssh_authorized_keys") {
		t.Errorf("Boot args = %q, want path-only user-data input", joined)
	}
}

func TestServerArgsRejectInvalidZoneAndMissingOptions(t *testing.T) {
	for _, request := range []map[string]any{
		{"Zone": "tk1v", "Name": "server", "CloudimgArchiveID": int64(234), "DiskPlanID": int64(4), "DiskSizeGB": int64(20)},
		{"Zone": zone, "Name": "server", "CloudimgArchiveID": int64(234), "DiskPlanID": int64(4)},
	} {
		if _, err := serverArgs("test-create", request, ""); err == nil {
			t.Errorf("serverArgs accepted invalid request: %+v", request)
		}
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
