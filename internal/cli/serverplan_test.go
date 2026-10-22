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
)

func TestServerPlanGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-product-server.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := apigen.DecodeConfig(configData)
	if err != nil {
		t.Fatal(err)
	}
	want, err := apigen.Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(repositoryPath("internal/iaas/serverplanapi/serverplan_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated ServerPlan commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-product-server.json API_OUTPUT=internal/iaas/serverplanapi/serverplan_api_generated.go")
	}
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
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) = %d; stderr: %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help does not contain %q: %s", text, help)
				}
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
	if code != 0 || stderr != "" {
		t.Fatalf("find: code %d, stderr %q", code, stderr)
	}
	if request := api.findRequest; request == nil || request.Zone != "test-zone" || request.Count != 3 || request.From != 1 {
		t.Fatalf("Find request = %+v, want zone, count, and offset", request)
	}
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
	if err := json.Unmarshal([]byte(output), &projected); err != nil {
		t.Fatal(err)
	}
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
	if code != 0 || stderr != "" || !strings.Contains(output, `"Name": "test-plan"`) {
		t.Fatalf("read: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.readRequest == nil || api.readRequest.Zone != "test-zone" || api.readRequest.ID != types.ID(17) {
		t.Fatalf("Read request = %+v, want zone and ID", api.readRequest)
	}

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
	code := runCLI([]string{"iaas-api", "product", "server", "find", "--request", `{"Zone":"test-zone","Filter":{"Name":"test-plan"}}`}, &stdout, &stderr, commandLine)
	if code != 0 || stderr.Len() != 0 || len(caller.calls) != 1 {
		t.Fatalf("find: code %d, stderr %q, calls %+v", code, stderr.String(), caller.calls)
	}
	var condition map[string]any
	if err := json.Unmarshal(caller.calls[0].body, &condition); err != nil {
		t.Fatalf("decode SDK find condition %q: %v", caller.calls[0].body, err)
	}
	filter, ok := condition["Filter"].(map[string]any)
	if !ok || filter["Name"] == nil {
		t.Fatalf("SDK FindCondition = %s, want Name filter", caller.calls[0].body)
	}
	if _, ok := condition["Names"]; ok {
		t.Fatalf("API FindCondition contains non-API field Names: %s", caller.calls[0].body)
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
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
	}
	if api.findRequest != nil || api.readRequest != nil {
		t.Fatalf("invalid request reached API: %+v", api)
	}
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
			if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), test.wantErr) {
				t.Fatalf("code %d, stdout %q, stderr %q; want %q", code, stdout.String(), stderr.String(), test.wantErr)
			}
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
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("find: code %d, stderr %q", code, stderr.String())
	}
	var result iaas.ServerPlanFindResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode SDK result %q: %v", stdout.String(), err)
	}
	if result.Total != 1 || result.From != 0 || result.Count != 1 ||
		len(result.ServerPlans) != 1 || result.ServerPlans[0].ID != types.ID(17) ||
		result.ServerPlans[0].Name != "test-plan" || result.ServerPlans[0].CPU != 4 ||
		result.ServerPlans[0].MemoryMB != 8192 {
		t.Fatalf("SDK-decoded ServerPlan = %+v", result)
	}
	if len(caller.calls) != 1 || caller.calls[0].method != "GET" {
		t.Fatalf("SDK caller requests = %+v, want one GET", caller.calls)
	}
	parsed, err := url.Parse(caller.calls[0].uri)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/server") {
		t.Fatalf("SDK request path = %q, want ServerPlan endpoint", parsed.Path)
	}
	var condition struct {
		Count int
		From  int
	}
	if err := json.Unmarshal(caller.calls[0].body, &condition); err != nil {
		t.Fatalf("decode SDK find condition %q: %v", caller.calls[0].body, err)
	}
	if condition.Count != 3 || condition.From != 1 {
		t.Fatalf("SDK FindCondition = %+v, want Count 3 and From 1", condition)
	}
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
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("read: code %d, stderr %q", code, stderr.String())
	}
	var result iaas.ServerPlan
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode SDK result %q: %v", stdout.String(), err)
	}
	if result.ID != types.ID(17) || result.Name != "test-plan" || result.CPU != 4 || result.MemoryMB != 8192 {
		t.Fatalf("SDK-decoded ServerPlan = %+v", result)
	}
	if len(caller.calls) != 1 || caller.calls[0].method != "GET" {
		t.Fatalf("SDK caller requests = %+v, want one GET", caller.calls)
	}
	parsed, err := url.Parse(caller.calls[0].uri)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/server/17") {
		t.Fatalf("SDK request path = %q, want ServerPlan Read endpoint", parsed.Path)
	}
}

type serverPlanAPITestDouble struct {
	findRequest *serverplanapi.FindRequest
	readRequest *serverplanapi.ReadRequest
	findErr     error
	readErr     error
}

func (api *serverPlanAPITestDouble) FindWithContext(_ context.Context, request *serverplanapi.FindRequest) (*iaas.ServerPlanFindResult, error) {
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return &iaas.ServerPlanFindResult{
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
	calls []serverPlanAPICall
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
	return []byte(`{"Total":1,"From":0,"Count":1,"ServerPlans":[{"ID":"17","Name":"test-plan","CPU":4,"MemoryMB":8192}]}`), nil
}
