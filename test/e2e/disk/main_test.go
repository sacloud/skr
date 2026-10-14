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
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type fakeCLI struct {
	item       *diskItem
	failAt     string
	badCreate  bool
	changeRead bool
	deleted    []types.ID
	calls      []string
	queryOut   []byte
}

func (f *fakeCLI) call(_ context.Context, step string, value any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if f.failAt == step {
		return nil, errors.New("injected CLI failure")
	}
	request, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected request shape")
	}
	if request["Zone"] != zone {
		return nil, errors.New("unexpected zone")
	}
	if step == "test-find-query" && f.queryOut != nil {
		return f.queryOut, nil
	}
	switch {
	case strings.Contains("-"+step+"-", "-find-"):
		if f.item == nil || request["Names"] == nil {
			return []byte("[]"), nil
		}
		names := request["Names"].([]string)
		if f.item.Name != names[0] {
			return []byte("[]"), nil
		}
		return json.Marshal([]diskItem{*f.item})
	case step == "test-create":
		f.item = &diskItem{
			ID: types.ID(1234), Name: request["Name"].(string),
			Description: request["Description"].(string), SizeMB: diskSizeMB,
		}
		data, err := json.Marshal(f.item)
		if f.badCreate {
			return []byte("{"), nil
		}
		return data, err
	case step == "test-read" || step == "test-read-updated" || step == "cleanup-read":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("missing Disk")
		}
		item := *f.item
		if f.changeRead && step == "cleanup-read" {
			item.Description = "changed identity"
		}
		return json.Marshal(item)
	case step == "test-update":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("missing Disk")
		}
		f.item.Name = request["Name"].(string)
		return json.Marshal(f.item)
	case step == "test-delete", step == "cleanup-delete":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("missing Disk")
		}
		f.deleted = append(f.deleted, f.item.ID)
		f.item = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func testScenario(t *testing.T, fake *fakeCLI) error {
	t.Helper()
	return (scenario{client: fake}).run(context.Background(), namePrefix+"test")
}

func TestScenarioDeletesDiskAndVerifiesAbsence(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil || len(fake.deleted) != 1 || fake.deleted[0] != types.ID(1234) {
		t.Fatalf("Disk state after E2E: item=%+v deleted=%v", fake.item, fake.deleted)
	}
	for _, step := range []string{"test-create", "test-find", "test-find-query", "test-read", "test-update", "test-read-updated", "test-delete", "test-find-original-after", "test-find-updated-after"} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestScenarioCleansUpAfterUpdateFailure(t *testing.T) {
	fake := &fakeCLI{failAt: "test-update"}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want injected update failure", err)
	}
	if fake.item != nil || len(fake.deleted) != 1 {
		t.Fatalf("Disk was not cleaned up: item=%+v deleted=%v", fake.item, fake.deleted)
	}
}

func TestScenarioRejectsInvalidProjectionAndCleansUp(t *testing.T) {
	for _, output := range []string{"null", "[]", `{"ID":1234}`, `[{"ID":4321,"Name":"wrong"}]`} {
		t.Run(output, func(t *testing.T) {
			fake := &fakeCLI{queryOut: []byte(output)}
			if err := testScenario(t, fake); err == nil {
				t.Fatal("invalid projection accepted")
			}
			if fake.item != nil || len(fake.deleted) != 1 {
				t.Fatal("created Disk not cleaned up")
			}
		})
	}
}

func TestScenarioRecoversWhenCreateOutputIsInvalid(t *testing.T) {
	fake := &fakeCLI{badCreate: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "decode Disk") {
		t.Fatalf("scenario error = %v, want response decode failure", err)
	}
	if fake.item != nil || len(fake.deleted) != 1 {
		t.Fatalf("created Disk was not recovered and deleted: item=%+v deleted=%v", fake.item, fake.deleted)
	}
}

func TestScenarioRefusesExistingDiskName(t *testing.T) {
	fake := &fakeCLI{item: &diskItem{
		ID: 5678, Name: namePrefix + "test", Description: "unrelated Disk", SizeMB: diskSizeMB,
	}}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want name collision refusal", err)
	}
	if len(fake.calls) != 2 || fake.item.Description != "unrelated Disk" || len(fake.deleted) != 0 {
		t.Fatalf("existing Disk was modified: calls=%v item=%+v", fake.calls, fake.item)
	}
}

func TestScenarioRefusesCleanupAfterIdentityChange(t *testing.T) {
	fake := &fakeCLI{failAt: "test-update", changeRead: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup refusal", err)
	}
	if fake.item == nil || len(fake.deleted) != 0 {
		t.Fatalf("Disk with changed identity was deleted: item=%+v deleted=%v", fake.item, fake.deleted)
	}
}

func TestDiskArgsUsesFlagsForSimpleRequestsAndJSONForComplexRequests(t *testing.T) {
	for _, test := range []struct {
		step    string
		request map[string]any
		want    []string
	}{
		{"test-create", map[string]any{"Zone": zone, "Name": "disk", "Description": "test", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB}, []string{"--zone is1b", "--name disk", "--description test", "--disk-plan-id 4", "--connection virtio", "--size-gb 20"}},
		{"test-create-complex", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "DistantFrom": []int{123}}, []string{"--request", `"DistantFrom":[123]`}},
		{"test-update", map[string]any{"Zone": zone, "ID": types.ID(123), "Name": "renamed"}, []string{"--zone is1b", "--id 123", "--name renamed"}},
		{"test-update-complex", map[string]any{"Zone": zone, "ID": types.ID(123), "EditParameter": map[string]any{"Hostname": "host"}}, []string{"--request", `"EditParameter":{"Hostname":"host"}`}},
		{"test-find", map[string]any{"Zone": zone, "Names": []string{"disk"}}, []string{"--request", `"Names":["disk"]`}},
		{"test-find-query", map[string]any{"Zone": zone, "Names": []string{"disk"}}, []string{"--request", `"Names":["disk"]`, "--query map({ID,Name})"}},
		{"test-read", map[string]any{"Zone": zone, "ID": types.ID(123)}, []string{"--zone", "is1b", "--id", "123"}},
		{"test-delete", map[string]any{"Zone": zone, "ID": types.ID(123)}, []string{"--zone", "is1b", "--id", "123", "--fail-if-not-found"}},
	} {
		args, err := diskArgs(test.step, test.request)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--output") {
			t.Errorf("%s args contain removed flag: %v", test.step, args)
		}
		for _, want := range test.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s args = %v, want %q", test.step, args, want)
			}
		}
	}
}

func TestDiskArgsPreservesAllRepresentableCreateAndUpdateFields(t *testing.T) {
	tests := []struct {
		step    string
		request map[string]any
		want    []string
	}{
		{
			step: "test-create",
			request: map[string]any{
				"Zone": zone, "Name": "disk", "DiskPlanID": types.ID(4),
				"Connection": "virtio", "SizeGB": 20, "Description": "description",
				"Tags": []string{"one", "two"}, "IconID": types.ID(0),
				"EncryptionAlgorithm": "aes256_xts", "KMSKeyID": types.ID(12),
				"DedicatedStorageContractID": types.ID(13), "SourceDiskID": types.ID(14),
				"SourceArchiveID": types.ID(15), "ServerID": types.ID(16), "NoWait": false,
			},
			want: []string{
				"--description description", "--tags one,two", "--icon-id 0",
				"--encryption-algorithm aes256_xts", "--kms-key-id 12",
				"--dedicated-storage-contract-id 13", "--source-disk-id 14",
				"--source-archive-id 15", "--server-id 16", "--no-wait=false",
			},
		},
		{
			step: "test-update",
			request: map[string]any{
				"Zone": zone, "ID": types.ID(123), "Name": "", "Description": "",
				"Tags": []string{"one", "two"}, "IconID": types.ID(0),
				"Connection": "ide", "NoWait": false,
			},
			want: []string{
				"--name ", "--description ", "--tags one,two", "--icon-id 0",
				"--connection ide", "--no-wait=false",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.step, func(t *testing.T) {
			args, err := diskArgs(test.step, test.request)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "--request") {
				t.Fatalf("representable request unexpectedly used JSON: %v", args)
			}
			for _, want := range test.want {
				if !strings.Contains(joined, want) {
					t.Errorf("args = %v, want %q", args, want)
				}
			}
		})
	}
}

func TestDiskArgsFallsBackToJSONForUnrepresentableFields(t *testing.T) {
	tests := []struct {
		name    string
		request map[string]any
	}{
		{"sdk helper input", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "OSType": 2}},
		{"complex input", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "DistantFrom": []types.ID{123}}},
		{"unknown field", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "Unmapped": "value"}},
		{"comma in tag", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "Tags": []string{"tag,with-comma"}}},
		{"empty tags", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB, "Tags": []string{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, err := diskArgs("test-create", test.request)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--request") {
				t.Fatalf("args = %v, want JSON fallback", args)
			}
			for field := range test.request {
				if field == "Zone" {
					continue
				}
				if !strings.Contains(joined, `"`+field+`":`) {
					t.Errorf("JSON args = %v, missing field %q", args, field)
				}
			}
		})
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
