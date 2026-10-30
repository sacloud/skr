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

// ipAddressZone はグローバル IP アドレスを扱う E2E で使用するゾーンです。
const ipAddressZone = "is1b"

// TestScenarioIPAddress は ipaddress コマンドの読み取り系ライブ E2E です。
// list でゾーン内のグローバル IP アドレス一覧を取得できること、割り当て済みのアドレスが
// あれば read で参照内容を確認できることを検証します。プロジェクトにグローバル IP アドレスが
// 無い場合、空の一覧は正常な不在として扱います。
// update-host-name は割り当て済みのサーバまたはスイッチが必要なため、別途合意して検証します。
// 存在しないアドレスの参照は、文書用のアドレスで確認し、不在が正常に分類されることを検証します。
func TestScenarioIPAddress(t *testing.T) {
	h := helper.New(t, "ipaddress-api", "ipaddress list と read が API に対して正しく動作することを確認する")

	// ipaddress list でゾーン内のグローバル IP アドレスを一覧します
	addrs := listIPAddresses(t, h)

	// 割り当て済みのアドレスがあれば read で参照内容を確認します
	if len(addrs) > 0 {
		read := readIPAddress(t, h, addrs[0].IPAddress)
		require.Equal(t, addrs[0].IPAddress, read.IPAddress,
			"ipaddress read should return the same address as list")
	}

	// 存在しないアドレスの参照は、不在としてエラーになることを確認します。
	// 192.0.2.0/24 は文書用のアドレス範囲で、割り当てられません
	_, stderr, err := h.CallAllowingFailure(t, "存在しないグローバル IP アドレスを参照する",
		"iaas-api", "ipaddress", "read",
		"--zone", ipAddressZone, "--ip-address", "192.0.2.1",
	)
	require.Error(t, err, "reading an unassigned IP address should fail (stderr: %s)", stderr)
}
