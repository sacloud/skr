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

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipv6AddrZone は IPv6 アドレスを扱う E2E で使用するゾーンです。
const ipv6AddrZone = "is1b"

// TestScenarioIPv6Addr は ipv6addr コマンドの読み取り系ライブ E2E です。
// find でゾーン内の IPv6 アドレス一覧を取得できること、登録済みのアドレスがあれば read で
// 参照内容を確認できることを検証します。プロジェクトに IPv6 アドレスが無い場合、空の一覧は
// 正常な不在として扱います。
// create、update、delete は有効化済みの IPv6 ネットワークが必要なため、別途合意して検証します。
// 存在しないアドレスの参照は、IPv6 アドレスとして有効な文書用プレフィクスのアドレスで確認し、
// 不在が正常に分類されることを検証します。
func TestScenarioIPv6Addr(t *testing.T) {
	h := helper.New(t, "ipv6addr-api", "ipv6addr find と read が API に対して正しく動作することを確認する")

	// ipv6addr find でゾーン内の IPv6 アドレスを一覧します
	addrs := findIPv6Addrs(t, h)

	// 登録済みのアドレスがあれば read で参照内容を確認します
	if len(addrs) > 0 {
		read := readIPv6Addr(t, h, addrs[0].IPv6Addr)
		require.Equal(t, addrs[0].IPv6Addr, read.IPv6Addr,
			"ipv6addr read should return the same address as find")
	}

	// 存在しないアドレスの参照は、不在としてエラーになることを確認します。
	// 2001:db8::/32 は文書用のプレフィクスで、割り当てられません
	_, stderr, err := h.CallAllowingFailure(t, "存在しない IPv6 アドレスを参照する",
		"iaas-api", "ipv6addr", "read",
		"--zone", ipv6AddrZone, "--ipv6-addr", "2001:db8::1",
	)
	require.Error(t, err, "reading an unassigned IPv6 address should fail (stderr: %s)", stderr)
}
