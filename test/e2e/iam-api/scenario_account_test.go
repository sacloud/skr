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
	"slices"
	"testing"
)

func TestAccountScenarioUpdatesWithReadValues(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	// user-2FA コマンドはこの実行で作成した user を対象とする。
	if err := s.createUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.accountScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"account-read-password-policy",
		"account-update-password-policy",
		"account-read-auth-conditions",
		"account-update-auth-conditions",
		"account-read-id-policy",
		"account-update-id-policy",
		"account-verify-id-policy",
		"account-read-organization-policy",
		"account-update-organization-policy",
		"account-verify-organization-policy",
		"account-read-organization",
		"account-update-organization",
		"account-read-service-policy",
		"account-enable-service-policy",
		"account-verify-service-policy-enabled",
		"account-update-service-policy",
		"account-disable-service-policy",
		"account-verify-service-policy-disabled",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("account scenario did not call %s: %v", want, fake.calls)
		}
	}
	// パスワードポリシーの更新はすべてのフィールドと、
	// 読み込んだ min_length の値を含める必要がある。
	var updated passwordPolicy
	if err := json.Unmarshal(updateRequestBody(fake, "account-update-password-policy"), &updated); err != nil {
		t.Fatalf("decode account-update-password-policy request: %v", err)
	}
	if updated.MinLength != 8 {
		t.Errorf("password policy request min_length = %d, want the read value 8", updated.MinLength)
	}
	// 認証条件の更新は生の読み込み応答を再利用する必要があり、
	// この実行でモデル化していないフィールドも含める。
	request := updateRequestBody(fake, "account-update-auth-conditions")
	var rawConditions map[string]any
	if err := json.Unmarshal(request, &rawConditions); err != nil {
		t.Fatalf("decode account-update-auth-conditions request: %v", err)
	}
	if _, ok := rawConditions["datetime_restriction"]; !ok {
		t.Errorf("auth conditions request = %s, want the raw read response including datetime_restriction", request)
	}
	if restriction, ok := rawConditions["ip_restriction"].(map[string]any); !ok || restriction["mode"] != "allow_all" {
		t.Errorf("auth conditions request ip_restriction = %v, want the read mode allow_all", rawConditions["ip_restriction"])
	}
	// organization name の更新は事前に読み込んだ名前を再利用する必要がある。
	if fake.orgName != "" {
		t.Errorf("organization name = %q, want the unchanged empty name", fake.orgName)
	}
	// サービスポリシーの切り替えは無効で終了する必要がある。
	if fake.servicePolicyEnabled {
		t.Error("the service policy must end disabled after the flip")
	}
}

func TestAccountScenarioRestoresServicePolicyWhenUpdateFails(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "account-update-service-policy"
	if err := s.servicePolicyFlip(context.Background()); err == nil {
		t.Fatal("service policy update succeeded although it was rejected")
	}
	// defer された復元はポリシーを再度無効にしている必要がある。
	if fake.servicePolicyEnabled {
		t.Error("the service policy must be disabled after the failed flip")
	}
	if !slices.Contains(fake.calls, "account-restore-service-policy-disabled") {
		t.Errorf("the failed flip did not restore the disabled policy: %v", fake.calls)
	}
}

func TestAccountScenarioFailsWhenPolicyUpdateIsRejected(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	fake.failAt = "account-update-organization-policy"
	if err := s.accountScenario(context.Background()); err == nil {
		t.Fatal("organization policy update succeeded although it was rejected")
	}
}

func TestUser2FAScenarioChecksPagesAndMissingIDs(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.createUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.user2faScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"u2fa-list-trusted-devices",
		"u2fa-list-security-keys",
		"u2fa-deactivate-otp",
		"u2fa-clear-trusted-devices",
		"u2fa-read-missing-security-key",
		"u2fa-delete-missing-trusted-device",
		"u2fa-delete-missing-security-key",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("user 2FA scenario did not call %s: %v", want, fake.calls)
		}
	}
	for _, want := range []string{
		"u2fa-list-trusted-devices",
		"u2fa-list-security-keys",
		"u2fa-deactivate-otp",
		"u2fa-clear-trusted-devices",
	} {
		if got := argument(fake.argsByStep[want], "--user-id"); got != s.user.id {
			t.Errorf("%s --user-id = %q, want the user of this run %q", want, got, s.user.id)
		}
	}
	for _, want := range []string{
		"u2fa-read-missing-security-key",
		"u2fa-delete-missing-trusted-device",
		"u2fa-delete-missing-security-key",
	} {
		if got := lastArg(fake.argsByStep[want]); got != missingTwoFactorResourceID {
			t.Errorf("%s resource ID = %q, want the missing ID %q", want, got, missingTwoFactorResourceID)
		}
	}
}

func TestUser2FAScenarioFailsWhenListIsRejected(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.createUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.failAt = "u2fa-list-trusted-devices"
	if err := s.user2faScenario(context.Background()); err == nil {
		t.Fatal("user 2FA scenario succeeded although the list was rejected")
	}
}

// updateRequestBody は --request とともに渡された JSON リクエスト本文を返します。
func updateRequestBody(fake *fakeCLI, step string) []byte {
	args := fake.argsByStep[step]
	for i, arg := range args {
		if arg == "--request" && i+1 < len(args) {
			return []byte(args[i+1])
		}
	}
	return nil
}
