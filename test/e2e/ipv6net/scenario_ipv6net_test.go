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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioIPv6Net は ipv6net コマンドのライブ E2E です。ルータ＋スイッチを作成して
// IPv6 アドレス割り当てを有効化し、ipv6net find で作成した IPv6 ネットワークを見つけられる
// こと、ipv6net read で参照内容が有効化した内容と一致することを確認します。
// テスト対象の操作は skr CLI で実行し、事前確認と後片て付けの確認は SDK で静かに実行します。
func TestScenarioIPv6Net(t *testing.T) {
	h := helper.New(t, "ipv6net-api", "ipv6net find と read が、IPv6 アドレス割り当てを有効化した IPv6 ネットワークを正しく検索・参照できることを確認する")

	// 事前確認と後片て付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	internets := internetSDK.New(client)

	name := ipv6NetNamePrefix + helper.RandomSuffix(t)

	// 作成前に同名リソースが存在しないことを SDK で確認する。確認の実行は証跡に残らず、
	// 既存リソースを変更しないまま終了する。
	requireNoInternetNameCollision(t, internets, name)

	var internetID, switchID types.ID
	defer func() {
		if err := cleanupInternet(t, h, internets, name, internetID, switchID); err != nil {
			t.Errorf("Internet cleanup failed: %v", err)
		}
	}()

	// ルータ＋スイッチを作成します。作成直後の404を最大30分リトライする SDK の待機を
	// 許容して35分の期限を設けます
	data := h.CallWithTimeout(t, ipv6NetCreateTimeout, "ルータ＋スイッチを作成する",
		"iaas-api", "internet", "create",
		"--zone", ipv6NetZone, "--name", name, "--description", ipv6NetDesc,
		"--network-mask-len", "28", "--band-width-mbps", "100",
	)
	router, err := parseInternetItem(data)
	require.NoError(t, err, "decode created Internet")
	internetID, switchID = router.ID, router.Switch.ID
	require.NoError(t, checkInternetIdentity(router, internetID, name, switchID),
		"the create response should contain the created Internet")

	// IPv6 アドレス割り当てを有効化します
	enabled, err := enableIPv6(t, h, internetID)
	require.NoError(t, err, "enable-ipv6 should succeed")
	require.NoError(t, checkIPv6NetItem(enabled, internetID),
		"enable-ipv6 should return a valid IPv6 network")

	// ipv6net find で有効化した IPv6 ネットワークを見つけられることを確認します
	nets := findIPv6Nets(t, h)
	found := findIPv6NetByID(nets, enabled.ID)
	require.NotNil(t, found, "ipv6net find should return the enabled IPv6 network (ID %s)", enabled.ID)
	require.Equal(t, enabled.IPv6Prefix, found.IPv6Prefix,
		"ipv6net find should return the same prefix as enable-ipv6")

	// ipv6net read で参照内容が有効化した内容と一致することを確認します
	read := readIPv6Net(t, h, enabled.ID)
	require.Equal(t, enabled, read, "ipv6net read should return the enabled IPv6 network")
}
