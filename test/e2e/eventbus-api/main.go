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
	zone                = "is1b"
	resourceNamePrefix  = "skr-e2e-eventbus-"
	resourceDescription = "skr-e2e-eventbus/"
	eventSource         = "//eventbus.sakura.ad.jp/eventlog"
	switchCreatedType   = "jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created"
	eventWaitTimeout    = 5 * time.Minute
	eventPollInterval   = 3 * time.Second
	triggerSettleDelay  = 60 * time.Second
	commandTimeout      = 60 * time.Second
	cleanupTimeout      = 3 * time.Minute
)

type item struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Status      struct {
		QueueName string `json:"QueueName"`
	} `json:"Status"`
}

func (i item) id() (string, error) {
	if len(i.ID) == 0 {
		return "", errors.New("resource ID is missing")
	}
	decoder := json.NewDecoder(bytes.NewReader(i.ID))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode resource ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("resource ID is empty")
		}
		return value, nil
	case json.Number:
		if value == "0" {
			return "", errors.New("resource ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("resource ID has unsupported JSON type %T", value)
	}
}

type message struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
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

	if err := r.evidence.Record(step, args, nil, stdout.String(), stderr.String(), runErr, step == "rotate-api-key"); err != nil {
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
	client      cli
	keyFile     string
	secretFile  string
	queue       resource
	config      resource
	trigger     resource
	sw          resource
	expected    string
	settleDelay time.Duration
}

func (s *scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func decodeItem(step string, data []byte) (item, error) {
	var result item
	if err := json.Unmarshal(data, &result); err != nil {
		return item{}, fmt.Errorf("%s: decode resource: %w", step, err)
	}
	if _, err := result.id(); err != nil {
		return item{}, fmt.Errorf("%s: %w", step, err)
	}
	return result, nil
}

func (s *scenario) list(ctx context.Context, step, kind string) ([]item, error) {
	var args []string
	switch kind {
	case "queue":
		args = []string{"simplemq-api", "queue", "list", "--output", "json"}
	case "process-configuration", "trigger":
		args = []string{"eventbus-api", kind, "list", "--output", "json"}
	case "switch":
		request, err := json.Marshal(map[string]any{"Zone": zone})
		if err != nil {
			return nil, fmt.Errorf("%s: encode Switch search: %w", step, err)
		}
		args = []string{"iaas-api", "switch", "find", "--request", string(request), "--output", "json"}
	default:
		return nil, fmt.Errorf("%s: unknown resource kind %q", step, kind)
	}
	data, err := s.call(ctx, step, args...)
	if err != nil {
		return nil, err
	}
	var items []item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode resource list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s *scenario) findByName(ctx context.Context, step string, resource resource) (item, bool, error) {
	items, err := s.list(ctx, step, resource.kind)
	if err != nil {
		return item{}, false, err
	}
	var matches []item
	for _, candidate := range items {
		if candidate.Name == resource.name {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return item{}, false, nil
	}
	if len(matches) != 1 || matches[0].Description != resource.description {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: resource identity is not unique and marked for this E2E", step, resource.name)
	}
	return matches[0], true, nil
}

func checkIdentity(candidate item, resource resource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != resource.name || candidate.Description != resource.description {
		return fmt.Errorf("%s identity mismatch for ID %s", resource.kind, id)
	}
	if resource.kind == "queue" && candidate.Status.QueueName != resource.name {
		return fmt.Errorf("SimpleMQ queue name mismatch for ID %s", id)
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
	for _, resource := range []resource{s.queue, s.config, s.trigger, s.sw} {
		items, err := s.list(ctx, "preflight-"+resource.kind+"s", resource.kind)
		if err != nil {
			return err
		}
		for _, candidate := range items {
			if candidate.Name == resource.name {
				return fmt.Errorf("%s name %q already exists; refusing to modify it", resource.kind, resource.name)
			}
		}
	}
	return nil
}

func (s *scenario) createQueue(ctx context.Context) error {
	s.queue.attempted = true
	data, err := s.call(ctx, "create-queue", "simplemq-api", "queue", "create",
		"--name", s.queue.name, "--description", s.queue.description, "--output", "json")
	if err != nil {
		return err
	}
	created, err := decodeItem("create-queue", data)
	if err != nil {
		return err
	}
	id, err := created.id()
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.queue, id); err != nil {
		return err
	}
	s.queue.id = id
	return err
}

func (s *scenario) rotateAPIKey(ctx context.Context) (string, error) {
	data, err := s.call(ctx, "rotate-api-key", "simplemq-api", "queue", "rotate-api-key", s.queue.id, "--output", "json")
	if err != nil {
		return "", err
	}
	var response struct {
		APIKey string `json:"APIKey"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("rotate-api-key: decode API key: %w", err)
	}
	if response.APIKey == "" {
		return "", errors.New("rotate-api-key: API key is empty")
	}
	return response.APIKey, nil
}

func (s *scenario) createConfiguration(ctx context.Context) error {
	parameters, err := json.Marshal(struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}{QueueName: s.queue.name, Content: s.expected})
	if err != nil {
		return fmt.Errorf("encode SimpleMQ parameters: %w", err)
	}
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        s.config.name,
			"Description": s.config.description,
			"Settings": map[string]string{
				"Destination": "simplemq",
				"Parameters":  string(parameters),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("encode process configuration: %w", err)
	}
	s.config.attempted = true
	data, err := s.call(ctx, "create-process-configuration", "eventbus-api", "process-configuration", "create",
		"--request", string(request), "--output", "json")
	if err != nil {
		return err
	}
	created, err := decodeItem("create-process-configuration", data)
	if err != nil {
		return err
	}
	id, err := created.id()
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.config, id); err != nil {
		return err
	}
	s.config.id = id
	return nil
}

func (s *scenario) setConfigurationSecret(ctx context.Context) error {
	_, err := s.call(ctx, "set-process-configuration-secret", "eventbus-api", "process-configuration",
		"update-secret", s.config.id, "--secret-file", s.secretFile)
	if err != nil {
		return err
	}
	return nil
}

func (s *scenario) createTrigger(ctx context.Context) error {
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        s.trigger.name,
			"Description": s.trigger.description,
			"Settings": map[string]any{
				"Source": eventSource,
				"Types":  []string{switchCreatedType},
				"Conditions": []map[string]any{
					{"Key": "zone", "Op": "eq", "Values": []string{zone}},
				},
				"ProcessConfigurationID": s.config.id,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("encode EventBus trigger: %w", err)
	}
	s.trigger.attempted = true
	data, err := s.call(ctx, "create-trigger", "eventbus-api", "trigger", "create",
		"--request", string(request), "--output", "json")
	if err != nil {
		return err
	}
	created, err := decodeItem("create-trigger", data)
	if err != nil {
		return err
	}
	id, err := created.id()
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.trigger, id); err != nil {
		return err
	}
	s.trigger.id = id
	return nil
}

func (s *scenario) createSwitch(ctx context.Context) error {
	s.sw.attempted = true
	data, err := s.call(ctx, "create-switch", "iaas-api", "switch", "create",
		"--zone", zone, "--name", s.sw.name, "--description", s.sw.description, "--output", "json")
	if err != nil {
		return err
	}
	created, err := decodeItem("create-switch", data)
	if err != nil {
		return err
	}
	id, err := created.id()
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.sw, id); err != nil {
		return err
	}
	s.sw.id = id
	return nil
}

func (s *scenario) waitForMessage(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, eventWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(eventPollInterval)
	defer ticker.Stop()
	for attempt := 0; ; attempt++ {
		step := fmt.Sprintf("receive-event-message-%03d", attempt)
		data, err := s.call(pollCtx, step, "simplemq-api", "message", "receive",
			"--queue-name", s.queue.name, "--api-key-file", s.keyFile, "--output", "json")
		if err != nil {
			if pollCtx.Err() != nil {
				return fmt.Errorf("no matching EventBus message received within %s: %w", eventWaitTimeout, pollCtx.Err())
			}
			return err
		}
		var messages []message
		if err := json.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("%s: decode received messages: %w", step, err)
		}
		for _, received := range messages {
			if received.Content == s.expected {
				if received.ID == "" {
					return errors.New("received EventBus message has no ID")
				}
				return nil
			}
		}
		select {
		case <-pollCtx.Done():
			return fmt.Errorf("no matching EventBus message received within %s: %w", eventWaitTimeout, pollCtx.Err())
		case <-ticker.C:
		}
	}
}

func (s *scenario) read(ctx context.Context, step string, resource resource) (item, error) {
	var args []string
	switch resource.kind {
	case "queue":
		args = []string{"simplemq-api", "queue", "read", resource.id, "--output", "json"}
	case "process-configuration", "trigger":
		args = []string{"eventbus-api", resource.kind, "read", resource.id, "--output", "json"}
	case "switch":
		args = []string{"iaas-api", "switch", "read", "--zone", zone, "--id", resource.id, "--output", "json"}
	default:
		return item{}, fmt.Errorf("%s: unknown resource kind %q", step, resource.kind)
	}
	data, err := s.call(ctx, step, args...)
	if err != nil {
		return item{}, err
	}
	return decodeItem(step, data)
}

func (s *scenario) delete(ctx context.Context, step string, resource resource) error {
	var args []string
	switch resource.kind {
	case "queue":
		args = []string{"simplemq-api", "queue", "delete", resource.id}
	case "process-configuration", "trigger":
		args = []string{"eventbus-api", resource.kind, "delete", resource.id}
	case "switch":
		args = []string{"iaas-api", "switch", "delete", "--zone", zone, "--id", resource.id, "--fail-if-not-found=true"}
	default:
		return fmt.Errorf("%s: unknown resource kind %q", step, resource.kind)
	}
	_, err := s.call(ctx, step, args...)
	return err
}

func (s *scenario) cleanupResource(ctx context.Context, resource *resource) error {
	if !resource.attempted {
		return nil
	}
	id := resource.id
	if id == "" {
		found, ok, err := s.findByName(ctx, "cleanup-find-"+resource.kind, *resource)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		id, err = found.id()
		if err != nil {
			return err
		}
	}
	current, err := s.read(ctx, "cleanup-read-"+resource.kind, resourceWithID(*resource, id))
	if err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", resource.kind, err)
	}
	if err := checkIdentity(current, *resource, id); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	if resource.kind == "queue" {
		if _, err := s.call(ctx, "cleanup-clear-queue", "simplemq-api", "queue", "clear-messages", id); err != nil {
			return fmt.Errorf("clear E2E queue messages: %w", err)
		}
	}
	if err := s.delete(ctx, "cleanup-delete-"+resource.kind, resourceWithID(*resource, id)); err != nil {
		return fmt.Errorf("delete E2E %s: %w", resource.kind, err)
	}
	remaining, ok, err := s.findByName(ctx, "cleanup-verify-"+resource.kind, *resource)
	if err != nil {
		return fmt.Errorf("verify E2E %s deletion: %w", resource.kind, err)
	}
	if ok {
		remainingID, idErr := remaining.id()
		if idErr != nil {
			return idErr
		}
		return fmt.Errorf("E2E %s %s remains after delete", resource.kind, remainingID)
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
	for _, resource := range []*resource{&s.trigger, &s.config, &s.sw, &s.queue} {
		if err := s.cleanupResource(ctx, resource); err != nil {
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

	if err := s.createQueue(ctx); err != nil {
		return err
	}
	apiKey, err := s.rotateAPIKey(ctx)
	if err != nil {
		return err
	}
	//nolint:gosec // The API key is written only to a private, mode-0600 temporary file.
	keyFileContents, err := json.Marshal(struct {
		APIKey string `json:"APIKey"`
	}{APIKey: apiKey})
	if err != nil {
		return fmt.Errorf("encode private SimpleMQ API key file: %w", err)
	}
	if err := os.WriteFile(s.secretFile, keyFileContents, 0o600); err != nil {
		return fmt.Errorf("write private EventBus secret file: %w", err)
	}
	if err := os.WriteFile(s.keyFile, []byte(apiKey), 0o600); err != nil {
		return fmt.Errorf("write private SimpleMQ API key file: %w", err)
	}
	if err := s.createConfiguration(ctx); err != nil {
		return err
	}
	if err := s.setConfigurationSecret(ctx); err != nil {
		return err
	}
	if err := s.createTrigger(ctx); err != nil {
		return err
	}
	trigger, err := s.read(ctx, "verify-trigger", s.trigger)
	if err != nil {
		return fmt.Errorf("verify EventBus trigger before creating the Switch: %w", err)
	}
	if err := checkIdentity(trigger, s.trigger, s.trigger.id); err != nil {
		return fmt.Errorf("verify EventBus trigger before creating the Switch: %w", err)
	}
	if s.settleDelay > 0 {
		fmt.Printf("Waiting %s for the trigger to settle before creating the Switch.\n", s.settleDelay)
		timer := time.NewTimer(s.settleDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for EventBus trigger: %w", ctx.Err())
		case <-timer.C:
		}
	}
	if err := s.createSwitch(ctx); err != nil {
		return err
	}
	fmt.Println("Waiting for the Switch-created event to arrive in SimpleMQ.")
	return s.waitForMessage(ctx)
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique E2E resource suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating and deleting EventBus/SimpleMQ resources and a test Switch in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/eventbus-api --skr ./skr --confirm-is1b-live")
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
	recorder, err := evidence.New("eventbus-api")
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
	secretDir, err := os.MkdirTemp("", "skr-e2e-eventbus-secret-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create private API key directory:", err)
		return 1
	}
	keyFile := filepath.Join(secretDir, "simplemq.key")
	secretFile := filepath.Join(secretDir, "eventbus-secret.json")
	defer func() {
		if err := os.Remove(keyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove private API key file:", err)
			exitCode = 1
		}
		if err := os.Remove(secretFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove private EventBus secret file:", err)
			exitCode = 1
		}
		if err := os.Remove(secretDir); err != nil {
			fmt.Fprintln(os.Stderr, "remove private API key directory:", err)
			exitCode = 1
		}
	}()
	suffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	testMessage := "SKRE2ESWITCHCREATED" + strings.ToUpper(suffix)
	fmt.Println("SimpleMQ queue and EventBus resources:", name)
	fmt.Println("Test Switch zone:", zone)
	fmt.Println("Check `skr config current` before running; this command uses the selected SDK profile.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := &cliRunner{binary: path, evidence: recorder}
	s := &scenario{
		client:      runner,
		keyFile:     keyFile,
		secretFile:  secretFile,
		queue:       resource{kind: "queue", name: name, description: desc},
		config:      resource{kind: "process-configuration", name: name, description: desc},
		trigger:     resource{kind: "trigger", name: name, description: desc},
		sw:          resource{kind: "switch", name: name, description: desc},
		expected:    testMessage,
		settleDelay: triggerSettleDelay,
	}
	if err := s.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "EventBus E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("EventBus E2E passed; the test Switch, EventBus resources, and SimpleMQ queue were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
