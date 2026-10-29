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

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/sacloud/skr/internal/iaas/archiveapi"
	"github.com/sacloud/skr/internal/iaas/diskapi"
	"github.com/sacloud/skr/internal/iaas/diskplanapi"
	"github.com/sacloud/skr/internal/iaas/internetplanapi"
	"github.com/sacloud/skr/internal/iaas/packetfilterapi"
	"github.com/sacloud/skr/internal/iaas/privatehostplanapi"
	"github.com/sacloud/skr/internal/iaas/serverapi"
	"github.com/sacloud/skr/internal/iaas/switchapi"
)

func TestIaaSCommonZoneValidation(t *testing.T) {
	resources := []struct {
		name    string
		config  string
		command string
	}{
		{name: "archive", config: "iaas-archive", command: "archive"},
		{name: "disk", config: "iaas-disk", command: "disk"},
		{name: "diskplan", config: "iaas-product-disk", command: "product disk"},
		{name: "internetplan", config: "iaas-product-internet", command: "product internet"},
		{name: "packetfilter", config: "iaas-packetfilter", command: "packetfilter"},
		{name: "privatehostplan", config: "iaas-product-privatehost", command: "product privatehost"},
		{name: "server", config: "iaas-server", command: "server"},
		{name: "serverplan", config: "iaas-product-server", command: "product server"},
		{name: "switch", config: "iaas-switch", command: "switch"},
	}
	for _, resource := range resources {
		data, err := os.ReadFile(repositoryPath("api/commands/" + resource.config + ".json"))
		require.NoError(t, err, "read %s config", resource.name)
		config, err := apigen.DecodeConfig(data, apigen.ModeIaaS)
		require.NoError(t, err, "decode %s config", resource.name)
		for _, operation := range config.Operations {
			if operation.Handwritten {
				continue
			}
			if !operation.ValidateZone {
				require.Fail(t, "common zone validation is not enabled", "%s %s", resource.name, operation.Name)
			}
			for _, zone := range []string{"", "all"} {
				for _, input := range []string{"flags", "JSON", "missing Zone JSON"} {
					if input == "missing Zone JSON" && zone != "" {
						continue
					}
					t.Run(resource.name+"/"+operation.Name+"/"+input+"/"+zone, func(t *testing.T) {
						commandLine := newCLI()
						factoryCalled := false
						factoryError := func() error {
							factoryCalled = true
							return errors.New("unexpected API setup")
						}
						commandLine.IaaSAPI.Archive = archiveapi.NewCommands(archiveapi.Runtime{DecodeRequest: decodeRequest}, func() (archiveapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Disk = diskapi.NewCommands(diskapi.Runtime{DecodeRequest: decodeRequest}, func() (diskapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Product.Disk = diskplanapi.NewCommands(diskplanapi.Runtime{DecodeRequest: decodeRequest}, func() (diskplanapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Product.Internet = internetplanapi.NewCommands(internetplanapi.Runtime{DecodeRequest: decodeRequest}, func() (internetplanapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.PacketFilter = packetfilterapi.NewCommands(packetfilterapi.Runtime{
							DecodeRequest: decodeRequest, TransformCreateRequest: transformPacketFilterCreateRequest, TransformUpdateRequest: transformPacketFilterUpdateRequest,
						}, func() (packetfilterapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Product.PrivateHost = privatehostplanapi.NewCommands(privatehostplanapi.Runtime{DecodeRequest: decodeRequest}, func() (privatehostplanapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Server = serverapi.NewCommands(serverapi.Runtime{
							DecodeRequest: decodeRequest, TransformCreateRequest: serverapi.TransformCreateRequest,
						}, func() (serverapi.API, error) { return nil, factoryError() })
						commandLine.IaaSAPI.Switch = switchapi.NewCommands(switchapi.Runtime{DecodeRequest: decodeRequest}, func() (switchapi.API, error) { return nil, factoryError() })
						args := append([]string{"iaas-api"}, strings.Fields(resource.command)...)
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
						require.Equal(t, 1, code, "common zone error %q before API setup", want)
						require.Empty(t, stdout, "common zone error should not write output")
						require.Equal(t, want, stderr.String(), "common zone error for %v", args)
						require.False(t, factoryCalled, "the API factory should not be called before zone validation")
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
			require.Equal(t, 1, code, "boot should fail with a zone error")
			require.Empty(t, stdout, "boot zone error should not write output")
			require.Contains(t, stderr.String(), want, "boot zone error %q", stderr.String())
			require.False(t, factoryCalled, "the API factory should not be called before zone validation")
		})
	}
}
