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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

// scriptBinary はテスト用の実行可能スクリプトを作成し、パスを返します。
func scriptBinary(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "skr")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil { // #nosec G306 -- テスト実行ファイルは実行可能である必要がある。
		t.Fatal(err)
	}
	return path
}

// testRecorder はテスト用の evidence Recorder を作成します。
func testRecorder(t *testing.T, base string) *evidence.Recorder {
	t.Helper()
	recorder, err := evidence.CreateAt(filepath.Join(base, "evidence"), time.Now(), evidence.Scenario{})
	if err != nil {
		t.Fatal(err)
	}
	return recorder
}

func TestRunnerRecordsStepAndDescription(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf 'ok\\n'\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder)
	output, err := runner.Call(context.Background(), "e2e-check", "動作確認ステップ", "config", "current")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "ok\n" {
		t.Fatalf("output = %q, want %q", output, "ok\n")
	}
	if err := recorder.SetResult("passed", time.Now()); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "動作確認ステップ") {
		t.Errorf("report is missing the step description:\n%s", report)
	}
}

func TestRunnerCallSubcommandPrefixesArgs(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf '%s\\n' \"$@\"\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder, WithPrefix("container-registry-api"))
	output, err := runner.CallSubcommand(context.Background(), "e2e-list", "一覧取得", "registry", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "container-registry-api\nregistry\nlist\n"
	if string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func TestRunnerCallDoesNotPrefixArgs(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf '%s\\n' \"$@\"\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder, WithPrefix("container-registry-api"))
	output, err := runner.Call(context.Background(), "e2e-current", "current 確認", "config", "current")
	if err != nil {
		t.Fatal(err)
	}
	want := "config\ncurrent\n"
	if string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func TestRunnerSetsEnv(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf '%s\\n' \"$E2E_TEST_VALUE\"\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder, WithEnv("E2E_TEST_VALUE=hello"))
	output, err := runner.Call(context.Background(), "e2e-env", "環境変数の確認")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello\n" {
		t.Fatalf("output = %q, want %q", output, "hello\n")
	}
}

func TestRunnerRejectsInvalidStep(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf 'must not run\\n'\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder)
	if _, err := runner.Call(context.Background(), "invalid/step", "不正なステップ名"); err == nil {
		t.Fatal("expected an error for an invalid step")
	} else if !strings.Contains(err.Error(), "invalid evidence step") {
		t.Fatalf("error = %v, want invalid evidence step", err)
	}
}

func TestRunnerFailsWhenBinaryFails(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf 'error output' >&2\nexit 3\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder)
	_, err := runner.Call(context.Background(), "e2e-fail", "失敗するステップ")
	if err == nil {
		t.Fatal("expected an error when the binary fails")
	}
	if !strings.Contains(err.Error(), "skr failed") {
		t.Fatalf("error = %v, want skr failed", err)
	}
	entries, err := os.ReadDir(recorder.Dir())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "e2e-fail.json") {
			found = true
			data, err := os.ReadFile(filepath.Join(recorder.Dir(), entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), `"exit_status": 3`) {
				t.Errorf("evidence is missing exit status 3:\n%s", data)
			}
		}
	}
	if !found {
		t.Fatalf("evidence was not recorded: %v", entries)
	}
}

func TestRunnerTimesOut(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "sleep 5\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder, WithTimeout(100*time.Millisecond))
	if _, err := runner.Call(context.Background(), "e2e-timeout", "タイムアウト確認"); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestRunnerRecordsRequestFileContents(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf 'ok\\n'\n")
	recorder := testRecorder(t, base)

	requestPath := filepath.Join(base, "create.json")
	request := "{\n  \"Name\": \"skr-e2e-test\"\n}\n"
	if err := os.WriteFile(requestPath, []byte(request), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(binary, recorder)
	if _, err := runner.Call(context.Background(), "e2e-create", "リクエストファイルを記録する",
		"registry", "create", "--request", "@"+requestPath); err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetResult("passed", time.Now()); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(recorder.Dir(), "REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{requestPath, `"Name": "skr-e2e-test"`} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
}

func TestRunnerReturnsFileReadError(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf 'must not run\\n'\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder)
	if _, err := runner.Call(context.Background(), "e2e-create", "存在しないリクエストファイル",
		"registry", "create", "--request", "@"+filepath.Join(base, "missing.json")); err == nil {
		t.Fatal("expected an error when the request file cannot be read")
	} else if !strings.Contains(err.Error(), "read request file for evidence") {
		t.Fatalf("error = %v, want request file read failure", err)
	}
}

func TestRunnerEnvFallbackToProcessEnv(t *testing.T) {
	base := t.TempDir()
	binary := scriptBinary(t, base, "printf '%s\\n' \"$PATH\"\n")
	recorder := testRecorder(t, base)

	runner := NewRunner(binary, recorder)
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	output, err := runner.Call(context.Background(), "e2e-path", "PATH 環境変数が引き継がれていることを確認する")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(output)) == "" {
		t.Fatal("PATH was not inherited from the process environment")
	}
}
