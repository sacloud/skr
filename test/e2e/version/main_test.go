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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

func TestCheckVersionOutput(t *testing.T) {
	for _, test := range []struct {
		output  string
		wantErr bool
	}{
		{"v0.0.4 linux/arm64, build xxxxxx\n", false},
		{"v1.2.3 darwin/amd64, build abc1234", false},
		{"v0.0.4 windows/amd64, build deadbeef\n", false},
		{"", true},
		{"skr v0.0.4\n", true},
		{"v0.0.4\n", true},
		{"v0.0 linux/arm64, build x\n", true},
		{"version v0.0.4 linux/arm64, build x\n", true},
	} {
		err := checkVersionOutput(test.output)
		if (err != nil) != test.wantErr {
			t.Errorf("checkVersionOutput(%q) error = %v, wantErr %v", test.output, err, test.wantErr)
		}
	}
}

func fakeBinary(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-skr")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVersionRunnerRecordsEvidenceAndVerifiesOutput(t *testing.T) {
	binary := fakeBinary(t, "#!/bin/sh\necho 'v0.0.4 linux/arm64, build test'\n")
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	test := runner{binary: binary, evidence: recorder}
	if err := test.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // path is a fixed filename in this test's evidence directory.
	data, err := os.ReadFile(filepath.Join(recorder.Dir(), "001-version-print.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"\"step\": \"version-print\"", "\"exit_status\": 0", "v0.0.4 linux/arm64, build test"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("evidence does not contain %q: %s", expected, data)
		}
	}
}

func TestVersionRunnerReportsFailures(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		binary := fakeBinary(t, "#!/bin/sh\necho 'boom' >&2\nexit 3\n")
		recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		test := runner{binary: binary, evidence: recorder}
		err = test.run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "skr version failed") {
			t.Fatalf("error = %v, want command failure", err)
		}
		//nolint:gosec // path is a fixed filename in this test's evidence directory.
		data, err := os.ReadFile(filepath.Join(recorder.Dir(), "001-version-print.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{"\"exit_status\": 3", "boom"} {
			if !strings.Contains(string(data), expected) {
				t.Errorf("evidence does not contain %q: %s", expected, data)
			}
		}
	})
	t.Run("unexpected output", func(t *testing.T) {
		binary := fakeBinary(t, "#!/bin/sh\necho 'not-a-version'\n")
		recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		test := runner{binary: binary, evidence: recorder}
		err = test.run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("error = %v, want output check failure", err)
		}
	})
}
