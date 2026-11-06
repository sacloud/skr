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
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// automatedActionCondition はテストで使う実行条件式です。CEL 形式で記述します。
	automatedActionCondition = `event.evaluationResult.status == "REJECTED"`
	// automatedActionNamePrefix はテスト用アクションの名前の接頭辞です。ランダムな接尾辞と
	// ともに使い、過去の実行のリソースと衝突しないようにします。
	automatedActionNamePrefix = "skr-e2e-security-control-action-"
)

// TestScenarioAutomatedAction は security-control-api automated-action コマンドのライブ E2E です。
// 一覧を取得した後、無効状態の自動アクションを作成して読み取り・一覧・更新を確認し、
// 削除と不在確認までを行います。実行パラメータには既存の通知先グループを参照のみで使い、
// 評価ルールが無効のため通知は発生しません。後片付けで作成したアクションを ID 照合のうえ
// 削除し、一覧に残っていないことを確認します。課金対象リソースは作成しません。
func TestScenarioAutomatedAction(t *testing.T) {
	// 通知先グループは既存のグループを参照のみで使うため、環境変数で指定します。
	groupID := os.Getenv("SKR_E2E_SECURITY_CONTROL_NOTIFICATION_GROUP_ID")
	if groupID == "" {
		t.Skipf("SKR_E2E_SECURITY_CONTROL_NOTIFICATION_GROUP_ID is not set, skipping E2E test")
	}

	h := helper.New(t, "security-control-api-automated-action",
		"自動アクションの作成・読み取り・一覧・更新・削除が正しく動作することを確認する")

	// アクションの実行パラメータに使うサービスプリンシパル ID を取得する
	activation := readActivation(t, h, "アクションの実行パラメータに使うサービスプリンシパル ID を取得する")
	sp := activation.ServicePrincipalId
	require.NotEmpty(t, sp, "activation should have a service principal ID: %+v", activation)

	actionName := automatedActionNamePrefix + helper.RandomSuffix(t)

	// 作成前に同名のアクションが存在しないことを確認する
	before := listAutomatedActions(t, h, "作成前に同名のアクションが存在しないことを確認する")
	for _, item := range before {
		require.NotEqual(t, actionName, item.Name, "action name %q already exists; refusing to create it", actionName)
	}

	// 無効状態で自動アクションを作成する (評価ルールが無効のため通知は発生しない)
	created := createAutomatedAction(t, h, actionName, automatedActionCondition, sp, groupID, false, "自動アクションを無効状態で作成する")
	require.NotEmpty(t, created.AutomatedActionId, "created action should have an ID: %+v", created)
	require.Equal(t, actionName, created.Name, "created action should have the requested name: %+v", created)
	require.Equal(t, "simpleNotification", created.Action.ActionType, "created action should be simpleNotification: %+v", created)
	require.Equal(t, groupID, created.Action.ActionParameter.NotificationGroupId, "created action should reference the notification group: %+v", created)
	require.Equal(t, sp, created.Action.ActionParameter.ServicePrincipalId, "created action should reference the service principal: %+v", created)
	require.Equal(t, automatedActionCondition, created.ExecutionCondition, "created action should have the requested execution condition: %+v", created)
	require.False(t, created.IsEnabled, "created action should be disabled: %+v", created)
	actionID := created.AutomatedActionId

	// 後片付け: 残存していれば ID・名称・指定内容を照合して削除し、一覧に残っていないことを確認する
	defer func() {
		cleanupAutomatedAction(t, h, actionID, actionName, automatedActionCondition, sp, groupID)
		remaining := listAutomatedActions(t, h, "削除後にアクションが一覧に残っていないことを確認する")
		for _, item := range remaining {
			require.NotEqual(t, actionID, item.AutomatedActionId, "action %q should not remain after deletion", actionID)
		}
	}()

	// 作成したアクションを読み取る
	read := readAutomatedAction(t, h, actionID, "作成したアクションを読み取る")
	require.Equal(t, actionID, read.AutomatedActionId, "read response should have the created action ID: %+v", read)
	require.Equal(t, actionName, read.Name, "read response should have the created action name: %+v", read)
	require.Equal(t, "simpleNotification", read.Action.ActionType, "read response should be simpleNotification: %+v", read)
	require.False(t, read.IsEnabled, "read response should be disabled: %+v", read)

	// 作成したアクションが一覧に含まれることを確認する
	listed := listAutomatedActions(t, h, "作成したアクションが一覧に含まれることを確認する")
	found := false
	for _, item := range listed {
		if item.AutomatedActionId == actionID {
			found = true
			require.Equal(t, actionName, item.Name, "listed action should have the created name: %+v", item)
		}
	}
	require.True(t, found, "created action %q should be listed: %+v", actionID, listed)

	// update でアクション全体を置き換えても無効状態を維持する。有効なルールの評価結果が
	// 実行条件に一致しても通知が発生しないため、このシナリオでは有効化しない
	updated := updateAutomatedAction(t, h, actionID, actionName, automatedActionCondition, sp, groupID, false, "update でアクション全体を置き換えても無効状態を維持する")
	require.Equal(t, actionID, updated.AutomatedActionId, "update response should have the same action ID: %+v", updated)
	require.False(t, updated.IsEnabled, "update should keep the action disabled: %+v", updated)
	require.Equal(t, actionName, updated.Name, "update should keep the name: %+v", updated)
	require.Equal(t, automatedActionCondition, updated.ExecutionCondition, "update should keep the execution condition: %+v", updated)
	require.Equal(t, "simpleNotification", updated.Action.ActionType, "update should keep the action type: %+v", updated)
	require.Equal(t, groupID, updated.Action.ActionParameter.NotificationGroupId, "update should keep the notification group: %+v", updated)
	require.Equal(t, sp, updated.Action.ActionParameter.ServicePrincipalId, "update should keep the service principal: %+v", updated)

	// 削除する前に読み取りで ID・名称・指定内容を再照合する
	verified := readAutomatedAction(t, h, actionID, "削除前に ID・名称・指定内容を再照合する")
	require.Equal(t, actionID, verified.AutomatedActionId, "read response should have the action ID: %+v", verified)
	require.Equal(t, actionName, verified.Name, "read response should have the action name: %+v", verified)
	require.Equal(t, automatedActionCondition, verified.ExecutionCondition, "read response should have the execution condition: %+v", verified)
	require.Equal(t, "simpleNotification", verified.Action.ActionType, "read response should be simpleNotification: %+v", verified)
	require.Equal(t, groupID, verified.Action.ActionParameter.NotificationGroupId, "read response should reference the notification group: %+v", verified)
	require.Equal(t, sp, verified.Action.ActionParameter.ServicePrincipalId, "read response should reference the service principal: %+v", verified)
	require.False(t, verified.IsEnabled, "read response should be disabled: %+v", verified)

	deleteAutomatedAction(t, h, actionID, "作成したアクションを削除する")

	// 一覧に残っていないことを確認する
	remaining := listAutomatedActions(t, h, "削除後にアクションが一覧に残っていないことを確認する")
	for _, item := range remaining {
		require.NotEqual(t, actionID, item.AutomatedActionId, "action %q should not remain after deletion", actionID)
	}
}

// automatedActionItem は security-control-api automated-action コマンドの JSON 出力です。
type automatedActionItem struct {
	AutomatedActionId string `json:"automatedActionId"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Action            struct {
		ActionType      string `json:"actionType"`
		ActionParameter struct {
			ServicePrincipalId  string `json:"servicePrincipalId"`
			NotificationGroupId string `json:"notificationGroupId"`
		} `json:"actionParameter"`
	} `json:"action"`
	ExecutionCondition string `json:"executionCondition"`
	IsEnabled          bool   `json:"isEnabled"`
}

// listAutomatedActions は skr CLI で自動アクションの一覧を取得します。
func listAutomatedActions(t *testing.T, h *helper.Helper, description string) []automatedActionItem {
	t.Helper()

	data := h.Call(t, description, "security-control-api", "automated-action", "list")
	var out struct {
		Items []automatedActionItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &out), "decode automated-action list response: %s", string(data))
	return out.Items
}

// readAutomatedAction は skr CLI で自動アクションを読み取ります。
func readAutomatedAction(t *testing.T, h *helper.Helper, actionID string, description string) automatedActionItem {
	t.Helper()

	data := h.Call(t, description, "security-control-api", "automated-action", "read", actionID)
	var out automatedActionItem
	require.NoError(t, json.Unmarshal(data, &out), "decode automated-action read response: %s", string(data))
	require.Equal(t, actionID, out.AutomatedActionId, "read response should have the requested action ID: %s", string(data))
	return out
}

// createAutomatedAction は skr CLI で自動アクションを作成します。
func createAutomatedAction(t *testing.T, h *helper.Helper, actionName, condition, sp, groupID string, enabled bool, description string) automatedActionItem {
	t.Helper()

	data := h.Call(t, description,
		"security-control-api", "automated-action", "create",
		"--name", actionName,
		"--execution-condition", condition,
		"--service-principal-id", sp,
		"--notification-group-id", groupID,
		"--is-enabled="+strconv.FormatBool(enabled),
	)
	var out automatedActionItem
	require.NoError(t, json.Unmarshal(data, &out), "decode automated-action create response: %s", string(data))
	return out
}

// updateAutomatedAction は skr CLI で自動アクションを更新します。更新リクエストは
// アクション全体を置き換えるため、必要なフィールドをすべて指定します。
func updateAutomatedAction(t *testing.T, h *helper.Helper, actionID, actionName, condition, sp, groupID string, enabled bool, description string) automatedActionItem {
	t.Helper()

	data := h.Call(t, description,
		"security-control-api", "automated-action", "update", actionID,
		"--name", actionName,
		"--execution-condition", condition,
		"--service-principal-id", sp,
		"--notification-group-id", groupID,
		"--is-enabled="+strconv.FormatBool(enabled),
	)
	var out automatedActionItem
	require.NoError(t, json.Unmarshal(data, &out), "decode automated-action update response: %s", string(data))
	return out
}

// deleteAutomatedAction は skr CLI で自動アクションを削除します。
func deleteAutomatedAction(t *testing.T, h *helper.Helper, actionID string, description string) {
	t.Helper()

	h.Call(t, description, "security-control-api", "automated-action", "delete", actionID)
}

// cleanupAutomatedAction は後片付けでアクションを削除します。読み取りで保持した ID・名称と
// 作成時に指定した内容を照合してから削除し、不在なら変更せずに終了します。
func cleanupAutomatedAction(t *testing.T, h *helper.Helper, actionID, actionName, condition, sp, groupID string) {
	t.Helper()

	data, stderr, err := h.CallAllowingFailure(t, "後片付け: アクションの残存を確認する",
		"security-control-api", "automated-action", "read", actionID)
	if err != nil {
		if automatedActionNotFound(stderr) {
			// 不在なら変更せずに終了する
			return
		}
		require.NoError(t, err, "cleanup: read automated action %q failed: %s", actionID, stderr)
	}
	var read automatedActionItem
	require.NoError(t, json.Unmarshal(data, &read), "cleanup: decode automated action read response: %s", string(data))
	require.Equal(t, actionID, read.AutomatedActionId, "cleanup: read response should have the created action ID: %+v", read)
	require.Equal(t, actionName, read.Name, "cleanup: action %q does not match the created action; refusing to delete it: %+v", actionID, read)
	require.Equal(t, condition, read.ExecutionCondition, "cleanup: execution condition of action %q does not match the created one; refusing to delete it: %+v", actionID, read)
	require.Equal(t, "simpleNotification", read.Action.ActionType, "cleanup: action %q does not match the created action; refusing to delete it: %+v", actionID, read)
	require.Equal(t, sp, read.Action.ActionParameter.ServicePrincipalId, "cleanup: service principal of action %q does not match the created one; refusing to delete it: %+v", actionID, read)
	require.Equal(t, groupID, read.Action.ActionParameter.NotificationGroupId, "cleanup: notification group of action %q does not match the created one; refusing to delete it: %+v", actionID, read)
	deleteAutomatedAction(t, h, actionID, "後片付け: 残存したアクションを削除する")
}

// automatedActionNotFound はコマンドの失敗が対象不在の API 404 かどうかを判定します。
func automatedActionNotFound(stderr string) bool {
	return strings.HasPrefix(strings.TrimSpace(stderr), "security-control: AutomatedActions.Read: API Error 404:")
}
