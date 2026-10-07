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
	"encoding/json"
	"os"
	"path/filepath"
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
