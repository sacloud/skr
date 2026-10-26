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
	"strings"
	"testing"
)

func newServicePrincipalScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	s := testScenario(t, fake)
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createServicePrincipal(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServicePrincipalScenarioUpdatesAndRestoresName(t *testing.T) {
	fake := newFakeCLI()
	s := newServicePrincipalScenario(t, fake)
	if err := s.updateServicePrincipal(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"update-service-principal", "restore-service-principal"} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("service principal scenario did not call %s: %v", want, fake.calls)
		}
	}
	for step, wantName := range map[string]string{
		"update-service-principal":  s.servicePrincipal.name + "-renamed",
		"restore-service-principal": s.servicePrincipal.name,
	} {
		request := updateRequest(fake, step)
		var payload map[string]string
		if err := json.Unmarshal([]byte(request), &payload); err != nil {
			t.Fatalf("decode %s request %q: %v", step, request, err)
		}
		if payload["description"] == "" {
			t.Errorf("%s request = %s, want a description", step, request)
		}
		if payload["name"] != wantName {
			t.Errorf("%s request name = %q, want %q", step, payload["name"], wantName)
		}
	}
	current, ok := fake.findByID(fake.servicePrincipals, "5001")
	if !ok || current.Name != s.servicePrincipal.name {
		t.Fatalf("service principal = %+v, want the restored name %q", current, s.servicePrincipal.name)
	}
}

func TestServicePrincipalScenarioCoversKeyLifecycle(t *testing.T) {
	fake := newFakeCLI()
	s := newServicePrincipalScenario(t, fake)
	if err := s.servicePrincipalKeysScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"upload-key",
		"list-keys",
		"disable-key",
		"enable-key",
		"issue-token",
		"delete-key",
		"list-keys-after-delete",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("service principal key scenario did not call %s: %v", want, fake.calls)
		}
	}
	if !fake.spKeyDeleted {
		t.Error("the key was not deleted after the lifecycle")
	}
	// 公開鍵と assertion はプライベートなファイルに書き込まれる。
	if got := fake.argsByStep["upload-key"]; !slices.Contains(got, "--public-key-file") {
		t.Errorf("upload-key args = %v, want --public-key-file", got)
	}
	if got := fake.argsByStep["issue-token"]; !slices.Contains(got, "--assertion-file") {
		t.Errorf("issue-token args = %v, want --assertion-file", got)
	}
}

func TestServicePrincipalScenarioFailsWhenIssueTokenIsRejected(t *testing.T) {
	fake := newFakeCLI()
	s := newServicePrincipalScenario(t, fake)
	fake.failAt = "issue-token"
	if err := s.servicePrincipalKeysScenario(context.Background()); err == nil {
		t.Fatal("key scenario succeeded although issue-token failed")
	}
	if !strings.Contains(strings.Join(fake.calls, ","), "issue-token") {
		t.Fatal("issue-token was not called")
	}
}
