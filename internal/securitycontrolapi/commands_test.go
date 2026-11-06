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

package securitycontrolapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/security-control/apis/v1"
	"github.com/stretchr/testify/require"
)

const (
	testServicePrincipalID = "112700012345"
	testNotificationGroup  = "112700056789"
)

func testPtr[T any](value T) *T {
	return &value
}

func testEvaluationRuleRuntime() EvaluationRuleRuntime {
	return EvaluationRuleRuntime{
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}
}

func testAutomatedActionRuntime() AutomatedActionRuntime {
	return AutomatedActionRuntime{
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}
}

type stubEvaluationRuleAPI struct {
	updateID      string
	updateRequest *v1.EvaluationRuleInput
}

func (s *stubEvaluationRuleAPI) List(context.Context, v1.EvaluationRulesListParams) (*v1.EvaluationRulesListOK, error) {
	return nil, nil
}

func (s *stubEvaluationRuleAPI) Read(context.Context, string) (*v1.EvaluationRule, error) {
	return nil, nil
}

func (s *stubEvaluationRuleAPI) Update(_ context.Context, id string, request *v1.EvaluationRuleInput) (*v1.EvaluationRule, error) {
	s.updateID = id
	s.updateRequest = request
	return &v1.EvaluationRule{}, nil
}

type stubAutomatedActionAPI struct {
	createRequest *v1.AutomatedActionInput
	updateID      string
	updateRequest *v1.AutomatedActionInput
}

func (s *stubAutomatedActionAPI) List(context.Context, v1.AutomatedActionsListParams) (*v1.AutomatedActionsListOK, error) {
	return nil, nil
}

func (s *stubAutomatedActionAPI) Create(_ context.Context, request *v1.AutomatedActionInput) (*v1.AutomatedActionOutput, error) {
	s.createRequest = request
	return &v1.AutomatedActionOutput{}, nil
}

func (s *stubAutomatedActionAPI) Read(context.Context, string) (*v1.AutomatedActionOutput, error) {
	return nil, nil
}

func (s *stubAutomatedActionAPI) Update(_ context.Context, id string, request *v1.AutomatedActionInput) (*v1.AutomatedActionOutput, error) {
	s.updateID = id
	s.updateRequest = request
	return &v1.AutomatedActionOutput{}, nil
}

func (s *stubAutomatedActionAPI) Delete(context.Context, string) error { return nil }

func TestEvaluationRuleUpdateBuildsRuleFromFlags(t *testing.T) {
	stub := &stubEvaluationRuleAPI{}
	command := &EvaluationRuleUpdateCommand{
		ID:                 "server-no-public-ip",
		ServicePrincipalID: testPtr(testServicePrincipalID),
		IsEnabled:          testPtr(true),
		Targets:            []string{"is1b", "tk1a"},
		factory:            func() (EvaluationRuleAPI, error) { return stub, nil },
		runtime:            testEvaluationRuleRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.Equal(t, "server-no-public-ip", stub.updateID)
	require.NotNil(t, stub.updateRequest)
	require.True(t, stub.updateRequest.IsEnabled)
	require.Equal(t, "server-no-public-ip", string(stub.updateRequest.Rule.OneOf.Type))
	require.True(t, stub.updateRequest.Rule.OneOf.ServerNoPublicIP.Parameter.Set)
	require.Equal(t, testServicePrincipalID, stub.updateRequest.Rule.OneOf.ServerNoPublicIP.Parameter.Value.ServicePrincipalId.Value)
	require.Equal(t, []string{"is1b", "tk1a"}, stub.updateRequest.Rule.OneOf.ServerNoPublicIP.Parameter.Value.Zones)
}

func TestEvaluationRuleUpdateOmitsServicePrincipalIDWhenNotGiven(t *testing.T) {
	stub := &stubEvaluationRuleAPI{}
	command := &EvaluationRuleUpdateCommand{
		ID:        "server-no-public-ip",
		IsEnabled: testPtr(false),
		factory:   func() (EvaluationRuleAPI, error) { return stub, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.NotNil(t, stub.updateRequest)
	require.False(t, stub.updateRequest.IsEnabled)
	// SDK の入力補完は servicePrincipalId を常に設定するため、未指定のまま
	// 「未指定」を API に伝えることを確認する
	require.True(t, stub.updateRequest.Rule.OneOf.ServerNoPublicIP.Parameter.Set)
	require.False(t, stub.updateRequest.Rule.OneOf.ServerNoPublicIP.Parameter.Value.ServicePrincipalId.Set)
}

func TestEvaluationRuleUpdateUsesSitesForObjectStorageRule(t *testing.T) {
	stub := &stubEvaluationRuleAPI{}
	command := &EvaluationRuleUpdateCommand{
		ID:        "objectstorage-bucket-encryption-enabled",
		IsEnabled: testPtr(true),
		Targets:   []string{"isk01"},
		factory:   func() (EvaluationRuleAPI, error) { return stub, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.NotNil(t, stub.updateRequest)
	require.Equal(t, "objectstorage-bucket-encryption-enabled", string(stub.updateRequest.Rule.OneOf.Type))
	require.Equal(t, []string{"isk01"}, stub.updateRequest.Rule.OneOf.ObjectStorageBucketEncryptionEnabled.Parameter.Value.Sites)
}

func TestEvaluationRuleUpdateRejectsUnknownRuleID(t *testing.T) {
	command := &EvaluationRuleUpdateCommand{
		ID:        "unknown-rule",
		IsEnabled: testPtr(true),
		factory:   func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown-rule")
}

func TestEvaluationRuleUpdateRejectsFlagsForParameterlessRule(t *testing.T) {
	command := &EvaluationRuleUpdateCommand{
		ID:                 "addon-threat-detections",
		ServicePrincipalID: testPtr(testServicePrincipalID),
		IsEnabled:          testPtr(true),
		factory:            func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:            testEvaluationRuleRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--service-principal-id")

	command2 := &EvaluationRuleUpdateCommand{
		ID:        "addon-vulnerability-detections",
		IsEnabled: testPtr(true),
		Targets:   []string{"is1b"},
		factory:   func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	err = command2.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--targets")
}

func TestEvaluationRuleUpdateRejectsRequestAndFlags(t *testing.T) {
	command := &EvaluationRuleUpdateCommand{
		ID:        "server-no-public-ip",
		Request:   testPtr(`{"isEnabled":true}`),
		IsEnabled: testPtr(true),
		factory:   func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--request と個別フラグは併用できません")
}

func TestEvaluationRuleUpdateRequiresFlagsOrRequest(t *testing.T) {
	command := &EvaluationRuleUpdateCommand{
		ID:      "server-no-public-ip",
		factory: func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime: testEvaluationRuleRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--request または個別フラグが必要です")

	command2 := &EvaluationRuleUpdateCommand{
		ID:                 "server-no-public-ip",
		ServicePrincipalID: testPtr(testServicePrincipalID),
		factory:            func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:            testEvaluationRuleRuntime(),
	}
	err = command2.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--is-enabled が必要です")
}

func TestEvaluationRuleUpdateRejectsTargetsForRuleWithoutZones(t *testing.T) {
	command := &EvaluationRuleUpdateCommand{
		ID:        "elb-logging-enabled",
		IsEnabled: testPtr(true),
		Targets:   []string{"is1b"},
		factory:   func() (EvaluationRuleAPI, error) { return &stubEvaluationRuleAPI{}, nil },
		runtime:   testEvaluationRuleRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--targets")
}

func TestAutomatedActionCreateBuildsSimpleNotificationAction(t *testing.T) {
	stub := &stubAutomatedActionAPI{}
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:                testPtr("evaluation-rejected"),
			ExecutionCondition:  testPtr(`event.evaluationResult.status == "REJECTED"`),
			IsEnabled:           testPtr(true),
			ServicePrincipalID:  testPtr(testServicePrincipalID),
			NotificationGroupID: testPtr(testNotificationGroup),
		},
		factory: func() (AutomatedActionAPI, error) { return stub, nil },
		runtime: testAutomatedActionRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.NotNil(t, stub.createRequest)
	require.Equal(t, "evaluation-rejected", stub.createRequest.Name)
	require.True(t, stub.createRequest.IsEnabled)
	require.Equal(t, "simpleNotification", string(stub.createRequest.Action.OneOf.Type))
	require.Equal(t, testServicePrincipalID, stub.createRequest.Action.OneOf.ActionDefinitionSimpleNotification.ActionParameter.ServicePrincipalId)
	require.Equal(t, testNotificationGroup, stub.createRequest.Action.OneOf.ActionDefinitionSimpleNotification.ActionParameter.NotificationGroupId)

	body, err := json.Marshal(stub.createRequest)
	require.NoError(t, err)
	require.Contains(t, string(body), `"actionType":"simpleNotification"`)
	require.Contains(t, string(body), `"notificationGroupId":"`+testNotificationGroup+`"`)
}

func TestAutomatedActionCreateBuildsWorkflowsAction(t *testing.T) {
	stub := &stubAutomatedActionAPI{}
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:               testPtr("workflow-action"),
			Description:        testPtr("ワークフローを実行する"),
			ExecutionCondition: testPtr(`event.evaluationResult.status == "REJECTED"`),
			IsEnabled:          testPtr(false),
			ServicePrincipalID: testPtr(testServicePrincipalID),
			WorkflowID:         testPtr("workflow-1"),
			RevisionID:         testPtr(3),
			WorkflowName:       testPtr("run-name"),
			Args:               testPtr(`{"server_id": 12345}`),
		},
		factory: func() (AutomatedActionAPI, error) { return stub, nil },
		runtime: testAutomatedActionRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.NotNil(t, stub.createRequest)
	require.Equal(t, "workflows", string(stub.createRequest.Action.OneOf.Type))
	parameter := stub.createRequest.Action.OneOf.ActionDefinitionWorkflows.ActionParameter
	require.Equal(t, "workflow-1", parameter.WorkflowId)
	require.True(t, parameter.RevisionId.Set)
	require.Equal(t, 3, parameter.RevisionId.Value)
	require.Equal(t, "run-name", parameter.Name.Value)
	require.True(t, parameter.Args.Set)
	require.Equal(t, `{"server_id": 12345}`, parameter.Args.Value)
	require.False(t, parameter.RevisionAlias.Set)
}

func TestAutomatedActionCreateRejectsBothTargets(t *testing.T) {
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:                testPtr("name"),
			ExecutionCondition:  testPtr("condition"),
			IsEnabled:           testPtr(true),
			ServicePrincipalID:  testPtr(testServicePrincipalID),
			NotificationGroupID: testPtr(testNotificationGroup),
			WorkflowID:          testPtr("workflow-1"),
		},
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--notification-group-id と --workflow-id は併用できません")
}

func TestAutomatedActionCreateRequiresTarget(t *testing.T) {
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:               testPtr("name"),
			ExecutionCondition: testPtr("condition"),
			IsEnabled:          testPtr(true),
			ServicePrincipalID: testPtr(testServicePrincipalID),
		},
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--notification-group-id または --workflow-id が必要です")
}

func TestAutomatedActionCreateRejectsWorkflowsFlagWithNotificationGroup(t *testing.T) {
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:                testPtr("name"),
			ExecutionCondition:  testPtr("condition"),
			IsEnabled:           testPtr(true),
			ServicePrincipalID:  testPtr(testServicePrincipalID),
			NotificationGroupID: testPtr(testNotificationGroup),
			RevisionID:          testPtr(3),
		},
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--revision-id は --workflow-id と併用してください")
}

func TestAutomatedActionCreateRequiresFlags(t *testing.T) {
	command := &AutomatedActionCreateCommand{
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "個別フラグまたは --request が必要です")

	command2 := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name:                testPtr("name"),
			IsEnabled:           testPtr(true),
			ServicePrincipalID:  testPtr(testServicePrincipalID),
			NotificationGroupID: testPtr(testNotificationGroup),
		},
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err = command2.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--execution-condition が必要です")
}

func TestAutomatedActionCreateRejectsRequestAndFlags(t *testing.T) {
	command := &AutomatedActionCreateCommand{
		automatedActionFlags: automatedActionFlags{
			Name: testPtr("name"),
		},
		Request: testPtr(`{"name":"name"}`),
		factory: func() (AutomatedActionAPI, error) { return &stubAutomatedActionAPI{}, nil },
		runtime: testAutomatedActionRuntime(),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--request と個別フラグは併用できません")
}

func TestAutomatedActionUpdateCallsSDKWithID(t *testing.T) {
	stub := &stubAutomatedActionAPI{}
	command := &AutomatedActionUpdateCommand{
		ID: "action-1",
		automatedActionFlags: automatedActionFlags{
			Name:                testPtr("updated"),
			ExecutionCondition:  testPtr("condition"),
			IsEnabled:           testPtr(true),
			ServicePrincipalID:  testPtr(testServicePrincipalID),
			NotificationGroupID: testPtr(testNotificationGroup),
		},
		factory: func() (AutomatedActionAPI, error) { return stub, nil },
		runtime: testAutomatedActionRuntime(),
	}
	require.NoError(t, command.Run(&kong.Context{}))

	require.Equal(t, "action-1", stub.updateID)
	require.Equal(t, "updated", stub.updateRequest.Name)
}

func TestCommandsReferenceAllGroups(t *testing.T) {
	parser, err := kong.New(&Commands{}, kong.Name("skr"))
	require.NoError(t, err)
	var names []string
	for _, node := range parser.Model.Node.Leaves(false) {
		names = append(names, strings.TrimSpace(node.Path()))
	}
	joined := strings.Join(names, "\n")
	for _, path := range []string{
		"activation create", "activation read", "activation update",
		"evaluation-rule list", "evaluation-rule read", "evaluation-rule update",
		"automated-action list", "automated-action read", "automated-action create",
		"automated-action update", "automated-action delete",
	} {
		require.Contains(t, joined, path)
	}
}
