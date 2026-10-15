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
)

func TestArchiveGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-archive.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/archiveapi/archive_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Archive commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-archive.json API_OUTPUT=internal/iaas/archiveapi/archive_api_generated.go")
	}
}

func TestIaaSArchiveHelp(t *testing.T) {
	for _, test := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"archive", "disk-plan"}},
		{args: []string{"iaas-api", "archive", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "archive", "find", "--help"}, want: []string{"--zone", "--os-type", "--tags", "--count", "--from", "--request", "Names", "DiskPlanName", "SizeMB", "Tags", "併用できません"}, notWant: []string{"--zone all"}},
		{args: []string{"iaas-api", "archive", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
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
			for _, text := range test.notWant {
				if strings.Contains(help, text) {
					t.Errorf("help contains %q: %s", text, help)
				}
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
	if code != 0 || stderr != "" {
		t.Fatalf("find by OS type: code %d, stderr %q", code, stderr)
	}
	if request := api.findRequests[0]; request.Zone != "test-zone" || request.OSType != ostype.Ubuntu2404 || request.Count != 5 || request.From != 2 {
		t.Fatalf("Find request = %+v, want zone, OS type, count, and offset", request)
	}

	output, stderr, code := run("find", "--zone", "test-zone", "--tags", "cloud-init,public", "--query", "map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})")
	if code != 0 || stderr != "" {
		t.Fatalf("find by tags: code %d, stderr %q", code, stderr)
	}
	request := api.findRequests[1]
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
	if err := json.Unmarshal([]byte(output), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID.String() != "123" ||
		projected[0].Name != "sample-archive" || projected[0].DiskPlanID.String() != "4" ||
		projected[0].DiskPlanName != "ssd" || projected[0].SizeMB != 20480 ||
		len(projected[0].Tags) != 1 || projected[0].Tags[0] != "test" {
		t.Fatalf("projected Archive = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Names":["cloud-init"],"Tags":["public"]}`)
	if code != 0 || stderr != "" {
		t.Fatalf("find with JSON filters: code %d, stderr %q", code, stderr)
	}
	request = api.findRequests[2]
	if len(request.Names) != 1 || request.Names[0] != "cloud-init" || len(request.Tags) != 1 || request.Tags[0] != "public" {
		t.Fatalf("JSON Find request = %+v, want Names and Tags", request)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "123")
	if code != 0 || stderr != "" || !strings.Contains(output, `"Name": "sample-archive"`) {
		t.Fatalf("read: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.readRequest == nil || api.readRequest.Zone != "test-zone" || api.readRequest.ID != types.ID(123) {
		t.Fatalf("Read request = %+v, want zone and ID", api.readRequest)
	}
}

func TestIaaSArchiveRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &archiveAPITestDouble{}
	for _, args := range [][]string{
		{"find"},
		{"find", "--zone", "all"},
		{"find", "--zone", "test-zone", "--os-type", "not-a-supported-os"},
		{"find", "--zone", "test-zone", "--os-type", "ubuntu2404", "--tags", "public"},
		{"find", "--request", `{"Zone":"all"}`},
		{"find", "--request", `{"Zone":"test-zone","OSType":1000}`},
		{"find", "--request", `{"Zone":"test-zone","OSType":2,"Names":["image"]}`},
		{"find", "--request", `{"Zone":123}`},
		{"find", "--zone", "test-zone", "--unknown"},
		{"read", "--zone", "all", "--id", "123"},
		{"read", "--zone", "test-zone", "--id", "0"},
		{"read", "--zone", "test-zone"},
		{"read", "--request", `{"Zone":123,"ID":"invalid"}`},
		{"read", "--zone", "test-zone", "--id", "123", "--request", `{"Zone":"test-zone","ID":123}`},
		{"read", "--zone", "test-zone", "--id", "123", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "archive"}, args...), &stdout, &stderr, commandLine)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
	}
	if len(api.findRequests) != 0 || api.readRequest != nil {
		t.Fatalf("invalid request reached API: %+v", api)
	}
}

func TestIaaSArchiveJSONUnknownFieldsAndTrailingValues(t *testing.T) {
	api := &archiveAPITestDouble{}
	commandLine := newCLI()
	setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "archive", "find", "--request", `{"Zone":"test-zone","Unknown":"ignored"}`}, &stdout, &stderr, commandLine)
	if code != 0 || stderr.Len() != 0 || len(api.findRequests) != 1 || api.findRequests[0].Zone != "test-zone" {
		t.Fatalf("unknown JSON field: code %d, stderr %q, requests %+v", code, stderr.String(), api.findRequests)
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "archive", "find", "--request", `{"Zone":"test-zone"} {}`}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 || len(api.findRequests) != 1 {
		t.Fatalf("trailing JSON value: code %d, stdout %q, stderr %q, requests %+v", code, stdout.String(), stderr.String(), api.findRequests)
	}
}

func TestIaaSArchivePropagatesAPIErrors(t *testing.T) {
	api := &archiveAPITestDouble{findErr: errors.New("archive API failure")}
	commandLine := newCLI()
	setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "archive", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "archive API failure") {
		t.Fatalf("find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
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
