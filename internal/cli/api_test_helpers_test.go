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
	"github.com/sacloud/skr/internal/containerregistryapi"
	"github.com/sacloud/skr/internal/iaas/archiveapi"
	"github.com/sacloud/skr/internal/iaas/diskapi"
	"github.com/sacloud/skr/internal/iaas/diskplanapi"
	"github.com/sacloud/skr/internal/iaas/internetapi"
	"github.com/sacloud/skr/internal/iaas/internetplanapi"
	"github.com/sacloud/skr/internal/iaas/licenseinfoapi"
	"github.com/sacloud/skr/internal/iaas/packetfilterapi"
	"github.com/sacloud/skr/internal/iaas/privatehostplanapi"
	"github.com/sacloud/skr/internal/iaas/regionapi"
	"github.com/sacloud/skr/internal/iaas/serverapi"
	"github.com/sacloud/skr/internal/iaas/serverplanapi"
	"github.com/sacloud/skr/internal/iaas/serviceclassapi"
	"github.com/sacloud/skr/internal/iaas/switchapi"
	"github.com/sacloud/skr/internal/iaas/zoneapi"
)

func setArchiveAPIFactory(commandLine *cli, factory archiveapi.APIFactory) {
	commandLine.IaaSAPI.Archive = archiveapi.NewCommands(archiveapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setDiskAPIFactory(commandLine *cli, factory diskapi.APIFactory) {
	commandLine.IaaSAPI.Disk = diskapi.NewCommands(diskapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setDiskPlanAPIFactory(commandLine *cli, factory diskplanapi.APIFactory) {
	commandLine.IaaSAPI.Product.Disk = diskplanapi.NewCommands(diskplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setInternetAPIFactory(commandLine *cli, factory internetapi.APIFactory) {
	commandLine.IaaSAPI.Internet = internetapi.NewCommands(internetapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setInternetPlanAPIFactory(commandLine *cli, factory internetplanapi.APIFactory) {
	commandLine.IaaSAPI.Product.Internet = internetplanapi.NewCommands(internetplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setLicenseInfoAPIFactory(commandLine *cli, factory licenseinfoapi.APIFactory) {
	commandLine.IaaSAPI.Product.License = licenseinfoapi.NewCommands(licenseinfoapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setPacketFilterAPIFactory(commandLine *cli, factory packetfilterapi.APIFactory) {
	commandLine.IaaSAPI.PacketFilter = packetfilterapi.NewCommands(packetfilterapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
		TransformCreateRequest: transformPacketFilterCreateRequest,
		TransformUpdateRequest: transformPacketFilterUpdateRequest,
	}, factory)
}

func setPrivateHostPlanAPIFactory(commandLine *cli, factory privatehostplanapi.APIFactory) {
	commandLine.IaaSAPI.Product.PrivateHost = privatehostplanapi.NewCommands(privatehostplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setServerAPIFactory(commandLine *cli, factory serverapi.APIFactory) {
	commandLine.IaaSAPI.Server = serverapi.NewCommands(serverapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
		TransformCreateRequest: serverapi.TransformCreateRequest,
	}, factory)
}

func setServerPlanAPIFactory(commandLine *cli, factory serverplanapi.APIFactory) {
	commandLine.IaaSAPI.Product.Server = serverplanapi.NewCommands(serverplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setSwitchAPIFactory(commandLine *cli, factory switchapi.APIFactory) {
	commandLine.IaaSAPI.Switch = switchapi.NewCommands(switchapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setZoneAPIFactory(commandLine *cli, factory zoneapi.APIFactory) {
	commandLine.IaaSAPI.Zone = zoneapi.NewCommands(zoneapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setRegionAPIFactory(commandLine *cli, factory regionapi.APIFactory) {
	commandLine.IaaSAPI.Region = regionapi.NewCommands(regionapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setServiceClassAPIFactory(commandLine *cli, factory serviceclassapi.APIFactory) {
	commandLine.IaaSAPI.ServiceClass = serviceclassapi.NewCommands(serviceclassapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, factory)
}

func setContainerRegistryAPIFactory(commandLine *cli, runtime containerregistryapi.Runtime, factory containerregistryapi.APIFactory) {
	commandLine.ContainerRegistryAPI = containerregistryapi.NewCommands(runtime, factory)
}
