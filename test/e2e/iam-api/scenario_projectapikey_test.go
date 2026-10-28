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

// このファイルは iam-api E2E の project API key シナリオです。
// テスト用の project 内でキーのライフサイクルを確認します。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const projectAPIKeyRole = "resource-creator"

// TestScenarioIAMProjectAPIKey はテスト用の project 内で API key を作成、読み込み、一覧表示、更新、削除します。
// create と read の応答には access token が含まれるため、
// すべての出力は --query で投影され、
// 実行記録にはトークン自体が保存されることはありません。
func TestScenarioIAMProjectAPIKey(t *testing.T) {
	h := helper.New(t, "iam-api-project-api-key",
		"テスト用のプロジェクト内で API キーのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	run := &iamRun{
		folder:        iamResource{kind: "folder", name: name, description: desc},
		project:       iamResource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		projectAPIKey: iamResource{kind: "project-api-key", name: name + "-pak"},
	}

	// 作成前に同名のリソースが存在しないことを確認する
	requireNoNameCollision(t, h, run)

	// 後片付け: project と folder を再照合つき削除して残っていないことを確認する
	defer func() {
		if err := cleanupIAMResources(t, h, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// 認証コンテキストから principal の project 制限を読み込む
	context := checkAuthContext(t, h)
	if context.LimitedToProjectID != nil {
		run.limitedProjectID = *context.LimitedToProjectID
	}
	// テスト用の folder と project を作成する
	createFolderAndProject(t, h, run)
	// project API key のライフサイクルを確認する
	runProjectAPIKeyScenario(t, h, run)
}

// runProjectAPIKeyScenario は、この実行の project 内で API key を作成、読み込み、一覧表示、更新、削除します。
// 後片付けは前のステップが失敗した場合のみ削除します。
func runProjectAPIKeyScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.projectAPIKey.attempted = true
	// リクエストは秘密情報を含まないため、テスト専用ディレクトリに書き込みます。
	createFile := writeProjectAPIKeyRequest(t, run, "pak-create", run.projectAPIKey.name, true)
	data := h.Call(t, "テスト用のproject API keyを作成する", "iam-api", "project-api-key", "create",
		"--request", "@"+createFile, "--query", "{id: .id, name: .name, iam_roles: .iam_roles}")
	id, name, roles, err := decodeAPIKeyProjection("create-project-api-key", data)
	require.NoError(t, err, "create-project-api-key: 応答のデコードに失敗しました")
	require.Equal(t, run.projectAPIKey.name, name, "create-project-api-key: 作成後の名前がリクエストと一致すること")
	require.NoError(t, checkAPIKeyRoles(roles), "create-project-api-key: ロールが期待値と一致すること")
	run.projectAPIKey.id = id

	data = h.Call(t, "project API keyを読み込む", "iam-api", "project-api-key", "read", run.projectAPIKey.id,
		"--query", "{id: .id, name: .name, iam_roles: .iam_roles}")
	id, name, roles, err = decodeAPIKeyProjection("read-project-api-key", data)
	require.NoError(t, err, "read-project-api-key: 応答のデコードに失敗しました")
	require.Equal(t, run.projectAPIKey.id, id, "read-project-api-key: 読み込んだキーのIDが一致すること")
	require.Equal(t, run.projectAPIKey.name, name, "read-project-api-key: 読み込んだキーの名前が一致すること")
	require.NoError(t, checkAPIKeyRoles(roles), "read-project-api-key: ロールが期待値と一致すること")

	data = h.Call(t, "project API keyの一覧を確認する", "iam-api", "project-api-key", "list", "--query", managedListQuery)
	found, err := checkManagedListed("list-project-api-keys", data, run.projectAPIKey.id, run.projectAPIKey.name)
	require.NoError(t, err, "list-project-api-keys: 一覧のデコードに失敗しました")
	require.True(t, found, "list-project-api-keys: キー %sが一覧に存在すること", run.projectAPIKey.id)

	renamed := run.projectAPIKey.name + "-renamed"
	run.projectAPIKey.renamedTo = renamed
	updateFile := writeProjectAPIKeyRequest(t, run, "pak-update", renamed, false)
	data = h.Call(t, "project API keyの名前を更新する", "iam-api", "project-api-key", "update", run.projectAPIKey.id,
		"--request", "@"+updateFile, "--query", "{id: .id, name: .name}")
	_, name, _, err = decodeAPIKeyProjection("update-project-api-key", data)
	require.NoError(t, err, "update-project-api-key: 応答のデコードに失敗しました")
	require.Equal(t, renamed, name, "update-project-api-key: 更新後の名前がリクエストと一致すること")
	run.projectAPIKey.name = renamed
	run.projectAPIKey.renamedTo = ""

	h.Call(t, "project API keyを削除する", "iam-api", "project-api-key", "delete", run.projectAPIKey.id)
	run.projectAPIKey.deleted = true
}

// writeProjectAPIKeyRequest は作成または更新のリクエスト JSON を書き込みます。
// create リクエストは key をこの実行の project に固定しますが、
// principal が特定の project に限定されている場合は
// 認証コンテキストの project に固定します。
func writeProjectAPIKeyRequest(t *testing.T, run *iamRun, purpose, name string, create bool) string {
	t.Helper()
	request := map[string]any{
		"name":        name,
		"description": run.projectAPIKey.name,
		"iam_roles":   []string{projectAPIKeyRole},
	}
	if create {
		pinned := projectAPIKeyProjectID(run)
		projectID, err := strconv.ParseInt(pinned, 10, 64)
		require.NoError(t, err, "project ID %qを解析できること", pinned)
		request["project_id"] = projectID
	}
	encoded, err := json.Marshal(request)
	require.NoError(t, err, "project API keyの%sリクエストをエンコードできること", purpose)
	path := filepath.Join(t.TempDir(), "skr-e2e-iam-"+purpose+".json")
	require.NoError(t, os.WriteFile(path, encoded, 0o600), "project API keyの%sリクエストファイルの書き込みに失敗しました", purpose)
	return path
}

// projectAPIKeyProjectID は新しい API key を固定する project を返します。
// 特定の project に限定された principal はそこにしか key を作成できないため、
// 認証コンテキストの project がこの実行の project より優先されます。
func projectAPIKeyProjectID(run *iamRun) string {
	if run.limitedProjectID > 0 {
		return strconv.FormatInt(run.limitedProjectID, 10)
	}
	return run.project.id
}

// decodeAPIKeyProjection は投影された {id, name, iam_roles} の出力をデコードします。
func decodeAPIKeyProjection(step string, data []byte) (id string, name string, roles []string, err error) {
	var result struct {
		ID       any      `json:"id"`
		Name     string   `json:"name"`
		IAMRoles []string `json:"iam_roles"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", nil, fmt.Errorf("%s: decode key: %w", step, err)
	}
	switch value := result.ID.(type) {
	case string:
		id = value
	case float64:
		id = strconv.FormatInt(int64(value), 10)
	default:
		return "", "", nil, fmt.Errorf("%s: key ID is missing", step)
	}
	if id == "" {
		return "", "", nil, fmt.Errorf("%s: key ID is missing", step)
	}
	return id, result.Name, result.IAMRoles, nil
}

func checkAPIKeyRoles(roles []string) error {
	if len(roles) != 1 || roles[0] != projectAPIKeyRole {
		return fmt.Errorf("iam_roles = %q, want [%q]", roles, projectAPIKeyRole)
	}
	return nil
}
