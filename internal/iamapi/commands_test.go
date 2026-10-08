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

package iamapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeRequestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memberships.json")
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o600); err != nil {
		t.Fatal(err)
	}
	var value []int
	if err := DecodeRequest("@"+path, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 3 || value[0] != 1 || value[2] != 3 {
		t.Fatalf("decoded %#v, want [1 2 3]", value)
	}
	if err := DecodeRequest("@/missing/memberships.json", &value); err == nil {
		t.Error("DecodeRequest(@missing) succeeded, want an error")
	}
	if err := DecodeRequest("{invalid", &value); err == nil {
		t.Error("DecodeRequest(invalid JSON) succeeded, want an error")
	}
}

func TestReadTextInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assertion.txt")
	if err := os.WriteFile(path, []byte("  assertion-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := readTextInput(path, "--assertion-file", "assertion")
	if err != nil {
		t.Fatal(err)
	}
	if value != "assertion-value" {
		t.Fatalf("readTextInput() = %q, want trimmed assertion", value)
	}
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "", want: "--assertion-file が必要です"},
		{path: filepath.Join(t.TempDir(), "missing.txt"), want: "read assertion file"},
	} {
		if _, err := readTextInput(test.path, "--assertion-file", "assertion"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("readTextInput(%q) error = %v, want %q", test.path, err, test.want)
		}
	}
	emptyPath := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(emptyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTextInput(emptyPath, "--assertion-file", "assertion"); err == nil || err.Error() != "assertion is empty" {
		t.Errorf("readTextInput(empty) error = %v, want assertion is empty", err)
	}
}
