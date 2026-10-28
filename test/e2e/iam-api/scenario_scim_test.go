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

// このファイルは iam-api E2E の SCIM シナリオです。
// user provisioning configuration のライフサイクルを確認します。
package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type scimConfiguration struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
}

type secretTokenLength struct {
	TokenLength int `json:"token_length"`
}

// scimScenario は user provisioning configuration を作成、読み込み、更新、一覧表示、
// 秘密トークンの再生成、削除を行います。
// configuration は organization 全体のものであるため、シナリオ内で削除されます。
// 後片付けは前のステップが失敗した場合のみ削除します。
func (s *iamRun) scimScenario(t *testing.T) {
	t.Helper()
	s.scim.attempted = true
	s.scim.currentName = s.scim.name
	data := s.call(t, "テスト用のSCIM設定を作成する", "iam-api", "scim", "create",
		"--name", s.scim.name, "--query", "{id: .id, name: .name}")
	id, name, err := decodeNamedResource("create-scim", data)
	require.NoError(t, err, "create-scim: 応答のデコードに失敗しました")
	require.Equal(t, s.scim.name, name, "create-scim: 作成後の名前がリクエストと一致すること")
	s.scim.id = id

	data = s.call(t, "SCIM設定を読み込む", "iam-api", "scim", "read", s.scim.id,
		"--query", "{id: .id, name: .name, base_url: .base_url}")
	read, err := decodeScimConfiguration("read-scim", data)
	require.NoError(t, err, "read-scim: 応答のデコードに失敗しました")
	require.Equal(t, s.scim.id, read.ID, "read-scim: 読み込んだ設定のIDが一致すること")
	require.Equal(t, s.scim.currentName, read.Name, "read-scim: 読み込んだ設定の名前が一致すること")
	require.NotEmpty(t, read.BaseURL, "read-scim: base_urlが存在すること")

	renamed := s.scim.name + "-renamed"
	data = s.call(t, "SCIM設定の名前を更新する", "iam-api", "scim", "update", s.scim.id,
		"--name", renamed, "--query", "{id: .id, name: .name}")
	_, name, err = decodeNamedResource("update-scim", data)
	require.NoError(t, err, "update-scim: 応答のデコードに失敗しました")
	require.Equal(t, renamed, name, "update-scim: 更新後の名前がリクエストと一致すること")
	s.scim.currentName = renamed

	data = s.call(t, "SCIM設定の一覧を確認する", "iam-api", "scim", "list", "--query", managedListQuery)
	found, err := checkManagedListed("list-scim", data, s.scim.id, s.scim.currentName)
	require.NoError(t, err, "list-scim: 一覧のデコードに失敗しました")
	require.True(t, found, "list-scim: 設定 %sが一覧に存在すること", s.scim.id)

	// 応答には秘密トークンが含まれるため、
	// query はその長さのみを投影し、実行記録にはトークン自体が保存されない。
	data = s.call(t, "SCIMトークンを再発行する", "iam-api", "scim", "regenerate-token", s.scim.id,
		"--query", "{token_length: (.secret_token | length)}")
	var token secretTokenLength
	require.NoError(t, json.Unmarshal(data, &token), "regenerate-scim-token: トークン長のデコードに失敗しました")
	require.Positive(t, token.TokenLength, "regenerate-scim-token: トークン長が正の値であること")

	s.call(t, "SCIM設定を削除する", "iam-api", "scim", "delete", s.scim.id)
	s.scim.id = ""
}

func decodeScimConfiguration(step string, data []byte) (scimConfiguration, error) {
	var configuration scimConfiguration
	if err := json.Unmarshal(data, &configuration); err != nil {
		return scimConfiguration{}, fmt.Errorf("%s: decode configuration: %w", step, err)
	}
	if configuration.ID == "" {
		return scimConfiguration{}, fmt.Errorf("%s: configuration ID is missing", step)
	}
	return configuration, nil
}
