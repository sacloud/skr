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
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
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
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return switchapi.NewAPI(server), nil })
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

func TestQueryRawOutputThroughCLI(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	profilePath := filepath.Join(profileDir, "example")
	err := os.Mkdir(profilePath, 0o700)
	require.NoError(t, err)
	err2 := os.WriteFile(filepath.Join(profileDir, "current"), []byte("example\n"), 0o600)
	require.NoError(t, err2)
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	runCommand := func(args ...string) string {
		t.Helper()
		commandLine := newCLI()
		setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return switchapi.NewAPI(server), nil })
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		return stdout.String()
	}
	runCommand("iaas-api", "switch", "create", "--zone", "test-zone", "--name", "query-raw-test")
	// 文字列は引用符なしで取り込めます。
	got := runCommand("iaas-api", "switch", "find", "--zone", "test-zone", "--query-raw", "map(select(.Name == \"query-raw-test\")) | .[0].Name")
	require.Equal(t, "query-raw-test\n", got, "--query-raw output = %q, want raw string", got)
	// 文字列以外は --query と同じ JSON 出力です。
	gotJSON := runCommand("iaas-api", "switch", "find", "--zone", "test-zone", "--query-raw", "map({ID,Name})")
	var projected []map[string]json.RawMessage
	err3 := json.Unmarshal([]byte(gotJSON), &projected)
	require.NoError(t, err3)
	require.Len(t, projected, 1, "projected output = %s", gotJSON)
	require.JSONEq(t, `"query-raw-test"`, string(projected[0]["Name"]), "unexpected value")
}

func TestQueryErrorsBeforeAPICall(t *testing.T) {
	for _, flag := range []string{"query", "query-raw"} {
		for _, expression := range []string{"(", "unknown_function"} {
			t.Run(flag+"/"+expression, func(t *testing.T) {
				commandLine := newCLI()
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
					t.Fatal("API factory called with invalid query")
					return nil, nil
				})
				var stdout, stderr bytes.Buffer
				code := runCLI([]string{"iaas-api", "switch", "create", "--zone", "test-zone", "--name", "test", "--" + flag, expression}, &stdout, &stderr, commandLine)
				require.NotEqual(t, 0, code, "invalid query: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
				require.EqualValues(t, 0, stdout.Len(), "unexpected value")
				require.Contains(t, stderr.String(), "--"+flag+" expression", "missing expected text %q", "--"+flag+" expression")
			})
		}
	}
}

func TestQueryAndQueryRawAreMutuallyExclusive(t *testing.T) {
	commandLine := newCLI()
	setSwitchAPIFactory(commandLine, func() (switchapi.API, error) {
		t.Fatal("API factory called with both --query and --query-raw")
		return nil, nil
	})
	var stdout, stderr bytes.Buffer
	args := []string{"iaas-api", "switch", "create", "--zone", "test-zone", "--name", "test", "--query", ".ID", "--query-raw", ".ID"}
	code := runCLI(args, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	for _, text := range []string{"--query", "--query-raw", "can't be used together"} {
		require.Contains(t, stderr.String(), text, "missing expected text %q", text)
	}
}

func TestQueryFlagFromContextRejectsUnexpectedValueType(t *testing.T) {
	var parsed struct {
		QueryRaw int `name:"query-raw"`
	}
	parser, err := kong.New(&parsed, kong.Name("skr"), kong.Writers(io.Discard, io.Discard), kong.Exit(func(int) {}))
	require.NoError(t, err)
	ctx, err2 := parser.Parse([]string{"--query-raw", "1"})
	require.NoError(t, err2)
	_, _, err3 := queryFlagFromContext(ctx)
	require.Error(t, err3, "queryFlagFromContext() error = %v, want unexpected value type error", err3)
	require.Contains(t, err3.Error(), "read --query-raw value", "missing expected text %q", "read --query-raw value")
}

func TestWriteQueryOutput(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		raw              bool
		value            any
		want             string
	}{
		{"preserves large numbers", ".ID", false, map[string]any{"ID": json.Number("9007199254740993")}, "9007199254740993\n"},
		{"writes multiple results", ".[]", false, []int{1, 2}, "1\n2\n"},
		{"empty result", ".[]", false, []int{}, ""},
		{"empty projection", "map({ID,Name})", false, []any{}, "[]\n"},
		{"halt stops after previous results", `("before", halt, "after")`, false, map[string]any{}, "\"before\"\n"},
		// --query-raw は jq -r に準拠し、文字列だけ引用符なしで出力します。
		{"raw string drops quotes", ".ID", true, map[string]any{"ID": "123456789012"}, "123456789012\n"},
		{"raw multiple strings", ".[]", true, []string{"a", "b"}, "a\nb\n"},
		{"raw number stays json", ".ID", true, map[string]any{"ID": json.Number("1")}, "1\n"},
		{"raw boolean stays json", ".enabled", true, map[string]any{"enabled": true}, "true\n"},
		{"raw null stays json", ".Name", true, map[string]any{"Other": "x"}, "null\n"},
		{"raw object stays json", ".", true, map[string]any{"Name": "example"}, "{\n  \"Name\": \"example\"\n}\n"},
		{"raw empty result", ".[]", true, []int{}, ""},
		{"raw mixed results", ".[]", true, []any{"a", 1, map[string]any{"K": "v"}}, "a\n1\n{\n  \"K\": \"v\"\n}\n"},
		{"raw halt stops after previous results", `("before", halt, "after")`, true, map[string]any{}, "before\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := writeQueryOutput(&output, &queryFlag{expression: test.expression, raw: test.raw, name: map[bool]string{false: "query", true: "query-raw"}[test.raw]}, test.value)
			require.NoError(t, err)
			got := output.String()
			require.Equal(t, test.want, got, "writeQueryOutput() = %q, want %q", got, test.want)
		})
	}
}

func TestWriteQueryOutputDoesNotWritePartialResultsOnError(t *testing.T) {
	for _, raw := range []bool{false, true} {
		var output bytes.Buffer
		err := writeQueryOutput(&output, &queryFlag{expression: `(.Name, error("query failed"))`, raw: raw, name: "query"}, map[string]any{"Name": "example"})
		require.Error(t, err, "writeQueryOutput() succeeded, want evaluation error")
		got := output.String()
		require.Empty(t, got, "writeQueryOutput() wrote partial output %q", got)
	}
}
