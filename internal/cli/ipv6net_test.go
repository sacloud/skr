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
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/apigen"
	ipv6netapi "github.com/sacloud/skr/internal/iaas/ipv6netapi"
	"github.com/stretchr/testify/require"
)

func TestIPv6NetGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-ipv6net.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/ipv6netapi/ipv6net_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated IPv6Net commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-ipv6net.json API_OUTPUT=internal/iaas/ipv6netapi/ipv6net_api_generated.go")
}

func TestIaaSIPv6NetHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"ipv6net"}},
		{args: []string{"iaas-api", "ipv6net", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "ipv6net", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "SwitchIDs", "SwitchNames", "InternetIDs", "InternetNames", "Sort", "併用できません"}},
		{args: []string{"iaas-api", "ipv6net", "read", "--help"}, want: []string{"--zone", "--id", "--request", "併用できません"}},
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

type ipv6NetTestDouble struct {
	findRequests []*ipv6netapi.FindRequest
	readRequests []*ipv6netapi.ReadRequest
	findErr      error
	readErr      error
}

func (api *ipv6NetTestDouble) FindWithContext(_ context.Context, request *ipv6netapi.FindRequest) ([]*iaas.IPv6Net, error) {
	api.findRequests = append(api.findRequests, request)
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.IPv6Net{{ID: 123, IPv6Prefix: "2001:db8:", IPv6PrefixLen: 64}}, nil
}

func (api *ipv6NetTestDouble) ReadWithContext(_ context.Context, request *ipv6netapi.ReadRequest) (*iaas.IPv6Net, error) {
	api.readRequests = append(api.readRequests, request)
	if api.readErr != nil {
		return nil, api.readErr
	}
	return &iaas.IPv6Net{ID: request.ID, IPv6Prefix: "2001:db8:", IPv6PrefixLen: 64}, nil
}

func TestIaaSIPv6NetCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &ipv6NetTestDouble{}
	commandLine := newCLI()
	setIPv6NetAPIFactory(commandLine, func() (ipv6netapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "ipv6net"}, args...), &stdout, &stderr, commandLine)
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
	_, stderr, code = run("find", "--request", `{"Zone":"is1b","SwitchIDs":["123"],"SwitchNames":["sw"],"InternetIDs":["456"],"InternetNames":["rt"],"Sort":[{"Key":"IPv6Prefix","Order":1}],"Count":10,"From":20}`)
	require.Equal(t, 0, code, "find JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.findRequests, 2, "Find request = %+v, want filters and sort from JSON", api.findRequests)
	request := api.findRequests[1]
	require.Equal(t, []string{"123"}, request.SwitchIDs, "unexpected value")
	require.Equal(t, []string{"sw"}, request.SwitchNames, "unexpected value")
	require.Equal(t, []string{"456"}, request.InternetIDs, "unexpected value")
	require.Equal(t, []string{"rt"}, request.InternetNames, "unexpected value")
	require.Len(t, request.Sort, 1, "unexpected value")
	require.Equal(t, "IPv6Prefix", request.Sort[0].Key, "unexpected value")
	require.Equal(t, 10, request.Count, "unexpected value")
	require.Equal(t, 20, request.From, "unexpected value")
	var found []struct {
		ID            json.Number `json:"ID"`
		IPv6Prefix    string      `json:"IPv6Prefix"`
		IPv6PrefixLen int         `json:"IPv6PrefixLen"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &found), "output = %s", output)
	require.Len(t, found, 1, "unexpected value")
	require.Equal(t, "123", found[0].ID.String(), "unexpected value")

	// read: --request 経路
	output, stderr, code = run("read", "--request", `{"Zone":"is1b","ID":123}`)
	require.Equal(t, 0, code, "read JSON: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 1, "Read request = %+v, want values from JSON", api.readRequests)
	require.EqualValues(t, types.ID(123), api.readRequests[0].ID, "unexpected value")
	require.Contains(t, output, `"IPv6Prefix": "2001:db8:"`, "missing expected text %q: %s", `"IPv6Prefix": "2001:db8:"`, output)

	// read: 個別フラグ経路
	_, stderr, code = run("read", "--zone", "is1b", "--id", "123")
	require.Equal(t, 0, code, "read flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Len(t, api.readRequests, 2, "Read request = %+v, want values from flags", api.readRequests)
	require.EqualValues(t, types.ID(123), api.readRequests[1].ID, "unexpected value")
}

func TestIaaSIPv6NetRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &ipv6NetTestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "is1b", "--request", `{"Zone":"is1b"}`},
		{"find", "--request", `{"Zone":"is1b","SwitchIDs":[123]}`},
		{"find", "--request", `{"Zone":"is1b","Count":"many"}`},
		{"read"},
		{"read", "--zone", "is1b", "--request", `{"Zone":"is1b","ID":123}`},
		{"read", "--request", `{"Zone":"is1b","ID":"invalid"}`},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			commandLine := newCLI()
			testDouble := &ipv6NetTestDouble{}
			setIPv6NetAPIFactory(commandLine, func() (ipv6netapi.API, error) { return testDouble, nil })
			var stdout, stderr bytes.Buffer
			code := runCLI(append([]string{"iaas-api", "ipv6net"}, args...), &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
		})
	}
	require.EqualValues(t, 0, len(api.findRequests)+len(api.readRequests), "invalid requests reached API: %+v", api)
}

func TestIaaSIPv6NetPropagatesAPIErrors(t *testing.T) {
	api := &ipv6NetTestDouble{findErr: errors.New("ipv6net API failure")}
	commandLine := newCLI()
	setIPv6NetAPIFactory(commandLine, func() (ipv6netapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "ipv6net", "find", "--zone", "is1b"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "code %d stdout %q stderr %q; want API error", code, stdout.String(), stderr.String())
	require.Contains(t, stderr.String(), "ipv6net API failure", "stderr = %q, want API error", stderr.String())
}
