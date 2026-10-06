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
	case step == "test-find-table":
		return []byte("ID Name\n1234 " + f.item.Name + "\n"), nil
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
	for _, step := range []string{"test-create", "test-find", "test-find-table", "test-read", "test-update", "test-read-updated", "test-delete", "test-find-original-after", "test-find-updated-after"} {
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

func TestDiskArgsUsesJSONForComplexRequestsAndFlagsForIDs(t *testing.T) {
	for _, test := range []struct {
		step    string
		request map[string]any
		want    []string
	}{
		{"test-create", map[string]any{"Zone": zone, "Name": "disk", "DiskPlanID": diskPlanID, "SizeGB": diskSizeGB}, []string{"--request", `"DiskPlanID":4`, "--output", "json"}},
		{"test-update", map[string]any{"Zone": zone, "ID": types.ID(123), "Name": "renamed"}, []string{"--request", `"Name":"renamed"`, "--output", "json"}},
		{"test-find", map[string]any{"Zone": zone, "Names": []string{"disk"}}, []string{"--request", `"Names":["disk"]`, "--output", "json"}},
		{"test-read", map[string]any{"Zone": zone, "ID": types.ID(123)}, []string{"--zone", "is1b", "--id", "123", "--output", "json"}},
		{"test-delete", map[string]any{"Zone": zone, "ID": types.ID(123)}, []string{"--zone", "is1b", "--id", "123", "--fail-if-not-found", "--output", "json"}},
	} {
		args, err := diskArgs(test.step, test.request)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		for _, want := range test.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s args = %v, want %q", test.step, args, want)
			}
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
