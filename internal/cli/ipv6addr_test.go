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
	ipv6addrapi "github.com/sacloud/skr/internal/iaas/ipv6addrapi"
	"github.com/stretchr/testify/require"
)

func TestIPv6AddrGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-ipv6addr.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/ipv6addrapi/ipv6addr_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated IPv6Addr commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-ipv6addr.json API_OUTPUT=internal/iaas/ipv6addrapi/ipv6addr_api_generated.go")
}

func TestIaaSIPv6AddrHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"ipv6addr"}},
		{args: []string{"iaas-api", "ipv6addr", "--help"}, want: []string{"find", "create", "read", "update", "delete"}},
		{args: []string{"iaas-api", "ipv6addr", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "SwitchIDs", "HostNames", "IPv6Addresses", "Sort", "併用できません"}},
		{args: []string{"iaas-api", "ipv6addr", "create", "--help"}, want: []string{"--zone", "--ipv6-addr", "--host-name", "--request", "併用できません"}},
		{args: []string{"iaas-api", "ipv6addr", "update", "--help"}, want: []string{"--zone", "--ipv6-addr", "--host-name", "--request", "併用できません"}},
		{args: []string{"iaas-api", "ipv6addr", "delete", "--help"}, want: []string{"--zone", "--ipv6-addr", "--request", "FailIfNotFound", "併用できません"}},
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

type ipv6AddrTestDouble struct {
	findRequests   []*ipv6addrapi.FindRequest
	createRequests []*ipv6addrapi.CreateRequest
	readRequests   []*ipv6addrapi.ReadRequest
	updateRequests []*ipv6addrapi.UpdateRequest
	deleteRequests []*ipv6addrapi.DeleteRequest
	findErr        error
	createErr      error
	readErr        error
	updateErr      error
	deleteErr      error
}

func (api *ipv6AddrTestDouble) FindWithContext(_ context.Context, request *ipv6addrapi.FindRequest) ([]*iaas.IPv6Addr, error) {
	api.findRequests = append(api.findRequests, request)
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.IPv6Addr{{IPv6Addr: "2001:db8::1", HostName: "test-host"}}, nil
}

func (api *ipv6AddrTestDouble) CreateWithContext(_ context.Context, request *ipv6addrapi.CreateRequest) (*iaas.IPv6Addr, error) {
	api.createRequests = append(api.createRequests, request)
	if api.createErr != nil {
		return nil, api.createErr
	}
	return &iaas.IPv6Addr{IPv6Addr: request.IPv6Addr, HostName: request.HostName}, nil
}

func (api *ipv6AddrTestDouble) ReadWithContext(_ context.Context, request *ipv6addrapi.ReadRequest) (*iaas.IPv6Addr, error) {
	api.readRequests = append(api.readRequests, request)
	if api.readErr != nil {
		return nil, api.readErr
	}
	return &iaas.IPv6Addr{IPv6Addr: request.IPv6Addr, HostName: "test-host"}, nil
}

func (api *ipv6AddrTestDouble) UpdateWithContext(_ context.Context, request *ipv6addrapi.UpdateRequest) (*iaas.IPv6Addr, error) {
	api.updateRequests = append(api.updateRequests, request)
	if api.updateErr != nil {
		return nil, api.updateErr
	}
	return &iaas.IPv6Addr{IPv6Addr: request.IPv6Addr, HostName: request.HostName}, nil
}

func (api *ipv6AddrTestDouble) DeleteWithContext(_ context.Context, request *ipv6addrapi.DeleteRequest) error {
	api.deleteRequests = append(api.deleteRequests, request)
	return api.deleteErr
}

func TestIaaSIPv6AddrCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &ipv6AddrTestDouble{}
	commandLine := newCLI()
	setIPv6AddrAPIFactory(commandLine, func() (ipv6addrapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "ipv6addr"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	// find: 個別フラグ経路
	output, stderr, code := run("find", "--zone", "is1b", "--count", "5", "--from", "2")
	require.Equal(t, 0, code, "find flags: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.findRequests, 1, "Find request = %+v, want zone, count and from", api.findRequests)
	require.Equal(t, "is1b", api.findRequests[0].Zone, "unexpected value")
	require.Equal(t, 5, api.findRequests[0].Count, "unexpected value")
	require.Equal(t, 2, api.findRequests[0].From, "unexpected value")

	// find: --request 経路。絞り込み条件と Sort は JSON で指定する
	_, stderr, code = run("find", "--request", `{"Zone":"is1b","SwitchIDs":["123"],"HostNames":["www.example.com"],"IPv6Addresses":["2001:db8::1"],"Sort":[{"Key":"IPv6Addr","Order":1}],"Count":10,"From":20}`)
	require.Equal(t, 0, code, "find JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.findRequests, 2, "Find request = %+v, want filters and sort from JSON", api.findRequests)
	request := api.findRequests[1]
	require.Equal(t, []string{"123"}, request.SwitchIDs, "unexpected value")
	require.Equal(t, []string{"www.example.com"}, request.HostNames, "unexpected value")
	require.Equal(t, []string{"2001:db8::1"}, request.IPv6Addresses, "unexpected value")
	require.Len(t, request.Sort, 1, "unexpected value")
	require.Equal(t, "IPv6Addr", string(request.Sort[0].Key), "unexpected value")
	require.Equal(t, 10, request.Count, "unexpected value")
	require.Equal(t, 20, request.From, "unexpected value")
	var found []struct {
		IPv6Addr string `json:"IPv6Addr"`
		HostName string `json:"HostName"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &found), "output = %s", output)
	require.Len(t, found, 1, "unexpected value")
	require.Equal(t, "2001:db8::1", found[0].IPv6Addr, "unexpected value")

	// create: 個別フラグ経路
	output, stderr, code = run("create", "--zone", "is1b", "--ipv6-addr", "2001:db8::2", "--host-name", "create.example.com")
	require.Equal(t, 0, code, "create flags: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.createRequests, 1, "Create request = %+v, want values from flags", api.createRequests)
	require.Equal(t, "is1b", api.createRequests[0].Zone, "unexpected value")
	require.Equal(t, "2001:db8::2", api.createRequests[0].IPv6Addr, "unexpected value")
	require.Equal(t, "create.example.com", api.createRequests[0].HostName, "unexpected value")
	require.Contains(t, output, `"HostName": "create.example.com"`, "missing expected text %q: %s", `"HostName": "create.example.com"`, output)

	// create: --request 経路
	_, stderr, code = run("create", "--request", `{"Zone":"is1b","IPv6Addr":"2001:db8::3","HostName":"json.example.com"}`)
	require.Equal(t, 0, code, "create JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.createRequests, 2, "Create request = %+v, want values from JSON", api.createRequests)
	require.Equal(t, "2001:db8::3", api.createRequests[1].IPv6Addr, "unexpected value")
	require.Equal(t, "json.example.com", api.createRequests[1].HostName, "unexpected value")

	// read: --request 経路
	output, stderr, code = run("read", "--request", `{"Zone":"is1b","IPv6Addr":"2001:db8::1"}`)
	require.Equal(t, 0, code, "read JSON: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 1, "Read request = %+v, want values from JSON", api.readRequests)
	require.Equal(t, "2001:db8::1", api.readRequests[0].IPv6Addr, "unexpected value")
	require.Contains(t, output, `"HostName": "test-host"`, "missing expected text %q: %s", `"HostName": "test-host"`, output)

	// read: 個別フラグ経路
	_, stderr, code = run("read", "--zone", "is1b", "--ipv6-addr", "2001:db8::1")
	require.Equal(t, 0, code, "read flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 2, "Read request = %+v, want values from flags", api.readRequests)
	require.Equal(t, "is1b", api.readRequests[1].Zone, "unexpected value")

	// update: --request 経路
	output, stderr, code = run("update", "--request", `{"Zone":"is1b","IPv6Addr":"2001:db8::1","HostName":"updated.example.com"}`)
	require.Equal(t, 0, code, "update JSON: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.updateRequests, 1, "Update request = %+v, want values from JSON", api.updateRequests)
	require.Equal(t, "updated.example.com", api.updateRequests[0].HostName, "unexpected value")
	require.Contains(t, output, `"HostName": "updated.example.com"`, "missing expected text %q: %s", `"HostName": "updated.example.com"`, output)

	// update: 個別フラグ経路
	_, stderr, code = run("update", "--zone", "is1b", "--ipv6-addr", "2001:db8::1", "--host-name", "flags.example.com")
	require.Equal(t, 0, code, "update flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.updateRequests, 2, "Update request = %+v, want values from flags", api.updateRequests)
	require.Equal(t, "flags.example.com", api.updateRequests[1].HostName, "unexpected value")

	// delete: --request 経路。FailIfNotFound は JSON 専用フィールド
	output, stderr, code = run("delete", "--request", `{"Zone":"is1b","IPv6Addr":"2001:db8::1","FailIfNotFound":true}`)
	require.Equal(t, 0, code, "delete JSON: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.deleteRequests, 1, "Delete request = %+v, want values from JSON", api.deleteRequests)
	require.True(t, api.deleteRequests[0].FailIfNotFound, "unexpected value")

	// delete: 個別フラグ経路
	_, stderr, code = run("delete", "--zone", "is1b", "--ipv6-addr", "2001:db8::1")
	require.Equal(t, 0, code, "delete flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.deleteRequests, 2, "Delete request = %+v, want values from flags", api.deleteRequests)
	require.False(t, api.deleteRequests[1].FailIfNotFound, "unexpected value")
}

func TestIaaSIPv6AddrRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &ipv6AddrTestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "is1b", "--request", `{"Zone":"is1b"}`},
		{"find", "--request", `{"Zone":"is1b","Count":"many"}`},
		{"create"},
		{"create", "--zone", "is1b", "--request", `{"Zone":"is1b"}`},
		{"create", "--request", `{"Zone":"is1b","IPv6Addr":123,"HostName":"h"}`},
		{"read", "--request", `{"Zone":"is1b","IPv6Addr":123}`},
		{"update", "--request", `{"Zone":"is1b","IPv6Addr":123}`},
		{"delete", "--request", `{"Zone":"is1b","IPv6Addr":"2001:db8::1","FailIfNotFound":"yes"}`},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			commandLine := newCLI()
			testDouble := &ipv6AddrTestDouble{}
			setIPv6AddrAPIFactory(commandLine, func() (ipv6addrapi.API, error) { return testDouble, nil })
			var stdout, stderr bytes.Buffer
			code := runCLI(append([]string{"iaas-api", "ipv6addr"}, args...), &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
		})
	}
	require.EqualValues(t, 0, len(api.findRequests)+len(api.createRequests)+len(api.readRequests)+len(api.updateRequests)+len(api.deleteRequests), "invalid requests reached API: %+v", api)
}

func TestIaaSIPv6AddrPropagatesAPIErrors(t *testing.T) {
	api := &ipv6AddrTestDouble{findErr: errors.New("ipv6addr API failure")}
	commandLine := newCLI()
	setIPv6AddrAPIFactory(commandLine, func() (ipv6addrapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "ipv6addr", "find", "--zone", "is1b"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "code %d stdout %q stderr %q; want API error", code, stdout.String(), stderr.String())
	require.Contains(t, stderr.String(), "ipv6addr API failure", "stderr = %q, want API error", stderr.String())
}
