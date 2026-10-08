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
	"os"
	"strconv"
	"strings"
	"testing"
)

type fakeCLI struct {
	users  map[string]item
	groups map[string]item
	calls  []string
	failAt string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failAt {
		return nil, errFail(step)
	}
	switch {
	case step == "verify-user-list":
		return f.list("user")
	case strings.HasPrefix(step, "preflight-"), strings.HasPrefix(step, "cleanup-verify-"):
		kind := strings.TrimPrefix(step, "preflight-")
		kind = strings.TrimPrefix(kind, "cleanup-verify-")
		kind = strings.TrimSuffix(kind, "s")
		return f.list(kind)
	case step == "create-user":
		name := argument(args, "--name")
		code := argument(args, "--code")
		if code == "" {
			code = name
		}
		passwordFile := argument(args, "--password-file")
		if passwordFile == "" {
			return nil, errInvalid("create-user requires --password-file")
		}
		//nolint:gosec // The path is created by this test in t.TempDir().
		password, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(password), "Sk9") {
			return nil, errInvalid("create-user requires the E2E password file contents")
		}
		created := item{ID: 1001, Name: name, Code: code, Description: argument(args, "--description")}
		f.users[code] = created
		return json.Marshal(created)
	case step == "create-group":
		created := item{ID: 2001, Name: argument(args, "--name"), Description: argument(args, "--description")}
		f.groups[created.Name] = created
		return json.Marshal(created)
	case step == "read-user", step == "verify-email", step == "unregister-email":
		return f.readOne(f.users, lastArg(args))
	case step == "read-group":
		return f.readOne(f.groups, lastArg(args))
	case step == "update-user":
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.users[current.Code] = current
		return json.Marshal(current)
	case step == "register-email":
		if argument(args, "--email") == "" {
			return nil, errInvalid("register-email requires --email")
		}
		return nil, nil
	case step == "update-memberships", step == "read-memberships":
		return json.Marshal([]map[string]int{{"id": 1001}})
	case strings.HasPrefix(step, "cleanup-read-"):
		kind := strings.TrimPrefix(step, "cleanup-read-")
		if kind == "user" {
			return f.readOne(f.users, lastArg(args))
		}
		return f.readOne(f.groups, lastArg(args))
	case strings.HasPrefix(step, "cleanup-delete-"):
		kind := strings.TrimPrefix(step, "cleanup-delete-")
		id := lastArg(args)
		if kind == "user" {
			current, ok := f.findByID(f.users, id)
			if !ok {
				return nil, errInvalid("user not found: " + id)
			}
			delete(f.users, current.Code)
			return nil, nil
		}
		current, ok := f.findByID(f.groups, id)
		if !ok {
			return nil, errInvalid("group not found: " + id)
		}
		delete(f.groups, current.Name)
		return nil, nil
	default:
		return nil, errInvalid("unexpected E2E step " + step)
	}
}

func (f *fakeCLI) list(kind string) ([]byte, error) {
	switch kind {
	case "user":
		return json.Marshal(mapValues(f.users))
	case "group":
		return json.Marshal(mapValues(f.groups))
	default:
		return nil, errInvalid("unknown resource kind " + kind)
	}
}

func (f *fakeCLI) readOne(values map[string]item, id string) ([]byte, error) {
	current, ok := f.findByID(values, id)
	if !ok {
		return nil, errInvalid("resource not found: " + id)
	}
	return json.Marshal(current)
}

func (f *fakeCLI) findByID(values map[string]item, id string) (item, bool) {
	for _, value := range values {
		if strconv.Itoa(value.ID) == id {
			return value, true
		}
	}
	return item{}, false
}

func mapValues(values map[string]item) []item {
	result := make([]item, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}

func argument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

type stepError struct{ message string }

func (e *stepError) Error() string { return e.message }

func errFail(step string) error {
	return &stepError{message: "injected CLI failure at " + step}
}

func errInvalid(message string) error {
	return &stepError{message: message}
}

func testScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	suffix := "test00ff"
	//nolint:gosec // The scenario uses a fake password only in an in-memory CLI test.
	return &scenario{
		client:   fake,
		password: "Sk9-Password-Test-0000000000",
		suffix:   suffix,
		user:     resource{kind: "user", name: "skr-e2e-iam-" + suffix, description: "skr-e2e-iam/" + suffix},
		group:    resource{kind: "group", name: "skr-e2e-iam-" + suffix, description: "skr-e2e-iam/" + suffix},
		email:    "skr-e2e-iam@example.com",
		renamed:  "skr-e2e-iam-" + suffix + "-renamed",
	}
}

func TestScenarioCreatesAndCleansUpResources(t *testing.T) {
	fake := &fakeCLI{users: make(map[string]item), groups: make(map[string]item)}
	s := testScenario(t, fake)
	if err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.users) != 0 || len(fake.groups) != 0 {
		t.Fatalf("resources remain after E2E: users=%+v groups=%+v", fake.users, fake.groups)
	}
	for _, want := range []string{
		"preflight-users",
		"preflight-groups",
		"create-user",
		"verify-user-list",
		"read-user",
		"update-user",
		"register-email",
		"verify-email",
		"unregister-email",
		"create-group",
		"update-memberships",
		"read-memberships",
		"cleanup-delete-group",
		"cleanup-verify-group",
		"cleanup-delete-user",
		"cleanup-verify-user",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-user") < strings.Index(got, "cleanup-delete-group") {
		t.Fatal("the group must be deleted before its member user")
	}
}

func TestScenarioRecoversResourcesWhenCreateReturnsError(t *testing.T) {
	fake := &fakeCLI{users: make(map[string]item), groups: make(map[string]item), failAt: "create-group"}
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure at create-group") {
		t.Fatalf("scenario error = %v, want create-group failure", err)
	}
	if len(fake.groups) != 0 {
		t.Fatalf("group remains after failure: %+v", fake.groups)
	}
	if !contains(fake.calls, "cleanup-delete-user") || !contains(fake.calls, "cleanup-verify-user") {
		t.Fatalf("created user was not cleaned up: %v", fake.calls)
	}
	if len(fake.users) != 0 {
		t.Fatalf("user remains after cleanup: %+v", fake.users)
	}
}

func TestScenarioRefusesExistingResource(t *testing.T) {
	fake := &fakeCLI{
		users:  make(map[string]item),
		groups: make(map[string]item),
	}
	fake.users["existing"] = item{ID: 42, Name: "skr-e2e-iam-test00ff", Code: "existing", Description: "unrelated"}
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want existing resource refusal", err)
	}
	if len(fake.calls) != 2 || len(fake.users) != 1 {
		t.Fatalf("scenario modified an existing resource: calls=%v users=%+v", fake.calls, fake.users)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
