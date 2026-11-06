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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	securitycontrol "github.com/sacloud/sacloud-sdk-go/api/security-control"
	v1 "github.com/sacloud/sacloud-sdk-go/api/security-control/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	Activation      ActivationCommands      `cmd:"" name:"activation" help:"セキュリティコントロールの有効化状態を管理します。"`
	EvaluationRule  EvaluationRuleCommands  `cmd:"" name:"evaluation-rule" help:"評価ルールの設定を管理します。"`
	AutomatedAction AutomatedActionCommands `cmd:"" name:"automated-action" help:"自動アクションを管理します。"`
}

// NewActivationAPI は共有クライアントからセキュリティコントロールの有効化 API を構築します。
func NewActivationAPI(trace bool) (ActivationAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return securitycontrol.NewActivationOp(client), nil
}

// NewEvaluationRuleAPI は共有クライアントから評価ルール API を構築します。
func NewEvaluationRuleAPI(trace bool) (EvaluationRuleAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return securitycontrol.NewEvaluationRulesOp(client), nil
}

// NewAutomatedActionAPI は共有クライアントから自動アクション API を構築します。
func NewAutomatedActionAPI(trace bool) (AutomatedActionAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return securitycontrol.NewAutomatedActionsOp(client), nil
}

// newClient は共有のクライアント設定からセキュリティコントロール API のクライアントを構築します。
// エンドポイントは SDK の既定値に従い、SAKURA_ENDPOINTS_SECURITY_CONTROL で上書きできます。
func newClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return securitycontrol.NewClient(client)
}

func (r EvaluationRuleRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r EvaluationRuleRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

func (r AutomatedActionRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r AutomatedActionRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

// parameterlessRules はパラメータを持たない評価ルール ID です。--service-principal-id は
// API のパラメータに servicePrincipalId が存在しないため受け付けません。
var parameterlessRules = map[v1.EvaluationRuleID]bool{
	v1.EvaluationRuleIDAddonThreatDetections:        true,
	v1.EvaluationRuleIDAddonVulnerabilityDetections: true,
}

// targetAcceptingRules は zones または sites をパラメータに持つ評価ルール ID です。
var targetAcceptingRules = map[v1.EvaluationRuleID]bool{
	v1.EvaluationRuleIDServerNoPublicIP:                     true,
	v1.EvaluationRuleIDDiskEncryptionEnabled:                true,
	v1.EvaluationRuleIDDbaEncryptionEnabled:                 true,
	v1.EvaluationRuleIDDbaNoPublicIP:                        true,
	v1.EvaluationRuleIDObjectstorageBucketEncryptionEnabled: true,
	v1.EvaluationRuleIDAddonThreatDetectionEnabled:          true,
	v1.EvaluationRuleIDNosqlEncryptionEnabled:               true,
}

// EvaluationRuleUpdateCommand は評価ルールの更新コマンドです。--service-principal-id と
// --targets は SDK の入力補完への補助入力で API リクエストのフィールドに直接対応しないため、
// 手書きで実装しています。
type EvaluationRuleUpdateCommand struct {
	ID                 string   `arg:"" help:"更新する評価ルール ID。例: server-no-public-ip"`
	Request            *string  `help:"API の rule (evaluationRuleId と parameter) と isEnabled を含む JSON を直接または @path.json で指定します。個別フラグと併用できません。例: --request='{\"rule\":{\"evaluationRuleId\":\"server-no-public-ip\",\"parameter\":{\"servicePrincipalId\":\"112700012345\",\"zones\":[\"is1b\"]}},\"isEnabled\":true}'"`
	ServicePrincipalID *string  `name:"service-principal-id" help:"任意: 評価対象リソースに対する読み取り権限を持つサービスプリンシパルの ID。省略すると、セキュリティコントロール有効化時に設定されたサービスプリンシパルが使われます。addon-threat-detections と addon-vulnerability-detections では指定できません。"`
	IsEnabled          *bool    `name:"is-enabled" help:"必須: true の場合、ルールを有効化します。false の場合は無効化します。"`
	Targets            []string `name:"targets" help:"任意: 評価対象の指定。繰り返し指定できます (--targets is1b --targets tk1b)。ゾーン対象のルールではゾーン、objectstorage-bucket-encryption-enabled ではオブジェクトストレージのサイトを指定します。省略すると全ゾーン・全サイトが対象になります。パラメータに zones や sites を持たないルールでは指定できません。"`
	factory            EvaluationRuleAPIFactory
	runtime            EvaluationRuleRuntime
}

func (c *EvaluationRuleUpdateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request, err := c.buildRequest()
	if err != nil {
		return err
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	updated, err := op.Update(context.Background(), c.ID, request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, updated)
}

// buildRequest は --request と個別フラグのどちらの経路でも API リクエストを組み立てます。
// 両経路は相互排他です。
func (c *EvaluationRuleUpdateCommand) buildRequest() (*v1.EvaluationRuleInput, error) {
	if c.Request != nil {
		// --targets= のように空配列の明示指定も Kong は非 nil のスライスを設定するため、
		// 長さではなく nil で「未指定」を判定する
		if c.ServicePrincipalID != nil || c.IsEnabled != nil || c.Targets != nil {
			return nil, fmt.Errorf("--request と個別フラグは併用できません")
		}
		if c.runtime.DecodeRequest == nil {
			return nil, fmt.Errorf("API リクエスト処理が設定されていません")
		}
		var request v1.EvaluationRuleInput
		if err := c.runtime.DecodeRequest(*c.Request, &request); err != nil {
			return nil, err
		}
		return &request, nil
	}
	if c.ServicePrincipalID == nil && c.IsEnabled == nil && c.Targets == nil {
		return nil, fmt.Errorf("--request または個別フラグが必要です")
	}
	if c.IsEnabled == nil {
		return nil, fmt.Errorf("--is-enabled が必要です")
	}
	if err := validateEvaluationRuleID(c.ID); err != nil {
		return nil, err
	}
	if err := validateEvaluationRuleFlags(c); err != nil {
		return nil, err
	}
	params := &securitycontrol.EvaluationRuleInputParams{
		ID:      c.ID,
		Targets: c.Targets,
		Enabled: *c.IsEnabled,
	}
	if c.ServicePrincipalID != nil {
		params.ServicePrincipalID = *c.ServicePrincipalID
	}
	input := securitycontrol.SetupEvaluationRuleInput(params)
	if c.ServicePrincipalID == nil {
		// SDK の入力補完は servicePrincipalId を常に設定するため、未指定の場合は
		// API の「未指定時に有効化時のサービスプリンシパルが使われる」仕様に合わせて省略する
		clearParameterServicePrincipalID(&input.Rule)
	}
	return input, nil
}

// validateEvaluationRuleID は評価ルール ID を検証します。SDK の入力補完は未知の
// ルール ID で panic するため、API に送信する前に CLI で検証します。
func validateEvaluationRuleID(id string) error {
	for _, known := range v1.EvaluationRuleID("").AllValues() {
		if known == v1.EvaluationRuleID(id) {
			return nil
		}
	}
	return fmt.Errorf("評価ルール ID %q は不正です。list で確認できる evaluationRuleId を指定してください", id)
}

// validateEvaluationRuleFlags はルール ID と個別フラグの組み合わせを検証します。
// SDK の入力補完はパラメータを持たないルールへの対応しない値を黙って無視するため、
// CLI で明示的にエラーにします。
func validateEvaluationRuleFlags(command *EvaluationRuleUpdateCommand) error {
	id := v1.EvaluationRuleID(command.ID)
	if command.ServicePrincipalID != nil && parameterlessRules[id] {
		return fmt.Errorf("--service-principal-id は %s では指定できません。このルールにはパラメータがありません", id)
	}
	if command.Targets != nil && !targetAcceptingRules[id] {
		return fmt.Errorf("--targets は %s では指定できません。このルールのパラメータには zones や sites がありません", id)
	}
	return nil
}

// clearParameterServicePrincipalID はパラメータの servicePrincipalId を省略状態にします。
// SDK の入力補完は未指定でも空文字列を設定するため、有効化時に設定されたサービスプリンシパルを
// 使う「未指定」の意味を API に正しく伝えるために呼び出します。
func clearParameterServicePrincipalID(rule *v1.EvaluationRuleUnion) {
	zero := v1.OptString{}
	switch rule.OneOf.Type {
	case v1.ServerNoPublicIPEvaluationRuleUnionSum:
		rule.OneOf.ServerNoPublicIP.Parameter.Value.ServicePrincipalId = zero
	case v1.DiskEncryptionEnabledEvaluationRuleUnionSum:
		rule.OneOf.DiskEncryptionEnabled.Parameter.Value.ServicePrincipalId = zero
	case v1.DbaEncryptionEnabledEvaluationRuleUnionSum:
		rule.OneOf.DbaEncryptionEnabled.Parameter.Value.ServicePrincipalId = zero
	case v1.DbaNoPublicIPEvaluationRuleUnionSum:
		rule.OneOf.DbaNoPublicIP.Parameter.Value.ServicePrincipalId = zero
	case v1.ObjectStorageBucketACLChangedEvaluationRuleUnionSum:
		rule.OneOf.ObjectStorageBucketACLChanged.Parameter.Value.ServicePrincipalId = zero
	case v1.ObjectStorageBucketEncryptionEnabledEvaluationRuleUnionSum:
		rule.OneOf.ObjectStorageBucketEncryptionEnabled.Parameter.Value.ServicePrincipalId = zero
	case v1.AddonDatalakeNoPublicAccessEvaluationRuleUnionSum:
		rule.OneOf.AddonDatalakeNoPublicAccess.Parameter.Value.ServicePrincipalId = zero
	case v1.AddonDwhNoPublicAccessEvaluationRuleUnionSum:
		rule.OneOf.AddonDwhNoPublicAccess.Parameter.Value.ServicePrincipalId = zero
	case v1.AddonThreatDetectionEnabledEvaluationRuleUnionSum:
		rule.OneOf.AddonThreatDetectionEnabled.Parameter.Value.ServicePrincipalId = zero
	case v1.ELBLoggingEnabledEvaluationRuleUnionSum:
		rule.OneOf.ELBLoggingEnabled.Parameter.Value.ServicePrincipalId = zero
	case v1.IAMMemberOperationDetectedEvaluationRuleUnionSum:
		rule.OneOf.IAMMemberOperationDetected.Parameter.Value.ServicePrincipalId = zero
	case v1.NoSQLEncryptionEnabledEvaluationRuleUnionSum:
		rule.OneOf.NoSQLEncryptionEnabled.Parameter.Value.ServicePrincipalId = zero
	}
}

// automatedActionFlags は自動アクションの作成・更新で共有する個別フラグです。
// アクションの実行先は --notification-group-id と --workflow-id のどちらか一方で選択します。
type automatedActionFlags struct {
	Name                *string `name:"name" help:"必須: 自動アクションの名称。"`
	Description         *string `name:"description" help:"任意: 自動アクションの説明。"`
	ExecutionCondition  *string `name:"execution-condition" help:"必須: 自動アクションを実行する条件式。CEL (Common Expression Language) 形式で記述します。例: event.evaluationResult.status == \"REJECTED\""`
	IsEnabled           *bool   `name:"is-enabled" help:"必須: true の場合、自動アクションを有効化します。"`
	ServicePrincipalID  *string `name:"service-principal-id" help:"必須: アクションの実行に使うサービスプリンシパル ID。simpleNotification には 作成・削除 ロール、workflows には ワークフロー ロールが必要です。"`
	NotificationGroupID *string `name:"notification-group-id" help:"simpleNotification アクションで通知する通知先グループ ID。--workflow-id と併用できません。"`
	WorkflowID          *string `name:"workflow-id" help:"workflows アクションで実行するワークフロー ID。--notification-group-id と併用できません。"`
	RevisionID          *int    `name:"revision-id" help:"任意: 実行するワークフローのリビジョン ID。--workflow-id と併用します。省略すると最新のリビジョンが使われます。"`
	RevisionAlias       *string `name:"revision-alias" help:"任意: 実行するリビジョンのエイリアス。--workflow-id と併用します。省略すると最新のリビジョンが使われます。"`
	WorkflowName        *string `name:"workflow-name" help:"任意: ワークフロー実行名。--workflow-id と併用します。"`
	Args                *string `name:"args" help:"任意: 実行時にワークフローへ渡す引数の JSON。--workflow-id と併用します。引数を要求するワークフローでは必須です。引数に秘密情報を含められるため、値を直接指定することはできません。@path で保護したファイルから、- で標準入力から読み込みます。例: --args @args.json"`
}

// AutomatedActionCreateCommand は自動アクションの作成コマンドです。アクション定義の
// 実行先種別を個別フラグから一意に組み立てるため、手書きで実装しています。
type AutomatedActionCreateCommand struct {
	automatedActionFlags
	Request *string `help:"API リクエスト JSON を @path.json または - (標準入力) で指定します。個別フラグと併用できません。workflows アクションの実行引数に秘密情報を含められるため、JSON を直接指定することはできません。action.actionType に simpleNotification または workflows を指定します。"`
	factory AutomatedActionAPIFactory
	runtime AutomatedActionRuntime
}

func (c *AutomatedActionCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request, err := automatedActionRequest(c.Request, &c.automatedActionFlags, c.runtime.DecodeRequest, os.Stdin)
	if err != nil {
		return err
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	created, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, created)
}

// AutomatedActionUpdateCommand は自動アクションの更新コマンドです。アクション定義の
// 実行先種別を個別フラグから一意に組み立てるため、手書きで実装しています。
type AutomatedActionUpdateCommand struct {
	ID string `arg:"" help:"更新する自動アクション ID。list の出力にある automatedActionId。"`
	automatedActionFlags
	Request *string `help:"API リクエスト JSON を @path.json または - (標準入力) で指定します。個別フラグと併用できません。workflows アクションの実行引数に秘密情報を含められるため、JSON を直接指定することはできません。name、action、executionCondition、isEnabled は自動アクション全体を置き換えます。"`
	factory AutomatedActionAPIFactory
	runtime AutomatedActionRuntime
}

func (c *AutomatedActionUpdateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request, err := automatedActionRequest(c.Request, &c.automatedActionFlags, c.runtime.DecodeRequest, os.Stdin)
	if err != nil {
		return err
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	updated, err := op.Update(context.Background(), c.ID, request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, updated)
}

// automatedActionRequest は --request と個別フラグのどちらの経路でも
// API リクエストを組み立てます。両経路は相互排他です。
// --request はワークフロー引数に秘密情報を含められるため、@path か - (標準入力) のみを
// 受け付け、インラインの JSON は拒否します。
func automatedActionRequest(requestJSON *string, flags *automatedActionFlags, decode func(string, any) error, stdin io.Reader) (*v1.AutomatedActionInput, error) {
	if requestJSON != nil {
		if flags.flagsSet() {
			return nil, fmt.Errorf("--request と個別フラグは併用できません")
		}
		data, err := readProtectedInput(*requestJSON, stdin, "--request", "workflows アクションの実行引数に秘密情報を含められるため、JSON を直接指定することはできません")
		if err != nil {
			return nil, err
		}
		if decode == nil {
			return nil, fmt.Errorf("API リクエスト処理が設定されていません")
		}
		var request v1.AutomatedActionInput
		if err := decode(string(data), &request); err != nil {
			return nil, err
		}
		return &request, nil
	}
	if !flags.flagsSet() {
		return nil, fmt.Errorf("個別フラグまたは --request が必要です")
	}
	if err := flags.validateRequired(); err != nil {
		return nil, err
	}
	return flags.buildInput(stdin)
}

// readProtectedInput は @path で始まる入力を保護したファイルから、- を標準入力から
// 読み込みます。それ以外の形式は、秘密情報を含められる入力をコマンドライン引数に
// 直接渡させないためのエラーにします。flag と reason はエラーメッセージの説明に使います。
func readProtectedInput(input string, stdin io.Reader, flag string, reason string) ([]byte, error) {
	switch {
	case input == "-":
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("%s の標準入力の読み込みに失敗しました: %w", flag, err)
		}
		return data, nil
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		//nolint:gosec // パスは --request / --args でユーザーが明示的に指定したものです。
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s のファイル %q の読み込みに失敗しました: %w", flag, path, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("%s には @path または - (標準入力) を指定してください。%s", flag, reason)
	}
}

// flagsSet は --request 以外の個別フラグが指定されているかを返します。
func (f *automatedActionFlags) flagsSet() bool {
	return f.Name != nil || f.Description != nil || f.ExecutionCondition != nil ||
		f.IsEnabled != nil || f.ServicePrincipalID != nil || f.NotificationGroupID != nil ||
		f.WorkflowID != nil || f.RevisionID != nil || f.RevisionAlias != nil ||
		f.WorkflowName != nil || f.Args != nil
}

// validateRequired は個別フラグの経路で必須の値を検証します。
func (f *automatedActionFlags) validateRequired() error {
	for _, required := range []struct {
		name  string
		value *string
	}{
		{"--name", f.Name},
		{"--execution-condition", f.ExecutionCondition},
		{"--service-principal-id", f.ServicePrincipalID},
	} {
		if required.value == nil {
			return fmt.Errorf("%s が必要です", required.name)
		}
	}
	if f.IsEnabled == nil {
		return fmt.Errorf("--is-enabled が必要です")
	}
	return nil
}

// buildInput は個別フラグから API リクエストを組み立てます。アクションの実行先は
// --notification-group-id と --workflow-id のどちらが指定されたかで決まり、両方または
// どちらも指定された場合はエラーにします。
func (f *automatedActionFlags) buildInput(stdin io.Reader) (*v1.AutomatedActionInput, error) {
	if f.NotificationGroupID != nil && f.WorkflowID != nil {
		return nil, fmt.Errorf("--notification-group-id と --workflow-id は併用できません。どちらか一方を指定してください")
	}
	if f.NotificationGroupID == nil && f.WorkflowID == nil {
		return nil, fmt.Errorf("--notification-group-id または --workflow-id が必要です")
	}
	if f.WorkflowID == nil {
		for _, restricted := range []struct {
			name  string
			value bool
		}{
			{"--revision-id", f.RevisionID != nil},
			{"--revision-alias", f.RevisionAlias != nil},
			{"--workflow-name", f.WorkflowName != nil},
			{"--args", f.Args != nil},
		} {
			if restricted.value {
				return nil, fmt.Errorf("%s は --workflow-id と併用してください", restricted.name)
			}
		}
	}
	input := &v1.AutomatedActionInput{
		Name:               *f.Name,
		ExecutionCondition: *f.ExecutionCondition,
		IsEnabled:          *f.IsEnabled,
	}
	if f.Description != nil {
		input.Description = v1.NewOptString(*f.Description)
	}
	if f.WorkflowID != nil {
		workflows := v1.SakuraWorkflows{
			ServicePrincipalId: *f.ServicePrincipalID,
			WorkflowId:         *f.WorkflowID,
		}
		if f.RevisionID != nil {
			workflows.RevisionId = v1.NewOptInt(*f.RevisionID)
		}
		if f.RevisionAlias != nil {
			workflows.RevisionAlias = v1.NewOptString(*f.RevisionAlias)
		}
		if f.Args != nil {
			argsValue, err := workflowArgsValue(*f.Args, stdin)
			if err != nil {
				return nil, err
			}
			workflows.Args = v1.NewOptString(argsValue)
		}
		if f.WorkflowName != nil {
			workflows.Name = v1.NewOptString(*f.WorkflowName)
		}
		input.Action = v1.ActionDefinition{
			OneOf: v1.NewActionDefinitionWorkflowsActionDefinitionSum(v1.ActionDefinitionWorkflows{
				ActionType:      v1.ActionDefinitionWorkflowsActionTypeWorkflows,
				ActionParameter: workflows,
			}),
		}
	} else {
		input.Action = v1.ActionDefinition{
			OneOf: v1.NewActionDefinitionSimpleNotificationActionDefinitionSum(v1.ActionDefinitionSimpleNotification{
				ActionType: v1.ActionDefinitionSimpleNotificationActionTypeSimpleNotification,
				ActionParameter: v1.SakuraSimpleNotification{
					ServicePrincipalId:  *f.ServicePrincipalID,
					NotificationGroupId: *f.NotificationGroupID,
				},
			}),
		}
	}
	return input, nil
}

// workflowArgsValue は --args の入力を読み込みます。ワークフロー引数に秘密情報を
// 含められるため、@path で保護したファイルか - (標準入力) のみを受け付け、
// インラインの値は拒否します。
func workflowArgsValue(input string, stdin io.Reader) (string, error) {
	data, err := readProtectedInput(input, stdin, "--args", "ワークフロー引数に秘密情報を含められるため、値を直接指定することはできません")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
