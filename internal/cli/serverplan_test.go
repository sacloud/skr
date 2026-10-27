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
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/apigen"
	serverplanapi "github.com/sacloud/skr/internal/iaas/serverplanapi"
	"github.com/stretchr/testify/require"
)

func TestServerPlanGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-product-server.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/serverplanapi/serverplan_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated ServerPlan commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-product-server.json API_OUTPUT=internal/iaas/serverplanapi/serverplan_api_generated.go")
}

func TestIaaSServerPlanHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"product"}},
		{args: []string{"iaas-api", "product", "--help"}, want: []string{"disk", "server"}},
		{args: []string{"iaas-api", "product", "server", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "product", "server", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "Filter", "Sort", "@path.json", "Total", "ServerPlans"}},
		{args: []string{"iaas-api", "product", "server", "read", "--help"}, want: []string{"--zone", "--id", "--request", "@path.json"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help does not contain %q: %s", text, help)
			}
		})
	}
}

func TestIaaSServerPlanCommandsUseSDKRequests(t *testing.T) {
	api := &serverPlanAPITestDouble{}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "product", "server"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("find", "--zone", "test-zone", "--count", "3", "--from", "1", "--query", ".ServerPlans | map({ID,Name,CPU,MemoryMB,GPU,CPUModel,GPUModel,Commitment,Generation,ConfidentialVM,Availability})")
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.findRequest
	require.NotNil(t, request, "Find request = %+v, want zone, count, and offset", request)
	require.Equal(t, "test-zone", request.Zone, "unexpected value")
	require.EqualValues(t, 3, request.Count, "unexpected value")
	require.EqualValues(t, 1, request.From, "unexpected value")
	var projected []struct {
		ID             json.Number
		Name           string
		CPU            int
		MemoryMB       int
		GPU            int
		CPUModel       string
		GPUModel       string
		Commitment     types.ECommitment
		Generation     types.EPlanGeneration
		ConfidentialVM bool
		Availability   types.EAvailability
	}
	err := json.Unmarshal([]byte(output), &projected)
	require.NoError(t, err)
	if len(projected) != 1 || projected[0].ID.String() != "17" || projected[0].Name != "test-plan" ||
		projected[0].CPU != 4 || projected[0].MemoryMB != 8192 {
		t.Fatalf("projected ServerPlan = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"json-zone","Filter":{"Name":"test-plan"},"Count":2}`)
	if code != 0 || stderr != "" || api.findRequest == nil ||
		api.findRequest.Zone != "json-zone" || api.findRequest.Filter["Name"] != "test-plan" ||
		api.findRequest.Count != 2 {
		t.Fatalf("find JSON: code %d, stderr %q, request %+v", code, stderr, api.findRequest)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "17")
	require.Equal(t, 0, code, "read: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"Name": "test-plan"`, "missing expected text %q", `"Name": "test-plan"`)
	require.NotNil(t, api.readRequest, "Read request = %+v, want zone and ID", api.readRequest)
	require.Equal(t, "test-zone", api.readRequest.Zone, "unexpected value")
	require.EqualValues(t, types.ID(17), api.readRequest.ID, "unexpected value")

	_, stderr, code = run("read", "--request", `{"Zone":"json-zone","ID":17}`)
	if code != 0 || stderr != "" || api.readRequest == nil ||
		api.readRequest.Zone != "json-zone" || api.readRequest.ID != types.ID(17) {
		t.Fatalf("read JSON: code %d, stderr %q, request %+v", code, stderr, api.readRequest)
	}
}

func TestIaaSServerPlanFilterIsPassedToLowLevelAPI(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverPlanAPICaller{}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) {
		return serverplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "server", "find", "--request", `{"Zone":"test-zone","Filter":{"Name":"test-plan","CPU>=":4}}`}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "find: code %d, stderr %q, calls %+v", code, stderr.String(), caller.calls)
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	require.Len(t, caller.calls, 1, "unexpected length")
	var condition struct {
		Filter map[string]json.RawMessage
	}
	err := json.Unmarshal(caller.calls[0].body, &condition)
	require.NoError(t, err, "decode SDK find condition %q", caller.calls[0].body)
	got := string(condition.Filter["Name"])
	require.Equal(t, `"test-plan"`, got, "SDK Name filter = %s, want partial-match string", got)
	got2 := string(condition.Filter["CPU>="])
	require.Equal(t, `"4"`, got2, "SDK CPU filter = %s, want comparison value", got2)
}

func TestIaaSServerPlanJSONFilterPreservesLargeNumericIDs(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverPlanAPICaller{}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) {
		return serverplanapi.NewSDKAPI(caller), nil
	})

	for _, test := range []struct {
		name       string
		filter     string
		wantIDJSON string
	}{
		{
			name:       "scalar",
			filter:     `"ID":1234567890123456789`,
			wantIDJSON: `["1234567890123456789"]`,
		},
		{
			name:       "array",
			filter:     `"ID":[1234567890123456789,1234567890123456790]`,
			wantIDJSON: `["1234567890123456789","1234567890123456790"]`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			request := fmt.Sprintf(`{"Zone":"test-zone","Filter":{%s}}`, test.filter)
			code := runCLI([]string{"iaas-api", "product", "server", "find", "--request", request}, &stdout, &stderr, commandLine)
			require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
			require.EqualValues(t, 0, stderr.Len(), "unexpected value")
			call := caller.calls[len(caller.calls)-1]
			var condition struct {
				Filter map[string]json.RawMessage
			}
			err := json.Unmarshal(call.body, &condition)
			require.NoError(t, err, "decode SDK find condition %q", call.body)
			got := string(condition.Filter["ID"])
			require.Equal(t, test.wantIDJSON, got, "SDK ID filter = %s, want %s", got, test.wantIDJSON)
		})
	}
}

func TestIaaSServerPlanRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &serverPlanAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":123}`},
		{"find", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
		{"find", "--request", `{"Zone":"test-zone"} {}`},
		{"read", "--zone", "all", "--id", "17"},
		{"read", "--zone", "test-zone"},
		{"read", "--request", `{"Zone":"test-zone","ID":"invalid"}`},
		{"read", "--zone", "test-zone", "--id", "17", "--request", `{"Zone":"test-zone","ID":17}`},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "product", "server"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.Nil(t, api.findRequest, "invalid request reached API: %+v", api)
	require.Nil(t, api.readRequest, "should be nil")
}

func TestIaaSServerPlanPropagatesAPIErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		api     *serverPlanAPITestDouble
		wantErr string
	}{
		{name: "find", args: []string{"find", "--zone", "test-zone"}, api: &serverPlanAPITestDouble{findErr: errors.New("server plan Find failure")}, wantErr: "server plan Find failure"},
		{name: "read", args: []string{"read", "--zone", "test-zone", "--id", "17"}, api: &serverPlanAPITestDouble{readErr: errors.New("server plan Read failure")}, wantErr: "server plan Read failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			commandLine := newCLI()
			setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) { return test.api, nil })
			var stdout, stderr bytes.Buffer
			args := append([]string{"iaas-api", "product", "server"}, test.args...)
			code := runCLI(args, &stdout, &stderr, commandLine)
			require.NotEqual(t, 0, code, "code %d, stdout %q, stderr %q; want %q", code, stdout.String(), stderr.String(), test.wantErr)
			require.EqualValues(t, 0, stdout.Len(), "unexpected value")
			require.Contains(t, stderr.String(), test.wantErr, "missing expected text %q", test.wantErr)
		})
	}
}

func TestIaaSServerPlanFindUsesSDKCaller(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverPlanAPICaller{}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) {
		return serverplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "server", "find", "--zone", "test-zone", "--count", "3", "--from", "1"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var result iaas.ServerPlanFindResult
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err, "decode SDK result %q", stdout.String())
	if result.Total != 1 || result.From != 0 || result.Count != 1 ||
		len(result.ServerPlans) != 1 || result.ServerPlans[0].ID != types.ID(17) ||
		result.ServerPlans[0].Name != "test-plan" || result.ServerPlans[0].CPU != 4 ||
		result.ServerPlans[0].MemoryMB != 8192 {
		t.Fatalf("SDK-decoded ServerPlan = %+v", result)
	}
	require.Len(t, caller.calls, 1, "SDK caller requests = %+v, want one GET", caller.calls)
	require.Equal(t, "GET", caller.calls[0].method, "unexpected value")
	parsed, err := url.Parse(caller.calls[0].uri)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/server"), "SDK request path = %q, want ServerPlan endpoint", parsed.Path)
	var condition struct {
		Count int
		From  int
	}
	err2 := json.Unmarshal(caller.calls[0].body, &condition)
	require.NoError(t, err2, "decode SDK find condition %q", caller.calls[0].body)
	require.EqualValues(t, 3, condition.Count, "SDK FindCondition = %+v, want Count 3 and From 1", condition)
	require.EqualValues(t, 1, condition.From, "unexpected value")
}

func TestIaaSServerPlanEmptyFindPreservesEnvelopeAndQueryArray(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverPlanAPICaller{findResponse: []byte(`{"Total":0,"From":0,"Count":0,"ServerPlans":[]}`)}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) {
		return serverplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "server", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var envelope map[string]json.RawMessage
	err := json.Unmarshal(stdout.Bytes(), &envelope)
	require.NoError(t, err, "decode empty Find envelope %q", stdout.String())
	for key, want := range map[string]string{
		"Total": "0", "From": "0", "Count": "0", "ServerPlans": "[]",
	} {
		got := strings.TrimSpace(string(envelope[key]))
		require.Equal(t, want, got, "empty Find field %s = %q, want %q", key, got, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "product", "server", "find", "--zone", "test-zone", "--query", ".ServerPlans | map({ID,Name})"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "empty Find query: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	require.Equal(t, "[]\n", stdout.String(), "unexpected value")
}

func TestIaaSServerPlanReadUsesSDKCaller(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &serverPlanAPICaller{}
	commandLine := newCLI()
	setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) {
		return serverplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "server", "read", "--zone", "test-zone", "--id", "17"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "read: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var result iaas.ServerPlan
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err, "decode SDK result %q", stdout.String())
	require.EqualValues(t, types.ID(17), result.ID, "SDK-decoded ServerPlan = %+v", result)
	require.Equal(t, "test-plan", result.Name, "unexpected value")
	require.EqualValues(t, 4, result.CPU, "unexpected value")
	require.EqualValues(t, 8192, result.MemoryMB, "unexpected value")
	require.Len(t, caller.calls, 1, "SDK caller requests = %+v, want one GET", caller.calls)
	require.Equal(t, "GET", caller.calls[0].method, "unexpected value")
	parsed, err := url.Parse(caller.calls[0].uri)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/server/17"), "SDK request path = %q, want ServerPlan Read endpoint", parsed.Path)
}

type serverPlanAPITestDouble struct {
	findRequest *serverplanapi.FindRequest
	readRequest *serverplanapi.ReadRequest
	findErr     error
	readErr     error
}

func (api *serverPlanAPITestDouble) FindWithContext(_ context.Context, request *serverplanapi.FindRequest) (*serverplanapi.FindResult, error) {
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return &serverplanapi.FindResult{
		Total: 1, From: 0, Count: 1,
		ServerPlans: []*iaas.ServerPlan{{
			ID: 17, Name: "test-plan", CPU: 4, MemoryMB: 8192, CPUModel: "test-cpu",
		}},
	}, nil
}

func (api *serverPlanAPITestDouble) ReadWithContext(_ context.Context, request *serverplanapi.ReadRequest) (*iaas.ServerPlan, error) {
	api.readRequest = request
	if api.readErr != nil {
		return nil, api.readErr
	}
	return &iaas.ServerPlan{ID: request.ID, Name: "test-plan", CPU: 4, MemoryMB: 8192}, nil
}

type serverPlanAPICaller struct {
	calls        []serverPlanAPICall
	findResponse []byte
}

type serverPlanAPICall struct {
	method string
	uri    string
	body   []byte
}

func (caller *serverPlanAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal SDK request body: %w", err)
	}
	caller.calls = append(caller.calls, serverPlanAPICall{method: method, uri: uri, body: data})
	if strings.HasSuffix(uri, "/api/cloud/1.1/product/server/17") {
		return []byte(`{"ServerPlan":{"ID":"17","Name":"test-plan","CPU":4,"MemoryMB":8192}}`), nil
	}
	if caller.findResponse != nil {
		return caller.findResponse, nil
	}
	return []byte(`{"Total":1,"From":0,"Count":1,"ServerPlans":[{"ID":"17","Name":"test-plan","CPU":4,"MemoryMB":8192}]}`), nil
}
