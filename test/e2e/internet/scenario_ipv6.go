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

// ipv6NetItem is the IPv6 network returned by enable-ipv6 and read-ipv6.
type ipv6NetItem struct {
	ID            types.ID `json:"ID"`
	IPv6Prefix    string   `json:"IPv6Prefix"`
	IPv6PrefixLen int      `json:"IPv6PrefixLen"`
}

// ipv6Scenario enables the IPv6 address allocation on the created Router +
// Switch, verifies the reflection with read-ipv6, disables the allocation, and
// verifies that read-ipv6 then fails. The allocation is removed before the
// existing delete flow runs.
func (s scenario) ipv6Scenario(ctx context.Context, internetID types.ID) error {
	enabled, err := s.enableIPv6(ctx, "test-enable-ipv6", internetID)
	if err != nil {
		return err
	}
	prefix, err := netip.ParseAddr(enabled.IPv6Prefix)
	if enabled.ID.IsEmpty() || err != nil || !prefix.Is6() ||
		enabled.IPv6PrefixLen < 1 || enabled.IPv6PrefixLen > 128 {
		return fmt.Errorf("enabled IPv6Net has an invalid identity for Internet ID %s", internetID)
	}
	read, err := s.readIPv6(ctx, "test-read-ipv6", internetID)
	if err != nil {
		return fmt.Errorf("read enabled IPv6Net: %w", err)
	}
	if read.ID != enabled.ID || read.IPv6Prefix != enabled.IPv6Prefix || read.IPv6PrefixLen != enabled.IPv6PrefixLen {
		return fmt.Errorf("read IPv6Net identity mismatch for Internet ID %s", internetID)
	}
	if err := s.disableIPv6(ctx, "test-disable-ipv6", internetID); err != nil {
		return fmt.Errorf("disable IPv6Net: %w", err)
	}
	if _, err := s.readIPv6(ctx, "test-read-ipv6", internetID); err == nil {
		return fmt.Errorf("read-ipv6 succeeded after disable-ipv6 for Internet ID %s", internetID)
	}
	return nil
}

func (s scenario) enableIPv6(ctx context.Context, step string, internetID types.ID) (ipv6NetItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "ID": internetID})
	if err != nil {
		return ipv6NetItem{}, err
	}
	return decodeIPv6Net(step, output)
}

func (s scenario) readIPv6(ctx context.Context, step string, internetID types.ID) (ipv6NetItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "ID": internetID})
	if err != nil {
		return ipv6NetItem{}, err
	}
	return decodeIPv6Net(step, output)
}

func (s scenario) disableIPv6(ctx context.Context, step string, internetID types.ID) error {
	_, err := s.call(ctx, step, map[string]any{"Zone": zone, "ID": internetID})
	return err
}

func decodeIPv6Net(step string, output []byte) (ipv6NetItem, error) {
	var item ipv6NetItem
	if err := json.Unmarshal(output, &item); err != nil {
		return ipv6NetItem{}, fmt.Errorf("%s: decode IPv6Net: %w", step, err)
	}
	return item, nil
}
