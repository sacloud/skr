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
)

// filterAddress is a documentation range (RFC 5737 TEST-NET-1) used to verify
// the packet-filter update; the filter is restored to the disabled state
// afterwards.
const filterAddress = "192.0.2.1"

type packetFilter struct {
	IsEnabled bool `json:"is_enabled"`
	Settings  []struct {
		FromIP             string `json:"from_ip"`
		FromIPPrefixLength int    `json:"from_ip_prefix_length"`
	} `json:"settings"`
}

// packetFilterScenario exercises packet-filter read and update. The scenario
// enables the filter with an IPv4 setting, verifies the reflection, and then
// restores the disabled state.
func (s *scenario) packetFilterScenario(ctx context.Context, appID string) error {
	initial, err := s.readPacketFilter(ctx, "read-packet-filter", appID)
	if err != nil {
		return err
	}
	if initial.IsEnabled {
		return fmt.Errorf("packet filter is already enabled for ID %q; refusing to modify it", appID)
	}

	if _, err := s.call(ctx, "update-packet-filter", "packet-filter", "update", appID,
		"--request", fmt.Sprintf(`{"is_enabled":true,"settings":[{"from_ip":"%s","from_ip_prefix_length":%d}]}`, filterAddress, 32)); err != nil {
		return err
	}
	updated, err := s.readPacketFilter(ctx, "read-updated-packet-filter", appID)
	if err != nil {
		return err
	}
	if err := checkPacketFilter(updated, true); err != nil {
		return err
	}

	if _, err := s.call(ctx, "restore-packet-filter", "packet-filter", "update", appID,
		"--request", `{"is_enabled":false}`); err != nil {
		return fmt.Errorf("restore packet filter: %w", err)
	}
	restored, err := s.readPacketFilter(ctx, "read-restored-packet-filter", appID)
	if err != nil {
		return fmt.Errorf("read restored packet filter: %w", err)
	}
	return checkPacketFilter(restored, false)
}

func (s *scenario) readPacketFilter(ctx context.Context, step, appID string) (packetFilter, error) {
	data, err := s.call(ctx, step, "packet-filter", "read", appID)
	if err != nil {
		return packetFilter{}, err
	}
	var result packetFilter
	if err := json.Unmarshal(data, &result); err != nil {
		return packetFilter{}, fmt.Errorf("%s: decode AppRun packet filter: %w", step, err)
	}
	return result, nil
}

func checkPacketFilter(value packetFilter, wantEnabled bool) error {
	if value.IsEnabled != wantEnabled {
		return fmt.Errorf("packet filter is_enabled = %v, want %v", value.IsEnabled, wantEnabled)
	}
	if wantEnabled {
		if len(value.Settings) != 1 || value.Settings[0].FromIP != filterAddress || value.Settings[0].FromIPPrefixLength != 32 {
			return fmt.Errorf("packet filter settings mismatch: %+v", value.Settings)
		}
	}
	return nil
}
