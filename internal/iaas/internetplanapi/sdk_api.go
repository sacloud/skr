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

package internetplanapi

import (
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

type sdkAPI struct {
	op iaas.InternetPlanAPI
}

func NewSDKAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewInternetPlanOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) (*FindResult, error) {
	result, err := api.op.Find(ctx, request.Zone, request.findCondition())
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("InternetPlan Find returned no result")
	}
	plans := result.InternetPlans
	if plans == nil {
		plans = []*iaas.InternetPlan{}
	}
	return &FindResult{
		Total: result.Total, From: result.From, Count: result.Count, InternetPlans: plans,
	}, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.InternetPlan, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}
