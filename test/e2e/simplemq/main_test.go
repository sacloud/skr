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
	item                  *queueItem
	messages              []messageItem
	failStep              string
	badCreateOutput       bool
	changeCleanupIdentity bool
	calls                 []string
	callArgs              map[string][]string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.callArgs == nil {
		f.callArgs = make(map[string][]string)
	}
	f.callArgs[step] = append([]string(nil), args...)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "e2e-profile-current":
		return []byte("test-profile\n"), nil
	case "e2e-preflight-list":
		return f.queueList()
	case "e2e-create":
		f.item = &queueItem{
			ID:          json.RawMessage(`"queue-123"`),
			Name:        argument(args, "--name"),
			Description: argument(args, "--description"),
		}
		f.item.Status.QueueName = f.item.Name
		data, err := json.Marshal(f.item)
		if f.badCreateOutput {
			return []byte("{"), nil
		}
		return data, err
	case "e2e-list-created", "e2e-cleanup-list", "e2e-cleanup-list-after":
		return f.queueList()
	case "e2e-list-created-table":
		return []byte("ID Name\nqueue-123 skr-e2e-sqm-test\n"), nil
	case "e2e-config":
		f.item.Settings.VisibilityTimeoutSeconds = 30
		f.item.Settings.ExpireSeconds = 345600
		return json.Marshal(f.item)
	case "e2e-rotate-api-key":
		return []byte(`{"APIKey":"test-only-api-key"}`), nil
	case "e2e-send":
		f.messages = []messageItem{{ID: "message-123", Content: argument(args, "--content")}}
		return json.Marshal(f.messages[0])
	case "e2e-receive":
		for i := range f.messages {
			f.messages[i].VisibilityTimeoutAt = 1000
		}
		return json.Marshal(f.messages)
	case "e2e-extend-timeout":
		if len(f.messages) == 0 {
			return nil, errors.New("no message to extend")
		}
		f.messages[0].VisibilityTimeoutAt += 30000
		return json.Marshal(f.messages[0])
	case "e2e-delete-message":
		f.messages = nil
		return nil, nil
	case "e2e-count-messages":
		return json.Marshal(map[string]int{"Count": len(f.messages)})
	case "e2e-cleanup-read":
		item := *f.item
		if f.changeCleanupIdentity {
			item.Description = "not-the-e2e-description"
		}
		return json.Marshal(item)
	case "e2e-cleanup-clear-messages":
		f.messages = nil
		return nil, nil
	case "e2e-cleanup-delete":
		f.item = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func (f *fakeCLI) queueList() ([]byte, error) {
	if f.item == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]queueItem{*f.item})
}

func argument(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func testScenario(t *testing.T, fake *fakeCLI) error {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "simplemq.key")
	return (scenario{client: fake, keyFile: keyFile}).run(context.Background(), "skr-e2e-sqm-test", "skr-e2e-simplemq/test")
}

func TestScenarioCleansUpAfterSuccess(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil {
		t.Fatal("test queue was not deleted")
	}
	for _, want := range []string{
		"e2e-list-created-table",
		"e2e-config",
		"e2e-rotate-api-key",
		"e2e-send",
		"e2e-receive",
		"e2e-extend-timeout",
		"e2e-delete-message",
		"e2e-cleanup-clear-messages",
		"e2e-cleanup-delete",
		"e2e-cleanup-list-after",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	configArgs := strings.Join(fake.callArgs["e2e-config"], " ")
	for _, want := range []string{"--visibility-timeout-seconds 30", "--expire-seconds 345600"} {
		if !strings.Contains(configArgs, want) {
			t.Errorf("E2E config args %q do not contain %q", configArgs, want)
		}
	}
	if got := strings.Join(fake.callArgs["e2e-profile-current"], " "); got != "config current" {
		t.Errorf("profile check args = %q, want %q", got, "config current")
	}
}

func TestScenarioCleansUpAfterFailure(t *testing.T) {
	fake := &fakeCLI{failStep: "e2e-send"}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want injected message send failure", err)
	}
	if fake.item != nil || !contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("created queue was not cleaned up after failure")
	}
}

func TestScenarioRecoversCreatedQueueWhenCreateOutputIsInvalid(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "decode created queue") {
		t.Fatalf("scenario error = %v, want create response decode failure", err)
	}
	if fake.item != nil || !contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("queue created before invalid output was not recovered and deleted")
	}
}

func TestScenarioRefusesExistingQueueName(t *testing.T) {
	fake := &fakeCLI{item: &queueItem{
		ID:          json.RawMessage(`"existing-queue"`),
		Name:        "skr-e2e-sqm-test",
		Description: "existing resource",
	}}
	fake.item.Status.QueueName = fake.item.Name
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want existing-name refusal", err)
	}
	if len(fake.calls) != 2 || fake.item.Description != "existing resource" {
		t.Fatalf("scenario touched an existing resource: calls=%v item=%+v", fake.calls, fake.item)
	}
}

func TestScenarioRefusesCleanupWhenQueueIdentityChanged(t *testing.T) {
	fake := &fakeCLI{failStep: "e2e-send", changeCleanupIdentity: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup identity refusal", err)
	}
	if fake.item == nil || contains(fake.calls, "e2e-cleanup-delete") {
		t.Fatal("scenario deleted a queue whose identity no longer matched")
	}
}

func TestCLIRunnerRedactsAPIKeyFromEvidence(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-skr")
	script := "#!/bin/sh\nprintf '%s' '{\"APIKey\":\"test-only-api-key\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil { //nolint:gosec // The test helper must be executable.
		t.Fatal(err)
	}
	recorder, err := evidence.CreateAt(filepath.Join(dir, "tmp", "simplemq-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	output, err := runner.call(context.Background(), "e2e-rotate-api-key", "simplemq-api", "queue", "rotate-api-key")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "test-only-api-key") {
		t.Fatalf("CLI output = %q, want raw API key for the caller", output)
	}
	evidencePath := filepath.Join(recorder.Dir(), "001-e2e-rotate-api-key.json")
	info, err := os.Stat(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence permissions = %04o, want 0600", info.Mode().Perm())
	}
	evidence, err := os.ReadFile(evidencePath) //nolint:gosec // Path is constructed under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(evidence), "test-only-api-key") || !strings.Contains(string(evidence), "[REDACTED: APIKey]") {
		t.Fatalf("evidence did not redact API key: %s", evidence)
	}
	order, err := os.ReadFile(filepath.Join(recorder.Dir(), "ORDER.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(order), "001\te2e-rotate-api-key\tok\t001-e2e-rotate-api-key.json") {
		t.Fatalf("evidence order missing API key step: %s", order)
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
