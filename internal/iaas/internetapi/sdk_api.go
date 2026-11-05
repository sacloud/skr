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

package internetapi

import (
	"context"
	"fmt"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

const (
	internetReadyTimeout  = 35 * time.Minute
	internetReadyInterval = 2 * time.Second
)

type sdkAPI struct {
	op       iaas.InternetAPI
	subnetOp iaas.SubnetAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{
		op:       iaas.NewInternetOp(caller),
		subnetOp: iaas.NewSubnetOp(caller),
	}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Internet, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	filter := search.Filter{}
	if len(request.Names) != 0 {
		filter[search.Key("Name")] = search.AndEqual(request.Names...)
	}
	if len(request.SwitchIDs) != 0 {
		filter[search.Key("Switch.ID")] = search.AndEqual(request.SwitchIDs...)
	}
	if len(request.SwitchNames) != 0 {
		filter[search.Key("Switch.Name")] = search.AndEqual(request.SwitchNames...)
	}
	if len(request.Tags) != 0 {
		filter[search.Key("Tags.Name")] = search.TagsAndEqual(request.Tags...)
	}
	if len(filter) != 0 {
		condition.Filter = filter
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.Internet, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Internet, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.Internet, error) {
	if request.NoWait || request.NotFoundRetry != 0 {
		return nil, fmt.Errorf("NoWait と NotFoundRetry はサポートしていません")
	}
	result, err := api.op.Create(ctx, request.Zone, &iaas.InternetCreateRequest{
		Name:           request.Name,
		Description:    request.Description,
		Tags:           request.Tags,
		IconID:         request.IconID,
		NetworkMaskLen: request.NetworkMaskLen,
		BandWidthMbps:  request.BandWidthMbps,
	})
	if err != nil {
		return nil, err
	}
	if request.EnableIPv6 {
		if result == nil || result.ID.IsEmpty() {
			return nil, fmt.Errorf("ルータ＋スイッチ作成 API の応答に ID がありません")
		}
		if err := api.waitForInternetReady(ctx, request.Zone, result.ID); err != nil {
			return nil, fmt.Errorf("ルータ＋スイッチ %d は作成されましたが、参照可能になるまで待機できませんでした: %w", result.ID, err)
		}
		if _, err := api.op.EnableIPv6(ctx, request.Zone, result.ID); err != nil {
			return nil, fmt.Errorf("ルータ＋スイッチ %d は作成されましたが、IPv6 の有効化に失敗しました: %w", result.ID, err)
		}
	}
	return result, nil
}

func (api *sdkAPI) waitForInternetReady(ctx context.Context, zone string, id types.ID) error {
	waitCtx, cancel := context.WithTimeout(ctx, internetReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(internetReadyInterval)
	defer ticker.Stop()

	for {
		internet, err := api.op.Read(waitCtx, zone, id)
		if err == nil {
			if internet == nil || internet.ID != id {
				return fmt.Errorf("ルータ＋スイッチ %d の読み込み応答が不正です", id)
			}
			return nil
		}
		if !iaas.IsNotFoundError(err) {
			return fmt.Errorf("ルータ＋スイッチ %d の状態を確認できません: %w", id, err)
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("ルータ＋スイッチ %d が %s 以内に参照可能になりませんでした: %w", id, internetReadyTimeout, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.Internet, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	hasUpdateFields := request.Name != nil || request.Description != nil || request.Tags != nil || request.IconID != nil
	if !hasUpdateFields && request.BandWidthMbps == nil && request.EnableIPv6 == nil {
		return api.readCurrent(ctx, request.Zone, request.ID)
	}
	var result *iaas.Internet
	var err error
	if hasUpdateFields {
		current, err := api.readCurrent(ctx, request.Zone, request.ID)
		if err != nil {
			return nil, err
		}
		update := &iaas.InternetUpdateRequest{
			Name:        current.Name,
			Description: current.Description,
			Tags:        current.Tags,
			IconID:      current.IconID,
		}
		if request.Name != nil {
			update.Name = *request.Name
		}
		if request.Description != nil {
			update.Description = *request.Description
		}
		if request.Tags != nil {
			update.Tags = *request.Tags
		}
		if request.IconID != nil {
			update.IconID = *request.IconID
		}
		result, err = api.op.Update(ctx, request.Zone, request.ID, update)
		if err != nil {
			return nil, err
		}
	}
	if request.BandWidthMbps != nil {
		result, err = api.op.UpdateBandWidth(ctx, request.Zone, request.ID, &iaas.InternetUpdateBandWidthRequest{
			BandWidthMbps: *request.BandWidthMbps,
		})
		if err != nil {
			return nil, err
		}
	}
	if request.EnableIPv6 != nil {
		if *request.EnableIPv6 {
			if _, err := api.op.EnableIPv6(ctx, request.Zone, request.ID); err != nil {
				return nil, err
			}
		} else if err := api.disableIPv6(ctx, request.Zone, request.ID); err != nil {
			return nil, err
		}
	}
	if result == nil {
		return api.readCurrent(ctx, request.Zone, request.ID)
	}
	return result, nil
}

func (api *sdkAPI) readCurrent(ctx context.Context, zone string, id types.ID) (*iaas.Internet, error) {
	result, err := api.op.Read(ctx, zone, id)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("ルータ＋スイッチ %d の読み込み結果が空です", id)
	}
	return result, nil
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	if err := requireID(request.ID); err != nil {
		return err
	}
	if request.Force {
		internet, err := api.op.Read(ctx, request.Zone, request.ID)
		if err != nil {
			if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
				return nil
			}
			return err
		}
		if internet.Switch != nil {
			for _, ipv6 := range internet.Switch.IPv6Nets {
				if ipv6 == nil {
					continue
				}
				if err := api.op.DisableIPv6(ctx, request.Zone, request.ID, ipv6.ID); err != nil {
					return err
				}
			}
			for _, subnet := range internet.Switch.Subnets {
				if subnet == nil || subnet.NextHop == "" {
					continue
				}
				if err := api.op.DeleteSubnet(ctx, request.Zone, request.ID, subnet.ID); err != nil {
					return err
				}
			}
		}
	}
	err := api.op.Delete(ctx, request.Zone, request.ID)
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}

func (api *sdkAPI) ListSubnetWithContext(ctx context.Context, request *ListSubnetRequest) ([]*iaas.Subnet, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	internet, err := api.op.Read(ctx, request.Zone, request.ID)
	if err != nil {
		return nil, err
	}
	if internet.Switch == nil {
		return []*iaas.Subnet{}, nil
	}
	result := make([]*iaas.Subnet, 0, len(internet.Switch.Subnets))
	for _, subnet := range internet.Switch.Subnets {
		if subnet == nil || subnet.ID.IsEmpty() {
			return nil, fmt.Errorf("ルータ＋スイッチ %d のサブネット応答に ID がありません", request.ID)
		}
		item, err := api.subnetOp.Read(ctx, request.Zone, subnet.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, fmt.Errorf("サブネット %d の読み込み結果が空です", subnet.ID)
		}
		result = append(result, item)
	}
	return result, nil
}

func (api *sdkAPI) AddSubnetWithContext(ctx context.Context, request *AddSubnetRequest) (*iaas.InternetSubnetOperationResult, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	return api.op.AddSubnet(ctx, request.Zone, request.ID, &iaas.InternetAddSubnetRequest{
		NetworkMaskLen: request.NetworkMaskLen,
		NextHop:        request.NextHop,
	})
}

func (api *sdkAPI) UpdateSubnetWithContext(ctx context.Context, request *UpdateSubnetRequest) (*iaas.InternetSubnetOperationResult, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	if err := requireID(request.SubnetID); err != nil {
		return nil, fmt.Errorf("SubnetID: %w", err)
	}
	return api.op.UpdateSubnet(ctx, request.Zone, request.ID, request.SubnetID, &iaas.InternetUpdateSubnetRequest{
		NextHop: request.NextHop,
	})
}

func (api *sdkAPI) DeleteSubnetWithContext(ctx context.Context, request *DeleteSubnetRequest) error {
	if err := requireID(request.ID); err != nil {
		return err
	}
	if err := requireID(request.SubnetID); err != nil {
		return fmt.Errorf("SubnetID: %w", err)
	}
	return api.op.DeleteSubnet(ctx, request.Zone, request.ID, request.SubnetID)
}

func (api *sdkAPI) ReadIPv6WithContext(ctx context.Context, request *ReadIPv6Request) (*iaas.IPv6NetInfo, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	internet, err := api.op.Read(ctx, request.Zone, request.ID)
	if err != nil {
		return nil, err
	}
	if internet.Switch != nil {
		for _, ipv6 := range internet.Switch.IPv6Nets {
			if ipv6 != nil {
				return ipv6, nil
			}
		}
	}
	return nil, fmt.Errorf("IPv6 ネットワークが割り当てられていません")
}

func (api *sdkAPI) EnableIPv6WithContext(ctx context.Context, request *EnableIPv6Request) (*iaas.IPv6NetInfo, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	return api.op.EnableIPv6(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) DisableIPv6WithContext(ctx context.Context, request *DisableIPv6Request) error {
	if err := requireID(request.ID); err != nil {
		return err
	}
	return api.disableIPv6(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) disableIPv6(ctx context.Context, zone string, id types.ID) error {
	internet, err := api.op.Read(ctx, zone, id)
	if err != nil {
		return err
	}
	if internet.Switch == nil {
		return nil
	}
	for _, ipv6 := range internet.Switch.IPv6Nets {
		if ipv6 == nil {
			continue
		}
		if err := api.op.DisableIPv6(ctx, zone, id, ipv6.ID); err != nil {
			return err
		}
	}
	return nil
}

func (api *sdkAPI) MonitorRouterWithContext(ctx context.Context, request *MonitorRouterRequest) ([]*iaas.MonitorRouterValue, error) {
	if err := requireID(request.ID); err != nil {
		return nil, err
	}
	end := request.End
	if end.IsZero() {
		end = time.Now()
	}
	start := request.Start
	if start.IsZero() {
		start = end.Add(-time.Hour)
	}
	result, err := api.op.MonitorRouter(ctx, request.Zone, request.ID, &iaas.MonitorCondition{
		Start: start,
		End:   end,
	})
	if err != nil {
		return nil, err
	}
	return result.Values, nil
}

func requireID(id types.ID) error {
	if id == 0 {
		return fmt.Errorf("ID is required")
	}
	return nil
}
