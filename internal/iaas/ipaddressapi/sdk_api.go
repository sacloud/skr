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

package ipaddressapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

const (
	defaultRetryMax      = 30
	defaultRetryInterval = 10
)

type sdkAPI struct {
	op iaas.IPAddressAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewIPAddressOp(caller)}
}

func (api *sdkAPI) ListWithContext(ctx context.Context, request *ListRequest) ([]*iaas.IPAddress, error) {
	result, err := api.op.List(ctx, request.Zone)
	if err != nil {
		return nil, err
	}
	return result.IPAddress, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.IPAddress, error) {
	return api.op.Read(ctx, request.Zone, request.IPAddress)
}

func (api *sdkAPI) UpdateHostNameWithContext(ctx context.Context, request *UpdateHostNameRequest) error {
	if _, err := api.op.Read(ctx, request.Zone, request.IPAddress); err != nil {
		return err
	}
	retryMax := request.RetryMax
	if retryMax == 0 {
		retryMax = defaultRetryMax
	}
	retryInterval := request.RetryInterval
	if retryInterval == 0 {
		retryInterval = defaultRetryInterval
	}
	for attempt := 0; attempt < retryMax; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("updating the HostName for %s failed: %s", request.IPAddress, err)
		}
		if _, err := api.op.UpdateHostName(ctx, request.Zone, request.IPAddress, request.HostName); err == nil {
			return nil
		} else if apiErr, ok := errors.AsType[iaas.APIError](err); ok && apiErr.ResponseCode() != 429 && apiErr.ResponseCode() < 500 {
			return fmt.Errorf("updating the HostName for %s failed: %s", request.IPAddress, apiErr)
		}
		time.Sleep(time.Duration(retryInterval) * time.Second)
	}
	return fmt.Errorf("updating the HostName for %s failed: giving up on setting the HostName: max attempts exceeded", request.IPAddress)
}
