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

const regionQuery = "map({ID,Name,Description,NameServers})"

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

type regionSummary struct {
	ID          int64
	Name        string
	Description string
	NameServers []string
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

	findOutput, err := s.client.call(ctx, "region-find", "iaas-api", "region", "find", "--query", regionQuery)
	if err != nil {
		return err
	}
	var regions []regionSummary
	if err := json.Unmarshal(findOutput, &regions); err != nil {
		return fmt.Errorf("decode Region find output: %w", err)
	}
	if len(regions) == 0 || regions[0].ID <= 0 || regions[0].Name == "" {
		return errors.New("Region find returned no item with a valid ID and name")
	}

	readOutput, err := s.client.call(ctx, "region-read", "iaas-api", "region", "read", "--id", fmt.Sprint(regions[0].ID), "--query", "{ID,Name,Description,NameServers}")
	if err != nil {
		return err
	}
	var read regionSummary
	if err := json.Unmarshal(readOutput, &read); err != nil {
		return fmt.Errorf("decode Region read output: %w", err)
	}
	if !sameRegion(read, regions[0]) {
		return fmt.Errorf("Region read result does not match Find result ID %d", regions[0].ID)
	}
	return nil
}

func sameRegion(left, right regionSummary) bool {
	if left.ID != right.ID || left.Name != right.Name || left.Description != right.Description || len(left.NameServers) != len(right.NameServers) {
		return false
	}
	for i := range left.NameServers {
		if left.NameServers[i] != right.NameServers[i] {
			return false
		}
	}
	return true
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-read-only-live", false, "Confirm read-only Region API requests using the selected SDK profile")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/region --skr ./skr --confirm-read-only-live")
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
	recorder, err := evidence.New("region-api")
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

	fmt.Println("This test only searches and reads Region data; it does not modify cloud resources.")
	fmt.Println("Review the selected SDK profile and credentials before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Region E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Region E2E passed; Find and Read returned matching Region data.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
