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

package serverapi

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadUserData(t *testing.T) {
	userData := "#cloud-config\nssh_authorized_keys:\n  - ssh-ed25519 AAAA-example\n"
	path := filepath.Join(t.TempDir(), "user-data.yaml")
	if err := os.WriteFile(path, []byte(userData), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   string
		stdin   io.Reader
		want    string
		wantErr string
	}{
		{name: "file", input: "@" + path, want: userData},
		{name: "standard input", input: "-", stdin: strings.NewReader(userData), want: userData},
		{name: "inline rejected", input: userData, wantErr: "--user-data は @path.yaml または -"},
		{name: "empty input", input: "-", stdin: strings.NewReader(" \n"), wantErr: "user-data is empty"},
		{name: "missing file", input: "@missing", wantErr: "read user-data file"},
		{name: "empty file path", input: "@", wantErr: "--user-data は @path.yaml または -"},
		{name: "stdin read failure", input: "-", stdin: errorReader{}, wantErr: "read user-data from standard input"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readUserData(test.input, test.stdin)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("readUserData() error = %v, want containing %q", err, test.wantErr)
				}
				if strings.Contains(err.Error(), "AAAA-example") {
					t.Fatalf("readUserData() error leaked user-data: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("readUserData() error = %v", err)
			}
			if got != test.want {
				t.Errorf("readUserData() = %q, want %q", got, test.want)
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
