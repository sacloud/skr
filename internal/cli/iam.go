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

package cli

import "github.com/sacloud/skr/internal/iamapi"

func (c *cli) initIAMAPI() {
	c.IAMAPI.User.SetRuntime(iamapi.UserRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.User.SetFactory(func() (iamapi.UserAPI, error) {
		return iamapi.NewUserAPI(c.Trace)
	})
	c.IAMAPI.Group.SetRuntime(iamapi.GroupRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Group.SetFactory(func() (iamapi.GroupAPI, error) {
		return iamapi.NewGroupAPI(c.Trace)
	})
	c.IAMAPI.Policy.SetRuntime(iamapi.PolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Policy.SetFactory(func() (iamapi.PolicyAPI, error) {
		return iamapi.NewPolicyAPI(c.Trace)
	})

	c.IAMAPI.Auth.SetRuntime(iamapi.AuthRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Auth.SetFactory(func() (iamapi.AuthAPI, error) { return iamapi.NewAuthAPI(c.Trace) })
	c.IAMAPI.Folder.SetRuntime(iamapi.FolderRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Folder.SetFactory(func() (iamapi.FolderAPI, error) { return iamapi.NewFolderAPI(c.Trace) })
	c.IAMAPI.IAMRole.SetRuntime(iamapi.IAMRoleRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.IAMRole.SetFactory(func() (iamapi.IAMRoleAPI, error) { return iamapi.NewIAMRoleAPI(c.Trace) })
	c.IAMAPI.IDPolicy.SetRuntime(iamapi.IDPolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.IDPolicy.SetFactory(func() (iamapi.IDPolicyAPI, error) { return iamapi.NewIDPolicyAPI(c.Trace) })
	c.IAMAPI.IDRole.SetRuntime(iamapi.IDRoleRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.IDRole.SetFactory(func() (iamapi.IDRoleAPI, error) { return iamapi.NewIDRoleAPI(c.Trace) })
	c.IAMAPI.Organization.SetRuntime(iamapi.OrganizationRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Organization.SetFactory(func() (iamapi.OrganizationAPI, error) { return iamapi.NewOrganizationAPI(c.Trace) })
	c.IAMAPI.Project.SetRuntime(iamapi.ProjectRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.Project.SetFactory(func() (iamapi.ProjectAPI, error) { return iamapi.NewProjectAPI(c.Trace) })
	c.IAMAPI.ProjectAPIKey.SetRuntime(iamapi.ProjectAPIKeyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.ProjectAPIKey.SetFactory(func() (iamapi.ProjectAPIKeyAPI, error) { return iamapi.NewProjectAPIKeyAPI(c.Trace) })
	c.IAMAPI.SCIM.SetRuntime(iamapi.ScimRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.SCIM.SetFactory(func() (iamapi.ScimAPI, error) { return iamapi.NewSCIMAPI(c.Trace) })
	c.IAMAPI.ServicePolicy.SetRuntime(iamapi.ServicePolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.ServicePolicy.SetFactory(func() (iamapi.ServicePolicyAPI, error) { return iamapi.NewServicePolicyAPI(c.Trace) })
	c.IAMAPI.ServicePrincipal.SetRuntime(iamapi.ServicePrincipalRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.ServicePrincipal.SetFactory(func() (iamapi.ServicePrincipalAPI, error) { return iamapi.NewServicePrincipalAPI(c.Trace) })
	c.IAMAPI.SSO.SetRuntime(iamapi.SSORuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.SSO.SetFactory(func() (iamapi.SSOAPI, error) { return iamapi.NewSSOAPI(c.Trace) })
	c.IAMAPI.User2FA.SetRuntime(iamapi.User2FARuntime{ValidateOutput: validateOutput, WriteOutput: writeOutput})
	c.IAMAPI.User2FA.SetFactory(func(userID int) (iamapi.User2FAAPI, error) {
		return iamapi.NewUser2FAAPI(c.Trace, userID)
	})
}
