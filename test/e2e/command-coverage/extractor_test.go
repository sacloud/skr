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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLeafPathOf(t *testing.T) {
	leaves := []string{
		"version",
		"config show",
		"iaas-api zone find",
		"iaas-api switch create",
	}

	tests := []struct {
		name    string
		command string
		want    string
		wantOK  bool
	}{
		{
			name:    "末端コマンドのみの行",
			command: "skr version",
			want:    "version",
			wantOK:  true,
		},
		{
			name:    "フラグを含む行",
			command: "skr iaas-api zone find --zone is1b --name example",
			want:    "iaas-api zone find",
			wantOK:  true,
		},
		{
			name:    "位置引数を含む行",
			command: "skr config show default",
			want:    "config show",
			wantOK:  true,
		},
		{
			name:    "トークン境界で一致しない行",
			command: "skr iaas-api zone find-all --zone is1b",
			wantOK:  false,
		},
		{
			name:    "skr で始まらない行",
			command: "skr-cli version",
			wantOK:  false,
		},
		{
			name:    "対応する末端コマンドがない行",
			command: "skr unknown",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LeafPathOf(tt.command, leaves)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestLeafPathOfChoosesLongestMatch(t *testing.T) {
	leaves := []string{
		"iaas-api server create",
		"iaas-api server create-from-disk",
	}

	got, ok := LeafPathOf("skr iaas-api server create-from-disk --disk 1", leaves)
	require.True(t, ok)
	require.Equal(t, "iaas-api server create-from-disk", got)

	got, ok = LeafPathOf("skr iaas-api server create --name example", leaves)
	require.True(t, ok)
	require.Equal(t, "iaas-api server create", got)
}

func TestEvidenceCommands(t *testing.T) {
	leaves := []string{
		"version",
		"iaas-api switch create",
		"iaas-api switch delete",
	}
	root := t.TempDir()
	dir := filepath.Join(root, "switch-api", "e2e-output")
	require.NoError(t, os.MkdirAll(dir, 0750))
	report := "# switch-api\n" +
		"IaaS スイッチのライフサイクルを確認する\n" +
		"\n**total duration**:\n3s\n" +
		"\n## スイッチを作成する\n\n**command**:\n" +
		"```\nskr iaas-api switch create --zone is1b --name skr-e2e-switch-1\n```\n" +
		"\n**stdout**:\n```\n{\"ID\":1}\n```\n" +
		"\n## スイッチを削除する\n\n**command**:\n" +
		"```\nskr iaas-api switch delete --zone is1b 1\n```\n" +
		"\n## SSH 接続を確認する\n\n**command**:\n" +
		"```\nssh -i material user@example.com\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "switch-api-20261028000000.md"), []byte(report), 0600))

	evidence, err := EvidenceCommands(root, leaves)
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{
		"iaas-api switch create": {},
		"iaas-api switch delete": {},
	}, evidence.Commands)
	require.Empty(t, evidence.Unknown)
}

func TestEvidenceCommandsReportsUnknownLines(t *testing.T) {
	leaves := []string{"version"}
	root := t.TempDir()
	dir := filepath.Join(root, "version", "e2e-output")
	require.NoError(t, os.MkdirAll(dir, 0750))
	report := "# version\n\n## version を表示する\n\n**command**:\n```\nskr unknown\n```\n\n## version\n\n**command**:\n```\nskr version\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version-20261028000000.md"), []byte(report), 0600))

	evidence, err := EvidenceCommands(root, leaves)
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{"version": {}}, evidence.Commands)
	require.Equal(t, []string{"skr unknown"}, evidence.Unknown)
}

func TestParseBaseline(t *testing.T) {
	data := []byte("" +
		"# コメント行\n" +
		"\n" +
		"version live\n" +
		"iaas-api server create live\n" +
		"iam-api sso link deferred #144 全組織ユーザーのログイン方法に影響するため\n" +
		"iam-api group update todo 旧 runner が未整備。ADR 0062 への移行時に対応\n" +
		"iaas-api server update implemented\n")
	entries, err := ParseBaseline(data)
	require.NoError(t, err)
	require.Len(t, entries, 5)

	require.Equal(t, Entry{
		Path:   "iam-api sso link",
		Status: StatusDeferred,
		Issue:  "#144",
		Note:   "全組織ユーザーのログイン方法に影響するため",
	}, entries["iam-api sso link"])
	require.Equal(t, Entry{
		Path:   "iaas-api server update",
		Status: StatusImplemented,
	}, entries["iaas-api server update"])
	require.Equal(t, Entry{
		Path:   "version",
		Status: StatusLive,
	}, entries["version"])
}

func TestParseBaselineErrors(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		errMsg string
	}{
		{
			name:   "トークンが足りない行",
			data:   "version\n",
			errMsg: "baseline line 1",
		},
		{
			name:   "パスのない行",
			data:   "live\n",
			errMsg: "baseline line 1",
		},
		{
			name:   "ステータスとして認識できない行",
			data:   "version unknown\n",
			errMsg: "baseline line 1",
		},
		{
			name:   "deferred に追跡 Issue の参照がない",
			data:   "version deferred 延期する\n",
			errMsg: "追跡 Issue の参照が必要",
		},
		{
			name:   "同じパスが重複する",
			data:   "version live\nversion live\n",
			errMsg: "duplicate path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBaseline([]byte(tt.data))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errMsg)
		})
	}
}
