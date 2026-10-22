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
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	"github.com/sacloud/skr/internal/apigen"
	"github.com/sacloud/skr/internal/iaas/archiveapi"
	"github.com/sacloud/skr/internal/iaas/diskapi"
	"github.com/sacloud/skr/internal/iaas/diskplanapi"
	"github.com/sacloud/skr/internal/iaas/packetfilterapi"
	"github.com/sacloud/skr/internal/iaas/serverapi"
	"github.com/sacloud/skr/internal/iaas/serverplanapi"
	"github.com/sacloud/skr/internal/iaas/switchapi"
)

func TestIaaSRequestBodyValidationIsDelegated(t *testing.T) {
	resources := []struct {
		name    string
		config  string
		command string
	}{
		{name: "archive", config: "iaas-archive", command: "archive"},
		{name: "disk", config: "iaas-disk", command: "disk"},
		{name: "diskplan", config: "iaas-product-disk", command: "product disk"},
		{name: "packetfilter", config: "iaas-packetfilter", command: "packetfilter"},
		{name: "server", config: "iaas-server", command: "server"},
		{name: "serverplan", config: "iaas-product-server", command: "product server"},
		{name: "switch", config: "iaas-switch", command: "switch"},
	}
	for _, resource := range resources {
		data, err := os.ReadFile(repositoryPath("api/commands/" + resource.config + ".json"))
		if err != nil {
			t.Fatal(err)
		}
		config, err := apigen.DecodeConfig(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range config.Operations {
			if resource.name == "archive" && operation.Name == "Find" {
				if operation.RequestValidator != "ValidateFindRequest" {
					t.Fatal("Archive Find must retain unknown OSType validation")
				}
			} else if operation.RequestValidator != "" {
				t.Fatalf("%s %s still configures a body validator", resource.name, operation.Name)
			}
			if operation.Handwritten {
				continue
			}
			t.Run(resource.name+"/"+operation.Name, func(t *testing.T) {
				commandLine := newCLI()
				failure := errors.New("API factory reached")
				setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return nil, failure })
				setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return nil, failure })
				setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return nil, failure })
				setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) { return nil, failure })
				setServerAPIFactory(commandLine, func() (serverapi.API, error) { return nil, failure })
				setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) { return nil, failure })
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return nil, failure })
				var stdout, stderr bytes.Buffer
				args := append([]string{"iaas-api"}, strings.Fields(resource.command)...)
				args = append(args, strings.ToLower(operation.Name), "--request", `{"Zone":"test-zone"}`)
				code := runCLI(args, &stdout, &stderr, commandLine)
				if code != 1 || stdout.Len() != 0 || stderr.String() != failure.Error()+"\n" {
					t.Fatalf("request body was rejected before API setup: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestIaaSSDKValidationErrorsAreReturned(t *testing.T) {
	for _, resource := range []string{"archive", "disk", "product disk", "packetfilter", "server", "product server", "switch"} {
		for _, input := range []string{"JSON", "flags"} {
			t.Run(resource+"/"+input, func(t *testing.T) {
				commandLine := newCLI()
				caller := &serverAPICaller{}
				setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return archive.New(caller), nil })
				setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return disk.New(caller), nil })
				setDiskPlanAPIFactory(commandLine, func() (diskplanapi.API, error) { return diskplanapi.NewSDKAPI(caller), nil })
				setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) { return packetfilter.New(caller), nil })
				setServerAPIFactory(commandLine, func() (serverapi.API, error) { return server.New(caller), nil })
				setServerPlanAPIFactory(commandLine, func() (serverplanapi.API, error) { return serverplanapi.NewSDKAPI(caller), nil })
				setSwitchAPIFactory(commandLine, func() (switchapi.API, error) { return swytch.New(caller), nil })
				args := append([]string{"iaas-api"}, strings.Fields(resource)...)
				args = append(args, "read")
				if input == "JSON" {
					args = append(args, "--request", `{"Zone":"test-zone"}`)
				} else {
					args = append(args, "--zone", "test-zone", "--id", "0")
				}
				var stdout, stderr bytes.Buffer
				code := runCLI(args, &stdout, &stderr, commandLine)
				if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "ID") ||
					!strings.Contains(stderr.String(), "required") || len(caller.calls) != 0 {
					t.Fatalf("code=%d stdout=%q stderr=%q HTTP calls=%+v; want SDK validation error", code, stdout.String(), stderr.String(), caller.calls)
				}
			})
		}
	}
}

func TestIaaSEmptyUpdatesReachSDK(t *testing.T) {
	for _, resource := range []string{"server", "disk", "packetfilter"} {
		for _, input := range []string{"JSON", "flags"} {
			t.Run(resource+"/"+input, func(t *testing.T) {
				commandLine := newCLI()
				serverAPI := &serverAPITestDouble{}
				diskAPI := &diskAPITestDouble{}
				filterAPI := &packetFilterUpdateRecorder{}
				setServerAPIFactory(commandLine, func() (serverapi.API, error) { return serverAPI, nil })
				setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return diskAPI, nil })
				setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) { return filterAPI, nil })
				args := []string{"iaas-api", resource, "update"}
				if input == "JSON" {
					args = append(args, "--request", `{"Zone":"test-zone","ID":123}`)
				} else {
					args = append(args, "--zone", "test-zone", "--id", "123")
				}
				var stdout, stderr bytes.Buffer
				if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 || stderr.Len() != 0 {
					t.Fatalf("code=%d stderr=%q; want request forwarded", code, stderr.String())
				}
				switch resource {
				case "server":
					if req := serverAPI.updateRequest; req == nil || req.ID != 123 || req.Zone != "test-zone" || req.Name != nil || req.Tags != nil || req.CPU != nil {
						t.Fatalf("unexpected request: %+v", req)
					}
				case "disk":
					if req := diskAPI.updateRequest; req == nil || req.ID != 123 || req.Zone != "test-zone" || req.Name != nil || req.Tags != nil || req.Connection != nil {
						t.Fatalf("unexpected request: %+v", req)
					}
				case "packetfilter":
					if req := filterAPI.request; req == nil || req.ID != 123 || req.Zone != "test-zone" || req.Name != nil || req.Expression != nil {
						t.Fatalf("unexpected request: %+v", req)
					}
				}
			})
		}
	}
}

func TestArchiveOSTypeCanBeCombinedWithNamesAndTags(t *testing.T) {
	for _, input := range []string{"JSON", "flags"} {
		t.Run(input, func(t *testing.T) {
			api := &archiveAPITestDouble{}
			commandLine := newCLI()
			setArchiveAPIFactory(commandLine, func() (archiveapi.API, error) { return api, nil })
			args := []string{"iaas-api", "archive", "find"}
			if input == "JSON" {
				args = append(args, "--request", fmt.Sprintf(`{"Zone":"test-zone","OSType":%d,"Names":["image"],"Tags":["custom"]}`, ostype.Ubuntu2404))
			} else {
				args = append(args, "--zone", "test-zone", "--os-type", "ubuntu2404", "--tags", "custom")
			}
			var stdout, stderr bytes.Buffer
			if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 || stderr.Len() != 0 {
				t.Fatalf("code=%d stderr=%q; want combined search forwarded", code, stderr.String())
			}
			if len(api.findRequests) != 1 {
				t.Fatalf("requests: %+v", api.findRequests)
			}
			req := api.findRequests[0]
			if req.OSType != ostype.Ubuntu2404 || len(req.Tags) != 1 || req.Tags[0] != "custom" {
				t.Fatalf("unexpected request: %+v", req)
			}
			if input == "JSON" && (len(req.Names) != 1 || req.Names[0] != "image") {
				t.Fatalf("names were not preserved: %+v", req)
			}
		})
	}
}

func TestServerBootSDKValidationErrorIsReturned(t *testing.T) {
	commandLine := newCLI()
	caller := &serverAPICaller{}
	setServerAPIFactory(commandLine, func() (serverapi.API, error) { return server.New(caller), nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "server", "boot", "--zone", "test-zone", "--id", "0"}, &stdout, &stderr, commandLine)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "required") || !strings.Contains(stderr.String(), "ID") || len(caller.calls) != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v; want SDK validation error", code, stdout.String(), stderr.String(), caller.calls)
	}
}

func TestPacketFilterMissingHashIsForwardedAndAPIErrorsAreReturned(t *testing.T) {
	for _, input := range []string{"JSON", "flags"} {
		t.Run(input, func(t *testing.T) {
			api := &packetFilterUpdateRecorder{err: errors.New("API rejected expression update")}
			commandLine := newCLI()
			setPacketFilterAPIFactory(commandLine, func() (packetfilterapi.API, error) { return api, nil })
			args := []string{"iaas-api", "packetfilter", "update"}
			if input == "JSON" {
				args = append(args, "--request", `{"Zone":"test-zone","ID":123,"Expression":[]}`)
			} else {
				args = append(args, "--zone", "test-zone", "--id", "123", "--expressions", `[]`)
			}
			var stdout, stderr bytes.Buffer
			code := runCLI(args, &stdout, &stderr, commandLine)
			if code != 1 || stdout.Len() != 0 || stderr.String() != api.err.Error()+"\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q; want API error", code, stdout.String(), stderr.String())
			}
			if req := api.request; req == nil || req.Expression == nil || len(*req.Expression) != 0 || req.OriginalExpressionHash != "" {
				t.Fatalf("unexpected request: %+v", req)
			}
		})
	}
}

type packetFilterUpdateRecorder struct {
	packetfilterapi.API
	request *packetfilter.UpdateRequest
	err     error
}

func (api *packetFilterUpdateRecorder) UpdateWithContext(_ context.Context, request *packetfilter.UpdateRequest) (*iaas.PacketFilter, error) {
	api.request = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.PacketFilter{ID: request.ID}, nil
}
