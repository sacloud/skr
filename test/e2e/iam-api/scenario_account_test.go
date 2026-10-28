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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const missingTwoFactorResourceID = "999999999999"

// accountScenario は organization スコープの書き込みコマンドを実行します。
// すべての更新は事前に読み込んだ値を送信するため、
// 書き込み経路を網羅しつつ organization 全体の設定は変更されません。
func (s *iamRun) accountScenario(t *testing.T) {
	t.Helper()
	s.updatePasswordPolicy(t)
	s.updateAuthConditions(t)
	s.updateIDPolicy(t)
	s.updateOrganizationPolicyBindings(t)
	s.updateOrganizationName(t)
	s.servicePolicyFlip(t)
}

// updatePasswordPolicy は現在のパスワードポリシーを読み込み、同じ値で更新します。
// 更新にはすべてのフィールドが必要なため、
// リクエストは読み込んだ応答をそのまま再利用します。
func (s *iamRun) updatePasswordPolicy(t *testing.T) {
	t.Helper()
	data := s.call(t, "organizationのパスワードポリシーを読み込む", "iam-api", "auth", "read-password-policy")
	var policy passwordPolicy
	require.NoError(t, json.Unmarshal(data, &policy), "account-read-password-policy: パスワードポリシーのデコードに失敗しました")
	require.Positive(t, policy.MinLength, "account-read-password-policy: min_lengthが正の値であること")

	updated := s.call(t, "読み込んだ値でパスワードポリシーを更新する",
		"iam-api", "auth", "update-password-policy", "--request", string(data))
	var updatedPolicy passwordPolicy
	require.NoError(t, json.Unmarshal(updated, &updatedPolicy), "account-update-password-policy: パスワードポリシーのデコードに失敗しました")
	require.Equal(t, policy, updatedPolicy, "account-update-password-policy: 更新後のポリシーが元の値と一致すること")
}

// updateAuthConditions は現在の認証条件を読み込み、同じ値で更新します。
// 生の JSON をそのまま送り返すため、
// この実行でモデル化していない datetime_restriction などのフィールドも保持されます。
func (s *iamRun) updateAuthConditions(t *testing.T) {
	t.Helper()
	data := s.call(t, "organizationの認証条件を読み込む", "iam-api", "auth", "read-auth-conditions")
	var conditions authConditions
	require.NoError(t, json.Unmarshal(data, &conditions), "account-read-auth-conditions: 認証条件のデコードに失敗しました")
	require.NotEmpty(t, conditions.IPRestriction.Mode, "account-read-auth-conditions: ip_restriction.modeが存在すること")

	updated := s.call(t, "読み込んだ値で認証条件を更新する",
		"iam-api", "auth", "update-auth-conditions", "--request", string(data))
	var updatedConditions authConditions
	require.NoError(t, json.Unmarshal(updated, &updatedConditions), "account-update-auth-conditions: 認証条件のデコードに失敗しました")
	require.Equal(t, conditions.IPRestriction.Mode, updatedConditions.IPRestriction.Mode,
		"account-update-auth-conditions: 更新後のip_restriction.modeが元の値と一致すること")
}

// updateIDPolicy は organization の ID ポリシー binding を、事前に読み込んだ binding で置き換えます。
// これにより割り当ては変更されません。
func (s *iamRun) updateIDPolicy(t *testing.T) {
	t.Helper()
	original := s.readPolicyBindings(t, "organizationのIDポリシーbindingを読み込む", "id-policy", "read-organization", "")
	s.updatePolicyBindings(t, "読み込んだ値でIDポリシーbindingを更新する",
		"id-policy", "update-organization", "", original)
	restored := s.readPolicyBindings(t, "更新後のIDポリシーbindingを確認する", "id-policy", "read-organization", "")
	require.Equal(t, original, restored, "account-verify-id-policy: bindingが元の値と一致すること")
}

// updateOrganizationPolicyBindings は organization の IAM ポリシー binding を、
// 事前に読み込んだ binding で置き換えます。
func (s *iamRun) updateOrganizationPolicyBindings(t *testing.T) {
	t.Helper()
	original := s.readPolicyBindings(t, "organizationのIAMポリシーbindingを読み込む", "policy", "read-organization", "")
	s.updatePolicyBindings(t, "読み込んだ値でIAMポリシーbindingを更新する",
		"policy", "update-organization", "", original)
	restored := s.readPolicyBindings(t, "更新後のIAMポリシーbindingを確認する", "policy", "read-organization", "")
	require.Equal(t, original, restored, "account-verify-organization-policy: bindingが元の値と一致すること")
}

// updateOrganizationName は organization name を、事前に読み込んだ名前で更新します。
// これにより表示される organization は変更されません。
func (s *iamRun) updateOrganizationName(t *testing.T) {
	t.Helper()
	data := s.call(t, "organizationを読み込む", "iam-api", "organization", "read")
	var organization organizationInfo
	require.NoError(t, json.Unmarshal(data, &organization), "account-read-organization: organizationのデコードに失敗しました")

	updated := s.call(t, "読み込んだ名前でorganizationを更新する",
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
func (s *iamRun) servicePolicyFlip(t *testing.T) {
	t.Helper()
	data := s.call(t, "organizationのサービスポリシーのルールを読み込む", "iam-api", "organization", "read-service-policy")
	var rules []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &rules), "account-read-service-policy: ルールのデコードに失敗しました")

	s.setServicePolicyEnabled(t, "organizationのサービスポリシーを有効にする", true)
	restored := false
	defer func() {
		if restored {
			return
		}
		s.setServicePolicyEnabled(t, "organizationのサービスポリシーを無効に戻す", false)
	}()

	s.checkServicePolicyEnabled(t, "有効化後のサービスポリシーの状態を確認する", true)
	updated := s.call(t, "読み込んだ値でサービスポリシーのルールを更新する",
		"iam-api", "organization", "update-service-policy", "--request", string(data))
	var updatedRules []json.RawMessage
	require.NoError(t, json.Unmarshal(updated, &updatedRules), "account-update-service-policy: ルールのデコードに失敗しました")
	require.Equal(t, rules, updatedRules, "account-update-service-policy: 更新後のルールが元の値と一致すること")

	s.setServicePolicyEnabled(t, "organizationのサービスポリシーを無効にする", false)
	restored = true
	s.checkServicePolicyEnabled(t, "無効化後のサービスポリシーの状態を確認する", false)
}

func (s *iamRun) setServicePolicyEnabled(t *testing.T, description string, enabled bool) {
	t.Helper()
	subcommand := "disable"
	if enabled {
		subcommand = "enable"
	}
	s.call(t, description, "iam-api", "service-policy", subcommand)
}

func (s *iamRun) checkServicePolicyEnabled(t *testing.T, description string, want bool) {
	t.Helper()
	data := s.call(t, description, "iam-api", "service-policy", "is-enabled")
	var enabled bool
	require.NoError(t, json.Unmarshal(data, &enabled), "%s: 真偽値のデコードに失敗しました", description)
	require.Equal(t, want, enabled, "%s: サービスポリシーの有効状態が期待値と一致すること", description)
}

// user2faScenario は、この実行で作成した user に対して user 2FA コマンドを実行します。
// user は OTP、trusted device、security key を持たないため、
// 一覧コマンドは空のページを返し、
// deactivate と clear コマンドはログイン方法を変更せずに成功し、
// 存在しないリソース ID に対するコマンドは not-found エラーを返します。
func (s *iamRun) user2faScenario(t *testing.T) {
	t.Helper()
	s.checkEmpty2FAPage(t, "userのtrusted device一覧が空であることを確認する", "list-trusted-devices")
	s.checkEmpty2FAPage(t, "userのsecurity key一覧が空であることを確認する", "list-security-keys")
	s.call(t, "userのOTPを無効化する", "iam-api", "user-2fa", "deactivate-otp", "--user-id", s.user.id)
	s.call(t, "userのtrusted deviceを解除する", "iam-api", "user-2fa", "clear-trusted-devices", "--user-id", s.user.id)
	s.expectNotFound(t, "存在しないsecurity keyの参照がnot-foundになることを確認する",
		"iam-api", "user-2fa", "read-security-key", "--user-id", s.user.id, missingTwoFactorResourceID)
	s.expectNotFound(t, "存在しないtrusted deviceの削除がnot-foundになることを確認する",
		"iam-api", "user-2fa", "delete-trusted-device", "--user-id", s.user.id, missingTwoFactorResourceID)
	s.expectNotFound(t, "存在しないsecurity keyの削除がnot-foundになることを確認する",
		"iam-api", "user-2fa", "delete-security-key", "--user-id", s.user.id, missingTwoFactorResourceID)
}

// checkEmpty2FAPage は user の 2FA リソースを一覧表示し、
// そのページが空であることを確認します。
func (s *iamRun) checkEmpty2FAPage(t *testing.T, description, subcommand string) {
	t.Helper()
	data := s.call(t, description, "iam-api", "user-2fa", subcommand, "--user-id", s.user.id)
	var page twoFactorPage
	require.NoError(t, json.Unmarshal(data, &page), "%s: ページのデコードに失敗しました", description)
	require.Empty(t, page.Items, "%s: 一覧が空であること", description)
	require.Zero(t, page.Count, "%s: 件数が0であること", description)
}

// expectNotFound は、
// コマンドが静かに成功するのではなく 404 not-found 分類で失敗することを確認します。
func (s *iamRun) expectNotFound(t *testing.T, description string, args ...string) {
	t.Helper()
	_, stderr, err := s.callWithErr(t, description, args...)
	require.Error(t, err, "%s: not-foundエラーになること", description)
	require.Contains(t, stderr, "API Error 404", "%s: 404 not-found分類で失敗すること", description)
}

type twoFactorPage struct {
	Count    int               `json:"count"`
	Items    []json.RawMessage `json:"items"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
}

// mustSettingsJSON は値を JSON 文字列に変換します。
// エラーはあり得ないため、フォールバックとして %#v を返します。
func mustSettingsJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(data)
}
