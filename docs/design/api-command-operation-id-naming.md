# API コマンド名の operationId 準拠と移行計画

## 目的

`iaas-api` 以外の `*-api` コマンドを、OpenAPI の `operationId` から機械派生した名前で `skr <domain>-api <operation>` の形に統一します。あわせて、機械派生と一致していることを CI で継続的に検証し、規則に反するコマンドが存在したり追加されたりしないようにします。

## 背景と課題

現行のコマンドは、リソース名をコマンドグループ、操作名を末端サブコマンドに置く構造で、名前は人間が割り当てています。operationId との対応は機械的でなく、API に存在しない語や、operationId と違う動詞がコマンド名に入っています。

| 現在のコマンド名 | API の operationId | 課題 |
|---|---|---|
| `skr simplemq-api queue config` | `updateQueue` | `config` は operationId に存在しない語 |
| `skr iam-api sso link` | `assignSsoProfile` | `link` は operationId の動詞と別の語 |
| `skr simplemq-api queue count-messages` | `readMessageCount` | `count` と `message` の語順が operationId と逆 |
| `skr simplemq-api queue clear-messages` | `clearQueue` | `messages` は operationId に存在しない語 |
| `skr object-storage-api bucket encryption enable` | `putBucketEncryption` | `enable` は operationId の動詞と別の語 |

operationId から名前を機械派生する方式では、このような課題は構造上発生しません。名前は operationId から一意に決まり、CI は派生名との一致を検証するだけで済みます。決定の経緯は [ADR 0074](../adr/0074-api-command-name-structure.md) を参照してください。

## 方針

- **コマンド構造**: `<domain>-api` 直下に末端サブコマンドを配置します。リソースのグループ階層は設けません。
- **名前**: operationId から機械派生します。語の選択や省略の設計裁量は入れません。
- **共有操作**: operationId を複数のリソースで共有する CommonServiceItem 操作は、spec のとおり 1 組のコマンドに統合し、リソースの種別を `--provider-class` で渡します。
- **CI**: 派生名との一致を検証します。

## 命名規則

1. SDK (`sacloud-sdk-go` v0.5.0) の各ドメインには OpenAPI spec が同梱され、各操作に `operationId` が定義されています。spec の配置は `api/<domain>/openapi/` です。
2. SDK は ogen が生成するコードで `operationId` を operation 名 (PascalCase) に正規化します。snake_case を含む `service-endpoint-gateway` の `seg_apply` は、この段階で `SegApply` に正規化されます。
3. operation 名を Kong と同じ camelcase 処理で語に分割し、ハイフンで結合して小文字化します。これが末端サブコマンド名です。たとえば `rotateAPIKey` → `rotate-api-key`、`segStatus_getPowerStatus` → `seg-status-get-power-status`、`sendMessage` → `send-message` になります。
4. `security-control` は spec を SDK に同梱しておらず、ogen 生成コードの定数が機械的な根拠になります。

命名の根拠は SDK の spec と ogen 生成コードです。

- spec: `api/<domain>/openapi/*.json|yaml` ([sacloud-sdk-go v0.5.0](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0))
- operation 名の定数: `api/<domain>/apis/**/oas_operations_gen.go`

## CommonServiceItem 操作の統合

`eventbus-api` と `simple-notification-api` が操作する CommonServiceItem は、さくらのクラウドのサービスを統合的に扱うための枠組みです。spec では `listCommonServiceItems` などの operationId を、`trigger`、`schedule`、`destination` など複数のリソースで共有します。現行 CLI は同じ SDK 操作をリソースごとのグループに複製していますが、リソースの違いは Provider.Class で表されるため、グループを設ける必要はありません。

operationId 機械派生に移行する際、この 2 ドメインでは spec のとおり 1 組の CommonServiceItem 操作に統合します。リソースの種別は `--provider-class` オプションで渡します。値は SDK が公開する `ProviderClass` の列挙値です。`eventbus-api` では `eventbustrigger`、`eventbusschedule`、`eventbusprocessconfiguration` を使います。`simple-notification-api` では `saknoticedestination`、`saknoticegroup`、`saknoticerouting` を使います。一覧操作では Provider.Class による絞り込みに、作成と更新ではリクエストの `CommonServiceItem.Provider.Class` の設定に使います。

リソースごとに使いやすい操作の形 (たとえば `trigger` の作成) が必要な場合は、高レベルコマンドとして提供します。

## 対象範囲

対象は次の 12 ドメインです。既存 276 コマンドはすべて名前が変わります。うち CommonServiceItem 操作を複製していた 30 コマンドは、spec のとおり 10 操作に統合されるため、変更後のコマンド数は 256 になります。

| ドメイン | 現在のコマンド数 | 変更後のコマンド数 | コマンド定義 |
|---|---|---|---|
| `kms-api` | 10 | 10 | 生成 (`api/commands/kms-key.json`、4 操作は手書き) |
| `secretmanager-api` | 9 | 9 | 生成 (2 ファイル、2 操作は手書き) |
| `security-control-api` | 11 | 11 | 生成 (3 ファイル、3 操作は手書き) |
| `service-endpoint-gateway-api` | 11 | 11 | 生成 (`api/commands/service-endpoint-gateway.json`、1 操作は手書き) |
| `simplemq-api` | 12 | 12 | 生成 (2 ファイル、4 操作は手書き) |
| `networking-suite-api` | 14 | 14 | 手書き (`internal/networkingsuiteapi`) |
| `eventbus-api` | 16 | 6 | 生成 (3 ファイル、1 操作は手書き) |
| `apprun-api` | 16 | 16 | 生成 (5 ファイル) |
| `simple-notification-api` | 21 | 11 | 生成 (4 ファイル) |
| `object-storage-api` | 34 | 34 | 手書き (`internal/objectstorageapi`) |
| `apprun-dedicated-api` | 35 | 35 | 手書き (`internal/apprundedicatedapi`) |
| `iam-api` | 87 | 87 | 生成 (15 ファイル、6 操作は手書き) |
| 合計 | 276 | 256 | |

対象外は次のとおりです。

- `iaas-api`: IaaS API には OpenAPI の `operationId` がなく、endpoint 構造に対応させる命名 ([ADR 0069](../adr/0069-api-endpoint-command-groups.md)) を維持します。
- `gslb-api` / `proxylb-api` / `container-registry-api`: IaaS API を SDK の service レイヤ経由で操作する手書きコマンドであり、`operationId` が存在しないため対応表を定義できません。対象に含める場合は別途合意します。

## 対応表

全コマンドの現在と変更後を示します。CommonServiceItem 操作の統合では、3 つのリソースの同名操作が 1 つのコマンドになります。操作の追加・削除時は、この表と SDK の spec を一致させてください。

**apprun-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr apprun-api application create` | `createApplication` | `skr apprun-api create-application` |
| `skr apprun-api user create` | `createUser` | `skr apprun-api create-user` |
| `skr apprun-api application delete` | `deleteApplication` | `skr apprun-api delete-application` |
| `skr apprun-api version delete` | `deleteApplicationVersion` | `skr apprun-api delete-application-version` |
| `skr apprun-api traffic list` | `listApplicationTraffic` | `skr apprun-api list-application-traffic` |
| `skr apprun-api version list` | `listApplicationVersions` | `skr apprun-api list-application-versions` |
| `skr apprun-api application list` | `listApplications` | `skr apprun-api list-applications` |
| `skr apprun-api application update` | `patchApplication` | `skr apprun-api patch-application` |
| `skr apprun-api packet-filter update` | `patchApplicationPacketFilter` | `skr apprun-api patch-application-packet-filter` |
| `skr apprun-api application read` | `readApplication` | `skr apprun-api read-application` |
| `skr apprun-api packet-filter read` | `readApplicationPacketFilter` | `skr apprun-api read-application-packet-filter` |
| `skr apprun-api application status` | `readApplicationStatus` | `skr apprun-api read-application-status` |
| `skr apprun-api version read` | `readApplicationVersion` | `skr apprun-api read-application-version` |
| `skr apprun-api version status` | `readApplicationVersionStatus` | `skr apprun-api read-application-version-status` |
| `skr apprun-api user read` | `readUser` | `skr apprun-api read-user` |
| `skr apprun-api traffic update` | `updateApplicationTraffic` | `skr apprun-api update-application-traffic` |

**apprun-dedicated-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr apprun-dedicated-api application create` | `createApplication` | `skr apprun-dedicated-api create-application` |
| `skr apprun-dedicated-api version create` | `createApplicationVersion` | `skr apprun-dedicated-api create-application-version` |
| `skr apprun-dedicated-api auto-scaling-group create` | `createAutoScalingGroup` | `skr apprun-dedicated-api create-auto-scaling-group` |
| `skr apprun-dedicated-api certificate create` | `createCertificate` | `skr apprun-dedicated-api create-certificate` |
| `skr apprun-dedicated-api cluster create` | `createCluster` | `skr apprun-dedicated-api create-cluster` |
| `skr apprun-dedicated-api load-balancer create` | `createLoadBalancer` | `skr apprun-dedicated-api create-load-balancer` |
| `skr apprun-dedicated-api application delete` | `deleteApplication` | `skr apprun-dedicated-api delete-application` |
| `skr apprun-dedicated-api version delete` | `deleteApplicationVersion` | `skr apprun-dedicated-api delete-application-version` |
| `skr apprun-dedicated-api auto-scaling-group delete` | `deleteAutoScalingGroup` | `skr apprun-dedicated-api delete-auto-scaling-group` |
| `skr apprun-dedicated-api certificate delete` | `deleteCertificate` | `skr apprun-dedicated-api delete-certificate` |
| `skr apprun-dedicated-api cluster delete` | `deleteCluster` | `skr apprun-dedicated-api delete-cluster` |
| `skr apprun-dedicated-api load-balancer delete` | `deleteLoadBalancer` | `skr apprun-dedicated-api delete-load-balancer` |
| `skr apprun-dedicated-api application containers` | `listApplicationContainers` | `skr apprun-dedicated-api list-application-containers` |
| `skr apprun-dedicated-api version list` | `listApplicationVersions` | `skr apprun-dedicated-api list-application-versions` |
| `skr apprun-dedicated-api application list` | `listApplications` | `skr apprun-dedicated-api list-applications` |
| `skr apprun-dedicated-api auto-scaling-group list` | `listAutoScalingGroups` | `skr apprun-dedicated-api list-auto-scaling-groups` |
| `skr apprun-dedicated-api certificate list` | `listCertificates` | `skr apprun-dedicated-api list-certificates` |
| `skr apprun-dedicated-api cluster list` | `listClusters` | `skr apprun-dedicated-api list-clusters` |
| `skr apprun-dedicated-api load-balancer node list` | `listLoadBalancerNodes` | `skr apprun-dedicated-api list-load-balancer-nodes` |
| `skr apprun-dedicated-api service-class list-lb` | `listLoadBalancerServiceClasses` | `skr apprun-dedicated-api list-load-balancer-service-classes` |
| `skr apprun-dedicated-api load-balancer list` | `listLoadBalancers` | `skr apprun-dedicated-api list-load-balancers` |
| `skr apprun-dedicated-api worker-node list` | `listWorkerNodes` | `skr apprun-dedicated-api list-worker-nodes` |
| `skr apprun-dedicated-api service-class list-worker` | `listWorkerServiceClasses` | `skr apprun-dedicated-api list-worker-service-classes` |
| `skr apprun-dedicated-api application read` | `readApplication` | `skr apprun-dedicated-api read-application` |
| `skr apprun-dedicated-api version read` | `readApplicationVersion` | `skr apprun-dedicated-api read-application-version` |
| `skr apprun-dedicated-api auto-scaling-group read` | `readAutoScalingGroup` | `skr apprun-dedicated-api read-auto-scaling-group` |
| `skr apprun-dedicated-api certificate read` | `readCertificate` | `skr apprun-dedicated-api read-certificate` |
| `skr apprun-dedicated-api cluster read` | `readCluster` | `skr apprun-dedicated-api read-cluster` |
| `skr apprun-dedicated-api load-balancer read` | `readLoadBalancer` | `skr apprun-dedicated-api read-load-balancer` |
| `skr apprun-dedicated-api load-balancer node read` | `readLoadBalancerNode` | `skr apprun-dedicated-api read-load-balancer-node` |
| `skr apprun-dedicated-api worker-node read` | `readWorkerNode` | `skr apprun-dedicated-api read-worker-node` |
| `skr apprun-dedicated-api application update` | `updateApplication` | `skr apprun-dedicated-api update-application` |
| `skr apprun-dedicated-api certificate update` | `updateCertificate` | `skr apprun-dedicated-api update-certificate` |
| `skr apprun-dedicated-api cluster update` | `updateCluster` | `skr apprun-dedicated-api update-cluster` |
| `skr apprun-dedicated-api worker-node update` | `updateWorkerNodeDraining` | `skr apprun-dedicated-api update-worker-node-draining` |

**eventbus-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr eventbus-api process-configuration create` / `skr eventbus-api schedule create` / `skr eventbus-api trigger create` | `createCommonServiceItem` | `skr eventbus-api create-common-service-item` |
| `skr eventbus-api process-configuration delete` / `skr eventbus-api schedule delete` / `skr eventbus-api trigger delete` | `deleteCommonServiceItem` | `skr eventbus-api delete-common-service-item` |
| `skr eventbus-api process-configuration list` / `skr eventbus-api schedule list` / `skr eventbus-api trigger list` | `listCommonServiceItems` | `skr eventbus-api list-common-service-items` |
| `skr eventbus-api process-configuration read` / `skr eventbus-api schedule read` / `skr eventbus-api trigger read` | `readCommonServiceItem` | `skr eventbus-api read-common-service-item` |
| `skr eventbus-api process-configuration update-secret` | `setProcessConfigurationSecret` | `skr eventbus-api set-process-configuration-secret` |
| `skr eventbus-api process-configuration update` / `skr eventbus-api schedule update` / `skr eventbus-api trigger update` | `updateCommonServiceItem` | `skr eventbus-api update-common-service-item` |

**iam-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr iam-api sso link` | `assignSsoProfile` | `skr iam-api assign-sso-profile` |
| `skr iam-api service-policy is-enabled` | `checkServicePolicyStatus` | `skr iam-api check-service-policy-status` |
| `skr iam-api user-2fa clear-trusted-devices` | `clearTrustedDevices` | `skr iam-api clear-trusted-devices` |
| `skr iam-api project-api-key create` | `createApiKey` | `skr iam-api create-api-key` |
| `skr iam-api folder create` | `createFolder` | `skr iam-api create-folder` |
| `skr iam-api group create` | `createGroup` | `skr iam-api create-group` |
| `skr iam-api project create` | `createProject` | `skr iam-api create-project` |
| `skr iam-api scim create` | `createScimConfiguration` | `skr iam-api create-scim-configuration` |
| `skr iam-api service-principal create` | `createServicePrincipal` | `skr iam-api create-service-principal` |
| `skr iam-api sso create` | `createSsoProfile` | `skr iam-api create-sso-profile` |
| `skr iam-api user create` | `createUser` | `skr iam-api create-user` |
| `skr iam-api user-2fa deactivate-otp` | `deactivateOtp` | `skr iam-api deactivate-otp` |
| `skr iam-api project-api-key delete` | `deleteApiKey` | `skr iam-api delete-api-key` |
| `skr iam-api folder delete` | `deleteFolder` | `skr iam-api delete-folder` |
| `skr iam-api group delete` | `deleteGroup` | `skr iam-api delete-group` |
| `skr iam-api project delete` | `deleteProject` | `skr iam-api delete-project` |
| `skr iam-api scim delete` | `deleteScimConfiguration` | `skr iam-api delete-scim-configuration` |
| `skr iam-api user-2fa delete-security-key` | `deleteSecurityKey` | `skr iam-api delete-security-key` |
| `skr iam-api service-principal delete` | `deleteServicePrincipal` | `skr iam-api delete-service-principal` |
| `skr iam-api service-principal delete-key` | `deleteServicePrincipalKey` | `skr iam-api delete-service-principal-key` |
| `skr iam-api sso delete` | `deleteSsoProfile` | `skr iam-api delete-sso-profile` |
| `skr iam-api user-2fa delete-trusted-device` | `deleteTrustedDevice` | `skr iam-api delete-trusted-device` |
| `skr iam-api user delete` | `deleteUser` | `skr iam-api delete-user` |
| `skr iam-api service-policy disable` | `disableServicePolicy` | `skr iam-api disable-service-policy` |
| `skr iam-api service-principal disable-key` | `disableServicePrincipalKey` | `skr iam-api disable-service-principal-key` |
| `skr iam-api service-policy enable` | `enableServicePolicy` | `skr iam-api enable-service-policy` |
| `skr iam-api service-principal enable-key` | `enableServicePrincipalKey` | `skr iam-api enable-service-principal-key` |
| `skr iam-api service-principal issue-token` | `issueServicePrincipalToken` | `skr iam-api issue-service-principal-token` |
| `skr iam-api project-api-key list` | `listApiKeys` | `skr iam-api list-api-keys` |
| `skr iam-api folder list` | `listFolders` | `skr iam-api list-folders` |
| `skr iam-api group list` | `listGroups` | `skr iam-api list-groups` |
| `skr iam-api iam-role list` | `listIamRoles` | `skr iam-api list-iam-roles` |
| `skr iam-api id-role list` | `listIdRoles` | `skr iam-api list-id-roles` |
| `skr iam-api group read-memberships` | `listMemberships` | `skr iam-api list-memberships` |
| `skr iam-api project list` | `listProjects` | `skr iam-api list-projects` |
| `skr iam-api scim list` | `listScimConfigurations` | `skr iam-api list-scim-configurations` |
| `skr iam-api user-2fa list-security-keys` | `listSecurityKeys` | `skr iam-api list-security-keys` |
| `skr iam-api service-policy list-rule-templates` | `listServicePolicyRuleTemplates` | `skr iam-api list-service-policy-rule-templates` |
| `skr iam-api service-principal list-keys` | `listServicePrincipalKeys` | `skr iam-api list-service-principal-keys` |
| `skr iam-api service-principal list` | `listServicePrincipals` | `skr iam-api list-service-principals` |
| `skr iam-api sso list` | `listSsoProfiles` | `skr iam-api list-sso-profiles` |
| `skr iam-api user-2fa list-trusted-devices` | `listTrustedDevices` | `skr iam-api list-trusted-devices` |
| `skr iam-api user list` | `listUsers` | `skr iam-api list-users` |
| `skr iam-api folder move` | `moveFolders` | `skr iam-api move-folders` |
| `skr iam-api project move` | `moveProjects` | `skr iam-api move-projects` |
| `skr iam-api project-api-key read` | `readApiKey` | `skr iam-api read-api-key` |
| `skr iam-api auth read-auth-context` | `readAuthContext` | `skr iam-api read-auth-context` |
| `skr iam-api folder read` | `readFolder` | `skr iam-api read-folder` |
| `skr iam-api policy read-folder` | `readFolderIamPolicy` | `skr iam-api read-folder-iam-policy` |
| `skr iam-api group read` | `readGroup` | `skr iam-api read-group` |
| `skr iam-api iam-role read` | `readIamRole` | `skr iam-api read-iam-role` |
| `skr iam-api id-role read` | `readIdRole` | `skr iam-api read-id-role` |
| `skr iam-api organization read` | `readOrganization` | `skr iam-api read-organization` |
| `skr iam-api auth read-auth-conditions` | `readOrganizationAuthConditions` | `skr iam-api read-organization-auth-conditions` |
| `skr iam-api policy read-organization` | `readOrganizationIamPolicy` | `skr iam-api read-organization-iam-policy` |
| `skr iam-api id-policy read-organization` | `readOrganizationIdPolicy` | `skr iam-api read-organization-id-policy` |
| `skr iam-api auth read-password-policy` | `readOrganizationPasswordPolicy` | `skr iam-api read-organization-password-policy` |
| `skr iam-api organization read-service-policy` | `readOrganizationServicePolicy` | `skr iam-api read-organization-service-policy` |
| `skr iam-api project read` | `readProject` | `skr iam-api read-project` |
| `skr iam-api policy read-project` | `readProjectIamPolicy` | `skr iam-api read-project-iam-policy` |
| `skr iam-api scim read` | `readScimConfiguration` | `skr iam-api read-scim-configuration` |
| `skr iam-api user-2fa read-security-key` | `readSecurityKey` | `skr iam-api read-security-key` |
| `skr iam-api service-principal read` | `readServicePrincipal` | `skr iam-api read-service-principal` |
| `skr iam-api sso read` | `readSsoProfile` | `skr iam-api read-sso-profile` |
| `skr iam-api user read` | `readUser` | `skr iam-api read-user` |
| `skr iam-api scim regenerate-token` | `regenerateScimConfigurationToken` | `skr iam-api regenerate-scim-configuration-token` |
| `skr iam-api user register-email` | `registerEmail` | `skr iam-api register-email` |
| `skr iam-api sso unlink` | `unassignSsoProfile` | `skr iam-api unassign-sso-profile` |
| `skr iam-api user unregister-email` | `unregisterEmail` | `skr iam-api unregister-email` |
| `skr iam-api project-api-key update` | `updateApiKey` | `skr iam-api update-api-key` |
| `skr iam-api folder update` | `updateFolder` | `skr iam-api update-folder` |
| `skr iam-api policy update-folder` | `updateFolderIamPolicy` | `skr iam-api update-folder-iam-policy` |
| `skr iam-api group update` | `updateGroup` | `skr iam-api update-group` |
| `skr iam-api group update-memberships` | `updateMemberships` | `skr iam-api update-memberships` |
| `skr iam-api organization update` | `updateOrganization` | `skr iam-api update-organization` |
| `skr iam-api auth update-auth-conditions` | `updateOrganizationAuthConditions` | `skr iam-api update-organization-auth-conditions` |
| `skr iam-api policy update-organization` | `updateOrganizationIamPolicy` | `skr iam-api update-organization-iam-policy` |
| `skr iam-api id-policy update-organization` | `updateOrganizationIdPolicy` | `skr iam-api update-organization-id-policy` |
| `skr iam-api auth update-password-policy` | `updateOrganizationPasswordPolicy` | `skr iam-api update-organization-password-policy` |
| `skr iam-api organization update-service-policy` | `updateOrganizationServicePolicy` | `skr iam-api update-organization-service-policy` |
| `skr iam-api project update` | `updateProject` | `skr iam-api update-project` |
| `skr iam-api policy update-project` | `updateProjectIamPolicy` | `skr iam-api update-project-iam-policy` |
| `skr iam-api scim update` | `updateScimConfiguration` | `skr iam-api update-scim-configuration` |
| `skr iam-api service-principal update` | `updateServicePrincipal` | `skr iam-api update-service-principal` |
| `skr iam-api sso update` | `updateSsoProfile` | `skr iam-api update-sso-profile` |
| `skr iam-api user update` | `updateUser` | `skr iam-api update-user` |
| `skr iam-api service-principal upload-key` | `uploadServicePrincipalKey` | `skr iam-api upload-service-principal-key` |

**kms-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr kms-api key change-status` | `changeKeyStatus` | `skr kms-api change-key-status` |
| `skr kms-api key create` | `createKey` | `skr kms-api create-key` |
| `skr kms-api key decrypt` | `decryptDataWithKey` | `skr kms-api decrypt-data-with-key` |
| `skr kms-api key delete` | `deleteKey` | `skr kms-api delete-key` |
| `skr kms-api key encrypt` | `encryptDataWithKey` | `skr kms-api encrypt-data-with-key` |
| `skr kms-api key list` | `listKeys` | `skr kms-api list-keys` |
| `skr kms-api key read` | `readKey` | `skr kms-api read-key` |
| `skr kms-api key rotate` | `rotateKey` | `skr kms-api rotate-key` |
| `skr kms-api key schedule-destruction` | `scheduleKeyDestruction` | `skr kms-api schedule-key-destruction` |
| `skr kms-api key update` | `updateKey` | `skr kms-api update-key` |

**networking-suite-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr networking-suite-api interface-connection create` | `createInterfaceConnection` | `skr networking-suite-api create-interface-connection` |
| `skr networking-suite-api subnet create` | `createSubnet` | `skr networking-suite-api create-subnet` |
| `skr networking-suite-api subnet-group create` | `createSubnetGroup` | `skr networking-suite-api create-subnet-group` |
| `skr networking-suite-api interface-connection delete` | `deleteInterfaceConnection` | `skr networking-suite-api delete-interface-connection` |
| `skr networking-suite-api subnet delete` | `deleteSubnet` | `skr networking-suite-api delete-subnet` |
| `skr networking-suite-api subnet-group delete` | `deleteSubnetGroup` | `skr networking-suite-api delete-subnet-group` |
| `skr networking-suite-api address list` | `listAddresses` | `skr networking-suite-api list-addresses` |
| `skr networking-suite-api subnet-group list` | `listSubnetGroups` | `skr networking-suite-api list-subnet-groups` |
| `skr networking-suite-api subnet list` | `listSubnets` | `skr networking-suite-api list-subnets` |
| `skr networking-suite-api address read` | `readAddress` | `skr networking-suite-api read-address` |
| `skr networking-suite-api subnet read` | `readSubnet` | `skr networking-suite-api read-subnet` |
| `skr networking-suite-api subnet-group read` | `readSubnetGroup` | `skr networking-suite-api read-subnet-group` |
| `skr networking-suite-api subnet update` | `updateSubnet` | `skr networking-suite-api update-subnet` |
| `skr networking-suite-api subnet-group update` | `updateSubnetGroup` | `skr networking-suite-api update-subnet-group` |

**object-storage-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr object-storage-api account create` | `createAccount` | `skr object-storage-api create-account` |
| `skr object-storage-api account access-key create` | `createAccountKey` | `skr object-storage-api create-account-key` |
| `skr object-storage-api bucket create` | `createBucket` | `skr object-storage-api create-bucket` |
| `skr object-storage-api permission create` | `createPermission` | `skr object-storage-api create-permission` |
| `skr object-storage-api permission access-key create` | `createPermissionKey` | `skr object-storage-api create-permission-key` |
| `skr object-storage-api account delete` | `deleteAccount` | `skr object-storage-api delete-account` |
| `skr object-storage-api account access-key delete` | `deleteAccountKey` | `skr object-storage-api delete-account-key` |
| `skr object-storage-api bucket delete` | `deleteBucket` | `skr object-storage-api delete-bucket` |
| `skr object-storage-api bucket encryption disable` | `deleteBucketEncryption` | `skr object-storage-api delete-bucket-encryption` |
| `skr object-storage-api bucket replication disable` | `deleteBucketReplication` | `skr object-storage-api delete-bucket-replication` |
| `skr object-storage-api permission delete` | `deletePermission` | `skr object-storage-api delete-permission` |
| `skr object-storage-api permission access-key delete` | `deletePermissionKey` | `skr object-storage-api delete-permission-key` |
| `skr object-storage-api account read` | `getAccount` | `skr object-storage-api get-account` |
| `skr object-storage-api account access-key read` | `getAccountKey` | `skr object-storage-api get-account-key` |
| `skr object-storage-api account access-key list` | `getAccountKeys` | `skr object-storage-api get-account-keys` |
| `skr object-storage-api bucket encryption read` | `getBucketEncryption` | `skr object-storage-api get-bucket-encryption` |
| `skr object-storage-api site status bucket-metering` | `getBucketMetering` | `skr object-storage-api get-bucket-metering` |
| `skr object-storage-api bucket penalty` | `getBucketPenalty` | `skr object-storage-api get-bucket-penalty` |
| `skr object-storage-api bucket quota` | `getBucketQuota` | `skr object-storage-api get-bucket-quota` |
| `skr object-storage-api bucket replication read` | `getBucketReplication` | `skr object-storage-api get-bucket-replication` |
| `skr object-storage-api bucket usage` | `getBucketUsage` | `skr object-storage-api get-bucket-usage` |
| `skr object-storage-api site read` | `getCluster` | `skr object-storage-api get-cluster` |
| `skr object-storage-api site list` | `getClusters` | `skr object-storage-api get-clusters` |
| `skr object-storage-api permission read` | `getPermission` | `skr object-storage-api get-permission` |
| `skr object-storage-api permission access-key read` | `getPermissionKey` | `skr object-storage-api get-permission-key` |
| `skr object-storage-api permission access-key list` | `getPermissionKeys` | `skr object-storage-api get-permission-keys` |
| `skr object-storage-api permission list` | `getPermissions` | `skr object-storage-api get-permissions` |
| `skr object-storage-api site plans` | `getPlans` | `skr object-storage-api get-plans` |
| `skr object-storage-api site status quota` | `getQuota` | `skr object-storage-api get-quota` |
| `skr object-storage-api site status read` | `getStatus` | `skr object-storage-api get-status` |
| `skr object-storage-api bucket list` | `listBuckets` | `skr object-storage-api list-buckets` |
| `skr object-storage-api bucket replication enable` | `postBucketReplication` | `skr object-storage-api post-bucket-replication` |
| `skr object-storage-api bucket encryption enable` | `putBucketEncryption` | `skr object-storage-api put-bucket-encryption` |
| `skr object-storage-api permission update` | `updatePermission` | `skr object-storage-api update-permission` |

**secretmanager-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr secretmanager-api vault create` | `createVault` | `skr secretmanager-api create-vault` |
| `skr secretmanager-api secret create` | `createVaultSecret` | `skr secretmanager-api create-vault-secret` |
| `skr secretmanager-api vault delete` | `deleteVault` | `skr secretmanager-api delete-vault` |
| `skr secretmanager-api secret delete` | `deleteVaultSecret` | `skr secretmanager-api delete-vault-secret` |
| `skr secretmanager-api secret list` | `listVaultSecrets` | `skr secretmanager-api list-vault-secrets` |
| `skr secretmanager-api vault list` | `listVaults` | `skr secretmanager-api list-vaults` |
| `skr secretmanager-api vault read` | `readVault` | `skr secretmanager-api read-vault` |
| `skr secretmanager-api secret unveil` | `unveilSecret` | `skr secretmanager-api unveil-secret` |
| `skr secretmanager-api vault update` | `updateVault` | `skr secretmanager-api update-vault` |

**security-control-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr security-control-api automated-action create` | `automatedActionsCreate` | `skr security-control-api automated-actions-create` |
| `skr security-control-api automated-action delete` | `automatedActionsDelete` | `skr security-control-api automated-actions-delete` |
| `skr security-control-api automated-action list` | `automatedActionsList` | `skr security-control-api automated-actions-list` |
| `skr security-control-api automated-action read` | `automatedActionsRead` | `skr security-control-api automated-actions-read` |
| `skr security-control-api automated-action update` | `automatedActionsUpdate` | `skr security-control-api automated-actions-update` |
| `skr security-control-api evaluation-rule list` | `evaluationRulesList` | `skr security-control-api evaluation-rules-list` |
| `skr security-control-api evaluation-rule read` | `evaluationRulesRead` | `skr security-control-api evaluation-rules-read` |
| `skr security-control-api evaluation-rule update` | `evaluationRulesUpdate` | `skr security-control-api evaluation-rules-update` |
| `skr security-control-api activation create` | `projectActivationCreate` | `skr security-control-api project-activation-create` |
| `skr security-control-api activation read` | `projectActivationRead` | `skr security-control-api project-activation-read` |
| `skr security-control-api activation update` | `projectActivationUpdate` | `skr security-control-api project-activation-update` |

**service-endpoint-gateway-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr service-endpoint-gateway-api appliance read-interface` | `segInterface_getInterface` | `skr service-endpoint-gateway-api seg-interface-get-interface` |
| `skr service-endpoint-gateway-api appliance shutdown` | `segStatus_deletePowerStatus` | `skr service-endpoint-gateway-api seg-status-delete-power-status` |
| `skr service-endpoint-gateway-api appliance read-power-status` | `segStatus_getPowerStatus` | `skr service-endpoint-gateway-api seg-status-get-power-status` |
| `skr service-endpoint-gateway-api appliance reset` | `segStatus_resetPowerStatus` | `skr service-endpoint-gateway-api seg-status-reset-power-status` |
| `skr service-endpoint-gateway-api appliance power-on` | `segStatus_updatePowerStatus` | `skr service-endpoint-gateway-api seg-status-update-power-status` |
| `skr service-endpoint-gateway-api appliance apply` | `seg_apply` | `skr service-endpoint-gateway-api seg-apply` |
| `skr service-endpoint-gateway-api appliance create` | `seg_create` | `skr service-endpoint-gateway-api seg-create` |
| `skr service-endpoint-gateway-api appliance delete` | `seg_delete` | `skr service-endpoint-gateway-api seg-delete` |
| `skr service-endpoint-gateway-api appliance read` | `seg_get` | `skr service-endpoint-gateway-api seg-get` |
| `skr service-endpoint-gateway-api appliance list` | `seg_list` | `skr service-endpoint-gateway-api seg-list` |
| `skr service-endpoint-gateway-api appliance update` | `seg_update` | `skr service-endpoint-gateway-api seg-update` |

**simple-notification-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr simple-notification-api destination read-status` | `checkNotificationDestinationStatus` | `skr simple-notification-api check-notification-destination-status` |
| `skr simple-notification-api destination create` / `skr simple-notification-api group create` / `skr simple-notification-api routing create` | `createCommonServiceItem` | `skr simple-notification-api create-common-service-item` |
| `skr simple-notification-api destination delete` / `skr simple-notification-api group delete` / `skr simple-notification-api routing delete` | `deleteCommonServiceItem` | `skr simple-notification-api delete-common-service-item` |
| `skr simple-notification-api destination list` / `skr simple-notification-api group list` / `skr simple-notification-api routing list` | `listCommonServiceItems` | `skr simple-notification-api list-common-service-items` |
| `skr simple-notification-api history list` | `listNotificationHistories` | `skr simple-notification-api list-notification-histories` |
| `skr simple-notification-api routing list-source` | `listNotificationSources` | `skr simple-notification-api list-notification-sources` |
| `skr simple-notification-api destination read` / `skr simple-notification-api group read` / `skr simple-notification-api routing read` | `readCommonServiceItem` | `skr simple-notification-api read-common-service-item` |
| `skr simple-notification-api history read` | `readNotificationHistory` | `skr simple-notification-api read-notification-history` |
| `skr simple-notification-api routing reorder` | `reorderNotificationRouting` | `skr simple-notification-api reorder-notification-routing` |
| `skr simple-notification-api group send-message` | `sendNotification` | `skr simple-notification-api send-notification` |
| `skr simple-notification-api destination update` / `skr simple-notification-api group update` / `skr simple-notification-api routing update` | `updateCommonServiceItem` | `skr simple-notification-api update-common-service-item` |

**simplemq-api**
| 現在のコマンド名 | operationId | 変更後のコマンド名 |
| --- | --- | --- |
| `skr simplemq-api queue clear-messages` | `clearQueue` | `skr simplemq-api clear-queue` |
| `skr simplemq-api queue create` | `createQueue` | `skr simplemq-api create-queue` |
| `skr simplemq-api message delete` | `deleteMessage` | `skr simplemq-api delete-message` |
| `skr simplemq-api queue delete` | `deleteQueue` | `skr simplemq-api delete-queue` |
| `skr simplemq-api message extend-timeout` | `extendMessageTimeout` | `skr simplemq-api extend-message-timeout` |
| `skr simplemq-api queue list` | `listQueues` | `skr simplemq-api list-queues` |
| `skr simplemq-api queue count-messages` | `readMessageCount` | `skr simplemq-api read-message-count` |
| `skr simplemq-api queue read` | `readQueue` | `skr simplemq-api read-queue` |
| `skr simplemq-api message receive` | `receiveMessage` | `skr simplemq-api receive-message` |
| `skr simplemq-api queue rotate-api-key` | `rotateAPIKey` | `skr simplemq-api rotate-api-key` |
| `skr simplemq-api message send` | `sendMessage` | `skr simplemq-api send-message` |
| `skr simplemq-api queue config` | `updateQueue` | `skr simplemq-api update-queue` |

## 移行計画

一括変更ではなく、ドメインごとに独立して完了する段階で進めます。各フェーズは個別にレビューとマージができ、完了するごとに CI の許容範囲を狭めます。

| フェーズ | 内容 |
|---|---|
| 0 | この計画の承認と、決定内容の ADR 化 |
| 1 | CI 導入。全ドメインの現行コマンドを baseline に登録して許容し、新規コマンドは最初から機械派生を強制 |
| 2 | pilot として `simplemq-api` (12 コマンド) を変更し、手順を確立 |
| 3 以降 | 残りのドメインを変更 (`kms-api`、`secretmanager-api`、`security-control-api`、`service-endpoint-gateway-api`、`networking-suite-api`、`eventbus-api`、`apprun-api`、`simple-notification-api`、`object-storage-api`、`apprun-dedicated-api`、`iam-api`) |
| 最終 | baseline の廃止と PLAN.md の更新 |

各ドメインのフェーズでは次の手順を踏みます。

1. コマンド構造の変更。生成対象は `api/commands/*.json` の定義を `skr <domain>-api <operation>` の形に変更します。手書き対象はコマンド識別子を operation 名の派生形に変更します。
2. 再生成と追従。`make generate-api` と `make generate-wiring` で生成コードを更新し、手書きの実装とテストを追従させます。
3. E2E シナリオ (`test/e2e/<domain>-api/`) のコマンド名を更新し、既存の E2E 網羅用 `test/e2e/command-coverage/baseline.txt` の旧パスを新パスへ置き換えます。各パスの実装状況とライブ検証状況を区別し、ライブ再実行は別途行って結果を記録します。
4. ドキュメントの更新。`docs/design/<domain>-api.md` と `docs/manual/tutorials/<domain>-api.md` のコマンド名と CLI ↔ API 対応を更新します。
5. CI baseline から当該ドメインのエントリを削除します。
6. `make test`、`make lint-go`、`make lint-text` を実行します。

## CI 設計

`internal/cli` にテストを追加し、次を検証します。

1. `LeafCommandPaths` で全末端サブコマンドを列挙します。
2. 対象ドメインごとに、SDK の ogen 生成コードが公開する operation 定数を Go コードとして参照し、Kong と同じ camelcase 処理で期待パスを計算します。SDK の更新で operation が改名・削除された場合はコンパイルエラーとして検出します。
3. 移行が完了したドメインでは、末端パスの集合が期待パスの集合と一致することを検証します。
4. 移行中のドメインは、各末端パスが期待パスと一致するか、baseline に登録された現行パスと一致することを検証します。baseline には現行パスと対応する operation 定数を登録します。
5. baseline に登録できない新規コマンドは、最初から期待パスと一致する必要があります。

全ドメイン完了後に baseline の機構自体を削除します。生成されたコマンドと設定の一致は、既存の再生成フロー (`make generate-api` / `make generate-wiring` とその差分検証) が担保します。

## 影響

- 対象 12 ドメインの 276 コマンドはすべて名前が変わる破壊的変更です。CommonServiceItem 操作の統合で、コマンド数は 276 から 256 に減ります。既存のスクリプトやドキュメントは影響を受けます。
- `eventbus-api` と `simple-notification-api` では、リソースごとに分かれていた操作が 1 組のコマンドに統合されます。リソースの種別は `--provider-class` で指定します。
- リリース時にリリースノートで変更内容を告知します。
- `docs/manual/tutorials/` 配下のチュートリアルは、各ドメインのフェーズで新しいコマンド名に更新します。
