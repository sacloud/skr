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
	"strings"
)

const missingTwoFactorResourceID = "999999999999"

// accountScenario exercises the organization-scoped write commands. Every
// update sends the values that were read before, so the organization-wide
// configuration stays unchanged while the write paths are covered.
func (s *scenario) accountScenario(ctx context.Context) error {
	if err := s.updatePasswordPolicy(ctx); err != nil {
		return err
	}
	if err := s.updateAuthConditions(ctx); err != nil {
		return err
	}
	if err := s.updateIDPolicy(ctx); err != nil {
		return err
	}
	if err := s.updateOrganizationPolicyBindings(ctx); err != nil {
		return err
	}
	if err := s.updateOrganizationName(ctx); err != nil {
		return err
	}
	return s.servicePolicyFlip(ctx)
}

// updatePasswordPolicy reads the current password policy and updates it with
// the same values. The update requires every field, so the request reuses the
// read response verbatim.
func (s *scenario) updatePasswordPolicy(ctx context.Context) error {
	data, err := s.call(ctx, "account-read-password-policy", "iam-api", "auth", "read-password-policy")
	if err != nil {
		return err
	}
	var policy passwordPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("account-read-password-policy: decode password policy: %w", err)
	}
	if policy.MinLength <= 0 {
		return fmt.Errorf("account-read-password-policy: min_length = %d, want a positive value", policy.MinLength)
	}
	updated, err := s.call(ctx, "account-update-password-policy", "iam-api", "auth", "update-password-policy", "--request", string(data))
	if err != nil {
		return err
	}
	var updatedPolicy passwordPolicy
	if err := json.Unmarshal(updated, &updatedPolicy); err != nil {
		return fmt.Errorf("account-update-password-policy: decode password policy: %w", err)
	}
	if !reflect.DeepEqual(updatedPolicy, policy) {
		return fmt.Errorf("account-update-password-policy: policy = %s, want %s", mustSettingsJSON(updatedPolicy), mustSettingsJSON(policy))
	}
	return nil
}

// updateAuthConditions reads the current authentication conditions and updates
// them with the same values. The raw JSON is sent back verbatim so fields this
// run does not model, such as datetime_restriction, stay intact.
func (s *scenario) updateAuthConditions(ctx context.Context) error {
	data, err := s.call(ctx, "account-read-auth-conditions", "iam-api", "auth", "read-auth-conditions")
	if err != nil {
		return err
	}
	var conditions authConditions
	if err := json.Unmarshal(data, &conditions); err != nil {
		return fmt.Errorf("account-read-auth-conditions: decode authentication conditions: %w", err)
	}
	if conditions.IPRestriction.Mode == "" {
		return fmt.Errorf("account-read-auth-conditions: ip_restriction.mode is empty")
	}
	updated, err := s.call(ctx, "account-update-auth-conditions", "iam-api", "auth", "update-auth-conditions", "--request", string(data))
	if err != nil {
		return err
	}
	var updatedConditions authConditions
	if err := json.Unmarshal(updated, &updatedConditions); err != nil {
		return fmt.Errorf("account-update-auth-conditions: decode authentication conditions: %w", err)
	}
	if updatedConditions.IPRestriction.Mode != conditions.IPRestriction.Mode {
		return fmt.Errorf(
			"account-update-auth-conditions: ip_restriction.mode = %q, want %q",
			updatedConditions.IPRestriction.Mode, conditions.IPRestriction.Mode,
		)
	}
	return nil
}

// updateIDPolicy replaces the organization ID policy bindings with the
// bindings that were read before, so the assignments stay unchanged.
func (s *scenario) updateIDPolicy(ctx context.Context) error {
	original, err := s.readPolicyBindings(ctx, "account-read-id-policy", "id-policy", "read-organization", "")
	if err != nil {
		return err
	}
	request, err := policyRequestJSON(original)
	if err != nil {
		return fmt.Errorf("account-update-id-policy: build request: %w", err)
	}
	if _, err := s.call(ctx, "account-update-id-policy", "iam-api", "id-policy", "update-organization", "--request", request); err != nil {
		return err
	}
	restored, err := s.readPolicyBindings(ctx, "account-verify-id-policy", "id-policy", "read-organization", "")
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(restored, original) {
		return fmt.Errorf("account-verify-id-policy: bindings = %s, want %s", policySummary(restored), policySummary(original))
	}
	return nil
}

// updateOrganizationPolicyBindings replaces the organization IAM policy
// bindings with the bindings that were read before.
func (s *scenario) updateOrganizationPolicyBindings(ctx context.Context) error {
	original, err := s.readPolicyBindings(ctx, "account-read-organization-policy", "policy", "read-organization", "")
	if err != nil {
		return err
	}
	request, err := policyRequestJSON(original)
	if err != nil {
		return fmt.Errorf("account-update-organization-policy: build request: %w", err)
	}
	if _, err := s.call(ctx, "account-update-organization-policy", "iam-api", "policy", "update-organization", "--request", request); err != nil {
		return err
	}
	restored, err := s.readPolicyBindings(ctx, "account-verify-organization-policy", "policy", "read-organization", "")
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(restored, original) {
		return fmt.Errorf("account-verify-organization-policy: bindings = %s, want %s", policySummary(restored), policySummary(original))
	}
	return nil
}

// updateOrganizationName updates the organization name with the name that was
// read before, so the visible organization stays unchanged.
func (s *scenario) updateOrganizationName(ctx context.Context) error {
	data, err := s.call(ctx, "account-read-organization", "iam-api", "organization", "read")
	if err != nil {
		return err
	}
	var organization organizationInfo
	if err := json.Unmarshal(data, &organization); err != nil {
		return fmt.Errorf("account-read-organization: decode organization: %w", err)
	}
	updated, err := s.call(ctx, "account-update-organization", "iam-api", "organization", "update", "--name", organization.Name)
	if err != nil {
		return err
	}
	var updatedOrganization organizationInfo
	if err := json.Unmarshal(updated, &updatedOrganization); err != nil {
		return fmt.Errorf("account-update-organization: decode organization: %w", err)
	}
	if updatedOrganization.Name != organization.Name {
		return fmt.Errorf("account-update-organization: name = %q, want %q", updatedOrganization.Name, organization.Name)
	}
	if updatedOrganization.ID != organization.ID {
		return fmt.Errorf("account-update-organization: id = %d, want %d", updatedOrganization.ID, organization.ID)
	}
	return nil
}

// servicePolicyFlip enables the organization service policy, updates the rules
// with the values that were read before, and disables the policy again. The
// rules can only be updated while the policy is enabled, so the flip is needed
// to cover the update. The policy starts and ends disabled, and the deferred
// restore keeps that state even when a step fails.
func (s *scenario) servicePolicyFlip(ctx context.Context) error {
	data, err := s.call(ctx, "account-read-service-policy", "iam-api", "organization", "read-service-policy")
	if err != nil {
		return err
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("account-read-service-policy: decode rules: %w", err)
	}
	if err := s.setServicePolicyEnabled(ctx, "account-enable-service-policy", true); err != nil {
		return err
	}
	restored := false
	defer func() {
		if restored {
			return
		}
		if err := s.setServicePolicyEnabled(ctx, "account-restore-service-policy-disabled", false); err != nil {
			fmt.Printf("restore disabled service policy: %v\n", err)
		}
	}()
	if err := s.checkServicePolicyEnabled(ctx, "account-verify-service-policy-enabled", true); err != nil {
		return err
	}
	updated, err := s.call(ctx, "account-update-service-policy", "iam-api", "organization", "update-service-policy", "--request", string(data))
	if err != nil {
		return err
	}
	var updatedRules []json.RawMessage
	if err := json.Unmarshal(updated, &updatedRules); err != nil {
		return fmt.Errorf("account-update-service-policy: decode rules: %w", err)
	}
	if !reflect.DeepEqual(rules, updatedRules) {
		return fmt.Errorf("account-update-service-policy: rules = %s, want %s", mustSettingsJSON(updatedRules), mustSettingsJSON(rules))
	}
	if err := s.setServicePolicyEnabled(ctx, "account-disable-service-policy", false); err != nil {
		return err
	}
	restored = true
	return s.checkServicePolicyEnabled(ctx, "account-verify-service-policy-disabled", false)
}

func (s *scenario) setServicePolicyEnabled(ctx context.Context, step string, enabled bool) error {
	subcommand := "disable"
	if enabled {
		subcommand = "enable"
	}
	_, err := s.call(ctx, step, "iam-api", "service-policy", subcommand)
	return err
}

func (s *scenario) checkServicePolicyEnabled(ctx context.Context, step string, want bool) error {
	data, err := s.call(ctx, step, "iam-api", "service-policy", "is-enabled")
	if err != nil {
		return err
	}
	var enabled bool
	if err := json.Unmarshal(data, &enabled); err != nil {
		return fmt.Errorf("%s: decode boolean: %w", step, err)
	}
	if enabled != want {
		return fmt.Errorf("%s: is-enabled = %v, want %v", step, enabled, want)
	}
	return nil
}

// user2faScenario exercises the user 2FA commands against the user created by
// this run. The user has no OTP, trusted devices, or security keys, so the
// list commands return empty pages, the deactivate and clear commands succeed
// without changing the login method, and the commands for a missing resource
// ID return the not-found error.
func (s *scenario) user2faScenario(ctx context.Context) error {
	if err := s.checkEmpty2FAPage(ctx, "u2fa-list-trusted-devices", "list-trusted-devices"); err != nil {
		return err
	}
	if err := s.checkEmpty2FAPage(ctx, "u2fa-list-security-keys", "list-security-keys"); err != nil {
		return err
	}
	if _, err := s.call(ctx, "u2fa-deactivate-otp", "iam-api", "user-2fa", "deactivate-otp", "--user-id", s.user.id); err != nil {
		return err
	}
	if _, err := s.call(ctx, "u2fa-clear-trusted-devices", "iam-api", "user-2fa", "clear-trusted-devices", "--user-id", s.user.id); err != nil {
		return err
	}
	if err := s.expectNotFound(ctx, "u2fa-read-missing-security-key", "iam-api", "user-2fa", "read-security-key", "--user-id", s.user.id, missingTwoFactorResourceID); err != nil {
		return err
	}
	if err := s.expectNotFound(ctx, "u2fa-delete-missing-trusted-device", "iam-api", "user-2fa", "delete-trusted-device", "--user-id", s.user.id, missingTwoFactorResourceID); err != nil {
		return err
	}
	return s.expectNotFound(ctx, "u2fa-delete-missing-security-key", "iam-api", "user-2fa", "delete-security-key", "--user-id", s.user.id, missingTwoFactorResourceID)
}

// checkEmpty2FAPage lists the 2FA resources of the user and verifies that the
// page is empty.
func (s *scenario) checkEmpty2FAPage(ctx context.Context, step, subcommand string) error {
	data, err := s.call(ctx, step, "iam-api", "user-2fa", subcommand, "--user-id", s.user.id)
	if err != nil {
		return err
	}
	var page twoFactorPage
	if err := json.Unmarshal(data, &page); err != nil {
		return fmt.Errorf("%s: decode page: %w", step, err)
	}
	if page.Count != 0 || len(page.Items) != 0 {
		return fmt.Errorf("%s: items = %d, count = %d, want an empty page", step, len(page.Items), page.Count)
	}
	return nil
}

// expectNotFound verifies that the command fails with the 404 not-found
// classification instead of succeeding silently.
func (s *scenario) expectNotFound(ctx context.Context, step string, args ...string) error {
	err := s.client.callExpectError(ctx, step, args...)
	if err == nil {
		return fmt.Errorf("%s: expected a not-found error, but the command succeeded", step)
	}
	if !strings.Contains(err.Error(), "404") {
		return fmt.Errorf("%s: error = %v, want the 404 not-found classification", step, err)
	}
	return nil
}

type twoFactorPage struct {
	Count    int               `json:"count"`
	Items    []json.RawMessage `json:"items"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
}

func mustSettingsJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(data)
}
