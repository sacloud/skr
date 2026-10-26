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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// addedSubnetMaskLen は、作成した Router + Switch に追加された
// IP アドレスブロックのプレフィックス長である。
const addedSubnetMaskLen = 28

// subnetResponse は add-subnet と update-subnet が返す Subnet リソースである。
// update-subnet でも返される。
type subnetResponse struct {
	ID             types.ID `json:"ID"`
	NextHop        string   `json:"NextHop"`
	NetworkAddress string   `json:"NetworkAddress"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
}

// updatedNextHop はサブネット更新確認に使用される next-hop アドレスを返します。
// Server のアドレスの次のアドレスで、プライマリサブネット内にとどまり、
// 初期の next hop とは異なります。
func updatedNextHop(serverIP string) (string, error) {
	address, err := netip.ParseAddr(serverIP)
	if err != nil || !address.Is4() {
		return "", fmt.Errorf("invalid Server IPv4 address %q", serverIP)
	}
	next := address.Next()
	if !next.Is4() {
		return "", fmt.Errorf("the next address of Server IPv4 address %q is not IPv4", serverIP)
	}
	return next.String(), nil
}

// subnetScenario は、作成した Router + Switch に対し、
// Server のアドレスを next hop として add-subnet、update-subnet、delete-subnet を実行します。
// 追加された IP アドレスブロックは削除され、
// 既存の削除フローが実行される前に不在が確認されます。
func (s scenario) subnetScenario(ctx context.Context, internetID types.ID, serverIP string) error {
	nextHop, err := updatedNextHop(serverIP)
	if err != nil {
		return err
	}
	added, err := s.addSubnet(ctx, "test-add-subnet", internetID, serverIP)
	if err != nil {
		return err
	}
	subnetID := added.ID
	if subnetID.IsEmpty() || added.NextHop != serverIP || added.NetworkMaskLen != addedSubnetMaskLen {
		return fmt.Errorf("added Subnet identity mismatch for Internet ID %s", internetID)
	}
	updated, err := s.updateSubnet(ctx, "test-update-subnet", internetID, subnetID, nextHop)
	if err != nil {
		return err
	}
	if updated.ID != subnetID || updated.NextHop != nextHop {
		return fmt.Errorf("updated Subnet NextHop mismatch for Subnet ID %s (expected next hop %s)", subnetID, nextHop)
	}
	if err := s.deleteSubnet(ctx, "test-delete-subnet", internetID, subnetID); err != nil {
		return err
	}
	present, err := s.subnetPresent(ctx, internetID, subnetID)
	if err != nil {
		return err
	}
	if present {
		return fmt.Errorf("added Subnet %s remains after delete-subnet", subnetID)
	}
	return nil
}

func (s scenario) addSubnet(ctx context.Context, step string, internetID types.ID, nextHop string) (subnetResponse, error) {
	output, err := s.call(ctx, step, map[string]any{
		"Zone": zone, "ID": internetID, "NetworkMaskLen": addedSubnetMaskLen, "NextHop": nextHop,
	})
	if err != nil {
		return subnetResponse{}, err
	}
	return decodeSubnetResponse(step, output)
}

func (s scenario) updateSubnet(ctx context.Context, step string, internetID, subnetID types.ID, nextHop string) (subnetResponse, error) {
	output, err := s.call(ctx, step, map[string]any{
		"Zone": zone, "ID": internetID, "SubnetID": int64(subnetID), "NextHop": nextHop,
	})
	if err != nil {
		return subnetResponse{}, err
	}
	return decodeSubnetResponse(step, output)
}

func (s scenario) deleteSubnet(ctx context.Context, step string, internetID, subnetID types.ID) error {
	_, err := s.call(ctx, step, map[string]any{
		"Zone": zone, "ID": internetID, "SubnetID": int64(subnetID),
	})
	return err
}

// subnetPresent は Router + Switch のサブネットを一覧表示し、
// 指定した IP アドレスブロックがまだ割り当てられているかどうかを報告します。
func (s scenario) subnetPresent(ctx context.Context, internetID, subnetID types.ID) (bool, error) {
	output, err := s.call(ctx, "test-list-subnet", map[string]any{"Zone": zone, "ID": internetID})
	if err != nil {
		return false, err
	}
	var subnets []subnetItem
	if err := json.Unmarshal(output, &subnets); err != nil {
		return false, fmt.Errorf("decode Internet subnet list: %w", err)
	}
	if subnets == nil {
		return false, fmt.Errorf("expected a JSON subnet array for Internet ID %s", internetID)
	}
	for _, item := range subnets {
		if item.ID == subnetID {
			return true, nil
		}
	}
	return false, nil
}

func decodeSubnetResponse(step string, output []byte) (subnetResponse, error) {
	var item subnetResponse
	if err := json.Unmarshal(output, &item); err != nil {
		return subnetResponse{}, fmt.Errorf("%s: decode Subnet: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return subnetResponse{}, fmt.Errorf("%s: Subnet ID is empty", step)
	}
	return item, nil
}
