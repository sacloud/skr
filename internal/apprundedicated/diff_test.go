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

package apprundedicated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
	"github.com/stretchr/testify/require"
)

func TestUnifiedDiff(t *testing.T) {
	t.Run("差分なしは空文字列", func(t *testing.T) {
		text := "a\nb\nc\n"
		require.Empty(t, unifiedDiff(text, text, diffContextLines))
	})

	t.Run("途中の変更", func(t *testing.T) {
		got := unifiedDiff("a\nb\nc\nd\ne\n", "a\nb\nC\nd\ne\n", diffContextLines)
		want := "--- 定義\n+++ 現状\n" +
			"@@ -1,5 +1,5 @@\n" +
			" a\n" +
			" b\n" +
			"-c\n" +
			"+C\n" +
			" d\n" +
			" e\n"
		require.Equal(t, want, got)
	})

	t.Run("文脈より離れた変更は別の hunk になります", func(t *testing.T) {
		local := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
		remote := "1-off\n2\n3\n4\n5\n6\n7\n8\n9\n10-off\n"
		got := unifiedDiff(local, remote, 2)
		require.Contains(t, got, "@@ -1,3 +1,3 @@\n-1\n+1-off\n 2\n 3\n")
		require.Contains(t, got, "@@ -8,3 +8,3 @@\n 8\n 9\n-10\n+10-off\n")
		// 2 つの hunk に分かれるため、hunk ヘッダーが 2 回現れます。
		require.Equal(t, 2, countOccurrences(got, "@@ "))
	})

	// GNU diff -u は、次の変更までの間隔が 2*context 行以下のとき hunk を 1 つに
	// 統合します。間隔がこれより短いときに hunk を分けると、前の hunk の文脈と
	// 次の hunk の文脈が重なり、同じ行が 2 度出力されます。
	t.Run("間隔が 2*context 以下の変更は 1 つの hunk に統合", func(t *testing.T) {
		local := "A\nE1\nE2\nE3\nE4\nE5\nE6\nB\n"
		remote := "X\nE1\nE2\nE3\nE4\nE5\nE6\nY\n"
		got := unifiedDiff(local, remote, 3)
		want := "--- 定義\n+++ 現状\n" +
			"@@ -1,8 +1,8 @@\n" +
			"-A\n" +
			"+X\n" +
			" E1\n E2\n E3\n E4\n E5\n E6\n" +
			"-B\n" +
			"+Y\n"
		require.Equal(t, want, got)
	})

	t.Run("間隔が 2*context を超える変更は文脈を重複させずに分割", func(t *testing.T) {
		local := "A\nE1\nE2\nE3\nE4\nE5\nE6\nE7\nB\n"
		remote := "X\nE1\nE2\nE3\nE4\nE5\nE6\nE7\nY\n"
		got := unifiedDiff(local, remote, 3)
		want := "--- 定義\n+++ 現状\n" +
			"@@ -1,4 +1,4 @@\n" +
			"-A\n" +
			"+X\n" +
			" E1\n E2\n E3\n" +
			"@@ -6,4 +6,4 @@\n" +
			" E5\n E6\n E7\n" +
			"-B\n" +
			"+Y\n"
		require.Equal(t, want, got)
	})

	t.Run("複数の文脈幅で GNU diff と同じ結果", func(t *testing.T) {
		if _, err := exec.LookPath("diff"); err != nil {
			t.Skip("diff コマンドがないためスキップします")
		}
		rng := rand.New(rand.NewSource(1)) // #nosec G404 -- 固定シードのテストデータ生成であり、暗号学的な安全性は不要です。
		labels := make([]string, 30)
		for i := range labels {
			labels[i] = fmt.Sprintf("L%02d", i)
		}
		for i := 0; i < 200; i++ {
			local := subsetText(rng, labels)
			remote := subsetText(rng, labels)
			for _, context := range []int{1, 2, 3, 5} {
				got := strings.TrimPrefix(unifiedDiff(local, remote, context), diffHeader)
				want := gnuDiffOutput(t, local, remote, context)
				require.Equal(t, want, got, "i=%d context=%d\nlocal=%q\nremote=%q", i, context, local, remote)
			}
		}
	})

	t.Run("先頭への追加", func(t *testing.T) {
		// 先頭への追加は、GNU diff -u と同じく追加前の行を含む hunk 範囲になります。
		got := unifiedDiff("b\nc\n", "a\nb\nc\n", diffContextLines)
		want := "--- 定義\n+++ 現状\n" +
			"@@ -1,2 +1,3 @@\n" +
			"+a\n" +
			" b\n" +
			" c\n"
		require.Equal(t, want, got)
	})

	t.Run("空の入力から生成への差分", func(t *testing.T) {
		got := unifiedDiff("", "a\nb\n", diffContextLines)
		require.Contains(t, got, "@@ -0,0 +1,2 @@\n+a\n+b\n")
	})

	t.Run("全行の削除", func(t *testing.T) {
		got := unifiedDiff("a\nb\n", "", diffContextLines)
		require.Contains(t, got, "@@ -1,2 +0,0 @@\n-a\n-b\n")
	})
}

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

// subsetText は labels を順序を保ったままランダムに間引いたテキストを返します。
// 行を一意にすることで共通部分列の選択が一意になり、LCS の実装差による
// 乱的な不一致を避けます。
func subsetText(rng *rand.Rand, labels []string) string {
	var lines []string
	for _, label := range labels {
		if rng.Intn(100) < 70 {
			lines = append(lines, label)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// gnuDiffOutput は GNU diff -U<context> の出力からヘッダーを除いた hunk 部分を返します。
// -u を同時に指定すると GNU diff が context を最低 3 に強制するため、
// -U のみを指定します。差分がないときは空文字列を返します。
func gnuDiffOutput(t *testing.T, local, remote string, context int) string {
	t.Helper()
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local")
	remotePath := filepath.Join(dir, "remote")
	require.NoError(t, os.WriteFile(localPath, []byte(local), 0600))
	require.NoError(t, os.WriteFile(remotePath, []byte(remote), 0600))
	// #nosec G204 -- 引数はテスト内で生成した固定の context 値と一時ファイルのパスです。
	out, err := exec.Command("diff", fmt.Sprintf("-U%d", context),
		"--label", "定義", "--label", "現状", localPath, remotePath).Output()
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 1, exitErr.ExitCode())
	return strings.TrimPrefix(string(out), "--- 定義\n+++ 現状\n")
}

func TestColorizeDiff(t *testing.T) {
	text := "@@ -1,2 +1,2 @@\n a\n-removed\n+added\n"
	got := colorizeDiff(text)
	require.Contains(t, got, colorRed+"-removed"+colorReset)
	require.Contains(t, got, colorGreen+"+added"+colorReset)
	// 文脈行は色を付けません。
	require.Contains(t, got, " a\n")
}

func TestColorEnabledRequiresTerminal(t *testing.T) {
	// bytes.Buffer など標準出力以外の書き込み先では色を付けません。
	var out bytes.Buffer
	require.False(t, colorEnabled(&out))
}

func TestDefinitionText(t *testing.T) {
	m := map[string]any{"cluster": "c1", "name": "app1"}
	got := definitionText(m)
	// JSON オブジェクトのキーは昇順に並び、末尾に改行を付けます。
	require.Equal(t, "{\n  \"cluster\": \"c1\",\n  \"name\": \"app1\"\n}\n", got)
}

func TestMarshalDefinition(t *testing.T) {
	def := &apprundedicatedservice.ApplicationDefinition{Cluster: "c1", Name: "app1"}
	data, err := marshalDefinition(def)
	require.NoError(t, err)
	require.True(t, len(data) > 0)
	require.Equal(t, byte('\n'), data[len(data)-1])
	var roundTrip apprundedicatedservice.ApplicationDefinition
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.Equal(t, "c1", roundTrip.Cluster)
	require.Equal(t, "app1", roundTrip.Name)
}
