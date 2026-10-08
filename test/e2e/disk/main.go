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

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	zone        = "is1b"
	namePrefix  = "skr-e2e-disk-"
	description = "Temporary Disk for skr API E2E"
	diskPlanID  = 4
	diskSizeGB  = 20
	diskSizeMB  = diskSizeGB * 1024
)

type diskItem struct {
	ID          types.ID `json:"ID"`
	Name        string   `json:"Name"`
	Description string   `json:"Description"`
	SizeMB      int      `json:"SizeMB"`
}

type cli interface {
	call(context.Context, string, any) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, request any) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	args, err := diskArgs(step, request)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The caller selects a local skr binary; structured arguments are not passed through a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, request, stdout.String(), stderr.String(), runErr, false); err != nil {
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

func diskArgs(step string, value any) ([]string, error) {
	if step == "profile-current" {
		return []string{"config", "current"}, nil
	}
	request, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected Disk request map", step)
	}
	requestZone, ok := request["Zone"].(string)
	if !ok || requestZone != zone {
		return nil, fmt.Errorf("%s: expected Zone %q", step, zone)
	}
	var queryArgs []string
	if strings.HasSuffix(step, "-query") {
		queryArgs = []string{"--query", "map({ID,Name})"}
	}
	op := ""
	switch {
	case strings.Contains("-"+step+"-", "-find-"):
		op = "find"
	case strings.Contains("-"+step+"-", "-read-"):
		op = "read"
	case strings.Contains("-"+step+"-", "-create-"):
		op = "create"
	case strings.Contains("-"+step+"-", "-update-"):
		op = "update"
	case strings.Contains("-"+step+"-", "-delete-"):
		op = "delete"
	default:
		return nil, fmt.Errorf("unknown Disk E2E step %q", step)
	}
	args := []string{"iaas-api", "disk", op}
	switch op {
	case "find", "create", "update":
		data, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("%s: encode request: %w", step, err)
		}
		args = append(args, "--request", string(data))
	case "read":
		id, err := requestID(request)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step, err)
		}
		args = append(args, "--zone", zone, "--id", id)
	case "delete":
		id, err := requestID(request)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step, err)
		}
		args = append(args, "--zone", zone, "--id", id, "--fail-if-not-found")
	}
	return append(args, queryArgs...), nil
}

func requestID(request map[string]any) (string, error) {
	id, ok := request["ID"]
	if !ok {
		return "", errors.New("missing ID")
	}
	value := fmt.Sprint(id)
	if value == "" || value == "0" {
		return "", errors.New("invalid ID")
	}
	return value, nil
}

type scenario struct {
	client cli
}

func (s scenario) call(ctx context.Context, step string, request any) ([]byte, error) {
	return s.client.call(ctx, step, request)
}

func (s scenario) find(ctx context.Context, step, name string) ([]diskItem, error) {
	data, err := s.call(ctx, step, map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return nil, err
	}
	var items []diskItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Disk list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	var matches []diskItem
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func (s scenario) item(ctx context.Context, step string, request map[string]any) (diskItem, error) {
	data, err := s.call(ctx, step, request)
	if err != nil {
		return diskItem{}, err
	}
	var item diskItem
	if err := json.Unmarshal(data, &item); err != nil {
		return diskItem{}, fmt.Errorf("%s: decode Disk: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return diskItem{}, fmt.Errorf("%s: Disk ID is empty", step)
	}
	return item, nil
}

func checkDisk(item diskItem, id types.ID, name string, checkSize bool) error {
	if item.ID != id || item.Name != name || item.Description != description ||
		(checkSize && item.SizeMB != diskSizeMB) {
		return fmt.Errorf("Disk identity mismatch for ID %s (expected name %q, E2E description, and 20 GB size)", id, name)
	}
	return nil
}

func (s scenario) cleanup(name, updatedName string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	id := knownID
	if id.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find-original", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Disk: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != description {
			return errors.New("cannot safely identify created Disk; manual cleanup required")
		}
		id = matches[0].ID
	}
	current, err := s.item(ctx, "cleanup-read", map[string]any{"Zone": zone, "ID": id})
	if err != nil {
		return fmt.Errorf("cannot verify Disk before cleanup: %w", err)
	}
	if err := checkDisk(current, id, name, true); err != nil {
		if updatedErr := checkDisk(current, id, updatedName, true); updatedErr != nil {
			return fmt.Errorf("cleanup refused: Disk identity changed; manual cleanup required: %w", err)
		}
	}
	if _, err := s.call(ctx, "cleanup-delete", map[string]any{"Zone": zone, "ID": id}); err != nil {
		return fmt.Errorf("delete test Disk %s: %w", id, err)
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
				return fmt.Errorf("Disk %s remains after delete", id)
			}
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	if _, err := s.call(ctx, "profile-current", nil); err != nil {
		return fmt.Errorf("selected SDK profile required: %w", err)
	}
	for _, candidate := range []struct{ step, name string }{
		{"preflight-find-original", name},
		{"preflight-find-updated", name + "-updated"},
	} {
		matches, err := s.find(ctx, candidate.step, candidate.name)
		if err != nil {
			return err
		}
		if len(matches) != 0 {
			return fmt.Errorf("test Disk name %q already exists; refusing to modify it", candidate.name)
		}
	}

	updatedName := name + "-updated"
	var id types.ID
	defer func() {
		if cleanupErr := s.cleanup(name, updatedName, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	created, err := s.item(ctx, "test-create", map[string]any{
		"Zone": zone, "Name": name, "Description": description,
		"DiskPlanID": diskPlanID, "Connection": "virtio", "SizeGB": diskSizeGB,
	})
	if err != nil {
		return err
	}
	id = created.ID
	if err := checkDisk(created, id, name, true); err != nil {
		return err
	}
	found, err := s.find(ctx, "test-find", name)
	if err != nil {
		return err
	}
	if len(found) != 1 || checkDisk(found[0], id, name, true) != nil {
		return fmt.Errorf("find did not return exactly the created Disk %s", id)
	}
	output, err := s.call(ctx, "test-find-query", map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return err
	}
	var projected []struct {
		ID   types.ID
		Name string
	}
	if err := json.Unmarshal(output, &projected); err != nil {
		return fmt.Errorf("decode projected Disk list: %w", err)
	}
	if len(projected) != 1 || projected[0].ID != id || projected[0].Name != name {
		return fmt.Errorf("projected Disk list does not match created Disk %s", id)
	}
	request := map[string]any{"Zone": zone, "ID": id}
	read, err := s.item(ctx, "test-read", request)
	if err != nil {
		return err
	}
	if err := checkDisk(read, id, name, true); err != nil {
		return err
	}
	updated, err := s.item(ctx, "test-update", map[string]any{"Zone": zone, "ID": id, "Name": updatedName})
	if err != nil {
		return err
	}
	if err := checkDisk(updated, id, updatedName, true); err != nil {
		return err
	}
	read, err = s.item(ctx, "test-read-updated", request)
	if err != nil {
		return err
	}
	if err := checkDisk(read, id, updatedName, true); err != nil {
		return err
	}
	if _, err := s.call(ctx, "test-delete", map[string]any{"Zone": zone, "ID": id}); err != nil {
		return err
	}
	id = 0
	for _, entry := range []struct{ step, name string }{
		{"test-find-original-after", name},
		{"test-find-updated-after", updatedName},
	} {
		matches, err := s.find(ctx, entry.step, entry.name)
		if err != nil {
			return err
		}
		if len(matches) != 0 {
			return fmt.Errorf("Disk %q remains after delete", entry.name)
		}
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating, updating, and deleting a 20 GB SSD test Disk in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/disk --skr ./skr --confirm-is1b-live")
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
	recorder, err := evidence.New("disk-api")
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
	suffixBytes := make([]byte, 6)
	if _, err := rand.Read(suffixBytes); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique E2E Disk name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffixBytes)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}}).run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "Disk E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Disk E2E passed; the test Disk was deleted and absence verified.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
