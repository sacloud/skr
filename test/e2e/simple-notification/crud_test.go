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
	"slices"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type crudFake struct {
	items              map[string][]crudItem
	calls              []string
	failStep           string
	failAfterStep      string
	changeAfterFailure bool
	missingID          bool
	cleanupOther       bool
}

func newCRUDFake() *crudFake {
	return &crudFake{items: map[string][]crudItem{
		"destination": {{
			ID: "987654321098", Name: "existing-email", Description: "existing", Tags: []string{},
			Settings: map[string]any{"Type": "email", "Value": "private@example.invalid", "Disabled": false},
		}},
		"group": {},
		"routing": {{
			ID: "987654321097", Name: "existing-routing", Description: "existing", Tags: []string{},
			Settings: map[string]any{"PriorityRank": float64(1), "SourceID": "2", "MatchLabels": []any{}, "TargetGroupID": "987654321096"},
		}},
	}}
}

func (f *crudFake) call(_ context.Context, step string, input any, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == f.failStep {
		return nil, errors.New("injected CLI failure")
	}
	if len(args) < 3 || args[0] != "simple-notification-api" {
		return nil, fmt.Errorf("unexpected command: %v", args)
	}
	resource, operation := args[1], args[2]
	for _, arg := range args {
		if strings.Contains(arg, "private@example.invalid") {
			return nil, errors.New("email address leaked into command arguments")
		}
	}
	switch operation {
	case "list-source":
		return []byte(`{"Sources":[{"id":"2","name":"test-source"}]}`), nil
	case "list":
		return json.Marshal(f.items[resource])
	case "read", "read-status":
		if len(args) < 4 {
			return nil, errors.New("missing resource ID")
		}
		for _, item := range f.items[resource] {
			if item.ID != args[3] {
				continue
			}
			if operation == "read-status" {
				return []byte(`{"NotificationStatus":{"IsValid":false}}`), nil
			}
			if f.cleanupOther && strings.HasPrefix(step, "crud-cleanup-read") {
				item.ID = "111111111111"
			}
			return json.Marshal(crudResponse{CommonServiceItem: item})
		}
		return nil, errors.New("resource not found")
	case "create", "update", "reorder":
		request, ok := input.(evidence.RequestFileInput)
		if !ok || args[len(args)-1] != "@"+request.Path {
			return nil, errors.New("expected a captured request file")
		}
		info, err := os.Stat(request.Path)
		if err != nil || info.Mode().Perm() != 0o600 {
			return nil, errors.New("request file is not private")
		}
		if operation == "reorder" {
			var reorder struct {
				Orders []struct {
					RoutingID    string
					PriorityRank float64
				}
			}
			if err := json.Unmarshal([]byte(request.Content), &reorder); err != nil {
				return nil, err
			}
			if len(reorder.Orders) != 1 {
				return nil, errors.New("reorder must target only the created routing")
			}
			for _, order := range reorder.Orders {
				for i := range f.items["routing"] {
					item := &f.items["routing"][i]
					if item.ID == order.RoutingID && item.Description != "existing" {
						item.Settings["PriorityRank"] = order.PriorityRank
						if step == f.failAfterStep {
							if f.changeAfterFailure {
								item.Description = "not-this-run"
							}
							return nil, errors.New("injected failure after applying change")
						}
						return []byte(`{"is_ok":true}`), nil
					}
				}
			}
			return nil, errors.New("reorder targeted a non-E2E routing")
		}
		var response crudResponse
		if err := json.Unmarshal([]byte(request.Content), &response); err != nil {
			return nil, err
		}
		item := response.CommonServiceItem
		slices.Sort(item.Tags)
		if operation == "create" {
			item.ID = fmt.Sprintf("%012d", 10+len(f.items[resource]))
			if resource == "routing" {
				item.Settings["PriorityRank"] = float64(2)
			}
			f.items[resource] = append(f.items[resource], item)
			if f.missingID {
				item.ID = ""
			}
		} else {
			item.ID = args[3]
			found := false
			for i, previous := range f.items[resource] {
				if previous.ID == item.ID {
					if item.Settings == nil {
						item.Settings = previous.Settings
					}
					f.items[resource][i] = item
					if step == f.failAfterStep {
						if f.changeAfterFailure {
							f.items[resource][i].Description = "not-this-run"
						}
						return nil, errors.New("injected failure after applying change")
					}
					found = true
				}
			}
			if !found {
				return nil, errors.New("update targeted an unknown resource")
			}
		}
		return json.Marshal(crudResponse{CommonServiceItem: item})
	case "delete":
		id := args[3]
		if strings.HasPrefix(id, "987") {
			return nil, errors.New("refused deletion of an existing resource")
		}
		f.items[resource] = slices.DeleteFunc(f.items[resource], func(item crudItem) bool { return item.ID == id })
		if step == f.failAfterStep {
			return nil, errors.New("injected failure after applying change")
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected operation: %s", operation)
	}
}

func runCRUDTest(t *testing.T, fake *crudFake) error {
	t.Helper()
	return (crudScenario{scenario{
		client: fake, destinationID: "987654321098",
		requestPath: filepath.Join(t.TempDir(), "request.json"),
	}}).runCRUD(context.Background(), "test-unique")
}

func TestCRUDScenarioCoversTemporaryResourcesWithoutSending(t *testing.T) {
	fake := newCRUDFake()
	if err := runCRUDTest(t, fake); err != nil {
		t.Fatal(err)
	}
	if len(fake.items["destination"]) != 1 || len(fake.items["group"]) != 0 || len(fake.items["routing"]) != 1 {
		t.Fatalf("temporary resources remain: %#v", fake.items)
	}
	for _, step := range []string{
		"crud-destination-metadata", "crud-destination-enable", "crud-destination-disable", "crud-destination-settings", "crud-destination-status",
		"crud-group-metadata", "crud-group-enable", "crud-group-disable", "crud-group-settings", "crud-routing-metadata", "crud-routing-match-label", "crud-routing-settings",
		"crud-routing-reorder-99", "crud-routing-reorder-100", "crud-routing-after",
		"crud-cleanup-delete-routing", "crud-cleanup-delete-group", "crud-cleanup-delete-destination",
	} {
		if !slices.Contains(fake.calls, step) {
			t.Errorf("missing step: %s", step)
		}
	}
	if rank := fake.items["routing"][0].Settings["PriorityRank"]; rank != float64(1) {
		t.Fatalf("existing routing rank changed: %v", rank)
	}
}

func TestCRUDScenarioCleansUpAfterFailure(t *testing.T) {
	for _, step := range []string{"crud-group-settings", "crud-routing-settings", "crud-routing-reorder-99"} {
		t.Run(step, func(t *testing.T) {
			fake := newCRUDFake()
			fake.failStep = step
			err := runCRUDTest(t, fake)
			if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
				t.Fatalf("error = %v", err)
			}
			if len(fake.items["destination"]) != 1 || len(fake.items["group"]) != 0 || len(fake.items["routing"]) != 1 {
				t.Fatalf("temporary resources remain: %#v", fake.items)
			}
		})
	}
}

func TestCRUDScenarioRefusesUnverifiedCleanup(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*crudFake)
	}{
		{"missing create ID", func(fake *crudFake) { fake.missingID = true }},
		{"changed ID", func(fake *crudFake) { fake.cleanupOther = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newCRUDFake()
			test.prepare(fake)
			err := runCRUDTest(t, fake)
			if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
				t.Fatalf("error = %v", err)
			}
			for _, call := range fake.calls {
				if strings.HasPrefix(call, "crud-cleanup-delete") {
					t.Fatalf("deleted an unverified resource: %s", call)
				}
			}
		})
	}
}

func TestCRUDScenarioReportsCleanupFailure(t *testing.T) {
	fake := newCRUDFake()
	fake.failStep = "crud-cleanup-delete-routing"
	if err := runCRUDTest(t, fake); err == nil {
		t.Fatal("cleanup failure was ignored")
	}
}

func TestCRUDScenarioContinuesCleanupAfterAppliedDeleteFails(t *testing.T) {
	fake := newCRUDFake()
	fake.failAfterStep = "crud-cleanup-delete-routing"
	err := runCRUDTest(t, fake)
	if err == nil || !strings.Contains(err.Error(), "after applying change") {
		t.Fatalf("error = %v, want retained delete error", err)
	}
	if len(fake.items["destination"]) != 1 || len(fake.items["group"]) != 0 || len(fake.items["routing"]) != 1 {
		t.Fatalf("cleanup did not remove dependent temporary resources: %#v", fake.items)
	}
	if !slices.Contains(fake.calls, "crud-cleanup-delete-group") || !slices.Contains(fake.calls, "crud-cleanup-delete-destination") {
		t.Fatalf("cleanup stopped before removing dependencies: %v", fake.calls)
	}
}

func TestCRUDScenarioRejectsOccupiedReorderRanks(t *testing.T) {
	fake := newCRUDFake()
	fake.items["routing"][0].Settings["PriorityRank"] = float64(99)
	if err := runCRUDTest(t, fake); err == nil || !strings.Contains(err.Error(), "must be unused") {
		t.Fatalf("error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("scenario did not stop at preflight: %v", fake.calls)
	}
}

func TestCRUDScenarioRejectsNameCollision(t *testing.T) {
	fake := newCRUDFake()
	fake.items["group"] = []crudItem{{ID: "987654321096", Name: "skr-e2e-crud-test-unique"}}
	err := runCRUDTest(t, fake)
	if err == nil || !strings.Contains(err.Error(), "name already exists") {
		t.Fatalf("error = %v", err)
	}

	if len(fake.items["destination"]) != 1 || len(fake.items["group"]) != 1 {
		t.Fatal("name collision cleanup affected an existing resource or left the temporary destination")
	}
}

func TestCRUDScenarioCleansUpAfterAppliedChangeFails(t *testing.T) {
	for _, step := range []string{
		"crud-destination-metadata", "crud-destination-enable", "crud-destination-disable",
		"crud-group-metadata", "crud-group-enable", "crud-group-disable",
		"crud-routing-metadata", "crud-routing-match-label", "crud-routing-reorder-99", "crud-routing-reorder-100",
	} {
		t.Run(step, func(t *testing.T) {
			fake := newCRUDFake()
			fake.failAfterStep = step
			err := runCRUDTest(t, fake)
			if err == nil || !strings.Contains(err.Error(), "after applying change") {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), "cleanup refused") {
				t.Fatalf("cleanup refused the applied E2E change: %v", err)
			}
			if len(fake.items["destination"]) != 1 || len(fake.items["group"]) != 0 || len(fake.items["routing"]) != 1 {
				t.Fatal("temporary resources remain after an applied change failed")
			}
		})
	}
}

func TestCRUDScenarioRefusesUnownedStateAfterAppliedChangeFails(t *testing.T) {
	fake := newCRUDFake()
	fake.failAfterStep = "crud-routing-reorder-99"
	fake.changeAfterFailure = true
	err := runCRUDTest(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("error = %v", err)
	}
	for _, step := range fake.calls {
		if strings.HasPrefix(step, "crud-cleanup-delete") {
			t.Fatalf("deleted an unverified resource: %s", step)
		}
	}
}
