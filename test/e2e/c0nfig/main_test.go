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
	responses map[string][]byte
	failStep  string
	calls     []string
	args      map[string][]string
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
	response, ok := f.responses[step]
	if !ok {
		return nil, errors.New("unexpected E2E step " + step)
	}
	return response, nil
}

func fakeResponses(spid string) map[string][]byte {
	return map[string][]byte{
		"config-create":          []byte("Profile \"skr-e2e-config\" created\n"),
		"config-current":         []byte("skr-e2e-config\n"),
		"config-list":            []byte("skr-e2e-config\n"),
		"config-show":            []byte(`{"ServicePrincipalID":"` + spid + `","ServicePrincipalKeyID":"kid-e2e","PrivateKeyPEMPath":"(masked)"}`),
		"config-edit":            []byte("Profile \"skr-e2e-config\" updated\n"),
		"config-show-after-edit": []byte(`{"ServicePrincipalID":"spid-edited","ServicePrincipalKeyID":"kid-e2e","PrivateKeyPEMPath":"(masked)"}`),
	}
}

func TestScenarioCreatesAndEditsProfile(t *testing.T) {
	fake := &fakeCLI{responses: fakeResponses("spid-e2e")}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantCalls := []string{
		"config-create",
		"config-current",
		"config-list",
		"config-show",
		"config-edit",
		"config-show-after-edit",
	}
	if len(fake.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", fake.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if fake.calls[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, fake.calls[i], want)
		}
	}

	wantCreate := []string{
		"config", "create", "skr-e2e-config",
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", "/tmp/key.pem",
		"--use",
	}
	if got := fake.args["config-create"]; strings.Join(got, "\x00") != strings.Join(wantCreate, "\x00") {
		t.Errorf("create args = %v, want %v", got, wantCreate)
	}

	wantEdit := []string{
		"config", "edit", "skr-e2e-config",
		"--service-principal-id", "spid-edited",
	}
	if got := fake.args["config-edit"]; strings.Join(got, "\x00") != strings.Join(wantEdit, "\x00") {
		t.Errorf("edit args = %v, want %v", got, wantEdit)
	}
}

func TestScenarioFailsWhenCreateFails(t *testing.T) {
	fake := &fakeCLI{responses: fakeResponses("spid-e2e"), failStep: "config-create"}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "create profile") {
		t.Fatalf("scenario error = %v, want create failure", err)
	}
}

func TestScenarioFailsWhenCurrentProfileUnexpected(t *testing.T) {
	responses := fakeResponses("spid-e2e")
	responses["config-current"] = []byte("other-profile\n")
	fake := &fakeCLI{responses: responses}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "current profile") {
		t.Fatalf("scenario error = %v, want current profile mismatch", err)
	}
}

func TestScenarioFailsWhenProfileMissingFromList(t *testing.T) {
	responses := fakeResponses("spid-e2e")
	responses["config-list"] = []byte("other-profile\n")
	fake := &fakeCLI{responses: responses}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("scenario error = %v, want missing profile in list", err)
	}
}

func TestScenarioFailsWhenShowValueUnexpected(t *testing.T) {
	responses := fakeResponses("wrong-spid")
	fake := &fakeCLI{responses: responses}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ServicePrincipalID") {
		t.Fatalf("scenario error = %v, want unexpected ServicePrincipalID", err)
	}
}

func TestScenarioFailsWhenEditFails(t *testing.T) {
	fake := &fakeCLI{responses: fakeResponses("spid-e2e"), failStep: "config-edit"}
	err := (scenario{
		client:      fake,
		keyPath:     "/tmp/key.pem",
		profileName: "skr-e2e-config",
	}).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "edit profile") {
		t.Fatalf("scenario error = %v, want edit failure", err)
	}
}

func TestCLIRunnerSetsProfileDirEnv(t *testing.T) {
	binary, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env executable not found")
	}

	dir := t.TempDir()
	recorder, err := evidence.CreateAt(filepath.Join(dir, "tmp", "c0nfig"), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runner := cliRunner{
		binary:   binary,
		env:      []string{"SAKURA_PROFILE_DIR=" + profileDir},
		evidence: recorder,
	}
	output, err := runner.call(context.Background(), "env-check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "SAKURA_PROFILE_DIR="+profileDir) {
		t.Fatalf("CLI output = %q, want SAKURA_PROFILE_DIR=%q", output, profileDir)
	}

	path := filepath.Join(recorder.Dir(), "001-env-check.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence permissions = %04o, want 0600", info.Mode().Perm())
	}
}
