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

// The version runner verifies the skr version command against a built binary.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

// commandTimeout bounds a single skr version invocation.
const commandTimeout = 30 * time.Second

// versionPattern matches "v<semver> <os>/<arch>, build <revision>".
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+ \S+/\S+, build \S+$`)

func checkVersionOutput(output string) error {
	line := strings.TrimSpace(output)
	if !versionPattern.MatchString(line) {
		return fmt.Errorf("version output %q does not match \"v<semver> <os>/<arch>, build <revision>\"", line)
	}
	return nil
}

type runner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r runner) run(ctx context.Context) error {
	timeout, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, "version") //nolint:gosec // The binary path is validated in runMain.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record("version-print", r.binary, []string{"version"}, nil,
		stdout.String(), stderr.String(), runErr, false); err != nil {
		return errors.Join(fmt.Errorf("record version evidence: %w", err), runErr)
	}
	if runErr != nil {
		return fmt.Errorf("skr version failed: %w (see evidence for output)", runErr)
	}
	return checkVersionOutput(stdout.String())
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-version-live", false, "Confirm running the real skr version command")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/version --skr ./skr --confirm-version-live")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	recorder, err := evidence.New("version")
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
		if exitCode == 0 {
			fmt.Println("Version E2E passed; skr version printed the version information and exited successfully.")
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	test := runner{binary: path, evidence: recorder}
	if err := test.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Version E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	return 0
}

func main() {
	os.Exit(runMain())
}
