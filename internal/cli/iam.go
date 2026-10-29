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
	c.IAMAPI.User = iamapi.NewUserCommands(iamapi.UserRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.UserAPI, error) {
		return iamapi.NewUserAPI(c.Trace)
	})
	c.IAMAPI.Group = iamapi.NewGroupCommands(iamapi.GroupRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.GroupAPI, error) {
		return iamapi.NewGroupAPI(c.Trace)
	})
	c.IAMAPI.Policy = iamapi.NewPolicyCommands(iamapi.PolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.PolicyAPI, error) {
		return iamapi.NewPolicyAPI(c.Trace)
	})

	c.IAMAPI.Auth = iamapi.NewAuthCommands(iamapi.AuthRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.AuthAPI, error) { return iamapi.NewAuthAPI(c.Trace) })
	c.IAMAPI.Folder = iamapi.NewFolderCommands(iamapi.FolderRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.FolderAPI, error) { return iamapi.NewFolderAPI(c.Trace) })
	c.IAMAPI.IAMRole = iamapi.NewIAMRoleCommands(iamapi.IAMRoleRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.IAMRoleAPI, error) { return iamapi.NewIAMRoleAPI(c.Trace) })
	c.IAMAPI.IDPolicy = iamapi.NewIDPolicyCommands(iamapi.IDPolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.IDPolicyAPI, error) { return iamapi.NewIDPolicyAPI(c.Trace) })
	c.IAMAPI.IDRole = iamapi.NewIDRoleCommands(iamapi.IDRoleRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.IDRoleAPI, error) { return iamapi.NewIDRoleAPI(c.Trace) })
	c.IAMAPI.Organization = iamapi.NewOrganizationCommands(iamapi.OrganizationRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.OrganizationAPI, error) { return iamapi.NewOrganizationAPI(c.Trace) })
	c.IAMAPI.Project = iamapi.NewProjectCommands(iamapi.ProjectRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.ProjectAPI, error) { return iamapi.NewProjectAPI(c.Trace) })
	c.IAMAPI.ProjectAPIKey = iamapi.NewProjectAPIKeyCommands(iamapi.ProjectAPIKeyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.ProjectAPIKeyAPI, error) { return iamapi.NewProjectAPIKeyAPI(c.Trace) })
	c.IAMAPI.SCIM = iamapi.NewScimCommands(iamapi.ScimRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.ScimAPI, error) { return iamapi.NewSCIMAPI(c.Trace) })
	c.IAMAPI.ServicePolicy = iamapi.NewServicePolicyCommands(iamapi.ServicePolicyRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.ServicePolicyAPI, error) { return iamapi.NewServicePolicyAPI(c.Trace) })
	c.IAMAPI.ServicePrincipal = iamapi.NewServicePrincipalCommands(iamapi.ServicePrincipalRuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.ServicePrincipalAPI, error) { return iamapi.NewServicePrincipalAPI(c.Trace) })
	c.IAMAPI.SSO = iamapi.NewSSOCommands(iamapi.SSORuntime{DecodeRequest: iamapi.DecodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput}, func() (iamapi.SSOAPI, error) { return iamapi.NewSSOAPI(c.Trace) })
	c.IAMAPI.User2FA = iamapi.NewUser2FACommands(iamapi.User2FARuntime{ValidateOutput: validateOutput, WriteOutput: writeOutput}, func(userID int) (iamapi.User2FAAPI, error) {
		return iamapi.NewUser2FAAPI(c.Trace, userID)
	})
}
