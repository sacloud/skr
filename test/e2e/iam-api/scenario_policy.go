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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
func (s *scenario) policyScenario(ctx context.Context) error {
	if err := s.updateFolderPolicy(ctx); err != nil {
		return err
	}
	return s.updateProjectPolicy(ctx)
}

// updateFolderPolicy はこの実行の principal に folder ロールを付与し、
// binding を確認してから元の binding に戻します。
func (s *scenario) updateFolderPolicy(ctx context.Context) error {
	original, err := s.readPolicyBindings(ctx, "read-folder-policy-before", "policy", "read-folder", s.folder.id)
	if err != nil {
		return err
	}
	updated := append(append([]policyBinding{}, original...), s.policyBinding(folderPolicyRole))
	if err := s.updatePolicyBindings(ctx, "update-folder-policy", "policy", "update-folder", s.folder.id, updated); err != nil {
		return err
	}
	if err := s.checkPolicyBindings(ctx, "read-folder-policy-updated", "policy", "read-folder", s.folder.id, updated); err != nil {
		return err
	}
	if err := s.updatePolicyBindings(ctx, "restore-folder-policy", "policy", "update-folder", s.folder.id, original); err != nil {
		return err
	}
	return s.checkPolicyBindings(ctx, "read-folder-policy-restored", "policy", "read-folder", s.folder.id, original)
}

// updateProjectPolicy は元の binding に加えて、この実行の principal に project ロールを付与し、
// 両方を確認してから元の binding に戻します。
// 新しく作成された project は作成した principal への owner binding から始まるため、
// 取得した binding のみを復元すれば、
// この実行自身のアクセスは維持されます。
func (s *scenario) updateProjectPolicy(ctx context.Context) error {
	original, err := s.readPolicyBindings(ctx, "read-project-policy-before", "policy", "read-project", s.project.id)
	if err != nil {
		return err
	}
	updated := append(append([]policyBinding{}, original...), s.policyBinding(projectPolicyRole))
	if err := s.updatePolicyBindings(ctx, "update-project-policy", "policy", "update-project", s.project.id, updated); err != nil {
		return err
	}
	if err := s.checkPolicyBindings(ctx, "read-project-policy-updated", "policy", "read-project", s.project.id, updated); err != nil {
		return err
	}
	if err := s.updatePolicyBindings(ctx, "restore-project-policy", "policy", "update-project", s.project.id, original); err != nil {
		return err
	}
	return s.checkPolicyBindings(ctx, "read-project-policy-restored", "policy", "read-project", s.project.id, original)
}

func (s *scenario) policyBinding(roleID string) policyBinding {
	return policyBinding{
		Role:       policyRole{Type: "preset", ID: roleID},
		Principals: []policyPrincipal{{Type: policyPrincipalTyp, ID: s.principalID}},
	}
}

// updatePolicyBindings はリソースの IAM ポリシー binding を
// 指定した JSON 配列のリクエストで置き換えます。
func (s *scenario) updatePolicyBindings(ctx context.Context, step, kind, subcommand, resourceID string, bindings []policyBinding) error {
	request, err := policyRequestJSON(bindings)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", step, err)
	}
	_, err = s.call(ctx, step, "iam-api", kind, subcommand, resourceID, "--request", request)
	return err
}

// checkPolicyBindings は binding を読み込み、
// 期待されるリストと比較します。
func (s *scenario) checkPolicyBindings(ctx context.Context, step, kind, subcommand, resourceID string, expected []policyBinding) error {
	actual, err := s.readPolicyBindings(ctx, step, kind, subcommand, resourceID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("%s: bindings = %s, want %s", step, policySummary(actual), policySummary(expected))
	}
	return nil
}

func policyRequestJSON(bindings []policyBinding) (string, error) {
	if bindings == nil {
		bindings = []policyBinding{}
	}
	data, err := json.Marshal(bindings)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func policySummary(bindings []policyBinding) string {
	data, err := json.Marshal(bindings)
	if err != nil {
		return fmt.Sprintf("%#v", bindings)
	}
	return string(data)
}
