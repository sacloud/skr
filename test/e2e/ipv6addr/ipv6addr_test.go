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

// ipv6AddrItem は ipv6addr コマンドの参照結果です。
type ipv6AddrItem struct {
	IPv6Addr string `json:"IPv6Addr"`
	HostName string `json:"HostName"`
}

// decodeIPv6Addr は ipv6addr コマンドの参照応答を解析します。
func decodeIPv6Addr(data []byte) (ipv6AddrItem, error) {
	var item ipv6AddrItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ipv6AddrItem{}, err
	}
	return item, nil
}

// findIPv6Addrs は skr CLI でゾーン内の IPv6 アドレスを検索し、必要な項目に絞って返します。
func findIPv6Addrs(t *testing.T, h *helper.Helper) []ipv6AddrItem {
	t.Helper()

	data := h.Call(t, "ゾーン内の IPv6 アドレスを検索する",
		"iaas-api", "ipv6addr", "find",
		"--zone", ipv6AddrZone,
		"--query", "map({IPv6Addr,HostName})",
	)
	var items []ipv6AddrItem
	require.NoError(t, json.Unmarshal(data, &items), "decode ipv6addr find response")
	return items
}

// readIPv6Addr は skr CLI で指定した IPv6 アドレスを参照します。
// 応答は --query で必要な項目に絞って解析します。
func readIPv6Addr(t *testing.T, h *helper.Helper, addr string) ipv6AddrItem {
	t.Helper()

	data := h.Call(t, "IPv6 アドレスを参照する",
		"iaas-api", "ipv6addr", "read",
		"--zone", ipv6AddrZone, "--ipv6-addr", addr,
		"--query", "{IPv6Addr,HostName}",
	)
	item, err := decodeIPv6Addr(data)
	require.NoError(t, err, "decode ipv6addr read response")
	return item
}

// createIPv6Addr は skr CLI で IPv6 アドレスに逆引きホスト名を登録します。
// 応答は --query で必要な項目に絞って解析します。
func createIPv6Addr(t *testing.T, h *helper.Helper, addr, hostName string) ipv6AddrItem {
	t.Helper()

	data := h.Call(t, "IPv6 アドレスに逆引きホスト名を登録する",
		"iaas-api", "ipv6addr", "create",
		"--zone", ipv6AddrZone, "--ipv6-addr", addr, "--host-name", hostName,
		"--query", "{IPv6Addr,HostName}",
	)
	item, err := decodeIPv6Addr(data)
	require.NoError(t, err, "decode ipv6addr create response")
	return item
}

// updateIPv6Addr は skr CLI で IPv6 アドレスの逆引きホスト名を変更します。
// 応答は --query で必要な項目に絞って解析します。
func updateIPv6Addr(t *testing.T, h *helper.Helper, addr, hostName string) ipv6AddrItem {
	t.Helper()

	data := h.Call(t, "IPv6 アドレスの逆引きホスト名を変更する",
		"iaas-api", "ipv6addr", "update",
		"--zone", ipv6AddrZone, "--ipv6-addr", addr, "--host-name", hostName,
		"--query", "{IPv6Addr,HostName}",
	)
	item, err := decodeIPv6Addr(data)
	require.NoError(t, err, "decode ipv6addr update response")
	return item
}

// deleteIPv6Addr は skr CLI で IPv6 アドレスの逆引きホスト名の登録を解除します。
// 成功時は出力しないため、実行の成否のみを確認します。
func deleteIPv6Addr(t *testing.T, h *helper.Helper, addr string) {
	t.Helper()

	_, _, err := h.CallAllowingFailure(t, "IPv6 アドレスの逆引きホスト名の登録を解除する",
		"iaas-api", "ipv6addr", "delete",
		"--zone", ipv6AddrZone, "--ipv6-addr", addr,
	)
	require.NoError(t, err, "ipv6addr delete should succeed")
}

// findIPv6AddrByAddr は検索結果から指定したアドレスの IPv6 アドレスを返します。
// 見つからない場合は nil を返します。
func findIPv6AddrByAddr(items []ipv6AddrItem, addr string) *ipv6AddrItem {
	for i := range items {
		if items[i].IPv6Addr == addr {
			return &items[i]
		}
	}
	return nil
}

// TestDecodeIPv6Addr は ipv6addr コマンドの参照応答の解析を確認します。
func TestDecodeIPv6Addr(t *testing.T) {
	item, err := decodeIPv6Addr([]byte(`{"IPv6Addr":"2001:db8::1","HostName":"www.example.com"}`))
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", item.IPv6Addr)
	require.Equal(t, "www.example.com", item.HostName)

	_, err = decodeIPv6Addr([]byte(`not-json`))
	require.Error(t, err, "invalid JSON should fail")
}

// TestFindIPv6AddrByAddr は検索結果からのアドレス指定検索を検証します。
func TestFindIPv6AddrByAddr(t *testing.T) {
	items := []ipv6AddrItem{
		{IPv6Addr: "2001:db8::1", HostName: "a.example.com"},
		{IPv6Addr: "2001:db8::2", HostName: "b.example.com"},
	}
	found := findIPv6AddrByAddr(items, "2001:db8::2")
	require.NotNil(t, found)
	require.Equal(t, "b.example.com", found.HostName)

	require.Nil(t, findIPv6AddrByAddr(items, "2001:db8::3"), "unknown address should not be found")
}
