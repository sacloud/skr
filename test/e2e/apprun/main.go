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
	"io"
	"net/http"
	"net/url"
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
	namePrefix = "skr-e2e-apprun-"
	image      = "ghcr.io/sacloud/apprun-samples/tutorial:latest"
	port       = 8080
	queryApps  = ".data | map({id,name,status,public_url})"
)

type appSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	PublicURL string `json:"public_url"`
}

type app struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Port       int    `json:"port"`
	PublicURL  string `json:"public_url"`
	Components []struct {
		DeploySource struct {
			ContainerRegistry struct {
				Image string `json:"image"`
			} `json:"container_registry"`
		} `json:"deploy_source"`
	} `json:"components"`
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
	return r.run(ctx, step, append([]string{"apprun-api"}, args...)...)
}

func (r cliRunner) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return r.run(ctx, step, args...)
}

func (r cliRunner) run(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	var requestFile any
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--request" || !strings.HasPrefix(args[i+1], "@") {
			continue
		}
		path := strings.TrimPrefix(args[i+1], "@")
		data, err := os.ReadFile(path) // #nosec G304 -- The E2E scenario creates request files in a private temporary directory.
		if err != nil {
			return nil, fmt.Errorf("%s: read request file for evidence: %w", step, err)
		}
		requestFile = evidence.RequestFileInput{Path: path, Content: string(data)}
		break
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The selected local skr binary and structured arguments are executed without a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, requestFile, stdout.String(), stderr.String(), runErr, false); err != nil {
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
	client     cli
	requestDir string
	checkURL   func(context.Context, string) error
}

func (s scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func (s scenario) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.callRoot(ctx, step, args...)
}

func (s scenario) writeCreateRequest(name string) (string, error) {
	request := map[string]any{
		"name":                     name,
		"timeout_seconds":          60,
		"port":                     port,
		"min_scale":                0,
		"max_scale":                1,
		"scale_target_concurrency": 100,
		"components": []any{
			map[string]any{
				"name":       "web",
				"max_cpu":    "0.5",
				"max_memory": "1Gi",
				"deploy_source": map[string]any{
					"container_registry": map[string]any{"image": image},
				},
			},
		},
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode AppRun create request: %w", err)
	}
	path := filepath.Join(s.requestDir, "create.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write AppRun create request: %w", err)
	}
	return path, nil
}

func (s scenario) list(ctx context.Context, step string) ([]appSummary, error) {
	items := make([]appSummary, 0)
	for page := 1; ; page++ {
		data, err := s.call(ctx, step, "application", "list", "--page-num", strconv.Itoa(page), "--page-size", "100", "--query", queryApps)
		if err != nil {
			return nil, err
		}
		var batch []appSummary
		if err := json.Unmarshal(data, &batch); err != nil {
			return nil, fmt.Errorf("%s: decode AppRun application list page %d: %w", step, page, err)
		}
		if batch == nil {
			return nil, fmt.Errorf("%s: expected a JSON application array on page %d", step, page)
		}
		items = append(items, batch...)
		if len(batch) < 100 {
			return items, nil
		}
	}
}

func (s scenario) read(ctx context.Context, step, id string) (app, error) {
	data, err := s.call(ctx, step, "application", "read", id)
	if err != nil {
		return app{}, err
	}
	var result app
	if err := json.Unmarshal(data, &result); err != nil {
		return app{}, fmt.Errorf("%s: decode AppRun application: %w", step, err)
	}
	return result, nil
}

func checkApp(value app, id, name string) error {
	if value.ID != id || value.Name != name || value.Port != port || value.PublicURL == "" ||
		len(value.Components) != 1 || value.Components[0].DeploySource.ContainerRegistry.Image != image {
		return fmt.Errorf("AppRun application identity or configuration mismatch for ID %q; refusing cleanup", id)
	}
	return nil
}

func (s scenario) cleanup(name, knownID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id := knownID
	if id == "" {
		return errors.New("created AppRun application ID is unknown; refusing cleanup; manually verify creation using private evidence")
	}

	current, err := s.read(ctx, "cleanup-read-before-delete", id)
	if err != nil {
		return fmt.Errorf("cannot verify AppRun application before cleanup: %w", err)
	}
	if err := checkApp(current, id, name); err != nil {
		return err
	}
	if _, err := s.call(ctx, "cleanup-delete", "application", "delete", id); err != nil {
		return fmt.Errorf("delete E2E AppRun application: %w", err)
	}
	remaining, err := s.list(ctx, "cleanup-list-after-delete")
	if err != nil {
		return fmt.Errorf("AppRun application absence could not be verified: %w", err)
	}
	for _, item := range remaining {
		if item.ID == id {
			return fmt.Errorf("AppRun application %s remains after deletion", id)
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	profile, err := s.callRoot(ctx, "profile-current", "config", "current")
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
	before, err := s.list(ctx, "preflight-list")
	if err != nil {
		return err
	}
	for _, item := range before {
		if item.Name == name {
			return fmt.Errorf("AppRun application name %q already exists; refusing to modify it", name)
		}
	}

	creationAttempted := false
	var id string
	defer func() {
		if !creationAttempted {
			return
		}
		if cleanupErr := s.cleanup(name, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	requestPath, err := s.writeCreateRequest(name)
	if err != nil {
		return err
	}
	creationAttempted = true
	createdData, err := s.call(ctx, "create", "application", "create", "--request", "@"+requestPath)
	if err != nil {
		return err
	}
	var created app
	if err := json.Unmarshal(createdData, &created); err != nil {
		return fmt.Errorf("decode created AppRun application: %w", err)
	}
	id = created.ID
	if id == "" {
		return errors.New("created AppRun application has no ID")
	}
	if err := checkApp(created, id, name); err != nil {
		return err
	}

	listed, err := s.list(ctx, "list-created")
	if err != nil {
		return err
	}
	found := 0
	for _, item := range listed {
		if item.ID == id && item.Name == name {
			found++
		}
	}
	if found != 1 {
		return fmt.Errorf("AppRun list returned %d entries for the created ID %s; want exactly one", found, id)
	}

	current, err := s.read(ctx, "read-created", id)
	if err != nil {
		return err
	}
	if err := checkApp(current, id, name); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx, id); err != nil {
		return err
	}
	if s.checkURL == nil {
		return errors.New("AppRun public URL checker is not configured")
	}
	if err := s.checkURL(ctx, current.PublicURL); err != nil {
		return err
	}
	return nil
}

func (s scenario) waitHealthy(ctx context.Context, id string) error {
	const waitLimit = 3 * time.Minute
	waitCtx, cancel := context.WithTimeout(ctx, waitLimit)
	defer cancel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		statusData, err := s.call(waitCtx, "read-status", "application", "status", id)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("waiting for AppRun application to become Healthy: %w", ctx.Err())
			}
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out after %s waiting for AppRun application to become Healthy", waitLimit)
			}
			return err
		}
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(statusData, &status); err != nil {
			return fmt.Errorf("decode AppRun status: %w", err)
		}
		switch status.Status {
		case "Healthy":
			return nil
		case "Deploying":
		case "UnHealthy":
			return errors.New("AppRun application status is UnHealthy")
		default:
			return fmt.Errorf("unexpected AppRun application status %q", status.Status)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for AppRun application to become Healthy: %w", ctx.Err())
		case <-waitCtx.Done():
			return fmt.Errorf("timed out after %s waiting for AppRun application to become Healthy", waitLimit)
		case <-ticker.C:
		}
	}
}

func checkPublicURL(ctx context.Context, recorder *evidence.Recorder, publicURL string) error {
	return checkPublicURLWithClient(ctx, recorder, publicURL, &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, 10*time.Second)
}

func checkPublicURLWithClient(ctx context.Context, recorder *evidence.Recorder, publicURL string, client *http.Client, interval time.Duration) error {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("AppRun returned an invalid HTTPS public URL")
	}
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var last string
	for {
		status, body, requestErr := getPublicURL(ctx, client, publicURL)
		success := requestErr == nil && status == http.StatusOK && strings.Contains(body, "Hello from AppRun!")
		attemptErr := requestErr
		if !success && attemptErr == nil {
			attemptErr = fmt.Errorf("unexpected HTTP status or sample response: HTTP %d", status)
		}
		output := fmt.Sprintf("HTTP status: %d\n\n%s", status, body)
		if err := recorder.Record("public-url", "GET", []string{publicURL}, nil, output, "", attemptErr, false); err != nil {
			return errors.Join(attemptErr, err)
		}
		if success {
			return nil
		}
		last = fmt.Sprintf("HTTP status %d, body %q, error %v", status, body, requestErr)
		select {
		case <-ctx.Done():
			return recordURLFailure(recorder, publicURL, last, ctx.Err())
		case <-deadline.C:
			return recordURLFailure(recorder, publicURL, last, errors.New("timed out waiting for AppRun public URL"))
		case <-ticker.C:
		}
	}
}

func getPublicURL(ctx context.Context, client *http.Client, publicURL string) (int, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL, nil)
	if err != nil {
		return 0, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, "", err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if err != nil {
		return response.StatusCode, string(data), errors.Join(err, closeErr)
	}
	if closeErr != nil {
		return response.StatusCode, string(data), fmt.Errorf("close AppRun public URL response: %w", closeErr)
	}
	return response.StatusCode, string(data), nil
}

func recordURLFailure(recorder *evidence.Recorder, publicURL, output string, runErr error) error {
	if err := recorder.Record("public-url", "GET", []string{publicURL}, nil, output, "", runErr, false); err != nil {
		return errors.Join(runErr, err)
	}
	return fmt.Errorf("AppRun public URL did not return the expected sample response: %s: %w", output, runErr)
}

func randomSuffix() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique AppRun name: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-apprun-live", false, "Confirm creating and deleting one temporary AppRun application; a request to its public URL may start a billable instance")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/apprun --skr ./skr --confirm-apprun-live")
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

	recorder, err := evidence.New("apprun")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())
	tempDir, err := os.MkdirTemp("", "skr-e2e-apprun-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create private temporary directory:", err)
		if saveErr := recorder.SetResult("failed", time.Now()); saveErr != nil {
			fmt.Fprintln(os.Stderr, "save E2E result:", saveErr)
		}
		return 1
	}
	defer func() {
		var cleanupErr error
		if err := os.Remove(filepath.Join(tempDir, "create.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove private AppRun request file: %w", err))
		}
		if err := os.Remove(tempDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove private AppRun temporary directory: %w", err))
		}
		if cleanupErr != nil {
			fmt.Fprintln(os.Stderr, cleanupErr)
			exitCode = 1
		}
		result := "failed"
		if exitCode == 0 {
			result = "passed"
		}
		if err := recorder.SetResult(result, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "save E2E result:", err)
			exitCode = 1
		}
	}()

	suffix, err := randomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := namePrefix + suffix
	fmt.Println("Unique AppRun application to create:", name)
	fmt.Println("This scenario creates one application, requests its public URL, then deletes only the application created by this run.")
	fmt.Println("Check `skr config current` and the selected project before running. AppRun usage can incur charges.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	test := scenario{
		client:     runner,
		requestDir: tempDir,
		checkURL:   func(ctx context.Context, publicURL string) error { return checkPublicURL(ctx, recorder, publicURL) },
	}
	if err := test.run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "AppRun E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("AppRun E2E passed; the sample response was observed and the created application was deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
