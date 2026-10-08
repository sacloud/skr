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

package evidence

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAtUsesTimestampAndAvoidsOverwrite(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "tmp", "eventbus-api")
	createdAt := time.Date(2026, 10, 5, 17, 1, 0, 0, time.Local)

	first, err := CreateAt(baseDir, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateAt(baseDir, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(first.Dir()), "202610051701"; got != want {
		t.Errorf("first evidence directory = %q, want %q", got, want)
	}
	if got, want := filepath.Base(second.Dir()), "202610051701-02"; got != want {
		t.Errorf("second evidence directory = %q, want %q", got, want)
	}
	for _, path := range []string{first.Dir(), second.Dir()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("evidence directory permissions = %o, want 700", got)
		}
	}
}

func TestRecorderOrdersAndRedactsEvidence(t *testing.T) {
	recorder, err := CreateAt(filepath.Join(t.TempDir(), "tmp", "simplemq-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record("profile-current", "./skr", []string{"config", "current"}, nil, "test-profile\n", "", nil, false); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record("rotate-api-key", "./skr", []string{"rotate-api-key"}, nil, `{"APIKey":"secret"}`, "", nil, true); err != nil {
		t.Fatal(err)
	}

	recordPath := filepath.Join(recorder.Dir(), "002-rotate-api-key.json")
	record, err := os.ReadFile(recordPath) //nolint:gosec // The path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(record), "secret") || !strings.Contains(string(record), "[REDACTED: APIKey]") {
		t.Fatalf("evidence did not redact API key: %s", record)
	}
	info, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("evidence file permissions = %o, want 600", got)
	}
	order, err := os.ReadFile(filepath.Join(recorder.Dir(), "ORDER.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"001\tprofile-current\tok\t001-profile-current.json",
		"002\trotate-api-key\tok\t002-rotate-api-key.json",
	} {
		if !strings.Contains(string(order), want) {
			t.Errorf("evidence order is missing %q:\n%s", want, order)
		}
	}
	if err := recorder.SetResult("passed", time.Now()); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(report), "secret") || !strings.Contains(string(report), "[REDACTED: APIKey]") {
		t.Fatalf("report did not preserve API key redaction: %s", report)
	}
}

func TestSetResultWritesPrivateResult(t *testing.T) {
	recorder, err := CreateAt(filepath.Join(t.TempDir(), "tmp", "switch-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	completed := time.Date(2026, 10, 5, 17, 1, 0, 0, time.Local)
	if err := recorder.SetResult("passed", completed); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(recorder.Dir(), "RESULT.txt")
	data, err := os.ReadFile(resultPath) //nolint:gosec // The path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Result: passed") || !strings.Contains(string(data), completed.Format(time.RFC3339)) {
		t.Fatalf("unexpected result file: %s", data)
	}
	info, err := os.Stat(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("result file permissions = %o, want 600", got)
	}
	reportInfo, err := os.Stat(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := reportInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("report file permissions = %o, want 600", got)
	}
}

func TestReportIncludesCommandsOutputAndExitStatus(t *testing.T) {
	recorder, err := CreateAt(filepath.Join(t.TempDir(), "tmp", "http-api"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record("http-get", "/path/to/skr", []string{"http", "--query", "map({ID,Name})"}, nil, "line 1\n```\n", "", nil, false); err != nil {
		t.Fatal(err)
	}
	failure := exec.Command(os.Args[0], "-test.run=TestEvidenceExitStatusHelper") //nolint:gosec // The test binary is invoked with a fixed test selector; no user input reaches the command.
	failure.Env = append(os.Environ(), "EVIDENCE_EXIT_STATUS_HELPER=1")
	runErr := failure.Run()
	if runErr == nil {
		t.Fatal("helper process succeeded, want exit status 7")
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("helper error = %T, want *exec.ExitError", runErr)
	}
	if err := recorder.Record("http-failure", "/path/to/skr", []string{"http", "url with spaces"}, nil, "partial output", "request failed\n", runErr, false); err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetResult("failed", time.Date(2026, 10, 5, 17, 1, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| 001 | `http-get` | 成功 | 0 |",
		"| 002 | `http-failure` | 失敗 | 7 |",
		"/path/to/skr http --query 'map({ID,Name})'",
		"/path/to/skr http 'url with spaces'",
		"````text\nline 1\n```\n````",
		"partial output",
		"request failed",
		"exit status 7",
		"## 001 http-get — 成功 (exit status 0)",
	} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(string(report), "<details>") || strings.Contains(string(report), "<summary>") {
		t.Fatalf("report should display all command details without expansion: %s", report)
	}
}

func TestExitStatusUnavailableForSignalTermination(t *testing.T) {
	if got := exitStatus(-1); got != nil {
		t.Fatalf("exitStatus(-1) = %d, want unavailable", *got)
	}
	if got := exitStatus(0); got == nil || *got != 0 {
		t.Fatalf("exitStatus(0) = %v, want 0", got)
	}
}

func TestEvidenceExitStatusHelper(t *testing.T) {
	if os.Getenv("EVIDENCE_EXIT_STATUS_HELPER") == "1" {
		os.Exit(7)
	}
}
