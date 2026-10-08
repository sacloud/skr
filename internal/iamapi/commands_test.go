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

package iamapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iam/apis/organization"
	"github.com/sacloud/sacloud-sdk-go/api/iam/apis/projectapikey"
	v1 "github.com/sacloud/sacloud-sdk-go/api/iam/apis/v1"
)

func TestDecodeProjectAPIKeyRequestUsesAPIFieldNames(t *testing.T) {
	createJSON := `{"project_id":7,"name":"test-key","description":"test","server_resource_id":"srv-1","iam_roles":["resource-viewer"],"zone_id":"is1a"}`
	var create projectapikey.CreateParams
	if err := DecodeRequest(createJSON, &create); err != nil {
		t.Fatal(err)
	}
	if create.ProjectID != 7 || create.Name != "test-key" || len(create.IamRoles) != 1 || create.IamRoles[0] != "resource-viewer" {
		t.Fatalf("decoded create request = %#v", create)
	}
	if create.ServerResourceID == nil || *create.ServerResourceID != "srv-1" || create.Zone == nil || *create.Zone != "is1a" {
		t.Fatalf("decoded optional create fields = %#v", create)
	}

	updatePath := filepath.Join(t.TempDir(), "api-key-update.json")
	if err := os.WriteFile(updatePath, []byte(`{"name":"updated-key","description":"updated","iam_roles":["resource-editor"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var update projectapikey.UpdateParams
	if err := DecodeRequest("@"+updatePath, &update); err != nil {
		t.Fatal(err)
	}
	if update.Name != "updated-key" || len(update.IamRoles) != 1 || update.IamRoles[0] != "resource-editor" {
		t.Fatalf("decoded update request = %#v", update)
	}
}

func TestDecodeOrganizationServicePolicyRequestUsesAPIFieldNames(t *testing.T) {
	var request organization.GetServicePolicyParams
	if err := DecodeRequest(`{"is_active":false,"is_dry_run":true,"name":"service","code":"svc","type":"bool"}`, &request); err != nil {
		t.Fatal(err)
	}
	if request.IsActive == nil || *request.IsActive {
		t.Errorf("IsActive = %v, want explicit false", request.IsActive)
	}
	if request.IsDryRun == nil || !*request.IsDryRun {
		t.Errorf("IsDryRun = %v, want explicit true", request.IsDryRun)
	}
	if request.Name == nil || *request.Name != "service" {
		t.Errorf("Name = %v, want service", request.Name)
	}
	if request.Code == nil || *request.Code != "svc" {
		t.Errorf("Code = %v, want svc", request.Code)
	}
	if request.Type == nil || *request.Type != v1.ReadOrganizationServicePolicyType("bool") {
		t.Errorf("Type = %v, want bool", request.Type)
	}

	if err := DecodeRequest("{}", &request); err != nil {
		t.Fatal(err)
	}
	if request.IsActive != nil || request.IsDryRun != nil || request.Name != nil || request.Code != nil || request.Type != nil {
		t.Fatalf("empty request populated omitted filters: %#v", request)
	}
}

func TestDecodeRequestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memberships.json")
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o600); err != nil {
		t.Fatal(err)
	}
	var value []int
	if err := DecodeRequest("@"+path, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 3 || value[0] != 1 || value[2] != 3 {
		t.Fatalf("decoded %#v, want [1 2 3]", value)
	}
	if err := DecodeRequest("@/missing/memberships.json", &value); err == nil {
		t.Error("DecodeRequest(@missing) succeeded, want an error")
	}
	if err := DecodeRequest("{invalid", &value); err == nil {
		t.Error("DecodeRequest(invalid JSON) succeeded, want an error")
	}
}

func TestReadTextInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assertion.txt")
	if err := os.WriteFile(path, []byte("  assertion-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := readTextInput(path, "--assertion-file", "assertion")
	if err != nil {
		t.Fatal(err)
	}
	if value != "assertion-value" {
		t.Fatalf("readTextInput() = %q, want trimmed assertion", value)
	}
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "", want: "--assertion-file が必要です"},
		{path: filepath.Join(t.TempDir(), "missing.txt"), want: "read assertion file"},
	} {
		if _, err := readTextInput(test.path, "--assertion-file", "assertion"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("readTextInput(%q) error = %v, want %q", test.path, err, test.want)
		}
	}
	emptyPath := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(emptyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTextInput(emptyPath, "--assertion-file", "assertion"); err == nil || err.Error() != "assertion is empty" {
		t.Errorf("readTextInput(empty) error = %v, want assertion is empty", err)
	}
}

type issueTokenTestAPI struct {
	ServicePrincipalAPI
	assertion string
}

func (a *issueTokenTestAPI) IssueToken(_ context.Context, assertion string) (*v1.ServicePrincipalOAuth2AccessToken, error) {
	a.assertion = assertion
	return &v1.ServicePrincipalOAuth2AccessToken{AccessToken: "secret-access-token"}, nil
}

func TestServicePrincipalIssueTokenCommandSuccess(t *testing.T) {
	assertionFile := filepath.Join(t.TempDir(), "assertion.txt")
	if err := os.WriteFile(assertionFile, []byte(" signed-assertion \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &issueTokenTestAPI{}
	var output []byte
	command := ServicePrincipalIssueTokenCommand{
		AssertionFile: assertionFile,
		factory: func() (ServicePrincipalAPI, error) {
			return api, nil
		},
		runtime: ServicePrincipalRuntime{
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				var err error
				output, err = json.Marshal(value)
				return err
			},
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if api.assertion != "signed-assertion" {
		t.Fatalf("IssueToken assertion = %q, want trimmed file contents", api.assertion)
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "secret-access-token" {
		t.Fatalf("serialized token response = %s", output)
	}
}

type organizationServicePolicyTestAPI struct {
	OrganizationAPI
	requests []organization.GetServicePolicyParams
}

func (a *organizationServicePolicyTestAPI) ReadServicePolicy(
	_ context.Context,
	request organization.GetServicePolicyParams,
) ([]v1.RuleResponse, error) {
	a.requests = append(a.requests, request)
	return nil, nil
}

func TestOrganizationReadServicePolicyFlagsPreserveOptionalValues(t *testing.T) {
	api := &organizationServicePolicyTestAPI{}
	isActive := false
	isDryRun := true
	name := "test-service"
	code := "test-code"
	policyType := "bool"
	command := OrganizationReadServicePolicyCommand{
		IsActive: &isActive,
		IsDryRun: &isDryRun,
		Name:     &name,
		Code:     &code,
		Type:     &policyType,
		factory: func() (OrganizationAPI, error) {
			return api, nil
		},
		runtime: OrganizationRuntime{
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput:    func(*kong.Context, any) error { return nil },
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("ReadServicePolicy call count = %d, want 1", len(api.requests))
	}
	got := api.requests[0]
	if got.IsActive == nil || *got.IsActive {
		t.Errorf("IsActive = %v, want explicit false", got.IsActive)
	}
	if got.IsDryRun == nil || !*got.IsDryRun {
		t.Errorf("IsDryRun = %v, want explicit true", got.IsDryRun)
	}
	if got.Name == nil || *got.Name != name || got.Code == nil || *got.Code != code {
		t.Errorf("name/code filters = %v/%v, want %q/%q", got.Name, got.Code, name, code)
	}
	if got.Type == nil || *got.Type != v1.ReadOrganizationServicePolicyType(policyType) {
		t.Errorf("Type = %v, want %q", got.Type, policyType)
	}

	command = OrganizationReadServicePolicyCommand{
		factory: func() (OrganizationAPI, error) {
			return api, nil
		},
		runtime: OrganizationRuntime{
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput:    func(*kong.Context, any) error { return nil },
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	omitted := api.requests[1]
	if omitted.IsActive != nil || omitted.IsDryRun != nil || omitted.Name != nil || omitted.Code != nil || omitted.Type != nil {
		t.Fatalf("omitted flags populated request fields: %#v", omitted)
	}
}
