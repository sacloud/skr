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

import (
	eventbusv1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	iamv1 "github.com/sacloud/sacloud-sdk-go/api/iam/apis/v1"
)

// apiCommandNameBaseline は operationId 機械派生への移行が完了していないコマンドの
// 現行パスと、対応する SDK の operation 定数を登録します。
// 値は SDK の ogen 生成コードが公開する operation 定数で、operationId を正規化した
// operation 名を値として持ちます。
// ドメインの移行を完了したら、該当ドメインのエントリを削除します。
// 詳細は docs/design/api-command-operation-id-naming.md を参照してください。
var apiCommandNameBaseline = map[string]string{
	// apprun-dedicated-api
	// eventbus-api
	"eventbus-api create-common-service-item":       eventbusv1.CreateCommonServiceItemOperation,
	"eventbus-api delete-common-service-item":       eventbusv1.DeleteCommonServiceItemOperation,
	"eventbus-api list-common-service-items":        eventbusv1.ListCommonServiceItemsOperation,
	"eventbus-api read-common-service-item":         eventbusv1.ReadCommonServiceItemOperation,
	"eventbus-api set-process-configuration-secret": eventbusv1.SetProcessConfigurationSecretOperation,
	"eventbus-api update-common-service-item":       eventbusv1.UpdateCommonServiceItemOperation,
	// iam-api
	"iam-api auth read-auth-conditions":          iamv1.ReadOrganizationAuthConditionsOperation,
	"iam-api auth read-auth-context":             iamv1.ReadAuthContextOperation,
	"iam-api auth read-password-policy":          iamv1.ReadOrganizationPasswordPolicyOperation,
	"iam-api auth update-auth-conditions":        iamv1.UpdateOrganizationAuthConditionsOperation,
	"iam-api auth update-password-policy":        iamv1.UpdateOrganizationPasswordPolicyOperation,
	"iam-api folder create":                      iamv1.CreateFolderOperation,
	"iam-api folder delete":                      iamv1.DeleteFolderOperation,
	"iam-api folder list":                        iamv1.ListFoldersOperation,
	"iam-api folder move":                        iamv1.MoveFoldersOperation,
	"iam-api folder read":                        iamv1.ReadFolderOperation,
	"iam-api folder update":                      iamv1.UpdateFolderOperation,
	"iam-api group create":                       iamv1.CreateGroupOperation,
	"iam-api group delete":                       iamv1.DeleteGroupOperation,
	"iam-api group list":                         iamv1.ListGroupsOperation,
	"iam-api group read":                         iamv1.ReadGroupOperation,
	"iam-api group read-memberships":             iamv1.ListMembershipsOperation,
	"iam-api group update":                       iamv1.UpdateGroupOperation,
	"iam-api group update-memberships":           iamv1.UpdateMembershipsOperation,
	"iam-api iam-role list":                      iamv1.ListIamRolesOperation,
	"iam-api iam-role read":                      iamv1.ReadIamRoleOperation,
	"iam-api id-policy read-organization":        iamv1.ReadOrganizationIdPolicyOperation,
	"iam-api id-policy update-organization":      iamv1.UpdateOrganizationIdPolicyOperation,
	"iam-api id-role list":                       iamv1.ListIdRolesOperation,
	"iam-api id-role read":                       iamv1.ReadIdRoleOperation,
	"iam-api organization read":                  iamv1.ReadOrganizationOperation,
	"iam-api organization read-service-policy":   iamv1.ReadOrganizationServicePolicyOperation,
	"iam-api organization update":                iamv1.UpdateOrganizationOperation,
	"iam-api organization update-service-policy": iamv1.UpdateOrganizationServicePolicyOperation,
	"iam-api policy read-folder":                 iamv1.ReadFolderIamPolicyOperation,
	"iam-api policy read-organization":           iamv1.ReadOrganizationIamPolicyOperation,
	"iam-api policy read-project":                iamv1.ReadProjectIamPolicyOperation,
	"iam-api policy update-folder":               iamv1.UpdateFolderIamPolicyOperation,
	"iam-api policy update-organization":         iamv1.UpdateOrganizationIamPolicyOperation,
	"iam-api policy update-project":              iamv1.UpdateProjectIamPolicyOperation,
	"iam-api project create":                     iamv1.CreateProjectOperation,
	"iam-api project delete":                     iamv1.DeleteProjectOperation,
	"iam-api project list":                       iamv1.ListProjectsOperation,
	"iam-api project move":                       iamv1.MoveProjectsOperation,
	"iam-api project read":                       iamv1.ReadProjectOperation,
	"iam-api project update":                     iamv1.UpdateProjectOperation,
	"iam-api project-api-key create":             iamv1.CreateApiKeyOperation,
	"iam-api project-api-key delete":             iamv1.DeleteApiKeyOperation,
	"iam-api project-api-key list":               iamv1.ListApiKeysOperation,
	"iam-api project-api-key read":               iamv1.ReadApiKeyOperation,
	"iam-api project-api-key update":             iamv1.UpdateApiKeyOperation,
	"iam-api scim create":                        iamv1.CreateScimConfigurationOperation,
	"iam-api scim delete":                        iamv1.DeleteScimConfigurationOperation,
	"iam-api scim list":                          iamv1.ListScimConfigurationsOperation,
	"iam-api scim read":                          iamv1.ReadScimConfigurationOperation,
	"iam-api scim regenerate-token":              iamv1.RegenerateScimConfigurationTokenOperation,
	"iam-api scim update":                        iamv1.UpdateScimConfigurationOperation,
	"iam-api service-policy disable":             iamv1.DisableServicePolicyOperation,
	"iam-api service-policy enable":              iamv1.EnableServicePolicyOperation,
	"iam-api service-policy is-enabled":          iamv1.CheckServicePolicyStatusOperation,
	"iam-api service-policy list-rule-templates": iamv1.ListServicePolicyRuleTemplatesOperation,
	"iam-api service-principal create":           iamv1.CreateServicePrincipalOperation,
	"iam-api service-principal delete":           iamv1.DeleteServicePrincipalOperation,
	"iam-api service-principal delete-key":       iamv1.DeleteServicePrincipalKeyOperation,
	"iam-api service-principal disable-key":      iamv1.DisableServicePrincipalKeyOperation,
	"iam-api service-principal enable-key":       iamv1.EnableServicePrincipalKeyOperation,
	"iam-api service-principal issue-token":      iamv1.IssueServicePrincipalTokenOperation,
	"iam-api service-principal list":             iamv1.ListServicePrincipalsOperation,
	"iam-api service-principal list-keys":        iamv1.ListServicePrincipalKeysOperation,
	"iam-api service-principal read":             iamv1.ReadServicePrincipalOperation,
	"iam-api service-principal update":           iamv1.UpdateServicePrincipalOperation,
	"iam-api service-principal upload-key":       iamv1.UploadServicePrincipalKeyOperation,
	"iam-api sso create":                         iamv1.CreateSsoProfileOperation,
	"iam-api sso delete":                         iamv1.DeleteSsoProfileOperation,
	"iam-api sso link":                           iamv1.AssignSsoProfileOperation,
	"iam-api sso list":                           iamv1.ListSsoProfilesOperation,
	"iam-api sso read":                           iamv1.ReadSsoProfileOperation,
	"iam-api sso unlink":                         iamv1.UnassignSsoProfileOperation,
	"iam-api sso update":                         iamv1.UpdateSsoProfileOperation,
	"iam-api user create":                        iamv1.CreateUserOperation,
	"iam-api user delete":                        iamv1.DeleteUserOperation,
	"iam-api user list":                          iamv1.ListUsersOperation,
	"iam-api user read":                          iamv1.ReadUserOperation,
	"iam-api user register-email":                iamv1.RegisterEmailOperation,
	"iam-api user unregister-email":              iamv1.UnregisterEmailOperation,
	"iam-api user update":                        iamv1.UpdateUserOperation,
	"iam-api user-2fa clear-trusted-devices":     iamv1.ClearTrustedDevicesOperation,
	"iam-api user-2fa deactivate-otp":            iamv1.DeactivateOtpOperation,
	"iam-api user-2fa delete-security-key":       iamv1.DeleteSecurityKeyOperation,
	"iam-api user-2fa delete-trusted-device":     iamv1.DeleteTrustedDeviceOperation,
	"iam-api user-2fa list-security-keys":        iamv1.ListSecurityKeysOperation,
	"iam-api user-2fa list-trusted-devices":      iamv1.ListTrustedDevicesOperation,
	"iam-api user-2fa read-security-key":         iamv1.ReadSecurityKeyOperation,
}
