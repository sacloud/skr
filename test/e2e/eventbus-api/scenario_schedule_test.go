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
	"strings"
	"testing"
)

func TestScheduleScenarioCoversLifecycle(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.createConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduleScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.items["schedule"]; ok {
		t.Fatal("schedule remains after the scenario")
	}
	if !s.schedule.deleted {
		t.Error("schedule was not marked deleted")
	}
	for _, step := range []string{
		"create-schedule",
		"list-schedule",
		"read-schedule",
		"update-schedule",
		"read-updated-schedule",
		"restore-schedule",
		"read-restored-schedule",
		"delete-schedule",
		"verify-schedule-deleted",
	} {
		found := false
		for _, call := range fake.calls {
			if call == step {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
}

func TestScheduleScenarioReportsFailures(t *testing.T) {
	t.Run("create api failure", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item), failAt: "create-schedule"}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := s.scheduleScenario(context.Background())
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want injected CLI failure", err)
		}
	})
	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item)}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		s.client = &overrideCLI{base: fake, replace: map[string]func() []byte{
			"read-updated-schedule": func() []byte {
				data, err := fake.scheduleResponse(s.schedule.description)
				return mustResponse(t, data, err)
			},
		}}
		err := s.scheduleScenario(context.Background())
		if err == nil || !strings.Contains(err.Error(), "schedule identity mismatch") {
			t.Fatalf("error = %v, want schedule identity mismatch", err)
		}
	})
	t.Run("schedule remains after delete", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item)}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		remaining := []byte(`[{"ID":"schedule-id","Name":"skr-e2e-eventbus-test","Description":"skr-e2e-eventbus/test"}]`)
		s.client = &overrideCLI{base: fake, replace: map[string]func() []byte{
			"verify-schedule-deleted": func() []byte { return remaining },
		}}
		err := s.scheduleScenario(context.Background())
		if err == nil || !strings.Contains(err.Error(), "remains after delete") {
			t.Fatalf("error = %v, want schedule remains after delete", err)
		}
	})
}

func TestScheduleCreateRequestNeverFiresDuringRun(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.createConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduleScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.scheduleSettings.RecurringStep != 1 || fake.scheduleSettings.RecurringUnit != "day" {
		t.Errorf("recurrence = %d %s, want 1 day", fake.scheduleSettings.RecurringStep, fake.scheduleSettings.RecurringUnit)
	}
}
