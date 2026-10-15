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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
	"github.com/sacloud/skr/internal/apigen"
	diskplanapi "github.com/sacloud/skr/internal/iaas/diskplanapi"
)

func TestDiskPlanGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-diskplan.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/diskplanapi/diskplan_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated DiskPlan commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-diskplan.json API_OUTPUT=internal/iaas/diskplanapi/diskplan_api_generated.go")
	}
}

func TestIaaSDiskPlanHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"archive", "disk-plan"}},
		{args: []string{"iaas-api", "disk-plan", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "disk-plan", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "Names", "Availability", "Size"}},
		{args: []string{"iaas-api", "disk-plan", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
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

func TestIaaSDiskPlanCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &diskPlanAPITestDouble{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "disk-plan"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("find", "--zone", "test-zone", "--count", "3", "--from", "1", "--query", "map({ID,Name,StorageClass,Availability,Size})")
	if code != 0 || stderr != "" {
		t.Fatalf("find: code %d, stderr %q", code, stderr)
	}
	if request := api.findRequest; request == nil || request.Zone != "test-zone" || request.Count != 3 || request.From != 1 {
		t.Fatalf("Find request = %+v, want zone, count, and offset", request)
	}
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
	if err := json.Unmarshal([]byte(output), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID.String() != "4" ||
		projected[0].Name != "ssd" || projected[0].StorageClass != "ssd" ||
		projected[0].Availability != "available" || len(projected[0].Size) != 1 ||
		projected[0].Size[0].DisplaySize != 20 || projected[0].Size[0].DisplaySuffix != "GB" ||
		projected[0].Size[0].SizeMB != 20480 {
		t.Fatalf("projected DiskPlan = %s", output)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Names":["ssd"]}`)
	if code != 0 || stderr != "" || api.findRequest == nil ||
		len(api.findRequest.Names) != 1 || api.findRequest.Names[0] != "ssd" {
		t.Fatalf("find JSON: code %d, stderr %q, request %+v", code, stderr, api.findRequest)
	}

	output, stderr, code = run("read", "--zone", "test-zone", "--id", "4")
	if code != 0 || stderr != "" || !strings.Contains(output, `"Name": "ssd"`) {
		t.Fatalf("read: code %d, stderr %q, output %q", code, stderr, output)
	}
	if api.readRequest == nil || api.readRequest.Zone != "test-zone" || api.readRequest.ID != types.ID(4) {
		t.Fatalf("Read request = %+v, want zone and ID", api.readRequest)
	}
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
		{"read", "--zone", "test-zone", "--id", "0"},
		{"read", "--zone", "test-zone"},
		{"read", "--request", `{"Zone":123,"ID":"invalid"}`},
		{"read", "--zone", "test-zone", "--id", "4", "--request", `{"Zone":"test-zone","ID":4}`},
		{"read", "--zone", "test-zone", "--id", "4", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
		code := runCLI(append([]string{"iaas-api", "disk-plan"}, args...), &stdout, &stderr, commandLine)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
	}
	if api.findRequest != nil || api.readRequest != nil {
		t.Fatalf("invalid request reached API: %+v", api)
	}
}

func TestIaaSDiskPlanJSONUnknownFieldsAndTrailingValues(t *testing.T) {
	api := &diskPlanAPITestDouble{}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "disk-plan", "find", "--request", `{"Zone":"test-zone","Unknown":"ignored"}`}, &stdout, &stderr, commandLine)
	if code != 0 || stderr.Len() != 0 || api.findRequest == nil || api.findRequest.Zone != "test-zone" {
		t.Fatalf("unknown JSON field: code %d, stderr %q, request %+v", code, stderr.String(), api.findRequest)
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "disk-plan", "find", "--request", `{"Zone":"test-zone"} {}`}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("trailing JSON value: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestIaaSDiskPlanPropagatesAPIErrors(t *testing.T) {
	api := &diskPlanAPITestDouble{findErr: errors.New("disk plan API failure")}
	commandLine := newCLI()
	setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "disk-plan", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "disk plan API failure") {
		t.Fatalf("find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

type diskPlanAPITestDouble struct {
	findRequest *diskplan.FindRequest
	readRequest *diskplan.ReadRequest
	findErr     error
}

func (api *diskPlanAPITestDouble) FindWithContext(_ context.Context, request *diskplan.FindRequest) ([]*iaas.DiskPlan, error) {
	api.findRequest = request
	if api.findErr != nil {
		return nil, api.findErr
	}
	return []*iaas.DiskPlan{{
		ID: 4, Name: "ssd", StorageClass: "ssd", Availability: types.EAvailability("available"),
		Size: []*iaas.DiskPlanSizeInfo{{
			Availability: types.EAvailability("available"), DisplaySize: 20, DisplaySuffix: "GB", SizeMB: 20480,
		}},
	}}, nil
}

func (api *diskPlanAPITestDouble) ReadWithContext(_ context.Context, request *diskplan.ReadRequest) (*iaas.DiskPlan, error) {
	api.readRequest = request
	return &iaas.DiskPlan{ID: request.ID, Name: "ssd", StorageClass: "ssd"}, nil
}
