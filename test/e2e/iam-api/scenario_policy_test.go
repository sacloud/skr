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
	"slices"
	"testing"
)

func newPolicyScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	s := testScenario(t, fake)
	// runner と同じ方法で principal ID を取得する。
	if err := s.checkAuthContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createFolder(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPolicyScenarioUpdatesAndRestoresFolderBindings(t *testing.T) {
	fake := newFakeCLI()
	original := []policyBinding{fakeBinding("folder-viewer", 2002)}
	fake.folderPolicyBindings = original
	s := newPolicyScenario(t, fake)
	if err := s.updateFolderPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"read-folder-policy-before",
		"update-folder-policy",
		"read-folder-policy-updated",
		"restore-folder-policy",
		"read-folder-policy-restored",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("folder policy scenario did not call %s: %v", want, fake.calls)
		}
	}
	var updated []policyBinding
	if err := json.Unmarshal([]byte(updateRequest(fake, "update-folder-policy")), &updated); err != nil {
		t.Fatalf("decode update-folder-policy request: %v", err)
	}
	if len(updated) != 2 || updated[1].Role.ID != folderPolicyRole || updated[1].Principals[0].ID != s.principalID {
		t.Fatalf("update-folder-policy request = %s", updateRequest(fake, "update-folder-policy"))
	}
	if !slices.EqualFunc(fake.folderPolicyBindings, original, policyBindingEqual) {
		t.Fatalf("folder bindings = %s, want the original bindings", policySummary(fake.folderPolicyBindings))
	}
}

func TestPolicyScenarioUpdatesAndRestoresProjectBindings(t *testing.T) {
	fake := newFakeCLI()
	s := newPolicyScenario(t, fake)
	if err := s.updateProjectPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"read-project-policy-before",
		"update-project-policy",
		"read-project-policy-updated",
		"restore-project-policy",
		"read-project-policy-restored",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("project policy scenario did not call %s: %v", want, fake.calls)
		}
	}
	var updated []policyBinding
	if err := json.Unmarshal([]byte(updateRequest(fake, "update-project-policy")), &updated); err != nil {
		t.Fatalf("decode update-project-policy request: %v", err)
	}
	if len(updated) != 2 || updated[1].Role.ID != projectPolicyRole {
		t.Fatalf("update-project-policy request = %s", updateRequest(fake, "update-project-policy"))
	}
	// fake は project を作成した principal への owner binding から始まるため、
	// 取得した binding を復元すればその binding は維持される。
	if len(fake.projectPolicyBindings) != 1 || fake.projectPolicyBindings[0].Role.ID != "owner" {
		t.Fatalf("project bindings = %s, want the restored owner binding", policySummary(fake.projectPolicyBindings))
	}
}

func TestPolicyScenarioFailsWhenUpdateIsRejected(t *testing.T) {
	fake := newFakeCLI()
	s := newPolicyScenario(t, fake)
	fake.failAt = "update-folder-policy"
	if err := s.updateFolderPolicy(context.Background()); err == nil {
		t.Fatal("folder policy update succeeded although it was rejected")
	}
}

func updateRequest(fake *fakeCLI, step string) string {
	args := fake.argsByStep[step]
	for i, arg := range args {
		if arg == "--request" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func policyBindingEqual(a, b policyBinding) bool {
	return a.Role == b.Role && slices.Equal(a.Principals, b.Principals)
}
