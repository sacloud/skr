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

package commandcoverage

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/sacloud/skr/internal/cli"
	"github.com/stretchr/testify/require"
)

// TestBaselineCoversAllLeafCommands は、CLI 全体の末端サブコマンドが
// baseline にすべて登録されていること、baseline の登録が実在するコマンドだけ
// であることを確認する。未登録の末端コマンドは E2E の網羅漏れとして検出する。
func TestBaselineCoversAllLeafCommands(t *testing.T) {
	leaves, err := cli.LeafCommandPaths()
	require.NoError(t, err)
	require.NotEmpty(t, leaves)

	data, err := os.ReadFile("baseline.txt")
	require.NoError(t, err)
	entries, err := ParseBaseline(data)
	require.NoError(t, err)

	var missing []string
	for _, leaf := range leaves {
		if _, ok := entries[leaf]; !ok {
			missing = append(missing, leaf)
		}
	}
	require.Empty(t, missing, "末端コマンドが baseline に未登録です。E2E の網羅状況を確認して登録してください: %v", missing)

	var unknown []string
	for path := range entries {
		if !slices.Contains(leaves, path) {
			unknown = append(unknown, path)
		}
	}
	require.Empty(t, unknown, "baseline に実在しない末端コマンドが登録されています: %v", unknown)
}

// TestReportEvidenceCoverage は、実行記録から抽出した末端コマンドの状況を報告する。
// 実行記録に現れたコマンドが baseline で live になっていない場合は失敗とし、
// baseline の更新を促す。ローカルに実行記録がない場合も報告だけ行う。
func TestReportEvidenceCoverage(t *testing.T) {
	data, err := os.ReadFile("baseline.txt")
	require.NoError(t, err)
	entries, err := ParseBaseline(data)
	require.NoError(t, err)

	counts := make(map[string]int)
	for _, entry := range entries {
		counts[entry.Status]++
	}
	t.Logf("末端コマンド: %d (live %d, implemented %d, deferred %d, todo %d)",
		len(entries), counts[StatusLive], counts[StatusImplemented], counts[StatusDeferred], counts[StatusTodo])

	leaves, err := cli.LeafCommandPaths()
	require.NoError(t, err)
	evidence, err := EvidenceCommands("..", leaves)
	require.NoError(t, err)
	t.Logf("実行記録で検出した末端コマンド: %d / %d", len(evidence.Commands), len(entries))
	for _, line := range evidence.Unknown {
		t.Logf("末端コマンドに対応しない記録行 (古い実行記録の可能性): %s", line)
	}

	var stale []string
	for path := range evidence.Commands {
		if entries[path].Status != StatusLive {
			stale = append(stale, fmt.Sprintf("%s (baseline: %s)", path, entries[path].Status))
		}
	}
	require.Empty(t, stale, "実行記録に現れた末端コマンドが baseline で live になっていません。baseline を更新してください: %v", stale)
}
