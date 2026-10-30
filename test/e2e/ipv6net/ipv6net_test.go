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

// decodeIPv6Net は ipv6net コマンドの参照応答を解析します。
func decodeIPv6Net(data []byte) (helper.IPv6NetItem, error) {
	var item helper.IPv6NetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return helper.IPv6NetItem{}, fmt.Errorf("decode IPv6Net: %w", err)
	}
	return item, nil
}

// findIPv6Nets は skr CLI でゾーン内の IPv6 ネットワークを検索し、必要な項目に絞って返します。
func findIPv6Nets(t *testing.T, h *helper.Helper) []helper.IPv6NetItem {
	t.Helper()

	data := h.Call(t, "ゾーン内の IPv6 ネットワークを検索する",
		"iaas-api", "ipv6net", "find",
		"--zone", helper.InternetZone,
		"--query", "map({ID,IPv6Prefix,IPv6PrefixLen})",
	)
	var items []helper.IPv6NetItem
	require.NoError(t, json.Unmarshal(data, &items), "decode ipv6net find response")
	return items
}

// findIPv6NetByID は検索結果から指定した ID の IPv6 ネットワークを返します。
// 見つからない場合は nil を返します。
func findIPv6NetByID(items []helper.IPv6NetItem, id types.ID) *helper.IPv6NetItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// readIPv6Net は skr CLI で指定した ID の IPv6 ネットワークを参照します。
// 応答は --query で必要な項目に絞って解析します。
func readIPv6Net(t *testing.T, h *helper.Helper, id types.ID) helper.IPv6NetItem {
	t.Helper()

	data := h.Call(t, "IPv6 ネットワークを参照する",
		"iaas-api", "ipv6net", "read",
		"--zone", helper.InternetZone, "--id", id.String(),
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

// TestFindIPv6NetByID は検索結果からの ID 指定検索を検証します。
func TestFindIPv6NetByID(t *testing.T) {
	items := []helper.IPv6NetItem{
		{ID: types.ID(1), IPv6Prefix: "2001:db8:0:1::", IPv6PrefixLen: 64},
		{ID: types.ID(2), IPv6Prefix: "2001:db8:0:2::", IPv6PrefixLen: 64},
	}
	found := findIPv6NetByID(items, types.ID(2))
	require.NotNil(t, found)
	require.Equal(t, "2001:db8:0:2::", found.IPv6Prefix)

	require.Nil(t, findIPv6NetByID(items, types.ID(3)), "unknown ID should not be found")
}
