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
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/gslb"
	"github.com/sacloud/skr/internal/gslbapi"
	"github.com/stretchr/testify/require"
)

type gslbAPITestDouble struct {
	findRequest   *gslb.FindRequest
	readRequest   *gslb.ReadRequest
	createRequest *gslb.CreateRequest
	updateRequest *gslb.UpdateRequest
	deleteRequest *gslb.DeleteRequest
	err           error
}

func (api *gslbAPITestDouble) FindWithContext(_ context.Context, request *gslb.FindRequest) ([]*iaas.GSLB, error) {
	api.findRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.GSLB{{ID: 123, Name: "test-gslb"}}, nil
}

func (api *gslbAPITestDouble) ReadWithContext(_ context.Context, request *gslb.ReadRequest) (*iaas.GSLB, error) {
	api.readRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.GSLB{ID: request.ID, Name: "test-gslb"}, nil
}

func (api *gslbAPITestDouble) CreateWithContext(_ context.Context, request *gslb.CreateRequest) (*iaas.GSLB, error) {
	api.createRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.GSLB{ID: 123, Name: request.Name, FQDN: "site-123.gslbN.sakura.ne.jp"}, nil
}

func (api *gslbAPITestDouble) UpdateWithContext(_ context.Context, request *gslb.UpdateRequest) (*iaas.GSLB, error) {
	api.updateRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.GSLB{ID: request.ID, Name: "test-gslb"}, nil
}

func (api *gslbAPITestDouble) DeleteWithContext(_ context.Context, request *gslb.DeleteRequest) error {
	api.deleteRequest = request
	return api.err
}

func setGSLBAPIFactory(commandLine *cli, factory gslbapi.APIFactory) {
	commandLine.GSLBAPI = gslbapi.NewCommands(gslbapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, factory)
}

func TestRunGSLBAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"gslb-api", "--help"}, []string{"find", "read", "create", "update", "delete", "ヘルスチェック"}},
		{[]string{"gslb-api", "create", "--help"}, []string{"--name", "--health-check-protocol", "--destination-server", "--request", "@path.json"}},
		{[]string{"gslb-api", "update", "--help"}, []string{"--id", "--monitoring-suite-log-enabled", "一覧全体を置き換えます", "監視設定全体を置き換える", "保持したい監視項目も再指定"}},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, want := range test.want {
				require.Contains(t, help, want, "help does not contain %q: %s", want, help)
			}
		})
	}
}

func TestGSLBCommandOperations(t *testing.T) {
	api := &gslbAPITestDouble{}
	commandLine := newCLI()
	setGSLBAPIFactory(commandLine, func() (gslbapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr=%q", args, stderr.String())
		return stdout.Bytes()
	}

	listed := run("gslb-api", "find", "--name", "test-gslb", "--tags", "web,production", "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	err := json.Unmarshal(listed, &projection)
	require.NoError(t, err)
	if len(projection) != 1 || string(projection[0]["Name"]) != `"test-gslb"` {
		t.Fatalf("find output = %s", listed)
	}
	if api.findRequest == nil || len(api.findRequest.Names) != 1 || api.findRequest.Names[0] != "test-gslb" ||
		!reflectStrings(api.findRequest.Tags, []string{"web", "production"}) {
		t.Fatalf("find request = %+v", api.findRequest)
	}

	created := run("gslb-api", "create", "--name", "test-gslb", "--health-check-protocol", "http",
		"--health-check-path", "/health", "--health-check-port", "8080",
		"--destination-server", "ip-address=192.0.2.10,enabled=true,weight=5")
	var createResult iaas.GSLB
	err2 := json.Unmarshal(created, &createResult)
	require.NoError(t, err2)
	if api.createRequest == nil || api.createRequest.Name != "test-gslb" ||
		api.createRequest.HealthCheck == nil || api.createRequest.HealthCheck.Protocol != types.GSLBHealthCheckProtocols.HTTP ||
		api.createRequest.HealthCheck.Path != "/health" || api.createRequest.HealthCheck.Port != types.StringNumber(8080) ||
		len(api.createRequest.DestinationServers) != 1 || api.createRequest.DestinationServers[0].Weight != types.StringNumber(5) {
		t.Fatalf("create request = %+v", api.createRequest)
	}

	run("gslb-api", "read", "--id", "123")
	require.NotNil(t, api.readRequest, "read request = %+v", api.readRequest)
	require.EqualValues(t, 123, api.readRequest.ID, "unexpected value")

	run("gslb-api", "update", "--id", "123", "--weighted=false", "--delay-loop", "0",
		"--description", "", "--monitoring-suite-log-enabled=false")
	if api.updateRequest == nil || api.updateRequest.ID != 123 || api.updateRequest.Weighted == nil ||
		*api.updateRequest.Weighted != types.StringFalse || api.updateRequest.DelayLoop == nil ||
		*api.updateRequest.DelayLoop != 0 || api.updateRequest.Description == nil || *api.updateRequest.Description != "" ||
		api.updateRequest.MonitoringSuiteLog == nil || api.updateRequest.MonitoringSuiteLog.Enabled {
		t.Fatalf("update request = %+v", api.updateRequest)
	}
	require.Nil(t, api.updateRequest.Tags, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.HealthCheck, "should be nil")
	require.Nil(t, api.updateRequest.DestinationServers, "should be nil")

	run("gslb-api", "delete", "--id", "123", "--fail-if-not-found")
	require.NotNil(t, api.deleteRequest, "delete request = %+v", api.deleteRequest)
	require.EqualValues(t, 123, api.deleteRequest.ID, "unexpected value")
	require.True(t, api.deleteRequest.FailIfNotFound, "unexpected value")
}

func TestGSLBJSONInputAndMutualExclusion(t *testing.T) {
	api := &gslbAPITestDouble{}
	commandLine := newCLI()
	setGSLBAPIFactory(commandLine, func() (gslbapi.API, error) { return api, nil })

	run := func(args ...string) (int, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		return code, stderr.String()
	}
	code, stderr := run("gslb-api", "create", "--request",
		`{"Name":"json-gslb","HealthCheck":{"Protocol":"ping"},"DestinationServers":[]}`)
	require.Equal(t, 0, code, "JSON create code=%d stderr=%q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if api.createRequest == nil || api.createRequest.Name != "json-gslb" ||
		api.createRequest.HealthCheck == nil || api.createRequest.HealthCheck.Protocol != types.GSLBHealthCheckProtocols.Ping ||
		api.createRequest.DestinationServers == nil {
		t.Fatalf("JSON create request = %+v", api.createRequest)
	}

	for _, args := range [][]string{
		{"gslb-api", "create", "--request", `{"Name":"x"}`, "--name", "x"},
		{"gslb-api", "update", "--request", `{"ID":123}`, "--weighted=false"},
		{"gslb-api", "delete", "--request", `{"ID":123}`, "--fail-if-not-found=false"},
	} {
		code, stderr = run(args...)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stderr %q", args, code, stderr)
		require.Contains(t, stderr, "--request と個別フラグ", "missing expected text %q", "--request と個別フラグ")
	}
}

func TestGSLBCommandErrorsAreReturned(t *testing.T) {
	apiErr := errors.New("GSLB API failure")
	api := &gslbAPITestDouble{err: apiErr}
	commandLine := newCLI()
	setGSLBAPIFactory(commandLine, func() (gslbapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"gslb-api", "find"}, &stdout, &stderr, commandLine)
	if code != 1 || stdout.Len() != 0 || stderr.String() != apiErr.Error()+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

type gslbAPICaller struct {
	method string
	uri    string
	body   map[string]any
}

func (caller *gslbAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	caller.method, caller.uri = method, uri
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &caller.body); err != nil {
			return nil, err
		}
	}
	return []byte(`{"CommonServiceItem":{"ID":321,"Name":"sdk-created","Settings":{"GSLB":{"DelayLoop":10,"HealthCheck":{"Protocol":"ping"},"Servers":[{"IPAddress":"192.0.2.10","Enabled":"True","Weight":5}]}},"Status":{"FQDN":"site-321.gslbN.sakura.ne.jp"}}}`), nil
}

func TestGSLBCLIUsesSDKRequestConversion(t *testing.T) {
	caller := &gslbAPICaller{}
	commandLine := newCLI()
	setGSLBAPIFactory(commandLine, func() (gslbapi.API, error) { return gslb.New(caller), nil })

	var stdout, stderr bytes.Buffer
	args := []string{
		"gslb-api", "create", "--name", "sdk-created", "--health-check-protocol", "ping",
		"--delay-loop", "10", "--destination-server", "ip-address=192.0.2.10,weight=5",
	}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.Equal(t, "POST", caller.method, "SDK call = %s %s", caller.method, caller.uri)
	require.Contains(t, caller.uri, "/commonserviceitem", "missing expected text %q", "/commonserviceitem")
	item, ok := caller.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	settings := item["Settings"].(map[string]any)
	gslbSettings := settings["GSLB"].(map[string]any)
	healthCheck := gslbSettings["HealthCheck"].(map[string]any)
	servers := gslbSettings["Servers"].([]any)
	if item["Name"] != "sdk-created" || gslbSettings["DelayLoop"] != float64(10) ||
		healthCheck["Protocol"] != "ping" || len(servers) != 1 {
		t.Fatalf("converted SDK request = %#v", item)
	}
	server := servers[0].(map[string]any)
	if server["IPAddress"] != "192.0.2.10" || server["Enabled"] != "True" || server["Weight"] != "5" {
		t.Fatalf("converted destination server = %#v", server)
	}
	var result iaas.GSLB
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err)
	require.EqualValues(t, 321, result.ID, "decoded SDK result = %+v", result)
	require.Equal(t, "site-321.gslbN.sakura.ne.jp", result.FQDN, "unexpected value")
}

func reflectStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}
