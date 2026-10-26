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
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

const (
	fakeChildFolderID    = 3002
	fakeScimID           = "c0010000-0000-4000-8000-000000000001"
	fakeSSOID            = 6001
	fakeAPIKeyID         = 7001
	fakeSPKeyID          = "6a2b0000-0000-4000-8000-000000000001"
	fakeSPKeyKid         = "fake-kid-1"
	fakeLimitedProjectID = 113700055789
)

type fakeCLI struct {
	users             map[string]item
	groups            map[string]item
	folders           map[string]item
	projects          map[string]item
	servicePrincipals map[string]item
	listPages         map[string][][]item
	listPageCounts    map[string][]int
	listHasNext       map[string][]bool
	calls             []string
	argsByStep        map[string][]string
	failAt            string
	failAfterApply    string

	// knobs for the read-only scenario responses
	emptyRoleCatalog   bool
	iamRoleReadID      string
	idRoleReadID       string
	authResourceID     int64
	passwordMinLength  int
	organizationID     int64
	brokenPolicyJSON   bool
	emptyRuleTemplates bool

	// state for the managed resource scenarios
	scimName     string
	scimDeleted  bool
	ssoName      string
	ssoDeleted   bool
	pakName      string
	pakDeleted   bool
	spKeyID      string
	spKeyKid     string
	spKeyStatus  string
	spKeyDeleted bool

	folderPolicyBindings  []policyBinding
	projectPolicyBindings []policyBinding

	// lastPakRequest records the parsed create-project-api-key request body.
	lastPakRequest map[string]any

	// knob for the hierarchy scenario parent verification
	childFolderParentID int64

	// state for the account scenario
	orgName                    string
	servicePolicyEnabled       bool
	servicePolicyRules         json.RawMessage
	idPolicyBindings           []policyBinding
	organizationPolicyBindings []policyBinding
}

const (
	fakeMainFolderID           = 3001
	fakeMainProjectID          = 4001
	fakeMainServicePrincipalID = 5001
)

func newFakeCLI() *fakeCLI {
	return &fakeCLI{
		users:             make(map[string]item),
		groups:            make(map[string]item),
		folders:           make(map[string]item),
		projects:          make(map[string]item),
		servicePrincipals: make(map[string]item),

		folderPolicyBindings:  []policyBinding{},
		projectPolicyBindings: []policyBinding{fakeServicePrincipalBinding("owner", 113702071936)},
		childFolderParentID:   fakeMainFolderID,

		servicePolicyRules:         json.RawMessage("[]"),
		idPolicyBindings:           []policyBinding{fakeBinding("identity-admin", 1001)},
		organizationPolicyBindings: []policyBinding{fakeBinding("identity-admin", 1001)},
	}
}

func (f *fakeCLI) callExpectError(ctx context.Context, step string, args ...string) error {
	_, err := f.call(ctx, step, args...)
	return err
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.argsByStep == nil {
		f.argsByStep = make(map[string][]string)
	}
	f.argsByStep[step] = append([]string(nil), args...)
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	if step == f.failAt && step != "create-group" {
		return nil, errFail(step)
	}
	if response, responseErr, ok := f.readOnlyResponse(step, args); ok {
		return response, responseErr
	}
	if response, responseErr, ok := f.managedResponse(step, args); ok {
		return response, responseErr
	}
	if response, responseErr, ok := f.accountResponse(step, args); ok {
		return response, responseErr
	}
	if len(args) >= 3 && args[0] == "iam-api" && args[2] == "list" {
		page := 1
		if value := argument(args, "--page"); value != "" {
			var err error
			page, err = strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
		}
		return f.list(args[1], page)
	}
	switch {
	case step == "create-user":
		name := argument(args, "--name")
		code := argument(args, "--code")
		if code == "" {
			code = name
		}
		passwordFile := argument(args, "--password-file")
		if passwordFile == "" {
			return nil, errInvalid("create-user requires --password-file")
		}
		//nolint:gosec // The path is created by this test in t.TempDir().
		password, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(password), "Sk9") {
			return nil, errInvalid("create-user requires the E2E password file contents")
		}
		created := item{ID: 1001, Name: name, Code: code, Description: argument(args, "--description")}
		f.users[code] = created
		return json.Marshal(created)
	case step == "create-group":
		created := item{ID: 2001, Name: argument(args, "--name"), Description: argument(args, "--description")}
		f.groups[created.Name] = created
		if step == f.failAt {
			return nil, errFail(step)
		}
		return json.Marshal(created)
	case step == "create-folder":
		created := item{ID: 3001, Name: argument(args, "--name"), Description: argument(args, "--description")}
		f.folders[created.Name] = created
		return json.Marshal(created)
	case step == "create-project":
		created := item{
			ID:          4001,
			Name:        argument(args, "--name"),
			Code:        argument(args, "--code"),
			Description: argument(args, "--description"),
		}
		f.projects[created.Name] = created
		return json.Marshal(created)
	case step == "create-service-principal":
		projectID, err := strconv.Atoi(argument(args, "--project-id"))
		if err != nil {
			return nil, err
		}
		created := item{
			ID:          5001,
			Name:        argument(args, "--name"),
			Description: argument(args, "--description"),
			ProjectID:   projectID,
		}
		f.servicePrincipals[created.Name] = created
		return json.Marshal(created)
	case step == "read-user", step == "verify-email", step == "verify-email-unregistered":
		return f.readOne(f.users, lastArg(args))
	case step == "read-group":
		return f.readOne(f.groups, lastArg(args))
	case step == "read-folder":
		return f.readOne(f.folders, lastArg(args))
	case step == "read-project":
		return f.readOne(f.projects, lastArg(args))
	case step == "read-service-principal":
		return f.readOne(f.servicePrincipals, lastArg(args))
	case step == "update-user":
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.users[current.Code] = current
		if step == f.failAfterApply {
			return nil, errFail(step)
		}
		return json.Marshal(current)
	case step == "update-folder":
		current, ok := f.findByID(f.folders, args[3])
		if !ok {
			return nil, errInvalid("folder not found: " + args[3])
		}
		delete(f.folders, current.Name)
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.folders[current.Name] = current
		if step == f.failAfterApply {
			return nil, errFail(step)
		}
		return json.Marshal(current)
	case step == "update-project":
		current, ok := f.findByID(f.projects, args[3])
		if !ok {
			return nil, errInvalid("project not found: " + args[3])
		}
		delete(f.projects, current.Name)
		current.Name = argument(args, "--name")
		current.Description = argument(args, "--description")
		f.projects[current.Name] = current
		if step == f.failAfterApply {
			return nil, errFail(step)
		}
		return json.Marshal(current)
	case step == "register-email":
		email := argument(args, "--email")
		if email == "" {
			return nil, errInvalid("register-email requires --email")
		}
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Email = email
		f.users[current.Code] = current
		return nil, nil
	case step == "unregister-email":
		current, ok := f.findByID(f.users, args[3])
		if !ok {
			return nil, errInvalid("user not found: " + args[3])
		}
		current.Email = ""
		f.users[current.Code] = current
		return nil, nil
	case step == "update-memberships", step == "read-memberships":
		return json.Marshal([]map[string]int{{"id": 1001}})
	case strings.HasPrefix(step, "cleanup-read-"):
		kind := strings.TrimPrefix(step, "cleanup-read-")
		switch kind {
		case "user":
			return f.readOne(f.users, lastArg(args))
		case "group":
			return f.readOne(f.groups, lastArg(args))
		case "folder":
			return f.readOne(f.folders, lastArg(args))
		case "project":
			return f.readOne(f.projects, lastArg(args))
		case "service-principal":
			return f.readOne(f.servicePrincipals, lastArg(args))
		}
		return nil, errInvalid("unknown cleanup resource " + kind)
	case strings.HasPrefix(step, "cleanup-delete-"):
		kind := strings.TrimPrefix(step, "cleanup-delete-")
		id := lastArg(args)
		switch kind {
		case "user":
			current, ok := f.findByID(f.users, id)
			if !ok {
				return nil, errInvalid("user not found: " + id)
			}
			delete(f.users, current.Code)
			return nil, nil
		case "group":
			current, ok := f.findByID(f.groups, id)
			if !ok {
				return nil, errInvalid("group not found: " + id)
			}
			delete(f.groups, current.Name)
			return nil, nil
		case "folder":
			current, ok := f.findByID(f.folders, id)
			if !ok {
				return nil, errInvalid("folder not found: " + id)
			}
			delete(f.folders, current.Name)
			return nil, nil
		case "project":
			current, ok := f.findByID(f.projects, id)
			if !ok {
				return nil, errInvalid("project not found: " + id)
			}
			delete(f.projects, current.Name)
			return nil, nil
		case "service-principal":
			current, ok := f.findByID(f.servicePrincipals, id)
			if !ok {
				return nil, errInvalid("service principal not found: " + id)
			}
			delete(f.servicePrincipals, current.Name)
			return nil, nil
		}
		return nil, errInvalid("unknown cleanup resource " + kind)
	default:
		return nil, errInvalid("unexpected E2E step " + step)
	}
}

// readOnlyResponse serves the fixed read-only iam-api responses. The third
// return value reports whether the step belongs to the read-only scenario.
func (f *fakeCLI) readOnlyResponse(step string, args []string) ([]byte, error, bool) {
	switch step {
	case "iam-role-list":
		if f.emptyRoleCatalog {
			return mustRoleJSON(roleList{Items: []roleSummary{}}), nil, true
		}
		return mustRoleJSON(roleList{Items: fakeRoles()}), nil, true
	case "iam-role-read":
		role := fakeRoles()[0]
		if f.iamRoleReadID != "" {
			role.ID = f.iamRoleReadID
		}
		if lastArg(args) != "addon-admin" {
			return nil, errInvalid("iam role not found: " + lastArg(args)), true
		}
		return mustRoleJSON(role), nil, true
	case "id-role-list":
		return mustRoleJSON(roleList{Items: fakeIDRoles()}), nil, true
	case "id-role-read":
		role := fakeIDRoles()[0]
		if f.idRoleReadID != "" {
			role.ID = f.idRoleReadID
		}
		if lastArg(args) != "identity-admin" {
			return nil, errInvalid("id role not found: " + lastArg(args)), true
		}
		return mustRoleJSON(role), nil, true
	case "id-policy-read-organization", "policy-read-organization":
		if f.brokenPolicyJSON {
			return []byte("not-json"), nil, true
		}
		return mustRoleJSON([]policyBinding{fakeBinding("identity-admin", 1001)}), nil, true
	case "policy-read-folder", "policy-read-project":
		if f.brokenPolicyJSON {
			return []byte("not-json"), nil, true
		}
		return mustRoleJSON([]policyBinding{}), nil, true
	case "read-auth-context":
		context := authContext{
			ResourceID:         113702071936,
			AuthType:           "service_principal",
			LimitedToProjectID: int64Ptr(113700055789),
			MemberCode:         "hkp83875",
		}
		if f.authResourceID != 0 {
			context.ResourceID = f.authResourceID
		}
		return mustRoleJSON(context), nil, true
	case "read-auth-conditions":
		return mustRoleJSON(authConditions{IPRestriction: ipRestriction{Mode: "allow_all"}}), nil, true
	case "read-password-policy":
		minLength := 8
		if f.passwordMinLength != 0 {
			minLength = f.passwordMinLength
		}
		return mustRoleJSON(passwordPolicy{MinLength: minLength}), nil, true
	case "organization-read":
		organization := organizationInfo{ID: 113701027680}
		if f.organizationID != 0 {
			organization.ID = f.organizationID
		}
		return mustRoleJSON(organization), nil, true
	case "read-service-policy":
		return mustRoleJSON([]json.RawMessage{}), nil, true
	case "service-policy-is-enabled":
		return []byte("false\n"), nil, true
	case "list-rule-templates":
		if f.emptyRuleTemplates {
			return mustRoleJSON(ruleTemplateList{Items: []ruleTemplateSummary{}}), nil, true
		}
		return mustRoleJSON(ruleTemplateList{Items: []ruleTemplateSummary{{
			Code:           "billing-restrict-discount-passport",
			Name:           "Discount passport purchase prohibited",
			Description:    "Rule template that prohibits purchasing discount passports",
			Type:           "boolean",
			SupportsDryRun: true,
		}}}), nil, true
	default:
		return nil, nil, false
	}
}

func (f *fakeCLI) managedResponse(step string, args []string) ([]byte, error, bool) {
	switch step {
	case "create-scim":
		f.scimName = argument(args, "--name")
		return mustRoleJSON(map[string]any{"id": fakeScimID, "name": f.scimName}), nil, true
	case "read-scim":
		return mustRoleJSON(map[string]any{
			"id":       fakeScimID,
			"name":     f.scimName,
			"base_url": "https://secure.sakura.ad.jp/cloud/api/iam/1.0/scim/" + fakeScimID + "/v2/",
		}), nil, true
	case "update-scim":
		f.scimName = argument(args, "--name")
		return mustRoleJSON(map[string]any{"id": fakeScimID, "name": f.scimName}), nil, true
	case "list-scim":
		items := []map[string]string{}
		if !f.scimDeleted && f.scimName != "" {
			items = append(items, map[string]string{"id": fakeScimID, "name": f.scimName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "regenerate-scim-token":
		// The scenario projects the secret token to its length with --query,
		// so the fake serves the projected shape.
		return mustRoleJSON(map[string]any{"token_length": len("fake-secret-token")}), nil, true
	case "delete-scim":
		f.scimDeleted = true
		return nil, nil, true
	case "create-sso":
		f.ssoName = readRequestName(args)
		return mustRoleJSON(map[string]any{"id": fakeSSOID, "name": f.ssoName}), nil, true
	case "read-sso":
		return mustRoleJSON(map[string]any{
			"id":           strconv.Itoa(fakeSSOID),
			"name":         f.ssoName,
			"description":  f.ssoName,
			"sp_entity_id": "https://secure.sakura.ad.jp/cloud/auth/saml/fake",
			"sp_acs_url":   "https://secure.sakura.ad.jp/cloud/auth/saml/fake/acs",
			"assigned":     false,
		}), nil, true
	case "update-sso":
		f.ssoName = readRequestName(args)
		return mustRoleJSON(map[string]any{"id": fakeSSOID, "name": f.ssoName}), nil, true
	case "list-sso":
		items := []map[string]string{}
		if !f.ssoDeleted && f.ssoName != "" {
			items = append(items, map[string]string{"id": strconv.Itoa(fakeSSOID), "name": f.ssoName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "delete-sso":
		f.ssoDeleted = true
		return nil, nil, true
	case "create-project-api-key":
		f.lastPakRequest = readRequestBody(args)
		if projectID := readRequestProjectID(args); projectID != fakeMainProjectID && projectID != fakeLimitedProjectID {
			return nil, errInvalid("create-project-api-key pins the key to an unexpected project"), true
		}
		f.pakName = readRequestName(args)
		return mustRoleJSON(map[string]any{
			"id":        strconv.Itoa(fakeAPIKeyID),
			"name":      f.pakName,
			"iam_roles": []string{projectAPIKeyRole},
		}), nil, true
	case "read-project-api-key":
		return mustRoleJSON(map[string]any{
			"id":        strconv.Itoa(fakeAPIKeyID),
			"name":      f.pakName,
			"iam_roles": []string{projectAPIKeyRole},
		}), nil, true
	case "update-project-api-key":
		f.pakName = readRequestName(args)
		return mustRoleJSON(map[string]any{
			"id":        strconv.Itoa(fakeAPIKeyID),
			"name":      f.pakName,
			"iam_roles": []string{projectAPIKeyRole},
		}), nil, true
	case "list-project-api-keys":
		items := []map[string]string{}
		if !f.pakDeleted && f.pakName != "" {
			items = append(items, map[string]string{"id": strconv.Itoa(fakeAPIKeyID), "name": f.pakName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "delete-project-api-key":
		f.pakDeleted = true
		return nil, nil, true
	case "cleanup-find-scim", "cleanup-verify-scim":
		items := []map[string]string{}
		if !f.scimDeleted && f.scimName != "" {
			items = append(items, map[string]string{"id": fakeScimID, "name": f.scimName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "cleanup-read-scim":
		return mustRoleJSON(map[string]any{"id": fakeScimID, "name": f.scimName}), nil, true
	case "cleanup-delete-scim":
		f.scimDeleted = true
		return nil, nil, true
	case "cleanup-find-sso", "cleanup-verify-sso":
		items := []map[string]string{}
		if !f.ssoDeleted && f.ssoName != "" {
			items = append(items, map[string]string{"id": strconv.Itoa(fakeSSOID), "name": f.ssoName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "cleanup-read-sso":
		return mustRoleJSON(map[string]any{"id": strconv.Itoa(fakeSSOID), "name": f.ssoName}), nil, true
	case "cleanup-delete-sso":
		f.ssoDeleted = true
		return nil, nil, true
	case "cleanup-find-project-api-key", "cleanup-verify-project-api-key":
		items := []map[string]string{}
		if !f.pakDeleted && f.pakName != "" {
			items = append(items, map[string]string{"id": strconv.Itoa(fakeAPIKeyID), "name": f.pakName})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "cleanup-read-project-api-key":
		return mustRoleJSON(map[string]any{"id": strconv.Itoa(fakeAPIKeyID), "name": f.pakName}), nil, true
	case "cleanup-delete-project-api-key":
		f.pakDeleted = true
		return nil, nil, true
	case "create-child-folder":
		created := item{ID: fakeChildFolderID, Name: argument(args, "--name"), Description: argument(args, "--description")}
		f.folders[created.Name] = created
		data, err := json.Marshal(created)
		return data, err, true
	case "move-child-folder":
		current, ok := f.findByID(f.folders, strconv.Itoa(fakeChildFolderID))
		if !ok {
			return nil, errInvalid("child folder not found"), true
		}
		parentID := argument(args, "--parent-id")
		if parentID != strconv.Itoa(fakeMainFolderID) {
			return nil, errInvalid("move-child-folder: unexpected parent-id " + parentID), true
		}
		parent := int64(fakeMainFolderID)
		current.ProjectID = int(parent)
		f.folders[current.Name] = current
		return nil, nil, true
	case "read-child-folder":
		current, ok := f.findByID(f.folders, strconv.Itoa(fakeChildFolderID))
		if !ok {
			return nil, errInvalid("child folder not found"), true
		}
		parent := f.childFolderParentID
		return mustRoleJSON(map[string]any{
			"id":        current.ID,
			"name":      current.Name,
			"parent_id": &parent,
		}), nil, true
	case "move-project":
		_, ok := f.findByID(f.projects, strconv.Itoa(fakeMainProjectID))
		if !ok {
			return nil, errInvalid("project not found"), true
		}
		parentID := argument(args, "--parent-folder-id")
		if parentID != strconv.Itoa(fakeChildFolderID) {
			return nil, errInvalid("move-project: unexpected parent-folder-id " + parentID), true
		}
		return nil, nil, true
	case "read-project-moved":
		current, ok := f.findByID(f.projects, strconv.Itoa(fakeMainProjectID))
		if !ok {
			return nil, errInvalid("project not found"), true
		}
		parent := int64(fakeChildFolderID)
		return mustRoleJSON(map[string]any{
			"id":               current.ID,
			"name":             current.Name,
			"parent_folder_id": &parent,
		}), nil, true
	case "read-folder-policy-before":
		return mustRoleJSON(f.folderPolicyBindings), nil, true
	case "read-folder-policy-updated", "read-folder-policy-restored":
		return mustRoleJSON(f.folderPolicyBindings), nil, true
	case "read-project-policy-before":
		return mustRoleJSON(f.projectPolicyBindings), nil, true
	case "read-project-policy-updated", "read-project-policy-restored":
		return mustRoleJSON(f.projectPolicyBindings), nil, true
	case "update-folder-policy":
		f.folderPolicyBindings = parsePolicyBindings(args)
		return nil, nil, true
	case "update-project-policy":
		f.projectPolicyBindings = parsePolicyBindings(args)
		return nil, nil, true
	case "restore-folder-policy":
		f.folderPolicyBindings = parsePolicyBindings(args)
		return nil, nil, true
	case "restore-project-policy":
		f.projectPolicyBindings = parsePolicyBindings(args)
		return nil, nil, true
	case "update-service-principal", "restore-service-principal":
		name := readRequestName(args)
		current, ok := f.findByID(f.servicePrincipals, strconv.Itoa(fakeMainServicePrincipalID))
		if !ok {
			return nil, errInvalid("service principal not found"), true
		}
		delete(f.servicePrincipals, current.Name)
		current.Name = name
		f.servicePrincipals[name] = current
		return mustRoleJSON(map[string]any{"id": current.ID, "name": current.Name}), nil, true
	case "upload-key":
		f.spKeyID = fakeSPKeyID
		f.spKeyKid = fakeSPKeyKid
		f.spKeyStatus = "enabled"
		return mustRoleJSON(map[string]any{"id": f.spKeyID, "kid": f.spKeyKid, "status": f.spKeyStatus}), nil, true
	case "list-keys", "list-keys-after-delete":
		items := []map[string]any{}
		if !f.spKeyDeleted && f.spKeyID != "" {
			items = append(items, map[string]any{"id": f.spKeyID, "kid": f.spKeyKid, "status": f.spKeyStatus})
		}
		return mustRoleJSON(map[string]any{"items": items}), nil, true
	case "disable-key":
		f.spKeyStatus = "disabled"
		return mustRoleJSON(map[string]any{"id": f.spKeyID, "kid": f.spKeyKid, "status": f.spKeyStatus}), nil, true
	case "enable-key":
		f.spKeyStatus = "enabled"
		return mustRoleJSON(map[string]any{"id": f.spKeyID, "kid": f.spKeyKid, "status": f.spKeyStatus}), nil, true
	case "issue-token":
		return mustRoleJSON(map[string]any{"token_type": "Bearer", "expires_in": 3600}), nil, true
	case "delete-key":
		f.spKeyDeleted = true
		return nil, nil, true
	}
	return nil, nil, false
}

func parsePolicyBindings(args []string) []policyBinding {
	request := argument(args, "--request")
	var bindings []policyBinding
	if err := json.Unmarshal([]byte(request), &bindings); err != nil {
		return nil
	}
	return bindings
}

// accountResponse serves the organization-scoped write steps of the account
// and user 2FA scenarios. The scenario updates each setting with the values it
// read before, so the fake stores the request and serves it back on the
// verify read.
func (f *fakeCLI) accountResponse(step string, args []string) ([]byte, error, bool) {
	switch step {
	case "account-read-password-policy":
		minLength := 8
		if f.passwordMinLength != 0 {
			minLength = f.passwordMinLength
		}
		return mustRoleJSON(passwordPolicy{MinLength: minLength}), nil, true
	case "account-update-password-policy":
		request := argument(args, "--request")
		var policy passwordPolicy
		if err := json.Unmarshal([]byte(request), &policy); err != nil {
			return nil, errInvalid("account-update-password-policy: invalid request JSON"), true
		}
		if policy.MinLength <= 0 {
			return nil, errInvalid("account-update-password-policy: min_length must be positive"), true
		}
		return []byte(request), nil, true
	case "account-read-auth-conditions":
		return mustRoleJSON(map[string]any{
			"ip_restriction":          map[string]any{"mode": "allow_all"},
			"require_two_factor_auth": map[string]any{"enabled": false},
			"datetime_restriction":    map[string]any{"after": nil, "before": nil},
		}), nil, true
	case "account-update-auth-conditions":
		request := argument(args, "--request")
		var conditions authConditions
		if err := json.Unmarshal([]byte(request), &conditions); err != nil {
			return nil, errInvalid("account-update-auth-conditions: invalid request JSON"), true
		}
		if conditions.IPRestriction.Mode == "" {
			return nil, errInvalid("account-update-auth-conditions: ip_restriction.mode is required"), true
		}
		return []byte(request), nil, true
	case "account-read-id-policy", "account-verify-id-policy":
		return mustRoleJSON(f.idPolicyBindings), nil, true
	case "account-update-id-policy":
		f.idPolicyBindings = parsePolicyBindings(args)
		if f.idPolicyBindings == nil {
			f.idPolicyBindings = []policyBinding{}
		}
		return mustRoleJSON(f.idPolicyBindings), nil, true
	case "account-read-organization-policy", "account-verify-organization-policy":
		return mustRoleJSON(f.organizationPolicyBindings), nil, true
	case "account-update-organization-policy":
		f.organizationPolicyBindings = parsePolicyBindings(args)
		if f.organizationPolicyBindings == nil {
			f.organizationPolicyBindings = []policyBinding{}
		}
		return mustRoleJSON(f.organizationPolicyBindings), nil, true
	case "account-read-organization":
		organizationID := int64(113701027680)
		if f.organizationID != 0 {
			organizationID = f.organizationID
		}
		return mustRoleJSON(organizationInfo{ID: organizationID, Name: f.orgName}), nil, true
	case "account-update-organization":
		organizationID := int64(113701027680)
		if f.organizationID != 0 {
			organizationID = f.organizationID
		}
		f.orgName = argument(args, "--name")
		return mustRoleJSON(organizationInfo{ID: organizationID, Name: f.orgName}), nil, true
	case "account-read-service-policy":
		return mustRoleJSON(f.servicePolicyRules), nil, true
	case "account-enable-service-policy":
		f.servicePolicyEnabled = true
		return nil, nil, true
	case "account-disable-service-policy", "account-restore-service-policy-disabled":
		f.servicePolicyEnabled = false
		return nil, nil, true
	case "account-verify-service-policy-enabled":
		if !f.servicePolicyEnabled {
			return nil, errInvalid("account-verify-service-policy-enabled: the service policy is not enabled"), true
		}
		return []byte("true\n"), nil, true
	case "account-verify-service-policy-disabled":
		if f.servicePolicyEnabled {
			return nil, errInvalid("account-verify-service-policy-disabled: the service policy is enabled"), true
		}
		return []byte("false\n"), nil, true
	case "account-update-service-policy":
		// The rules can only be updated while the policy is enabled.
		if !f.servicePolicyEnabled {
			return nil, errInvalid("account-update-service-policy: API Error 409 - サービスポリシーが有効化されていません"), true
		}
		request := argument(args, "--request")
		var rules []json.RawMessage
		if err := json.Unmarshal([]byte(request), &rules); err != nil {
			return nil, errInvalid("account-update-service-policy: invalid request JSON"), true
		}
		f.servicePolicyRules = json.RawMessage(request)
		return []byte(request), nil, true
	case "u2fa-list-trusted-devices", "u2fa-list-security-keys":
		return mustRoleJSON(map[string]any{
			"items": []json.RawMessage{}, "count": 0, "next": nil, "previous": nil,
		}), nil, true
	case "u2fa-deactivate-otp", "u2fa-clear-trusted-devices":
		if argument(args, "--user-id") == "" {
			return nil, errInvalid(step + " requires --user-id"), true
		}
		return nil, nil, true
	case "u2fa-read-missing-security-key", "u2fa-delete-missing-trusted-device", "u2fa-delete-missing-security-key":
		id := lastArg(args)
		if id != missingTwoFactorResourceID {
			return nil, errInvalid(step + ": unexpected resource ID " + id), true
		}
		return nil, errInvalid(step + ": API Error 404 - 見つかりませんでした。"), true
	}
	return nil, nil, false
}

func readRequestBody(args []string) map[string]any {
	request := argument(args, "--request")
	if strings.HasPrefix(request, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(request, "@"))
		if err != nil {
			return nil
		}
		request = string(data)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(request), &body); err != nil {
		return nil
	}
	return body
}

func readRequestName(args []string) string {
	if body := readRequestBody(args); body != nil {
		if name, ok := body["name"].(string); ok {
			return name
		}
	}
	return ""
}

func readRequestProjectID(args []string) int {
	if body := readRequestBody(args); body != nil {
		if id, ok := body["project_id"].(float64); ok {
			return int(id)
		}
	}
	return 0
}

func fakeRoles() []roleSummary {
	return []roleSummary{
		{
			ID:                      "addon-admin",
			Name:                    "Add-on service",
			Description:             "Full add-on operations",
			Category:                "addon",
			LowestGrantableResource: "project",
		},
	}
}

func fakeIDRoles() []roleSummary {
	return []roleSummary{
		{ID: "identity-admin", Name: "ID administrator", Description: "Full identity operations"},
	}
}

func fakeBinding(roleID string, principalID int64) policyBinding {
	return policyBinding{
		Role:       policyRole{Type: "preset", ID: roleID},
		Principals: []policyPrincipal{{Type: "user", ID: principalID}},
	}
}

func fakeServicePrincipalBinding(roleID string, principalID int64) policyBinding {
	return policyBinding{
		Role:       policyRole{Type: "preset", ID: roleID},
		Principals: []policyPrincipal{{Type: "service-principal", ID: principalID}},
	}
}

func int64Ptr(value int64) *int64 { return &value }

func mustRoleJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func (f *fakeCLI) list(kind string, page int) ([]byte, error) {
	if pages, ok := f.listPages[kind]; ok {
		if page > 0 && page <= len(pages) {
			items := pages[page-1]
			if items == nil {
				items = []item{}
			}
			pageCount := len(items)
			if counts, ok := f.listPageCounts[kind]; ok && page <= len(counts) {
				pageCount = counts[page-1]
			}
			var hasNext *bool
			if values, ok := f.listHasNext[kind]; ok && page <= len(values) {
				hasNext = &values[page-1]
			}
			return json.Marshal(projectedPage{Items: items, PageCount: pageCount, HasNext: hasNext})
		}
		return json.Marshal(projectedPage{Items: []item{}})
	}
	if page != 1 {
		return json.Marshal(projectedPage{Items: []item{}})
	}
	switch kind {
	case "user":
		items := mapValues(f.users)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "group":
		items := mapValues(f.groups)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "folder":
		items := mapValues(f.folders)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "project":
		items := mapValues(f.projects)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	case "service-principal":
		items := mapValues(f.servicePrincipals)
		return json.Marshal(projectedPage{Items: items, PageCount: len(items)})
	default:
		return nil, errInvalid("unknown resource kind " + kind)
	}
}

func (f *fakeCLI) readOne(values map[string]item, id string) ([]byte, error) {
	current, ok := f.findByID(values, id)
	if !ok {
		return nil, errInvalid("resource not found: " + id)
	}
	return json.Marshal(current)
}

func (f *fakeCLI) findByID(values map[string]item, id string) (item, bool) {
	for _, value := range values {
		if strconv.Itoa(value.ID) == id {
			return value, true
		}
	}
	return item{}, false
}

func mapValues(values map[string]item) []item {
	result := make([]item, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}

func argument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

type stepError struct{ message string }

func (e *stepError) Error() string { return e.message }

func errFail(step string) error {
	return &stepError{message: "injected CLI failure at " + step}
}

func errInvalid(message string) error {
	return &stepError{message: message}
}

func testScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	suffix := "test00ff"
	name := "skr-e2e-iam-" + suffix
	//nolint:gosec // The scenario uses a fake password only in an in-memory CLI test.
	return &scenario{
		client:   fake,
		password: "Sk9-Password-Test-0000000000",
		suffix:   suffix,
		user:     resource{kind: "user", name: name, description: "skr-e2e-iam/" + suffix},
		group:    resource{kind: "group", name: name, description: "skr-e2e-iam/" + suffix},
		folder:   resource{kind: "folder", name: name, description: "skr-e2e-iam/" + suffix},
		childFolder: resource{
			kind: "folder", name: name + "-child", description: "skr-e2e-iam/" + suffix,
		},
		project: resource{kind: "project", name: name, code: "e2e" + suffix, description: "skr-e2e-iam/" + suffix},
		servicePrincipal: resource{
			kind: "service-principal", name: name, description: "skr-e2e-iam/" + suffix,
		},
		scim: managedResource{
			kind: "scim", name: name + "-scim",
		},
		ssoProfile: managedResource{
			kind: "sso", name: name + "-sso",
		},
		projectAPIKey: managedResource{
			kind: "project-api-key", name: name + "-pak",
		},
		email:   "skr-e2e-iam@example.com",
		renamed: name + "-renamed",
	}
}

func TestScenarioCreatesAndCleansUpResources(t *testing.T) {
	fake := newFakeCLI()
	s := testScenario(t, fake)
	if err := s.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.users) != 0 || len(fake.groups) != 0 || len(fake.folders) != 0 || len(fake.projects) != 0 || len(fake.servicePrincipals) != 0 {
		t.Fatalf("resources remain after E2E: users=%+v groups=%+v folders=%+v projects=%+v servicePrincipals=%+v",
			fake.users, fake.groups, fake.folders, fake.projects, fake.servicePrincipals)
	}
	for _, want := range []string{
		"preflight-users-page-1",
		"preflight-groups-page-1",
		"preflight-folders-page-1",
		"preflight-projects-page-1",
		"preflight-service-principals-page-1",
		"iam-role-list",
		"iam-role-read",
		"id-role-list",
		"id-role-read",
		"id-policy-read-organization",
		"read-auth-context",
		"read-auth-conditions",
		"read-password-policy",
		"policy-read-organization",
		"organization-read",
		"service-policy-is-enabled",
		"read-service-policy",
		"list-rule-templates",
		"create-folder",
		"create-project",
		"policy-read-folder",
		"policy-read-project",
		"create-user",
		"create-service-principal",
		"verify-user-list-page-1",
		"read-user",
		"update-user",
		"update-folder",
		"update-project",
		"register-email",
		"verify-email",
		"unregister-email",
		"verify-email-unregistered",
		"create-group",
		"update-memberships",
		"read-memberships",
		"u2fa-list-trusted-devices",
		"u2fa-list-security-keys",
		"u2fa-deactivate-otp",
		"u2fa-clear-trusted-devices",
		"u2fa-read-missing-security-key",
		"u2fa-delete-missing-trusted-device",
		"u2fa-delete-missing-security-key",
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
		"cleanup-delete-service-principal",
		"cleanup-verify-service-principal-page-1",
		"cleanup-delete-group",
		"cleanup-verify-group-page-1",
		"cleanup-delete-user",
		"cleanup-verify-user-page-1",
		"cleanup-delete-project",
		"cleanup-verify-project-page-1",
		"cleanup-delete-folder",
		"cleanup-verify-folder-page-1",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("scenario did not call %s: %v", want, fake.calls)
		}
	}
	queryArgs := strings.Join(fake.argsByStep["preflight-users-page-1"], " ")
	if !strings.Contains(queryArgs, "--query") || !strings.Contains(queryArgs, `.name == "skr-e2e-iam-test00ff"`) {
		t.Errorf("preflight list query = %q, want an exact-name projection", queryArgs)
	}
	if strings.Contains(queryArgs, "email") {
		t.Errorf("preflight query projects unnecessary email data: %q", queryArgs)
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-user") < strings.Index(got, "cleanup-delete-group") {
		t.Fatal("the group must be deleted before its member user")
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-project") < strings.Index(got, "cleanup-delete-service-principal") {
		t.Fatal("the service principal must be deleted before its project")
	}
	if got := strings.Join(fake.calls, ","); strings.Index(got, "cleanup-delete-folder") < strings.Index(got, "cleanup-delete-project") {
		t.Fatal("the project must be deleted before its folder")
	}
}

func TestListReadsEveryPage(t *testing.T) {
	fake := &fakeCLI{
		users:             make(map[string]item),
		groups:            make(map[string]item),
		folders:           make(map[string]item),
		projects:          make(map[string]item),
		servicePrincipals: make(map[string]item),
		listPages: map[string][][]item{
			"user": {
				{{ID: 1001, Name: "first"}},
				{{ID: 1002, Name: "second"}},
			},
		},
		listPageCounts: map[string][]int{"user": {userGroupPageSize, 1}},
	}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-users", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 1001 || items[1].ID != 1002 {
		t.Fatalf("list returned %#v, want items from both pages", items)
	}
	for _, want := range []string{"preflight-users-page-1", "preflight-users-page-2"} {
		if !contains(fake.calls, want) {
			t.Errorf("list did not fetch %s: %v", want, fake.calls)
		}
	}
	if contains(fake.calls, "preflight-users-page-3") {
		t.Errorf("list fetched a page after a short final page: %v", fake.calls)
	}
}

func TestListQueriesCompile(t *testing.T) {
	for _, kind := range []string{"user", "group", "folder", "project", "service-principal"} {
		for _, targetNames := range [][]string{nil, {"target"}} {
			query := listQuery(kind, targetNames...)
			parsed, err := gojq.Parse(query)
			if err != nil {
				t.Errorf("parse %s list query %q: %v", kind, query, err)
				continue
			}
			if _, err := gojq.Compile(parsed); err != nil {
				t.Errorf("compile %s list query %q: %v", kind, query, err)
			}
		}
	}
}

func TestListStopsWhenAPIHasNoNextPage(t *testing.T) {
	fake := newFakeCLI()
	fake.listPages = map[string][][]item{
		"project": {
			{{ID: 1001, Name: "first"}},
			{{ID: 1002, Name: "unrequested-page"}},
		},
	}
	fake.listHasNext = map[string][]bool{"project": {false}}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-projects", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 1001 {
		t.Fatalf("list returned %#v, want the first API page only", items)
	}
	if contains(fake.calls, "preflight-projects-page-2") {
		t.Fatalf("list requested a page after the API reported no next page: %v", fake.calls)
	}
}

func TestListContinuesAcrossUnmatchedProjectedPages(t *testing.T) {
	fake := newFakeCLI()
	fake.listPages = map[string][][]item{
		"user": {
			{},
			{{ID: 1002, Name: "target", Description: "skr-e2e-iam/test00ff"}},
		},
	}
	fake.listPageCounts = map[string][]int{"user": {userGroupPageSize, 1}}
	s := testScenario(t, fake)
	items, err := s.list(context.Background(), "preflight-users", "user", "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "target" {
		t.Fatalf("list returned %#v, want target from the second page", items)
	}
	if !contains(fake.calls, "preflight-users-page-2") || contains(fake.calls, "preflight-users-page-3") {
		t.Fatalf("list did not stop after checking the final page: %v", fake.calls)
	}
}

func TestUserGroupListsRequestLargePages(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		fake := newFakeCLI()
		s := testScenario(t, fake)
		if _, err := s.list(context.Background(), "preflight-"+kind, kind); err != nil {
			t.Fatal(err)
		}
		args := fake.argsByStep["preflight-"+kind+"-page-1"]
		if got := argument(args, "--per-page"); got != strconv.Itoa(userGroupPageSize) {
			t.Errorf("%s per-page = %q, want %d", kind, got, userGroupPageSize)
		}
	}
}

func TestScenarioRecoversResourcesWhenCreateReturnsError(t *testing.T) {
	fake := newFakeCLI()
	fake.failAt = "create-group"
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure at create-group") {
		t.Fatalf("scenario error = %v, want create-group failure", err)
	}
	if len(fake.groups) != 0 {
		t.Fatalf("group remains after failure: %+v", fake.groups)
	}
	for _, want := range []string{
		"cleanup-find-group-page-1",
		"cleanup-delete-group",
		"cleanup-verify-group-page-1",
	} {
		if !contains(fake.calls, want) {
			t.Errorf("uncertain group create was not recovered via %s: %v", want, fake.calls)
		}
	}
	if !contains(fake.calls, "cleanup-delete-user") || !contains(fake.calls, "cleanup-verify-user-page-1") {
		t.Fatalf("created user was not cleaned up: %v", fake.calls)
	}
	if len(fake.users) != 0 {
		t.Fatalf("user remains after cleanup: %+v", fake.users)
	}
}

func TestScenarioCleansUpAfterRenameResponseIsLost(t *testing.T) {
	for _, step := range []string{"update-user", "update-folder", "update-project"} {
		t.Run(step, func(t *testing.T) {
			fake := newFakeCLI()
			fake.failAfterApply = step
			s := testScenario(t, fake)
			err := s.run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "injected CLI failure at "+step) {
				t.Fatalf("scenario error = %v, want simulated %s response loss", err, step)
			}
			if len(fake.folders) != 0 || len(fake.projects) != 0 {
				t.Fatalf("renamed resources remain after cleanup: folders=%+v projects=%+v", fake.folders, fake.projects)
			}
			wantSteps := []string{"cleanup-delete-folder", "cleanup-delete-project"}
			switch step {
			case "update-user":
				wantSteps = append(wantSteps,
					"cleanup-verify-folder-page-1",
					"cleanup-verify-project-page-1",
					"cleanup-verify-user-pending-name-page-1",
					"cleanup-delete-user",
				)
			case "update-folder":
				wantSteps = append(wantSteps, "cleanup-verify-folder-pending-name-page-1", "cleanup-verify-project-page-1")
			case "update-project":
				wantSteps = append(wantSteps, "cleanup-verify-folder-page-1", "cleanup-verify-project-pending-name-page-1")
			}
			for _, want := range wantSteps {
				if !contains(fake.calls, want) {
					t.Errorf("cleanup did not call %s after %s: %v", want, step, fake.calls)
				}
			}
			if step == "update-user" && !contains(fake.calls, "cleanup-verify-user-page-1") {
				t.Errorf("cleanup did not verify the original user name after %s: %v", step, fake.calls)
			}
			if len(fake.users) != 0 {
				t.Errorf("renamed user remains after cleanup: %+v", fake.users)
			}
		})
	}
}

func TestScenarioRefusesExistingResource(t *testing.T) {
	fake := &fakeCLI{
		users:  make(map[string]item),
		groups: make(map[string]item),
	}
	fake.users["existing"] = item{ID: 42, Name: "skr-e2e-iam-test00ff", Code: "existing", Description: "unrelated"}
	s := testScenario(t, fake)
	err := s.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want existing resource refusal", err)
	}
	if len(fake.calls) != 2 || len(fake.users) != 1 {
		t.Fatalf("scenario modified an existing resource: calls=%v users=%+v", fake.calls, fake.users)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
