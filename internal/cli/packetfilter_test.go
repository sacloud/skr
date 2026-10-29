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

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	"github.com/sacloud/skr/internal/apigen"
	packetfilterapi "github.com/sacloud/skr/internal/iaas/packetfilterapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
	"github.com/stretchr/testify/require"
)

func TestPacketFilterGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-packetfilter.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/packetfilterapi/packetfilter_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated PacketFilter commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-packetfilter.json API_OUTPUT=internal/iaas/packetfilterapi/packetfilter_api_generated.go")
}

func TestRunIaaSPacketFilterHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"packetfilter"}},
		{args: []string{"iaas-api", "packetfilter", "--help"}, want: []string{"find", "read", "create", "update", "delete", "--expressions", "--request"}},
		{args: []string{"iaas-api", "packetfilter", "create", "--help"}, want: []string{"--zone", "--name", "--description", "Expression", "tcp", "allow", "--expressions", "@path.json", "併用不可"}},
		{args: []string{"iaas-api", "packetfilter", "update", "--help"}, want: []string{"--zone", "--id", "--name", "--description", "Expression", "OriginalExpressionHash", "変更する項目", "併用不可"}},
		{args: []string{"iaas-api", "packetfilter", "find", "--help"}, want: []string{"Names", "--count", "--from", "単一のゾーン"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help does not contain %q", text)
			}
		})
	}
}

func TestIaaSPacketFilterAPIWithLocalMock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) {
			return packetfilter.New(server), nil
		})
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	expressions := `[{"Protocol":"tcp","SourceNetwork":"192.0.2.0/24","DestinationPort":"443","Action":"allow","Description":"Allow HTTPS"}]`
	var created iaas.PacketFilter
	err := json.Unmarshal(runCommand("iaas-api", "packetfilter", "create", "--zone", "test-zone", "--name", "packetfilter-test", "--description", "test filter", "--expressions", expressions), &created)
	require.NoError(t, err)
	require.NotEqualValues(t, 0, created.ID, "create returned %#v", created)
	require.Equal(t, "packetfilter-test", created.Name, "unexpected value")
	require.Len(t, created.Expression, 1, "unexpected length")
	require.Equal(t, "mock-expression-hash", created.ExpressionHash, "unexpected value")
	rule := created.Expression[0]
	if rule.Protocol != types.Protocols.TCP || rule.SourceNetwork != "192.0.2.0/24" ||
		rule.DestinationPort != "443" || rule.Action != types.Actions.Allow {
		t.Fatalf("create returned unexpected rule: %+v", rule)
	}

	findRequest := `{"Zone":"test-zone","Names":["packetfilter-test"]}`
	var found []*iaas.PacketFilter
	err2 := json.Unmarshal(runCommand("iaas-api", "packetfilter", "find", "--request", findRequest), &found)
	require.NoError(t, err2)
	require.Len(t, found, 1, "find returned %#v, want PacketFilter %s", found, created.ID)
	require.Equal(t, created.ID, found[0].ID, "unexpected value")

	var read iaas.PacketFilter
	readRequest, err := json.Marshal(map[string]any{"Zone": "test-zone", "ID": created.ID})
	require.NoError(t, err)
	err3 := json.Unmarshal(runCommand("iaas-api", "packetfilter", "read", "--request", string(readRequest)), &read)
	require.NoError(t, err3)
	require.Equal(t, created.ID, read.ID, "read returned %#v", read)
	require.Len(t, read.Expression, 1, "unexpected length")

	rulesPath := t.TempDir() + "/rules.json"
	updatedExpressions := `[{"Protocol":"tcp","SourceNetwork":"192.0.2.0/24","DestinationPort":"8443","Action":"allow","Description":"Allow HTTPS alternative"}]`
	err4 := os.WriteFile(rulesPath, []byte(updatedExpressions), 0o600)
	require.NoError(t, err4)
	var updated iaas.PacketFilter
	err5 := json.Unmarshal(runCommand("iaas-api", "packetfilter", "update", "--zone", "test-zone", "--id", created.ID.String(), "--name", "updated-filter", "--expressions", "@"+rulesPath, "--original-expression-hash", created.ExpressionHash), &updated)
	require.NoError(t, err5)
	if updated.ID != created.ID || updated.Name != "updated-filter" || updated.Description != "test filter" ||
		len(updated.Expression) != 1 || updated.Expression[0].DestinationPort != "8443" {
		t.Fatalf("update did not preserve omitted values: %+v", updated)
	}

	output := runCommand("iaas-api", "packetfilter", "delete", "--zone", "test-zone", "--id", created.ID.String(), "--fail-if-not-found")
	require.Len(t, output, 0, "delete output = %q, want empty", output)
	found = nil
	err6 := json.Unmarshal(runCommand("iaas-api", "packetfilter", "find", "--request", `{"Zone":"test-zone"}`), &found)
	require.NoError(t, err6)
	require.Len(t, found, 0, "find after delete returned %#v, want empty", found)
}

func TestPacketFilterRejectsInvalidRequestsBeforeFactory(t *testing.T) {
	for _, test := range []struct {
		args []string
	}{
		{args: []string{"find", "--zone", "all"}},
		{args: []string{"find", "--request", `{"Zone":"all"}`}},
		{args: []string{"create", "--zone", "all", "--name", "filter"}},
		{args: []string{"create", "--request", `{"Zone":"test-zone","Name":"filter"}`, "--expressions", `[]`}},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			factoryCalled := false
			commandLine := newCLI()
			setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) {
				factoryCalled = true
				return packetfilter.New(iaasmock.NewTestServer(iaasmock.Config{})), nil
			})
			var stdout, stderr bytes.Buffer
			args := append([]string{"iaas-api", "packetfilter"}, test.args...)
			code := runCLI(args, &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "run(%v): code=%d stdout=%q stderr=%q; want input error", args, code, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
			require.False(t, factoryCalled, "invalid request created an API client")
		})
	}
}

func TestPacketFilterHelpExampleDecodes(t *testing.T) {
	var request packetfilter.CreateRequest
	err := decodeRequest(`{"Zone":"is1b","Name":"example","Expression":[{"Protocol":"tcp","SourceNetwork":"192.0.2.0/24","DestinationPort":"443","Action":"allow","Description":"Allow HTTPS"}]}`, &request)
	require.NoError(t, err)
	if request.Zone != "is1b" || request.Name != "example" || len(request.Expression) != 1 ||
		request.Expression[0].Protocol != types.Protocols.TCP || request.Expression[0].Action != types.Actions.Allow {
		t.Fatalf("decoded request = %+v", request)
	}
}

func TestDecodePacketFilterExpressions(t *testing.T) {
	path := t.TempDir() + "/rules.json"
	fileContent := `[{"Protocol":"udp","DestinationPort":"53","Action":"allow"}]`
	err := os.WriteFile(path, []byte(fileContent), 0o600)
	require.NoError(t, err)
	for _, input := range []string{
		`[{"Protocol":"tcp","DestinationPort":"443","Action":"allow"}]`,
		"@" + path,
	} {
		expressions, err := decodePacketFilterExpressions([]string{input})
		require.NoError(t, err, "decodePacketFilterExpressions(%q)", input)
		require.Len(t, expressions, 1, "decoded %q to %d expressions, want 1", input, len(expressions))
	}
	for _, inputs := range [][]string{
		{"{"},
		{`{"Protocol":"tcp"}`},
		{"[]", "[]"},
	} {
		if _, err := decodePacketFilterExpressions(inputs); err == nil {
			t.Errorf("decodePacketFilterExpressions(%q) succeeded, want error", inputs)
		}
	}
}
