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

// このファイルは iam-api E2E のポリシーバインディングシナリオです。
// テスト用の folder と project にロールを付与し、元の binding に戻します。
package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	folderPolicyRole   = "folder-admin"
	projectPolicyRole  = "resource-creator"
	policyPrincipalTyp = "service-principal"
)

// runPolicyBindingScenario は、この実行で作成した folder と project の IAM ポリシー binding を更新し、
// 元の binding に戻します。
// 更新は binding リスト全体を置き換えるため、
// 更新前に元の binding を取得し、後で復元します。
func runPolicyBindingScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	updateFolderPolicy(t, h, run)
	updateProjectPolicy(t, h, run)
}

// updateFolderPolicy はこの実行の principal に folder ロールを付与し、
// binding を確認してから元の binding に戻します。
func updateFolderPolicy(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	original := readPolicyBindings(t, h, "folderのロール付与前のbindingを読み込む", "policy", "read-folder", run.folder.id)
	updated := append(append([]policyBinding{}, original...), newPolicyBinding(run, folderPolicyRole))
	updatePolicyBindings(t, h, "folderのbindingを更新してロールを付与する", "policy", "update-folder", run.folder.id, updated)
	checkPolicyBindings(t, h, "更新後のfolderのbindingを確認する", "policy", "read-folder", run.folder.id, updated)
	updatePolicyBindings(t, h, "folderのbindingを元に戻す", "policy", "update-folder", run.folder.id, original)
	checkPolicyBindings(t, h, "復元後のfolderのbindingを確認する", "policy", "read-folder", run.folder.id, original)
}

// updateProjectPolicy は元の binding に加えて、この実行の principal に project ロールを付与し、
// 両方を確認してから元の binding に戻します。
// 新しく作成された project は作成した principal への owner binding から始まるため、
// 取得した binding のみを復元すれば、
// この実行自身のアクセスは維持されます。
func updateProjectPolicy(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	original := readPolicyBindings(t, h, "projectのロール付与前のbindingを読み込む", "policy", "read-project", run.project.id)
	updated := append(append([]policyBinding{}, original...), newPolicyBinding(run, projectPolicyRole))
	updatePolicyBindings(t, h, "projectのbindingを更新してロールを付与する", "policy", "update-project", run.project.id, updated)
	checkPolicyBindings(t, h, "更新後のprojectのbindingを確認する", "policy", "read-project", run.project.id, updated)
	updatePolicyBindings(t, h, "projectのbindingを元に戻す", "policy", "update-project", run.project.id, original)
	checkPolicyBindings(t, h, "復元後のprojectのbindingを確認する", "policy", "read-project", run.project.id, original)
}

func newPolicyBinding(run *iamRun, roleID string) policyBinding {
	return policyBinding{
		Role:       policyRole{Type: "preset", ID: roleID},
		Principals: []policyPrincipal{{Type: policyPrincipalTyp, ID: run.principalID}},
	}
}

// updatePolicyBindings はリソースの IAM ポリシー binding を
// 指定した JSON 配列のリクエストで置き換えます。
// organization レベルの操作では resourceID を指定せず、
// コマンド引数に空の位置引数を渡しません。
func updatePolicyBindings(t *testing.T, h *helper.Helper, description, kind, subcommand, resourceID string, bindings []policyBinding) {
	t.Helper()
	request := mustSettingsJSON(bindings)
	args := []string{"iam-api", kind, subcommand}
	if resourceID != "" {
		args = append(args, resourceID)
	}
	h.Call(t, description, append(args, "--request", request)...)
}

// checkPolicyBindings は binding を読み込み、
// 期待されるリストと比較します。
func checkPolicyBindings(t *testing.T, h *helper.Helper, description, kind, subcommand, resourceID string, expected []policyBinding) {
	t.Helper()
	actual := readPolicyBindings(t, h, description, kind, subcommand, resourceID)
	require.Equal(t, expected, actual, "%s: bindingが期待値と一致すること", description)
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
