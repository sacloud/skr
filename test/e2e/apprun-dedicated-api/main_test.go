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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	calls                 []string
	cluster               bool
	group                 bool
	application           bool
	version               bool
	active                bool
	failAt                string
	changeClusterIdentity bool
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failAt {
		return nil, errors.New("injected failure")
	}
	switch step {
	case "preflight-clusters", "cleanup-verify-cluster-deleted":
		if f.cluster {
			return []byte(`{"items":[{"clusterID":"cluster-id","name":"skr-e2e-abcd1234"}]}`), nil
		}
		return []byte(`{"items":[]}`), nil
	case "create-cluster":
		f.cluster = true
		return []byte(`{"clusterID":"cluster-id"}`), nil
	case "verify-created-cluster", "cleanup-read-cluster":
		if step == "cleanup-read-cluster" && f.changeClusterIdentity {
			return []byte(`{"clusterID":"cluster-id","name":"unrelated","servicePrincipalID":"principal-id"}`), nil
		}
		return []byte(`{"clusterID":"cluster-id","name":"skr-e2e-abcd1234","servicePrincipalID":"principal-id"}`), nil
	case "create-auto-scaling-group":
		f.group = true
		return []byte(`{"autoScalingGroupID":"group-id"}`), nil
	case "verify-created-auto-scaling-group", "cleanup-read-auto-scaling-group":
		return []byte(`{"autoScalingGroupID":"group-id","name":"skr-e2e-abcd1234","zone":"is1b","workerServiceClassPath":"worker-class"}`), nil
	case "create-application":
		f.application = true
		return []byte(`{"applicationID":"application-id"}`), nil
	case "verify-created-application", "verify-active-version", "cleanup-read-application", "cleanup-read-application-state":
		active := "null"
		if step == "verify-active-version" || f.active {
			active = "1"
		}
		return []byte(`{"applicationID":"application-id","name":"skr-e2e-abcd1234","clusterID":"cluster-id","activeVersion":` + active + `}`), nil
	case "create-version":
		f.version = true
		return []byte(`{"version":1}`), nil
	case "verify-created-version":
		return []byte(`{"version":1}`), nil
	case "activate-version":
		f.active = true
		return nil, nil
	case "cleanup-deactivate-application":
		f.active = false
		return nil, nil
	case "cleanup-delete-version":
		return []byte(`[]`), nil
	case "cleanup-verify-version-deleted":
		return []byte(`{"items":[]}`), nil
	case "verify-container-placement":
		return []byte(`[{}]`), nil
	case "cleanup-delete-application":
		f.application = false
		return nil, nil
	case "cleanup-delete-auto-scaling-group":
		f.group = false
		return nil, nil
	case "cleanup-delete-cluster":
		f.cluster = false
		return nil, nil
	case "cleanup-verify-auto-scaling-group-deleted":
		if f.group {
			return []byte(`{"items":[{"autoScalingGroupID":"group-id","name":"skr-e2e-abcd1234"}]}`), nil
		}
		return []byte(`{"items":[]}`), nil
	case "cleanup-verify-application-deleted":
		if f.application {
			return []byte(`{"items":[{"applicationID":"application-id","name":"skr-e2e-abcd1234","clusterID":"cluster-id"}]}`), nil
		}
		return []byte(`{"items":[]}`), nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func writeJSONFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func makeScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	dir := t.TempDir()
	return &scenario{
		client:         fake,
		clusterRequest: writeJSONFile(t, dir, "cluster.json", `{"name":"placeholder","servicePrincipalID":"principal-id","ports":[{"port":443,"protocol":"https"}]}`),
		groupRequest:   writeJSONFile(t, dir, "group.json", `{"name":"placeholder","zone":"tk1a","workerServiceClassPath":"worker-class","minNodes":1,"maxNodes":4,"nameServers":["192.0.2.1"],"interfaces":[{"interfaceIndex":0,"upstream":"shared","ipPool":[],"connectsToLB":false}]}`),
		versionRequest: writeJSONFile(t, dir, "version.json", `{"cpu":1000,"memory":2048,"scalingMode":"automatic","fixedScale":3,"minScale":2,"maxScale":5,"image":"example.invalid/test:latest","registryPassword":"private-test-secret"}`),
		cluster:        resource{name: "skr-e2e-abcd1234"},
		group:          resource{name: "skr-e2e-abcd1234"},
		application:    resource{name: "skr-e2e-abcd1234"},
	}
}

func TestScenarioCreatesActivatesAndDeletesOnlyItsResources(t *testing.T) {
	fake := &fakeCLI{}
	s := makeScenario(t, fake)
	if err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.cluster || fake.group || fake.application {
		t.Fatalf("test resources remain: cluster=%t group=%t application=%t", fake.cluster, fake.group, fake.application)
	}
	for _, want := range []string{
		"create-cluster",
		"create-auto-scaling-group",
		"create-application",
		"create-version",
		"activate-version",
		"verify-active-version",
		"verify-container-placement",
		"cleanup-deactivate-application",
		"cleanup-delete-version",
		"cleanup-delete-application",
		"cleanup-delete-auto-scaling-group",
		"cleanup-delete-cluster",
		"cleanup-verify-cluster-deleted",
	} {
		if !containsStep(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	if indexOf(fake.calls, "cleanup-delete-application") > indexOf(fake.calls, "cleanup-delete-auto-scaling-group") ||
		indexOf(fake.calls, "cleanup-delete-auto-scaling-group") > indexOf(fake.calls, "cleanup-delete-cluster") {
		t.Fatalf("resources were not deleted in dependency order: %v", fake.calls)
	}
}

func TestScenarioRefusesExistingCluster(t *testing.T) {
	fake := &fakeCLI{cluster: true}
	s := makeScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want name collision refusal", err)
	}
	if containsStep(fake.calls, "create-cluster") || len(fake.calls) != 2 {
		t.Fatalf("existing cluster was modified: calls=%v", fake.calls)
	}
}

func TestPrepareRequestsPinsZoneAndKeepsSensitiveInputInFiles(t *testing.T) {
	fake := &fakeCLI{}
	s := makeScenario(t, fake)
	dir := t.TempDir()
	if err := s.prepareRequests(dir); err != nil {
		t.Fatal(err)
	}
	cluster, err := os.ReadFile(s.clusterRequest)
	if err != nil {
		t.Fatal(err)
	}
	group, err := os.ReadFile(s.groupRequest)
	if err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile(s.versionRequest)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{cluster, group} {
		if !strings.Contains(string(data), s.cluster.name) {
			t.Errorf("generated resource name missing from request: %s", data)
		}
	}
	if !strings.Contains(string(group), `"zone":"is1b"`) {
		t.Fatalf("ASG request zone = %s, want is1b", group)
	}
	for _, field := range []string{`"minNodes":1`, `"maxNodes":1`} {
		if !strings.Contains(string(group), field) {
			t.Errorf("ASG request missing bounded node count %s: %s", field, group)
		}
	}
	if !strings.Contains(string(version), "private-test-secret") {
		t.Fatalf("private version request was not preserved: %s", version)
	}
	for _, field := range []string{`"scalingMode":"manual"`, `"fixedScale":1`} {
		if !strings.Contains(string(version), field) {
			t.Errorf("version request missing bounded scale setting %s: %s", field, version)
		}
	}
	for _, field := range []string{`"minScale"`, `"maxScale"`, `"scaleInThreshold"`, `"scaleOutThreshold"`} {
		if strings.Contains(string(version), field) {
			t.Errorf("automatic scaling field %s was not removed: %s", field, version)
		}
	}
	info, err := os.Stat(s.versionRequest)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("private request permissions = %o, want 600", got)
	}
}

func TestScenarioCleansUpAfterVersionActivationFailure(t *testing.T) {
	fake := &fakeCLI{failAt: "activate-version"}
	s := makeScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("scenario error = %v, want injected activation failure", err)
	}
	if fake.cluster || fake.group || fake.application {
		t.Fatalf("resources were not cleaned up after failure: cluster=%t group=%t app=%t", fake.cluster, fake.group, fake.application)
	}
}

func TestScenarioRefusesCleanupAfterClusterIdentityChanges(t *testing.T) {
	fake := &fakeCLI{failAt: "create-auto-scaling-group", changeClusterIdentity: true}
	s := makeScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup refusal", err)
	}
	if !fake.cluster || containsStep(fake.calls, "cleanup-delete-cluster") {
		t.Fatalf("cluster with changed identity was deleted: calls=%v", fake.calls)
	}
}

func TestCLIRunnerRedactsSensitiveResponsesFromEvidence(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-skr")
	script := "#!/bin/sh\nprintf '%s' '{\"registryPassword\":\"private-test-secret\"}'\nprintf '%s' 'private-test-secret' >&2\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil { //nolint:gosec // The test helper must be executable.
		t.Fatal(err)
	}
	recorder, err := evidence.CreateAt(filepath.Join(dir, "evidence"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := &cliRunner{binary: binary, evidence: recorder}
	output, err := runner.call(context.Background(), "verify-created-version", "apprun-dedicated-api", "version", "read")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "private-test-secret") {
		t.Fatalf("CLI output = %q, want the unredacted response for validation", output)
	}
	record, err := os.ReadFile(filepath.Join(recorder.Dir(), "001-verify-created-version.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(record), "private-test-secret") || !strings.Contains(string(record), "REDACTED") {
		t.Fatalf("private response leaked into evidence: %s", record)
	}
}

func containsStep(steps []string, expected string) bool {
	return indexOf(steps, expected) >= 0
}

func indexOf(steps []string, expected string) int {
	for index, step := range steps {
		if step == expected {
			return index
		}
	}
	return -1
}
