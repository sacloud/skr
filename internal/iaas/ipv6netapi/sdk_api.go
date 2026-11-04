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

package ipv6netapi

import (
	"context"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.IPv6NetAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewIPv6NetOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.IPv6Net, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	if len(request.SwitchIDs) > 0 {
		condition.Filter = search.Filter{search.Key("Switch.ID"): search.AndEqual(request.SwitchIDs...)}
	}
	if len(request.SwitchNames) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("Switch.Name")] = search.AndEqual(request.SwitchNames...)
	}
	if len(request.InternetIDs) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("Switch.Internet.ID")] = search.AndEqual(request.InternetIDs...)
	}
	if len(request.InternetNames) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("Switch.Internet.Name")] = search.AndEqual(request.InternetNames...)
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.IPv6Nets, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.IPv6Net, error) {
	return api.op.Read(ctx, request.Zone, request.ID)
}
