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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordResultPassed(t *testing.T) {
	recorder := testRecorder(t, t.TempDir())
	exitCode := 0
	RecordResult(recorder, &exitCode)()
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	result, err := os.ReadFile(filepath.Join(recorder.Dir(), "RESULT.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(result), "Result: passed\n") {
		t.Fatalf("result = %q, want passed", result)
	}
}

func TestRecordResultFailed(t *testing.T) {
	recorder := testRecorder(t, t.TempDir())
	exitCode := 1
	RecordResult(recorder, &exitCode)()
	result, err := os.ReadFile(filepath.Join(recorder.Dir(), "RESULT.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(result), "Result: failed\n") {
		t.Fatalf("result = %q, want failed", result)
	}
}

func TestRecordResultReflectsWriteFailure(t *testing.T) {
	base := t.TempDir()
	recorder := testRecorder(t, base)
	// evidence ディレクトリを削除して結果の書き込みを失敗させ、終了コードへ反映されることを確認する。
	if err := os.RemoveAll(recorder.Dir()); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	RecordResult(recorder, &exitCode)()
	if exitCode == 0 {
		t.Fatal("write failure was not reflected in the exit code")
	}
}

func TestSignalContextCancels(t *testing.T) {
	ctx, stop := SignalContext()
	if ctx == nil || stop == nil {
		t.Fatal("SignalContext returned nil")
	}
	stop()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not canceled by stop")
	}
}
