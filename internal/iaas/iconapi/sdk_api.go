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

package iconapi

import (
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.IconAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewIconOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Icon, error) {
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
	if request.Scope != "" {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("Scope")] = search.OrEqual(request.Scope)
	}
	result, err := api.op.Find(ctx, condition)
	if err != nil {
		return nil, err
	}
	return result.Icons, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Icon, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	return api.op.Read(ctx, request.ID)
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.Icon, error) {
	return api.op.Create(ctx, &iaas.IconCreateRequest{
		Name:  request.Name,
		Tags:  request.Tags,
		Image: request.Image,
	})
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.Icon, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	params := &iaas.IconUpdateRequest{}
	if request.Name != nil {
		params.Name = *request.Name
	}
	if request.Tags != nil {
		params.Tags = *request.Tags
	}
	return api.op.Update(ctx, request.ID, params)
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	if request.ID == 0 {
		return fmt.Errorf("ID is required")
	}
	err := api.op.Delete(ctx, request.ID)
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}
