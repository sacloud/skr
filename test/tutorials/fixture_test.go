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

package tutorials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadFixture(t *testing.T) {
	dir := t.TempDir()

	t.Run("正常系", func(t *testing.T) {
		path := filepath.Join(dir, "ok.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
			"values": {"ZONE": "is1a"},
			"skip": ["ssh -i key.pem"],
			"stub": {"skr iaas-api zone read --id 1": "skr iaas-api zone read --id \"$ZONE_ID\""},
			"mock": {"zones": ["is1a", "is1b"]}
		}`), 0o600))
		fixture, err := loadFixture(dir, "ok.md")
		require.NoError(t, err)
		require.Equal(t, []string{"is1a", "is1b"}, fixture.Mock.Zones)
		// まだ何も照合していないため、values/skip/stub は未使用として検出される。
		err = fixture.checkUnused()
		require.Error(t, err)
		require.Contains(t, err.Error(), "values: ZONE")
		require.Contains(t, err.Error(), "skip: ssh -i key.pem")
		require.Contains(t, err.Error(), "stub: skr iaas-api zone read --id 1")
	})

	t.Run("ファイルなし", func(t *testing.T) {
		_, err := loadFixture(dir, "missing.md")
		require.Error(t, err)
	})

	t.Run("不正な JSON", func(t *testing.T) {
		path := filepath.Join(dir, "broken.json")
		require.NoError(t, os.WriteFile(path, []byte(`{`), 0o600))
		_, err := loadFixture(dir, "broken.md")
		require.Error(t, err)
	})

	t.Run("空のトークン", func(t *testing.T) {
		path := filepath.Join(dir, "empty-token.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"values": {"": "is1a"}}`), 0o600))
		_, err := loadFixture(dir, "empty-token.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not have empty token")
	})

	t.Run("空の値", func(t *testing.T) {
		path := filepath.Join(dir, "empty-value.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"values": {"ZONE": ""}}`), 0o600))
		_, err := loadFixture(dir, "empty-value.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("空白を含む値は stub で表現する", func(t *testing.T) {
		path := filepath.Join(dir, "ws-token.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"values": {"SERVER NAME": "x"}}`), 0o600))
		_, err := loadFixture(dir, "ws-token.md")
		require.Error(t, err)
	})
}

func TestFixtureSubstituteValues(t *testing.T) {
	fixture := &Fixture{Values: map[string]string{
		"ZONE":         "is1b",
		"QUEUE-NAME":   "eventbus-queue-x1y2",
		"QUEUE-NAME-2": "eventbus-queue-a1b2",
	}}
	require.NoError(t, fixture.validate("test.json"))

	t.Run("境界の一致のみ置換", func(t *testing.T) {
		// クォートの直後は境界のため、"$QUEUE-NAME" は置換対象になる。
		require.Equal(t,
			`skr iaas-api queue create --name "$eventbus-queue-x1y2" --zone is1b`,
			fixture.substituteValues(`skr iaas-api queue create --name "$QUEUE-NAME" --zone ZONE`),
		)
	})

	t.Run("トークンの部分一致は置換しない", func(t *testing.T) {
		// QUEUE-NAME-2 は QUEUE-NAME として置換されず、長い方のトークンで置換される。
		require.Equal(t,
			"skr x --name eventbus-queue-a1b2",
			fixture.substituteValues("skr x --name QUEUE-NAME-2"),
		)
	})

	t.Run("使用済みトークンの記録と未使用検出", func(t *testing.T) {
		f := &Fixture{Values: map[string]string{"A": "1", "B": "2"}}
		require.NoError(t, f.validate("test.json"))
		f.substituteValues("skr x --a A")
		err := f.checkUnused()
		require.Error(t, err)
		require.Contains(t, err.Error(), "values: B")
	})
}

func TestFixtureShouldSkip(t *testing.T) {
	fixture := &Fixture{Skip: []string{"cat big-file.txt"}}
	require.NoError(t, fixture.validate("test.json"))

	t.Run("harness 固定の skip", func(t *testing.T) {
		require.True(t, fixture.shouldSkip("ssh -i key.pem host"))
		require.True(t, fixture.shouldSkip("exit"))
		require.False(t, fixture.shouldSkip("sshkeygen")) // 前綴り一致は語単位
	})

	t.Run("fixture 固有の skip と使用記録", func(t *testing.T) {
		require.True(t, fixture.shouldSkip("cat big-file.txt | head"))
		require.NoError(t, fixture.checkUnused())
	})

	t.Run("skip しないコマンド", func(t *testing.T) {
		require.False(t, fixture.shouldSkip("skr version"))
	})
}

func TestFixtureStub(t *testing.T) {
	fixture := &Fixture{Stub: map[string]string{
		"skr iaas-api zone read --id 1": `skr iaas-api zone read --id "$ZONE_ID"`,
	}}
	require.NoError(t, fixture.validate("test.json"))

	t.Run("正規化して一致", func(t *testing.T) {
		replacement, ok := fixture.stubReplacement("skr iaas-api zone read  --id   1")
		require.True(t, ok)
		require.Equal(t, `skr iaas-api zone read --id "$ZONE_ID"`, replacement)
		require.NoError(t, fixture.checkUnused())
	})

	t.Run("一致しないコマンド", func(t *testing.T) {
		_, ok := fixture.stubReplacement("skr iaas-api zone find")
		require.False(t, ok)
	})

	t.Run("未使用の stub はエラー", func(t *testing.T) {
		f := &Fixture{Stub: map[string]string{"skr x": "skr y"}}
		require.NoError(t, f.validate("test.json"))
		err := f.checkUnused()
		require.Error(t, err)
		require.Contains(t, err.Error(), "stub: skr x")
	})
}

func TestDetectFixtureNameConflicts(t *testing.T) {
	require.NoError(t, detectFixtureNameConflicts([]string{"a/zone.md", "b/server.md"}))
	err := detectFixtureNameConflicts([]string{"a/zone.md", "b/zone.md"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "zone.json conflicts")
}

func TestValidateStubCommand(t *testing.T) {
	t.Run("skr コマンドの kong パース", func(t *testing.T) {
		require.NoError(t, validateStubCommand("skr iaas-api zone find --query 'map(.Name)'"))
		require.Error(t, validateStubCommand("skr not-a-command"))
		require.Error(t, validateStubCommand("skr iaas-api zone read --unknown-flag 1"))
	})

	t.Run("gojq 式のコンパイル", func(t *testing.T) {
		require.NoError(t, validateStubCommand("jq 'map(.Name)' input.json"))
		require.Error(t, validateStubCommand("skr iaas-api zone find --query 'map('"))
	})

	t.Run("空のコマンド", func(t *testing.T) {
		require.Error(t, validateStubCommand(""))
	})
}

func TestShellFields(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "単純な分割",
			command: "skr iaas-api zone find",
			want:    []string{"skr", "iaas-api", "zone", "find"},
		},
		{
			name:    "シングルクォートはトークンに含めない",
			command: "skr iaas-api zone find --query 'map(.Name)'",
			want:    []string{"skr", "iaas-api", "zone", "find", "--query", "map(.Name)"},
		},
		{
			name:    "ダブルクォート内の空白は保持",
			command: `skr http "https://example.com/a b" --method GET`,
			want:    []string{"skr", "http", "https://example.com/a b", "--method", "GET"},
		},
		{
			name:    "エスケープは解釈しない",
			command: `--request "{\"ID\":1}"`,
			want:    []string{"--request", `{\ID\:1}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shellFields(tt.command))
		})
	}
}
