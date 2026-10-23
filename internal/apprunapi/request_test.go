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

package apprunapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun/apis/v1"
)

const validCreateRequest = `{"name":"test","timeout_seconds":60,"port":8080,"min_scale":0,"max_scale":1,"components":[{"name":"web","max_cpu":"0.5","max_memory":"1Gi","deploy_source":{"container_registry":{"image":"example.invalid/image:latest"}}}]}`

func TestDecodeRequestSources(t *testing.T) {
	path := writeRequestFile(t, validCreateRequest)
	for _, input := range []string{validCreateRequest, "@" + path, "-"} {
		t.Run(input, func(t *testing.T) {
			if input == "-" {
				setStdin(t, validCreateRequest)
			}
			var request v1.CreateApplicationBody
			if err := DecodeRequest(input, &request); err != nil {
				t.Fatal(err)
			}
			if request.Name != "test" || len(request.Components) != 1 {
				t.Fatalf("decoded request = %#v", request)
			}
		})
	}
}

func TestDecodeRequestPreservesUpdateOmission(t *testing.T) {
	var omitted v1.PatchApplicationBody
	if err := DecodeRequest(`{}`, &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.MaxScale.IsSet() {
		t.Fatal("omitted max_scale was marked as set")
	}
	var explicitZero v1.PatchApplicationBody
	if err := DecodeRequest(`{"max_scale":0}`, &explicitZero); err != nil {
		t.Fatal(err)
	}
	if got, ok := explicitZero.MaxScale.Get(); !ok || got != 0 {
		t.Fatalf("max_scale = %d, %t; want explicitly set zero", got, ok)
	}
}

func TestDecodeRequestIgnoresUnknownFields(t *testing.T) {
	var request v1.PatchApplicationBody
	if err := DecodeRequest(`{"max_scale":1,"future_field":"ignored"}`, &request); err != nil {
		t.Fatal(err)
	}
	if got, ok := request.MaxScale.Get(); !ok || got != 1 {
		t.Fatalf("max_scale = %d, %t; want 1", got, ok)
	}
}

func TestDecodeRequestErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		want    string
		wrapped error
		stdin   bool
	}{
		{name: "empty file path", input: "@", want: "request file path is empty"},
		{name: "missing file", input: "@" + filepath.Join(t.TempDir(), "missing.json"), want: "read request file", wrapped: os.ErrNotExist},
		{name: "malformed file", input: "@" + writeRequestFile(t, "{"), want: "decode request JSON"},
		{name: "trailing JSON", input: `{} {}`, want: "decode request JSON"},
		{name: "wrong field type", input: `{"port":"8080"}`, want: "decode request JSON"},
		{name: "stdin read failure", input: "-", want: "read request from standard input", stdin: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.stdin {
				file, err := os.Open(writeRequestFile(t, "{}"))
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				previous := os.Stdin
				os.Stdin = file
				t.Cleanup(func() { os.Stdin = previous })
			}
			var request v1.CreateApplicationBody
			err := DecodeRequest(test.input, &request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeRequest() error = %v, want %q", err, test.want)
			}
			if test.wrapped != nil && !errors.Is(err, test.wrapped) {
				t.Fatalf("DecodeRequest() error = %v, want wrapped %v", err, test.wrapped)
			}
		})
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

func setStdin(t *testing.T, content string) {
	t.Helper()
	file, err := os.Open(writeRequestFile(t, content))
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = previous
		_ = file.Close()
	})
}
