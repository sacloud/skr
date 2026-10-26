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
)

func TestWriteRequest(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteRequest(dir, "find", map[string]any{"Names": []string{"target"}})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "find.json") {
		t.Fatalf("path = %q, want %q", path, filepath.Join(dir, "find.json"))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %04o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path) //nolint:gosec // 直前に書き込まれたプライベートな一時ファイルのみを読み込む。
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"Names\":[\"target\"]}\n" {
		t.Fatalf("content = %q, want JSON object with trailing newline", data)
	}
}

func TestWriteRequestEncodeError(t *testing.T) {
	_, err := WriteRequest(t.TempDir(), "bad", map[string]any{"channel": make(chan int)})
	if err == nil {
		t.Fatal("expected an encode error for an unsupported value")
	}
	if !strings.Contains(err.Error(), "encode bad request") {
		t.Fatalf("error = %v, want encode failure", err)
	}
}
