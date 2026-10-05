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

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

const (
	zone           = "tk1v"
	name           = "skr-e2e-switch"
	prefix         = "skr-e2e-"
	description    = "Temporary switch for skr API tutorial"
	pageSize       = 100
	maxSwitchPages = 100
)

type switchItem struct {
	ID          types.ID
	Name        string
	Description string
}

type cliRunner struct {
	binary   string
	evidence string
}

func operation(step string) (string, error) {
	for _, candidate := range []string{"find", "read", "create", "update", "delete"} {
		if strings.Contains("-"+step+"-", "-"+candidate+"-") {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unknown E2E step %q", step)
}

func (r cliRunner) call(ctx context.Context, step string, request any) ([]byte, error) {
	var args []string
	if request == nil {
		args = []string{"config", "current"}
	} else {
		var err error
		args, err = switchArgs(step, request)
		if err != nil {
			return nil, err
		}
	}

	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // Binary is an explicitly selected local skr executable; arguments are structured JSON, not a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	record := struct {
		Step    string   `json:"step"`
		Args    []string `json:"args"`
		Request any      `json:"request,omitempty"`
		Stdout  string   `json:"stdout"`
		Stderr  string   `json:"stderr"`
		Error   string   `json:"error,omitempty"`
	}{Step: step, Args: args, Request: request, Stdout: stdout.String(), Stderr: stderr.String()}
	if runErr != nil {
		record.Error = runErr.Error()
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%s: encode evidence: %w", step, err)
	}
	if err := os.WriteFile(filepath.Join(r.evidence, step+".json"), append(data, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("%s: write evidence: %w", step, err)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: CLI failed: %w (see evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

func switchArgs(step string, value any) ([]string, error) {
	op, err := operation(step)
	if err != nil {
		return nil, err
	}
	request, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected Switch request map", step)
	}
	zone, ok := request["Zone"].(string)
	if !ok || zone == "" {
		return nil, fmt.Errorf("%s: missing Zone", step)
	}
	args := []string{"iaas-api", "switch", op, "--zone", zone}
	fields := map[string]string{"ID": "--id", "Name": "--name", "Description": "--description", "Count": "--count", "From": "--from", "FailIfNotFound": "--fail-if-not-found"}
	for field := range request {
		if field != "Zone" && field != "Names" && fields[field] == "" {
			return nil, fmt.Errorf("%s: unsupported request field %s", step, field)
		}
	}
	for _, field := range []string{"ID", "Name", "Description", "Count", "From", "FailIfNotFound"} {
		value, exists := request[field]
		if !exists {
			continue
		}
		if field == "FailIfNotFound" {
			args = append(args, fmt.Sprintf("%s=%v", fields[field], value))
			continue
		}
		args = append(args, fields[field], fmt.Sprint(value))
	}
	if _, exists := request["Names"]; exists {
		if op != "find" {
			return nil, fmt.Errorf("%s: Names only supported by find", step)
		}
		// Names is a slice in the SDK request; keep the JSON path for complex inputs.
		data, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("%s: encode request: %w", step, err)
		}
		return []string{"iaas-api", "switch", op, "--request", string(data)}, nil
	}
	return args, nil
}

type scenario struct {
	call func(context.Context, string, any) ([]byte, error)
}

func (s scenario) item(ctx context.Context, step string, request any) (switchItem, error) {
	data, err := s.call(ctx, step, request)
	if err != nil {
		return switchItem{}, err
	}
	var item switchItem
	if err := json.Unmarshal(data, &item); err != nil {
		return switchItem{}, fmt.Errorf("%s: decode Switch: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return switchItem{}, fmt.Errorf("%s: Switch ID is empty", step)
	}
	return item, nil
}

func (s scenario) find(ctx context.Context, step, name string) ([]switchItem, error) {
	data, err := s.call(ctx, step, map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return nil, err
	}
	var items []switchItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Switch list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	var matches []switchItem
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func checkItem(item switchItem, id types.ID, name string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("Switch identity mismatch for ID %s (expected name %q and test description)", id, name)
	}
	return nil
}

func (s scenario) list(ctx context.Context, phase string) ([]switchItem, error) {
	var all []switchItem
	seen := make(map[types.ID]bool)
	for page := range maxSwitchPages {
		step := fmt.Sprintf("%s-find-page-%03d", phase, page)
		data, err := s.call(ctx, step, map[string]any{"Zone": zone, "Count": pageSize, "From": page * pageSize})
		if err != nil {
			return nil, err
		}
		var items []switchItem
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("%s: decode Switch list: %w", step, err)
		}
		if items == nil || len(items) > pageSize {
			return nil, fmt.Errorf("%s: unexpected list size", step)
		}
		for _, item := range items {
			if item.ID.IsEmpty() || seen[item.ID] {
				return nil, fmt.Errorf("%s: missing or duplicate Switch ID in paginated list", step)
			}
			seen[item.ID] = true
		}
		all = append(all, items...)
		if len(items) < pageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("Switch list exceeded %d pages; refusing partial cleanup", maxSwitchPages)
}

func (s scenario) removePriorTests(ctx context.Context) error {
	items, err := s.list(ctx, "preflight")
	if err != nil {
		return err
	}
	var matches []switchItem
	for _, item := range items {
		if strings.HasPrefix(item.Name, prefix) {
			if item.Description != description {
				return fmt.Errorf("refusing to delete %q: description differs from E2E marker", item.Name)
			}
			matches = append(matches, item)
		}
	}
	for i, match := range matches {
		request := map[string]any{"Zone": zone, "ID": match.ID}
		current, err := s.item(ctx, fmt.Sprintf("preflight-read-%03d", i), request)
		if err != nil {
			return err
		}
		if err := checkItem(current, match.ID, match.Name); err != nil {
			return fmt.Errorf("preflight cleanup refused: %w", err)
		}
		if _, err := s.call(ctx, fmt.Sprintf("preflight-delete-%03d", i),
			map[string]any{"Zone": zone, "ID": match.ID, "FailIfNotFound": true}); err != nil {
			return fmt.Errorf("preflight delete Switch %s: %w", match.ID, err)
		}
	}
	after, err := s.list(ctx, "preflight-after")
	if err != nil {
		return err
	}
	for _, item := range after {
		if strings.HasPrefix(item.Name, prefix) {
			return fmt.Errorf("preflight cleanup incomplete: Switch %s remains", item.ID)
		}
	}
	return nil
}

func (s scenario) cleanup(name, updatedName string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var id = knownID
	if id.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find-original", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Switch: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != description {
			return errors.New("cannot safely identify created Switch; manual cleanup required")
		}
		id = matches[0].ID
	}
	request := map[string]any{"Zone": zone, "ID": id}
	current, err := s.item(ctx, "cleanup-read", request)
	if err != nil {
		return fmt.Errorf("cannot verify Switch before cleanup: %w", err)
	}
	if err := checkItem(current, id, name); err != nil {
		if err := checkItem(current, id, updatedName); err != nil {
			return fmt.Errorf("cleanup refused: Switch identity changed; manual cleanup required: %w", err)
		}
	}
	_, err = s.call(ctx, "cleanup-delete", map[string]any{"Zone": zone, "ID": id, "FailIfNotFound": true})
	if err != nil {
		return fmt.Errorf("delete test Switch %s: %w", id, err)
	}
	for _, entry := range []struct{ step, name string }{
		{"cleanup-find-original-after", name},
		{"cleanup-find-updated-after", updatedName},
	} {
		matches, err := s.find(ctx, entry.step, entry.name)
		if err != nil {
			return err
		}
		for _, item := range matches {
			if item.ID == id {
				return fmt.Errorf("Switch %s remains after delete", id)
			}
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	updatedName := name + "-updated"
	if _, err := s.call(ctx, "profile-current", nil); err != nil {
		return fmt.Errorf("selected profile required: %w", err)
	}
	if err := s.removePriorTests(ctx); err != nil {
		return fmt.Errorf("preflight cleanup failed: %w", err)
	}
	before, err := s.find(ctx, "before-find", name)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return fmt.Errorf("test Switch name %q already exists; refusing to modify it", name)
	}
	beforeUpdated, err := s.find(ctx, "before-find-updated", updatedName)
	if err != nil {
		return err
	}
	if len(beforeUpdated) != 0 {
		return fmt.Errorf("test Switch name %q already exists; refusing to modify it", updatedName)
	}

	var id types.ID
	defer func() {
		if cleanupErr := s.cleanup(name, updatedName, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()
	created, err := s.item(ctx, "test-create", map[string]any{"Zone": zone, "Name": name, "Description": description})
	if err != nil {
		return err
	}
	id = created.ID
	if err := checkItem(created, id, name); err != nil {
		return err
	}
	found, err := s.find(ctx, "test-find", name)
	if err != nil {
		return err
	}
	if len(found) != 1 || checkItem(found[0], id, name) != nil {
		return fmt.Errorf("find did not return exactly the created Switch %s", id)
	}
	request := map[string]any{"Zone": zone, "ID": id}
	read, err := s.item(ctx, "test-read", request)
	if err != nil {
		return err
	}
	if err := checkItem(read, id, name); err != nil {
		return err
	}
	updated, err := s.item(ctx, "test-update", map[string]any{"Zone": zone, "ID": id, "Name": updatedName})
	if err != nil {
		return err
	}
	if err := checkItem(updated, id, updatedName); err != nil {
		return err
	}
	read, err = s.item(ctx, "test-read-updated", request)
	if err != nil {
		return err
	}
	return checkItem(read, id, updatedName)
}

func evidenceDir() (string, error) {
	dir, err := os.MkdirTemp("", "skr-e2e-switch-")
	if err != nil {
		return "", fmt.Errorf("create evidence directory: %w", err)
	}
	command := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree") //nolint:gosec // The directory was just created by MkdirTemp and is not user input.
	output, err := command.Output()
	if err == nil && strings.TrimSpace(string(output)) == "true" {
		_ = os.Remove(dir)
		return "", fmt.Errorf("refusing to save evidence inside a Git worktree: %s", dir)
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return "", fmt.Errorf("verify evidence location: %w", err)
	}
	return dir, nil
}

func runMain() int {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-tk1v", false, "Confirm deleting prior skr-e2e- Switches and creating/updating/deleting a test Switch in tk1v")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/switch --skr ./skr --confirm-tk1v")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	dir, err := evidenceDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, outside Git):", dir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: dir}
	if err := (scenario{call: runner.call}).run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "Switch E2E failed:", err)
		return 1
	}
	fmt.Println("Switch E2E passed; test Switch deleted and absence verified.")
	return 0
}

func main() {
	os.Exit(runMain())
}
