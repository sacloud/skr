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
	"fmt"
	"net/netip"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipv6NetItem は enable-ipv6 と read-ipv6 が返す IPv6 ネットワークです。
type ipv6NetItem struct {
	ID            types.ID `json:"ID"`
	IPv6Prefix    string   `json:"IPv6Prefix"`
	IPv6PrefixLen int      `json:"IPv6PrefixLen"`
}

// decodeIPv6Net は IPv6 ネットワークの参照応答を解析します。
func decodeIPv6Net(data []byte) (ipv6NetItem, error) {
	var item ipv6NetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ipv6NetItem{}, fmt.Errorf("decode IPv6Net: %w", err)
	}
	return item, nil
}

// checkIPv6Net は IPv6 ネットワークの応答が期待する内容 (ID と有効な IPv6 プレフィクス) と
// 一致することを確認します。
func checkIPv6Net(item ipv6NetItem, internetID types.ID) error {
	prefix, err := netip.ParseAddr(item.IPv6Prefix)
	if item.ID.IsEmpty() || err != nil || !prefix.Is6() ||
		item.IPv6PrefixLen < 1 || item.IPv6PrefixLen > 128 {
		return fmt.Errorf("enabled IPv6Net has an invalid identity for Internet ID %s (prefix %q, prefix length %d)", internetID, item.IPv6Prefix, item.IPv6PrefixLen)
	}
	return nil
}

// ipv6Scenario は作成したルータ＋スイッチに対し、enable-ipv6、read-ipv6、disable-ipv6 を
// 実行し、有効化後の参照と無効化後の参照失敗を確認します。
// Internet 削除は IPv6 の無効化も行うため、無効化した状態で後片て付けに進みます。
func ipv6Scenario(t *testing.T, h *helper.Helper, internetID types.ID) {
	t.Helper()

	// IPv6 アドレス割り当てを有効化します
	data, _, err := h.CallAllowingFailure(t, "IPv6 アドレス割り当てを有効化する",
		"iaas-api", "internet", "enable-ipv6", "--zone", internetZone, "--id", internetID.String(),
	)
	require.NoError(t, err, "enable-ipv6 should succeed")
	enabled, err := decodeIPv6Net(data)
	require.NoError(t, err, "decode enable-ipv6 response")
	require.NoError(t, checkIPv6Net(enabled, internetID),
		"enable-ipv6 should return a valid IPv6 network")

	// 参照で有効化した内容を確認します
	read := readIPv6(t, h, "有効化した IPv6 ネットワークを参照する", internetID)
	require.Equal(t, enabled, read, "read-ipv6 should return the enabled IPv6 network")

	// IPv6 アドレス割り当てを無効化します
	h.Call(t, "IPv6 アドレス割り当てを無効化する",
		"iaas-api", "internet", "disable-ipv6", "--zone", internetZone, "--id", internetID.String(),
	)

	// 無効化後は参照が失敗すること
	_, stderr, err := h.CallAllowingFailure(t, "無効化後に IPv6 ネットワークを参照する",
		"iaas-api", "internet", "read-ipv6", "--zone", internetZone, "--id", internetID.String(),
	)
	require.Error(t, err, "read-ipv6 should fail after disable-ipv6 (stderr: %s)", stderr)
}

// readIPv6 は skr CLI で IPv6 ネットワークを参照します。
func readIPv6(t *testing.T, h *helper.Helper, description string, internetID types.ID) ipv6NetItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "internet", "read-ipv6", "--zone", internetZone, "--id", internetID.String(),
		"--query", "{ID,IPv6Prefix,IPv6PrefixLen}")
	item, err := decodeIPv6Net(data)
	require.NoError(t, err, "%s: decode IPv6Net", description)
	return item
}

// TestDecodeIPv6Net は IPv6 ネットワークの参照応答の解析を確認します。
func TestDecodeIPv6Net(t *testing.T) {
	item, err := decodeIPv6Net([]byte(`{"ID":16,"IPv6Prefix":"2001:db8::","IPv6PrefixLen":64}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(16), item.ID)
	require.Equal(t, "2001:db8::", item.IPv6Prefix)
	require.Equal(t, 64, item.IPv6PrefixLen)

	_, err = decodeIPv6Net([]byte("{"))
	require.Error(t, err, "invalid JSON should fail")
}

// TestCheckIPv6Net は IPv6 ネットワークの応答の内容確認を確認します。
func TestCheckIPv6Net(t *testing.T) {
	require.NoError(t, checkIPv6Net(ipv6NetItem{ID: 16, IPv6Prefix: "2001:db8::", IPv6PrefixLen: 64}, 12))

	require.Error(t, checkIPv6Net(ipv6NetItem{IPv6Prefix: "2001:db8::", IPv6PrefixLen: 64}, 12),
		"an empty ID should be refused")
	require.Error(t, checkIPv6Net(ipv6NetItem{ID: 16, IPv6Prefix: "invalid", IPv6PrefixLen: 64}, 12),
		"an invalid prefix should be refused")
	require.Error(t, checkIPv6Net(ipv6NetItem{ID: 16, IPv6Prefix: "192.0.2.2", IPv6PrefixLen: 64}, 12),
		"an IPv4 prefix should be refused")
	require.Error(t, checkIPv6Net(ipv6NetItem{ID: 16, IPv6Prefix: "2001:db8::", IPv6PrefixLen: 0}, 12),
		"an out-of-range prefix length should be refused")
	require.Error(t, checkIPv6Net(ipv6NetItem{ID: 16, IPv6Prefix: "2001:db8::", IPv6PrefixLen: 129}, 12),
		"an out-of-range prefix length should be refused")
}

// TestIPv6NetDecodesUnknownFields は未知のフィールドを含む応答も解析できることを確認します。
func TestIPv6NetDecodesUnknownFields(t *testing.T) {
	item, err := decodeIPv6Net([]byte(`{"ID":16,"IPv6Prefix":"2001:db8::","IPv6PrefixLen":64,"Unknown":"x"}`))
	require.NoError(t, err, "unknown fields should be ignored")
	require.Equal(t, "2001:db8::", item.IPv6Prefix)
}
