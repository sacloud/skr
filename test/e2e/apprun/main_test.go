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
	"strings"
	"testing"
)

type fakeCLI struct {
	value             *app
	failStep          string
	status            string
	badCreateOutput   bool
	changeCleanupData bool
	deleted           bool
	calls             []string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "preflight-list", "list-created", "cleanup-list-unknown-id", "cleanup-list-after-delete":
		if f.value == nil {
			return []byte("[]"), nil
		}
		return json.Marshal([]appSummary{{ID: f.value.ID, Name: f.value.Name}})
	case "create":
		if !strings.HasPrefix(args[len(args)-1], "@") {
			return nil, errors.New("request file required")
		}
		data, err := os.ReadFile(strings.TrimPrefix(args[len(args)-1], "@")) // #nosec G304 -- Request file is created under t.TempDir by the scenario.
		if err != nil {
			return nil, err
		}
		var request struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		f.value = newFakeApp(request.Name)
		if f.badCreateOutput {
			return []byte("{"), nil
		}
		return json.Marshal(f.value)
	case "read-created", "cleanup-read-before-delete", "cleanup-read-unknown-id":
		if f.value == nil {
			return nil, errors.New("application not found")
		}
		value := *f.value
		if f.changeCleanupData && strings.HasPrefix(step, "cleanup") {
			value.Name = "unexpected-app"
		}
		return json.Marshal(value)
	case "read-status":
		status := f.status
		if status == "" {
			status = "Healthy"
		}
		return json.Marshal(map[string]string{"status": status})
	case "cleanup-delete":
		f.value = nil
		f.deleted = true
		return nil, nil
	default:
		return nil, errors.New("unexpected AppRun E2E step " + step)
	}
}

func (f *fakeCLI) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	if strings.Join(args, " ") != "config current" {
		return nil, errors.New("unexpected root command " + strings.Join(args, " "))
	}
	f.calls = append(f.calls, step)
	return []byte("test-profile\n"), nil
}

func newFakeApp(name string) *app {
	value := &app{ID: "app-id", Name: name, Port: port, PublicURL: "https://example.invalid"}
	value.Components = make([]struct {
		DeploySource struct {
			ContainerRegistry struct {
				Image string `json:"image"`
			} `json:"container_registry"`
		} `json:"deploy_source"`
	}, 1)
	value.Components[0].DeploySource.ContainerRegistry.Image = image
	return value
}

func runScenario(t *testing.T, fake *fakeCLI, name string) error {
	t.Helper()
	return (scenario{
		client:     fake,
		requestDir: t.TempDir(),
		checkURL:   func(context.Context, string) error { return nil },
	}).run(context.Background(), name)
}

func TestScenarioCreatesChecksAndCleansUpApp(t *testing.T) {
	fake := &fakeCLI{}
	if err := runScenario(t, fake, namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	if fake.value != nil || !fake.deleted {
		t.Fatalf("AppRun application remains after scenario: value=%+v deleted=%t", fake.value, fake.deleted)
	}
	for _, step := range []string{"preflight-list", "create", "list-created", "read-created", "read-status", "cleanup-delete", "cleanup-list-after-delete"} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
}

func TestScenarioCleansUpWhenCreateResponseCannotBeDecoded(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true}
	err := runScenario(t, fake, namePrefix+"decode-error")
	if err == nil || !strings.Contains(err.Error(), "decode created AppRun application") {
		t.Fatalf("run error = %v, want create response decode failure", err)
	}
	if fake.value != nil || !fake.deleted {
		t.Fatal("created AppRun application was not identified and cleaned up")
	}
}

func TestScenarioRefusesCleanupWhenIdentityChanges(t *testing.T) {
	fake := &fakeCLI{changeCleanupData: true}
	err := runScenario(t, fake, namePrefix+"identity")
	if err == nil || !strings.Contains(err.Error(), "refusing cleanup") {
		t.Fatalf("run error = %v, want identity mismatch", err)
	}
	if fake.value == nil || fake.deleted {
		t.Fatal("scenario deleted an application whose identity changed")
	}
}

func TestScenarioRefusesNameCollision(t *testing.T) {
	name := namePrefix + "collision"
	fake := &fakeCLI{value: newFakeApp(name)}
	err := runScenario(t, fake, name)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("run error = %v, want name collision", err)
	}
	if fake.deleted || containsStep(fake.calls, "create") {
		t.Fatal("scenario modified or deleted a pre-existing AppRun application")
	}
}

func TestScenarioCleansUpWhenPublicCheckFails(t *testing.T) {
	fake := &fakeCLI{}
	scenario := scenario{
		client:     fake,
		requestDir: t.TempDir(),
		checkURL:   func(context.Context, string) error { return errors.New("unexpected sample response") },
	}
	runErr := scenario.run(context.Background(), namePrefix+"probe-failure")
	if runErr == nil || !strings.Contains(runErr.Error(), "unexpected sample response") {
		t.Fatalf("run error = %v, want public check failure", runErr)
	}
	if fake.value != nil || !fake.deleted {
		t.Fatal("AppRun application was not cleaned up after public check failure")
	}
}

func TestScenarioFailsOnUnHealthyStatusAndCleansUp(t *testing.T) {
	fake := &fakeCLI{status: "UnHealthy"}
	publicURLChecked := false
	scenario := scenario{
		client:     fake,
		requestDir: t.TempDir(),
		checkURL: func(context.Context, string) error {
			publicURLChecked = true
			return nil
		},
	}
	err := scenario.run(context.Background(), namePrefix+"unhealthy")
	if err == nil || !strings.Contains(err.Error(), "UnHealthy") {
		t.Fatalf("run error = %v, want unhealthy status", err)
	}
	if fake.value != nil || !fake.deleted {
		t.Fatal("AppRun application was not cleaned up after unhealthy status")
	}
	if publicURLChecked {
		t.Fatal("scenario requested the public URL for an unhealthy application")
	}
}

func TestWriteCreateRequestUsesPublicSampleWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	path, err := (scenario{requestDir: dir}).writeCreateRequest(namePrefix + "test")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- The path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{image, `"port":8080`, `"max_cpu":"0.5"`, `"max_memory":"1Gi"`} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("create request is missing %q: %s", expected, data)
		}
	}
	for _, secret := range []string{"password", "secret", "token"} {
		if strings.Contains(strings.ToLower(string(data)), secret) {
			t.Errorf("create request unexpectedly contains secret field %q: %s", secret, data)
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
