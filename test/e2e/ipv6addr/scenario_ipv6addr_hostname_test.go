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
	"net/netip"
	"strings"
	"testing"

	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipv6AddrNamePrefix はテスト用ルータ＋スイッチの名前の接頭辞です。ランダムな接尾辞とともに
// 使い、過去の実行のリソースと衝突しないようにします。
const ipv6AddrNamePrefix = "skr-e2e-ipv6addr-"

// ipv6AddrInternetDesc はテスト用ルータ＋スイッチに付ける固定の説明です。後片て付けの識別にも使います。
const ipv6AddrInternetDesc = "Temporary Internet for skr ipv6addr E2E"

// TestScenarioIPv6AddrHostName は ipv6addr コマンドの変更系ライブ E2E です。ルータ＋スイッチを
// 作成して IPv6 アドレス割り当てを有効化し、有効化したプレフィクス内のアドレスに対して create で
// 逆引きホスト名を登録できること、update で変更できること、delete で解除できることを確認します。
// テスト対象の操作は skr CLI で実行し、事前確認と後片て付けの確認は SDK で静かに実行します。
func TestScenarioIPv6AddrHostName(t *testing.T) {
	h := helper.New(t, "ipv6addr-api", "ipv6addr create、update、delete が、有効化済み IPv6 ネットワークのアドレスの逆引きホスト名を正しく登録・変更・解除できることを確認する")

	// 事前確認と後片て付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	internets := internetSDK.New(client)

	name := ipv6AddrNamePrefix + helper.RandomSuffix(t)

	// 作成前に同名リソースが存在しないことを SDK で確認する。確認の実行は証跡に残らず、
	// 既存リソースを変更しないまま終了する。
	helper.RequireNoInternetNameCollision(t, internets, helper.InternetZone, name)

	var created helper.InternetItem
	defer func() {
		if err := helper.CleanupInternet(t, h, internets, helper.InternetZone, name, ipv6AddrInternetDesc, created.ID, created.Switch.ID); err != nil {
			t.Errorf("Internet cleanup failed: %v", err)
		}
	}()

	// ルータ＋スイッチを作成して IPv6 アドレス割り当てを有効化します
	created = helper.CreateInternet(t, h, helper.InternetZone, name, ipv6AddrInternetDesc)
	net, err := helper.EnableIPv6(t, h, helper.InternetZone, created.ID)
	require.NoError(t, err, "enable-ipv6 should succeed")
	require.NoError(t, helper.CheckIPv6NetItem(net, created.ID),
		"enable-ipv6 should return a valid IPv6 network")

	// 逆引きホスト名を登録するアドレスは、使用可能な範囲 (末尾64bitが 0:0:0:4 以降) から選びます
	require.True(t, strings.HasSuffix(net.IPv6Prefix, "::"),
		"the enabled IPv6 prefix should end with :: (got %q)", net.IPv6Prefix)
	addr := net.IPv6Prefix + "10"

	// 逆引き登録には、対象のアドレスへの正引き (AAAA レコード) が必要です
	// (https://manual.sakura.ad.jp/cloud/network/switch/reverse-dns.html)。
	// テストのアドレスは実行ごとに変わるため、ホスト名に含まれるアドレスを応答する
	// ワイルドカード DNS (sslip.io) で正引きを表現します。圧縮形とフル形の 2 種類の
	// 表記が同じアドレスに解決されることを利用して、変更後のホスト名も用意します。
	parsed, err := netip.ParseAddr(addr)
	require.NoError(t, err, "parse enabled IPv6 address")
	hostName1 := strings.ReplaceAll(addr, ":", "-") + ".sslip.io"
	hostName2 := strings.ReplaceAll(parsed.StringExpanded(), ":", "-") + ".sslip.io"

	// ipv6addr create で逆引きホスト名を登録します
	createdAddr := createIPv6Addr(t, h, addr, hostName1)
	require.Equal(t, addr, createdAddr.IPv6Addr,
		"ipv6addr create should return the registered address")
	require.Equal(t, hostName1, createdAddr.HostName,
		"ipv6addr create should return the registered host name")

	// ipv6addr read で登録内容を確認します
	require.Equal(t, ipv6AddrItem{IPv6Addr: addr, HostName: hostName1}, readIPv6Addr(t, h, addr),
		"ipv6addr read should return the registered address and host name")

	// ipv6addr update で逆引きホスト名を変更します
	updatedAddr := updateIPv6Addr(t, h, addr, hostName2)
	require.Equal(t, hostName2, updatedAddr.HostName,
		"ipv6addr update should return the updated host name")

	// ipv6addr read で変更後のホスト名を確認します
	require.Equal(t, ipv6AddrItem{IPv6Addr: addr, HostName: hostName2}, readIPv6Addr(t, h, addr),
		"ipv6addr read should return the updated host name")

	// ipv6addr delete で逆引きホスト名の登録を解除します
	deleteIPv6Addr(t, h, addr)

	// 解除後の参照は、不在としてエラーになることを確認します
	_, stderr, err := h.CallAllowingFailure(t, "解除した IPv6 アドレスを参照する",
		"iaas-api", "ipv6addr", "read",
		"--zone", ipv6AddrZone, "--ipv6-addr", addr,
	)
	require.Error(t, err, "reading a released IPv6 address should fail (stderr: %s)", stderr)
}
