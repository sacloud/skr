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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	resourceNamePrefix  = "skr-e2e-iam-"
	resourceDescription = "skr-e2e-iam/"
	userGroupPageSize   = 1000
	commandTimeout      = 60 * time.Second
	cleanupTimeout      = 3 * time.Minute
)

type item struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Email       string `json:"email"`
	ProjectID   int    `json:"project_id"`
}

type projectedPage struct {
	Items     []item `json:"items"`
	PageCount int    `json:"page_count"`
	HasNext   *bool  `json:"has_next,omitempty"`
}

func (i item) id() string {
	return strconv.Itoa(i.ID)
}

// markedForThisRun reports whether the resource was created by this E2E run.
// The description carries the unique run suffix and never changes, so it stays
// valid even after the user is renamed.
func (i item) markedForThisRun(suffix string) bool {
	return i.Description == resourceDescription+suffix
}

type cli interface {
	call(ctx context.Context, step string, args ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r *cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The caller selects a local skr binary and passes structured arguments; no shell is used.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	if err := r.evidence.Record(step, args, nil, stdout.String(), stderr.String(), runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (see evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type resource struct {
	kind        string
	name        string
	code        string
	description string
	projectID   int
	id          string
	attempted   bool
}

type scenario struct {
	client           cli
	password         string
	suffix           string
	user             resource
	group            resource
	folder           resource
	project          resource
	servicePrincipal resource
	email            string
	renamed          string
}

func (s *scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func decodePage(step string, data []byte) (projectedPage, error) {
	var page projectedPage
	if err := json.Unmarshal(data, &page); err != nil {
		return projectedPage{}, fmt.Errorf("%s: decode projected list: %w", step, err)
	}
	if page.Items == nil {
		return projectedPage{}, fmt.Errorf("%s: projected items must be a JSON array", step)
	}
	if page.PageCount < 0 {
		return projectedPage{}, fmt.Errorf("%s: page count is negative", step)
	}
	return page, nil
}

func decodeItem(step string, data []byte) (item, error) {
	var result item
	if err := json.Unmarshal(data, &result); err != nil {
		return item{}, fmt.Errorf("%s: decode resource: %w", step, err)
	}
	if result.ID == 0 {
		return item{}, fmt.Errorf("%s: resource ID is missing", step)
	}
	return result, nil
}

func (s *scenario) list(ctx context.Context, step, kind string, targetNames ...string) ([]item, error) {
	if kind != "user" && kind != "group" && kind != "folder" && kind != "project" && kind != "service-principal" {
		return nil, fmt.Errorf("%s: unknown resource kind %q", step, kind)
	}
	if len(targetNames) > 1 {
		return nil, fmt.Errorf("%s: at most one resource name can be matched", step)
	}
	args := []string{"iam-api", kind, "list"}

	query := listQuery(kind, targetNames...)
	var all []item
	for pageNumber := 1; ; pageNumber++ {
		pageStep := fmt.Sprintf("%s-page-%d", step, pageNumber)
		pageArgs := append(append([]string(nil), args...), "--page", strconv.Itoa(pageNumber), "--query", query)
		if kind == "user" || kind == "group" {
			pageArgs = append(pageArgs, "--per-page", strconv.Itoa(userGroupPageSize))
		}
		data, err := s.call(ctx, pageStep, pageArgs...)
		if err != nil {
			return nil, err
		}
		page, err := decodePage(pageStep, data)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.HasNext != nil && !*page.HasNext {
			return all, nil
		}
		if page.HasNext == nil && (page.PageCount == 0 ||
			(kind != "user" && kind != "group") || page.PageCount < userGroupPageSize) {
			return all, nil
		}
	}
}

func listQuery(kind string, targetNames ...string) string {
	fields := "{id,name,code,description}"
	pageItems := "map(" + fields + ")"
	pageCount := "length"
	hasNext := ""
	if kind == "folder" || kind == "project" || kind == "service-principal" {
		fields = "{id,name,code,description,project_id}"
		pageItems = ".items | map(" + fields + ")"
		pageCount = "(.items | length)"
		hasNext = ", has_next: (.next != null)"
	}
	if len(targetNames) == 1 {
		selection := "map(select(.name == " + strconv.Quote(targetNames[0]) + ") | " + fields + ")"
		if kind == "folder" || kind == "project" || kind == "service-principal" {
			pageItems = ".items | " + selection
		} else {
			pageItems = selection
		}
	}
	return "{items: (" + pageItems + "), page_count: " + pageCount + hasNext + "}"
}

func (s *scenario) findByName(ctx context.Context, step string, target resource) (item, bool, error) {
	items, err := s.list(ctx, step, target.kind, target.name)
	if err != nil {
		return item{}, false, err
	}
	var matches []item
	for _, candidate := range items {
		if candidate.Name == target.name {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return item{}, false, nil
	}
	if len(matches) != 1 || !matches[0].markedForThisRun(s.suffix) {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: resource identity is not unique and marked for this E2E", step, target.name)
	}
	if target.code != "" && matches[0].Code != target.code {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: resource code does not match this E2E", step, target.name)
	}
	if target.projectID != 0 && matches[0].ProjectID != target.projectID {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: project ID does not match this E2E", step, target.name)
	}
	return matches[0], true, nil
}

func checkIdentity(candidate item, target resource) error {
	if target.id != "" && candidate.id() != target.id {
		return fmt.Errorf("%s ID mismatch: got %s, want %s", target.kind, candidate.id(), target.id)
	}
	if candidate.Name != target.name {
		return fmt.Errorf("%s name mismatch: got %q, want %q", target.kind, candidate.Name, target.name)
	}
	if candidate.Description != target.description || !strings.HasPrefix(candidate.Description, resourceDescription) {
		return fmt.Errorf("%s description mismatch for %q", target.kind, target.name)
	}
	if target.code != "" && candidate.Code != target.code {
		return fmt.Errorf("%s code mismatch for %q", target.kind, target.name)
	}
	if target.projectID != 0 && candidate.ProjectID != target.projectID {
		return fmt.Errorf("%s project ID mismatch for %q", target.kind, target.name)
	}
	return nil
}

func (s *scenario) preflight(ctx context.Context) error {
	profile, err := s.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	for _, target := range []resource{s.user, s.group, s.folder, s.project, s.servicePrincipal} {
		items, err := s.list(ctx, "preflight-"+target.kind+"s", target.kind, target.name)
		if err != nil {
			return err
		}
		for _, candidate := range items {
			if candidate.Name == target.name {
				return fmt.Errorf("%s name %q already exists; refusing to modify it", target.kind, target.name)
			}
		}
	}
	return nil
}

func (s *scenario) createFolder(ctx context.Context) error {
	s.folder.attempted = true
	data, err := s.call(ctx, "create-folder", "iam-api", "folder", "create",
		"--name", s.folder.name, "--description", s.folder.description)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-folder", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.folder); err != nil {
		return err
	}
	s.folder.id = created.id()
	return nil
}

func (s *scenario) createProject(ctx context.Context) error {
	s.project.attempted = true
	data, err := s.call(ctx, "create-project", "iam-api", "project", "create",
		"--code", s.project.code, "--name", s.project.name, "--description", s.project.description,
		"--parent-folder-id", s.folder.id)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-project", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.project); err != nil {
		return err
	}
	s.project.id = created.id()
	s.servicePrincipal.projectID = created.ID
	return nil
}

func (s *scenario) createUser(ctx context.Context) error {
	s.user.attempted = true
	passwordFile := filepath.Join(os.TempDir(), "skr-e2e-iam-password-"+s.suffix)

	if err := os.WriteFile(passwordFile, []byte(s.password), 0o600); err != nil {
		return fmt.Errorf("write private password file: %w", err)
	}
	defer func() {
		if err := os.Remove(passwordFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Printf("remove private password file: %v\n", err)
		}
	}()
	data, err := s.call(ctx, "create-user", "iam-api", "user", "create",
		"--name", s.user.name, "--code", s.user.code, "--description", s.user.description,
		"--password-file", passwordFile)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-user", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.user); err != nil {
		return err
	}
	s.user.id = created.id()
	return nil
}

func (s *scenario) createServicePrincipal(ctx context.Context) error {
	s.servicePrincipal.attempted = true
	data, err := s.call(ctx, "create-service-principal", "iam-api", "service-principal", "create",
		"--project-id", s.project.id, "--name", s.servicePrincipal.name,
		"--description", s.servicePrincipal.description)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-service-principal", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.servicePrincipal); err != nil {
		return err
	}
	if strconv.Itoa(created.ProjectID) != s.project.id {
		return fmt.Errorf("create-service-principal: project ID = %d, want %s", created.ProjectID, s.project.id)
	}
	s.servicePrincipal.id = created.id()
	return nil
}

func (s *scenario) verifyUser(ctx context.Context) error {
	items, err := s.list(ctx, "verify-user-list", "user", s.user.name)
	if err != nil {
		return err
	}
	for _, candidate := range items {
		if candidate.id() == s.user.id {
			return nil
		}
	}
	return fmt.Errorf("verify-user-list: created user %s is missing from the list", s.user.id)
}

func (s *scenario) read(ctx context.Context, step string, target resource) (item, error) {
	command := target.kind
	data, err := s.call(ctx, step, "iam-api", command, "read", target.id)
	if err != nil {
		return item{}, err
	}
	return decodeItem(step, data)
}

func (s *scenario) updateUser(ctx context.Context) error {
	data, err := s.call(ctx, "update-user", "iam-api", "user", "update", s.user.id,
		"--name", s.renamed, "--description", s.user.description)
	if err != nil {
		return err
	}
	updated, err := decodeItem("update-user", data)
	if err != nil {
		return err
	}
	if updated.Name != s.renamed {
		return fmt.Errorf("update-user: name = %q, want %q", updated.Name, s.renamed)
	}
	expected := s.user
	expected.name = s.renamed
	if err := checkIdentity(updated, expected); err != nil {
		return fmt.Errorf("update-user: %w", err)
	}
	// The renamed user keeps its code and description, so the run identity stays valid.
	s.user.name = s.renamed
	return nil
}

func (s *scenario) updateFolder(ctx context.Context) error {
	renamed := s.folder.name + "-renamed"
	data, err := s.call(ctx, "update-folder", "iam-api", "folder", "update", s.folder.id,
		"--name", renamed, "--description", s.folder.description)
	if err != nil {
		return err
	}
	updated, err := decodeItem("update-folder", data)
	if err != nil {
		return err
	}
	if updated.Name != renamed {
		return fmt.Errorf("update-folder: name = %q, want %q", updated.Name, renamed)
	}
	expected := s.folder
	expected.name = renamed
	if err := checkIdentity(updated, expected); err != nil {
		return fmt.Errorf("update-folder: %w", err)
	}
	s.folder.name = renamed
	return nil
}

func (s *scenario) updateProject(ctx context.Context) error {
	renamed := s.project.name + "-renamed"
	data, err := s.call(ctx, "update-project", "iam-api", "project", "update", s.project.id,
		"--name", renamed, "--description", s.project.description)
	if err != nil {
		return err
	}
	updated, err := decodeItem("update-project", data)
	if err != nil {
		return err
	}
	if updated.Name != renamed {
		return fmt.Errorf("update-project: name = %q, want %q", updated.Name, renamed)
	}
	expected := s.project
	expected.name = renamed
	if err := checkIdentity(updated, expected); err != nil {
		return fmt.Errorf("update-project: %w", err)
	}
	s.project.name = renamed
	return nil
}

func (s *scenario) registerEmail(ctx context.Context) error {
	if _, err := s.call(ctx, "register-email", "iam-api", "user", "register-email", s.user.id, "--email", s.email); err != nil {
		return err
	}
	read, err := s.read(ctx, "verify-email", s.user)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, s.user); err != nil {
		return fmt.Errorf("verify-email: %w", err)
	}
	if read.Email != s.email {
		return fmt.Errorf("verify-email: email = %q, want %q", read.Email, s.email)
	}
	return nil
}

func (s *scenario) unregisterEmail(ctx context.Context) error {
	if _, err := s.call(ctx, "unregister-email", "iam-api", "user", "unregister-email", s.user.id); err != nil {
		return err
	}
	read, err := s.read(ctx, "verify-email-unregistered", s.user)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, s.user); err != nil {
		return fmt.Errorf("verify-email-unregistered: %w", err)
	}
	if read.Email != "" {
		return fmt.Errorf("verify-email-unregistered: email = %q, want empty", read.Email)
	}
	return nil
}

func (s *scenario) createGroup(ctx context.Context) error {
	s.group.attempted = true
	data, err := s.call(ctx, "create-group", "iam-api", "group", "create",
		"--name", s.group.name, "--description", s.group.description)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-group", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.group); err != nil {
		return err
	}
	s.group.id = created.id()
	return nil
}

func (s *scenario) updateMemberships(ctx context.Context) error {
	data, err := s.call(ctx, "update-memberships", "iam-api", "group", "update-memberships", s.group.id,
		"--request", "["+s.user.id+"]")
	if err != nil {
		return err
	}
	var memberships []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &memberships); err != nil {
		return fmt.Errorf("update-memberships: decode memberships: %w", err)
	}
	if len(memberships) != 1 || strconv.Itoa(memberships[0].ID) != s.user.id {
		return fmt.Errorf("update-memberships: memberships = %#v, want user %s", memberships, s.user.id)
	}
	return nil
}

func (s *scenario) readMemberships(ctx context.Context) error {
	data, err := s.call(ctx, "read-memberships", "iam-api", "group", "read-memberships", s.group.id)
	if err != nil {
		return err
	}
	var memberships []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &memberships); err != nil {
		return fmt.Errorf("read-memberships: decode memberships: %w", err)
	}
	if len(memberships) != 1 || strconv.Itoa(memberships[0].ID) != s.user.id {
		return fmt.Errorf("read-memberships: memberships = %#v, want user %s", memberships, s.user.id)
	}
	return nil
}

func (s *scenario) cleanupResource(ctx context.Context, target *resource) error {
	if !target.attempted {
		return nil
	}
	id := target.id
	if id == "" {
		found, ok, err := s.findByName(ctx, "cleanup-find-"+target.kind, *target)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		id = found.id()
	}
	current, err := s.read(ctx, "cleanup-read-"+target.kind, resourceWithID(*target, id))
	if err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", target.kind, err)
	}
	if err := checkIdentity(current, resourceWithID(*target, id)); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	if _, err := s.call(ctx, "cleanup-delete-"+target.kind, "iam-api", target.kind, "delete", id); err != nil {
		return fmt.Errorf("delete E2E %s: %w", target.kind, err)
	}
	remaining, ok, err := s.findByName(ctx, "cleanup-verify-"+target.kind, *target)
	if err != nil {
		return fmt.Errorf("verify E2E %s deletion: %w", target.kind, err)
	}
	if ok {
		return fmt.Errorf("E2E %s %s remains after delete", target.kind, remaining.id())
	}
	return nil
}

func resourceWithID(value resource, id string) resource {
	value.id = id
	return value
}

func (s *scenario) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var result error
	for _, target := range []*resource{&s.servicePrincipal, &s.group, &s.user, &s.project, &s.folder} {
		if err := s.cleanupResource(ctx, target); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (s *scenario) run(ctx context.Context) (result error) {
	if err := s.preflight(ctx); err != nil {
		return err
	}
	defer func() {
		if err := s.cleanup(); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", err))
		}
	}()

	if err := s.createFolder(ctx); err != nil {
		return err
	}
	if err := s.createProject(ctx); err != nil {
		return err
	}
	if err := s.createUser(ctx); err != nil {
		return err
	}
	if err := s.createServicePrincipal(ctx); err != nil {
		return err
	}
	if err := s.verifyUser(ctx); err != nil {
		return err
	}
	if _, err := s.read(ctx, "read-user", s.user); err != nil {
		return err
	}
	if err := s.updateUser(ctx); err != nil {
		return err
	}
	if err := s.registerEmail(ctx); err != nil {
		return err
	}
	if err := s.unregisterEmail(ctx); err != nil {
		return err
	}
	if err := s.updateFolder(ctx); err != nil {
		return err
	}
	if err := s.updateProject(ctx); err != nil {
		return err
	}
	if err := s.createGroup(ctx); err != nil {
		return err
	}
	if err := s.updateMemberships(ctx); err != nil {
		return err
	}
	return s.readMemberships(ctx)
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique E2E resource suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-."

func randomPassword() (string, error) {
	// 24 characters starting with a letter and a digit so that the IAM password
	// requirement (letters and digits) is satisfied.
	const prefix = "Sk9"
	value := make([]byte, 21)
	for index := range value {
		position, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		if err != nil {
			return "", fmt.Errorf("generate E2E password: %w", err)
		}
		value[index] = passwordChars[position.Int64()]
	}
	return prefix + string(value), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-iam-live", false, "Confirm creating, updating, and deleting a test IAM user, group, folder, project, and service principal in the authenticated organization")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/iam-api --skr ./skr --confirm-iam-live")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stat skr binary:", err)
		return 1
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	recorder, err := evidence.New("iam-api")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())
	defer func() {
		result := "failed"
		if exitCode == 0 {
			result = "passed"
		}
		if err := recorder.SetResult(result, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	password, err := randomPassword()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	suffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	fmt.Println("IAM folder, project, service principal, user and group:", name)
	fmt.Println("Check `skr config current` before running; this command uses the selected SDK profile.")
	fmt.Println("This command creates test IAM resources in the selected organization, verifies them, then deletes only those resources.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := &cliRunner{binary: path, evidence: recorder}
	s := &scenario{
		client:   runner,
		password: password,
		suffix:   suffix,
		user:     resource{kind: "user", name: name, code: "e2e_" + suffix, description: desc},
		group:    resource{kind: "group", name: name, description: desc},
		folder:   resource{kind: "folder", name: name, description: desc},
		project:  resource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		servicePrincipal: resource{
			kind: "service-principal", name: name, description: desc,
		},
		email:   "skr-e2e-iam@example.com",
		renamed: name + "-renamed",
	}
	if err := s.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "IAM E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("IAM E2E passed; the test IAM resources were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
