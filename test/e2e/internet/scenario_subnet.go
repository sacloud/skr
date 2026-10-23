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

// addedSubnetMaskLen is the prefix length of the IP address block added to the
// created Router + Switch.
const addedSubnetMaskLen = 28

// subnetResponse is the Subnet resource returned by add-subnet and
// update-subnet.
type subnetResponse struct {
	ID             types.ID `json:"ID"`
	NextHop        string   `json:"NextHop"`
	NetworkAddress string   `json:"NetworkAddress"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
}

// updatedNextHop returns the next-hop address used to verify the subnet update:
// the next address after the Server's address, which stays inside the primary
// subnet and differs from the initial next hop.
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

// subnetScenario exercises add-subnet, update-subnet, and delete-subnet against
// the created Router + Switch using the Server's address as the next hop. The
// added IP address block is deleted and its absence is verified before the
// existing delete flow runs.
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

// subnetPresent lists the Router + Switch's subnets and reports whether the
// given IP address block is still assigned.
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
