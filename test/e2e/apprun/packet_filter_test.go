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
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// filterAddress はパケットフィルターの設定確認に使う文書用の範囲（RFC 5737 TEST-NET-1）です。
// シナリオは更新の確認後、フィルターを無効状態に戻します。
const filterAddress = "192.0.2.1"

// packetFilter は CLI の JSON 出力をパケットフィルターとして解釈するための項目です。
type packetFilter struct {
	IsEnabled bool `json:"is_enabled"`
	Settings  []struct {
		FromIP             string `json:"from_ip"`
		FromIPPrefixLength int    `json:"from_ip_prefix_length"`
	} `json:"settings"`
}

// runPacketFilterScenario はパケットフィルターの参照と更新を実行します。
// 更新前にフィルターが無効であることを確認し、IPv4 設定で有効にして反映を確認してから
// 無効状態に戻します。
func runPacketFilterScenario(t *testing.T, h *helper.Helper, appID string) {
	t.Helper()

	// 更新前にパケットフィルターが無効であることを確認する
	initial := parsePacketFilter(t, h.Call(t, "パケットフィルターを参照する",
		"apprun-api", "packet-filter", "read", appID), appID)
	require.False(t, initial.IsEnabled, "packet filter is already enabled for %s; refusing to modify it", appID)

	// パケットフィルターを有効にする
	updateRequest := h.WriteRequestJSONFile(t, "packet-filter-update", map[string]any{
		"is_enabled": true,
		"settings":   []any{map[string]any{"from_ip": filterAddress, "from_ip_prefix_length": 32}},
	})
	_ = h.Call(t, "パケットフィルターを有効にする",
		"apprun-api", "packet-filter", "update", appID, "--request", "@"+updateRequest)

	// 有効化した設定がパケットフィルターの参照に反映されていることを確認する
	updated := parsePacketFilter(t, h.Call(t, "更新後のパケットフィルターを参照する",
		"apprun-api", "packet-filter", "read", appID), appID)
	require.True(t, updated.IsEnabled, "packet filter should be enabled after the update")
	require.Len(t, updated.Settings, 1, "packet filter should have the setting requested by this run")
	require.Equal(t, filterAddress, updated.Settings[0].FromIP, "packet filter from_ip should match the value requested by this run")
	require.Equal(t, 32, updated.Settings[0].FromIPPrefixLength, "packet filter from_ip_prefix_length should match the value requested by this run")

	// パケットフィルターを無効状態に戻す
	restoreRequest := h.WriteRequestJSONFile(t, "packet-filter-restore", map[string]any{"is_enabled": false})
	_ = h.Call(t, "パケットフィルターを無効状態に戻す",
		"apprun-api", "packet-filter", "update", appID, "--request", "@"+restoreRequest)

	// 無効状態に戻ったことを確認する
	restored := parsePacketFilter(t, h.Call(t, "元に戻ったパケットフィルターを参照する",
		"apprun-api", "packet-filter", "read", appID), appID)
	require.False(t, restored.IsEnabled, "packet filter should be restored to the disabled state")
}

// parsePacketFilter は CLI の JSON 出力をパケットフィルターとして解釈します。
func parsePacketFilter(t *testing.T, data []byte, appID string) packetFilter {
	t.Helper()

	var value packetFilter
	err := json.Unmarshal(data, &value)
	require.NoError(t, err, "decode packet filter of application %s: %v", appID, err)
	return value
}
