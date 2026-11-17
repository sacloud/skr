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

package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

// requestCommandPaths は --request フラグを持つ末端コマンドの kong パスを列挙します。
// 生成コマンドと手書きコマンドの両方が対象です (ADR 0085)。
func requestCommandPaths(t *testing.T) []string {
	t.Helper()

	parser, err := kong.New(newCLI(), kong.Vars{"version": "test"})
	require.NoError(t, err)

	var paths []string
	var walk func(node *kong.Node)
	walk = func(node *kong.Node) {
		hasRequest := false
		for _, flag := range node.Flags {
			if flag.Name == "request" {
				hasRequest = true
			}
		}
		if hasRequest {
			paths = append(paths, node.Path())
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(parser.Model.Node)
	require.NotEmpty(t, paths, "--request を持つコマンドが 1 つも見つかりませんでした")
	return paths
}

// TestRequestCommandCoverageBaseline は --request を受け付ける全末端コマンドの一覧を
// baseline ファイルと照合します。--request を追加するコマンドを新設したときは
// baseline を更新した上で、渡したリクエスト JSON の解釈と送信内容を検証する単体テストを
// 用意してください (docs/design/api-command-generation.md)。
func TestRequestCommandCoverageBaseline(t *testing.T) {
	paths := requestCommandPaths(t)
	want, err := os.ReadFile(repositoryPath("internal/cli/testdata/request-command-baseline.txt"))
	require.NoError(t, err)
	wantPaths := strings.Split(strings.TrimSpace(string(want)), "\n")
	require.Equal(t, wantPaths, paths,
		"--request を持つコマンドの一覧が baseline と一致しません。コマンドを追加・変更した場合は internal/cli/testdata/request-command-baseline.txt を更新し、リクエスト JSON の解釈を検証する単体テストを追加してください")
}
