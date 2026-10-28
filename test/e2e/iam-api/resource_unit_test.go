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

// このファイルは iam-api E2E の共有ロジック (listQuery、paginationDone、
// decodePage、decodeItem、checkIdentity、randomPassword) の単体テストです。
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListQueryCompiles(t *testing.T) {
	tests := []struct {
		kind   string
		target []string
		want   string
	}{
		{
			kind: "user",
			want: `{items: (map({id,name,code,description})), page_count: length}`,
		},
		{
			kind:   "user",
			target: []string{"skr-e2e-iam-abc"},
			want:   `{items: (map(select(.name == "skr-e2e-iam-abc") | {id,name,code,description})), page_count: length}`,
		},
		{
			kind: "group",
			want: `{items: (map({id,name,code,description})), page_count: length}`,
		},
		{
			kind: "folder",
			want: `{items: (.items | map({id,name,code,description,project_id})), page_count: (.items | length), has_next: (.next != null)}`,
		},
		{
			kind: "project",
			want: `{items: (.items | map({id,name,code,description,project_id})), page_count: (.items | length), has_next: (.next != null)}`,
		},
		{
			kind:   "project",
			target: []string{"skr-e2e-iam-abc"},
			want:   `{items: (.items | map(select(.name == "skr-e2e-iam-abc") | {id,name,code,description,project_id})), page_count: (.items | length), has_next: (.next != null)}`,
		},
		{
			kind: "service-principal",
			want: `{items: (.items | map({id,name,code,description,project_id})), page_count: (.items | length), has_next: (.next != null)}`,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/%v", test.kind, test.target), func(t *testing.T) {
			require.Equal(t, test.want, listQuery(test.kind, test.target...))
		})
	}
}

func TestPaginationDone(t *testing.T) {
	next := true
	ended := false
	tests := []struct {
		page projectedPage
		kind string
		want bool
	}{
		{page: projectedPage{}, kind: "user", want: true},
		{page: projectedPage{PageCount: 1}, kind: "user", want: true},
		{page: projectedPage{PageCount: userGroupPageSize}, kind: "user", want: false},
		{page: projectedPage{PageCount: 3}, kind: "group", want: true},
		{page: projectedPage{HasNext: &ended}, kind: "folder", want: true},
		{page: projectedPage{PageCount: 5, HasNext: &next}, kind: "folder", want: false},
		{page: projectedPage{HasNext: &next}, kind: "project", want: false},
		{page: projectedPage{PageCount: 2}, kind: "project", want: true},
		{page: projectedPage{PageCount: 2}, kind: "service-principal", want: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/%d/%v", test.kind, test.page.PageCount, test.page.HasNext), func(t *testing.T) {
			require.Equal(t, test.want, paginationDone(test.page, test.kind))
		})
	}
}

func TestDecodePage(t *testing.T) {
	page, err := decodePage([]byte(`{"items": [{"id": 1, "name": "n"}], "page_count": 1}`), "label")
	require.NoError(t, err, "items と page_count を含む応答をデコードできること")
	require.Equal(t, []iamItem{{ID: 1, Name: "n"}}, page.Items)
	require.Equal(t, 1, page.PageCount)

	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "配列ではないitems", data: `{"items": null, "page_count": 0}`, want: "projected items must be a JSON array"},
		{name: "負のpage_count", data: `{"items": [], "page_count": -1}`, want: "page count is negative"},
		{name: "JSONではない", data: `not-json`, want: "decode projected list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodePage([]byte(test.data), "label")
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestDecodeItem(t *testing.T) {
	item, err := decodeItem([]byte(`{"id": 42, "name": "n"}`), "label")
	require.NoError(t, err, "ID を含む応答をデコードできること")
	require.Equal(t, iamItem{ID: 42, Name: "n"}, item)
	require.Equal(t, "42", item.id())

	_, err = decodeItem([]byte(`{"name": "n"}`), "label")
	require.ErrorContains(t, err, "resource ID is missing")
}

func TestCheckIdentity(t *testing.T) {
	target := iamResource{kind: "user", id: "42", name: "skr-e2e-iam-abc", code: "e2e_abc", description: resourceDescription + "abc"}
	require.NoError(t, checkIdentity(iamItem{ID: 42, Name: "skr-e2e-iam-abc", Code: "e2e_abc", Description: resourceDescription + "abc"}, target),
		"識別情報が一致する応答を受け入れること")
	// renamedTo は名前変更ステップの実行中だけ target に設定されます。
	renamedTarget := target
	renamedTarget.renamedTo = "skr-e2e-iam-abc-renamed"
	require.NoError(t, checkIdentity(iamItem{ID: 42, Name: "skr-e2e-iam-abc-renamed", Code: "e2e_abc", Description: resourceDescription + "abc"}, renamedTarget),
		"名前変更ステップの実行中に改名された名前を受け入れること")

	tests := []struct {
		name     string
		c2       iamItem
		expected string
	}{
		{name: "IDが違う", c2: iamItem{ID: 43, Name: "skr-e2e-iam-abc", Code: "e2e_abc", Description: resourceDescription + "abc"}, expected: "ID mismatch"},
		{name: "名前が違う", c2: iamItem{ID: 42, Name: "skr-e2e-iam-other", Code: "e2e_abc", Description: resourceDescription + "abc"}, expected: "name mismatch"},
		{name: "説明が違う", c2: iamItem{ID: 42, Name: "skr-e2e-iam-abc", Code: "e2e_abc", Description: "skr-e2e-iam/other"}, expected: "description mismatch"},
		{name: "説明が接頭辞を持たない", c2: iamItem{ID: 42, Name: "skr-e2e-iam-abc", Code: "e2e_abc", Description: "other"}, expected: "description mismatch"},
		{name: "codeが違う", c2: iamItem{ID: 42, Name: "skr-e2e-iam-abc", Code: "other", Description: resourceDescription + "abc"}, expected: "code mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkIdentity(test.c2, target)
			require.ErrorContains(t, err, test.expected)
		})
	}
}

func TestCheckIdentityDescriptionPrefix(t *testing.T) {
	target := iamResource{kind: "user", id: "42", name: "skr-e2e-iam-abc", description: resourceDescription + "abc"}
	require.NoError(t, checkIdentity(iamItem{ID: 42, Name: "skr-e2e-iam-abc", Description: resourceDescription + "abc"}, target),
		"code を持たないリソースは code を照合しないこと")

	projectTarget := iamResource{kind: "project", id: "42", name: "skr-e2e-iam-abc", description: resourceDescription + "abc", projectID: 113702071936}
	require.NoError(t, checkIdentity(iamItem{ID: 42, Name: "skr-e2e-iam-abc", Description: resourceDescription + "abc", ProjectID: 113702071936}, projectTarget),
		"projectID を持つリソースは project ID を照合すること")
	err := checkIdentity(iamItem{ID: 42, Name: "skr-e2e-iam-abc", Description: resourceDescription + "abc", ProjectID: 999}, projectTarget)
	require.ErrorContains(t, err, "project ID mismatch")
}

func TestRandomPassword(t *testing.T) {
	value := randomPassword(t)
	require.Len(t, value, 24, "パスワードは24文字であること")
	require.True(t, strings.ContainsAny(value[:1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"), "パスワードの先頭は文字であること")
	require.True(t, strings.ContainsAny(value[2:3], "0123456789"), "パスワードの3文字目は数字であること")
	for _, r := range value {
		require.True(t, strings.ContainsRune(passwordChars, r), "パスワードは許可された文字だけで構成されること")
	}
}

// TestPolicyRequestJSON は binding の JSON が元の値と一致することを確認します。
func TestPolicyRequestJSON(t *testing.T) {
	bound := []policyBinding{{
		Role:       policyRole{Type: "preset", ID: folderPolicyRole},
		Principals: []policyPrincipal{{Type: policyPrincipalTyp, ID: 113702071936}},
	}}
	var decoded []policyBinding
	require.NoError(t, json.Unmarshal([]byte(mustSettingsJSON(bound)), &decoded), "bindingのJSONをデコードできること")
	require.Equal(t, bound, decoded, "bindingのJSONが元の値と一致すること")
}

func TestRandomPasswordUnique(t *testing.T) {
	first := randomPassword(t)
	second := randomPassword(t)
	require.NotEqual(t, first, second, "生成したパスワードが毎回異なること")
}
