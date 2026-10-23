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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	value             *app
	failStep          string
	status            string
	badCreateOutput   bool
	changeCleanupData bool
	deleted           bool
	calls             []string
	pages             map[string][][]appSummary
	failPage          int
	otherID           bool
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "preflight-list", "list-created", "cleanup-list-after-delete":
		page, err := strconv.Atoi(args[3])
		if err != nil {
			return nil, err
		}
		if f.failPage == page {
			return nil, errors.New("injected page failure")
		}
		if pages, ok := f.pages[step]; ok {
			if page > len(pages) {
				return []byte("[]"), nil
			}
			return json.Marshal(pages[page-1])
		}
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
		if f.otherID {
			f.value.ID = "another-run-id"
		}
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

func TestScenarioRefusesCleanupWhenCreateResponseCannotBeDecoded(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true}
	err := runScenario(t, fake, namePrefix+"decode-error")
	if err == nil || !strings.Contains(err.Error(), "decode created AppRun application") {
		t.Fatalf("run error = %v, want create response decode failure", err)
	}
	if fake.value == nil || fake.deleted || !strings.Contains(err.Error(), "manually verify") {
		t.Fatal("unknown creation ID must require manual verification without deletion")
	}
}

func TestScenarioProtectsSameNameAndSettingsFromAnotherRun(t *testing.T) {
	fake := &fakeCLI{badCreateOutput: true, otherID: true}
	err := runScenario(t, fake, namePrefix+"other-run")
	if err == nil || !strings.Contains(err.Error(), "refusing cleanup") {
		t.Fatalf("run error = %v", err)
	}
	if fake.deleted || fake.value == nil || fake.value.ID != "another-run-id" {
		t.Fatal("deleted another run's application")
	}
}

func fullPage() []appSummary {
	items := make([]appSummary, 100)
	for i := range items {
		items[i] = appSummary{ID: fmt.Sprintf("existing-%d", i), Name: fmt.Sprintf("existing-%d", i)}
	}
	return items
}

func TestScenarioRefusesNameCollisionOnSecondPage(t *testing.T) {
	name := namePrefix + "page-two"
	fake := &fakeCLI{pages: map[string][][]appSummary{
		"preflight-list": {fullPage(), {{ID: "other-id", Name: name}}},
	}}
	err := runScenario(t, fake, name)
	if err == nil || !strings.Contains(err.Error(), "already exists") || containsStep(fake.calls, "create") {
		t.Fatalf("second-page collision was not protected: %v", err)
	}
}

func TestScenarioFindsCreatedApplicationOnSecondPage(t *testing.T) {
	name := namePrefix + "created-page-two"
	fake := &fakeCLI{pages: map[string][][]appSummary{
		"list-created": {fullPage(), {{ID: "app-id", Name: name}}},
	}}
	if err := runScenario(t, fake, name); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupDetectsRemainingApplicationOnSecondPage(t *testing.T) {
	name := namePrefix + "remaining"
	fake := &fakeCLI{value: newFakeApp(name), pages: map[string][][]appSummary{
		"cleanup-list-after-delete": {fullPage(), {{ID: "app-id", Name: name}}},
	}}
	err := (scenario{client: fake}).cleanup(name, "app-id")
	if err == nil || !strings.Contains(err.Error(), "remains after deletion") {
		t.Fatalf("cleanup error = %v", err)
	}
}

func TestListDoesNotTreatPageFailureAsAbsence(t *testing.T) {
	fake := &fakeCLI{pages: map[string][][]appSummary{"preflight-list": {fullPage()}}, failPage: 2}
	items, err := (scenario{client: fake}).list(context.Background(), "preflight-list")
	if err == nil || items != nil {
		t.Fatalf("list = %v, %v; want failure without partial results", items, err)
	}
}

func TestPublicURLRecordsEachAttempt(t *testing.T) {
	attempt := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporarily unavailable"))
			return
		}
		_, _ = w.Write([]byte("Hello from AppRun!"))
	}))
	defer server.Close()
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPublicURLWithClient(context.Background(), recorder, server.URL, server.Client(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for i, status := range []int{503, 200} {
		data, err := os.ReadFile(filepath.Join(recorder.Dir(), fmt.Sprintf("%03d-public-url.json", i+1)))
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Stdout string `json:"stdout"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		body := "Hello from AppRun!"
		if i == 0 {
			body = "temporarily unavailable"
		}
		if !strings.Contains(record.Stdout, fmt.Sprintf("HTTP status: %d", status)) ||
			!strings.Contains(record.Stdout, body) || (i == 0) != (record.Error != "") {
			t.Fatalf("unexpected attempt record: %+v", record)
		}
	}
}

func TestPublicURLFailsWhenEvidenceCannotBeWritten(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		_, _ = w.Write([]byte("Hello from AppRun!"))
	}))
	defer server.Close()
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the record path makes writing fail even when tests run as root.
	if err := os.Mkdir(filepath.Join(recorder.Dir(), "001-public-url.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	err = checkPublicURLWithClient(context.Background(), recorder, server.URL, server.Client(), time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "write evidence") || attempts != 1 {
		t.Fatalf("evidence failure: attempts = %d, error = %v", attempts, err)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("injected connection failure")
}

func TestPublicURLRecordsConnectionFailure(t *testing.T) {
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = checkPublicURLWithClient(ctx, recorder, "https://example.invalid", &http.Client{
		Transport: failingTransport{},
	}, time.Millisecond)
	if err == nil {
		t.Fatal("connection failure succeeded")
	}
	data, err := os.ReadFile(filepath.Join(recorder.Dir(), "001-public-url.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Stdout string `json:"stdout"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(record.Stdout, "HTTP status: 0") || !strings.Contains(record.Error, "injected connection failure") {
		t.Fatalf("connection failure record = %+v", record)
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
