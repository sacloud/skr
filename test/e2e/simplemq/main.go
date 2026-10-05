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
	queuePrefix       = "skr-e2e-sqm-"
	messageBody       = "HelloSimpleMQ"
	descriptionPrefix = "skr-e2e-simplemq/"
)

type queueItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Status      struct {
		QueueName string `json:"QueueName"`
	} `json:"Status"`
	Settings struct {
		VisibilityTimeoutSeconds int `json:"VisibilityTimeoutSeconds"`
		ExpireSeconds            int `json:"ExpireSeconds"`
	} `json:"Settings"`
}

func (item queueItem) id() (string, error) {
	if len(item.ID) == 0 {
		return "", errors.New("queue ID is missing")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(item.ID))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode queue ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("queue ID is empty")
		}
		return value, nil
	case json.Number:
		if value == "0" {
			return "", errors.New("queue ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("queue ID has unsupported JSON type %T", value)
	}
}

type messageItem struct {
	ID                  string `json:"id"`
	Content             string `json:"content"`
	VisibilityTimeoutAt int64  `json:"visibility_timeout_at"`
}

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The caller provides a selected skr executable and structured arguments; no shell is used.
	if hasTableOutput(args) {
		command.Env = append(os.Environ(), "COLUMNS=200")
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	if err := r.evidence.Record(step, args, nil, stdout.String(), stderr.String(), runErr, step == "e2e-rotate-api-key"); err != nil {
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

func hasTableOutput(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--output" && args[i+1] == "table" {
			return true
		}
	}
	return false
}

type scenario struct {
	client  cli
	keyFile string
}

func (s scenario) call(ctx context.Context, step, operation string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"simplemq-api"}, strings.Fields(operation)...)
	commandArgs = append(commandArgs, args...)
	return s.client.call(ctx, step, commandArgs...)
}

func (s scenario) list(ctx context.Context, step string) ([]queueItem, error) {
	data, err := s.call(ctx, step, "queue list", "--output", "json")
	if err != nil {
		return nil, err
	}
	var items []queueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode queue list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON queue array", step)
	}
	return items, nil
}

func (s scenario) item(ctx context.Context, step, id string) (queueItem, error) {
	data, err := s.call(ctx, step, "queue read", id, "--output", "json")
	if err != nil {
		return queueItem{}, err
	}
	var item queueItem
	if err := json.Unmarshal(data, &item); err != nil {
		return queueItem{}, fmt.Errorf("%s: decode queue: %w", step, err)
	}
	if _, err := item.id(); err != nil {
		return queueItem{}, fmt.Errorf("%s: %w", step, err)
	}
	return item, nil
}

func (s scenario) find(ctx context.Context, name string) ([]queueItem, error) {
	items, err := s.list(ctx, "e2e-cleanup-list")
	if err != nil {
		return nil, err
	}
	var matches []queueItem
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func checkQueue(item queueItem, id, name, description string) error {
	itemID, err := item.id()
	if err != nil {
		return err
	}
	if itemID != id || item.Name != name || item.Status.QueueName != name || item.Description != description {
		return fmt.Errorf("queue identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	return nil
}

func (s scenario) cleanup(name, description, knownID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	id := knownID
	if id == "" {
		matches, err := s.find(ctx, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created queue: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].Description != description {
			return errors.New("cannot safely identify E2E queue; manual cleanup required")
		}
		id, err = matches[0].id()
		if err != nil {
			return fmt.Errorf("cannot safely identify E2E queue: %w", err)
		}
	}

	current, err := s.item(ctx, "e2e-cleanup-read", id)
	if err != nil {
		return fmt.Errorf("cannot verify queue before cleanup: %w", err)
	}
	if err := checkQueue(current, id, name, description); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}

	clearErr := func() error {
		_, err := s.call(ctx, "e2e-cleanup-clear-messages", "queue clear-messages", id)
		return err
	}()
	_, deleteErr := s.call(ctx, "e2e-cleanup-delete", "queue delete", id)
	remaining, err := s.list(ctx, "e2e-cleanup-list-after")
	if err != nil {
		return errors.Join(
			wrapIfError("clear messages from E2E queue", clearErr),
			wrapIfError("delete E2E queue", deleteErr),
			fmt.Errorf("queue absence could not be verified: %w", err),
		)
	}
	remainingFound := false
	for _, item := range remaining {
		itemID, err := item.id()
		if err != nil {
			return errors.Join(
				wrapIfError("clear messages from E2E queue", clearErr),
				wrapIfError("delete E2E queue", deleteErr),
				fmt.Errorf("decode queue after cleanup: %w", err),
			)
		}
		if itemID == id || item.Name == name {
			remainingFound = true
		}
	}
	return errors.Join(
		wrapIfError("clear messages from E2E queue", clearErr),
		wrapIfError("delete E2E queue", deleteErr),
		func() error {
			if remainingFound {
				return fmt.Errorf("E2E queue %s remains after delete", id)
			}
			return nil
		}(),
	)
}

func wrapIfError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func (s scenario) run(ctx context.Context, name, description string) (result error) {
	profile, err := s.client.call(ctx, "e2e-profile-current", "config", "current")
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

	before, err := s.list(ctx, "e2e-preflight-list")
	if err != nil {
		return err
	}
	for _, item := range before {
		if item.Name == name {
			return fmt.Errorf("queue name %q already exists; refusing to modify it", name)
		}
	}

	creationAttempted := false
	var id string
	defer func() {
		if !creationAttempted {
			return
		}
		if cleanupErr := s.cleanup(name, description, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	creationAttempted = true
	data, err := s.call(ctx, "e2e-create", "queue create",
		"--name", name, "--description", description, "--output", "json")
	if err != nil {
		return err
	}
	var created queueItem
	if err := json.Unmarshal(data, &created); err != nil {
		return fmt.Errorf("decode created queue: %w", err)
	}
	id, err = created.id()
	if err != nil {
		return err
	}
	if err := checkQueue(created, id, name, description); err != nil {
		return err
	}

	listed, err := s.list(ctx, "e2e-list-created")
	if err != nil {
		return err
	}
	var found int
	for _, item := range listed {
		if item.Name == name {
			found++
			if err := checkQueue(item, id, name, description); err != nil {
				return err
			}
		}
	}
	if found != 1 {
		return fmt.Errorf("queue list returned %d items named %q; want the created queue once", found, name)
	}
	table, err := s.call(ctx, "e2e-list-created-table", "queue list", "--output", "table")
	if err != nil {
		return err
	}
	if !strings.Contains(string(table), name) {
		return fmt.Errorf("queue table output does not contain created queue %q", name)
	}

	data, err = s.call(ctx, "e2e-config", "queue config", id,
		"--visibility-timeout-seconds", "30", "--expire-seconds", "345600", "--output", "json")
	if err != nil {
		return err
	}
	var configured queueItem
	if err := json.Unmarshal(data, &configured); err != nil {
		return fmt.Errorf("decode configured queue: %w", err)
	}
	if err := checkQueue(configured, id, name, description); err != nil {
		return err
	}
	if configured.Settings.VisibilityTimeoutSeconds != 30 || configured.Settings.ExpireSeconds != 345600 {
		return fmt.Errorf("queue settings are %d/%d; want 30/345600 seconds",
			configured.Settings.VisibilityTimeoutSeconds, configured.Settings.ExpireSeconds)
	}

	keyOutput, err := s.call(ctx, "e2e-rotate-api-key", "queue rotate-api-key", id, "--output", "json")
	if err != nil {
		return err
	}
	var keyResponse struct {
		APIKey string `json:"APIKey"`
	}
	if err := json.Unmarshal(keyOutput, &keyResponse); err != nil {
		return fmt.Errorf("decode API key response: %w", err)
	}
	if keyResponse.APIKey == "" {
		return errors.New("API key response is empty")
	}
	if err := os.WriteFile(s.keyFile, []byte(keyResponse.APIKey), 0o600); err != nil {
		return fmt.Errorf("write private API key file: %w", err)
	}

	data, err = s.call(ctx, "e2e-send", "message send",
		"--queue-name", name, "--api-key-file", s.keyFile,
		"--content", messageBody, "--output", "json")
	if err != nil {
		return err
	}
	var sent messageItem
	if err := json.Unmarshal(data, &sent); err != nil {
		return fmt.Errorf("decode sent message: %w", err)
	}
	if sent.ID == "" || sent.Content != messageBody {
		return errors.New("send response did not contain the submitted message and an ID")
	}

	data, err = s.call(ctx, "e2e-receive", "message receive",
		"--queue-name", name, "--api-key-file", s.keyFile, "--output", "json")
	if err != nil {
		return err
	}
	var received []messageItem
	if err := json.Unmarshal(data, &received); err != nil {
		return fmt.Errorf("decode received messages: %w", err)
	}
	if len(received) != 1 || received[0].ID != sent.ID || received[0].Content != messageBody {
		return fmt.Errorf("receive returned %d messages; expected the sent message %s", len(received), sent.ID)
	}

	data, err = s.call(ctx, "e2e-extend-timeout", "message extend-timeout",
		"--queue-name", name, "--api-key-file", s.keyFile, received[0].ID, "--output", "json")
	if err != nil {
		return err
	}
	var extended messageItem
	if err := json.Unmarshal(data, &extended); err != nil {
		return fmt.Errorf("decode extended message: %w", err)
	}
	if extended.ID != sent.ID || extended.VisibilityTimeoutAt <= received[0].VisibilityTimeoutAt {
		return errors.New("extend-timeout did not advance the message visibility timeout")
	}

	if _, err := s.call(ctx, "e2e-delete-message", "message delete",
		"--queue-name", name, "--api-key-file", s.keyFile, sent.ID); err != nil {
		return err
	}
	data, err = s.call(ctx, "e2e-count-messages", "queue count-messages", id, "--output", "json")
	if err != nil {
		return err
	}
	var count struct {
		Count int `json:"Count"`
	}
	if err := json.Unmarshal(data, &count); err != nil {
		return fmt.Errorf("decode message count: %w", err)
	}
	if count.Count != 0 {
		return fmt.Errorf("queue has %d messages after deleting the test message; want 0", count.Count)
	}
	return nil
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique queue suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-simplemq-live", false, "Confirm creating, configuring, sending/receiving a test message, and deleting a uniquely named SimpleMQ queue")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/simplemq --skr ./skr --confirm-simplemq-live")
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
	recorder, err := evidence.New("simplemq-api")
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
	secretDir, err := os.MkdirTemp("", "skr-e2e-simplemq-secret-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create private key directory:", err)
		return 1
	}
	keyFile := filepath.Join(secretDir, "simplemq.key")
	defer func() {
		var cleanupErr error
		if err := os.Remove(keyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove private API key file:", err)
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if err := os.Remove(secretDir); err != nil {
			fmt.Fprintln(os.Stderr, "remove private API key directory:", err)
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if cleanupErr != nil {
			exitCode = 1
		}
	}()

	suffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := queuePrefix + suffix
	description := descriptionPrefix + suffix
	fmt.Println("Unique queue to create:", name)
	fmt.Println("Check `skr config current` before running; this command uses the selected SDK profile.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	if err := (scenario{client: runner, keyFile: keyFile}).run(ctx, name, description); err != nil {
		fmt.Fprintln(os.Stderr, "SimpleMQ E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("SimpleMQ E2E passed; the test message and uniquely created queue were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
