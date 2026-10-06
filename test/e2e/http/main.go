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

const zoneListURL = "https://secure.sakura.ad.jp/cloud/zone/is1a/api/cloud/1.1/zone"

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

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The binary is an explicitly selected local skr executable; arguments are structured, not a shell command.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	if err := r.evidence.Record(step, args, nil, stdout.String(), stderr.String(), runErr, false); err != nil {
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

	body, err := s.client.call(ctx, "http-get-zones", "http", zoneListURL, "--method", "GET")
	if err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return errors.New("get_zone response is empty or is not valid JSON")
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-http-live", false, "Confirm a read-only GET request to secure.sakura.ad.jp using the selected SDK profile")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/http --skr ./skr --confirm-http-live")
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
	recorder, err := evidence.New("http-api")
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

	fmt.Println("This test sends one read-only GET request to the Sakura Cloud zone-list API.")
	fmt.Println("Review the selected SDK profile and credentials before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	if err := (scenario{client: runner}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "HTTP E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("HTTP E2E passed; the zone-list response is valid JSON.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
