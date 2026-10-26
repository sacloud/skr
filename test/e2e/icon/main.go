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
	"image"
	"image/color"
	"image/png"
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

const namePrefix = "skr-e2e-icon-"

type iconItem struct {
	ID   types.ID
	Name string
	Tags []string
}

type cli interface {
	call(context.Context, string, map[string]any) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	args, err := iconArgs(step, request)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The binary is explicitly selected; arguments are passed without a shell.
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

func iconArgs(step string, request map[string]any) ([]string, error) {
	if request == nil {
		return []string{"config", "current"}, nil
	}
	op := ""
	if strings.HasSuffix(step, "-query") {
		op = "find"
	}
	for _, candidate := range []string{"find", "read", "create", "update", "delete"} {
		if op != "" {
			break
		}
		if strings.Contains("-"+step+"-", "-"+candidate+"-") {
			op = candidate
			break
		}
	}
	if op == "" {
		return nil, fmt.Errorf("unknown Icon E2E step %q", step)
	}
	args := []string{"iaas-api", "icon", op}
	switch op {
	case "find":
		data, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("%s: encode FindRequest: %w", step, err)
		}
		args = append(args, "--request", string(data))
	case "create":
		name, nameOK := request["Name"].(string)
		imageFile, imageOK := request["ImageFile"].(string)
		if !nameOK || !imageOK || name == "" || imageFile == "" {
			return nil, fmt.Errorf("%s: Name and ImageFile are required", step)
		}
		tags, tagsOK := request["Tags"].([]string)
		if !tagsOK {
			return nil, fmt.Errorf("%s: Tags must be a string array", step)
		}
		args = append(args, "--name", name, "--image-file", imageFile, "--tags", strings.Join(tags, ","))
	case "read", "update", "delete":
		id, ok := request["ID"].(types.ID)
		if !ok || id.IsEmpty() {
			return nil, fmt.Errorf("%s: valid ID is required", step)
		}
		args = append(args, "--id", id.String())
		if op == "update" {
			name, ok := request["Name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("%s: Name is required", step)
			}
			tags, ok := request["Tags"].([]string)
			if !ok {
				return nil, fmt.Errorf("%s: Tags must be a string array", step)
			}
			args = append(args, "--name", name, "--tags", strings.Join(tags, ","))
		}
		if op == "delete" {
			args = append(args, "--fail-if-not-found")
		}
	}
	if strings.HasSuffix(step, "-query") {
		args = append(args, "--query", "map({ID,Name,Tags})")
	}
	return args, nil
}

type scenario struct {
	client cli
}

func (s scenario) run(ctx context.Context, name, imageFile string) (result error) {
	profile, err := s.client.call(ctx, "profile-current", nil)
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}

	matches, err := s.find(ctx, "preflight-find-query", name)
	if err != nil {
		return err
	}
	if len(matches) != 0 {
		return fmt.Errorf("random test icon name %q already exists; refusing to modify it", name)
	}

	created, err := s.item(ctx, "test-create", map[string]any{
		"Name": name, "ImageFile": imageFile, "Tags": []string{"skr-e2e", "temporary"},
	})
	if err != nil {
		cleanupErr := s.recoverAndCleanup(ctx, name, []string{"skr-e2e", "temporary"})
		if cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("could not safely recover a partially created Icon: %w", cleanupErr))
		}
		return err
	}
	if created.ID.IsEmpty() || created.Name != name || !sameTags(created.Tags, []string{"skr-e2e", "temporary"}) {
		return fmt.Errorf("created Icon identity does not match the test request")
	}
	expected := *created
	cleanupIdentities := []iconItem{expected}
	deleted := false
	defer func() {
		if deleted {
			return
		}
		if err := s.cleanup(ctx, cleanupIdentities...); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed; verify Icon ID %s manually: %w", created.ID, err))
		}
	}()

	read, err := s.item(ctx, "test-read", map[string]any{"ID": created.ID})
	if err != nil {
		return err
	}
	if !sameIcon(*read, expected) {
		return fmt.Errorf("read Icon identity does not match the created resource")
	}

	requestedUpdate := iconItem{ID: created.ID, Name: name + "-updated", Tags: []string{"skr-e2e", "updated"}}
	cleanupIdentities = append(cleanupIdentities, requestedUpdate)
	updated, err := s.item(ctx, "test-update", map[string]any{
		"ID": created.ID, "Name": name + "-updated", "Tags": []string{"skr-e2e", "updated"},
	})
	if err != nil {
		return err
	}
	if !sameIcon(*updated, requestedUpdate) {
		return fmt.Errorf("updated Icon fields do not match the request")
	}
	expected = requestedUpdate

	read, err = s.item(ctx, "test-read-updated", map[string]any{"ID": created.ID})
	if err != nil {
		return err
	}
	if !sameIcon(*read, expected) {
		return fmt.Errorf("updated Icon identity does not match the read result")
	}
	matches, err = s.find(ctx, "test-find-query", expected.Name)
	if err != nil {
		return err
	}
	if len(matches) != 1 || !sameIcon(matches[0], expected) {
		return fmt.Errorf("Find did not return only the updated test Icon")
	}

	if err := s.cleanup(ctx, expected); err != nil {
		return err
	}
	deleted = true
	matches, err = s.find(ctx, "verify-delete-query", expected.Name)
	if err != nil {
		return err
	}
	if len(matches) != 0 {
		return fmt.Errorf("Icon %s remains after deletion", created.ID)
	}
	return nil
}

func (s scenario) recoverAndCleanup(ctx context.Context, name string, tags []string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	matches, err := s.find(cleanupCtx, "cleanup-find-query", name)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 || matches[0].Name != name || !sameTags(matches[0].Tags, tags) || matches[0].ID.IsEmpty() {
		return fmt.Errorf("matching Icon is ambiguous or has unexpected identity; refusing to delete")
	}
	return s.cleanup(cleanupCtx, matches[0])
}

func (s scenario) item(ctx context.Context, step string, request map[string]any) (*iconItem, error) {
	data, err := s.client.call(ctx, step, request)
	if err != nil {
		return nil, err
	}
	var item iconItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("%s: decode Icon: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return nil, fmt.Errorf("%s: Icon ID is empty", step)
	}
	return &item, nil
}

func (s scenario) find(ctx context.Context, step, name string) ([]iconItem, error) {
	data, err := s.client.call(ctx, step, map[string]any{"Names": []string{name}})
	if err != nil {
		return nil, err
	}
	var items []iconItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Icon list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) cleanup(ctx context.Context, identities ...iconItem) error {
	expected := identities[0]
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	current, err := s.item(cleanupCtx, "cleanup-read", map[string]any{"ID": expected.ID})
	if err != nil {
		return err
	}
	matches := false
	for _, identity := range identities {
		matches = matches || (identity.ID == expected.ID && sameIcon(*current, identity))
	}
	if !matches {
		return fmt.Errorf("Icon identity changed for ID %s; refusing to delete", expected.ID)
	}
	_, err = s.client.call(cleanupCtx, "cleanup-delete", map[string]any{"ID": expected.ID})
	return err
}

func sameIcon(actual, expected iconItem) bool {
	return actual.ID == expected.ID && actual.Name == expected.Name && sameTags(actual.Tags, expected.Tags)
}

func sameTags(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	remaining := make(map[string]int, len(expected))
	for _, tag := range expected {
		remaining[tag]++
	}
	for _, tag := range actual {
		if remaining[tag] == 0 {
			return false
		}
		remaining[tag]--
	}
	return true
}

func createImageFile() (string, error) {
	file, err := os.CreateTemp("", "skr-icon-e2e-*.png")
	if err != nil {
		return "", fmt.Errorf("create temporary icon image: %w", err)
	}
	path := file.Name()
	imageData := image.NewRGBA(image.Rect(0, 0, 60, 56))
	imageData.Set(10, 10, color.RGBA{R: 255, A: 255})
	if err := png.Encode(file, imageData); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("encode temporary icon image: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temporary icon image: %w", err)
	}
	return path, nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-icon-live", false, "Confirm creating, updating, and deleting a user icon with the selected SDK profile")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/icon --skr ./skr --confirm-icon-live")
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
	recorder, err := evidence.New("icon-api", evidence.Scenario{
		Title:       "アイコンの CRUD",
		Description: "IaaS アイコンの作成・参照・更新・削除を実行し、リソースのライフサイクルが正しく動作することを確認します。",
	})
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

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		fmt.Fprintln(os.Stderr, "generate test Icon name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(random)
	imagePath, err := createImageFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.Remove(imagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove temporary image:", err)
			exitCode = 1
		}
	}()

	fmt.Println("This E2E creates, updates, and deletes one uniquely named user icon.")
	fmt.Println("Review the selected SDK profile and project before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}}).run(ctx, name, imagePath); err != nil {
		fmt.Fprintln(os.Stderr, "Icon E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Icon E2E passed; create, find, read, update, and delete results were verified.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
