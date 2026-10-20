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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/serviceclass"
	"github.com/sacloud/skr/internal/apigen"
	serviceclassapi "github.com/sacloud/skr/internal/iaas/serviceclassapi"
)

func TestServiceClassGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-serviceclass.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/serviceclassapi/serviceclass_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated ServiceClass commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-serviceclass.json API_OUTPUT=internal/iaas/serviceclassapi/serviceclass_api_generated.go")
	}
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
		if code := run(test.args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", test.args, code, stderr.String())
		}
		help := strings.Join(strings.Fields(stdout.String()), " ")
		for _, text := range test.want {
			if !strings.Contains(help, text) {
				t.Errorf("help for %v does not contain %q: %s", test.args, text, help)
			}
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
	if code != 0 || stderr != "" {
		t.Fatalf("find flags: code %d, stderr %q", code, stderr)
	}
	if request := api.findRequest; request == nil || request.Zone != "test-zone" || request.Count != 3 || request.From != 1 {
		t.Fatalf("Find request = %+v, want zone, count, and offset", request)
	}
	var result []struct {
		ID               int64
		ServiceClassName string
		ServiceClassPath string
		DisplayName      string
		IsPublic         bool
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
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
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
	}
	if api.findCalls != 0 {
		t.Fatalf("invalid requests reached API: find calls %d", api.findCalls)
	}
}

func TestIaaSServiceClassPropagatesAPIErrors(t *testing.T) {
	api := &serviceClassAPITestDouble{findErr: errors.New("serviceclass API failure")}
	commandLine := newCLI()
	setServiceClassAPIFactory(commandLine, func() (serviceclassapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "service-class", "find", "--zone", "test-zone"}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "serviceclass API failure") {
		t.Fatalf("find error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
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
