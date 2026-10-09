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
	registry              *registryItem
	users                 []userItem
	failStep              string
	badCreateOutput       bool
	changeCleanupIdentity bool
	unexpectedUser        bool
	deleted               bool
	deletedUser           bool
	calls                 []string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "e2e-profile-current":
		return []byte("test-profile\n"), nil
	case "e2e-preflight-list", "e2e-list-created", "e2e-cleanup-list", "e2e-cleanup-list-after":
		return f.projectedRegistries(args)
	case "e2e-create":
		request, err := readCreateRequest(argument(args, "--request"))
		if err != nil {
			return nil, err
		}
		f.registry = &registryItem{
			ID:           json.RawMessage(`123`),
			Name:         request.Name,
			RegistryName: request.Name,
			Description:  request.Description,
		}
		data, err := json.Marshal(f.registry)
		if f.badCreateOutput {
			return []byte("{"), nil
		}
		return data, err
	case "e2e-update":
		if f.registry == nil {
			return nil, errors.New("registry not found")
		}
		request, err := readUpdateRequest(argument(args, "--request"))
		if err != nil {
			return nil, err
		}
		f.registry.Description = request.Description
		return json.Marshal(f.registry)
	case "e2e-read-updated", "e2e-cleanup-read", "e2e-cleanup-read-before-delete":
		if f.registry == nil {
			return nil, errors.New("registry not found")
		}
		item := *f.registry
		if f.changeCleanupIdentity && strings.HasPrefix(step, "e2e-cleanup") {
			item.Name = "unexpected-registry"
		}
		return json.Marshal(item)
	case "e2e-add-user":
		f.users = []userItem{{UserName: argument(args, "--user-name"), Permission: argument(args, "--permission")}}
		if f.unexpectedUser {
			f.users = append(f.users, userItem{UserName: "other-user", Permission: "readonly"})
		}
		return nil, nil
	case "e2e-update-user":
		if len(f.users) != 1 {
			return nil, errors.New("user not found")
		}
		f.users[0].Permission = argument(args, "--permission")
		return nil, nil
	case "e2e-list-user", "e2e-list-updated-user", "e2e-list-deleted-user", "e2e-cleanup-users", "e2e-cleanup-users-after":
		if f.users == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(f.users)
	case "e2e-delete-user", "e2e-cleanup-delete-user":
		f.users = nil
		f.deletedUser = true
		return nil, nil
	case "e2e-cleanup-delete":
		f.registry = nil
		f.deleted = true
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func (f *fakeCLI) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	if strings.Join(args, " ") != "config current" {
		return nil, errors.New("unexpected root CLI command " + strings.Join(args, " "))
	}
	return f.call(ctx, step, args...)
}

func (f *fakeCLI) projectedRegistries(args []string) ([]byte, error) {
	if f.registry == nil {
		return []byte("[]"), nil
	}
	request, err := readRequest(argument(args, "--request"))
	if err != nil {
		return nil, err
	}
	if len(request.Names) > 0 && request.Names[0] != f.registry.Name {
		return []byte("[]"), nil
	}
	return json.Marshal([]registryItem{*f.registry})
}

type requestFind struct {
	Names []string
}

type requestCreate struct {
	Name        string
	Description string
}

type requestUpdate struct {
	Description string
}

func readRequest(value string) (requestFind, error) {
	path := strings.TrimPrefix(value, "@")
	data, err := os.ReadFile(path) // #nosec G304 -- The path comes from a request file created under t.TempDir.
	if err != nil {
		return requestFind{}, err
	}
	var request requestFind
	if err := json.Unmarshal(data, &request); err != nil {
		return requestFind{}, err
	}
	return request, nil
}

func readCreateRequest(value string) (requestCreate, error) {
	path := strings.TrimPrefix(value, "@")
	data, err := os.ReadFile(path) // #nosec G304 -- The path comes from a request file created under t.TempDir.
	if err != nil {
		return requestCreate{}, err
	}
	var request requestCreate
	if err := json.Unmarshal(data, &request); err != nil {
		return requestCreate{}, err
	}
	return request, nil
}

func readUpdateRequest(value string) (requestUpdate, error) {
	path := strings.TrimPrefix(value, "@")
	data, err := os.ReadFile(path) // #nosec G304 -- The path comes from a request file created under t.TempDir.
	if err != nil {
		return requestUpdate{}, err
	}
	var request requestUpdate
	if err := json.Unmarshal(data, &request); err != nil {
		return requestUpdate{}, err
	}
	return request, nil
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
	tempDir := t.TempDir()
	passwordFile := filepath.Join(tempDir, "password")
	if err := os.WriteFile(passwordFile, []byte("test-only-password"), 0o600); err != nil {
		t.Fatal(err)
	}
	return (scenario{client: fake, requestDir: tempDir, passwordFile: passwordFile}).run(
		context.Background(), "skr-e2e-cr-test", descriptionPrefix+"test",
	)
}

func TestScenarioCreatesAndCleansUpRegistryAndUser(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.registry != nil || len(fake.users) != 0 || !fake.deleted || !fake.deletedUser {
		t.Fatalf("E2E resources remain: registry=%+v users=%+v deleted=%t userDeleted=%t",
			fake.registry, fake.users, fake.deleted, fake.deletedUser)
	}
	for _, step := range []string{
		"e2e-list-created",
		"e2e-update",
		"e2e-add-user",
		"e2e-list-updated-user",
		"e2e-delete-user",
		"e2e-cleanup-delete",
		"e2e-cleanup-list-after",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
}

func TestScenarioRecoversWhenCreateOutputCannotBeDecoded(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "decode created container registry") {
		t.Fatalf("run error = %v, want create decode failure", err)
	}
	if fake.registry != nil || !fake.deleted {
		t.Fatal("created registry was not recovered and removed")
	}
}

func TestScenarioCleansUpAfterUserUpdateFailure(t *testing.T) {
	fake := &fakeCLI{failStep: "e2e-update-user"}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("run error = %v, want injected update failure", err)
	}
	if fake.registry != nil || len(fake.users) != 0 || !fake.deleted {
		t.Fatal("registry and user were not cleaned up after user update failure")
	}
}

func TestScenarioRefusesCleanupWhenRegistryIdentityChanges(t *testing.T) {
	fake := &fakeCLI{changeCleanupIdentity: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "refusing cleanup") {
		t.Fatalf("run error = %v, want identity mismatch", err)
	}
	if fake.registry == nil || fake.deleted {
		t.Fatal("scenario deleted a registry whose identity changed")
	}
}

func TestScenarioRefusesCleanupWhenUnexpectedUserExists(t *testing.T) {
	fake := &fakeCLI{unexpectedUser: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "unexpected user") {
		t.Fatalf("run error = %v, want unexpected user safety failure", err)
	}
	if fake.registry == nil || fake.deleted {
		t.Fatal("scenario deleted a registry containing an unexpected user")
	}
}

func TestScenarioRefusesNameCollision(t *testing.T) {
	fake := &fakeCLI{registry: &registryItem{
		ID:           json.RawMessage(`123`),
		Name:         "skr-e2e-cr-test",
		RegistryName: "skr-e2e-cr-test",
		Description:  "pre-existing",
	}}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("run error = %v, want name collision", err)
	}
	if fake.deleted || fake.registry.Description != "pre-existing" {
		t.Fatal("scenario modified or deleted a pre-existing registry")
	}
	if containsStep(fake.calls, "e2e-create") {
		t.Fatal("scenario attempted to create a colliding registry")
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

func TestCLIRunnerRecordsRequestFileContents(t *testing.T) {
	tempDir := t.TempDir()
	binary := filepath.Join(tempDir, "skr")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'ok\\n'\n"), 0o700); err != nil { // #nosec G306 -- The test executable must be executable and is created under t.TempDir.
		t.Fatal(err)
	}
	requestPath := filepath.Join(tempDir, "create.json")
	request := "{\n  \"Name\": \"skr-e2e-cr-test\"\n}\n"
	if err := os.WriteFile(requestPath, []byte(request), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder, err := evidence.CreateAt(filepath.Join(tempDir, "evidence"), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	runner := cliRunner{binary: binary, evidence: recorder}
	if _, err := runner.call(context.Background(), "e2e-create",
		"registry", "create", "--request", "@"+requestPath); err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetResult("passed", time.Now()); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{requestPath, `"Name": "skr-e2e-cr-test"`} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
}
