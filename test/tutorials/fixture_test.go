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

	t.Run("重複したモックサービス", func(t *testing.T) {
		// 重複宣言は同じモックを複数回起動する恐れがあるため拒否します。
		path := filepath.Join(dir, "duplicate-service.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["simplemq", "simplemq"]}}`), 0o600))
		_, err := loadFixture(dir, "duplicate-service.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), `declares "simplemq" more than once`)
	})

	t.Run("未知のモックサービス", func(t *testing.T) {
		path := filepath.Join(dir, "unknown-service.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["dummy"]}}`), 0o600))
		_, err := loadFixture(dir, "unknown-service.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown mock service "dummy"`)
	})

	t.Run("eventbus だけを宣言したモックサービス", func(t *testing.T) {
		// サービスリンクの宛先が実 API になるため、simplemq との同時宣言を要求する。
		path := filepath.Join(dir, "eventbus-only.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["eventbus"]}}`), 0o600))
		_, err := loadFixture(dir, "eventbus-only.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires simplemq")
	})

	t.Run("eventbus を宣言せずイベントを宣言", func(t *testing.T) {
		path := filepath.Join(dir, "event-without-service.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "events": [{"after": "skr x", "source": "s", "type": "t"}]}}`), 0o600))
		_, err := loadFixture(dir, "event-without-service.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires the eventbus mock service")
	})

	t.Run("送信元のないイベント", func(t *testing.T) {
		path := filepath.Join(dir, "event-without-source.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["simplemq", "eventbus"], "events": [{"after": "skr x", "source": "", "type": "t"}]}}`), 0o600))
		_, err := loadFixture(dir, "event-without-source.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty source")
	})

	t.Run("種別のないイベント", func(t *testing.T) {
		path := filepath.Join(dir, "event-without-type.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["simplemq", "eventbus"], "events": [{"after": "skr x", "source": "s", "type": ""}]}}`), 0o600))
		_, err := loadFixture(dir, "event-without-type.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty type")
	})

	t.Run("after が空のイベント", func(t *testing.T) {
		path := filepath.Join(dir, "event-without-after.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"mock": {"zones": ["is1a"], "services": ["simplemq", "eventbus"], "events": [{"after": "  ", "source": "s", "type": "t"}]}}`), 0o600))
		_, err := loadFixture(dir, "event-without-after.md")
		require.Error(t, err)
		require.Contains(t, err.Error(), "blank after")
	})
}

func TestFixtureSubstituteValues(t *testing.T) {
	fixture := &Fixture{Values: map[string]string{
		"ZONE":         "is1b",
		"QUEUE-NAME":   "eventbus-queue-x1y2",
		"QUEUE-NAME-2": "eventbus-queue-a1b2",
	}}
	require.NoError(t, fixture.validate("test.json"))

	substitute := func(command string) string {
		t.Helper()
		replaced, err := fixture.substituteValues(command)
		require.NoError(t, err)
		return replaced
	}

	t.Run("境界の一致のみ置換", func(t *testing.T) {
		// クォートの直後は境界のため、"$QUEUE-NAME" は置換対象になる。
		require.Equal(t,
			`skr iaas-api queue create --name "$eventbus-queue-x1y2" --zone is1b`,
			substitute(`skr iaas-api queue create --name "$QUEUE-NAME" --zone ZONE`),
		)
	})

	t.Run("ハイフンで連結された名前の一部でも置換する", func(t *testing.T) {
		// ハイフンは境界として扱うため、プレースホルダーを名前の接尾辞に
		// 埋め込んだ形式でも置換される。
		require.Equal(t,
			`skr iaas-api switch create --name eventbus-switch-is1b`,
			substitute(`skr iaas-api switch create --name eventbus-switch-ZONE`),
		)
	})

	t.Run("アンダースコアで連結された変数の一部は置換しない", func(t *testing.T) {
		// ZONE_ID は ZONE とは別の変数のため、ZONE としては置換されない。
		require.Equal(t,
			`skr x --id "$ZONE_ID" --zone is1b`,
			substitute(`skr x --id "$ZONE_ID" --zone ZONE`),
		)
	})

	t.Run("トークンの部分一致は置換しない", func(t *testing.T) {
		// QUEUE-NAME-2 は QUEUE-NAME として置換されず、長い方のトークンで置換される。
		require.Equal(t,
			"skr x --name eventbus-queue-a1b2",
			substitute("skr x --name QUEUE-NAME-2"),
		)
	})

	t.Run("値を持たない長いプレースホルダーの一部になるトークンはエラー", func(t *testing.T) {
		// SWITCH-NAME の値だけを定義して TEST-SWITCH-NAME を定義しないと、
		// 意図しない名前でリソースを作るため fail-closed でエラーにする。
		f := &Fixture{Values: map[string]string{"SWITCH-NAME": "sakura-switch-x1y2"}}
		require.NoError(t, f.validate("test.json"))
		_, err := f.substituteValues("skr iaas-api switch create --name TEST-SWITCH-NAME")
		require.Error(t, err)
		require.Contains(t, err.Error(), "SWITCH-NAME")
		require.Contains(t, err.Error(), "TEST-SWITCH-NAME")
	})

	t.Run("jq のフィールド名など値を持たない語はそのまま残る", func(t *testing.T) {
		require.Equal(t,
			`skr x --query '{ID,Name}'`,
			substitute(`skr x --query '{ID,Name}'`),
		)
	})

	t.Run("使用済みトークンの記録と未使用検出", func(t *testing.T) {
		f := &Fixture{Values: map[string]string{"A": "1", "B": "2"}}
		require.NoError(t, f.validate("test.json"))
		_, err := f.substituteValues("skr x --a A")
		require.NoError(t, err)
		err = f.checkUnused()
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
