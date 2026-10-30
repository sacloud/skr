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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipv6NetItem は ipv6net コマンドの参照結果です。
type ipv6NetItem struct {
	ID            types.ID `json:"ID"`
	IPv6Prefix    string   `json:"IPv6Prefix"`
	IPv6PrefixLen int      `json:"IPv6PrefixLen"`
}

// decodeIPv6Net は ipv6net コマンドの参照応答を解析します。
func decodeIPv6Net(data []byte) (ipv6NetItem, error) {
	var item ipv6NetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ipv6NetItem{}, fmt.Errorf("decode IPv6Net: %w", err)
	}
	return item, nil
}

// checkIPv6NetItem は IPv6 ネットワークの応答が有効な内容を持つことを確認します。
func checkIPv6NetItem(item ipv6NetItem, internetID types.ID) error {
	if item.ID.IsEmpty() {
		return fmt.Errorf("IPv6Net ID is empty (expected for Internet %s)", internetID)
	}
	if item.IPv6Prefix == "" || item.IPv6PrefixLen == 0 {
		return fmt.Errorf("IPv6Net %s has empty prefix (expected for Internet %s)", item.ID, internetID)
	}
	return nil
}

// enableIPv6 は skr CLI でルータ＋スイッチの IPv6 アドレス割り当てを有効化します。
// 応答は --query で必要な項目に絞って解析します。
func enableIPv6(t *testing.T, h *helper.Helper, internetID types.ID) (ipv6NetItem, error) {
	t.Helper()

	data := h.Call(t, "ルータ＋スイッチの IPv6 アドレス割り当てを有効化する",
		"iaas-api", "internet", "enable-ipv6",
		"--zone", ipv6NetZone, "--id", internetID.String(),
		"--query", "{ID,IPv6Prefix,IPv6PrefixLen}",
	)
	item, err := decodeIPv6Net(data)
	if err != nil {
		return ipv6NetItem{}, err
	}
	return item, nil
}

// findIPv6Nets は skr CLI でゾーン内の IPv6 ネットワークを検索し、必要な項目に絞って返します。
func findIPv6Nets(t *testing.T, h *helper.Helper) []ipv6NetItem {
	t.Helper()

	data := h.Call(t, "ゾーン内の IPv6 ネットワークを検索する",
		"iaas-api", "ipv6net", "find",
		"--zone", ipv6NetZone,
		"--query", "map({ID,IPv6Prefix,IPv6PrefixLen})",
	)
	var items []ipv6NetItem
	require.NoError(t, json.Unmarshal(data, &items), "decode ipv6net find response")
	return items
}

// findIPv6NetByID は検索結果から指定した ID の IPv6 ネットワークを返します。
// 見つからない場合は nil を返します。
func findIPv6NetByID(items []ipv6NetItem, id types.ID) *ipv6NetItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// readIPv6Net は skr CLI で指定した ID の IPv6 ネットワークを参照します。
// 応答は --query で必要な項目に絞って解析します。
func readIPv6Net(t *testing.T, h *helper.Helper, id types.ID) ipv6NetItem {
	t.Helper()

	data := h.Call(t, "IPv6 ネットワークを参照する",
		"iaas-api", "ipv6net", "read",
		"--zone", ipv6NetZone, "--id", id.String(),
		"--query", "{ID,IPv6Prefix,IPv6PrefixLen}",
	)
	item, err := decodeIPv6Net(data)
	require.NoError(t, err, "decode ipv6net read response")
	return item
}

// TestDecodeIPv6Net は ipv6net コマンドの参照応答の解析を確認します。
func TestDecodeIPv6Net(t *testing.T) {
	item, err := decodeIPv6Net([]byte(`{"ID":1,"IPv6Prefix":"2001:db8:0:1::","IPv6PrefixLen":64}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(1), item.ID)
	require.Equal(t, "2001:db8:0:1::", item.IPv6Prefix)
	require.Equal(t, 64, item.IPv6PrefixLen)

	_, err = decodeIPv6Net([]byte(`not-json`))
	require.Error(t, err, "invalid JSON should fail")
}

// TestCheckIPv6NetItem は IPv6 ネットワークの内容確認を検証します。
func TestCheckIPv6NetItem(t *testing.T) {
	valid := ipv6NetItem{ID: types.ID(1), IPv6Prefix: "2001:db8:0:1::", IPv6PrefixLen: 64}
	require.NoError(t, checkIPv6NetItem(valid, types.ID(100)))

	noPrefix := ipv6NetItem{ID: types.ID(1), IPv6PrefixLen: 64}
	require.Error(t, checkIPv6NetItem(noPrefix, types.ID(100)), "response without prefix should fail")

	noID := ipv6NetItem{IPv6Prefix: "2001:db8:0:1::", IPv6PrefixLen: 64}
	require.Error(t, checkIPv6NetItem(noID, types.ID(100)), "response without ID should fail")
}

// TestFindIPv6NetByID は検索結果からの ID 指定検索を検証します。
func TestFindIPv6NetByID(t *testing.T) {
	items := []ipv6NetItem{
		{ID: types.ID(1), IPv6Prefix: "2001:db8:0:1::", IPv6PrefixLen: 64},
		{ID: types.ID(2), IPv6Prefix: "2001:db8:0:2::", IPv6PrefixLen: 64},
	}
	found := findIPv6NetByID(items, types.ID(2))
	require.NotNil(t, found)
	require.Equal(t, "2001:db8:0:2::", found.IPv6Prefix)

	require.Nil(t, findIPv6NetByID(items, types.ID(3)), "unknown ID should not be found")
}
