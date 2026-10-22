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

package serverplanapi

import (
	"context"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

type sdkAPI struct {
	op iaas.ServerPlanAPI
}

func NewSDKAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewServerPlanOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) (*iaas.ServerPlanFindResult, error) {
	return api.op.Find(ctx, request.Zone, request.findCondition())
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.ServerPlan, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}
