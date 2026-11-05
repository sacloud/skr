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

package ipv6addrapi

import (
	"context"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.IPv6AddrAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewIPv6AddrOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.IPv6Addr, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	if len(request.SwitchIDs) > 0 {
		condition.Filter = search.Filter{search.Key("IPv6Net.Switch.ID"): search.AndEqual(request.SwitchIDs...)}
	}
	if len(request.HostNames) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("HostName")] = search.AndEqual(request.HostNames...)
	}
	if len(request.IPv6Addresses) > 0 {
		if condition.Filter == nil {
			condition.Filter = search.Filter{}
		}
		condition.Filter[search.Key("IPv6Addr")] = search.AndEqual(request.IPv6Addresses...)
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.IPv6Addrs, nil
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.IPv6Addr, error) {
	return api.op.Create(ctx, request.Zone, &iaas.IPv6AddrCreateRequest{
		IPv6Addr: request.IPv6Addr,
		HostName: request.HostName,
	})
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.IPv6Addr, error) {
	return api.op.Read(ctx, request.Zone, request.IPv6Addr)
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.IPv6Addr, error) {
	return api.op.Update(ctx, request.Zone, request.IPv6Addr, &iaas.IPv6AddrUpdateRequest{
		HostName: request.HostName,
	})
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	err := api.op.Delete(ctx, request.Zone, request.IPv6Addr)
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}
