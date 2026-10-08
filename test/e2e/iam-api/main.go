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
	commandTimeout      = 60 * time.Second
	cleanupTimeout      = 3 * time.Minute
)

type item struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Email       string `json:"email"`
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

	if err := r.evidence.Record(step, r.binary, args, nil, stdout.String(), stderr.String(), runErr, false); err != nil {
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
	description string
	id          string
	attempted   bool
}

type scenario struct {
	client   cli
	password string
	suffix   string
	user     resource
	group    resource
	email    string
	renamed  string
}

func (s *scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func decodeItems(step string, data []byte) ([]item, error) {
	var items []item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode resource list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
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

func (s *scenario) list(ctx context.Context, step, kind string) ([]item, error) {
	var args []string
	switch kind {
	case "user":
		args = []string{"iam-api", "user", "list"}
	case "group":
		args = []string{"iam-api", "group", "list"}
	default:
		return nil, fmt.Errorf("%s: unknown resource kind %q", step, kind)
	}
	var all []item
	for page := 1; ; page++ {
		pageStep := fmt.Sprintf("%s-page-%d", step, page)
		pageArgs := append(append([]string(nil), args...), "--page", strconv.Itoa(page))
		data, err := s.call(ctx, pageStep, pageArgs...)
		if err != nil {
			return nil, err
		}
		items, err := decodeItems(pageStep, data)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return all, nil
		}
		all = append(all, items...)
	}
}

func (s *scenario) findByName(ctx context.Context, step string, target resource) (item, bool, error) {
	items, err := s.list(ctx, step, target.kind)
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
	return matches[0], true, nil
}

func checkIdentity(candidate item, target resource) error {
	if !candidate.markedForThisRun(strings.TrimPrefix(target.description, resourceDescription)) {
		return fmt.Errorf("%s identity mismatch for %q", target.kind, target.name)
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
	for _, target := range []resource{s.user, s.group} {
		items, err := s.list(ctx, "preflight-"+target.kind+"s", target.kind)
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
		"--name", s.user.name, "--code", s.user.name, "--description", s.user.description,
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

func (s *scenario) verifyUser(ctx context.Context) error {
	items, err := s.list(ctx, "verify-user-list", "user")
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
	// The renamed user keeps its code and description, so the run identity stays valid.
	s.user.name = s.renamed
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
	if !read.markedForThisRun(s.suffix) {
		return fmt.Errorf("verify-email: identity mismatch for user %s", s.user.id)
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
	if !read.markedForThisRun(s.suffix) {
		return fmt.Errorf("verify-email-unregistered: identity mismatch for user %s", s.user.id)
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
	for _, target := range []*resource{&s.group, &s.user} {
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

	if err := s.createUser(ctx); err != nil {
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
	confirmed := flag.Bool("confirm-iam-live", false, "Confirm creating and deleting a test IAM user and group in the authenticated organization")
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
	fmt.Println("IAM user and group:", name)
	fmt.Println("Check `skr config current` before running; this command uses the selected SDK profile.")
	fmt.Println("This command creates a test IAM user and group, then deletes them after verification.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := &cliRunner{binary: path, evidence: recorder}
	s := &scenario{
		client:   runner,
		password: password,
		suffix:   suffix,
		user:     resource{kind: "user", name: name, description: desc},
		group:    resource{kind: "group", name: name, description: desc},
		email:    "skr-e2e-iam@example.com",
		renamed:  name + "-renamed",
	}
	if err := s.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "IAM E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("IAM E2E passed; the test IAM user and group were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
