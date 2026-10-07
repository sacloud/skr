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
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	env      []string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The binary is an explicitly selected local skr executable; arguments are structured, not a shell command.
	command.Env = append(os.Environ(), r.env...)
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

func writePrivateKey(dir string) (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate RSA key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	path := filepath.Join(dir, "key.pem")
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}
	return path, nil
}

type scenario struct {
	client            cli
	keyPath           string
	profileName       string
	secondProfileName string
}

func (s scenario) run(ctx context.Context) error {
	_, err := s.client.call(ctx, "c0nfig-create",
		"config", "create", s.profileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", s.keyPath,
		"--use",
	)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}

	current, err := s.client.call(ctx, "c0nfig-current", "config", "current")
	if err != nil {
		return fmt.Errorf("get current profile: %w", err)
	}
	if strings.TrimSpace(string(current)) != s.profileName {
		return fmt.Errorf("current profile = %q, want %q", string(current), s.profileName)
	}

	_, err = s.client.call(ctx, "c0nfig-create-second",
		"config", "create", s.secondProfileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", s.keyPath,
	)
	if err != nil {
		return fmt.Errorf("create second profile: %w", err)
	}

	_, err = s.client.call(ctx, "c0nfig-use",
		"config", "use", s.secondProfileName,
	)
	if err != nil {
		return fmt.Errorf("use profile: %w", err)
	}

	current, err = s.client.call(ctx, "c0nfig-current-after-use", "config", "current")
	if err != nil {
		return fmt.Errorf("get current profile after use: %w", err)
	}
	if strings.TrimSpace(string(current)) != s.secondProfileName {
		return fmt.Errorf("current profile after use = %q, want %q", string(current), s.secondProfileName)
	}

	list, err := s.client.call(ctx, "c0nfig-list", "config", "list")
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	for _, name := range []string{s.profileName, s.secondProfileName} {
		if !strings.Contains(string(list), name) {
			return fmt.Errorf("profile list does not contain %q: %q", name, string(list))
		}
	}

	show, err := s.client.call(ctx, "c0nfig-show", "config", "show", s.profileName)
	if err != nil {
		return fmt.Errorf("show profile: %w", err)
	}
	attrs, err := parseProfileAttributes(show)
	if err != nil {
		return fmt.Errorf("decode show output: %w", err)
	}
	if err := assertProfileAttribute(attrs, "ServicePrincipalID", "spid-e2e"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}
	if err := assertProfileAttribute(attrs, "ServicePrincipalKeyID", "kid-e2e"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}
	if err := assertProfileAttribute(attrs, "PrivateKeyPEMPath", "(masked)"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}

	_, err = s.client.call(ctx, "c0nfig-edit",
		"config", "edit", s.profileName,
		"--service-principal-id", "spid-edited",
	)
	if err != nil {
		return fmt.Errorf("edit profile: %w", err)
	}

	showAfterEdit, err := s.client.call(ctx, "c0nfig-show-after-edit", "config", "show", s.profileName)
	if err != nil {
		return fmt.Errorf("show edited profile: %w", err)
	}
	attrsAfterEdit, err := parseProfileAttributes(showAfterEdit)
	if err != nil {
		return fmt.Errorf("decode edited show output: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "ServicePrincipalID", "spid-edited"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "ServicePrincipalKeyID", "kid-e2e"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "PrivateKeyPEMPath", "(masked)"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}

	return nil
}

func parseProfileAttributes(data []byte) (map[string]any, error) {
	var attrs map[string]any
	if err := json.Unmarshal(data, &attrs); err != nil {
		return nil, err
	}
	return attrs, nil
}

func assertProfileAttribute(attrs map[string]any, key, want string) error {
	got, ok := attrs[key].(string)
	if !ok {
		return fmt.Errorf("attribute %q is not a string", key)
	}
	if got != want {
		return fmt.Errorf("attribute %q = %q, want %q", key, got, want)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-config-live", false, "Confirm creating and editing a profile in a temporary profile directory")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/c0nfig --skr ./skr --confirm-config-live")
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

	profileDir, err := os.MkdirTemp("", "skr-e2e-config-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temporary profile directory:", err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(profileDir); err != nil {
			fmt.Fprintln(os.Stderr, "remove temporary profile directory:", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}()

	keyPath, err := writePrivateKey(profileDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "write test private key:", err)
		return 1
	}

	recorder, err := evidence.New("c0nfig")
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

	fmt.Println("This test uses a temporary profile directory and does not overwrite existing profiles.")
	fmt.Println("Profile directory:", profileDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{
		binary:   path,
		env:      []string{"SAKURA_PROFILE_DIR=" + profileDir},
		evidence: recorder,
	}
	if err := (scenario{
		client:            runner,
		keyPath:           keyPath,
		profileName:       "skr-e2e-config",
		secondProfileName: "skr-e2e-config-second",
	}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Config E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Config E2E passed; profiles were created, switched, shown, edited, and listed in isolation.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
