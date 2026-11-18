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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractScript(t *testing.T) {
	tests := []struct {
		name    string
		content string
		check   func(t *testing.T, script *tutorialScript)
	}{
		{
			name: "単純なコマンドとインライン期待出力",
			content: "```console\n" +
				"$ skr iaas-api zone find --query 'map(.Name)'\n" +
				"[]\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				require.Len(t, script.Blocks, 1)
				steps := script.Blocks[0].Steps
				require.Len(t, steps, 1)
				require.Equal(t, "skr iaas-api zone find --query 'map(.Name)'", steps[0].Command)
				require.Equal(t, []string{"[]"}, steps[0].Expected)
				require.Equal(t, 2, steps[0].Line)
			},
		},
		{
			name: "行継続を連結する",
			content: "```console\n" +
				"$ skr iaas-api server create --zone is1b \\\n" +
				"    --name \"$SERVER_NAME\" \\\n" +
				"    --query-raw '.ID'\n" +
				"12345\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				steps := script.Blocks[0].Steps
				require.Len(t, steps, 1)
				require.Equal(t, "skr iaas-api server create --zone is1b \\\n--name \"$SERVER_NAME\" \\\n--query-raw '.ID'", steps[0].Command)
				require.Equal(t, []string{"12345"}, steps[0].Expected)
			},
		},
		{
			name: "heredoc をコマンドの一部として保持する",
			content: "```console\n" +
				"$ cat > \"$USER_DATA_PATH\" <<EOF\n" +
				"#cloud-config\n" +
				"users:\n" +
				"  - default\n" +
				"EOF\n" +
				"$ skr version\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				steps := script.Blocks[0].Steps
				require.Len(t, steps, 2)
				require.Equal(t, "cat > \"$USER_DATA_PATH\" <<EOF\n#cloud-config\nusers:\n  - default", steps[0].Command)
				require.Equal(t, "skr version", steps[1].Command)
				require.Empty(t, steps[0].Expected)
			},
		},
		{
			name: "連結後の heredoc とコマンド内の << を区別する",
			content: "```console\n" +
				"$ skr http post https://example.com <<'BODY'\n" +
				"{\"key\": \"value\"}\n" +
				"BODY\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				steps := script.Blocks[0].Steps
				require.Len(t, steps, 1)
				require.Equal(t, "skr http post https://example.com <<'BODY'\n{\"key\": \"value\"}", steps[0].Command)
			},
		},
		{
			name: "JSON コードブロックを直前のブロックの最後の step に対応付ける",
			content: "```console\n" +
				"$ skr iaas-api zone find --query 'map(.Name)'\n" +
				"```\n" +
				"\n" +
				"```json\n" +
				"[\n" +
				"  \"test-zone-a\"\n" +
				"]\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				require.Len(t, script.Blocks, 1)
				example, ok := script.JSONExamples[0]
				require.True(t, ok, "JSON example should be attached to block 0")
				require.Equal(t, 5, example.Line)
				require.Contains(t, example.Text, "test-zone-a")
			},
		},
		{
			name: "複数ブロックと複数コマンド",
			content: "```console\n" +
				"$ skr c0nfig current\n" +
				"$ skr iaas-api zone find\n" +
				"```\n" +
				"text between blocks\n" +
				"```console\n" +
				"$ skr version\n" +
				"```\n",
			check: func(t *testing.T, script *tutorialScript) {
				require.Len(t, script.Blocks, 2)
				require.Len(t, script.Blocks[0].Steps, 2)
				require.Len(t, script.Blocks[1].Steps, 1)
				require.Empty(t, script.JSONExamples)
			},
		},
		{
			name: "コマンドの前に期待出力がある場合はエラー",
			content: "```console\n" +
				"unexpected output\n" +
				"$ skr version\n" +
				"```\n",
			check: func(t *testing.T, _ *tutorialScript) {
				t.Fatal("must not reach here")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := extractScript("test.md", tt.content)
			if tt.name == "コマンドの前に期待出力がある場合はエラー" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "expected output before any command")
				return
			}
			require.NoError(t, err)
			tt.check(t, script)
		})
	}
}

func TestHeredocStart(t *testing.T) {
	tests := []struct {
		command   string
		term      string
		stripTabs bool
		ok        bool
	}{
		{command: "cat > f <<EOF", term: "EOF", ok: true},
		{command: "cat > f <<-EOF", term: "EOF", stripTabs: true, ok: true},
		{command: "cat > f << 'EOF'", term: "EOF", ok: true},
		{command: "skr http post https://example.com <<BODY", term: "BODY", ok: true},
		{command: "echo hi", ok: false},
		{command: "skr iaas-api server read --query '<EOF'", ok: false},
	}
	for _, tt := range tests {
		term, strip, ok := heredocStart(tt.command)
		require.Equal(t, tt.ok, ok, "command: %s", tt.command)
		if tt.ok {
			require.Equal(t, tt.term, term, "command: %s", tt.command)
			require.Equal(t, tt.stripTabs, strip, "command: %s", tt.command)
		}
	}
}
