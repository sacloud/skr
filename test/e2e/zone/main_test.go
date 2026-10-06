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
	queryOut []byte
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
	case "zone-list-table":
		return []byte("+------+\n| Name |\n+------+\n"), nil
	case "zone-names-jq":
		if f.queryOut != nil {
			return f.queryOut, nil
		}
		return []byte(`["zone-a","zone-b"]`), nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioListsZonesAndAppliesJQ(t *testing.T) {
	fake := &fakeCLI{}
	if err := (scenario{client: fake}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fake.args["zone-list-table"], " "), "iaas-api zone find --output table"; got != want {
		t.Errorf("table args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["zone-names-jq"], " "), "iaas-api zone find --query map(.Name)"; got != want {
		t.Errorf("query args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.calls, " "), "profile-current zone-list-table zone-names-jq"; got != want {
		t.Errorf("scenario calls = %q, want %q", got, want)
	}
}

func TestScenarioRejectsMissingProfileEmptyListAndInvalidQueryOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		failStep string
		output   []byte
		wantErr  string
	}{
		{name: "missing profile", failStep: "profile-current", wantErr: "selected SDK profile"},
		{name: "empty zone list", output: []byte(`[]`), wantErr: "zone list is empty"},
		{name: "invalid jq JSON", output: []byte("not-json"), wantErr: "decode zone names"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{failStep: test.failStep, queryOut: test.output}
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
	recorder, err := evidence.CreateAt(filepath.Join(t.TempDir(), "tmp", "zone-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	output, err := runner.call(context.Background(), "zone-list-jq", "iaas-api", "zone", "find", "--query", "map(.Name)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "map(.Name)") {
		t.Fatalf("command output = %q, want echoed arguments", output)
	}
	info, err := os.Stat(filepath.Join(recorder.Dir(), "001-zone-list-jq.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence file permissions = %04o, want 0600", info.Mode().Perm())
	}
}
