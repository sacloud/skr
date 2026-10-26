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

type groupItem struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Settings    struct {
		Destinations []string `json:"Destinations"`
		Disabled     bool     `json:"Disabled"`
	} `json:"Settings"`
}

type groupResponse struct {
	CommonServiceItem groupItem `json:"CommonServiceItem"`
}

type notificationHistory struct {
	RequestID string               `json:"request_id"`
	Message   string               `json:"message"`
	Statuses  []notificationStatus `json:"statuses"`
}

type notificationStatus struct {
	GroupID       string `json:"group_id"`
	DestinationID string `json:"destination_id"`
	Status        int    `json:"status"`
}

type cli interface {
	call(context.Context, string, any, ...string) ([]byte, error)
}

type evidenceRecorder interface {
	Record(string, string, []string, any, string, string, error, bool) error
	Dir() string
}

type cliRunner struct {
	binary       string
	evidence     evidenceRecorder
	recordErrors *[]error
	runCommand   func(context.Context, string, []string) ([]byte, []byte, error)
}

func (r cliRunner) call(ctx context.Context, step string, input any, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	runCommand := r.runCommand
	if runCommand == nil {
		runCommand = executeCLI
	}
	stdout, stderr, runErr := runCommand(ctx, r.binary, args)
	recordErr := r.evidence.Record(step, r.binary, args, input, string(stdout), string(stderr), runErr, false)
	if recordErr != nil {
		if r.recordErrors == nil {
			return stdout, fmt.Errorf("%s: record E2E evidence: %w", step, recordErr)
		}
		*r.recordErrors = append(*r.recordErrors, recordErr)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see private evidence)", step, runErr)
	}
	return stdout, nil
}

func executeCLI(ctx context.Context, binary string, args []string) ([]byte, []byte, error) {
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, binary, args...) //nolint:gosec // Structured arguments are passed without a shell to the selected skr binary.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	return stdout.Bytes(), stderr.Bytes(), runErr
}

type scenario struct {
	client        cli
	destinationID string
	requestPath   string
	message       string
	historyPoll   time.Duration
	historyLimit  time.Duration
}

func (s scenario) call(ctx context.Context, step string, input any, args ...string) ([]byte, error) {
	commandArgs := append([]string{"simple-notification-api"}, args...)
	return s.client.call(ctx, step, input, commandArgs...)
}

func (s scenario) list(ctx context.Context, step string) ([]groupItem, error) {
	data, err := s.call(ctx, step, nil, "group", "list",
		"--query", ".CommonServiceItems | map({ID,Name,Description,Settings})")
	if err != nil {
		return nil, err
	}
	var items []groupItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode projected group list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON group array", step)
	}
	return items, nil
}

func (s scenario) read(ctx context.Context, step, id string) (groupItem, error) {
	data, err := s.call(ctx, step, nil, "group", "read", id)
	if err != nil {
		return groupItem{}, err
	}
	var response groupResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return groupItem{}, fmt.Errorf("%s: decode group response: %w", step, err)
	}
	if response.CommonServiceItem.ID == "" {
		return groupItem{}, fmt.Errorf("%s: group ID is missing", step)
	}
	return response.CommonServiceItem, nil
}

func sameGroup(item groupItem, id, name, description, destinationID string) bool {
	return item.ID == id &&
		item.Name == name &&
		item.Description == description &&
		len(item.Settings.Destinations) == 1 &&
		item.Settings.Destinations[0] == destinationID &&
		!item.Settings.Disabled
}

func (s scenario) cleanup(name, description, knownID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if knownID == "" {
		return errors.New("cannot safely identify E2E group without the ID returned by create; manual cleanup required")
	}

	current, err := s.read(ctx, "e2e-cleanup-read", knownID)
	if err != nil {
		return fmt.Errorf("cannot verify group before cleanup: %w", err)
	}
	if !sameGroup(current, knownID, name, description, s.destinationID) {
		return errors.New("cleanup refused: group identity or destination does not match this E2E run")
	}

	_, deleteErr := s.call(ctx, "e2e-cleanup-delete", nil, "group", "delete", knownID)
	remaining, listErr := s.list(ctx, "e2e-cleanup-list-after")
	if listErr != nil {
		return errors.Join(
			wrapIfError("delete E2E group", deleteErr),
			fmt.Errorf("group absence could not be verified: %w", listErr),
		)
	}
	for _, item := range remaining {
		if item.ID == knownID || item.Name == name {
			return errors.Join(
				wrapIfError("delete E2E group", deleteErr),
				fmt.Errorf("E2E group %s remains after delete", knownID),
			)
		}
	}
	return wrapIfError("delete E2E group", deleteErr)
}

func wrapIfError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func (s scenario) selectedProfile(ctx context.Context) (string, error) {
	profile, err := s.client.call(ctx, "profile-current", nil, "config", "current")
	if err != nil {
		return "", fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return "", errors.New("selected SDK profile name is empty")
	}
	return string(profile), nil
}

func (s scenario) run(ctx context.Context, name, description string) (result error) {
	profile, err := s.selectedProfile(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Selected SDK profile: %s", profile)
	if !strings.HasSuffix(profile, "\n") {
		fmt.Println()
	}

	statusData, err := s.call(ctx, "e2e-read-destination-status", nil, "destination", "read-status", s.destinationID)
	if err != nil {
		return fmt.Errorf("cannot verify supplied notification destination: %w", err)
	}
	var destinationStatus struct {
		NotificationStatus struct {
			IsValid bool `json:"IsValid"`
		} `json:"NotificationStatus"`
	}
	if err := json.Unmarshal(statusData, &destinationStatus); err != nil {
		return fmt.Errorf("decode destination status: %w", err)
	}
	if !destinationStatus.NotificationStatus.IsValid {
		return fmt.Errorf("notification destination %s is not valid for sending", s.destinationID)
	}

	before, err := s.list(ctx, "e2e-preflight-list")
	if err != nil {
		return err
	}
	for _, item := range before {
		if item.Name == name {
			return fmt.Errorf("group name %q already exists; refusing to modify it", name)
		}
	}

	createAttempted := false
	var id string
	defer func() {
		if !createAttempted {
			return
		}
		if cleanupErr := s.cleanup(name, description, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	request := struct {
		CommonServiceItem struct {
			Name        string `json:"Name"`
			Description string `json:"Description"`
			Settings    struct {
				Destinations []string `json:"Destinations"`
				Disabled     bool     `json:"Disabled"`
			} `json:"Settings"`
		} `json:"CommonServiceItem"`
	}{}
	request.CommonServiceItem.Name = name
	request.CommonServiceItem.Description = description
	request.CommonServiceItem.Settings.Destinations = []string{s.destinationID}
	requestData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode group create request: %w", err)
	}
	if err := os.WriteFile(s.requestPath, requestData, 0o600); err != nil {
		return fmt.Errorf("write private group request file: %w", err)
	}

	createAttempted = true
	input := evidence.RequestFileInput{Path: s.requestPath, Content: string(requestData)}
	data, err := s.call(ctx, "e2e-create-group", input,
		"group", "create", "--request", "@"+s.requestPath)
	if err != nil {
		return err
	}
	var created groupResponse
	if err := json.Unmarshal(data, &created); err != nil {
		return fmt.Errorf("decode created group: %w", err)
	}
	id = created.CommonServiceItem.ID
	if id == "" || !sameGroup(created.CommonServiceItem, id, name, description, s.destinationID) {
		return errors.New("create response does not match the requested group identity and destination")
	}

	listed, err := s.list(ctx, "e2e-list-created-groups")
	if err != nil {
		return err
	}
	var found int
	for _, item := range listed {
		if item.Name == name {
			found++
			if !sameGroup(item, id, name, description, s.destinationID) {
				return errors.New("group list does not match the created group")
			}
		}
	}
	if found != 1 {
		return fmt.Errorf("group list returned %d groups named %q; want one", found, name)
	}

	current, err := s.read(ctx, "e2e-read-created-group", id)
	if err != nil {
		return err
	}
	if !sameGroup(current, id, name, description, s.destinationID) {
		return errors.New("group read does not match the created group")
	}

	data, err = s.call(ctx, "e2e-send-message", nil, "group", "send-message", id, "--message", s.message)
	if err != nil {
		return err
	}
	var sendResponse struct {
		IsOK bool `json:"is_ok"`
	}
	if err := json.Unmarshal(data, &sendResponse); err != nil {
		return fmt.Errorf("decode send response: %w", err)
	}
	if !sendResponse.IsOK {
		return errors.New("notification API did not accept the test message")
	}
	return s.waitForNotificationHistory(ctx, id)
}

func (s scenario) waitForNotificationHistory(ctx context.Context, groupID string) error {
	pollInterval := s.historyPoll
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	historyLimit := s.historyLimit
	if historyLimit <= 0 {
		historyLimit = 2 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, historyLimit)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for attempt := 1; ; attempt++ {
		step := fmt.Sprintf("e2e-notification-history-%02d", attempt)
		data, err := s.call(waitCtx, step, nil, "history", "list",
			"--query", ".NotificationHistories | map({request_id,message:.message.body,statuses:[.statuses[] | {group_id,destination_id,status}]})")
		if err != nil {
			return fmt.Errorf("read notification history: %w", err)
		}
		var histories []notificationHistory
		if err := json.Unmarshal(data, &histories); err != nil {
			return fmt.Errorf("decode notification history: %w", err)
		}
		if histories == nil {
			return errors.New("notification history query did not return a JSON array")
		}

		var matching *notificationHistory
		for index := range histories {
			if histories[index].Message != s.message {
				continue
			}
			if matching != nil {
				return errors.New("notification history contains multiple entries for the unique E2E message")
			}
			matching = &histories[index]
		}
		if matching != nil {
			if matching.RequestID == "" {
				return errors.New("matching notification history has no request ID")
			}
			foundStatus := false
			for _, status := range matching.Statuses {
				if status.GroupID != groupID {
					continue
				}
				if status.DestinationID != s.destinationID {
					return errors.New("notification history destination does not match the supplied destination")
				}
				foundStatus = true
				switch status.Status {
				case 2:
					return s.verifyNotificationHistory(waitCtx, *matching)
				case 0, 1:
					continue
				case 3, 9:
					return fmt.Errorf("notification processing ended with status %d", status.Status)
				default:
					return fmt.Errorf("notification history returned unknown status %d", status.Status)
				}
			}
			if !foundStatus {
				return errors.New("matching notification history has no status for the created group")
			}
		}

		select {
		case <-waitCtx.Done():
			if matching == nil {
				return fmt.Errorf("notification history for the test message did not appear before timeout: %w", waitCtx.Err())
			}
			return fmt.Errorf("notification processing did not complete before timeout: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (s scenario) verifyNotificationHistory(ctx context.Context, expected notificationHistory) error {
	data, err := s.call(ctx, "e2e-read-notification-history", nil, "history", "read", expected.RequestID,
		"--query", ".NotificationHistory | {request_id,message:.message.body,statuses:[.statuses[] | {group_id,destination_id,status}]}")
	if err != nil {
		return err
	}
	var actual notificationHistory
	if err := json.Unmarshal(data, &actual); err != nil {
		return fmt.Errorf("decode individual notification history: %w", err)
	}
	if actual.RequestID != expected.RequestID || actual.Message != expected.Message ||
		len(actual.Statuses) != 1 || len(expected.Statuses) != 1 ||
		actual.Statuses[0] != expected.Statuses[0] {
		return errors.New("individual notification history does not match the list entry")
	}
	return nil
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique group suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	destinationID := flag.String("destination-id", "", "ID of an existing verified, enabled notification destination")
	crudOnly := flag.Bool("crud-only", false, "Test temporary email destination, disabled group and isolated routing CRUD without sending a test notification; registration may send a verification email")
	confirmed := flag.Bool("confirm-simple-notification-live", false, "Confirm live resource changes; the default scenario sends a test notification, while --crud-only may send a registration verification email")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *destinationID == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/simple-notification --skr ./skr --destination-id DESTINATION-ID [--crud-only] --confirm-simple-notification-live")
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
	recorder, err := evidence.New("simple-notification-api", evidence.Scenario{
		Title:       "シンプル通知の各リソース操作",
		Description: "シンプル通知 API で宛先、グループ、履歴、ルーティングの作成・参照・更新・削除が正しく動作することを確認します。",
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

	tempDir, err := os.MkdirTemp("", "skr-e2e-simple-notification-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create private request directory:", err)
		return 1
	}
	requestPath := filepath.Join(tempDir, "create-request.json")
	defer func() {
		var cleanupErr error
		if err := os.Remove(requestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove private request file:", err)
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if err := os.Remove(tempDir); err != nil {
			fmt.Fprintln(os.Stderr, "remove private request directory:", err)
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
	name := "skr-e2e-notify-" + suffix
	description := "skr-e2e-simple-notification/" + suffix
	message := "skr E2E test notification " + suffix

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var recordErrors []error
	runner := cliRunner{binary: path, evidence: recorder, recordErrors: &recordErrors}
	var runErr error
	if *crudOnly {
		fmt.Println("CRUD-only: temporary resources will be created and deleted; no test notification will be sent.")
		runErr = (crudScenario{scenario{client: runner, destinationID: *destinationID, requestPath: requestPath}}).runCRUD(ctx, suffix)
	} else {
		fmt.Println("Group to create:", name)
		fmt.Println("A real notification will be sent to the supplied destination ID. Only the created group will be deleted.")
		runErr = (scenario{
			client: runner, destinationID: *destinationID, requestPath: requestPath, message: message,
		}).run(ctx, name, description)
	}
	runErr = errors.Join(runErr, errors.Join(recordErrors...))
	if runErr != nil {
		if *crudOnly {
			fmt.Fprintln(os.Stderr, "Simple Notification CRUD E2E failed:", runErr)
		} else {
			fmt.Fprintln(os.Stderr, "Simple Notification E2E failed:", runErr)
		}
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	if *crudOnly {
		fmt.Println("Simple Notification CRUD E2E passed: temporary resources were verified and deleted.")
		return 0
	}
	fmt.Println("Simple Notification E2E passed: notification processing completed and the temporary group was deleted.")
	fmt.Println("Delivery is not guaranteed; inspect the service notification history for delivery status.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
