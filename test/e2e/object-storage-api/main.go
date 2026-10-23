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

const bucketListQuery = "map({name,resource_id,plan})"

var errUnverifiedBucketSetting = errors.New("bucket setting ownership is unverified; refusing bucket deletion; manual inspection required")

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

type commandError struct {
	step         string
	stderr       string
	cause        error
	recordFailed bool
}

func (e *commandError) Error() string {
	return fmt.Sprintf("%s: skr failed: %v (see private evidence for stderr)", e.step, e.cause)
}

func (e *commandError) Unwrap() error { return e.cause }

func accountNotFound(err error) bool {
	var failure *commandError
	return errors.As(err, &failure) &&
		!failure.recordFailed &&
		strings.HasPrefix(strings.TrimSpace(failure.stderr), "object-storage: Accounts.Read: API Error 404:")
}

func (r cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The selected binary and structured arguments are not a shell command.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	var request any
	for i, arg := range args {
		if arg == "--request" && i+1 < len(args) && strings.HasPrefix(args[i+1], "@") {
			path := strings.TrimPrefix(args[i+1], "@")
			content, err := os.ReadFile(path) //nolint:gosec // Records the same explicit request file passed to the CLI.
			if err != nil {
				return nil, fmt.Errorf("read request for evidence: %w", err)
			}
			request = evidence.RequestFileInput{Path: path, Content: string(content)}
		}
	}
	runErr := command.Run()
	secret := strings.Contains(strings.Join(args, " "), "access-key")
	recordedStderr := stderr.String()
	if secret {
		recordedStderr = "[REDACTED: access-key command]"
	}
	if runErr != nil {
		runErr = &commandError{step: step, stderr: stderr.String(), cause: runErr}
	}
	recordErr := r.evidence.Record(step, r.binary, args, request, stdout.String(), recordedStderr, runErr, secret)
	var failure *commandError
	if recordErr != nil && errors.As(runErr, &failure) {
		failure.recordFailed = true
	}
	return stdout.Bytes(), errors.Join(runErr, recordErr)
}

type account struct {
	ResourceID string `json:"resource_id"`
}

type bucket struct {
	Name       string          `json:"name"`
	ResourceID string          `json:"resource_id"`
	Plan       json.RawMessage `json:"plan"`
}

type createdBucket struct {
	Name       string          `json:"name"`
	ClusterID  string          `json:"cluster_id"`
	ResourceID string          `json:"resource_id"`
	Plan       json.RawMessage `json:"plan"`
}

type scenario struct {
	client cli
	siteID string
	plan   string
	extra  func(context.Context, string) error
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	profile, err := s.client.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	if err := s.checkSite(ctx); err != nil {
		return err
	}

	accountOutput, err := s.client.call(ctx, "site-account-read", "object-storage-api", "account", "read", s.siteID)
	if err != nil {
		return fmt.Errorf("site account must already exist: %w", err)
	}
	var existingAccount account
	if err := json.Unmarshal(accountOutput, &existingAccount); err != nil {
		return fmt.Errorf("decode site account: %w", err)
	}
	if existingAccount.ResourceID == "" {
		return errors.New("site account read returned no resource ID")
	}
	if _, err := s.permissions(ctx, "permission-schema-preflight"); err != nil {
		return err
	}

	items, err := s.list(ctx, "bucket-preflight")
	if err != nil {
		return err
	}
	if matches := bucketsNamed(items, name); len(matches) != 0 {
		return fmt.Errorf("generated bucket name %q already exists; refusing to modify it", name)
	}

	createArgs := []string{"object-storage-api", "bucket", "create", s.siteID, name}
	if s.plan != "" {
		createArgs = append(createArgs, "--plan", s.plan)
	}
	createdOutput, err := s.client.call(ctx, "bucket-create", createArgs...)
	if err != nil {
		return fmt.Errorf("bucket creation may have succeeded without returning its identity; inspect site %s and bucket %s before cleanup: %w", s.siteID, name, err)
	}
	var created createdBucket
	if err := json.Unmarshal(createdOutput, &created); err != nil {
		return fmt.Errorf("decode created bucket identity; inspect site %s and bucket %s before cleanup: %w", s.siteID, name, err)
	}
	if created.Name != name || created.ClusterID != s.siteID {
		return fmt.Errorf("created bucket identity does not match the request; inspect site %s and bucket %s before cleanup", s.siteID, name)
	}

	deleted := false
	defer func() {
		if deleted || errors.Is(result, errUnverifiedBucketSetting) {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := s.cleanup(cleanupCtx, created, true); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed; verify site %s bucket %s manually: %w", created.ClusterID, created.Name, err))
		}
	}()

	items, err = s.list(ctx, "bucket-verify-create")
	if err != nil {
		return err
	}
	matches := bucketsNamed(items, name)
	if len(matches) != 1 || matches[0].Name != created.Name {
		return fmt.Errorf("bucket list did not return exactly the created bucket %q", name)
	}
	if matches[0].ResourceID == "" {
		return fmt.Errorf("bucket list returned no resource ID for %q; refusing automatic deletion", name)
	}
	created.ResourceID = matches[0].ResourceID
	bucketPlan := matches[0]
	if len(bucketPlan.Plan) == 0 || string(bucketPlan.Plan) == "null" {
		bucketPlan.Plan = created.Plan
	}
	if err := s.checkBucketPlan(bucketPlan); err != nil {
		return err
	}
	if err := s.runPermission(ctx, name); err != nil {
		return err
	}
	if s.extra != nil {
		if err := s.extra(ctx, name); err != nil {
			return err
		}
	}

	if err := s.cleanup(ctx, created, false); err != nil {
		return err
	}
	items, err = s.list(ctx, "bucket-verify-delete")
	if err != nil {
		return err
	}
	if len(bucketsNamed(items, name)) != 0 {
		return fmt.Errorf("bucket %q remains after deletion", name)
	}
	deleted = true
	return nil
}

func (s scenario) list(ctx context.Context, step string) ([]bucket, error) {
	output, err := s.client.call(ctx, step,
		"object-storage-api", "bucket", "list", s.siteID,
		"--query", bucketListQuery,
	)
	if err != nil {
		return nil, err
	}
	return decodeArray[bucket](output, "bucket list")
}

func (s scenario) cleanup(ctx context.Context, created createdBucket, retry bool) error {
	checkStep := "bucket-cleanup-check"
	deleteStep := "bucket-delete"
	if retry {
		checkStep = "bucket-cleanup-retry-check"
		deleteStep = "bucket-delete-retry"
	}
	items, err := s.list(ctx, checkStep)
	if err != nil {
		return err
	}
	matches := bucketsNamed(items, created.Name)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 || matches[0].Name != created.Name || created.ResourceID == "" || matches[0].ResourceID != created.ResourceID {
		return fmt.Errorf("refusing to delete bucket %q because its resource ID does not match the created bucket", created.Name)
	}
	_, err = s.client.call(ctx, deleteStep, "object-storage-api", "bucket", "delete", created.ClusterID, created.Name)
	if err != nil {
		return err
	}
	items, err = s.list(ctx, deleteStep+"-verify")
	if err != nil {
		return err
	}
	if len(bucketsNamed(items, created.Name)) != 0 {
		return fmt.Errorf("bucket %q remains after cleanup", created.Name)
	}
	return nil
}

func bucketsNamed(items []bucket, name string) []bucket {
	var matches []bucket
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches
}

func newBucketName() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary bucket name: %w", err)
	}
	return "skr-e2e-" + hex.EncodeToString(random[:]), nil
}

func validateSiteID(siteID string) error {
	switch siteID {
	case "isk01", "tky01":
		return nil
	default:
		return fmt.Errorf("site ID must be a standard Object Storage site: isk01 or tky01")
	}
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	siteID := flag.String("site-id", "", "Object Storage site ID: isk01 or tky01; archive and account also support arc02")
	mode := flag.String("scenario", "tutorial", "Scenario: tutorial, metrics, archive, account, encryption, or replication")
	plan := flag.String("plan", "", "Archive plan value from site plans arc02 (archive scenario only)")
	kmsKeyID := flag.String("kms-key-id", "", "Existing KMS key ID for the encryption scenario (tky01 only); the key is not modified")
	targetSiteID := flag.String("target-site-id", "", "Other standard site for replication; creates a second billable bucket")
	createTargetAccount := flag.Bool("confirm-create-target-account-live", false, "Replication only: create and delete an absent target-site account and its test key")
	metricsBucket := flag.String("bucket-name", "", "Existing bucket name for the read-only metrics scenario")
	metricsID := flag.String("bucket-resource-id", "", "Expected resource ID of the existing metrics bucket")
	confirmed := flag.Bool("confirm-create-bucket-live", false, "Confirm creating and deleting one temporary billable bucket using the selected SDK profile")
	accountConfirmed := flag.Bool("confirm-create-account-live", false, "Confirm creating and deleting an absent source-site account and its test key; refuses existing accounts")
	flag.Parse()
	optionsErr := validateOptions(*mode, *siteID, *plan, *confirmed, *accountConfirmed)
	if optionsErr == nil {
		optionsErr = validateDependencies(*mode, *siteID, *kmsKeyID, *targetSiteID)
	}
	if optionsErr == nil {
		optionsErr = validateMetricsTarget(*mode, *metricsBucket, *metricsID, *confirmed)
	}
	if optionsErr == nil && *createTargetAccount && *mode != "replication" {
		optionsErr = errors.New("--confirm-create-target-account-live is only for replication")
	}
	if optionsErr != nil || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "use --help for scenario-specific flags and explicit live confirmations")
		if optionsErr != nil {
			fmt.Fprintln(os.Stderr, optionsErr)
		}
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
	name, err := newBucketName()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	recorder, err := evidence.New("object-storage-api-" + *mode)
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

	fmt.Printf("Scenario %s at site %s. See docs/design/object-storage-api.md for resources, fees, and prerequisites.\n", *mode, *siteID)
	fmt.Println("Review the selected SDK profile, site account, and expected charges before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := scenario{client: cliRunner{binary: path, evidence: recorder}, siteID: *siteID, plan: *plan}
	switch *mode {
	case "metrics":
		s.extra = s.readMetrics
	case "encryption":
		s.extra = func(ctx context.Context, name string) error {
			return s.runEncryption(ctx, name, *kmsKeyID)
		}
	case "replication":
		s.extra = func(ctx context.Context, name string) error {
			target := scenario{client: s.client, siteID: *targetSiteID}
			target.extra = func(ctx context.Context, targetName string) error {
				return s.runReplication(ctx, name, targetName, *targetSiteID)
			}
			if *createTargetAccount {
				return target.runAccountWith(ctx, func(ctx context.Context) error { return target.run(ctx, name+"-dest") })
			}
			return target.run(ctx, name+"-dest")
		}
	}
	var scenarioErr error
	switch {
	case *mode == "metrics" && !*confirmed:
		scenarioErr = s.runMetrics(ctx, *metricsBucket, *metricsID)
	case *mode == "account":
		scenarioErr = s.runAccount(ctx)
	case *accountConfirmed:
		scenarioErr = s.runAccountWith(ctx, func(ctx context.Context) error { return s.run(ctx, name) })
	default:
		scenarioErr = s.run(ctx, name)
	}
	if scenarioErr != nil {
		fmt.Fprintln(os.Stderr, "Object Storage E2E failed:", scenarioErr)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Object Storage E2E passed; scenario resources were verified, deleted, and confirmed absent.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
