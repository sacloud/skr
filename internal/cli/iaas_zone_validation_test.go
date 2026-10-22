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
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/sacloud/skr/internal/iaas/archiveapi"
	"github.com/sacloud/skr/internal/iaas/diskapi"
	"github.com/sacloud/skr/internal/iaas/diskplanapi"
	"github.com/sacloud/skr/internal/iaas/packetfilterapi"
	"github.com/sacloud/skr/internal/iaas/serverapi"
	"github.com/sacloud/skr/internal/iaas/switchapi"
)

func TestIaaSCommonZoneValidation(t *testing.T) {
	for _, resource := range []string{"archive", "disk", "diskplan", "packetfilter", "server", "switch"} {
		data, err := os.ReadFile(repositoryPath("api/commands/iaas-" + resource + ".json"))
		if err != nil {
			t.Fatal(err)
		}
		config, err := apigen.DecodeConfig(data)
		if err != nil {
			t.Fatal(err)
		}
		commandName := resource
		if resource == "diskplan" {
			commandName = "product disk"
		}
		for _, operation := range config.Operations {
			if operation.Handwritten {
				continue
			}
			if !operation.ValidateZone {
				t.Fatalf("%s %s does not enable common zone validation", resource, operation.Name)
			}
			for _, zone := range []string{"", "all"} {
				for _, input := range []string{"flags", "JSON", "missing Zone JSON"} {
					if input == "missing Zone JSON" && zone != "" {
						continue
					}
					t.Run(resource+"/"+operation.Name+"/"+input+"/"+zone, func(t *testing.T) {
						commandLine := newCLI()
						factoryCalled := false
						factoryError := func() error {
							factoryCalled = true
							return errors.New("unexpected API setup")
						}
						commandLine.IaaSAPI.Archive = archiveapi.NewCommands(archiveapi.Runtime{DecodeRequest: decodeRequest}, func() (archiveapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Disk = diskapi.NewCommands(diskapi.Runtime{DecodeRequest: decodeRequest}, func() (diskapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Product.Disk = diskplanapi.NewCommands(diskplanapi.Runtime{DecodeRequest: decodeRequest}, func() (diskplanapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.PacketFilter = packetfilterapi.NewCommands(packetfilterapi.Runtime{
							DecodeRequest: decodeRequest, TransformCreateRequest: transformPacketFilterCreateRequest, TransformUpdateRequest: transformPacketFilterUpdateRequest,
						}, func() (packetfilterapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Server = serverapi.NewCommands(serverapi.Runtime{
							DecodeRequest: decodeRequest, TransformCreateRequest: serverapi.TransformCreateRequest,
						}, func() (serverapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Switch = switchapi.NewCommands(switchapi.Runtime{DecodeRequest: decodeRequest}, func() (switchapi.API, error) { return nil, factoryError() })
						args := append([]string{"iaas-api"}, strings.Fields(commandName)...)
						args = append(args, strings.ToLower(operation.Name))
						switch input {
						case "missing Zone JSON":
							args = append(args, "--request", `{}`)
						case "JSON":
							args = append(args, "--request", `{"Zone":"`+zone+`"}`)
						default:
							args = append(args, "--zone", zone)
							for _, flag := range operation.Flags {
								if !flag.Required || flag.Name == "zone" {
									continue
								}
								value := "1"
								if flag.Type == "string" {
									value = "example"
								}
								args = append(args, "--"+flag.Name, value)
							}
						}
						var stdout, stderr bytes.Buffer
						code := runCLI(args, &stdout, &stderr, commandLine)
						want := "Zone が必要です (--request JSON も指定できます)\n"
						if zone == "all" {
							want = "Zone に \"all\" は指定できません。検索対象のゾーン名を指定してください\n"
						}
						if code != 1 || stdout.Len() != 0 || stderr.String() != want || factoryCalled {
							t.Fatalf("code=%d stdout=%q stderr=%q factoryCalled=%t, want common zone error %q before API setup", code, stdout.String(), stderr.String(), factoryCalled, want)
						}
					})
				}
			}
		}
	}
}

func TestServerBootCommonZoneValidation(t *testing.T) {
	for _, zone := range []string{"", "all"} {
		t.Run(zone, func(t *testing.T) {
			commandLine := newCLI()
			factoryCalled := false
			commandLine.IaaSAPI.Server = serverapi.NewCommands(serverapi.Runtime{}, func() (serverapi.API, error) {
				factoryCalled = true
				return nil, errors.New("unexpected API setup")
			})
			var stdout, stderr bytes.Buffer
			code := runCLI([]string{"iaas-api", "server", "boot", "--zone", zone, "--id", "123"}, &stdout, &stderr, commandLine)
			want := "Zone が必要です"
			if zone == "all" {
				want = `Zone に "all"`
			}
			if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), want) || factoryCalled {
				t.Fatalf("code=%d stdout=%q stderr=%q factoryCalled=%t, want zone error", code, stdout.String(), stderr.String(), factoryCalled)
			}
		})
	}
}
