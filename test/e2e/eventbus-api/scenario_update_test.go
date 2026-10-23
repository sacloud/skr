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
	"strings"
	"testing"
)

// overrideCLI replaces specific step responses while delegating everything else
// to the base fake CLI.
type overrideCLI struct {
	base    *fakeCLI
	replace map[string]func() []byte
}

func (o *overrideCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	data, err := o.base.call(ctx, step, args...)
	if err != nil {
		return data, err
	}
	if replaced, ok := o.replace[step]; ok {
		return replaced(), nil
	}
	return data, nil
}

func TestUpdateConfigurationAndVerifyUpdatesMessageContent(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.createConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.updateConfigurationAndVerify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "SKRE2ESWITCHCREATEDTESTUPDATED"; s.expected != want {
		t.Errorf("expected message content = %q, want %q", s.expected, want)
	}
	if fake.messageContent != s.expected {
		t.Errorf("fake message content = %q, want %q", fake.messageContent, s.expected)
	}
	if fake.configSettings.Destination != "simplemq" {
		t.Errorf("Settings Destination = %q, want simplemq", fake.configSettings.Destination)
	}
	var parameters struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(fake.configSettings.Parameters), &parameters); err != nil {
		t.Fatalf("updated Parameters are not valid JSON: %v", err)
	}
	if parameters.QueueName != s.queue.name {
		t.Errorf("Parameters queue_name = %q, want %q", parameters.QueueName, s.queue.name)
	}
	if parameters.Content != s.expected {
		t.Errorf("Parameters content = %q, want %q", parameters.Content, s.expected)
	}
}

func TestUpdateConfigurationAndVerifyReportsFailures(t *testing.T) {
	staleConfiguration := func(s *scenario) configurationItem {
		var result configurationItem
		result.ID = json.RawMessage(`"config-id"`)
		result.Name = s.config.name
		result.Description = s.config.description
		result.Settings.Destination = "simplemq"
		result.Settings.Parameters = `{"queue_name":"skr-e2e-eventbus-test","content":"SKRE2ESWITCHCREATEDTEST"}`
		return result
	}
	t.Run("stale content", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item)}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		stale, err := json.Marshal(staleConfiguration(s))
		if err != nil {
			t.Fatal(err)
		}
		s.client = &overrideCLI{base: fake, replace: map[string]func() []byte{
			"read-updated-process-configuration": func() []byte { return stale },
		}}
		err = s.updateConfigurationAndVerify(context.Background())
		if err == nil || !strings.Contains(err.Error(), "updated process-configuration Parameters mismatch") {
			t.Fatalf("error = %v, want updated process-configuration Parameters mismatch", err)
		}
		if s.expected == "SKRE2ESWITCHCREATEDTESTUPDATED" {
			t.Errorf("expected message content was updated despite the mismatch: %q", s.expected)
		}
	})
	t.Run("api failure", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item), failAt: "update-process-configuration"}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := s.updateConfigurationAndVerify(context.Background())
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want injected CLI failure", err)
		}
	})
}

func TestUpdateTriggerAndVerifyRestoresDescription(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.createConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createTrigger(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.updateTriggerAndVerify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.items["trigger"].Description; got != s.trigger.description {
		t.Errorf("trigger Description = %q, want restored %q", got, s.trigger.description)
	}
	for _, step := range []string{"update-trigger", "read-updated-trigger", "restore-trigger", "read-restored-trigger"} {
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

func TestUpdateTriggerAndVerifyReportsFailures(t *testing.T) {
	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item)}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := s.createTrigger(context.Background()); err != nil {
			t.Fatal(err)
		}
		original := fake.items["trigger"].Description
		s.client = &overrideCLI{base: fake, replace: map[string]func() []byte{
			"read-updated-trigger": func() []byte {
				data, err := fake.triggerResponse(original)
				return mustResponse(t, data, err)
			},
		}}
		err := s.updateTriggerAndVerify(context.Background())
		if err == nil || !strings.Contains(err.Error(), "trigger identity mismatch") {
			t.Fatalf("error = %v, want trigger identity mismatch", err)
		}
	})
	t.Run("restore api failure", func(t *testing.T) {
		fake := &fakeCLI{items: make(map[string]item), failAt: "restore-trigger"}
		s := testScenario(t, fake)
		if err := s.createConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := s.createTrigger(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := s.updateTriggerAndVerify(context.Background())
		if err == nil || !strings.Contains(err.Error(), "restore trigger Description") {
			t.Fatalf("error = %v, want restore trigger Description failure", err)
		}
	})
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustResponse(t *testing.T, data []byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return data
}
