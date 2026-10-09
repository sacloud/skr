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
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	registryPrefix    = "skr-e2e-cr-"
	descriptionPrefix = "skr-e2e-container-registry/"
	userName          = "skr-e2e"
	queryRegistries   = "map({ID,Name,SubDomainLabel,Description})"
	queryUsers        = "map({UserName,Permission})"
)

type registryItem struct {
	ID           json.RawMessage `json:"ID"`
	Name         string          `json:"Name"`
	RegistryName string          `json:"SubDomainLabel"`
	Description  string          `json:"Description"`
}

func (item registryItem) id() (string, error) {
	if len(item.ID) == 0 {
		return "", errors.New("container registry ID is missing")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(item.ID))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode container registry ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("container registry ID is empty")
		}
		return value, nil
	case json.Number:
		if value == "0" {
			return "", errors.New("container registry ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("container registry ID has unsupported JSON type %T", value)
	}
}

type userItem struct {
	UserName   string `json:"UserName"`
	Permission string `json:"Permission"`
}

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
	callRoot(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"container-registry-api"}, args...)
	return r.run(ctx, step, commandArgs...)
}

func (r cliRunner) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return r.run(ctx, step, args...)
}

func (r cliRunner) run(ctx context.Context, step string, commandArgs ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	var requestFile any
	for i := 0; i+1 < len(commandArgs); i++ {
		if commandArgs[i] != "--request" || !strings.HasPrefix(commandArgs[i+1], "@") {
			continue
		}
		path := strings.TrimPrefix(commandArgs[i+1], "@")
		content, err := os.ReadFile(path) // #nosec G304 -- The request path is created by this E2E runner in a private temporary directory.
		if err != nil {
			return nil, fmt.Errorf("%s: read request file for evidence: %w", step, err)
		}
		requestFile = evidence.RequestFileInput{Path: path, Content: string(content)}
		break
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, commandArgs...) //nolint:gosec // The caller provides a selected skr executable and structured arguments; no shell is used.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	if err := r.evidence.Record(step, r.binary, commandArgs, requestFile, stdout.String(), stderr.String(), runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type scenario struct {
	client       cli
	requestDir   string
	passwordFile string
}

func (s scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func (s scenario) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.callRoot(ctx, step, args...)
}

func (s scenario) writeRequest(name string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", name, err)
	}
	path := filepath.Join(s.requestDir, name+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write %s request: %w", name, err)
	}
	return path, nil
}

func (s scenario) list(ctx context.Context, step, requestPath string) ([]registryItem, error) {
	data, err := s.call(ctx, step, "registry", "list", "--request", "@"+requestPath, "--query", queryRegistries)
	if err != nil {
		return nil, err
	}
	var items []registryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode projected registries: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON registry array", step)
	}
	return items, nil
}

func (s scenario) find(ctx context.Context, step, requestPath, name string) ([]registryItem, error) {
	items, err := s.list(ctx, step, requestPath)
	if err != nil {
		return nil, err
	}
	matches := make([]registryItem, 0, len(items))
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func (s scenario) read(ctx context.Context, step, id string) (registryItem, error) {
	data, err := s.call(ctx, step, "registry", "read", id)
	if err != nil {
		return registryItem{}, err
	}
	var item registryItem
	if err := json.Unmarshal(data, &item); err != nil {
		return registryItem{}, fmt.Errorf("%s: decode container registry: %w", step, err)
	}
	if _, err := item.id(); err != nil {
		return registryItem{}, fmt.Errorf("%s: %w", step, err)
	}
	return item, nil
}

func (s scenario) listUsers(ctx context.Context, step, registryID string) ([]userItem, error) {
	data, err := s.call(ctx, step, "registry", "user", "list", registryID, "--query", queryUsers)
	if err != nil {
		return nil, err
	}
	var users []userItem
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("%s: decode projected users: %w", step, err)
	}
	if users == nil {
		return nil, fmt.Errorf("%s: expected a JSON user array", step)
	}
	return users, nil
}

func checkRegistry(item registryItem, id, name, originalDescription, updatedDescription string) error {
	itemID, err := item.id()
	if err != nil {
		return err
	}
	if itemID != id || item.Name != name || item.RegistryName != name ||
		(item.Description != originalDescription && item.Description != updatedDescription) {
		return fmt.Errorf("container registry identity mismatch for ID %s; refusing cleanup", id)
	}
	return nil
}

func (s scenario) cleanup(name, originalDescription, updatedDescription, knownID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	findRequest, err := s.writeRequest("cleanup-find", map[string]any{"Names": []string{name}})
	if err != nil {
		return err
	}
	matches, err := s.find(ctx, "e2e-cleanup-list", findRequest, name)
	if err != nil {
		return fmt.Errorf("cannot locate possibly created container registry: %w", err)
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 {
		return errors.New("cannot safely identify E2E container registry; manual cleanup required")
	}
	id, err := matches[0].id()
	if err != nil {
		return fmt.Errorf("cannot safely identify E2E container registry: %w", err)
	}
	if knownID != "" && knownID != id {
		return errors.New("container registry ID differs from the ID created by this run; refusing cleanup")
	}
	if err := checkRegistry(matches[0], id, name, originalDescription, updatedDescription); err != nil {
		return err
	}

	current, err := s.read(ctx, "e2e-cleanup-read", id)
	if err != nil {
		return fmt.Errorf("cannot verify container registry before cleanup: %w", err)
	}
	if err := checkRegistry(current, id, name, originalDescription, updatedDescription); err != nil {
		return err
	}

	users, err := s.listUsers(ctx, "e2e-cleanup-users", id)
	if err != nil {
		return fmt.Errorf("cannot verify users before cleanup: %w", err)
	}
	if len(users) > 1 || (len(users) == 1 && users[0].UserName != userName) {
		return errors.New("container registry contains an unexpected user; refusing cleanup")
	}
	if len(users) == 1 {
		if _, err := s.call(ctx, "e2e-cleanup-delete-user", "registry", "user", "delete", id, userName); err != nil {
			return fmt.Errorf("delete E2E container registry user: %w", err)
		}
		users, err = s.listUsers(ctx, "e2e-cleanup-users-after", id)
		if err != nil {
			return fmt.Errorf("cannot verify user deletion: %w", err)
		}
		if len(users) != 0 {
			return errors.New("E2E container registry user remains after deletion")
		}
	}

	current, err = s.read(ctx, "e2e-cleanup-read-before-delete", id)
	if err != nil {
		return fmt.Errorf("cannot recheck container registry before deletion: %w", err)
	}
	if err := checkRegistry(current, id, name, originalDescription, updatedDescription); err != nil {
		return err
	}
	if _, err := s.call(ctx, "e2e-cleanup-delete", "registry", "delete", id); err != nil {
		return fmt.Errorf("delete E2E container registry: %w", err)
	}
	remaining, err := s.find(ctx, "e2e-cleanup-list-after", findRequest, name)
	if err != nil {
		return fmt.Errorf("container registry absence could not be verified: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("E2E container registry %s remains after deletion", id)
	}
	return nil
}

func (s scenario) run(ctx context.Context, name, description string) (result error) {
	profile, err := s.callRoot(ctx, "e2e-profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	fmt.Printf("Selected SDK profile: %s", profile)
	if !strings.HasSuffix(string(profile), "\n") {
		fmt.Println()
	}

	findRequest, err := s.writeRequest("find", map[string]any{"Names": []string{name}})
	if err != nil {
		return err
	}
	before, err := s.find(ctx, "e2e-preflight-list", findRequest, name)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return fmt.Errorf("registry name %q already exists; refusing to modify it", name)
	}

	updatedDescription := description + "/updated"
	creationAttempted := false
	var id string
	defer func() {
		if !creationAttempted {
			return
		}
		if cleanupErr := s.cleanup(name, description, updatedDescription, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	createRequest, err := s.writeRequest("create", map[string]any{
		"Name":        name,
		"Description": description,
	})
	if err != nil {
		return err
	}
	creationAttempted = true
	data, err := s.call(ctx, "e2e-create", "registry", "create", "--request", "@"+createRequest)
	if err != nil {
		return err
	}
	var created registryItem
	if err := json.Unmarshal(data, &created); err != nil {
		return fmt.Errorf("decode created container registry: %w", err)
	}
	id, err = created.id()
	if err != nil {
		return err
	}
	if created.Name != name || created.RegistryName != name || created.Description != description {
		return errors.New("created container registry does not match the requested identity")
	}

	listed, err := s.find(ctx, "e2e-list-created", findRequest, name)
	if err != nil {
		return err
	}
	if len(listed) != 1 {
		return fmt.Errorf("registry list returned %d matches for %q; want one", len(listed), name)
	}
	if err := checkRegistry(listed[0], id, name, description, updatedDescription); err != nil {
		return err
	}

	updateRequest, err := s.writeRequest("update", map[string]any{"Description": updatedDescription})
	if err != nil {
		return err
	}
	data, err = s.call(ctx, "e2e-update", "registry", "update", id, "--request", "@"+updateRequest)
	if err != nil {
		return err
	}
	var updated registryItem
	if err := json.Unmarshal(data, &updated); err != nil {
		return fmt.Errorf("decode updated container registry: %w", err)
	}
	if err := checkRegistry(updated, id, name, description, updatedDescription); err != nil {
		return err
	}
	if updated.Description != updatedDescription {
		return errors.New("container registry description was not updated")
	}

	current, err := s.read(ctx, "e2e-read-updated", id)
	if err != nil {
		return err
	}
	if err := checkRegistry(current, id, name, updatedDescription, updatedDescription); err != nil {
		return err
	}

	if _, err := s.call(ctx, "e2e-add-user", "registry", "user", "add", id,
		"--user-name", userName, "--password-file", s.passwordFile, "--permission", "readwrite"); err != nil {
		return err
	}
	users, err := s.listUsers(ctx, "e2e-list-user", id)
	if err != nil {
		return err
	}
	if len(users) != 1 || users[0].UserName != userName || users[0].Permission != "readwrite" {
		return errors.New("user list does not match the user created by this run")
	}

	if _, err := s.call(ctx, "e2e-update-user", "registry", "user", "update", id, userName, "--permission", "readonly"); err != nil {
		return err
	}
	users, err = s.listUsers(ctx, "e2e-list-updated-user", id)
	if err != nil {
		return err
	}
	if len(users) != 1 || users[0].UserName != userName || users[0].Permission != "readonly" {
		return errors.New("user list does not reflect the permission update")
	}

	if _, err := s.call(ctx, "e2e-delete-user", "registry", "user", "delete", id, userName); err != nil {
		return err
	}
	users, err = s.listUsers(ctx, "e2e-list-deleted-user", id)
	if err != nil {
		return err
	}
	if len(users) != 0 {
		return errors.New("user remains after deletion")
	}
	return nil
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique registry suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-container-registry-live", false, "Confirm creating, updating, and deleting a uniquely named Container Registry and user")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/container-registry-api --skr ./skr --confirm-container-registry-live")
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
	recorder, err := evidence.New("container-registry-api")
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
	tempDir, err := os.MkdirTemp("", "skr-e2e-container-registry-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create private temporary directory:", err)
		return 1
	}
	passwordFile := filepath.Join(tempDir, "registry-password")
	defer func() {
		var cleanupErr error
		for _, filename := range []string{
			"registry-password",
			"find.json",
			"create.json",
			"update.json",
			"cleanup-find.json",
		} {
			if err := os.Remove(filepath.Join(tempDir, filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "remove private temporary file %s: %v\n", filename, err)
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
		if err := os.Remove(tempDir); err != nil {
			fmt.Fprintln(os.Stderr, "remove private temporary directory:", err)
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if cleanupErr != nil {
			exitCode = 1
		}
	}()
	passwordSuffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(passwordFile, []byte(passwordSuffix), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write private E2E password file:", err)
		return 1
	}
	suffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := registryPrefix + suffix
	description := descriptionPrefix + suffix
	fmt.Println("Unique registry to create:", name)
	fmt.Println("This scenario creates a registry and user, changes the description and permission, then deletes only those created resources.")
	fmt.Println("Check `skr config current` and the selected project before running.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	scenario := scenario{client: runner, requestDir: tempDir, passwordFile: passwordFile}
	if err := scenario.run(ctx, name, description); err != nil {
		fmt.Fprintln(os.Stderr, "Container Registry E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Container Registry E2E passed; the uniquely created registry and user were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
