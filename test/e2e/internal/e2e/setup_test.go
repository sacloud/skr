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

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateExecutable(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "skr")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil { // #nosec G306 -- テスト実行ファイルは実行可能である必要がある。
		t.Fatal(err)
	}
	got, err := ValidateExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.Abs(path); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestValidateExecutableMissingFile(t *testing.T) {
	if _, err := ValidateExecutable(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing binary")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

func TestValidateExecutableRejectsDirectory(t *testing.T) {
	if _, err := ValidateExecutable(t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory")
	} else if !strings.Contains(err.Error(), "executable built binary") {
		t.Fatalf("error = %v, want executable built binary", err)
	}
}

func TestValidateExecutableRejectsNonExecutableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skr")
	if err := os.WriteFile(path, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateExecutable(path); err == nil {
		t.Fatal("expected an error for a non-executable file")
	} else if !strings.Contains(err.Error(), "executable built binary") {
		t.Fatalf("error = %v, want executable built binary", err)
	}
}

func TestTempDir(t *testing.T) {
	dir, remove, err := TempDir("skr-e2e-test-*")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := remove(); err != nil {
		t.Fatalf("remove temporary directory: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}
