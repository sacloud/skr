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

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	item                  *groupItem
	failStep              string
	badCreateOutput       bool
	changeCleanupIdentity bool
	omitCreateID          bool
	invalidDestination    bool
	historyStatuses       []int
	historyCalls          int
	message               string
	calls                 []string
	args                  map[string][]string
	request               string
}

func (f *fakeCLI) call(_ context.Context, step string, input any, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = make(map[string][]string)
	}
	f.args[step] = append([]string(nil), args...)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "profile-current":
		return []byte("test-profile\n"), nil
	case "e2e-read-destination-status":
		return json.Marshal(struct {
			NotificationStatus struct {
				IsValid bool `json:"IsValid"`
			} `json:"NotificationStatus"`
		}{NotificationStatus: struct {
			IsValid bool `json:"IsValid"`
		}{IsValid: !f.invalidDestination}})
	case "e2e-preflight-list", "e2e-list-created-groups", "e2e-cleanup-list-after":
		return f.list()
	case "e2e-create-group":
		requestInput, ok := input.(evidence.RequestFileInput)
		if !ok {
			return nil, errors.New("create request was not captured")
		}
		f.request = requestInput.Content
		var request struct {
			CommonServiceItem struct {
				Name        string `json:"Name"`
				Description string `json:"Description"`
				Settings    struct {
					Destinations []string `json:"Destinations"`
					Disabled     bool     `json:"Disabled"`
				} `json:"Settings"`
			} `json:"CommonServiceItem"`
		}
		if err := json.Unmarshal([]byte(f.request), &request); err != nil {
			return nil, err
		}
		id := "123456789012"
		if f.omitCreateID {
			id = ""
		}
		f.item = &groupItem{ID: id, Name: request.CommonServiceItem.Name, Description: request.CommonServiceItem.Description}
		f.item.Settings.Destinations = request.CommonServiceItem.Settings.Destinations
		f.item.Settings.Disabled = request.CommonServiceItem.Settings.Disabled
		data, err := json.Marshal(groupResponse{CommonServiceItem: *f.item})
		if f.badCreateOutput {
			return []byte("{"), nil
		}
		return data, err
	case "e2e-read-created-group", "e2e-cleanup-read":
		if f.item == nil {
			return nil, errors.New("group does not exist")
		}
		item := *f.item
		if step == "e2e-cleanup-read" && f.changeCleanupIdentity {
			item.Description = "not-this-run"
		}
		return json.Marshal(groupResponse{CommonServiceItem: item})
	case "e2e-send-message":
		if f.item == nil {
			return nil, errors.New("group does not exist")
		}
		if len(args) == 0 {
			return nil, errors.New("send command has no arguments")
		}
		f.message = args[len(args)-1]
		return []byte(`{"is_ok":true}`), nil
	case "e2e-cleanup-delete":
		f.item = nil
		return nil, nil
	default:
		if strings.HasPrefix(step, "e2e-notification-history-") {
			if f.item == nil {
				return nil, errors.New("group does not exist")
			}
			status := 2
			if f.historyCalls < len(f.historyStatuses) {
				status = f.historyStatuses[f.historyCalls]
			}
			f.historyCalls++
			history := notificationHistory{RequestID: "request-123", Message: f.message}
			history.Statuses = []notificationStatus{{
				GroupID: f.item.ID, DestinationID: f.item.Settings.Destinations[0], Status: status,
			}}
			return json.Marshal([]notificationHistory{history})
		}
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func (f *fakeCLI) list() ([]byte, error) {
	if f.item == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]groupItem{*f.item})
}

func testScenario(t *testing.T, fake *fakeCLI) error {
	t.Helper()
	return (scenario{
		client: fake, destinationID: "987654321098",
		requestPath: filepath.Join(t.TempDir(), "create-request.json"),
		message:     "test notification", historyPoll: time.Millisecond, historyLimit: 100 * time.Millisecond,
	}).run(context.Background(), "skr-e2e-notify-test", "skr-e2e-simple-notification/test")
}

func TestScenarioSendsMessageAndCleansUpGroup(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil {
		t.Fatal("temporary group was not deleted")
	}
	if !strings.Contains(fake.request, `"Destinations":["987654321098"]`) {
		t.Fatalf("create request does not target the supplied destination: %s", fake.request)
	}
	for _, step := range []string{"e2e-read-destination-status", "e2e-list-created-groups", "e2e-read-created-group", "e2e-send-message", "e2e-notification-history-01", "e2e-cleanup-read", "e2e-cleanup-delete", "e2e-cleanup-list-after"} {
		if !contains(fake.calls, step) {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
	if got := strings.Join(fake.args["e2e-send-message"], " "); !strings.Contains(got, "group send-message 123456789012 --message test notification") {
		t.Errorf("send command args = %q", got)
	}
}

func TestScenarioWaitsForNotificationProcessing(t *testing.T) {
	fake := &fakeCLI{historyStatuses: []int{1, 2}}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.historyCalls != 2 {
		t.Fatalf("history calls = %d, want 2", fake.historyCalls)
	}
}

func TestScenarioCleansUpWhenNotificationProcessingFails(t *testing.T) {
	fake := &fakeCLI{historyStatuses: []int{9}}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "processing ended with status 9") {
		t.Fatalf("scenario error = %v, want notification processing failure", err)
	}
	if fake.item != nil || !contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("temporary group was not deleted after notification processing failed")
	}
}

func TestScenarioRejectsInvalidDestinationBeforeCreatingGroup(t *testing.T) {
	fake := &fakeCLI{invalidDestination: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "not valid for sending") {
		t.Fatalf("scenario error = %v, want invalid destination error", err)
	}
	if fake.item != nil || contains(fake.calls, "e2e-create-group") {
		t.Fatal("scenario created a group for an invalid destination")
	}
}

func TestScenarioCleansUpAfterSendFailure(t *testing.T) {
	fake := &fakeCLI{failStep: "e2e-send-message"}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want injected send failure", err)
	}
	if fake.item != nil || !contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("temporary group was not deleted after send failure")
	}
}

func TestScenarioRefusesExistingGroupName(t *testing.T) {
	existing := groupItem{ID: "111111111111", Name: "skr-e2e-notify-test", Description: "existing"}
	fake := &fakeCLI{item: &existing}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want name collision refusal", err)
	}
	if len(fake.calls) != 3 || fake.item.Description != "existing" {
		t.Fatalf("scenario touched an existing group: calls=%v item=%+v", fake.calls, fake.item)
	}
}

func TestScenarioRefusesCleanupWhenCreateOutputIsInvalid(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cannot safely identify E2E group") {
		t.Fatalf("scenario error = %v, want cleanup refusal without a returned ID", err)
	}
	if fake.item == nil || contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("scenario deleted a group without the ID returned by create")
	}
}

func TestScenarioRefusesCleanupWhenIdentityChanges(t *testing.T) {
	fake := &fakeCLI{failStep: "e2e-send-message", changeCleanupIdentity: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup refusal", err)
	}
	if fake.item == nil || contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("scenario deleted a group whose identity no longer matched")
	}
}

func TestScenarioRefusesCleanupWithoutCreatedID(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true, omitCreateID: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cannot safely identify E2E group") {
		t.Fatalf("scenario error = %v, want missing ID refusal", err)
	}
	if fake.item == nil || contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("scenario deleted a group without a verified ID")
	}
}

func TestScenarioRequestFileIsPrivate(t *testing.T) {
	fake := &fakeCLI{}
	scenario := scenario{
		client: fake, destinationID: "987654321098",
		requestPath: filepath.Join(t.TempDir(), "create-request.json"),
		message:     "test notification",
	}
	if err := scenario.run(context.Background(), "skr-e2e-notify-test", "skr-e2e-simple-notification/test"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(scenario.requestPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("request file permissions = %04o, want 0600", info.Mode().Perm())
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
