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

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	serverPlanZone  = "is1b"
	serverPlanQuery = ".ServerPlans | map({ID,Name,CPU,MemoryMB,GPU,CPUModel,GPUModel,Commitment,Generation,ConfidentialVM,Availability})"
)

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
	timeout, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The selected binary and structured arguments are not a shell command.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, nil, stdout.String(), stderr.String(), runErr, false); err != nil {
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

type serverPlanSummary struct {
	ID             int64
	Name           string
	CPU            int
	MemoryMB       int
	GPU            int
	CPUModel       string
	GPUModel       string
	Commitment     string
	Generation     int
	ConfidentialVM bool
	Availability   string
}

type scenario struct {
	client cli
}

func (s scenario) run(ctx context.Context) error {
	profile, err := s.client.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}

	output, err := s.client.call(ctx, "serverplan-find", "iaas-api", "product", "server", "find",
		"--zone", serverPlanZone, "--count", "1", "--from", "0", "--query", serverPlanQuery)
	if err != nil {
		return err
	}
	var plans []serverPlanSummary
	if err := json.Unmarshal(output, &plans); err != nil {
		return fmt.Errorf("decode ServerPlan find output: %w", err)
	}
	if len(plans) != 1 {
		return fmt.Errorf("ServerPlan find returned %d records, want one", len(plans))
	}
	plan := plans[0]
	if plan.ID <= 0 || plan.Name == "" {
		return errors.New("ServerPlan result is missing its ID or name")
	}

	output, err = s.client.call(ctx, "serverplan-read", "iaas-api", "product", "server", "read",
		"--zone", serverPlanZone, "--id", fmt.Sprint(plan.ID))
	if err != nil {
		return err
	}
	var read serverPlanSummary
	if err := json.Unmarshal(output, &read); err != nil {
		return fmt.Errorf("decode ServerPlan read output: %w", err)
	}
	if read.ID != plan.ID || read.Name != plan.Name || read.CPU != plan.CPU ||
		read.MemoryMB != plan.MemoryMB || read.GPU != plan.GPU {
		return fmt.Errorf("ServerPlan read did not match find result for ID %d", plan.ID)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-read-only-live", false, "Confirm read-only ServerPlan API requests using the selected SDK profile")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/serverplan --skr ./skr --confirm-read-only-live")
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
	recorder, err := evidence.New("serverplan-api", evidence.Scenario{
		Title:       "サーバープランの検索・参照",
		Description: "IaaS API でサーバープランの検索・参照が正しく動作することを確認します。",
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

	fmt.Printf("This test only searches and reads ServerPlan records in %s; it does not modify cloud resources.\n", serverPlanZone)
	fmt.Println("Review the selected SDK profile and credentials before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ServerPlan E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Printf("ServerPlan E2E passed; a plan returned by Find matched Read in %s.\n", serverPlanZone)
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
