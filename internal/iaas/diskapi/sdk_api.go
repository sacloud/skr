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
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.DiskAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewDiskOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Disk, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	if len(request.Names) > 0 {
		condition.Filter = search.Filter{search.Key("Name"): search.AndEqual(request.Names...)}
	}
	if len(request.Tags) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("Tags.Name")] = search.TagsAndEqual(request.Tags...)
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.Disks, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Disk, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.Disk, error) {
	params := &iaas.DiskCreateRequest{
		DiskPlanID:          request.DiskPlanID,
		Connection:          request.Connection,
		EncryptionAlgorithm: request.EncryptionAlgorithm,
		SourceDiskID:        request.SourceDiskID,
		SourceArchiveID:     request.SourceArchiveID,
		ServerID:            request.ServerID,
		Name:                request.Name,
		Description:         request.Description,
		Tags:                request.Tags,
		IconID:              request.IconID,
	}
	params.SetSizeGB(request.SizeGB)
	if request.DedicatedStorageContractID != 0 {
		return api.op.CreateOnDedicatedStorage(ctx, request.Zone, params, request.DistantFrom, request.KMSKeyID, request.DedicatedStorageContractID)
	}
	return api.op.Create(ctx, request.Zone, params, request.DistantFrom, request.KMSKeyID)
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.Disk, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	params := &iaas.DiskUpdateRequest{}
	if request.Name != nil {
		params.Name = *request.Name
	}
	if request.Description != nil {
		params.Description = *request.Description
	}
	if request.Tags != nil {
		params.Tags = *request.Tags
	}
	if request.IconID != nil {
		params.IconID = *request.IconID
	}
	if request.Connection != nil {
		params.Connection = *request.Connection
	}
	return api.op.Update(ctx, request.Zone, request.ID, params)
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	if request.ID == 0 {
		return fmt.Errorf("ID is required")
	}
	err := api.op.Delete(ctx, request.Zone, request.ID)
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}
