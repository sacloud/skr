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
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
)

// LeafCommandPaths は CLI 全体の末端サブコマンドのフルパスを返します。
// パスは "iaas-api server create" のようにルート名を含まない形式で、
// 昇順に並べ替えて重複を除いて返します。
// E2E のコマンド網羅テストが、テスト対象の CLI ツリーを独立に列挙するために使います。
func LeafCommandPaths() ([]string, error) {
	parser, err := kong.New(newCLI(), kong.Name("skr"))
	if err != nil {
		return nil, fmt.Errorf("build CLI model: %w", err)
	}
	seen := make(map[string]struct{})
	paths := make([]string, 0, 64)
	for _, node := range parser.Model.Node.Leaves(false) {
		path := strings.TrimSpace(node.Path())
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}
