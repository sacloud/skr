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

package regionapi

import (
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.RegionAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewRegionOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Region, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	if len(request.Names) != 0 {
		condition.Filter = search.Filter{
			search.Key("Name"): search.AndEqual(request.Names...),
		}
	}
	result, err := api.op.Find(ctx, condition)
	if err != nil {
		return nil, err
	}
	return result.Regions, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Region, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	return api.op.Read(ctx, request.ID)
}
