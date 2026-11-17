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
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/skr/internal/apigen"
	ipaddressapi "github.com/sacloud/skr/internal/iaas/ipaddressapi"
	"github.com/stretchr/testify/require"
)

func TestIPAddressGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-ipaddress.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/ipaddressapi/ipaddress_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated IPAddress commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-ipaddress.json API_OUTPUT=internal/iaas/ipaddressapi/ipaddress_api_generated.go")
}

func TestIaaSIPAddressHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"ipaddress"}},
		{args: []string{"iaas-api", "ipaddress", "--help"}, want: []string{"list", "read", "update-host-name"}},
		{args: []string{"iaas-api", "ipaddress", "list", "--help"}, want: []string{"--zone", "--request", "@path.json", "併用できません"}},
		{args: []string{"iaas-api", "ipaddress", "read", "--help"}, want: []string{"--zone", "--ip-address", "--request", "@path.json", "併用できません"}},
		{args: []string{"iaas-api", "ipaddress", "update-host-name", "--help"}, want: []string{"--zone", "--ip-address", "--host-name", "--request", "RetryMax", "RetryInterval", "併用できません"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help for %v does not contain %q: %s", test.args, text, help)
			}
		})
	}
}

type ipAddressTestDouble struct {
	listRequests   []*ipaddressapi.ListRequest
	readRequests   []*ipaddressapi.ReadRequest
	updateRequests []*ipaddressapi.UpdateHostNameRequest
	listErr        error
	readErr        error
	updateErr      error
}

func (api *ipAddressTestDouble) ListWithContext(_ context.Context, request *ipaddressapi.ListRequest) ([]*iaas.IPAddress, error) {
	api.listRequests = append(api.listRequests, request)
	if api.listErr != nil {
		return nil, api.listErr
	}
	return []*iaas.IPAddress{{HostName: "test-host", IPAddress: "192.0.2.10"}}, nil
}

func (api *ipAddressTestDouble) ReadWithContext(_ context.Context, request *ipaddressapi.ReadRequest) (*iaas.IPAddress, error) {
	api.readRequests = append(api.readRequests, request)
	if api.readErr != nil {
		return nil, api.readErr
	}
	return &iaas.IPAddress{HostName: "test-host", IPAddress: request.IPAddress}, nil
}

func (api *ipAddressTestDouble) UpdateHostNameWithContext(_ context.Context, request *ipaddressapi.UpdateHostNameRequest) error {
	api.updateRequests = append(api.updateRequests, request)
	return api.updateErr
}

func TestIaaSIPAddressCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &ipAddressTestDouble{}
	commandLine := newCLI()
	setIPAddressAPIFactory(commandLine, func() (ipaddressapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "ipaddress"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	// list: 個別フラグ経路
	output, stderr, code := run("list", "--zone", "is1b")
	require.Equal(t, 0, code, "list flags: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.listRequests, 1, "List request = %+v, want zone", api.listRequests)
	require.Equal(t, "is1b", api.listRequests[0].Zone, "unexpected value")
	var listed []struct {
		IPAddress string `json:"IPAddress"`
		HostName  string `json:"HostName"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &listed), "output = %s", output)
	require.Len(t, listed, 1, "unexpected value")
	require.Equal(t, "192.0.2.10", listed[0].IPAddress, "unexpected value")

	// list: --request 経路
	_, stderr, code = run("list", "--request", `{"Zone":"tk1a"}`)
	require.Equal(t, 0, code, "list JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.listRequests, 2, "List request = %+v, want zone from JSON", api.listRequests)
	require.Equal(t, "tk1a", api.listRequests[1].Zone, "unexpected value")

	// read: 個別フラグ経路
	output, stderr, code = run("read", "--zone", "is1b", "--ip-address", "192.0.2.10")
	require.Equal(t, 0, code, "read flags: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 1, "Read request = %+v, want zone and IP address", api.readRequests)
	require.Equal(t, "is1b", api.readRequests[0].Zone, "unexpected value")
	require.Equal(t, "192.0.2.10", api.readRequests[0].IPAddress, "unexpected value")
	require.Contains(t, output, `"HostName": "test-host"`, "missing expected text %q: %s", `"HostName": "test-host"`, output)

	// read: --request 経路
	_, stderr, code = run("read", "--request", `{"Zone":"is1b","IPAddress":"2001:db8::1"}`)
	require.Equal(t, 0, code, "read JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 2, "Read request = %+v, want values from JSON", api.readRequests)
	require.Equal(t, "is1b", api.readRequests[1].Zone, "unexpected value")
	require.Equal(t, "2001:db8::1", api.readRequests[1].IPAddress, "unexpected value")

	// update-host-name: 個別フラグ経路 (--host-name を省略すると空になる)
	_, stderr, code = run("update-host-name", "--zone", "is1b", "--ip-address", "192.0.2.10", "--host-name", "www.example.com")
	require.Equal(t, 0, code, "update-host-name flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.updateRequests, 1, "UpdateHostName request = %+v, want values from flags", api.updateRequests)
	require.Equal(t, "is1b", api.updateRequests[0].Zone, "unexpected value")
	require.Equal(t, "192.0.2.10", api.updateRequests[0].IPAddress, "unexpected value")
	require.Equal(t, "www.example.com", api.updateRequests[0].HostName, "unexpected value")

	// update-host-name: --request 経路。RetryMax と RetryInterval は JSON 専用フィールド
	_, stderr, code = run("update-host-name", "--request", `{"Zone":"is1b","IPAddress":"192.0.2.10","HostName":"","RetryMax":2,"RetryInterval":3}`)
	require.Equal(t, 0, code, "update-host-name JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.updateRequests, 2, "UpdateHostName request = %+v, want values from JSON", api.updateRequests)
	require.Equal(t, "is1b", api.updateRequests[1].Zone, "unexpected value")
	require.Equal(t, "192.0.2.10", api.updateRequests[1].IPAddress, "unexpected value")
	require.Empty(t, api.updateRequests[1].HostName, "unexpected value")
	require.Equal(t, 2, api.updateRequests[1].RetryMax, "unexpected value")
	require.Equal(t, 3, api.updateRequests[1].RetryInterval, "unexpected value")
}

func TestIaaSIPAddressRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	for _, args := range [][]string{
		{"list"},
		{"list", "--zone", "is1b", "--request", `{"Zone":"is1b"}`},
		{"list", "--request", `{"Zone":123}`},
		{"list", "--request", `{"Zone":"is1b"} {}`},
		{"read"},
		{"read", "--zone", "is1b", "--request", `{"Zone":"is1b","IPAddress":"192.0.2.10"}`},
		{"read", "--request", `{"Zone":"is1b","IPAddress":true}`},
		{"update-host-name"},
		{"update-host-name", "--request", `{"Zone":"is1b","IPAddress":"192.0.2.10","RetryMax":"many"}`},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			commandLine := newCLI()
			testDouble := &ipAddressTestDouble{}
			setIPAddressAPIFactory(commandLine, func() (ipaddressapi.API, error) { return testDouble, nil })
			var stdout, stderr bytes.Buffer
			code := runCLI(append([]string{"iaas-api", "ipaddress"}, args...), &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
			require.EqualValues(t, 0, len(testDouble.listRequests)+len(testDouble.readRequests)+len(testDouble.updateRequests), "invalid requests reached API: %+v", testDouble)
		})
	}
}

func TestIaaSIPAddressPropagatesAPIErrors(t *testing.T) {
	api := &ipAddressTestDouble{listErr: errors.New("ipaddress API failure")}
	commandLine := newCLI()
	setIPAddressAPIFactory(commandLine, func() (ipaddressapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "ipaddress", "list", "--zone", "is1b"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "code %d stdout %q stderr %q; want API error", code, stdout.String(), stderr.String())
	require.Contains(t, stderr.String(), "ipaddress API failure", "stderr = %q, want API error", stderr.String())
}
