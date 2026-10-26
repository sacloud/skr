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

func TestReadOnlyScenarioCoversCatalogAndPolicies(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.readOnlyScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"iam-role-list",
		"iam-role-read",
		"id-role-list",
		"id-role-read",
		"id-policy-read-organization",
		"read-auth-context",
		"read-auth-conditions",
		"read-password-policy",
		"policy-read-organization",
		"organization-read",
		"service-policy-is-enabled",
		"read-service-policy",
		"list-rule-templates",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("read-only scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := fake.argsByStep["iam-role-read"]; !slices.Equal(got, []string{"iam-api", "iam-role", "read", "addon-admin"}) {
		t.Errorf("iam-role-read args = %v", got)
	}
	if got := fake.argsByStep["id-role-read"]; !slices.Equal(got, []string{"iam-api", "id-role", "read", "identity-admin"}) {
		t.Errorf("id-role-read args = %v", got)
	}
	if got := fake.argsByStep["id-policy-read-organization"]; !slices.Equal(got, []string{"iam-api", "id-policy", "read-organization"}) {
		t.Errorf("id-policy-read-organization args = %v", got)
	}
}

func TestReadOnlyScenarioDoesNotCreateResources(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.readOnlyScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.users) != 0 || len(fake.groups) != 0 || len(fake.folders) != 0 || len(fake.projects) != 0 || len(fake.servicePrincipals) != 0 {
		t.Fatalf("read-only scenario created resources: users=%+v folders=%+v", fake.users, fake.folders)
	}
}

func TestReadResourcePoliciesUsesRunResources(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.createFolder(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.readResourcePolicies(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.argsByStep["policy-read-folder"]; !slices.Equal(got, []string{"iam-api", "policy", "read-folder", "3001"}) {
		t.Errorf("policy-read-folder args = %v, want the folder created by this run", got)
	}
	if got := fake.argsByStep["policy-read-project"]; !slices.Equal(got, []string{"iam-api", "policy", "read-project", "4001"}) {
		t.Errorf("policy-read-project args = %v, want the project created by this run", got)
	}
	if err := s.cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyScenarioReportsFailures(t *testing.T) {
	tests := []struct {
		name     string
		fake     func(f *fakeCLI)
		fails    string
		contains bool
	}{
		{
			name:  "empty role catalog",
			fake:  func(f *fakeCLI) { f.emptyRoleCatalog = true },
			fails: "iam-role-list: the role catalog is empty",
		},
		{
			name:  "iam-role read mismatch",
			fake:  func(f *fakeCLI) { f.iamRoleReadID = "other-role" },
			fails: "iam-role read mismatch: got \"other-role\"/\"Add-on service\", want \"addon-admin\"/\"Add-on service\"",
		},
		{
			name:  "id-role read mismatch",
			fake:  func(f *fakeCLI) { f.idRoleReadID = "other-role" },
			fails: "id-role read mismatch: got \"other-role\"/\"ID administrator\", want \"identity-admin\"/\"ID administrator\"",
		},
		{
			name:  "auth context without resource ID",
			fake:  func(f *fakeCLI) { f.authResourceID = -1 },
			fails: "read-auth-context: resource_id = -1, want a positive value",
		},
		{
			name:  "password policy without min length",
			fake:  func(f *fakeCLI) { f.passwordMinLength = -1 },
			fails: "read-password-policy: min_length = -1, want a positive value",
		},
		{
			name:  "organization without ID",
			fake:  func(f *fakeCLI) { f.organizationID = -1 },
			fails: "organization-read: id = -1, want a positive value",
		},
		{
			name:     "broken policy bindings JSON",
			fake:     func(f *fakeCLI) { f.brokenPolicyJSON = true },
			fails:    "id-policy-read-organization: decode policy bindings",
			contains: true,
		},
		{
			name:  "empty rule template catalog",
			fake:  func(f *fakeCLI) { f.emptyRuleTemplates = true },
			fails: "list-rule-templates: the rule template catalog is empty",
		},
		{
			name:  "injected CLI failure",
			fake:  func(f *fakeCLI) { f.failAt = "read-auth-context" },
			fails: "injected CLI failure at read-auth-context",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeCLI()
			tt.fake(fake)
			s := testScenario(t, fake)
			err := s.readOnlyScenario(context.Background())
			if err == nil {
				t.Fatalf("read-only scenario succeeded, want %q", tt.fails)
			}
			if tt.contains {
				if !strings.Contains(err.Error(), tt.fails) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.fails)
				}
				return
			}
			if err.Error() != tt.fails {
				t.Errorf("error = %q, want %q", err.Error(), tt.fails)
			}
		})
	}
}

func TestReadResourcePoliciesReportsFailures(t *testing.T) {
	fake := newFakeCLI()
	fake.brokenPolicyJSON = true
	s := testScenario(t, fake)
	if err := s.createFolder(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := s.readResourcePolicies(context.Background())
	if err == nil {
		t.Fatal("readResourcePolicies succeeded, want decode failure")
	}
	if !strings.HasPrefix(err.Error(), "policy-read-folder: decode policy bindings") {
		t.Errorf("error = %q", err.Error())
	}
	if err := s.cleanup(); err != nil {
		t.Fatal(err)
	}
}
