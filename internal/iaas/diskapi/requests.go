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

package diskapi

import (
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type FindRequest struct {
	Zone  string
	Names []string
	Tags  []string
	Sort  search.SortKeys
	Count int
	From  int
}

type ReadRequest struct {
	Zone string
	ID   types.ID
}

type CreateRequest struct {
	Zone                       string
	Name                       string
	Description                string
	Tags                       types.Tags
	IconID                     types.ID
	DiskPlanID                 types.ID
	Connection                 types.EDiskConnection
	EncryptionAlgorithm        types.EDiskEncryptionAlgorithm
	KMSKeyID                   types.ID
	DedicatedStorageContractID types.ID
	SourceDiskID               types.ID
	SourceArchiveID            types.ID
	ServerID                   types.ID
	SizeGB                     int
	DistantFrom                []types.ID
}

type UpdateRequest struct {
	Zone        string
	ID          types.ID
	Name        *string
	Description *string
	Tags        *types.Tags
	IconID      *types.ID
	Connection  *types.EDiskConnection
}

type DeleteRequest struct {
	Zone           string
	ID             types.ID
	FailIfNotFound bool
}
