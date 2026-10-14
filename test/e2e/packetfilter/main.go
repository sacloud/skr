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
	namePrefix  = "skr-e2e-packetfilter-"
	description = "Temporary PacketFilter for skr API E2E"
)

type packetFilterExpression struct {
	Protocol        string
	SourceNetwork   string
	SourcePort      string
	DestinationPort string
	Action          string
	Description     string
}

type packetFilterItem struct {
	ID             types.ID
	Name           string
	Description    string
	Expression     []packetFilterExpression
	ExpressionHash string
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
	args, err := packetFilterArgs(step, request)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The caller selects a local skr binary; arguments are structured, not passed through a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, request, stdout.String(), stderr.String(), runErr, request == nil); err != nil {
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

func packetFilterArgs(step string, value any) ([]string, error) {
	if value == nil {
		return []string{"config", "current"}, nil
	}
	request, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected PacketFilter request map", step)
	}
	requestZone, ok := request["Zone"].(string)
	if !ok || requestZone != zone {
		return nil, fmt.Errorf("%s: expected Zone %q", step, zone)
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
		return nil, fmt.Errorf("unknown PacketFilter E2E step %q", step)
	}
	args := []string{"iaas-api", "packetfilter", op}

	if strings.HasSuffix(step, "-query") {
		args = append(args, "--query", "map({ID,Name})")
	}
	switch op {
	case "find":
		data, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("%s: encode find request: %w", step, err)
		}
		args = append(args, "--request", string(data))
	case "create":
		name, err := requestString(step, request, "Name")
		if err != nil {
			return nil, err
		}
		description, err := requestString(step, request, "Description")
		if err != nil {
			return nil, err
		}
		expressions, err := requestExpressions(step, request)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(expressions)
		if err != nil {
			return nil, fmt.Errorf("%s: encode expressions: %w", step, err)
		}
		args = append(args, "--zone", zone, "--name", name, "--description", description, "--expressions", string(data))
	case "read", "delete":
		id, err := requestID(step, request)
		if err != nil {
			return nil, err
		}
		args = append(args, "--zone", zone, "--id", id)
		if op == "delete" {
			args = append(args, "--fail-if-not-found")
		}
	case "update":
		id, err := requestID(step, request)
		if err != nil {
			return nil, err
		}
		args = append(args, "--zone", zone, "--id", id)
		if name, exists := request["Name"]; exists {
			text, ok := name.(string)
			if !ok {
				return nil, fmt.Errorf("%s: Name must be a string", step)
			}
			args = append(args, "--name", text)
		}
		if hash, exists := request["OriginalExpressionHash"]; exists {
			text, ok := hash.(string)
			if !ok {
				return nil, fmt.Errorf("%s: OriginalExpressionHash must be a string", step)
			}
			args = append(args, "--original-expression-hash", text)
		}
		if _, exists := request["Expression"]; exists {
			expressions, err := requestExpressions(step, request)
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(expressions)
			if err != nil {
				return nil, fmt.Errorf("%s: encode expressions: %w", step, err)
			}
			args = append(args, "--expressions", string(data))
		}
	}
	return args, nil
}

func requestString(step string, request map[string]any, field string) (string, error) {
	value, ok := request[field].(string)
	if !ok {
		return "", fmt.Errorf("%s: missing or invalid %s", step, field)
	}
	return value, nil
}

func requestID(step string, request map[string]any) (string, error) {
	value, ok := request["ID"].(types.ID)
	if !ok || value.IsEmpty() {
		return "", fmt.Errorf("%s: missing or invalid ID", step)
	}
	return value.String(), nil
}

func requestExpressions(step string, request map[string]any) ([]map[string]any, error) {
	value, ok := request["Expression"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: Expression must be a JSON object array", step)
	}
	return value, nil
}

type scenario struct {
	client cli
}

func (s scenario) item(ctx context.Context, step string, request map[string]any) (packetFilterItem, error) {
	data, err := s.client.call(ctx, step, request)
	if err != nil {
		return packetFilterItem{}, err
	}
	var item packetFilterItem
	if err := json.Unmarshal(data, &item); err != nil {
		return packetFilterItem{}, fmt.Errorf("%s: decode PacketFilter: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return packetFilterItem{}, fmt.Errorf("%s: PacketFilter ID is empty", step)
	}
	return item, nil
}

func (s scenario) find(ctx context.Context, step, name string) ([]packetFilterItem, error) {
	data, err := s.client.call(ctx, step, map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return nil, err
	}
	var items []packetFilterItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode PacketFilter list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	var matches []packetFilterItem
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func checkIdentity(item packetFilterItem, id types.ID, name string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("PacketFilter identity mismatch for ID %s", id)
	}
	return nil
}

func sameExpression(actual, expected packetFilterExpression) bool {
	return actual.Protocol == expected.Protocol &&
		actual.SourceNetwork == expected.SourceNetwork &&
		actual.SourcePort == expected.SourcePort &&
		actual.DestinationPort == expected.DestinationPort &&
		actual.Action == expected.Action &&
		actual.Description == expected.Description
}

func (s scenario) cleanup(name, updatedName string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	id := knownID
	if id.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find-original", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created PacketFilter: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != description {
			return errors.New("cannot safely identify created PacketFilter; manual cleanup required")
		}
		id = matches[0].ID
	}

	current, err := s.item(ctx, "cleanup-read", map[string]any{"Zone": zone, "ID": id})
	if err != nil {
		return fmt.Errorf("cannot verify PacketFilter before cleanup: %w", err)
	}
	if err := checkIdentity(current, id, name); err != nil {
		if err := checkIdentity(current, id, updatedName); err != nil {
			return fmt.Errorf("cleanup refused: PacketFilter identity changed; manual cleanup required: %w", err)
		}
	}
	if _, err := s.client.call(ctx, "cleanup-delete", map[string]any{"Zone": zone, "ID": id}); err != nil {
		return fmt.Errorf("delete test PacketFilter %s: %w", id, err)
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
				return fmt.Errorf("PacketFilter %s remains after delete", id)
			}
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	updatedName := name + "-updated"
	if _, err := s.client.call(ctx, "profile-current", nil); err != nil {
		return fmt.Errorf("selected profile required: %w", err)
	}
	before, err := s.find(ctx, "before-find", name)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return fmt.Errorf("test PacketFilter name %q already exists; refusing to modify it", name)
	}
	beforeUpdated, err := s.find(ctx, "before-find-updated", updatedName)
	if err != nil {
		return err
	}
	if len(beforeUpdated) != 0 {
		return fmt.Errorf("test PacketFilter name %q already exists; refusing to modify it", updatedName)
	}

	var id types.ID
	defer func() {
		if cleanupErr := s.cleanup(name, updatedName, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()
	firstExpression := packetFilterExpression{
		Protocol: "tcp", SourceNetwork: "192.0.2.0/24", DestinationPort: "443",
		Action: "allow", Description: "E2E initial rule",
	}
	created, err := s.item(ctx, "test-create", map[string]any{
		"Zone": zone, "Name": name, "Description": description,
		"Expression": []map[string]any{{
			"Protocol": firstExpression.Protocol, "SourceNetwork": firstExpression.SourceNetwork,
			"SourcePort": firstExpression.SourcePort, "DestinationPort": firstExpression.DestinationPort,
			"Action": firstExpression.Action, "Description": firstExpression.Description,
		}},
	})
	if err != nil {
		return err
	}
	id = created.ID
	if err := checkIdentity(created, id, name); err != nil {
		return err
	}
	if len(created.Expression) != 1 || !sameExpression(created.Expression[0], firstExpression) {
		return fmt.Errorf("create did not return the requested rule for PacketFilter %s", id)
	}

	found, err := s.find(ctx, "test-find", name)
	if err != nil {
		return err
	}
	if len(found) != 1 || checkIdentity(found[0], id, name) != nil {
		return fmt.Errorf("find did not return exactly the created PacketFilter %s", id)
	}
	output, err := s.client.call(ctx, "test-find-query", map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return err
	}
	var projected []struct {
		ID   types.ID
		Name string
	}
	if err := json.Unmarshal(output, &projected); err != nil {
		return fmt.Errorf("decode projected PacketFilter list: %w", err)
	}
	if len(projected) != 1 || projected[0].ID != id || projected[0].Name != name {
		return fmt.Errorf("projected list does not match created PacketFilter %s", id)
	}

	request := map[string]any{"Zone": zone, "ID": id}
	read, err := s.item(ctx, "test-read", request)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, id, name); err != nil {
		return err
	}
	if len(read.Expression) != 1 || !sameExpression(read.Expression[0], firstExpression) {
		return fmt.Errorf("read did not return the configured rule for PacketFilter %s", id)
	}

	updatedExpression := packetFilterExpression{
		Protocol: "tcp", SourceNetwork: "192.0.2.0/24", DestinationPort: "8443",
		Action: "allow", Description: "E2E updated rule",
	}
	updated, err := s.item(ctx, "test-update", map[string]any{
		"Zone": zone, "ID": id, "Name": updatedName, "OriginalExpressionHash": read.ExpressionHash,
		"Expression": []map[string]any{{
			"Protocol": updatedExpression.Protocol, "SourceNetwork": updatedExpression.SourceNetwork,
			"SourcePort": updatedExpression.SourcePort, "DestinationPort": updatedExpression.DestinationPort,
			"Action": updatedExpression.Action, "Description": updatedExpression.Description,
		}},
	})
	if err != nil {
		return err
	}
	if err := checkIdentity(updated, id, updatedName); err != nil {
		return err
	}
	if len(updated.Expression) != 1 || !sameExpression(updated.Expression[0], updatedExpression) {
		return fmt.Errorf("update did not return the requested rule for PacketFilter %s", id)
	}

	read, err = s.item(ctx, "test-read-updated", request)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, id, updatedName); err != nil {
		return err
	}
	if len(read.Expression) != 1 || !sameExpression(read.Expression[0], updatedExpression) {
		return fmt.Errorf("read did not return the updated rule for PacketFilter %s", id)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating, updating, and deleting a randomly named PacketFilter in is1b; the filter is not attached to a NIC")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/packetfilter --skr ./skr --confirm-is1b-live")
		return 2
	}

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique PacketFilter name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffix[:])
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}

	recorder, err := evidence.New("packetfilter-api")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	if err := (scenario{client: runner}).run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "PacketFilter E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("PacketFilter E2E passed; test PacketFilter deleted and absence verified.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
