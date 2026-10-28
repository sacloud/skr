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

	"github.com/sacloud/skr/test/e2e/internal/helper"
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

// TestScenarioIAMScim は user provisioning configuration を作成、読み込み、更新、一覧表示、
// 秘密トークンの再生成、削除します。
func TestScenarioIAMScim(t *testing.T) {
	h := helper.New(t, "iam-api-scim",
		"テスト用の SCIM 設定のライフサイクルが正しく動作することを確認する")

	name := resourceNamePrefix + helper.RandomSuffix(t)
	run := &iamRun{
		scim: iamResource{kind: "scim", name: name + "-scim"},
	}

	// SCIM 設定のライフサイクルを確認する
	runScimScenario(t, h, run)
}

// runScimScenario は user provisioning configuration を作成、読み込み、更新、一覧表示、
// 秘密トークンの再生成、削除を行います。
// configuration は organization 全体のものであるため、シナリオ内で削除されます。
// 後片付けは前のステップが失敗した場合のみ削除します。
func runScimScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.scim.attempted = true
	data := h.Call(t, "テスト用のSCIM設定を作成する", "iam-api", "scim", "create",
		"--name", run.scim.name, "--query", "{id: .id, name: .name}")
	id, name, err := decodeNamedResource("create-scim", data)
	require.NoError(t, err, "create-scim: 応答のデコードに失敗しました")
	require.Equal(t, run.scim.name, name, "create-scim: 作成後の名前がリクエストと一致すること")
	run.scim.id = id

	data = h.Call(t, "SCIM設定を読み込む", "iam-api", "scim", "read", run.scim.id,
		"--query", "{id: .id, name: .name, base_url: .base_url}")
	read, err := decodeScimConfiguration("read-scim", data)
	require.NoError(t, err, "read-scim: 応答のデコードに失敗しました")
	require.Equal(t, run.scim.id, read.ID, "read-scim: 読み込んだ設定のIDが一致すること")
	require.Equal(t, run.scim.name, read.Name, "read-scim: 読み込んだ設定の名前が一致すること")
	require.NotEmpty(t, read.BaseURL, "read-scim: base_urlが存在すること")

	renamed := run.scim.name + "-renamed"
	run.scim.renamedTo = renamed
	data = h.Call(t, "SCIM設定の名前を更新する", "iam-api", "scim", "update", run.scim.id,
		"--name", renamed, "--query", "{id: .id, name: .name}")
	_, name, err = decodeNamedResource("update-scim", data)
	require.NoError(t, err, "update-scim: 応答のデコードに失敗しました")
	require.Equal(t, renamed, name, "update-scim: 更新後の名前がリクエストと一致すること")
	run.scim.name = renamed
	run.scim.renamedTo = ""

	data = h.Call(t, "SCIM設定の一覧を確認する", "iam-api", "scim", "list", "--query", managedListQuery)
	found, err := checkManagedListed("list-scim", data, run.scim.id, run.scim.name)
	require.NoError(t, err, "list-scim: 一覧のデコードに失敗しました")
	require.True(t, found, "list-scim: 設定 %sが一覧に存在すること", run.scim.id)

	// 応答には秘密トークンが含まれるため、
	// query はその長さのみを投影し、実行記録にはトークン自体が保存されない。
	data = h.Call(t, "SCIMトークンを再発行する", "iam-api", "scim", "regenerate-token", run.scim.id,
		"--query", "{token_length: (.secret_token | length)}")
	var token secretTokenLength
	require.NoError(t, json.Unmarshal(data, &token), "regenerate-scim-token: トークン長のデコードに失敗しました")
	require.Positive(t, token.TokenLength, "regenerate-scim-token: トークン長が正の値であること")

	h.Call(t, "SCIM設定を削除する", "iam-api", "scim", "delete", run.scim.id)
	run.scim.deleted = true
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
