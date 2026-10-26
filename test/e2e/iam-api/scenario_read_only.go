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

// readOnlyScenario exercises the read-only iam-api commands that do not depend
// on resources created by this run. Every command only reads data, so the
// organization-wide configuration stays untouched.
func (s *scenario) readOnlyScenario(ctx context.Context) error {
	if err := s.checkRoleCatalog(ctx, "iam-role"); err != nil {
		return err
	}
	if err := s.checkRoleCatalog(ctx, "id-role"); err != nil {
		return err
	}
	if _, err := s.readPolicyBindings(ctx, "id-policy-read-organization", "id-policy", "read-organization", ""); err != nil {
		return err
	}
	if err := s.checkAuthContext(ctx); err != nil {
		return err
	}
	if err := s.checkAuthConditions(ctx); err != nil {
		return err
	}
	if err := s.checkPasswordPolicy(ctx); err != nil {
		return err
	}
	if _, err := s.readPolicyBindings(ctx, "policy-read-organization", "policy", "read-organization", ""); err != nil {
		return err
	}
	if err := s.checkOrganization(ctx); err != nil {
		return err
	}
	return s.checkServicePolicy(ctx)
}

// readResourcePolicies reads the policy bindings of the folder and project
// created by this run. The bindings may be empty because this run does not
// grant any role to the temporary resources.
func (s *scenario) readResourcePolicies(ctx context.Context) error {
	if _, err := s.readPolicyBindings(ctx, "policy-read-folder", "policy", "read-folder", s.folder.id); err != nil {
		return err
	}
	if _, err := s.readPolicyBindings(ctx, "policy-read-project", "policy", "read-project", s.project.id); err != nil {
		return err
	}
	return nil
}

// checkRoleCatalog lists the available roles and reads the first one.
// kind is iam-role or id-role.
func (s *scenario) checkRoleCatalog(ctx context.Context, kind string) error {
	listStep := kind + "-list"
	data, err := s.call(ctx, listStep, "iam-api", kind, "list")
	if err != nil {
		return err
	}
	var list roleList
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("%s: decode roles: %w", listStep, err)
	}
	if len(list.Items) == 0 {
		return fmt.Errorf("%s: the role catalog is empty", listStep)
	}
	first := list.Items[0]
	if first.ID == "" || first.Name == "" {
		return fmt.Errorf("%s: the first role is missing id or name", listStep)
	}
	readStep := kind + "-read"
	readData, err := s.call(ctx, readStep, "iam-api", kind, "read", first.ID)
	if err != nil {
		return err
	}
	var read roleSummary
	if err := json.Unmarshal(readData, &read); err != nil {
		return fmt.Errorf("%s: decode role: %w", readStep, err)
	}
	if read.ID != first.ID || read.Name != first.Name {
		return fmt.Errorf("%s read mismatch: got %q/%q, want %q/%q", kind, read.ID, read.Name, first.ID, first.Name)
	}
	return nil
}

// readPolicyBindings reads IAM policy bindings for the organization, folder,
// or project. resourceID is empty for the organization.
func (s *scenario) readPolicyBindings(ctx context.Context, step, kind, subcommand, resourceID string) ([]policyBinding, error) {
	args := []string{"iam-api", kind, subcommand}
	if resourceID != "" {
		args = append(args, resourceID)
	}
	data, err := s.call(ctx, step, args...)
	if err != nil {
		return nil, err
	}
	var bindings []policyBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("%s: decode policy bindings: %w", step, err)
	}
	return bindings, nil
}

func (s *scenario) checkAuthContext(ctx context.Context) error {
	data, err := s.call(ctx, "read-auth-context", "iam-api", "auth", "read-auth-context")
	if err != nil {
		return err
	}
	var context authContext
	if err := json.Unmarshal(data, &context); err != nil {
		return fmt.Errorf("read-auth-context: decode authentication context: %w", err)
	}
	if context.ResourceID <= 0 {
		return fmt.Errorf("read-auth-context: resource_id = %d, want a positive value", context.ResourceID)
	}
	if context.AuthType == "" {
		return fmt.Errorf("read-auth-context: auth_type is empty")
	}
	s.principalID = context.ResourceID
	if context.LimitedToProjectID != nil {
		s.limitedProjectID = *context.LimitedToProjectID
	}
	return nil
}

func (s *scenario) checkAuthConditions(ctx context.Context) error {
	data, err := s.call(ctx, "read-auth-conditions", "iam-api", "auth", "read-auth-conditions")
	if err != nil {
		return err
	}
	var conditions authConditions
	if err := json.Unmarshal(data, &conditions); err != nil {
		return fmt.Errorf("read-auth-conditions: decode authentication conditions: %w", err)
	}
	if conditions.IPRestriction.Mode == "" {
		return fmt.Errorf("read-auth-conditions: ip_restriction.mode is empty")
	}
	return nil
}

func (s *scenario) checkPasswordPolicy(ctx context.Context) error {
	data, err := s.call(ctx, "read-password-policy", "iam-api", "auth", "read-password-policy")
	if err != nil {
		return err
	}
	var policy passwordPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("read-password-policy: decode password policy: %w", err)
	}
	if policy.MinLength <= 0 {
		return fmt.Errorf("read-password-policy: min_length = %d, want a positive value", policy.MinLength)
	}
	return nil
}

func (s *scenario) checkOrganization(ctx context.Context) error {
	data, err := s.call(ctx, "organization-read", "iam-api", "organization", "read")
	if err != nil {
		return err
	}
	var organization organizationInfo
	if err := json.Unmarshal(data, &organization); err != nil {
		return fmt.Errorf("organization-read: decode organization: %w", err)
	}
	if organization.ID <= 0 {
		return fmt.Errorf("organization-read: id = %d, want a positive value", organization.ID)
	}
	return nil
}

func (s *scenario) checkServicePolicy(ctx context.Context) error {
	data, err := s.call(ctx, "service-policy-is-enabled", "iam-api", "service-policy", "is-enabled")
	if err != nil {
		return err
	}
	var enabled bool
	if err := json.Unmarshal(data, &enabled); err != nil {
		return fmt.Errorf("service-policy-is-enabled: decode boolean: %w", err)
	}
	data, err = s.call(ctx, "read-service-policy", "iam-api", "organization", "read-service-policy")
	if err != nil {
		return err
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("read-service-policy: decode rules: %w", err)
	}
	data, err = s.call(ctx, "list-rule-templates", "iam-api", "service-policy", "list-rule-templates")
	if err != nil {
		return err
	}
	var templates ruleTemplateList
	if err := json.Unmarshal(data, &templates); err != nil {
		return fmt.Errorf("list-rule-templates: decode rule templates: %w", err)
	}
	if len(templates.Items) == 0 {
		return fmt.Errorf("list-rule-templates: the rule template catalog is empty")
	}
	return nil
}
