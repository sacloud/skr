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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/serviceclass"
	"github.com/sacloud/skr/internal/apigen"
	serviceclassapi "github.com/sacloud/skr/internal/iaas/serviceclassapi"
	"github.com/stretchr/testify/require"
)

func TestServiceClassGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-serviceclass.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/serviceclassapi/serviceclass_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated ServiceClass commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-serviceclass.json API_OUTPUT=internal/iaas/serviceclassapi/serviceclass_api_generated.go")
}

func TestIaaSServiceClassHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"service-class"}},
		{args: []string{"iaas-api", "service-class", "--help"}, want: []string{"find"}},
		{args: []string{"iaas-api", "service-class", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "Sort", "@path.json"}},
	} {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
		help := strings.Join(strings.Fields(stdout.String()), " ")
		for _, text := range test.want {
			require.Contains(t, help, text, "help for %v does not contain %q: %s", test.args, text, help)
		}
	}
}

func TestIaaSServiceClassFindUsesSDKRequests(t *testing.T) {
	api := &serviceClassAPITestDouble{}
	commandLine := newCLI()
	setServiceClassAPIFactory(commandLine, func() (serviceclassapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "service-class"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("find", "--zone", "test-zone", "--count", "3", "--from", "1", "--query", "map({ID,ServiceClassName,ServiceClassPath,DisplayName,IsPublic})")
	require.Equal(t, 0, code, "find flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.findRequest
	require.NotNil(t, request, "Find request = %+v, want zone, count, and offset", request)
	require.Equal(t, "test-zone", request.Zone, "unexpected value")
	require.EqualValues(t, 3, request.Count, "unexpected value")
	require.EqualValues(t, 1, request.From, "unexpected value")
	var result []struct {
		ID               int64
		ServiceClassName string
		ServiceClassPath string
		DisplayName      string
		IsPublic         bool
	}
	err := json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	if len(result) != 1 || result[0].ID != 7 || result[0].ServiceClassName != "test-class" ||
		result[0].ServiceClassPath != "cloud/test-class" || result[0].DisplayName != "Test Class" || !result[0].IsPublic {
		t.Fatalf("projected Find result = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"json-zone","Count":2,"From":4}`)
	if code != 0 || stderr != "" || api.findRequest == nil ||
		api.findRequest.Zone != "json-zone" || api.findRequest.Count != 2 || api.findRequest.From != 4 {
		t.Fatalf("find JSON: code %d, stderr %q, request %+v", code, stderr, api.findRequest)
	}
}

func TestIaaSServiceClassFindUsesSDKCaller(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	for _, test := range []struct {
		name     string
		args     []string
		zone     string
		count    int
		from     int
		wantSort bool
	}{
		{
			name:  "flags",
			args:  []string{"--zone", "flag-zone", "--count", "3", "--from", "1"},
			zone:  "flag-zone",
			count: 3,
			from:  1,
		},
		{
			name:     "JSON request",
			args:     []string{"--request", `{"Zone":"json-zone","Count":5,"From":8,"Sort":[{"Key":"Name","Order":1}]}`},
			zone:     "json-zone",
			count:    5,
			from:     8,
			wantSort: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &serviceClassAPICaller{}
			commandLine := newCLI()
			setServiceClassAPIFactory(commandLine, func() (serviceclassapi.API, error) {
				return serviceclass.New(caller), nil
			})

			var stdout, stderr bytes.Buffer
			args := append([]string{"iaas-api", "service-class", "find"}, test.args...)
			code := runCLI(args, &stdout, &stderr, commandLine)
			require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
			require.EqualValues(t, 0, stderr.Len(), "unexpected value")

			var result []iaas.ServiceClass
			err := json.Unmarshal(stdout.Bytes(), &result)
			require.NoError(t, err, "decode SDK result %q", stdout.String())
			if len(result) != 1 || result[0].ID != 17 ||
				result[0].ServiceClassName != "decoded-class" ||
				result[0].ServiceClassPath != "cloud/decoded-class" ||
				result[0].DisplayName != "Decoded Class" || !result[0].IsPublic {
				t.Fatalf("SDK-decoded result = %+v", result)
			}

			require.Len(t, caller.calls, 1, "SDK caller requests = %+v, want one GET", caller.calls)
			require.Equal(t, "GET", caller.calls[0].method, "unexpected value")
			parsed, err := url.Parse(caller.calls[0].uri)
			require.NoError(t, err)
			wantPath := "/cloud/zone/" + test.zone + "/api/cloud/1.1/public/price"
			require.True(t, strings.HasSuffix(parsed.Path, wantPath), "SDK request path = %q, want suffix %q", parsed.Path, wantPath)

			condition := caller.calls[0]
			require.Equal(t, test.count, condition.count, "SDK FindCondition = %+v, want Count %d and From %d", condition, test.count, test.from)
			require.Equal(t, test.from, condition.from, "unexpected value")
			if test.wantSort {
				require.Len(t, condition.sort, 1, "SDK FindCondition Sort = %+v, want descending Name", condition.sort)
				require.Equal(t, "-Name", condition.sort[0], "unexpected value")
			} else if len(condition.sort) != 0 {
				t.Errorf("SDK FindCondition Sort = %+v, want empty", condition.sort)
			}
		})
	}
}

func TestIaaSServiceClassRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &serviceClassAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":123}`},
		{"find", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
		{"find", "--request", `{"Zone":"test-zone"} {}`},
	} {
		commandLine := newCLI()
		setServiceClassAPIFactory(commandLine, func() (serviceclassapi.API, error) { return api, nil })
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "service-class"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.EqualValues(t, 0, api.findCalls, "invalid requests reached API: find calls %d", api.findCalls)
}

func TestIaaSServiceClassPropagatesAPIErrors(t *testing.T) {
	api := &serviceClassAPITestDouble{findErr: errors.New("serviceclass API failure")}
	commandLine := newCLI()
	setServiceClassAPIFactory(commandLine, func() (serviceclassapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "service-class", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), "serviceclass API failure", "missing expected text %q", "serviceclass API failure")
}

type serviceClassAPITestDouble struct {
	findRequest *serviceclass.FindRequest
	findErr     error
	findCalls   int
}

func (api *serviceClassAPITestDouble) FindWithContext(_ context.Context, request *serviceclass.FindRequest) ([]*iaas.ServiceClass, error) {
	api.findCalls++
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.ServiceClass{{
		ID: 7, ServiceClassName: "test-class", ServiceClassPath: "cloud/test-class",
		DisplayName: "Test Class", IsPublic: true,
	}}, nil
}

type serviceClassAPICaller struct {
	calls []serviceClassAPICall
}

type serviceClassAPICall struct {
	method string
	uri    string
	count  int
	from   int
	sort   []string
}

func (caller *serviceClassAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal SDK request body: %w", err)
	}
	var condition struct {
		Count int      `json:"Count"`
		From  int      `json:"From"`
		Sort  []string `json:"Sort"`
	}
	if err := json.Unmarshal(data, &condition); err != nil {
		return nil, fmt.Errorf("decode SDK request body %T (%s): %w", body, data, err)
	}
	caller.calls = append(caller.calls, serviceClassAPICall{
		method: method, uri: uri, count: condition.Count, from: condition.From, sort: condition.Sort,
	})
	return []byte(`{"Total":1,"Count":1,"ServiceClasses":[{"ServiceClassID":"17","ServiceClassName":"decoded-class","ServiceClassPath":"cloud/decoded-class","DisplayName":"Decoded Class","IsPublic":true}]}`), nil
}
