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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// versionPattern は "v<semver> <os>/<arch>, build <revision>" に一致する。
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+ \S+/\S+, build \S+$`)

// TestScenarioVersion は version コマンドのライブ E2E です。
// skr version がバージョン、OS、アーキテクチャ、リビジョンを含む期待される形式で出力することを確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioVersion(t *testing.T) {
	h := helper.New(t, "version",
		"skr version コマンドがバージョン、OS、アーキテクチャ、リビジョンを含む期待される形式で出力することを確認する")

	// version コマンドを実行して出力形式を確認する
	output := h.Call(t, "version コマンドを実行する", "version")
	require.Regexp(t, versionPattern, strings.TrimSpace(string(output)),
		"skr version should print \"v<semver> <os>/<arch>, build <revision>\"")
}
