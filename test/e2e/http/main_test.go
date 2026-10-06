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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	failStep string
	response []byte
	calls    []string
	args     map[string][]string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
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
	case "http-get-zones":
		return f.response, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioRequestsZoneListThroughHTTP(t *testing.T) {
	fake := &fakeCLI{response: []byte(`{"response":{"zones":[]}}`)}
	if err := (scenario{client: fake}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fake.args["profile-current"], " "), "config current"; got != want {
		t.Errorf("profile check args = %q, want %q", got, want)
	}
	wantArgs := []string{"http", zoneListURL, "--method", "GET"}
	gotArgs := fake.args["http-get-zones"]
	if strings.Join(gotArgs, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("HTTP request args = %v, want %v", gotArgs, wantArgs)
	}
	if len(fake.calls) != 2 || fake.calls[0] != "profile-current" || fake.calls[1] != "http-get-zones" {
		t.Errorf("scenario calls = %v, want profile check followed by zone-list request", fake.calls)
	}
}

func TestScenarioRequiresSelectedProfile(t *testing.T) {
	fake := &fakeCLI{failStep: "profile-current", response: []byte(`{}`)}
	err := (scenario{client: fake}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "selected SDK profile is required") {
		t.Fatalf("scenario error = %v, want missing-profile error", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "profile-current" {
		t.Fatalf("scenario continued without a selected profile: %v", fake.calls)
	}
}

func TestScenarioRejectsEmptyOrInvalidJSONResponse(t *testing.T) {
	for _, response := range [][]byte{nil, []byte("not-json")} {
		fake := &fakeCLI{response: response}
		err := (scenario{client: fake}).run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
			t.Errorf("scenario error for %q = %v, want invalid JSON error", response, err)
		}
	}
}

func TestCLIRunnerRecordsPrivateEvidence(t *testing.T) {
	binary, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo executable not found")
	}

	dir := t.TempDir()
	recorder, err := evidence.CreateAt(filepath.Join(dir, "tmp", "http-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	args := []string{"http", zoneListURL, "--method", "GET"}
	output, err := runner.call(context.Background(), "http-get-zones", args...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), zoneListURL) {
		t.Fatalf("CLI output = %q, want URL arguments", output)
	}

	path := filepath.Join(recorder.Dir(), "001-http-get-zones.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence permissions = %04o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path is constructed inside t.TempDir for this test.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SAKURA_ACCESS_TOKEN_SECRET") || !strings.Contains(string(data), zoneListURL) {
		t.Fatalf("evidence unexpectedly contains a credential or omits the endpoint: %s", data)
	}
}
