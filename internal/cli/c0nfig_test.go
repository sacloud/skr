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

package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

func TestRunConfigList(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "production", "staging"} {
		if err := profileOp.Create(&saclient.Profile{
			Name:       name,
			Attributes: map[string]any{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := profileOp.SetCurrentName("production"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "list"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	for _, name := range []string{"default", "production", "staging"} {
		if !strings.Contains(got, name) {
			t.Errorf("stdout = %q, want to contain %q", got, name)
		}
	}
	// stdout is a bytes.Buffer, so terminal detection is false and no marker is printed.
	if strings.Contains(got, "* ") {
		t.Errorf("stdout = %q, want no current-profile marker", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigListWithoutSelection(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name:       "default",
		Attributes: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "list"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "default\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigListWithUnreadableCurrentProfile(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	if err := os.Mkdir(filepath.Join(profileDir, "current"), 0o700); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "list"}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got == "" {
		t.Error("stderr is empty, want an error reading the current profile")
	}
}

func TestRunConfigListEmpty(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "list"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigShowCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name:       "production",
		Attributes: map[string]any{"zone": "is1a"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profileOp.SetCurrentName("production"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "show"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	var attrs map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &attrs); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if got, want := attrs["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigShowNamed(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "production"} {
		if err := profileOp.Create(&saclient.Profile{
			Name:       name,
			Attributes: map[string]any{"name": name},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := profileOp.SetCurrentName("default"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "show", "production"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	var attrs map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &attrs); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if got, want := attrs["name"], "production"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunConfigShowWithoutCurrent(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "show"}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "current") {
		t.Errorf("stderr = %q, want an error mentioning the current profile", got)
	}
}

func TestRunConfigShowUnknownProfile(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "show", "missing"}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "missing") {
		t.Errorf("stderr = %q, want an error mentioning the profile", got)
	}
}

func writeTestPrivateKey(t *testing.T, dir string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "key.pem")
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunConfigCreateNonInteractive(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
	if got := stdout.String(); !strings.Contains(got, "prod") || !strings.Contains(got, "created") {
		t.Errorf("stdout = %q, want creation message", got)
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profileOp.Read("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := profile.Attributes[attrServicePrincipalID], "spid-123"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrServicePrincipalKeyID], "kid-456"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], keyPath; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}

	current, err := profileOp.GetCurrentName()
	if err == nil {
		t.Errorf("current profile = %q, want no current profile set", current)
	}
}

func TestRunConfigCreateWithUse(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
		"--use",
	}
	if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	current, err := profileOp.GetCurrentName()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := current, "prod"; got != want {
		t.Errorf("current profile = %q, want %q", got, want)
	}
}

func TestRunConfigCreateInvalidName(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)
	baseArgs := []string{"--service-principal-id", "spid", "--service-principal-key-id", "kid", "--private-key-file", keyPath}

	for _, invalidName := range []string{".", "..", "a/b"} {
		t.Run(invalidName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"config", "create", invalidName}, baseArgs...)
			if exitCode := run(args, &stdout, &stderr); exitCode == 0 {
				t.Fatal("run() exit code = 0, want non-zero")
			}
			if got := stderr.String(); got == "" {
				t.Error("stderr is empty, want an error")
			}
		})
	}
}

func TestRunConfigCreateDefaultName(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profileOp.Read("default"); err != nil {
		t.Fatalf("read default profile: %v", err)
	}
}

func TestRunConfigCreateExistingProfileFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{Name: "prod", Attributes: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stderr.String(); !strings.Contains(got, "already exists") {
		t.Errorf("stderr = %q, want already exists error", got)
	}
}

func TestRunConfigCreateMissingFlagsFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing service principal id",
			args: []string{"config", "create", "prod", "--service-principal-key-id", "kid", "--private-key-file", keyPath},
		},
		{
			name: "missing service principal key id",
			args: []string{"config", "create", "prod", "--service-principal-id", "spid", "--private-key-file", keyPath},
		},
		{
			name: "missing private key file",
			args: []string{"config", "create", "prod", "--service-principal-id", "spid", "--service-principal-key-id", "kid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(tt.args, &stdout, &stderr); exitCode == 0 {
				t.Fatal("run() exit code = 0, want non-zero")
			}
			if got := stderr.String(); got == "" {
				t.Error("stderr is empty, want an error")
			}
		})
	}
}

func TestRunConfigCreateInvalidPrivateKeyFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := filepath.Join(profileDir, "invalid.pem")
	if err := os.WriteFile(keyPath, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stderr.String(); !strings.Contains(got, "private key") {
		t.Errorf("stderr = %q, want private key error", got)
	}
}

func TestRunConfigCreateInsecurePrivateKeyPermissionsFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private key permission validation is not supported on Windows")
	}

	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)
	if err := os.Chmod(keyPath, 0o644); err != nil { // #nosec G302 -- Lax permissions are intentional; this test verifies the CLI rejects insecure private key files.
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stderr.String(); !strings.Contains(got, "permissions are too lax") {
		t.Errorf("stderr = %q, want private key permission error", got)
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profileOp.Read("prod"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read profile error = %v, want os.ErrNotExist", err)
	}
}

func TestRunConfigEditNonInteractive(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	oldKey := writeTestPrivateKey(t, profileDir)
	newKey := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name: "prod",
		Attributes: map[string]any{
			attrServicePrincipalID:    "old-spid",
			attrServicePrincipalKeyID: "old-kid",
			attrPrivateKeyPEMPath:     oldKey,
			"zone":                    "is1a",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profileOp.SetCurrentName("prod"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "edit", "prod",
		"--service-principal-key-id", "new-kid",
		"--private-key-file", newKey,
	}
	if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	profile, err := profileOp.Read("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := profile.Attributes[attrServicePrincipalID], "old-spid"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrServicePrincipalKeyID], "new-kid"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], newKey; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}
	if got, want := profile.Attributes["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
}

func TestRunConfigEditInsecurePrivateKeyPermissionsFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private key permission validation is not supported on Windows")
	}

	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	oldKey := writeTestPrivateKey(t, profileDir)
	newKey := filepath.Join(profileDir, "insecure.pem")
	if err := os.WriteFile(newKey, []byte("not used"), 0o644); err != nil { // #nosec G306 -- Lax permissions are intentional; this test verifies the CLI rejects insecure private key files.
		t.Fatal(err)
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	attributes := map[string]any{
		attrServicePrincipalID:    "old-spid",
		attrServicePrincipalKeyID: "old-kid",
		attrPrivateKeyPEMPath:     oldKey,
	}
	if err := profileOp.Create(&saclient.Profile{Name: "prod", Attributes: attributes}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"config", "edit", "prod", "--private-key-file", newKey}
	if exitCode := run(args, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stderr.String(); !strings.Contains(got, "permissions are too lax") {
		t.Errorf("stderr = %q, want private key permission error", got)
	}

	profile, err := profileOp.Read("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], oldKey; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}
}

func TestRunConfigEditNoFlagsFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name:       "prod",
		Attributes: map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profileOp.SetCurrentName("prod"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "edit"}, &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() exit code = 0, want non-zero")
	}
	if got := stderr.String(); got == "" {
		t.Error("stderr is empty, want an error")
	}
}

func TestRunConfigEditUseSwitchesCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "prod"} {
		attrs := map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath}
		if err := profileOp.Create(&saclient.Profile{Name: name, Attributes: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	if err := profileOp.SetCurrentName("default"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"config", "edit", "prod", "--service-principal-id", "new-spid", "--use"}
	if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	current, err := profileOp.GetCurrentName()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := current, "prod"; got != want {
		t.Errorf("current profile = %q, want %q", got, want)
	}
}

func TestRunConfigEditUseSwitchesCurrentWithoutChanges(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "prod"} {
		attrs := map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath}
		if err := profileOp.Create(&saclient.Profile{Name: name, Attributes: attrs}); err != nil {
			t.Fatal(err)
		}
	}
	if err := profileOp.SetCurrentName("default"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "edit", "prod", "--use"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	current, err := profileOp.GetCurrentName()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := current, "prod"; got != want {
		t.Errorf("current profile = %q, want %q", got, want)
	}
}

func TestRunConfigShowMasksSensitiveAttributes(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if err := profileOp.Create(&saclient.Profile{
		Name: "prod",
		Attributes: map[string]any{
			"zone":                    "is1a",
			attrServicePrincipalID:    "spid-123",
			attrServicePrincipalKeyID: "kid-456",
			attrPrivateKeyPEMPath:     keyPath,
			"AccessToken":             "token",
			"AccessTokenSecret":       "secret",
			"PrivateKey":              "pem-content",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profileOp.SetCurrentName("prod"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"config", "show"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	var attrs map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &attrs); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if got, want := attrs["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
	if got, want := attrs[attrServicePrincipalID], "spid-123"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := attrs[attrServicePrincipalKeyID], "kid-456"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	for _, key := range []string{"AccessToken", "AccessTokenSecret", "PrivateKey", attrPrivateKeyPEMPath} {
		if got := attrs[key]; got != "(masked)" {
			t.Errorf("%s = %q, want masked", key, got)
		}
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}
