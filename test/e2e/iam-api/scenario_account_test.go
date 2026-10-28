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

// このファイルは iam-api E2E の organization スコープとユーザー 2 要素認証のシナリオです。
// すべての更新は事前に読み込んだ値を送信するため、organization 全体の設定は変更されません。
package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const missingTwoFactorResourceID = "999999999999"

// TestScenarioIAMAccount は organization スコープの書き込みコマンドを現在値の往復で確認し、
// テスト用の user に対して user 2FA コマンドを確認します。
func TestScenarioIAMAccount(t *testing.T) {
	h := helper.New(t, "iam-api-account",
		"organization スコープの更新コマンドが現在値を維持することと user 2FA コマンドの分類を確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	run := &iamRun{
		password: randomPassword(t),
		user:     iamResource{kind: "user", name: name, code: "e2e_" + suffix, description: resourceDescription + suffix},
	}

	// 作成前に同名の user が存在しないことを確認する
	requireNoNameCollision(t, h, run)

	// 後片付け: テスト用の user を再照合つき削除して残っていないことを確認する
	defer func() {
		if err := cleanupIAMResources(t, h, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// テスト用の user を作成する
	createUser(t, h, run)
	// organization スコープの更新コマンドを現在値の往復で確認する
	runAccountScenario(t, h)
	// user 2FA コマンドを確認する
	runUser2FAScenario(t, h, run)
}

// runAccountScenario は organization スコープの書き込みコマンドを実行します。
// すべての更新は事前に読み込んだ値を送信するため、
// 書き込み経路を網羅しつつ organization 全体の設定は変更されません。
func runAccountScenario(t *testing.T, h *helper.Helper) {
	t.Helper()
	updatePasswordPolicy(t, h)
	updateAuthConditions(t, h)
	updateIDPolicy(t, h)
	updateOrganizationPolicyBindings(t, h)
	updateOrganizationName(t, h)
	servicePolicyFlip(t, h)
}

// updatePasswordPolicy は現在のパスワードポリシーを読み込み、同じ値で更新します。
// 更新にはすべてのフィールドが必要なため、
// リクエストは読み込んだ応答をそのまま再利用します。
func updatePasswordPolicy(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "organizationのパスワードポリシーを読み込む", "iam-api", "auth", "read-password-policy")
	var policy passwordPolicy
	require.NoError(t, json.Unmarshal(data, &policy), "account-read-password-policy: パスワードポリシーのデコードに失敗しました")
	require.Positive(t, policy.MinLength, "account-read-password-policy: min_lengthが正の値であること")

	updated := h.Call(t, "読み込んだ値でパスワードポリシーを更新する",
		"iam-api", "auth", "update-password-policy", "--request", string(data))
	var updatedPolicy passwordPolicy
	require.NoError(t, json.Unmarshal(updated, &updatedPolicy), "account-update-password-policy: パスワードポリシーのデコードに失敗しました")
	require.Equal(t, policy, updatedPolicy, "account-update-password-policy: 更新後のポリシーが元の値と一致すること")
}

// updateAuthConditions は現在の認証条件を読み込み、同じ値で更新します。
// 生の JSON をそのまま送り返すため、
// この実行でモデル化していない datetime_restriction などのフィールドも保持されます。
func updateAuthConditions(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "organizationの認証条件を読み込む", "iam-api", "auth", "read-auth-conditions")
	var conditions authConditions
	require.NoError(t, json.Unmarshal(data, &conditions), "account-read-auth-conditions: 認証条件のデコードに失敗しました")
	require.NotEmpty(t, conditions.IPRestriction.Mode, "account-read-auth-conditions: ip_restriction.modeが存在すること")

	updated := h.Call(t, "読み込んだ値で認証条件を更新する",
		"iam-api", "auth", "update-auth-conditions", "--request", string(data))
	var updatedConditions authConditions
	require.NoError(t, json.Unmarshal(updated, &updatedConditions), "account-update-auth-conditions: 認証条件のデコードに失敗しました")
	require.Equal(t, conditions.IPRestriction.Mode, updatedConditions.IPRestriction.Mode,
		"account-update-auth-conditions: 更新後のip_restriction.modeが元の値と一致すること")
}

// updateIDPolicy は organization の ID ポリシー binding を、事前に読み込んだ binding で置き換えます。
// これにより割り当ては変更されません。
func updateIDPolicy(t *testing.T, h *helper.Helper) {
	t.Helper()
	original := readPolicyBindings(t, h, "organizationのIDポリシーbindingを読み込む", "id-policy", "read-organization", "")
	updatePolicyBindings(t, h, "読み込んだ値でIDポリシーbindingを更新する",
		"id-policy", "update-organization", "", original)
	restored := readPolicyBindings(t, h, "更新後のIDポリシーbindingを確認する", "id-policy", "read-organization", "")
	require.Equal(t, original, restored, "account-verify-id-policy: bindingが元の値と一致すること")
}

// updateOrganizationPolicyBindings は organization の IAM ポリシー binding を、
// 事前に読み込んだ binding で置き換えます。
func updateOrganizationPolicyBindings(t *testing.T, h *helper.Helper) {
	t.Helper()
	original := readPolicyBindings(t, h, "organizationのIAMポリシーbindingを読み込む", "policy", "read-organization", "")
	updatePolicyBindings(t, h, "読み込んだ値でIAMポリシーbindingを更新する",
		"policy", "update-organization", "", original)
	restored := readPolicyBindings(t, h, "更新後のIAMポリシーbindingを確認する", "policy", "read-organization", "")
	require.Equal(t, original, restored, "account-verify-organization-policy: bindingが元の値と一致すること")
}

// updateOrganizationName は organization name を、事前に読み込んだ名前で更新します。
// これにより表示される organization は変更されません。
func updateOrganizationName(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "organizationを読み込む", "iam-api", "organization", "read")
	var organization organizationInfo
	require.NoError(t, json.Unmarshal(data, &organization), "account-read-organization: organizationのデコードに失敗しました")

	updated := h.Call(t, "読み込んだ名前でorganizationを更新する",
		"iam-api", "organization", "update", "--name", organization.Name)
	var updatedOrganization organizationInfo
	require.NoError(t, json.Unmarshal(updated, &updatedOrganization), "account-update-organization: organizationのデコードに失敗しました")
	require.Equal(t, organization.Name, updatedOrganization.Name, "account-update-organization: 更新後の名前が元の値と一致すること")
	require.Equal(t, organization.ID, updatedOrganization.ID, "account-update-organization: IDが一致すること")
}

// servicePolicyFlip は organization のサービスポリシーを有効にし、
// 事前に読み込んだ値でルールを更新してから、再び無効にします。
// ルールはポリシーが有効な間のみ更新できるため、
// 更新を網羅するには有効化が必要です。
// ポリシーは無効の状態で開始・終了し、defer された復元はステップが失敗してもその状態を保ちます。
func servicePolicyFlip(t *testing.T, h *helper.Helper) {
	t.Helper()
	data := h.Call(t, "organizationのサービスポリシーのルールを読み込む", "iam-api", "organization", "read-service-policy")
	var rules []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &rules), "account-read-service-policy: ルールのデコードに失敗しました")

	setServicePolicyEnabled(t, h, "organizationのサービスポリシーを有効にする", true)
	restored := false
	defer func() {
		if restored {
			return
		}
		setServicePolicyEnabled(t, h, "organizationのサービスポリシーを無効に戻す", false)
	}()

	checkServicePolicyEnabled(t, h, "有効化後のサービスポリシーの状態を確認する", true)
	updated := h.Call(t, "読み込んだ値でサービスポリシーのルールを更新する",
		"iam-api", "organization", "update-service-policy", "--request", string(data))
	var updatedRules []json.RawMessage
	require.NoError(t, json.Unmarshal(updated, &updatedRules), "account-update-service-policy: ルールのデコードに失敗しました")
	require.Equal(t, rules, updatedRules, "account-update-service-policy: 更新後のルールが元の値と一致すること")

	setServicePolicyEnabled(t, h, "organizationのサービスポリシーを無効にする", false)
	restored = true
	checkServicePolicyEnabled(t, h, "無効化後のサービスポリシーの状態を確認する", false)
}

func setServicePolicyEnabled(t *testing.T, h *helper.Helper, description string, enabled bool) {
	t.Helper()
	subcommand := "disable"
	if enabled {
		subcommand = "enable"
	}
	h.Call(t, description, "iam-api", "service-policy", subcommand)
}

func checkServicePolicyEnabled(t *testing.T, h *helper.Helper, description string, want bool) {
	t.Helper()
	data := h.Call(t, description, "iam-api", "service-policy", "is-enabled")
	var enabled bool
	require.NoError(t, json.Unmarshal(data, &enabled), "%s: 真偽値のデコードに失敗しました", description)
	require.Equal(t, want, enabled, "%s: サービスポリシーの有効状態が期待値と一致すること", description)
}

// runUser2FAScenario は、この実行で作成した user に対して user 2FA コマンドを実行します。
// user は OTP、trusted device、security key を持たないため、
// 一覧コマンドは空のページを返し、
// deactivate と clear コマンドはログイン方法を変更せずに成功し、
// 存在しないリソース ID に対するコマンドは not-found エラーを返します。
func runUser2FAScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	checkEmpty2FAPage(t, h, run, "userのtrusted device一覧が空であることを確認する", "list-trusted-devices")
	checkEmpty2FAPage(t, h, run, "userのsecurity key一覧が空であることを確認する", "list-security-keys")
	h.Call(t, "userのOTPを無効化する", "iam-api", "user-2fa", "deactivate-otp", "--user-id", run.user.id)
	h.Call(t, "userのtrusted deviceを解除する", "iam-api", "user-2fa", "clear-trusted-devices", "--user-id", run.user.id)
	expectNotFound(t, h, "存在しないsecurity keyの参照がnot-foundになることを確認する",
		"iam-api", "user-2fa", "read-security-key", "--user-id", run.user.id, missingTwoFactorResourceID)
	expectNotFound(t, h, "存在しないtrusted deviceの削除がnot-foundになることを確認する",
		"iam-api", "user-2fa", "delete-trusted-device", "--user-id", run.user.id, missingTwoFactorResourceID)
	expectNotFound(t, h, "存在しないsecurity keyの削除がnot-foundになることを確認する",
		"iam-api", "user-2fa", "delete-security-key", "--user-id", run.user.id, missingTwoFactorResourceID)
}

// checkEmpty2FAPage は user の 2FA リソースを一覧表示し、
// そのページが空であることを確認します。
func checkEmpty2FAPage(t *testing.T, h *helper.Helper, run *iamRun, description, subcommand string) {
	t.Helper()
	data := h.Call(t, description, "iam-api", "user-2fa", subcommand, "--user-id", run.user.id)
	var page twoFactorPage
	require.NoError(t, json.Unmarshal(data, &page), "%s: ページのデコードに失敗しました", description)
	require.Empty(t, page.Items, "%s: 一覧が空であること", description)
	require.Zero(t, page.Count, "%s: 件数が0であること", description)
}

// expectNotFound は、
// コマンドが静かに成功するのではなく 404 not-found 分類で失敗することを確認します。
func expectNotFound(t *testing.T, h *helper.Helper, description string, args ...string) {
	t.Helper()
	_, stderr, err := h.CallAllowingFailure(t, description, args...)
	require.Error(t, err, "%s: not-foundエラーになること", description)
	require.Contains(t, stderr, "API Error 404", "%s: 404 not-found分類で失敗すること", description)
}

type twoFactorPage struct {
	Count    int               `json:"count"`
	Items    []json.RawMessage `json:"items"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
}
