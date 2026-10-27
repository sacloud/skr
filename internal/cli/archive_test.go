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
	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
	"github.com/sacloud/skr/internal/apigen"
	archiveapi "github.com/sacloud/skr/internal/iaas/archiveapi"
	"github.com/stretchr/testify/require"
)

func TestArchiveGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-archive.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/archiveapi/archive_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Archive commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-archive.json API_OUTPUT=internal/iaas/archiveapi/archive_api_generated.go")
}

func TestIaaSArchiveHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"archive", "product"}},
		{args: []string{"iaas-api", "archive", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "archive", "find", "--help"}, want: []string{"--zone", "--os-type", "--tags", "--count", "--from", "--request", "Names", "DiskPlanName", "SizeMB", "Tags", "上書き"}, notWant: []string{"--zone all", "併用できません"}},
		{args: []string{"iaas-api", "archive", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help does not contain %q: %s", text, help)
			}
			for _, text := range test.notWant {
				require.NotContains(t, help, text, "help contains %q: %s", text, help)
			}
		})
	}
}

func TestIaaSArchiveCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &archiveAPITestDouble{}
	commandLine := newCLI()
	setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })

	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "archive"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	_, stderr, code := run("find", "--zone", "test-zone", "--os-type", "ubuntu2404", "--count", "5", "--from", "2")
	require.Equal(t, 0, code, "find by OS type: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.findRequests[0]
	require.Equal(t, "test-zone", request.Zone, "Find request = %+v, want zone, OS type, count, and offset", request)
	require.Equal(t, ostype.Ubuntu2404, request.OSType, "unexpected value")
	require.EqualValues(t, 5, request.Count, "unexpected value")
	require.EqualValues(t, 2, request.From, "unexpected value")

	output, stderr, code := run("find", "--zone", "test-zone", "--tags", "cloud-init,public", "--query", "map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})")
	require.Equal(t, 0, code, "find by tags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request = api.findRequests[1]
	if request.Zone != "test-zone" || request.OSType != ostype.Custom ||
		len(request.Tags) != 2 || request.Tags[0] != "cloud-init" || request.Tags[1] != "public" {
		t.Fatalf("Find request = %+v, want tags and no OS type", request)
	}
	var projected []struct {
		ID           json.Number
		Name         string
		Description  string
		DiskPlanID   json.Number
		DiskPlanName string
		SizeMB       int
		Tags         []string
	}
	err := json.Unmarshal([]byte(output), &projected)
	require.NoError(t, err)
	if len(projected) != 1 || projected[0].ID.String() != "123" ||
		projected[0].Name != "sample-archive" || projected[0].DiskPlanID.String() != "4" ||
		projected[0].DiskPlanName != "ssd" || projected[0].SizeMB != 20480 ||
		len(projected[0].Tags) != 1 || projected[0].Tags[0] != "test" {
		t.Fatalf("projected Archive = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Names":["cloud-init"],"Tags":["public"]}`)
	require.Equal(t, 0, code, "find with JSON filters: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request = api.findRequests[2]
	require.Len(t, request.Names, 1, "JSON Find request = %+v, want Names and Tags", request)
	require.Equal(t, "cloud-init", request.Names[0], "unexpected value")
	require.Len(t, request.Tags, 1, "unexpected length")
	require.Equal(t, "public", request.Tags[0], "unexpected value")

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "123")
	require.Equal(t, 0, code, "read: code %d, stderr %q, output %q", code, stderr, output)
	require.Empty(t, stderr, "should be empty")
	require.Contains(t, output, `"Name": "sample-archive"`, "missing expected text %q", `"Name": "sample-archive"`)
	require.NotNil(t, api.readRequest, "Read request = %+v, want zone and ID", api.readRequest)
	require.Equal(t, "test-zone", api.readRequest.Zone, "unexpected value")
	require.EqualValues(t, types.ID(123), api.readRequest.ID, "unexpected value")
}

func TestIaaSArchiveRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &archiveAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--zone", "test-zone", "--os-type", "not-a-supported-os"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":"test-zone","OSType":1000}`},
		{"find", "--request", `{"Zone":123}`},
		{"find", "--zone", "test-zone", "--unknown"},
		{"read", "--zone", "all", "--id", "123"},
		{"read", "--zone", "test-zone"},
		{"read", "--request", `{"Zone":123,"ID":"invalid"}`},
		{"read", "--zone", "test-zone", "--id", "123", "--request", `{"Zone":"test-zone","ID":123}`},
		{"read", "--zone", "test-zone", "--id", "123", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "archive"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.Len(t, api.findRequests, 0, "invalid request reached API: %+v", api)
	require.Nil(t, api.readRequest, "should be nil")
}

func TestIaaSArchiveJSONUnknownFieldsAndTrailingValues(t *testing.T) {
	api := &archiveAPITestDouble{}
	commandLine := newCLI()
	setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "archive", "find", "--request", `{"Zone":"test-zone","Unknown":"ignored"}`}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "unknown JSON field: code %d, stderr %q, requests %+v", code, stderr.String(), api.findRequests)
	require.EqualValues(t, 0, stderr.Len(), "unexpected value")
	require.Len(t, api.findRequests, 1, "unexpected length")
	require.Equal(t, "test-zone", api.findRequests[0].Zone, "unexpected value")

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "archive", "find", "--request", `{"Zone":"test-zone"} {}`}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "trailing JSON value: code %d, stdout %q, stderr %q, requests %+v", code, stdout.String(), stderr.String(), api.findRequests)
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	require.Len(t, api.findRequests, 1, "unexpected length")
}

func TestIaaSArchivePropagatesAPIErrors(t *testing.T) {
	api := &archiveAPITestDouble{findErr: errors.New("archive API failure")}
	commandLine := newCLI()
	setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "archive", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), "archive API failure", "missing expected text %q", "archive API failure")
}

type archiveAPITestDouble struct {
	findRequests []*archive.FindRequest
	readRequest  *archive.ReadRequest
	findErr      error
}

func (api *archiveAPITestDouble) FindWithContext(_ context.Context, request *archive.FindRequest) ([]*iaas.Archive, error) {
	api.findRequests = append(api.findRequests, request)
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.Archive{{
		ID: 123, Name: "sample-archive", Description: "test archive", Tags: types.Tags{"test"},
		DiskPlanID: 4, DiskPlanName: "ssd", SizeMB: 20480,
	}}, nil
}

func (api *archiveAPITestDouble) ReadWithContext(_ context.Context, request *archive.ReadRequest) (*iaas.Archive, error) {
	api.readRequest = request
	return &iaas.Archive{ID: request.ID, Name: "sample-archive", DiskPlanID: 4, DiskPlanName: "ssd", SizeMB: 20480}, nil
}
