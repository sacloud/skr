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

// ipAddressItem は ipaddress コマンドの参照結果です。
type ipAddressItem struct {
	IPAddress string `json:"IPAddress"`
	HostName  string `json:"HostName"`
}

// decodeIPAddress は ipaddress コマンドの参照応答を解析します。
func decodeIPAddress(data []byte) (ipAddressItem, error) {
	var item ipAddressItem
	if err := json.Unmarshal(data, &item); err != nil {
		return ipAddressItem{}, err
	}
	return item, nil
}

// listIPAddresses は skr CLI でゾーン内のグローバル IP アドレスを一覧し、必要な項目に絞って返します。
func listIPAddresses(t *testing.T, h *helper.Helper) []ipAddressItem {
	t.Helper()

	data := h.Call(t, "ゾーン内のグローバル IP アドレスを一覧する",
		"iaas-api", "ipaddress", "list",
		"--zone", ipAddressZone,
		"--query", "map({IPAddress,HostName})",
	)
	var items []ipAddressItem
	require.NoError(t, json.Unmarshal(data, &items), "decode ipaddress list response")
	return items
}

// readIPAddress は skr CLI で指定したグローバル IP アドレスを参照します。
// 応答は --query で必要な項目に絞って解析します。
func readIPAddress(t *testing.T, h *helper.Helper, addr string) ipAddressItem {
	t.Helper()

	data := h.Call(t, "グローバル IP アドレスを参照する",
		"iaas-api", "ipaddress", "read",
		"--zone", ipAddressZone, "--ip-address", addr,
		"--query", "{IPAddress,HostName}",
	)
	item, err := decodeIPAddress(data)
	require.NoError(t, err, "decode ipaddress read response")
	return item
}

// TestDecodeIPAddress は ipaddress コマンドの参照応答の解析を確認します。
func TestDecodeIPAddress(t *testing.T) {
	item, err := decodeIPAddress([]byte(`{"IPAddress":"192.0.2.1","HostName":"www.example.com"}`))
	require.NoError(t, err)
	require.Equal(t, "192.0.2.1", item.IPAddress)
	require.Equal(t, "www.example.com", item.HostName)

	_, err = decodeIPAddress([]byte(`not-json`))
	require.Error(t, err, "invalid JSON should fail")
}
