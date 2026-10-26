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
	"slices"
	"strings"
	"testing"
)

func newProjectAPIKeyScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	s := testScenario(t, fake)
	// Capture the auth context the same way the runner does, including the
	// limited project.
	if err := s.checkAuthContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProjectAPIKeyScenarioCoversLifecycle(t *testing.T) {
	fake := newFakeCLI()
	s := newProjectAPIKeyScenario(t, fake)
	if err := s.projectAPIKeyScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"create-project-api-key",
		"read-project-api-key",
		"list-project-api-keys",
		"update-project-api-key",
		"delete-project-api-key",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("project API key scenario did not call %s: %v", want, fake.calls)
		}
	}
	// The create and read responses contain the access token, so the request
	// must be read from a private file instead of an inline value.
	if got := fake.argsByStep["create-project-api-key"]; !requestFromFile(got) {
		t.Errorf("create-project-api-key args = %v, want --request @path", got)
	}
	// The principal is limited to a project in the fake auth context, so the
	// key must be pinned to that project instead of the project of this run.
	if pinned, ok := fake.lastPakRequest["project_id"].(float64); !ok || int(pinned) != fakeLimitedProjectID {
		t.Errorf("create-project-api-key pinned project = %v, want %d", fake.lastPakRequest["project_id"], fakeLimitedProjectID)
	}
	if !fake.pakDeleted {
		t.Error("project API key scenario left the key undeleted")
	}
	if err := s.cleanupManaged(context.Background(), &s.projectAPIKey); err != nil {
		t.Fatalf("cleanupManaged after a completed scenario: %v", err)
	}
}

func TestProjectAPIKeyScenarioCleansUpWhenListFails(t *testing.T) {
	fake := newFakeCLI()
	s := newProjectAPIKeyScenario(t, fake)
	fake.failAt = "list-project-api-keys"
	if err := s.projectAPIKeyScenario(context.Background()); err == nil {
		t.Fatal("project API key scenario succeeded although the list failed")
	}
	if fake.pakDeleted {
		t.Fatal("the key was deleted before the cleanup ran")
	}
	if err := s.cleanupManaged(context.Background(), &s.projectAPIKey); err != nil {
		t.Fatalf("cleanupManaged: %v", err)
	}
	if !fake.pakDeleted {
		t.Error("cleanupManaged did not delete the left-behind key")
	}
}

func TestProjectAPIKeyScenarioCleanupRefusesIdentityMismatch(t *testing.T) {
	fake := newFakeCLI()
	s := newProjectAPIKeyScenario(t, fake)
	fake.failAt = "list-project-api-keys"
	if err := s.projectAPIKeyScenario(context.Background()); err == nil {
		t.Fatal("project API key scenario succeeded although the list failed")
	}
	fake.pakName = "someone-elses-key"
	if err := s.cleanupManaged(context.Background(), &s.projectAPIKey); err == nil {
		t.Fatal("cleanupManaged deleted a key with a different name")
	} else if !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.pakDeleted {
		t.Error("cleanupManaged deleted the mismatching key")
	}
}
