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

	"github.com/itchyny/gojq"
)

type fakeCLI struct {
	users             map[string]item
	groups            map[string]item
	folders           map[string]item
	projects          map[string]item
	servicePrincipals map[string]item
	listPages         map[string][][]item
	listPageCounts    map[string][]int
	listHasNext       map[string][]bool
	calls             []string
	argsByStep        map[string][]string
	failAt            string
}

func newFakeCLI() *fakeCLI {
	return &fakeCLI{
		users:             make(map[string]item),
		groups:            make(map[string]item),
		folders:           make(map[string]item),
		projects:          make(map[string]item),
		servicePrincipals: make(map[string]item),
	}
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.argsByStep == nil {
		f.argsByStep = make(map[string][]string)
	}
	f.argsByStep[step] = append([]string(nil), args...)
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
	case step == "create-folder":
		created := item{ID: 3001, Name: argument(args, "--name"), Description: argument(args, "--description")}
		f.folders[created.Name] = created
		return json.Marshal(created)
	case step == "create-project":
		created := item{
			ID:          4001,
			Name:        argument(args, "--name"),
			Code:        argument(args, "--code"),
			Description: argument(args, "--description"),
		}
		f.projects[created.Name] = created
		return json.Marshal(created)
	case step == "create-service-principal":
		projectID, err := strconv.Atoi(argument(args, "--project-id"))
		if err != nil {
			return nil, err
		}
		created := item{
			ID:          5001,
			Name:        argument(args, "--name"),
			Description: argument(args, "--description"),
			ProjectID:   projectID,
		}
		f.servicePrincipals[created.Name] = created
		return json.Marshal(created)
	case step == "read-user", step == "verify-email", step == "verify-email-unregistered":
		return f.readOne(f.users, lastArg(args))
	case step == "read-group":
		return f.readOne(f.groups, lastArg(args))
	case step == "read-folder":
		return f.readOne(f.folders, lastArg(args))
	case step == "read-project":
		return f.readOne(f.projects, lastArg(args))
	case step == "read-service-principal":
		return f.readOne(f.servicePrincipals, lastArg(args))
	case step == "update-user":
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.users[current.Code] = current
		return json.Marshal(current)
	case step == "update-folder":
		current, ok := f.findByID(f.folders, args[3])
		if !ok {
			return nil, errInvalid("folder not found: " + args[3])
		}
		delete(f.folders, current.Name)
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.folders[current.Name] = current
		return json.Marshal(current)
	case step == "update-project":
		current, ok := f.findByID(f.projects, args[3])
		if !ok {
			return nil, errInvalid("project not found: " + args[3])
		}
		delete(f.projects, current.Name)
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.projects[current.Name] = current
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
		switch kind {
		case "user":
			return f.readOne(f.users, lastArg(args))
		case "group":
			return f.readOne(f.groups, lastArg(args))
		case "folder":
			return f.readOne(f.folders, lastArg(args))
		case "project":
			return f.readOne(f.projects, lastArg(args))
		case "service-principal":
			return f.readOne(f.servicePrincipals, lastArg(args))
		}
		return nil, errInvalid("unknown cleanup resource " + kind)
	case strings.HasPrefix(step, "cleanup-delete-"):
		kind := strings.TrimPrefix(step, "cleanup-delete-")
		id := lastArg(args)
		switch kind {
		case "user":
			current, ok := f.findByID(f.users, id)
			if !ok {
				return nil, errInvalid("user not found: " + id)
			}
			delete(f.users, current.Code)
			return nil, nil
		case "group":
			current, ok := f.findByID(f.groups, id)
			if !ok {
				return nil, errInvalid("group not found: " + id)
			}
			delete(f.groups, current.Name)
			return nil, nil
		case "folder":
			current, ok := f.findByID(f.folders, id)
			if !ok {
				return nil, errInvalid("folder not found: " + id)
			}
			delete(f.folders, current.Name)
			return nil, nil
		case "project":
			current, ok := f.findByID(f.projects, id)
			if !ok {
				return nil, errInvalid("project not found: " + id)
			}
			delete(f.projects, current.Name)
			return nil, nil
		case "service-principal":
			current, ok := f.findByID(f.servicePrincipals, id)
			if !ok {
				return nil, errInvalid("service principal not found: " + id)
			}
			delete(f.servicePrincipals, current.Name)
			return nil, nil
		}
		return nil, errInvalid("unknown cleanup resource " + kind)
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
			pageCount := len(items)
			if counts, ok := f.listPageCounts[kind]; ok && page <= len(counts) {
				pageCount = counts[page-1]
			}
			var hasNext *bool
			if values, ok := f.listHasNext[kind]; ok && page <= len(values) {
				hasNext = &values[page-1]
			}
			return json.Marshal(projectedPage{Items: items, PageCount: pageCount, HasNext: hasNext})
		}
		return json.Marshal(projectedPage{Items: []item{}})
	}
	if page != 1 {
		return json.Marshal(projectedPage{Items: []item{}})
	}
	switch kind {
	case "user":
		items := mapValues(f.users)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "group":
		items := mapValues(f.groups)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "folder":
		items := mapValues(f.folders)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "project":
		items := mapValues(f.projects)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "service-principal":
		items := mapValues(f.servicePrincipals)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
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
		folder:   resource{kind: "folder", name: "skr-e2e-iam-" + suffix, description: "skr-e2e-iam/" + suffix},
		project:  resource{kind: "project", name: "skr-e2e-iam-" + suffix, description: "skr-e2e-iam/" + suffix},
		servicePrincipal: resource{
			kind: "service-principal", name: "skr-e2e-iam-" + suffix, description: "skr-e2e-iam/" + suffix,
		},
		email:   "skr-e2e-iam@example.com",
		renamed: "skr-e2e-iam-" + suffix + "-renamed",
	}
}

func TestScenarioCreatesAndCleansUpResources(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.users) != 0 || len(fake.groups) != 0 || len(fake.folders) != 0 || len(fake.projects) != 0 || len(fake.servicePrincipals) != 0 {
		t.Fatalf("resources remain after E2E: users=%+v groups=%+v folders=%+v projects=%+v servicePrincipals=%+v",
			fake.users, fake.groups, fake.folders, fake.projects, fake.servicePrincipals)
	}
	for _, want := range []string{
		"preflight-users-page-1",
		"preflight-groups-page-1",
		"preflight-folders-page-1",
		"preflight-projects-page-1",
		"preflight-service-principals-page-1",
		"create-folder",
		"create-project",
		"create-user",
		"create-service-principal",
		"verify-user-list-page-1",
		"read-user",
		"update-user",
		"update-folder",
		"update-project",
		"register-email",
		"verify-email",
		"unregister-email",
		"verify-email-unregistered",
		"create-group",
		"update-memberships",
		"read-memberships",
		"cleanup-delete-service-principal",
		"cleanup-verify-service-principal-page-1",
		"cleanup-delete-group",
		"cleanup-verify-group-page-1",
		"cleanup-delete-user",
		"cleanup-verify-user-page-1",
		"cleanup-delete-project",
		"cleanup-verify-project-page-1",
		"cleanup-delete-folder",
		"cleanup-verify-folder-page-1",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	queryArgs := strings.Join(fake.argsByStep["preflight-users-page-1"], " ")
	if !strings.Contains(queryArgs, "--query") || !strings.Contains(queryArgs, `.name == "skr-e2e-iam-test00ff"`) {
		t.Errorf("preflight list query = %q, want an exact-name projection", queryArgs)
	}
	if strings.Contains(queryArgs, "email") {
		t.Errorf("preflight query projects unnecessary email data: %q", queryArgs)
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-user") < strings.Index(got, "cleanup-delete-group") {
		t.Fatal("the group must be deleted before its member user")
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-project") < strings.Index(got, "cleanup-delete-service-principal") {
		t.Fatal("the service principal must be deleted before its project")
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-folder") < strings.Index(got, "cleanup-delete-project") {
		t.Fatal("the project must be deleted before its folder")
	}
}

func TestListReadsEveryPage(t *testing.T) {
	fake := &fakeCLI{
		users:             make(map[string]item),
		groups:            make(map[string]item),
		folders:           make(map[string]item),
		projects:          make(map[string]item),
		servicePrincipals: make(map[string]item),
		listPages: map[string][][]item{
			"user": {
				{{ID: 1001, Name: "first"}},
				{{ID: 1002, Name: "second"}},
			},
		},
		listPageCounts: map[string][]int{"user": {userGroupPageSize, 1}},
	}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-users", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 1001 || items[1].ID != 1002 {
		t.Fatalf("list returned %#v, want items from both pages", items)
	}
	for _, want := range []string{"preflight-users-page-1", "preflight-users-page-2"} {
		if !contains(fake.calls, want) {
			t.Errorf("list did not fetch %s: %v", want, fake.calls)
		}
	}
	if contains(fake.calls, "preflight-users-page-3") {
		t.Errorf("list fetched a page after a short final page: %v", fake.calls)
	}
}

func TestListQueriesCompile(t *testing.T) {
	for _, kind := range []string{"user", "group", "folder", "project", "service-principal"} {
		for _, targetNames := range [][]string{nil, {"target"}} {
			query := listQuery(kind, targetNames...)
			parsed, err := gojq.Parse(query)
			if err != nil {
				t.Errorf("parse %s list query %q: %v", kind, query, err)
				continue
			}
			if _, err := gojq.Compile(parsed); err != nil {
				t.Errorf("compile %s list query %q: %v", kind, query, err)
			}
		}
	}
}

func TestListStopsWhenAPIHasNoNextPage(t *testing.T) {
	fake := newFakeCLI()
	fake.listPages = map[string][][]item{
		"project": {
			{{ID: 1001, Name: "first"}},
			{{ID: 1002, Name: "unrequested-page"}},
		},
	}
	fake.listHasNext = map[string][]bool{"project": {false}}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-projects", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 1001 {
		t.Fatalf("list returned %#v, want the first API page only", items)
	}
	if contains(fake.calls, "preflight-projects-page-2") {
		t.Fatalf("list requested a page after the API reported no next page: %v", fake.calls)
	}
}

func TestListContinuesAcrossUnmatchedProjectedPages(t *testing.T) {
	fake := newFakeCLI()
	fake.listPages = map[string][][]item{
		"user": {
			{},
			{{ID: 1002, Name: "target", Description: "skr-e2e-iam/test00ff"}},
		},
	}
	fake.listPageCounts = map[string][]int{"user": {userGroupPageSize, 1}}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-users", "user", "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "target" {
		t.Fatalf("list returned %#v, want target from the second page", items)
	}
	if !contains(fake.calls, "preflight-users-page-2") || contains(fake.calls, "preflight-users-page-3") {
		t.Fatalf("list did not stop after checking the final page: %v", fake.calls)
	}
}

func TestUserGroupListsRequestLargePages(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		fake := newFakeCLI()
		s := testScenario(t, fake)
		if _, err := s.list(context.Background(), "preflight-"+kind, kind); err != nil {
			t.Fatal(err)
		}
		args := fake.argsByStep["preflight-"+kind+"-page-1"]
		if got := argument(args, "--per-page"); got != strconv.Itoa(userGroupPageSize) {
			t.Errorf("%s per-page = %q, want %d", kind, got, userGroupPageSize)
		}
	}
}

func TestScenarioRecoversResourcesWhenCreateReturnsError(t *testing.T) {
	fake := newFakeCLI()
	fake.failAt = "create-group"
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
