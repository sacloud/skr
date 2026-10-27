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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/region"
	"github.com/sacloud/skr/internal/apigen"
	regionapi "github.com/sacloud/skr/internal/iaas/regionapi"
	"github.com/stretchr/testify/require"
)

func TestRegionGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-region.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/regionapi/region_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Region commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-region.json API_OUTPUT=internal/iaas/regionapi/region_api_generated.go")
}

func TestIaaSRegionHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"region"}},
		{args: []string{"iaas-api", "region", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "region", "find", "--help"}, want: []string{"--count", "--from", "--request", "Names", "Sort", "map({ID,Name,Description})"}},
		{args: []string{"iaas-api", "region", "read", "--help"}, want: []string{"--id", "--request", "@path.json", "REGION-ID"}},
	} {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
		help := strings.Join(strings.Fields(stdout.String()), " ")
		for _, text := range test.want {
			require.Contains(t, help, text, "help for %v does not contain %q: %s", test.args, text, help)
		}
	}
}

func TestIaaSRegionFindAndReadUseSDKRequests(t *testing.T) {
	api := &regionAPITestDouble{}
	commandLine := newCLI()
	setRegionAPIFactory(commandLine, func() (regionapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "region"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("find", "--count", "3", "--from", "1", "--query", "map({ID,Name,Description})")
	require.Equal(t, 0, code, "find flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.findRequest
	require.NotNil(t, request, "Find request = %+v, want count 3 and from 1", request)
	require.EqualValues(t, 3, request.Count, "unexpected value")
	require.EqualValues(t, 1, request.From, "unexpected value")
	var result []struct {
		ID          int64
		Name        string
		Description string
	}
	err := json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	require.Len(t, result, 1, "projected Find result = %s", output)
	require.EqualValues(t, 7, result[0].ID, "unexpected value")
	require.Equal(t, "test-region", result[0].Name, "unexpected value")
	require.Equal(t, "test description", result[0].Description, "unexpected value")

	_, stderr, code = run("find", "--request", `{"Names":["test"],"Count":2}`)
	if code != 0 || stderr != "" || api.findRequest == nil ||
		len(api.findRequest.Names) != 1 || api.findRequest.Names[0] != "test" || api.findRequest.Count != 2 {
		t.Fatalf("find JSON: code %d, stderr %q, request %+v", code, stderr, api.findRequest)
	}

	output, stderr, code = run("read", "--id", "7")
	require.Equal(t, 0, code, "read: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"Name": "test-region"`, "missing expected text %q", `"Name": "test-region"`)
	request2 := api.readRequest
	require.NotNil(t, request2, "Read request = %+v, want ID 7", request2)
	require.EqualValues(t, types.ID(7), request2.ID, "unexpected value")

	_, stderr, code = run("read", "--request", `{"ID":8}`)
	require.Equal(t, 0, code, "read JSON: code %d, stderr %q, request %+v", code, stderr, api.readRequest)
	require.Empty(t, stderr, "should be empty")
	require.NotNil(t, api.readRequest, "should not be nil")
	require.EqualValues(t, types.ID(8), api.readRequest.ID, "unexpected value")
}

func TestIaaSRegionRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &regionAPITestDouble{}
	for _, args := range [][]string{
		{"read"},
		{"read", "--id", "invalid"},
		{"read", "--id", "7", "--request", `{"ID":7}`},
		{"read", "--request", `{"ID":"invalid"}`},
		{"find", "--request", `{"Count":"invalid"}`},
		{"find", "--request", `{"Count":1} {}`},
	} {
		commandLine := newCLI()
		setRegionAPIFactory(commandLine, func() (regionapi.API, error) { return api, nil })
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "region"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.EqualValues(t, 0, api.findCalls, "invalid requests reached API: find calls %d, read calls %d", api.findCalls, api.readCalls)
	require.EqualValues(t, 0, api.readCalls, "unexpected value")
}

func TestIaaSRegionPropagatesAPIErrors(t *testing.T) {
	api := &regionAPITestDouble{findErr: errors.New("region API failure")}
	commandLine := newCLI()
	setRegionAPIFactory(commandLine, func() (regionapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "region", "find"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), "region API failure", "missing expected text %q", "region API failure")
}

type regionAPITestDouble struct {
	findRequest *region.FindRequest
	readRequest *region.ReadRequest
	findErr     error
	findCalls   int
	readCalls   int
}

func (api *regionAPITestDouble) FindWithContext(_ context.Context, request *region.FindRequest) ([]*iaas.Region, error) {
	api.findCalls++
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.Region{{ID: 7, Name: "test-region", Description: "test description"}}, nil
}

func (api *regionAPITestDouble) ReadWithContext(_ context.Context, request *region.ReadRequest) (*iaas.Region, error) {
	api.readCalls++
	api.readRequest = request
	return &iaas.Region{ID: request.ID, Name: "test-region", Description: "test description"}, nil
}
