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

func TestScimScenarioCoversLifecycle(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.scimScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"create-scim",
		"read-scim",
		"update-scim",
		"list-scim",
		"regenerate-scim-token",
		"delete-scim",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("scim scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := fake.argsByStep["create-scim"]; !slices.Contains(got, s.scim.name) {
		t.Errorf("create-scim args = %v, want the configuration name %q", got, s.scim.name)
	}
	if got := fake.argsByStep["update-scim"]; !slices.Contains(got, s.scim.name+"-renamed") {
		t.Errorf("update-scim args = %v, want the renamed configuration", got)
	}
	if !fake.scimDeleted {
		t.Error("scim scenario left the configuration undeleted")
	}
	// The configuration is deleted inside the scenario, so the cleanup finds
	// nothing left to remove.
	if err := s.cleanupManaged(context.Background(), &s.scim); err != nil {
		t.Fatalf("cleanupManaged after a completed scenario: %v", err)
	}
}

func TestScimScenarioCleansUpWhenReadFails(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "read-scim"
	if err := s.scimScenario(context.Background()); err == nil {
		t.Fatal("scim scenario succeeded although read-scim failed")
	}
	if fake.scimDeleted {
		t.Fatal("the configuration was deleted before the cleanup ran")
	}
	if err := s.cleanupManaged(context.Background(), &s.scim); err != nil {
		t.Fatalf("cleanupManaged: %v", err)
	}
	if !fake.scimDeleted {
		t.Error("cleanupManaged did not delete the left-behind configuration")
	}
}

func TestScimScenarioCleanupRefusesIdentityMismatch(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "read-scim"
	if err := s.scimScenario(context.Background()); err == nil {
		t.Fatal("scim scenario succeeded although read-scim failed")
	}
	fake.scimName = "someone-elses-configuration"
	if err := s.cleanupManaged(context.Background(), &s.scim); err == nil {
		t.Fatal("cleanupManaged deleted a configuration with a different name")
	} else if !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.scimDeleted {
		t.Error("cleanupManaged deleted the mismatching configuration")
	}
}
