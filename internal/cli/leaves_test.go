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
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLeafCommandPaths は末端サブコマンドの一覧が�順・重複なしで返ることを確認する。
func TestLeafCommandPaths(t *testing.T) {
	paths, err := LeafCommandPaths()
	require.NoError(t, err)
	require.NotEmpty(t, paths, "the CLI should expose leaf commands")
	require.IsIncreasing(t, paths, "leaf paths should be sorted")
	require.Equal(t, len(paths), len(unique(paths)), "leaf paths should not contain duplicates")

	require.Contains(t, paths, "version", "a top-level leaf command should be listed")
	require.Contains(t, paths, "iaas-api server create", "a nested leaf command should be listed")
	require.NotContains(t, paths, "", "empty paths should be dropped")
	require.NotContains(t, paths, "skr version", "paths should not include the root name")
}

func unique(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		if !slices.Contains(result, p) {
			result = append(result, p)
		}
	}
	return result
}
