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

	"github.com/stretchr/testify/require"
)

const (
	folderPolicyRole   = "folder-admin"
	projectPolicyRole  = "resource-creator"
	policyPrincipalTyp = "service-principal"
)

// policyScenario は、この実行で作成した folder と project の IAM ポリシー binding を更新し、
// 元の binding に戻します。
// 更新は binding リスト全体を置き換えるため、
// 更新前に元の binding を取得し、後で復元します。
func (s *iamRun) policyScenario(t *testing.T) {
	t.Helper()
	s.updateFolderPolicy(t)
	s.updateProjectPolicy(t)
}

// updateFolderPolicy はこの実行の principal に folder ロールを付与し、
// binding を確認してから元の binding に戻します。
func (s *iamRun) updateFolderPolicy(t *testing.T) {
	t.Helper()
	original := s.readPolicyBindings(t, "folderのロール付与前のbindingを読み込む", "policy", "read-folder", s.folder.id)
	updated := append(append([]policyBinding{}, original...), s.policyBinding(folderPolicyRole))
	s.updatePolicyBindings(t, "folderのbindingを更新してロールを付与する", "policy", "update-folder", s.folder.id, updated)
	s.checkPolicyBindings(t, "更新後のfolderのbindingを確認する", "policy", "read-folder", s.folder.id, updated)
	s.updatePolicyBindings(t, "folderのbindingを元に戻す", "policy", "update-folder", s.folder.id, original)
	s.checkPolicyBindings(t, "復元後のfolderのbindingを確認する", "policy", "read-folder", s.folder.id, original)
}

// updateProjectPolicy は元の binding に加えて、この実行の principal に project ロールを付与し、
// 両方を確認してから元の binding に戻します。
// 新しく作成された project は作成した principal への owner binding から始まるため、
// 取得した binding のみを復元すれば、
// この実行自身のアクセスは維持されます。
func (s *iamRun) updateProjectPolicy(t *testing.T) {
	t.Helper()
	original := s.readPolicyBindings(t, "projectのロール付与前のbindingを読み込む", "policy", "read-project", s.project.id)
	updated := append(append([]policyBinding{}, original...), s.policyBinding(projectPolicyRole))
	s.updatePolicyBindings(t, "projectのbindingを更新してロールを付与する", "policy", "update-project", s.project.id, updated)
	s.checkPolicyBindings(t, "更新後のprojectのbindingを確認する", "policy", "read-project", s.project.id, updated)
	s.updatePolicyBindings(t, "projectのbindingを元に戻す", "policy", "update-project", s.project.id, original)
	s.checkPolicyBindings(t, "復元後のprojectのbindingを確認する", "policy", "read-project", s.project.id, original)
}

func (s *iamRun) policyBinding(roleID string) policyBinding {
	return policyBinding{
		Role:       policyRole{Type: "preset", ID: roleID},
		Principals: []policyPrincipal{{Type: policyPrincipalTyp, ID: s.principalID}},
	}
}

// updatePolicyBindings はリソースの IAM ポリシー binding を
// 指定した JSON 配列のリクエストで置き換えます。
// organization レベルの操作では resourceID を指定せず、
// コマンド引数に空の位置引数を渡しません。
func (s *iamRun) updatePolicyBindings(t *testing.T, description, kind, subcommand, resourceID string, bindings []policyBinding) {
	t.Helper()
	request := mustSettingsJSON(bindings)
	args := []string{"iam-api", kind, subcommand}
	if resourceID != "" {
		args = append(args, resourceID)
	}
	s.call(t, description, append(args, "--request", request)...)
}

// checkPolicyBindings は binding を読み込み、
// 期待されるリストと比較します。
func (s *iamRun) checkPolicyBindings(t *testing.T, description, kind, subcommand, resourceID string, expected []policyBinding) {
	t.Helper()
	actual := s.readPolicyBindings(t, description, kind, subcommand, resourceID)
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
