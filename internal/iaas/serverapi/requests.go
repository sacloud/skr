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

package serverapi

import (
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type FindRequest struct {
	Zone  string
	Names []string
	Tags  types.Tags
	Sort  search.SortKeys
	Count int
	From  int
}

type ReadRequest struct {
	Zone string
	ID   types.ID
}

type CreateRequest struct {
	Zone              string
	Name              string
	CPU               int
	MemoryGB          int
	Tags              types.Tags
	Description       string
	IconID            types.ID
	GPU               int
	GPUModel          string
	CPUModel          string
	Commitment        types.ECommitment
	Generation        types.EPlanGeneration
	ConfidentialVM    bool
	InterfaceDriver   types.EInterfaceDriver
	BootAfterCreate   bool
	NoWait            bool
	CDROMID           *types.ID
	PrivateHostID     types.ID
	Disks             []DiskRequest
	NetworkInterfaces []NetworkInterface
}

type DiskRequest struct {
	Name                string
	DiskPlanID          types.ID
	SizeGB              int
	SourceDiskID        types.ID
	SourceArchiveID     types.ID
	Connection          types.EDiskConnection
	EncryptionAlgorithm types.EDiskEncryptionAlgorithm
	KMSKeyID            types.ID
}

type NetworkInterface struct {
	Upstream       string
	PacketFilterID types.ID
}

type BootRequest struct {
	Zone     string
	ID       types.ID
	UserData string
}

type UpdateRequest struct {
	Zone              string
	ID                types.ID
	Tags              *types.Tags
	Name              *string
	Description       *string
	CPU               *int
	MemoryGB          *int
	GPU               *int
	GPUModel          *string
	CPUModel          *string
	IconID            *types.ID
	PrivateHostID     *types.ID
	InterfaceDriver   *types.EInterfaceDriver
	Commitment        *types.ECommitment
	Generation        *types.EPlanGeneration
	CDROMID           *types.ID
	Disks             []DiskRequest
	NetworkInterfaces []NetworkInterface
}

type DeleteRequest struct {
	Zone           string
	ID             types.ID
	WithDisks      bool
	FailIfNotFound bool
	Force          bool
}
