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
	users     map[string]item
	groups    map[string]item
	listPages map[string][][]item
	calls     []string
	failAt    string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failAt && step != "create-group" {
		return nil, errFail(step)
	}
	if len(args) >= 3 && args[0] == "iam-api" && args[2] == "list" {
		page := 1
		if value := argument(args, "--page"); value != "" {
			var err error
			page, err = strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
		}
		return f.list(args[1], page)
	}
	switch {
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
		if step == f.failAt {
			return nil, errFail(step)
		}
		return json.Marshal(created)
	case step == "read-user", step == "verify-email", step == "verify-email-unregistered":
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
		email := argument(args, "--email")
		if email == "" {
			return nil, errInvalid("register-email requires --email")
		}
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Email = email
		f.users[current.Code] = current
		return nil, nil
	case step == "unregister-email":
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Email = ""
		f.users[current.Code] = current
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

func (f *fakeCLI) list(kind string, page int) ([]byte, error) {
	if pages, ok := f.listPages[kind]; ok {
		if page > 0 && page <= len(pages) {
			items := pages[page-1]
			if items == nil {
				items = []item{}
			}
			return json.Marshal(items)
		}
		return json.Marshal([]item{})
	}
	if page != 1 {
		return json.Marshal([]item{})
	}
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
		"preflight-users-page-1",
		"preflight-groups-page-1",
		"create-user",
		"verify-user-list-page-1",
		"verify-user-list-page-2",
		"read-user",
		"update-user",
		"register-email",
		"verify-email",
		"unregister-email",
		"verify-email-unregistered",
		"create-group",
		"update-memberships",
		"read-memberships",
		"cleanup-delete-group",
		"cleanup-verify-group-page-1",
		"cleanup-delete-user",
		"cleanup-verify-user-page-1",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-user") < strings.Index(got, "cleanup-delete-group") {
		t.Fatal("the group must be deleted before its member user")
	}
}

func TestListReadsEveryPage(t *testing.T) {
	fake := &fakeCLI{
		users:  make(map[string]item),
		groups: make(map[string]item),
		listPages: map[string][][]item{
			"user": {
				{{ID: 1001, Name: "first"}},
				{{ID: 1002, Name: "second"}},
			},
		},
	}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-users", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 1001 || items[1].ID != 1002 {
		t.Fatalf("list returned %#v, want items from both pages", items)
	}
	for _, want := range []string{"preflight-users-page-1", "preflight-users-page-2", "preflight-users-page-3"} {
		if !contains(fake.calls, want) {
			t.Errorf("list did not fetch %s: %v", want, fake.calls)
		}
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
	for _, want := range []string{
		"cleanup-find-group-page-1",
		"cleanup-delete-group",
		"cleanup-verify-group-page-1",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("uncertain group create was not recovered via %s: %v", want, fake.calls)
		}
	}
	if !contains(fake.calls, "cleanup-delete-user") || !contains(fake.calls, "cleanup-verify-user-page-1") {
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
	if len(fake.calls) != 3 || len(fake.users) != 1 {
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
