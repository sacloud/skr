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

package containerregistryapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeRequestInlineAndFile(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
	}{
		{"inline", `{"Name":"registry"}`},
		{"file", "@" + writeRequestFile(t, `{"Name":"registry"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var request RegistryCreateInput
			if err := DecodeRequest(test.input, &request); err != nil {
				t.Fatal(err)
			}
			if request.Name != "registry" {
				t.Fatalf("request Name = %q", request.Name)
			}
		})
	}
}

func TestDecodeRequestRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	for _, input := range []string{
		`{"Name":"registry","Users":[{"Password":"secret"}]}`,
		`{"Name":"registry"} {}`,
	} {
		var request RegistryCreateInput
		if err := DecodeRequest(input, &request); err == nil {
			t.Fatalf("DecodeRequest(%q) succeeded", input)
		}
	}
}

func TestReadPasswordFileSupportsFileAndStdinMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("secret-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-password" {
		t.Fatalf("ReadPasswordFile() = %q", got)
	}
	if _, err := ReadPasswordFile(filepath.Join(t.TempDir(), "missing")); err == nil || strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("missing file error = %v", err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdin := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previousStdin
		if err := reader.Close(); err != nil {
			t.Errorf("close standard input pipe: %v", err)
		}
	})
	if _, err := writer.Write([]byte("stdin-password\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got, err = ReadPasswordFile("-")
	if err != nil {
		t.Fatal(err)
	}
	if got != "stdin-password" {
		t.Fatalf("ReadPasswordFile(-) = %q", got)
	}
}

func TestValidateRegistryName(t *testing.T) {
	for _, name := range []string{"a", "registry-01", "z9"} {
		if err := validateRegistryName(name); err != nil {
			t.Errorf("validateRegistryName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "Uppercase", "-registry", "registry-", "registry_name"} {
		if err := validateRegistryName(name); err == nil {
			t.Errorf("validateRegistryName(%q) succeeded", name)
		}
	}
}

func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
