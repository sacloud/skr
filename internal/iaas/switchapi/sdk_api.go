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

package switchapi

import (
	"context"
	"fmt"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/cleanup"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/query"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	caller iaas.APICaller
	op     iaas.SwitchAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{caller: caller, op: iaas.NewSwitchOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Switch, error) {
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
	return result.Switches, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Switch, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.Switch, error) {
	return api.op.Create(ctx, request.Zone, &request.SwitchCreateRequest)
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.Switch, error) {
	current, err := api.op.Read(ctx, request.Zone, request.ID)
	if err != nil {
		return nil, err
	}
	params := &iaas.SwitchUpdateRequest{
		Name:           current.Name,
		Description:    current.Description,
		Tags:           current.Tags,
		IconID:         current.IconID,
		NetworkMaskLen: current.NetworkMaskLen,
		DefaultRoute:   current.DefaultRoute,
	}
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
	if request.NetworkMaskLen != nil {
		params.NetworkMaskLen = *request.NetworkMaskLen
	}
	if request.DefaultRoute != nil {
		params.DefaultRoute = *request.DefaultRoute
	}
	return api.op.Update(ctx, request.Zone, request.ID, params)
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	if request.WaitForRelease {
		timeout := request.WaitForReleaseTimeout
		if timeout == 0 {
			timeout = int((time.Hour).Seconds())
		}
		tick := request.WaitForReleaseTick
		if tick == 0 {
			tick = 5
		}
		err := cleanup.DeleteSwitch(ctx, api.caller, request.Zone, request.ID, query.CheckReferencedOption{
			Timeout: time.Duration(timeout) * time.Second,
			Tick:    time.Duration(tick) * time.Second,
		})
		if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	err := api.op.Delete(ctx, request.Zone, request.ID)
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}
