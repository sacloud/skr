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

package helper

import (
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// TestParseInternetOutput はルータ＋スイッチの作成応答の解析を確認します。
func TestParseInternetOutput(t *testing.T) {
	item, err := ParseInternetOutput([]byte(`{"ID":12,"Name":"skr-e2e-ipv6net-test","Description":"d","NetworkMaskLen":28,"BandWidthMbps":100,"Switch":{"ID":13}}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(12), item.ID)
	require.Equal(t, "skr-e2e-ipv6net-test", item.Name)
	require.Equal(t, 28, item.NetworkMaskLen)
	require.Equal(t, 100, item.BandWidthMbps)
	require.Equal(t, types.ID(13), item.Switch.ID)

	_, err = ParseInternetOutput([]byte(`{"Name":"no-id"}`))
	require.Error(t, err, "response without ID should fail")
}

// TestCheckInternetIdentity はルータ＋スイッチの識別確認を検証します。
func TestCheckInternetIdentity(t *testing.T) {
	valid := InternetItem{ID: types.ID(1), Name: "n", Description: "d", NetworkMaskLen: 28, BandWidthMbps: 100}
	valid.Switch.ID = types.ID(2)
	require.NoError(t, CheckInternetIdentity(valid, types.ID(1), "n", "d", types.ID(2)))

	wrongName := valid
	wrongName.Name = "other"
	require.Error(t, CheckInternetIdentity(wrongName, types.ID(1), "n", "d", types.ID(2)), "name mismatch should fail")

	wrongMask := valid
	wrongMask.NetworkMaskLen = 27
	require.Error(t, CheckInternetIdentity(wrongMask, types.ID(1), "n", "d", types.ID(2)), "network mask mismatch should fail")

	noSwitch := valid
	noSwitch.Switch.ID = types.ID(0)
	require.Error(t, CheckInternetIdentity(noSwitch, types.ID(1), "n", "d", types.ID(0)), "empty switch ID should fail")
}

// TestCheckIPv6NetItem は IPv6 アドレス割り当ての内容確認を検証します。
