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
	diskplanapi "github.com/sacloud/skr/internal/iaas/diskplanapi"
	"github.com/stretchr/testify/require"
)

func TestDiskPlanGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-product-disk.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/diskplanapi/diskplan_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated DiskPlan commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-product-disk.json API_OUTPUT=internal/iaas/diskplanapi/diskplan_api_generated.go")
}

func TestIaaSDiskPlanHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"archive", "product"}},
		{args: []string{"iaas-api", "product", "--help"}, want: []string{"disk", "server"}},
		{args: []string{"iaas-api", "product", "disk", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "product", "disk", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "Filter", "Availability", "Size", "Total", "DiskPlans"}},
		{args: []string{"iaas-api", "product", "disk", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
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

func TestIaaSDiskPlanCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &diskPlanAPITestDouble{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "product", "disk"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("find", "--zone", "test-zone", "--count", "3", "--from", "1", "--query", ".DiskPlans | map({ID,Name,StorageClass,Availability,Size})")
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.findRequest
	require.NotNil(t, request, "Find request = %+v, want zone, count, and offset", request)
	require.Equal(t, "test-zone", request.Zone, "unexpected value")
	require.EqualValues(t, 3, request.Count, "unexpected value")
	require.EqualValues(t, 1, request.From, "unexpected value")
	var projected []struct {
		ID           json.Number
		Name         string
		StorageClass string
		Availability string
		Size         []struct {
			Availability  string
			DisplaySize   int
			DisplaySuffix string
			SizeMB        int
		}
	}
	err := json.Unmarshal([]byte(output), &projected)
	require.NoError(t, err)
	if len(projected) != 1 || projected[0].ID.String() != "4" ||
		projected[0].Name != "ssd" || projected[0].StorageClass != "ssd" ||
		projected[0].Availability != "available" || len(projected[0].Size) != 1 ||
		projected[0].Size[0].DisplaySize != 20 || projected[0].Size[0].DisplaySuffix != "GB" ||
		projected[0].Size[0].SizeMB != 20480 {
		t.Fatalf("projected DiskPlan = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Filter":{"Name":"ssd"}}`)
	if code != 0 || stderr != "" || api.findRequest == nil ||
		api.findRequest.Filter["Name"] != "ssd" {
		t.Fatalf("find JSON: code %d, stderr %q, request %+v", code, stderr, api.findRequest)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "4")
	require.Equal(t, 0, code, "read: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"Name": "ssd"`, "missing expected text %q", `"Name": "ssd"`)
	require.NotNil(t, api.readRequest, "Read request = %+v, want zone and ID", api.readRequest)
	require.Equal(t, "test-zone", api.readRequest.Zone, "unexpected value")
	require.EqualValues(t, types.ID(4), api.readRequest.ID, "unexpected value")
}

func TestIaaSDiskPlanUsesLowLevelAPIResponse(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &diskPlanAPICaller{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) {
		return diskplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "disk", "find", "--zone", "test-zone", "--count", "3", "--from", "1"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var result iaas.DiskPlanFindResult
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err, "decode API Find result %q", stdout.String())
	if result.Total != 1 || result.From != 0 || result.Count != 1 ||
		len(result.DiskPlans) != 1 || result.DiskPlans[0].ID != types.ID(4) {
		t.Fatalf("API Find result = %+v", result)
	}
	require.Len(t, caller.calls, 1, "SDK caller requests = %+v, want one GET", caller.calls)
	require.Equal(t, "GET", caller.calls[0].method, "unexpected value")
	parsed, err := url.Parse(caller.calls[0].uri)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/disk"), "SDK request path = %q, want DiskPlan endpoint", parsed.Path)
	var condition struct {
		Count int
		From  int
	}
	err2 := json.Unmarshal(caller.calls[0].body, &condition)
	require.NoError(t, err2, "decode API Find condition %q", caller.calls[0].body)
	require.EqualValues(t, 3, condition.Count, "API Find condition = %+v, want Count 3 and From 1", condition)
	require.EqualValues(t, 1, condition.From, "unexpected value")

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "product", "disk", "read", "--zone", "test-zone", "--id", "4"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "read: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var read iaas.DiskPlan
	err3 := json.Unmarshal(stdout.Bytes(), &read)
	require.NoError(t, err3, "decode API Read result %q", stdout.String())
	require.EqualValues(t, types.ID(4), read.ID, "API Read result = %+v", read)
	require.Equal(t, "ssd", read.Name, "unexpected value")
	require.Len(t, caller.calls, 2, "SDK caller requests = %+v, want two GETs", caller.calls)
	require.Equal(t, "GET", caller.calls[1].method, "unexpected value")
	parsed, err = url.Parse(caller.calls[1].uri)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/disk/4"), "SDK request path = %q, want DiskPlan Read endpoint", parsed.Path)
}

func TestIaaSDiskPlanJSONFilterPreservesLargeNumericIDs(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &diskPlanAPICaller{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) {
		return diskplanapi.NewSDKAPI(caller), nil
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
			code := runCLI([]string{"iaas-api", "product", "disk", "find", "--request", request}, &stdout, &stderr, commandLine)
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

func TestIaaSDiskPlanEmptyFindPreservesEnvelopeAndQueryArray(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &diskPlanAPICaller{findResponse: []byte(`{"Total":0,"From":0,"Count":0,"DiskPlans":[]}`)}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) {
		return diskplanapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "disk", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "find: code %d, stderr %q", code, stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	var envelope map[string]json.RawMessage
	err := json.Unmarshal(stdout.Bytes(), &envelope)
	require.NoError(t, err, "decode empty Find envelope %q", stdout.String())
	for key, want := range map[string]string{
		"Total": "0", "From": "0", "Count": "0", "DiskPlans": "[]",
	} {
		got := strings.TrimSpace(string(envelope[key]))
		require.Equal(t, want, got, "empty Find field %s = %q, want %q", key, got, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "product", "disk", "find", "--zone", "test-zone", "--query", ".DiskPlans | map({ID,Name})"}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "empty Find query: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	require.Equal(t, "[]\n", stdout.String(), "unexpected value")
}

func TestIaaSDiskPlanRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &diskPlanAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":123}`},
		{"find", "--zone", "test-zone", "--unknown"},
		{"read", "--zone", "all", "--id", "4"},
		{"read", "--zone", "test-zone"},
		{"read", "--request", `{"Zone":123,"ID":"invalid"}`},
		{"read", "--zone", "test-zone", "--id", "4", "--request", `{"Zone":"test-zone","ID":4}`},
		{"read", "--zone", "test-zone", "--id", "4", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "product", "disk"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.Nil(t, api.findRequest, "invalid request reached API: %+v", api)
	require.Nil(t, api.readRequest, "should be nil")
}

func TestIaaSDiskPlanJSONUnknownFieldsAndTrailingValues(t *testing.T) {
	api := &diskPlanAPITestDouble{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "disk", "find", "--request", `{"Zone":"test-zone","Unknown":"ignored"}`}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "unknown JSON field: code %d, stderr %q, request %+v", code, stderr.String(), api.findRequest)
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	require.NotNil(t, api.findRequest, "should not be nil")
	require.Equal(t, "test-zone", api.findRequest.Zone, "unexpected value")

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "product", "disk", "find", "--request", `{"Zone":"test-zone"} {}`}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "trailing JSON value: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
}

func TestIaaSDiskPlanPropagatesAPIErrors(t *testing.T) {
	api := &diskPlanAPITestDouble{findErr: errors.New("disk plan API failure")}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "disk", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), "disk plan API failure", "missing expected text %q", "disk plan API failure")
}

type diskPlanAPITestDouble struct {
	findRequest *diskplanapi.FindRequest
	readRequest *diskplanapi.ReadRequest
	findErr     error
}

type diskPlanAPICaller struct {
	calls        []diskPlanAPICall
	findResponse []byte
}

type diskPlanAPICall struct {
	method string
	uri    string
	body   []byte
}

func (caller *diskPlanAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal SDK request body: %w", err)
	}
	caller.calls = append(caller.calls, diskPlanAPICall{method: method, uri: uri, body: data})
	if strings.HasSuffix(uri, "/api/cloud/1.1/product/disk/4") {
		return []byte(`{"DiskPlan":{"ID":"4","Name":"ssd","StorageClass":"ssd","Availability":"available","Size":[{"Availability":"available","DisplaySize":20,"DisplaySuffix":"GB","SizeMB":20480}]}}`), nil
	}
	if caller.findResponse != nil {
		return caller.findResponse, nil
	}
	return []byte(`{"Total":1,"From":0,"Count":1,"DiskPlans":[{"ID":"4","Name":"ssd","StorageClass":"ssd","Availability":"available","Size":[{"Availability":"available","DisplaySize":20,"DisplaySuffix":"GB","SizeMB":20480}]}]}`), nil
}

func (api *diskPlanAPITestDouble) FindWithContext(_ context.Context, request *diskplanapi.FindRequest) (*diskplanapi.FindResult, error) {
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return &diskplanapi.FindResult{
		Total: 1, From: 0, Count: 1,
		DiskPlans: []*iaas.DiskPlan{{
			ID: 4, Name: "ssd", StorageClass: "ssd", Availability: types.EAvailability("available"),
			Size: []*iaas.DiskPlanSizeInfo{{
				Availability: types.EAvailability("available"), DisplaySize: 20, DisplaySuffix: "GB", SizeMB: 20480,
			}},
		}},
	}, nil
}

func (api *diskPlanAPITestDouble) ReadWithContext(_ context.Context, request *diskplanapi.ReadRequest) (*iaas.DiskPlan, error) {
	api.readRequest = request
	return &iaas.DiskPlan{ID: request.ID, Name: "ssd", StorageClass: "ssd"}, nil
}
