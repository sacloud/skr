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

// このファイルは iam-api E2E の read-only シナリオです。
// この実行でリソースを作成せず、読み取りコマンドを確認します。
package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

type roleSummary struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Description             string `json:"description"`
	Category                string `json:"category"`
	LowestGrantableResource string `json:"lowest_grantable_resource"`
}

type roleList struct {
	Items []roleSummary `json:"items"`
}

type policyRole struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type policyPrincipal struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

type policyBinding struct {
	Role       policyRole        `json:"role"`
	Principals []policyPrincipal `json:"principals"`
}

type authContext struct {
	ResourceID         int64  `json:"resource_id"`
	AuthType           string `json:"auth_type"`
	LimitedToProjectID *int64 `json:"limited_to_project_id"`
	MemberCode         string `json:"member_code"`
}

type ipRestriction struct {
	Mode string `json:"mode"`
}

type twoFactorAuth struct {
	Enabled bool `json:"enabled"`
}

type authConditions struct {
	IPRestriction        ipRestriction `json:"ip_restriction"`
	RequireTwoFactorAuth twoFactorAuth `json:"require_two_factor_auth"`
}

type passwordPolicy struct {
	MinLength        int  `json:"min_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireLowercase bool `json:"require_lowercase"`
	RequireSymbols   bool `json:"require_symbols"`
}

type organizationInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ruleTemplateSummary struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Type           string `json:"type"`
	SupportsDryRun bool   `json:"supports_dry_run"`
}

type ruleTemplateList struct {
	Items []ruleTemplateSummary `json:"items"`
}

// TestScenarioIAMReadOnly は、この実行でリソースを作成せず、
// IAM API の read-only コマンドを確認します。
// すべてのコマンドはデータの読み込みのみを行うため、
// organization 全体の設定は変更されません。
func TestScenarioIAMReadOnly(t *testing.T) {
	h := helper.New(t, "iam-api-read-only",
		"IAM API の読み取りコマンドが正しいデータを返すことを確認する")

	// iam-role と id-role のロールカタログを確認する
	checkRoleCatalog(t, h, "iam-role")
	checkRoleCatalog(t, h, "id-role")
	// organization の ID ポリシー binding を読み込む
	readPolicyBindings(t, h, "organizationのIDポリシーbindingを読み込む", "id-policy", "read-organization", "")
	// 認証コンテキストを読み込む
	checkAuthContext(t, h)
	// 認証条件とパスワードポリシーを読み込む
	checkAuthConditions(t, h)
	checkPasswordPolicy(t, h)
	// organization の IAM ポリシー binding を読み込む
	readPolicyBindings(t, h, "organizationのIAMポリシーbindingを読み込む", "policy", "read-organization", "")
	// organization とサービスポリシーを確認する
	checkOrganization(t, h)
	checkServicePolicy(t, h)
}

// checkRoleCatalog は利用可能なロールを一覧表示し、最初の1つを読み込みます。
// kind は iam-role または id-role です。
func checkRoleCatalog(t *testing.T, h *helper.Helper, kind string) {
	t.Helper()
	data := h.Call(t, kind+"のロール一覧を取得する", "iam-api", kind, "list")
	var list roleList
	require.NoError(t, json.Unmarshal(data, &list), "%s-list: ロール一覧のデコードに失敗しました", kind)
	require.NotEmpty(t, list.Items, "%s-list: ロールカタログが空です", kind)
	first := list.Items[0]
	require.NotEmpty(t, first.ID, "%s-list: 最初のロールにIDが存在すること", kind)
	require.NotEmpty(t, first.Name, "%s-list: 最初のロールに名前が存在すること", kind)

	readData := h.Call(t, kind+"のロールを読み込む", "iam-api", kind, "read", first.ID)
	var read roleSummary
	require.NoError(t, json.Unmarshal(readData, &read), "%s-read: ロールのデコードに失敗しました", kind)
	require.Equal(t, first.ID, read.ID, "%s-read: 読み込んだロールのIDが一覧と一致すること", kind)
	require.Equal(t, first.Name, read.Name, "%s-read: 読み込んだロールの名前が一覧と一致すること", kind)
}

// readPolicyBindings は organization、folder、project の IAM ポリシー binding を読み込みます。
// organization の場合 resourceID は空です。
func readPolicyBindings(t *testing.T, h *helper.Helper, description, kind, subcommand, resourceID string) []policyBinding {
	t.Helper()
	args := []string{"iam-api", kind, subcommand}
	if resourceID != "" {
		args = append(args, resourceID)
	}
	data := h.Call(t, description, args...)
	var bindings []policyBinding
	require.NoError(t, json.Unmarshal(data, &bindings), "%s: ポリシーbindingのデコードに失敗しました", description)
	return bindings
}

// checkAuthContext は認証コンテキストを読み込み、
// principal の ID と認証方式が存在することを確認します。
// principal が特定の project に限定されている場合、その project ID を返します。
func checkAuthContext(t *testing.T, h *helper.Helper) authContext {
	t.Helper()
	data := h.Call(t, "認証コンテキストを読み込む", "iam-api", "auth", "read-auth-context")
	var context authContext
	require.NoError(t, json.Unmarshal(data, &context), "read-auth-context: 認証コンテキストのデコードに失敗しました")
	require.Positive(t, context.ResourceID, "read-auth-context: resource_idが正の値であること")
	require.NotEmpty(t, context.AuthType, "read-auth-context: auth_typeが存在すること")
	return context
}

func checkAuthConditions(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "認証条件を読み込む", "iam-api", "auth", "read-auth-conditions")
	var conditions authConditions
	require.NoError(t, json.Unmarshal(data, &conditions), "read-auth-conditions: 認証条件のデコードに失敗しました")
	require.NotEmpty(t, conditions.IPRestriction.Mode, "read-auth-conditions: ip_restriction.modeが存在すること")
}

func checkPasswordPolicy(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "パスワードポリシーを読み込む", "iam-api", "auth", "read-password-policy")
	var policy passwordPolicy
	require.NoError(t, json.Unmarshal(data, &policy), "read-password-policy: パスワードポリシーのデコードに失敗しました")
	require.Positive(t, policy.MinLength, "read-password-policy: min_lengthが正の値であること")
}

func checkOrganization(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "organizationを読み込む", "iam-api", "organization", "read")
	var organization organizationInfo
	require.NoError(t, json.Unmarshal(data, &organization), "organization-read: organizationのデコードに失敗しました")
	require.Positive(t, organization.ID, "organization-read: idが正の値であること")
}

func checkServicePolicy(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "サービスポリシーの有効状態を確認する", "iam-api", "service-policy", "is-enabled")
	var enabled bool
	require.NoError(t, json.Unmarshal(data, &enabled), "service-policy-is-enabled: 真偽値のデコードに失敗しました")

	data = h.Call(t, "organizationのサービスポリシーのルールを読み込む", "iam-api", "organization", "read-service-policy")
	var rules []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &rules), "read-service-policy: ルールのデコードに失敗しました")

	data = h.Call(t, "サービスポリシーのルールテンプレート一覧を取得する", "iam-api", "service-policy", "list-rule-templates")
	var templates ruleTemplateList
	require.NoError(t, json.Unmarshal(data, &templates), "list-rule-templates: ルールテンプレートのデコードに失敗しました")
	require.NotEmpty(t, templates.Items, "list-rule-templates: ルールテンプレートのカタログが空です")
}
