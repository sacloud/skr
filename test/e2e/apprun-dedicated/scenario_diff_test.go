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

// このファイルは apprun-dedicated コマンドのうち、読み取りのみを行う diff の
// ライブ E2E シナリオです。存在しないアプリケーションに対して、識別子だけの
// 定義を現状とした差分が表示されることを確認します。
package main

import (
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioAppRunDedicatedDiff は diff コマンドのライブ E2E です。
// 存在しないクラスタ・アプリケーションを対象にするため、リソースは作成せず、
// 費用は発生しません。アプリケーションの一覧取得 (読み取り) だけを行います。
func TestScenarioAppRunDedicatedDiff(t *testing.T) {
	h := helper.New(t, "apprun-dedicated",
		"AppRun 専有型の高レベルコマンドのうち diff を実行し、存在しないアプリケーションに対して識別子だけの現状との unified diff が表示されることを確認する (リソースを作成しない)")

	suffix := helper.RandomSuffix(t)
	cluster := "skr-e2e-diff-cluster-" + suffix
	name := "skr-e2e-diff-app-" + suffix
	definition := map[string]any{
		"cluster":     cluster,
		"name":        name,
		"image":       "ghcr.io/sacloud/skr-e2e-apprun-dedicated:diff",
		"cpu":         1000,
		"memory":      1024,
		"scalingMode": "manual",
		"fixedScale":  1,
	}
	path := h.WriteRequestJSONFile(t, "diff-definition", definition)

	stdout, stderr, err := h.CallAllowingFailure(t, "存在しないアプリケーションに対して diff を実行する",
		"apprun-dedicated", "diff", "--app", path)
	require.NoError(t, err, "stderr: %s", stderr)
	text := string(stdout)

	// unified diff のヘッダを確認する。
	require.True(t, strings.HasPrefix(text, "--- 定義\n"), "stdout should start with the local header: %s", text)
	require.Contains(t, text, "+++ 現状\n")

	// 現状が識別子のみの定義 (cluster と name) であるため、ローカルの定義にだけ
	// 存在する項目は削除として表示される。cluster は両側に存在するため文脈行のままである。
	require.Contains(t, text, "-  \"image\": \"ghcr.io/sacloud/skr-e2e-apprun-dedicated:diff\"")
	require.Contains(t, text, "-  \"cpu\": 1000")
	require.NotContains(t, text, `+  "image"`, "現状に存在しないため追加行にはならない")
	require.NotContains(t, text, `+  "cpu"`, "現状に存在しないため追加行にはならない")
	require.Contains(t, text, "   \"cluster\": \""+cluster+"\"", "cluster は両側に存在するため文脈行のままである")
	require.Contains(t, text, "@@")

	// 除外フィールドの案内が標準エラー出力に書かれる。
	require.Contains(t, stderr, "registryPassword と secret=true の環境変数の値、registryPasswordAction は応答で取得できないため比較から除外しています")
}
