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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
	"github.com/stretchr/testify/require"
)

func TestOutputFlagIsRejectedBeforeAPICall(t *testing.T) {
	for _, format := range []string{"json", "table", "yaml"} {
		for _, query := range []bool{false, true} {
			t.Run(format+map[bool]string{false: "", true: "-query"}[query], func(t *testing.T) {
				commandLine := newCLI()
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
					t.Fatal("API factory called for unsupported --output")
					return nil, nil
				})
				args := []string{"iaas-api", "switch", "create", "--zone", "test-zone", "--name", "test", "--output", format}
				if query {
					args = append(args, "--query", ".Name")
				}
				var stdout, stderr bytes.Buffer
				code := runCLI(args, &stdout, &stderr, commandLine)
				require.NotEqual(t, 0, code, "removed flag: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
				require.EqualValues(t, 0, stdout.Len(), "unexpected value")
				require.Contains(t, stderr.String(), "--output", "missing expected text %q", "--output")
			})
		}
	}
}

func TestJSONOutputIgnoresLegacyProfileFormat(t *testing.T) {
	for _, test := range []struct {
		name, filename, contents string
	}{
		{"v1", "config.yaml", "version: 1\ncli:\n  default_output_type: table\n"},
		{"v0", "config.json", `{"DefaultOutputType":"table"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			profileDir := t.TempDir()
			t.Setenv("SAKURA_PROFILE_DIR", profileDir)
			profilePath := filepath.Join(profileDir, "example")
			err := os.Mkdir(profilePath, 0o700)
			require.NoError(t, err)
			err2 := os.WriteFile(filepath.Join(profileDir, "current"), []byte("example\n"), 0o600)
			require.NoError(t, err2)
			err3 := os.WriteFile(filepath.Join(profilePath, test.filename), []byte(test.contents), 0o600)
			require.NoError(t, err3)
			server := iaasmock.NewTestServer(iaasmock.Config{})
			t.Cleanup(server.Close)
			runCommand := func(args ...string) []byte {
				t.Helper()
				commandLine := newCLI()
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return swytch.New(server), nil })
				var stdout, stderr bytes.Buffer
				require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
				return stdout.Bytes()
			}
			var created iaas.Switch
			err4 := json.Unmarshal(runCommand("iaas-api", "switch", "create", "--zone", "test-zone", "--name", "output-test"), &created)
			require.NoError(t, err4)
			require.Equal(t, "output-test", created.Name, "JSON create returned %+v", created)
			output := runCommand("iaas-api", "switch", "find", "--zone", "test-zone", "--query", "map({ID,Name})")
			var projected []map[string]json.RawMessage
			err5 := json.Unmarshal(output, &projected)
			require.NoError(t, err5)
			if len(projected) != 1 || len(projected[0]) != 2 || string(projected[0]["Name"]) != `"output-test"` {
				t.Fatalf("projected output = %s", output)
			}
		})
	}
}

func TestQueryErrorsBeforeAPICall(t *testing.T) {
	for _, expression := range []string{"(", "unknown_function"} {
		t.Run(expression, func(t *testing.T) {
			commandLine := newCLI()
			setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
				t.Fatal("API factory called with invalid query")
				return nil, nil
			})
			var stdout, stderr bytes.Buffer
			code := runCLI([]string{"iaas-api", "switch", "create", "--zone", "test-zone", "--name", "test", "--query", expression}, &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "invalid query: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.Contains(t, stderr.String(), "--query expression", "missing expected text %q", "--query expression")
		})
	}
}

func TestWriteQueryOutput(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		value            any
		want             string
	}{
		{"preserves large numbers", ".ID", map[string]any{"ID": json.Number("9007199254740993")}, "9007199254740993\n"},
		{"writes multiple results", ".[]", []int{1, 2}, "1\n2\n"},
		{"empty result", ".[]", []int{}, ""},
		{"empty projection", "map({ID,Name})", []any{}, "[]\n"},
		{"halt stops after previous results", `("before", halt, "after")`, map[string]any{}, "\"before\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := writeQueryOutput(&output, test.expression, test.value)
			require.NoError(t, err)
			got := output.String()
			require.Equal(t, test.want, got, "writeQueryOutput() = %q, want %q", got, test.want)
		})
	}
}

func TestWriteQueryOutputDoesNotWritePartialResultsOnError(t *testing.T) {
	var output bytes.Buffer
	err := writeQueryOutput(&output, `(.Name, error("query failed"))`, map[string]any{"Name": "example"})
	require.Error(t, err, "writeQueryOutput() succeeded, want evaluation error")
	got := output.String()
	require.Empty(t, got, "writeQueryOutput() wrote partial output %q", got)
}
