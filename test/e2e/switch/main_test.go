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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	item      *switchItem
	prior     []switchItem
	failAt    string
	badCreate bool
	deleted   []types.ID
	zone      string
	queryOut  []byte
}

func (f *fakeCLI) call(_ context.Context, step string, value any) ([]byte, error) {
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
	wantZone := zone
	if f.zone != "" {
		wantZone = f.zone
	}
	if zone, ok := request["Zone"].(string); !ok || zone != wantZone {
		return nil, errors.New("unexpected zone")
	}
	if step == "test-find-query" && f.queryOut != nil {
		return f.queryOut, nil
	}
	op, err := operation(step)
	if err != nil {
		return nil, err
	}
	switch op {
	case "find":
		items := make([]switchItem, 0, len(f.prior)+1)
		if f.item != nil {
			items = append(items, *f.item)
		}
		items = append(items, f.prior...)
		if names, ok := request["Names"].([]string); ok {
			var filtered []switchItem
			for _, item := range items {
				if item.Name == names[0] {
					filtered = append(filtered, item)
				}
			}
			if filtered == nil {
				filtered = []switchItem{}
			}
			return json.Marshal(filtered)
		}
		from, count := request["From"].(int), request["Count"].(int)
		if from >= len(items) {
			return json.Marshal([]switchItem{})
		}
		if f.failAt == "ignore-page" {
			from = 0
		}
		end := min(from+count, len(items))
		return json.Marshal(items[from:end])
	case "create":
		f.item = &switchItem{
			ID:          types.ID(1234),
			Name:        request["Name"].(string),
			Description: request["Description"].(string),
			Tags:        request["Tags"].([]string),
		}
		if f.badCreate {
			return []byte("{"), nil
		}
		return json.Marshal(f.item)
	case "read":
		if f.item != nil && f.item.ID == request["ID"] {
			return json.Marshal(f.item)
		}
		for _, prior := range f.prior {
			if prior.ID == request["ID"] {
				return json.Marshal(prior)
			}
		}
		return nil, errors.New("missing switch")
	case "update":
		f.item.Name = request["Name"].(string)
		f.item.Tags = request["Tags"].([]string)
		return json.Marshal(f.item)
	case "delete":
		if request["FailIfNotFound"] != true {
			return nil, errors.New("unsafe delete")
		}
		if f.item != nil && request["ID"] == f.item.ID {
			f.deleted = append(f.deleted, f.item.ID)
			f.item = nil
			return nil, nil
		}
		for i, prior := range f.prior {
			if prior.ID == request["ID"] {
				f.deleted = append(f.deleted, prior.ID)
				f.prior = append(f.prior[:i], f.prior[i+1:]...)
				return nil, nil
			}
		}
		return nil, errors.New("unsafe delete")
	default:
		return nil, errors.New("unexpected operation")
	}
}

func TestScenarioCleansUpAfterSuccess(t *testing.T) {
	f := &fakeCLI{}
	if err := (scenario{call: f.call}).run(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || f.item != nil {
		t.Fatal("test switch was not deleted")
	}
}

func TestLiveScenarioPreservesPriorResources(t *testing.T) {
	f := &fakeCLI{zone: "is1b", prior: []switchItem{{ID: 9876, Name: "skr-e2e-other", Description: "existing resource"}}}
	if err := (scenario{call: f.call, zone: "is1b", skipPriorCleanup: true}).run(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if len(f.prior) != 1 || len(f.deleted) != 1 || f.deleted[0] != 1234 {
		t.Fatalf("live scenario modified prior resources: prior=%+v deleted=%v", f.prior, f.deleted)
	}
}

func TestScenarioRejectsInvalidProjectionAndCleansUp(t *testing.T) {
	for _, output := range []string{"null", "[]", `{"ID":1234,"Name":"skr-e2e-switch"}`, `[{"ID":1234,"Name":"wrong"}]`, `[{"ID":4321,"Name":"skr-e2e-switch"}]`} {
		t.Run(output, func(t *testing.T) {
			f := &fakeCLI{queryOut: []byte(output)}
			if err := (scenario{call: f.call}).run(context.Background(), name); err == nil {
				t.Fatal("invalid projection accepted")
			}
			if f.item != nil || len(f.deleted) != 1 {
				t.Fatal("created Switch not cleaned up")
			}
		})
	}
}

func TestScenarioCleansUpAfterUpdateFailure(t *testing.T) {
	f := &fakeCLI{failAt: "test-update"}
	err := (scenario{call: f.call}).run(context.Background(), name)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("run error = %v, want update failure", err)
	}
	if len(f.deleted) != 1 || f.item != nil {
		t.Fatal("test switch was not deleted on failure")
	}
}

func TestScenarioRecoversWhenCreateOutputCannotBeDecoded(t *testing.T) {
	f := &fakeCLI{badCreate: true}
	err := (scenario{call: f.call}).run(context.Background(), name)
	if err == nil || !strings.Contains(err.Error(), "decode Switch") {
		t.Fatalf("run error = %v, want decode failure", err)
	}
	if len(f.deleted) != 1 || f.item != nil {
		t.Fatal("test switch was not recovered and deleted")
	}
}

func TestScenarioDeletesPriorTestSwitch(t *testing.T) {
	f := &fakeCLI{prior: []switchItem{{ID: types.ID(9876), Name: "skr-e2e-old", Description: description}}}
	if err := (scenario{call: f.call}).run(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 2 || f.deleted[0] != types.ID(9876) || f.deleted[1] != types.ID(1234) {
		t.Fatalf("deleted IDs = %v, want old test Switch then new test Switch", f.deleted)
	}
}

func TestScenarioRefusesToDeleteUnmarkedSwitch(t *testing.T) {
	f := &fakeCLI{prior: []switchItem{{ID: types.ID(9876), Name: "skr-e2e-other", Description: "different purpose"}}}
	err := (scenario{call: f.call}).run(context.Background(), name)
	if err == nil || !strings.Contains(err.Error(), "description differs") {
		t.Fatalf("run error = %v, want marker mismatch", err)
	}
	if len(f.deleted) != 0 || len(f.prior) != 1 || f.item != nil {
		t.Fatal("unmarked Switch was deleted or test Switch was created")
	}
}

func TestScenarioChecksAllPagesBeforeDeletion(t *testing.T) {
	f := &fakeCLI{failAt: "ignore-page"}
	for i := range pageSize {
		f.prior = append(f.prior, switchItem{ID: types.ID(2000 + i), Name: "other", Description: "not a test"})
	}
	f.prior = append(f.prior, switchItem{ID: types.ID(9999), Name: "skr-e2e-other", Description: "not a test"})
	err := (scenario{call: f.call}).run(context.Background(), name)
	if err == nil || !strings.Contains(err.Error(), "duplicate Switch ID") {
		t.Fatalf("run error = %v, want unsafe pagination detection", err)
	}
	if len(f.deleted) != 0 || f.item != nil {
		t.Fatal("modified resources after pagination error")
	}
}

func TestScenarioHandlesPaginatedPriorTestSwitch(t *testing.T) {
	f := &fakeCLI{}
	for i := range pageSize {
		f.prior = append(f.prior, switchItem{ID: types.ID(2000 + i), Name: "other", Description: "not a test"})
	}
	f.prior = append(f.prior, switchItem{ID: types.ID(9999), Name: "skr-e2e-old", Description: description})
	if err := (scenario{call: f.call}).run(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 2 || f.deleted[0] != types.ID(9999) {
		t.Fatalf("deleted IDs = %v, want paginated prior Switch followed by new Switch", f.deleted)
	}
}

func TestScenarioReportsCleanupFailure(t *testing.T) {
	f := &fakeCLI{failAt: "cleanup-delete"}
	err := (scenario{call: f.call}).run(context.Background(), name)
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("run error = %v, want cleanup failure", err)
	}
	if f.item == nil {
		t.Fatal("mock did not preserve Switch after failed cleanup")
	}
}

func TestCLIRunnerRecordsPrivateEvidence(t *testing.T) {
	binary, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo executable not found")
	}

	dir := t.TempDir()
	recorder, err := evidence.CreateAt(filepath.Join(dir, "tmp", "switch-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	if _, err := runner.call(context.Background(), "test-find", map[string]any{"Zone": zone}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(recorder.Dir(), "001-test-find.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence permissions = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path is constructed inside t.TempDir for this test.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test-find") || !strings.Contains(string(data), `"tk1v"`) {
		t.Fatalf("evidence missing step or request: %s", data)
	}
	order, err := os.ReadFile(filepath.Join(recorder.Dir(), "ORDER.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(order), "001\ttest-find\tok\t001-test-find.json") {
		t.Fatalf("evidence order missing find step: %s", order)
	}
}

func TestSwitchArgsUsesFlagsAndJSONForNames(t *testing.T) {
	for _, tc := range []struct {
		step    string
		request map[string]any
		want    string
	}{
		{"test-create", map[string]any{"Zone": zone, "Name": name, "Description": description, "Tags": []string{"tutorial", "managed"}}, "--zone tk1v --name skr-e2e-switch --description Temporary switch for skr API tutorial --tags tutorial,managed"},
		{"test-find", map[string]any{"Zone": zone, "Names": []string{name}}, "--request {"},
		{"test-find-query", map[string]any{"Zone": zone, "Names": []string{name}}, "--request {"},
		{"preflight-find-page-000", map[string]any{"Zone": zone, "Count": pageSize, "From": 0}, "--zone tk1v --count 100 --from 0"},
		{"cleanup-delete", map[string]any{"Zone": zone, "ID": types.ID(123), "FailIfNotFound": true}, "--zone tk1v --id 123 --fail-if-not-found=true"},
	} {
		args, err := switchArgs(tc.step, tc.request)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(args, " "), tc.want) {
			t.Fatalf("%s: args = %v, want %q", tc.step, args, tc.want)
		}
		got := strings.Join(args, " ")
		if strings.Contains(got, "--output") {
			t.Errorf("%s: args contain removed flag: %v", tc.step, args)
		}
		if strings.HasSuffix(tc.step, "-query") && !strings.Contains(got, "--query map({ID,Name})") {
			t.Errorf("%s: args lack projection: %v", tc.step, args)
		}
	}
}
