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

func TestSSOScenarioCoversLifecycle(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.ssoScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"create-sso",
		"read-sso",
		"list-sso",
		"update-sso",
		"delete-sso",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("sso scenario did not call %s: %v", want, fake.calls)
		}
	}
	// The IdP certificate contains newlines, so the request must be read from
	// a private file instead of an inline value.
	if got := fake.argsByStep["create-sso"]; !requestFromFile(got) {
		t.Errorf("create-sso args = %v, want --request @path", got)
	}
	if !fake.ssoDeleted {
		t.Error("sso scenario left the profile undeleted")
	}
	if err := s.cleanupManaged(context.Background(), &s.ssoProfile); err != nil {
		t.Fatalf("cleanupManaged after a completed scenario: %v", err)
	}
}

func requestFromFile(args []string) bool {
	for i, arg := range args {
		if arg == "--request" && i+1 < len(args) {
			return strings.HasPrefix(args[i+1], "@")
		}
	}
	return false
}

func TestSSOScenarioCleansUpWhenUpdateFails(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "update-sso"
	if err := s.ssoScenario(context.Background()); err == nil {
		t.Fatal("sso scenario succeeded although update-sso failed")
	}
	if fake.ssoDeleted {
		t.Fatal("the profile was deleted before the cleanup ran")
	}
	if err := s.cleanupManaged(context.Background(), &s.ssoProfile); err != nil {
		t.Fatalf("cleanupManaged: %v", err)
	}
	if !fake.ssoDeleted {
		t.Error("cleanupManaged did not delete the left-behind profile")
	}
}

func TestSSOScenarioCleanupRefusesIdentityMismatch(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "update-sso"
	if err := s.ssoScenario(context.Background()); err == nil {
		t.Fatal("sso scenario succeeded although update-sso failed")
	}
	fake.ssoName = "someone-elses-profile"
	if err := s.cleanupManaged(context.Background(), &s.ssoProfile); err == nil {
		t.Fatal("cleanupManaged deleted a profile with a different name")
	} else if !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.ssoDeleted {
		t.Error("cleanupManaged deleted the mismatching profile")
	}
}
