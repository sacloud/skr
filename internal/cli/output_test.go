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

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

func TestProfileOutputType(t *testing.T) {
	for _, test := range []struct {
		name       string
		filename   string
		contents   string
		wantFormat string
	}{
		{
			name:       "v1",
			filename:   "config.yaml",
			contents:   "version: 1\ncli:\n  default_output_type: table\n",
			wantFormat: "table",
		},
		{
			name:       "v0",
			filename:   "config.json",
			contents:   `{"DefaultOutputType":"yaml"}`,
			wantFormat: "yaml",
		},
		{
			name:       "unset",
			filename:   "config.yaml",
			contents:   "version: 1\ncli:\n  argument_match_mode: exact\n",
			wantFormat: "json",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			profileDir := t.TempDir()
			profilePath := filepath.Join(profileDir, "example")
			if err := os.Mkdir(profilePath, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profileDir, "current"), []byte("example\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profilePath, test.filename), []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}

			format, err := profileOutputType([]string{"SAKURA_PROFILE_DIR=" + profileDir})
			if err != nil {
				t.Fatal(err)
			}
			if format != test.wantFormat {
				t.Fatalf("profileOutputType() = %q, want %q", format, test.wantFormat)
			}
		})
	}
}

func TestIaaSSwitchOutputFormatAndProfileDefault(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	t.Setenv("COLUMNS", "100")
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	commandLine.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
		return swytch.New(server), nil
	})
	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", args, code, stderr.String())
		}
		return stdout.String()
	}

	var created iaas.Switch
	if err := json.Unmarshal([]byte(runCommand("iaas-api", "switch", "create", "--zone", "test-zone", "--name", "output-test")), &created); err != nil {
		t.Fatal(err)
	}
	yamlOutput := runCommand("iaas-api", "switch", "read", "--zone", "test-zone", "--id", created.ID.String(), "--output", "yaml")
	if !strings.Contains(yamlOutput, "Name: output-test") {
		t.Errorf("YAML output = %q, want Name field", yamlOutput)
	}

	profilePath := filepath.Join(profileDir, "example")
	if err := os.Mkdir(profilePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "current"), []byte("example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilePath, "config.yaml"), []byte("version: 1\ncli:\n  default_output_type: table\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profileTable := runCommand("iaas-api", "switch", "find", "--zone", "test-zone")
	if got := tableHeaderCells(profileTable); len(got) < 3 || strings.Join(got[:3], " ") != "ID Name Description" {
		t.Errorf("profile table headers start with %v, want ID Name Description", got[:min(3, len(got))])
	}

	jsonOutput := runCommand("iaas-api", "switch", "read", "--zone", "test-zone", "--id", created.ID.String(), "--output", "json")
	if !strings.HasPrefix(strings.TrimSpace(jsonOutput), "{") {
		t.Errorf("--output json returned %q, want a JSON object", jsonOutput)
	}
}

func TestWriteTableFitsTerminalWidthAndAddsBorders(t *testing.T) {
	t.Setenv("COLUMNS", "40")
	value := []map[string]any{{
		"ID":          json.Number("12345"),
		"Name":        "サーバー長い名前",
		"Description": "日本語の長い説明文が端末幅で省略されることを確認します",
		"Tags":        []string{"production", "database"},
		"CreatedAt":   "2026-10-05T12:00:00Z",
	}}
	var output bytes.Buffer
	if err := writeTable(&output, value, nil); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	for _, line := range lines {
		if got := displayWidth(line); got > 40 {
			t.Errorf("table line width = %d, want <= 40: %q", got, line)
		}
	}
	if !strings.HasPrefix(lines[0], "+") || !strings.Contains(lines[1], "|") || !strings.HasPrefix(lines[2], "+") {
		t.Fatalf("table output lacks visible borders:\n%s", output.String())
	}
	headers := tableHeaderCells(output.String())
	if len(headers) < 3 || strings.Join(headers[:3], " ") != "ID Name Description" {
		t.Errorf("table headers = %v, want ID Name Description first", headers)
	}
	if !strings.Contains(output.String(), "columns omitted") {
		t.Errorf("table output should indicate omitted columns:\n%s", output.String())
	}
}

func TestWriteTablePreservesLargeNumericIDs(t *testing.T) {
	var output bytes.Buffer
	value := []map[string]any{{"ID": json.Number("9007199254740993")}}
	if err := writeTable(&output, value, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "9007199254740993") {
		t.Errorf("table output altered a large numeric ID:\n%s", output.String())
	}
}

func TestWriteTableOmitsZeroTimestamps(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	value := []map[string]any{{
		"ID":         "123",
		"Name":       "switch",
		"CreatedAt":  "0001-01-01T00:00:00Z",
		"ModifiedAt": "0001-01-01T00:00:00Z",
	}}
	var output bytes.Buffer
	if err := writeTable(&output, value, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "0001-01-01") {
		t.Errorf("table output shows zero timestamp:\n%s", got)
	}
	if !strings.Contains(got, "ModifiedAt") {
		t.Errorf("table output does not contain ModifiedAt column:\n%s", got)
	}
}

func TestMarshalYAMLPreservesLargeNumbersAndJSONFieldNames(t *testing.T) {
	output, err := marshalYAML(map[string]any{
		"ID":   json.Number("9007199254740993"),
		"Name": "example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(output); !strings.Contains(got, "ID: 9007199254740993") || !strings.Contains(got, "Name: example") {
		t.Fatalf("YAML output = %q, want original field names and exact numeric ID", got)
	}
}

func tableHeaderCells(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil
	}
	cells := strings.Split(strings.Trim(lines[1], "|"), "|")
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}
	return cells
}
