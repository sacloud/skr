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

package zones

import (
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
)

const All = "all"

type API interface {
	FindWithContext(context.Context, *zone.FindRequest) ([]*iaas.Zone, error)
}

type Factory func() (API, error)

func New(trace bool) (API, error) {
	client, err := iaasclient.New(trace)
	if err != nil {
		return nil, err
	}
	return zone.New(client), nil
}

func FindInAll[T any](
	ctx context.Context,
	newZoneAPI Factory,
	find func(context.Context, string) ([]T, error),
) ([]T, error) {
	if newZoneAPI == nil {
		return nil, fmt.Errorf("IaaS zone API factory is not configured")
	}
	zoneAPI, err := newZoneAPI()
	if err != nil {
		return nil, err
	}
	zones, err := zoneAPI.FindWithContext(ctx, &zone.FindRequest{})
	if err != nil {
		return nil, fmt.Errorf("list IaaS zones: %w", err)
	}

	result := make([]T, 0)
	for _, item := range zones {
		if item == nil || item.Name == "" {
			return nil, fmt.Errorf("list IaaS zones: SDK returned a zone without a name")
		}
		items, err := find(ctx, item.Name)
		if err != nil {
			return nil, fmt.Errorf("find in IaaS zone %q: %w", item.Name, err)
		}
		result = append(result, items...)
	}
	return result, nil
}

func ValidateSingleZone(zone string) error {
	if zone == All {
		return fmt.Errorf("Zone が %q の場合は find のみ対応しています", All)
	}
	return nil
}
