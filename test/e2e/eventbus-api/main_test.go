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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	items          map[string]item
	calls          []string
	failAt         string
	failCreate     bool
	messageContent string
	configSettings struct {
		Destination string `json:"Destination"`
		Parameters  string `json:"Parameters"`
	}
	triggerSettings struct {
		Source     string   `json:"Source"`
		Types      []string `json:"Types"`
		Conditions []struct {
			Key    string   `json:"Key"`
			Op     string   `json:"Op"`
			Values []string `json:"Values"`
		} `json:"Conditions"`
		ProcessConfigurationID string `json:"ProcessConfigurationID"`
	}
	scheduleSettings struct {
		ProcessConfigurationID string `json:"ProcessConfigurationID"`
		StartsAt               int64  `json:"StartsAt"`
		RecurringStep          int    `json:"RecurringStep"`
		RecurringUnit          string `json:"RecurringUnit"`
	}
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failAt && !f.failCreate {
		return nil, errors.New("injected CLI failure")
	}
	switch {
	case strings.HasPrefix(step, "preflight-"), strings.HasPrefix(step, "cleanup-find-"), strings.HasPrefix(step, "cleanup-verify-"):
		kind := strings.TrimPrefix(step, "preflight-")
		kind = strings.TrimPrefix(kind, "cleanup-find-")
		kind = strings.TrimSuffix(kind, "s")
		if strings.HasPrefix(step, "cleanup-verify-") {
			kind = strings.TrimPrefix(step, "cleanup-verify-")
		}
		return f.list(kind)
	case step == "create-queue":
		created := newFakeItem(`"queue-id"`, argument(args, "--name"), argument(args, "--description"))
		created.Status.QueueName = created.Name
		f.items["queue"] = created
		return f.maybeFail(step, created)
	case step == "rotate-api-key":
		return []byte(`{"APIKey":"test-only-api-key"}`), nil
	case step == "create-process-configuration", step == "create-trigger":
		var request struct {
			CommonServiceItem struct {
				Name        string `json:"Name"`
				Description string `json:"Description"`
				Settings    struct {
					Source     string   `json:"Source"`
					Types      []string `json:"Types"`
					Conditions []struct {
						Key    string   `json:"Key"`
						Op     string   `json:"Op"`
						Values []string `json:"Values"`
					} `json:"Conditions"`
					ProcessConfigurationID string `json:"ProcessConfigurationID"`
					Parameters             string `json:"Parameters"`
				} `json:"Settings"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(argument(args, "--request")), &request); err != nil {
			return nil, err
		}
		kind, id := "process-configuration", `"config-id"`
		if step == "create-trigger" {
			if request.CommonServiceItem.Settings.ProcessConfigurationID != "config-id" {
				return nil, fmt.Errorf("trigger references unknown process-configuration %q", request.CommonServiceItem.Settings.ProcessConfigurationID)
			}
			kind, id = "trigger", `"trigger-id"`
			f.triggerSettings.Source = request.CommonServiceItem.Settings.Source
			f.triggerSettings.Types = request.CommonServiceItem.Settings.Types
			f.triggerSettings.Conditions = request.CommonServiceItem.Settings.Conditions
			f.triggerSettings.ProcessConfigurationID = request.CommonServiceItem.Settings.ProcessConfigurationID
		} else {
			var parameters struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(request.CommonServiceItem.Settings.Parameters), &parameters); err != nil {
				return nil, err
			}
			f.messageContent = parameters.Content
		}
		created := newFakeItem(id, request.CommonServiceItem.Name, request.CommonServiceItem.Description)
		f.items[kind] = created
		return f.maybeFail(step, created)
	case step == "update-process-configuration":
		var request struct {
			CommonServiceItem struct {
				Settings struct {
					Destination string `json:"Destination"`
					Parameters  string `json:"Parameters"`
				} `json:"Settings"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(argument(args, "--request")), &request); err != nil {
			return nil, err
		}
		var parameters struct {
			QueueName string `json:"queue_name"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal([]byte(request.CommonServiceItem.Settings.Parameters), &parameters); err != nil {
			return nil, err
		}
		f.configSettings.Destination = request.CommonServiceItem.Settings.Destination
		f.configSettings.Parameters = request.CommonServiceItem.Settings.Parameters
		f.messageContent = parameters.Content
		return json.Marshal(f.items["config"])
	case step == "read-updated-process-configuration":
		return f.readConfigurationItem()
	case step == "set-process-configuration-secret":
		data, err := os.ReadFile(argument(args, "--secret-file"))
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(data), "test-only-api-key") {
			return nil, errors.New("SimpleMQ API key was not provided to EventBus")
		}
		return nil, nil
	case step == "create-switch":
		created := newFakeItem(`1234`, argument(args, "--name"), argument(args, "--description"))
		f.items["switch"] = created
		return f.maybeFail(step, created)
	case strings.HasPrefix(step, "receive-event-message-"):
		data, err := json.Marshal([]message{{ID: "message-id", Content: f.messageContent}})
		return data, err
	case step == "verify-trigger":
		return json.Marshal(f.items["trigger"])
	case step == "update-trigger", step == "restore-trigger":
		var request struct {
			CommonServiceItem struct {
				Description string `json:"Description"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(argument(args, "--request")), &request); err != nil {
			return nil, err
		}
		updated := f.items["trigger"]
		updated.Description = request.CommonServiceItem.Description
		f.items["trigger"] = updated
		return json.Marshal(updated)
	case step == "read-updated-trigger", step == "read-restored-trigger":
		return f.readTriggerItem()
	case step == "create-schedule":
		var request struct {
			CommonServiceItem struct {
				Name        string `json:"Name"`
				Description string `json:"Description"`
				Settings    struct {
					ProcessConfigurationID string `json:"ProcessConfigurationID"`
					StartsAt               int64  `json:"StartsAt"`
					RecurringStep          int    `json:"RecurringStep"`
					RecurringUnit          string `json:"RecurringUnit"`
				} `json:"Settings"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(argument(args, "--request")), &request); err != nil {
			return nil, err
		}
		if request.CommonServiceItem.Settings.ProcessConfigurationID != "config-id" {
			return nil, fmt.Errorf("schedule references unknown process-configuration %q", request.CommonServiceItem.Settings.ProcessConfigurationID)
		}
		f.scheduleSettings = request.CommonServiceItem.Settings
		created := newFakeItem(`"schedule-id"`, request.CommonServiceItem.Name, request.CommonServiceItem.Description)
		f.items["schedule"] = created
		return f.maybeFail(step, created)
	case step == "list-schedule", step == "verify-schedule-deleted":
		return f.list("schedule")
	case step == "read-schedule", step == "read-updated-schedule", step == "read-restored-schedule":
		return f.readScheduleItem()
	case step == "update-schedule", step == "restore-schedule":
		var request struct {
			CommonServiceItem struct {
				Description string `json:"Description"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(argument(args, "--request")), &request); err != nil {
			return nil, err
		}
		updated := f.items["schedule"]
		updated.Description = request.CommonServiceItem.Description
		f.items["schedule"] = updated
		return json.Marshal(updated)
	case step == "delete-schedule":
		delete(f.items, "schedule")
		return nil, nil
	case strings.HasPrefix(step, "cleanup-read-"):
		kind := strings.TrimPrefix(step, "cleanup-read-")
		return json.Marshal(f.items[kind])
	case step == "cleanup-clear-queue":
		return nil, nil
	case strings.HasPrefix(step, "cleanup-delete-"):
		kind := strings.TrimPrefix(step, "cleanup-delete-")
		delete(f.items, kind)
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected E2E step %s", step)
	}
}

func (f *fakeCLI) maybeFail(step string, created item) ([]byte, error) {
	data, err := json.Marshal(created)
	if step == f.failAt && f.failCreate {
		return nil, errors.New("injected CLI failure after resource creation")
	}
	return data, err
}

func (f *fakeCLI) list(kind string) ([]byte, error) {
	value, ok := f.items[kind]
	if !ok {
		return []byte("[]"), nil
	}
	return json.Marshal([]item{value})
}

func (f *fakeCLI) readConfigurationItem() ([]byte, error) {
	base := f.items["process-configuration"]
	result := configurationItem{
		ID:          base.ID,
		Name:        base.Name,
		Description: base.Description,
	}
	result.Settings.Destination = f.configSettings.Destination
	result.Settings.Parameters = f.configSettings.Parameters
	return json.Marshal(result)
}

func (f *fakeCLI) readTriggerItem() ([]byte, error) {
	return f.triggerResponse(f.items["trigger"].Description)
}

func (f *fakeCLI) triggerResponse(description string) ([]byte, error) {
	base := f.items["trigger"]
	result := triggerItem{
		ID:          base.ID,
		Name:        base.Name,
		Description: description,
	}
	result.Settings.Source = f.triggerSettings.Source
	result.Settings.Types = f.triggerSettings.Types
	result.Settings.Conditions = f.triggerSettings.Conditions
	result.Settings.ProcessConfigurationID = json.RawMessage(`"` + f.triggerSettings.ProcessConfigurationID + `"`)
	return json.Marshal(result)
}

func (f *fakeCLI) readScheduleItem() ([]byte, error) {
	return f.scheduleResponse(f.items["schedule"].Description)
}

func (f *fakeCLI) scheduleResponse(description string) ([]byte, error) {
	base := f.items["schedule"]
	result := scheduleItem{
		ID:          base.ID,
		Name:        base.Name,
		Description: description,
	}
	result.Settings.ProcessConfigurationID = json.RawMessage(`"` + f.scheduleSettings.ProcessConfigurationID + `"`)
	// The API returns StartsAt as a decimal string in responses.
	result.Settings.StartsAt = strconv.FormatInt(f.scheduleSettings.StartsAt, 10)
	result.Settings.RecurringStep = f.scheduleSettings.RecurringStep
	result.Settings.RecurringUnit = f.scheduleSettings.RecurringUnit
	return json.Marshal(result)
}

func newFakeItem(id, name, description string) item {
	return item{ID: json.RawMessage(id), Name: name, Description: description}
}

func argument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func testScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "simplemq.key")
	return &scenario{
		client:     fake,
		keyFile:    keyFile,
		secretFile: filepath.Join(dir, "eventbus-secret.json"),
		queue:      resource{kind: "queue", name: "skr-e2e-eventbus-test", description: resourceDescription + "test"},
		config:     resource{kind: "process-configuration", name: "skr-e2e-eventbus-test", description: resourceDescription + "test"},
		trigger:    resource{kind: "trigger", name: "skr-e2e-eventbus-test", description: resourceDescription + "test"},
		schedule:   resource{kind: "schedule", name: "skr-e2e-eventbus-test", description: resourceDescription + "test"},
		sw:         resource{kind: "switch", name: "skr-e2e-eventbus-test", description: resourceDescription + "test"},
		expected:   "SKRE2ESWITCHCREATEDTEST",
	}
}

func TestScenarioCreatesSwitchAndCleansUpResources(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.items) != 0 {
		t.Fatalf("resources remain after E2E: %+v", fake.items)
	}
	for _, want := range []string{
		"create-queue",
		"rotate-api-key",
		"create-process-configuration",
		"update-process-configuration",
		"read-updated-process-configuration",
		"set-process-configuration-secret",
		"create-trigger",
		"create-switch",
		"verify-trigger",
		"update-trigger",
		"read-updated-trigger",
		"restore-trigger",
		"read-restored-trigger",
		"create-schedule",
		"list-schedule",
		"read-schedule",
		"update-schedule",
		"read-updated-schedule",
		"restore-schedule",
		"read-restored-schedule",
		"delete-schedule",
		"verify-schedule-deleted",
		"receive-event-message-000",
		"cleanup-delete-trigger",
		"cleanup-delete-process-configuration",
		"cleanup-delete-switch",
		"cleanup-delete-queue",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-trigger") > strings.Index(got, "cleanup-delete-queue") {
		t.Fatal("trigger must be deleted before its process configuration and queue")
	}
}

func TestScenarioRecoversResourceWhenCreateReturnsError(t *testing.T) {
	fake := &fakeCLI{items: make(map[string]item), failAt: "create-switch", failCreate: true}
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure after resource creation") {
		t.Fatalf("scenario error = %v, want ambiguous create failure", err)
	}
	if len(fake.items) != 0 || !contains(fake.calls, "cleanup-delete-switch") {
		t.Fatalf("created resources were not cleaned up after ambiguous create: %+v, calls=%v", fake.items, fake.calls)
	}
}

func TestScenarioRefusesExistingResource(t *testing.T) {
	fake := &fakeCLI{items: map[string]item{
		"queue": newFakeItem(`"existing-queue"`, "skr-e2e-eventbus-test", "unrelated"),
	}}
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want existing resource refusal", err)
	}
	if len(fake.calls) != 2 || len(fake.items) != 1 {
		t.Fatalf("scenario modified an existing resource: calls=%v resources=%+v", fake.calls, fake.items)
	}
}

func TestCLIRunnerOrdersEvidenceAndRedactsAPIKey(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-skr")
	script := "#!/bin/sh\nif [ \"$1\" = rotate-api-key ]; then\n  printf '%s' '{\"APIKey\":\"test-only-api-key\"}'\nelse\n  printf '%s' 'test-profile\\n'\nfi\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil { //nolint:gosec // The test helper must be executable.
		t.Fatal(err)
	}
	recorder, err := evidence.CreateAt(filepath.Join(dir, "tmp", "eventbus-api"), time.Date(2026, 10, 5, 17, 1, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	runner := &cliRunner{binary: binary, evidence: recorder}
	if _, err := runner.call(context.Background(), "profile-current", "config", "current"); err != nil {
		t.Fatal(err)
	}
	output, err := runner.call(context.Background(), "rotate-api-key", "rotate-api-key")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "test-only-api-key") {
		t.Fatalf("CLI output = %q, want raw API key for caller", output)
	}
	recordPath := filepath.Join(recorder.Dir(), "002-rotate-api-key.json")
	record, err := os.ReadFile(recordPath) //nolint:gosec // The file is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(record), "test-only-api-key") || !strings.Contains(string(record), "[REDACTED: APIKey]") {
		t.Fatalf("evidence did not redact API key: %s", record)
	}
	var recorded struct {
		Sequence int `json:"sequence"`
	}
	if err := json.Unmarshal(record, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Sequence != 2 {
		t.Errorf("record sequence = %d, want 2", recorded.Sequence)
	}
	recordInfo, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := recordInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("evidence file permissions = %o, want 600", got)
	}
	order, err := os.ReadFile(filepath.Join(recorder.Dir(), "ORDER.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"001\tprofile-current\tok\t001-profile-current.json",
		"002\trotate-api-key\tok\t002-rotate-api-key.json",
	} {
		if !strings.Contains(string(order), want) {
			t.Errorf("evidence order is missing %q:\n%s", want, order)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
