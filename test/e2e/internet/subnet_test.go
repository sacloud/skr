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
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// subnetResponse は add-subnet と update-subnet が返す Subnet リソースです。
type subnetResponse struct {
	ID             types.ID `json:"ID"`
	NextHop        string   `json:"NextHop"`
	NetworkAddress string   `json:"NetworkAddress"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
}

// addedSubnetMaskLen は追加する IP アドレスブロックのプレフィクス長です。
const addedSubnetMaskLen = 28

// decodeSubnetResponse はサブネット操作の応答を解析します。ID が空の場合はエラーを返します。
func decodeSubnetResponse(data []byte) (subnetResponse, error) {
	var item subnetResponse
	if err := json.Unmarshal(data, &item); err != nil {
		return subnetResponse{}, fmt.Errorf("decode Subnet: %w", err)
	}
	if item.ID.IsEmpty() {
		return subnetResponse{}, errors.New("Subnet ID is empty")
	}
	return item, nil
}

// updatedNextHop はサブネット更新確認に使う next-hop アドレスを返します。
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

// subnetPresent は SDK で追加サブネットが残っているかを確認します。
// 後片て付けでの不在確認など、証跡に残さない安全確認で使います。
func subnetPresent(t *testing.T, internets *internetSDK.Service, internetID, subnetID types.ID) (bool, error) {
	t.Helper()

	subnets, err := internets.ListSubnetWithContext(t.Context(), &internetSDK.ListSubnetRequest{Zone: internetZone, ID: internetID})
	if err != nil {
		return false, fmt.Errorf("list subnets of Internet %s: %w", internetID, err)
	}
	for _, s := range subnets {
		if s != nil && s.ID == subnetID {
			return true, nil
		}
	}
	return false, nil
}

// subnetScenario は作成したルータ＋スイッチに対し、Server のアドレスを next hop として
// add-subnet、update-subnet、delete-subnet を実行し、削除後の不在を確認します。
// ネクストホップのサーバーを先に用意する必要があるため、Server の作成と SSH ログインのあとに
// 実行します。追加サブネットは Internet 削除とともに後片て付けされるため、ここで削除します。
func subnetScenario(t *testing.T, h *helper.Helper, internets *internetSDK.Service, internetID types.ID, serverIP string) {
	t.Helper()

	nextHop, err := updatedNextHop(serverIP)
	require.NoError(t, err, "derive the added subnet next hop from the Server IPv4 address")

	// 追加サブネットを追加します。next hop は Server のアドレスを指定します
	data, _, err := h.CallAllowingFailure(t, "追加サブネットを追加する",
		"iaas-api", "internet", "add-subnet",
		"--zone", internetZone, "--id", internetID.String(),
		"--network-mask-len", fmt.Sprintf("%d", addedSubnetMaskLen),
		"--next-hop", serverIP,
	)
	require.NoError(t, err, "add-subnet should succeed")
	added, err := decodeSubnetResponse(data)
	require.NoError(t, err, "decode add-subnet response")
	require.Equal(t, serverIP, added.NextHop,
		"add-subnet should return the requested next hop")
	require.Equal(t, addedSubnetMaskLen, added.NetworkMaskLen,
		"add-subnet should return the requested prefix length")

	// 追加したサブネットの next hop を更新します
	updated, _, err := h.CallAllowingFailure(t, "追加したサブネットの next hop を更新する",
		"iaas-api", "internet", "update-subnet",
		"--zone", internetZone, "--id", internetID.String(),
		"--subnet-id", added.ID.String(), "--next-hop", nextHop,
	)
	require.NoError(t, err, "update-subnet should succeed")
	updatedSubnet, err := decodeSubnetResponse(updated)
	require.NoError(t, err, "decode update-subnet response")
	require.Equal(t, added.ID, updatedSubnet.ID,
		"update-subnet should return the same subnet")
	require.Equal(t, nextHop, updatedSubnet.NextHop,
		"update-subnet should return the updated next hop")

	// 追加したサブネットを削除します
	h.Call(t, "追加したサブネットを削除する",
		"iaas-api", "internet", "delete-subnet",
		"--zone", internetZone, "--id", internetID.String(),
		"--subnet-id", added.ID.String(),
	)

	// 削除後はサブネットが残っていないことを SDK で確認する
	present, err := subnetPresent(t, internets, internetID, added.ID)
	require.NoError(t, err, "verify the added subnet absence after delete")
	require.False(t, present, "the added subnet should be deleted")
}

// TestDecodeSubnetResponse はサブネット操作の応答の解析を確認します。
func TestDecodeSubnetResponse(t *testing.T) {
	item, err := decodeSubnetResponse([]byte(`{"ID":15,"NextHop":"192.0.2.2","NetworkAddress":"192.0.2.0","NetworkMaskLen":28}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(15), item.ID)
	require.Equal(t, "192.0.2.2", item.NextHop)
	require.Equal(t, "192.0.2.0", item.NetworkAddress)
	require.Equal(t, 28, item.NetworkMaskLen)

	_, err = decodeSubnetResponse([]byte("{"))
	require.Error(t, err, "invalid JSON should fail")
}

// TestUpdatedNextHop はサブネット更新確認に使う next-hop アドレスの導出を確認します。
func TestUpdatedNextHop(t *testing.T) {
	next, err := updatedNextHop("192.0.2.2")
	require.NoError(t, err)
	require.Equal(t, "192.0.2.3", next)

	_, err = updatedNextHop("invalid")
	require.Error(t, err, "an invalid Server address should be refused")

	_, err = updatedNextHop("2001:db8::1")
	require.Error(t, err, "an IPv6 Server address should be refused")
}
