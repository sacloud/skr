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

// このファイルは iam-api E2E の共有ヘルパーの単体テストです。
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/require"
)

// TestListQueryCompiles は全種別の一覧クエリが gojq でコンパイルできることを確認します。
func TestListQueryCompiles(t *testing.T) {
	for _, kind := range []string{"user", "group", "folder", "project", "service-principal"} {
		for _, targetNames := range [][]string{nil, {"target"}} {
			query := listQuery(kind, targetNames...)
			parsed, err := gojq.Parse(query)
			require.NoError(t, err, "%s の一覧クエリ %qを解析できること", kind, query)
			_, err = gojq.Compile(parsed)
			require.NoError(t, err, "%s の一覧クエリ %qをコンパイルできること", kind, query)
		}
	}
}

// TestListQueryProjectsName は対象名を指定したクエリが名前で絞り込むことを確認します。
func TestListQueryProjectsName(t *testing.T) {
	query := listQuery("user", "skr-e2e-iam-test00ff")
	parsed, err := gojq.Parse(query)
	require.NoError(t, err, "クエリを解析できること")
	compiled, err := gojq.Compile(parsed)
	require.NoError(t, err, "クエリをコンパイルできること")
	input := []any{
		map[string]any{"id": 1001, "name": "skr-e2e-iam-test00ff", "code": "e2e", "description": "skr-e2e-iam/test00ff"},
		map[string]any{"id": 1002, "name": "unrelated", "code": "other", "description": "skr-e2e-iam/other"},
	}
	iter, ok := compiled.Run(input).Next()
	require.True(t, ok, "クエリが結果を1件返すこと")
	result, ok := iter.(map[string]any)
	require.True(t, ok, "結果がオブジェクトであること: %#v", iter)
	items, ok := result["items"].([]any)
	require.True(t, ok, "items が配列であること: %#v", result)
	require.Len(t, items, 1, "名前が一致する項目だけが残ること")
	first, ok := items[0].(map[string]any)
	require.True(t, ok, "項目がオブジェクトであること: %#v", items[0])
	require.Equal(t, "skr-e2e-iam-test00ff", first["name"], "絞り込まれた項目の名前が一致すること")
}

// TestPaginationDone は一覧の読み込みを打ち切るかどうかの判定を確認します。
func TestPaginationDone(t *testing.T) {
	tests := []struct {
		name string
		page projectedPage
		kind string
		want bool
	}{
		{name: "has next", page: projectedPage{Items: []item{}, PageCount: 1, HasNext: boolPtr(true)}, kind: "project", want: false},
		{name: "no next", page: projectedPage{Items: []item{}, PageCount: 1, HasNext: boolPtr(false)}, kind: "project", want: true},
		{name: "empty user page", page: projectedPage{Items: []item{}, PageCount: 0}, kind: "user", want: true},
		{name: "folder ignores page count", page: projectedPage{Items: []item{}, PageCount: userGroupPageSize}, kind: "folder", want: true},
		{name: "full user page continues", page: projectedPage{Items: []item{}, PageCount: userGroupPageSize}, kind: "user", want: false},
		{name: "short user page stops", page: projectedPage{Items: []item{}, PageCount: 1}, kind: "user", want: true},
		{name: "full group page continues", page: projectedPage{Items: []item{}, PageCount: userGroupPageSize}, kind: "group", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, paginationDone(tc.page, tc.kind), "打ち切り判定が期待値と一致すること")
		})
	}
}

// TestDecodePage は一覧の投影結果のデコードを確認します。
func TestDecodePage(t *testing.T) {
	data, err := json.Marshal(projectedPage{Items: []item{{ID: 1001, Name: "first"}}, PageCount: 1})
	require.NoError(t, err, "投影結果をエンコードできること")
	page, err := decodePage(data, "preflight")
	require.NoError(t, err, "投影結果をデコードできること")
	require.Len(t, page.Items, 1, "デコード後の項目数が一致すること")
	require.Equal(t, "first", page.Items[0].Name, "デコード後の名前が一致すること")

	_, err = decodePage([]byte("not-json"), "preflight")
	require.Error(t, err, "JSON以外の応答はエラーになること")
	_, err = decodePage([]byte(`{"items": null, "page_count": 0}`), "preflight")
	require.Error(t, err, "items が null の応答はエラーになること")
	_, err = decodePage([]byte(`{"items": [], "page_count": -1}`), "preflight")
	require.Error(t, err, "page_count が負の応答はエラーになること")
}

// TestDecodeItem はリソースの応答のデコードを確認します。
func TestDecodeItem(t *testing.T) {
	data, err := json.Marshal(item{ID: 1001, Name: "first", Code: "e2e", Description: "skr-e2e-iam/test00ff"})
	require.NoError(t, err, "リソースをエンコードできること")
	result, err := decodeItem(data, "create-user")
	require.NoError(t, err, "リソースをデコードできること")
	require.Equal(t, "1001", result.id(), "デコード後のIDが一致すること")
	require.Equal(t, "first", result.Name, "デコード後の名前が一致すること")

	_, err = decodeItem([]byte(`{"name": "first"}`), "create-user")
	require.Error(t, err, "IDがない応答はエラーになること")
	_, err = decodeItem([]byte("not-json"), "create-user")
	require.Error(t, err, "JSON以外の応答はエラーになること")
}

// TestMarkedForThisRun は実行の識別判定を確認します。
func TestMarkedForThisRun(t *testing.T) {
	marked := item{ID: 1001, Name: "skr-e2e-iam-test00ff", Description: "skr-e2e-iam/test00ff"}
	require.True(t, marked.markedForThisRun("test00ff"), "実行固有の説明を持つリソースはこの実行のものとして扱われること")
	unmarked := item{ID: 1002, Name: "skr-e2e-iam-test00ff", Description: "unrelated"}
	require.False(t, unmarked.markedForThisRun("test00ff"), "実行固有の説明を持たないリソースはこの実行のものとして扱われないこと")
}

// TestCheckIdentity はリソースの識別情報の照合を確認します。
func TestCheckIdentity(t *testing.T) {
	target := resource{kind: "user", name: "skr-e2e-iam-test00ff", code: "e2e_test00ff", description: "skr-e2e-iam/test00ff", projectID: 4001, id: "1001"}
	matching := item{ID: 1001, Name: "skr-e2e-iam-test00ff", Code: "e2e_test00ff", Description: "skr-e2e-iam/test00ff", ProjectID: 4001}
	require.NoError(t, checkIdentity(matching, target), "一致するリソースはエラーにならないこと")

	renamed := matching
	renamed.Name = "skr-e2e-iam-test00ff-renamed"
	require.Error(t, checkIdentity(renamed, target), "名前が違うリソースはエラーになること")
	require.NoError(t, checkIdentity(renamed, target, "skr-e2e-iam-test00ff-renamed"),
		"allowedNames に含まれる名前は許容されること")

	wrongID := matching
	wrongID.ID = 9999
	require.Error(t, checkIdentity(wrongID, target), "IDが違うリソースはエラーになること")

	wrongDescription := matching
	wrongDescription.Description = "unrelated"
	require.Error(t, checkIdentity(wrongDescription, target), "説明が違うリソースはエラーになること")

	wrongCode := matching
	wrongCode.Code = "other"
	require.Error(t, checkIdentity(wrongCode, target), "コードが違うリソースはエラーになること")

	wrongProject := matching
	wrongProject.ProjectID = 9999
	require.Error(t, checkIdentity(wrongProject, target), "プロジェクトIDが違うリソースはエラーになること")

	noProject := matching
	noProject.ProjectID = 0
	require.Error(t, checkIdentity(noProject, target), "プロジェクトIDがないリソースはエラーになること")
}

// TestRandomPasswordRequirements は生成したパスワードが IAM の要件を満たすことを確認します。
func TestRandomPasswordRequirements(t *testing.T) {
	for range 5 {
		password := randomPassword(t)
		require.Len(t, password, 24, "パスワードが24文字であること")
		require.Regexp(t, `^[A-Za-z0-9]`, password, "パスワードの先頭が文字と数字であること")
		for _, char := range password {
			require.True(t, strings.ContainsRune(passwordChars, char),
				"パスワードの文字 %q が許容文字に含まれること", char)
		}
	}
}

func boolPtr(value bool) *bool {
	return &value
}
