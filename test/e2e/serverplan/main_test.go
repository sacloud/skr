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
	find     []byte
	read     []byte
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
	case "serverplan-find":
		if f.find != nil {
			return f.find, nil
		}
		return []byte(`[{"ID":17,"Name":"test-plan","CPU":4,"MemoryMB":8192,"GPU":0}]`), nil
	case "serverplan-read":
		if f.read != nil {
			return f.read, nil
		}
		return []byte(`{"ID":17,"Name":"test-plan","CPU":4,"MemoryMB":8192,"GPU":0}`), nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioFindsAndReadsServerPlanInIs1b(t *testing.T) {
	fake := &fakeCLI{}
	if err := (scenario{client: fake}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fake.args["serverplan-find"], " "),
		"iaas-api product server find --zone is1b --count 1 --from 0 --query "+serverPlanQuery; got != want {
		t.Errorf("Find args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["serverplan-read"], " "),
		"iaas-api product server read --zone is1b --id 17"; got != want {
		t.Errorf("Read args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.calls, " "), "profile-current serverplan-find serverplan-read"; got != want {
		t.Errorf("scenario calls = %q, want %q", got, want)
	}
}

func TestScenarioRejectsInvalidIdentityAndMismatchedRead(t *testing.T) {
	for _, test := range []struct {
		name    string
		find    []byte
		read    []byte
		wantErr string
	}{
		{name: "empty results", find: []byte(`[]`), wantErr: "want one"},
		{name: "invalid JSON", find: []byte("not-json"), wantErr: "decode ServerPlan find"},
		{name: "missing identity", find: []byte(`[{"ID":0,"Name":""}]`), wantErr: "missing its ID or name"},
		{name: "read mismatch", read: []byte(`{"ID":18,"Name":"other-plan","CPU":4,"MemoryMB":8192}`), wantErr: "did not match find result"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{find: test.find, read: test.read}
			err := (scenario{client: fake}).run(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestScenarioPropagatesProfileAndAPIErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		failStep string
		wantErr  string
	}{
		{name: "profile", failStep: "profile-current", wantErr: "selected SDK profile"},
		{name: "find", failStep: "serverplan-find", wantErr: "injected CLI failure"},
		{name: "read", failStep: "serverplan-read", wantErr: "injected CLI failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{failStep: test.failStep}
			err := (scenario{client: fake}).run(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCLIRunnerRecordsPrivateEvidence(t *testing.T) {
	binary, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo executable not found")
	}
	recorder, err := evidence.CreateAt(filepath.Join(t.TempDir(), "tmp", "serverplan-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	output, err := runner.call(context.Background(), "serverplan-find", "iaas-api", "product", "server", "find")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "product server") {
		t.Fatalf("command output = %q, want echoed API route command", output)
	}
	info, err := os.Stat(filepath.Join(recorder.Dir(), "001-serverplan-find.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence file permissions = %04o, want 0600", info.Mode().Perm())
	}
}
