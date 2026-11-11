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
