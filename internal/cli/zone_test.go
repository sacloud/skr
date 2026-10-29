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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	"github.com/sacloud/skr/internal/apigen"
	zoneapi "github.com/sacloud/skr/internal/iaas/zoneapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

func TestZoneGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-zone.json"))
	require.NoError(t, err, "read zone config")
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err, "decode zone config")
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err, "generate zone commands")
	got, err := os.ReadFile(repositoryPath("internal/iaas/zoneapi/zone_api_generated.go"))
	require.NoError(t, err, "read generated zone commands")
	require.Equal(t, want, got, "generated Zone commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-zone.json API_OUTPUT=internal/iaas/zoneapi/zone_api_generated.go")
}

func TestIaaSZoneHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "zone", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "zone", "find", "--help"}, want: []string{"--query", "map(.Name)", "--request", "FindRequest"}},
		{args: []string{"iaas-api", "zone", "read", "--help"}, want: []string{"--id", "--request", "@path.json", "ZONE-ID", "併用できません"}},
	} {
		var stdout, stderr bytes.Buffer
		code := run(test.args, &stdout, &stderr)
		require.Equal(t, 0, code, "run(%v) stderr: %s", test.args, stderr.String())
		help := strings.Join(strings.Fields(stdout.String()), " ")
		for _, text := range test.want {
			require.Contains(t, help, text, "help for %v", test.args)
		}
	}
}

func TestIaaSZoneFindWithLocalMockAndQuery(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		require.Equal(t, 0, code, "run(%v) stderr: %s", args, stderr.String())
		require.Empty(t, stderr, "run(%v) stderr should be empty", args)
		return stdout.String()
	}

	var zones []*iaas.Zone
	require.NoError(t, json.Unmarshal([]byte(runCommand("iaas-api", "zone", "find")), &zones), "decode zone find output")
	require.Len(t, zones, 2, "configured mock zones")
	require.Equal(t, "test-zone-a", zones[0].Name, "zone 0 name")
	require.Equal(t, "test-zone-b", zones[1].Name, "zone 1 name")

	queryOutput := runCommand("iaas-api", "zone", "find", "--query", "map(.Name)")
	var names []string
	require.NoError(t, json.Unmarshal([]byte(queryOutput), &names), "decode query output %q", queryOutput)
	require.Equal(t, []string{"test-zone-a", "test-zone-b"}, names, "configured zone names")
}

func TestIaaSZoneReadWithLocalMock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		require.Equal(t, 0, code, "run(%v) stderr: %s", args, stderr.String())
		require.Empty(t, stderr, "run(%v) stderr should be empty", args)
		return stdout.String()
	}

	// --id フラグ経路でゾーンを参照する
	output := runCommand("iaas-api", "zone", "read", "--id", "2")
	require.Contains(t, output, `"Name": "test-zone-b"`, "read --id output %q", output)

	// --query で必要な項目へ絞り込む。応答は単一オブジェクトのためオブジェクト構築式を使う
	projected := runCommand("iaas-api", "zone", "read", "--id", "1", "--query", "{ID,Name}")
	var item struct {
		ID   int64
		Name string
	}
	require.NoError(t, json.Unmarshal([]byte(projected), &item), "decode projected output %q", projected)
	require.Equal(t, int64(1), item.ID, "projected read ID")
	require.Equal(t, "test-zone-a", item.Name, "projected read name")

	// --request 経路でも同じ SDK リクエストになる
	jsonOutput := runCommand("iaas-api", "zone", "read", "--request", `{"ID":2}`)
	require.Contains(t, jsonOutput, `"Name": "test-zone-b"`, "read --request output %q", jsonOutput)
}

func TestIaaSZoneRejectsInvalidReadRequestsBeforeCallingAPI(t *testing.T) {
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a"}})
	t.Cleanup(server.Close)
	for _, args := range [][]string{
		{"read"},
		{"read", "--id", "invalid"},
		{"read", "--id", "1", "--request", `{"ID":1}`},
		{"read", "--request", `{"ID":"invalid"}`},
		{"read", "--request", `{"ID":1} {}`},
		{"read", "--request", `{"Unknown":1}`},
		{"read", "--request", `not-json`},
	} {
		commandLine := newCLI()
		setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
			return zone.New(server), nil
		})
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "zone"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v should fail with a validation error", args)
		require.Empty(t, stdout, "%v should not write output", args)
		require.NotEmpty(t, stderr, "%v should report a validation error", args)
	}
}

func TestIaaSZoneReadPropagatesAPIErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "zone", "read", "--id", "99"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "read error should fail")
	require.Empty(t, stdout, "read error should not write output")
	require.Contains(t, stderr.String(), "not_found", "read error %q", stderr.String())
}
