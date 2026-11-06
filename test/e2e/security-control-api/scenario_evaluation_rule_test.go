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
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/security-control/apis/v1"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// evaluationRuleID は更新と復元を確認する評価ルールの ID です。
	evaluationRuleID = "server-no-public-ip"
	// evaluationRuleTargetZone は一時的に設定する評価対象ゾーンです。
	evaluationRuleTargetZone = "is1b"
)

// TestScenarioEvaluationRule は security-control-api evaluation-rule コマンドのライブ E2E です。
// 一覧で SDK が公開する全評価ルールの包含を確認し、対象ルールの読み取りが一覧と一致することを
// 確認した後、ルールを一時的に有効化して update と反映を確認します。後片付けで元の状態
// (無効・parameter なし) に戻し、読み取りで復元を確認します。評価結果はイベントログへの
// 記録のみで、リソースは変更されません。課金対象リソースは作成しません。
func TestScenarioEvaluationRule(t *testing.T) {
	h := helper.New(t, "security-control-api-evaluation-rule",
		"評価ルールの一覧・読み取り・更新が正しく動作し、更新後に元の状態へ復元できることを確認する")

	// 一覧を取得して SDK が公開する全評価ルールが含まれることを確認する
	items := listEvaluationRules(t, h, "評価ルールの一覧を取得する")
	ruleIDs := make([]string, 0, len(items))
	for _, item := range items {
		ruleIDs = append(ruleIDs, item.Rule.EvaluationRuleId)
	}
	knownIDs := make([]string, 0)
	for _, id := range v1.EvaluationRuleID("").AllValues() {
		knownIDs = append(knownIDs, string(id))
	}
	require.ElementsMatch(t, knownIDs, ruleIDs, "evaluation rules should contain all known rule IDs: %v", ruleIDs)

	// 対象ルールの現在値を確認する。有効なルールは変更しない。
	var current *evaluationRuleItem
	for i := range items {
		if items[i].Rule.EvaluationRuleId == evaluationRuleID {
			current = &items[i]
		}
	}
	require.NotNil(t, current, "rule %q should be listed: %v", evaluationRuleID, ruleIDs)
	require.False(t, current.IsEnabled, "rule %q is enabled; this scenario expects a disabled rule and refuses to modify it: %+v", evaluationRuleID, current)

	// 対象ルールを読み取り、一覧と一致することを確認する
	read := readEvaluationRule(t, h, evaluationRuleID, "評価ルールを読み取る")
	require.Equal(t, *current, read, "read response should match the list item")

	// 後片付け: 元の状態 (無効・parameter なし) に戻し、読み取りで復元を確認する
	defer func() {
		restoreEvaluationRule(t, h, evaluationRuleID)
		restored := readEvaluationRule(t, h, evaluationRuleID, "後片付け: ルールが元の状態に戻っていることを確認する")
		require.False(t, restored.IsEnabled, "rule should be disabled after cleanup: %+v", restored)
		require.Nil(t, restored.Rule.Parameter, "rule should not have a parameter after cleanup: %+v", restored)
	}()

	// ルールを一時的に有効化する (--targets is1b)
	updated := updateEvaluationRuleWithFlags(t, h, evaluationRuleID, "ルールを一時的に有効化する")
	require.True(t, updated.IsEnabled, "update should enable the rule: %+v", updated)
	require.NotNil(t, updated.Rule.Parameter, "update should set the evaluation target: %+v", updated)
	require.Equal(t, []string{evaluationRuleTargetZone}, updated.Rule.Parameter.Zones, "update should set the requested zone: %+v", updated)

	// 反映を確認する
	after := readEvaluationRule(t, h, evaluationRuleID, "更新が反映されていることを確認する")
	require.True(t, after.IsEnabled, "rule should be enabled after update: %+v", after)
	require.NotNil(t, after.Rule.Parameter, "rule should have an evaluation target after update: %+v", after)
	require.Equal(t, []string{evaluationRuleTargetZone}, after.Rule.Parameter.Zones, "rule should have the requested zone after update: %+v", after)
}

// evaluationRuleItem は security-control-api evaluation-rule コマンドの JSON 出力です。
type evaluationRuleItem struct {
	Rule struct {
		EvaluationRuleId string `json:"evaluationRuleId"`
		Parameter        *struct {
			Zones []string `json:"zones,omitempty"`
			Sites []string `json:"sites,omitempty"`
		} `json:"parameter,omitempty"`
	} `json:"rule"`
	Description      string   `json:"description"`
	IamRolesRequired []string `json:"iamRolesRequired"`
	IsEnabled        bool     `json:"isEnabled"`
}

// listEvaluationRules は skr CLI で評価ルールの一覧を取得します。
func listEvaluationRules(t *testing.T, h *helper.Helper, description string) []evaluationRuleItem {
	t.Helper()

	data := h.Call(t, description, "security-control-api", "evaluation-rule", "list")
	var out struct {
		Items []evaluationRuleItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &out), "decode evaluation-rule list response: %s", string(data))
	return out.Items
}

// readEvaluationRule は skr CLI で評価ルールを読み取ります。
func readEvaluationRule(t *testing.T, h *helper.Helper, ruleID string, description string) evaluationRuleItem {
	t.Helper()

	data := h.Call(t, description, "security-control-api", "evaluation-rule", "read", ruleID)
	var out evaluationRuleItem
	require.NoError(t, json.Unmarshal(data, &out), "decode evaluation-rule read response: %s", string(data))
	require.Equal(t, ruleID, out.Rule.EvaluationRuleId, "read response should have the requested rule ID: %s", string(data))
	return out
}

// updateEvaluationRuleWithFlags は skr CLI でルールを --is-enabled と --targets is1b で更新します。
func updateEvaluationRuleWithFlags(t *testing.T, h *helper.Helper, ruleID string, description string) evaluationRuleItem {
	t.Helper()

	data := h.Call(t, description,
		"security-control-api", "evaluation-rule", "update", ruleID,
		"--is-enabled",
		"--targets", evaluationRuleTargetZone,
	)
	var out evaluationRuleItem
	require.NoError(t, json.Unmarshal(data, &out), "decode evaluation-rule update response: %s", string(data))
	return out
}

// restoreEvaluationRule は skr CLI で --request JSON を使ってルールを元の状態
// (無効・parameter なし) に戻します。説明は API が管理する値のため、更新リクエストには
// 含まれません。
func restoreEvaluationRule(t *testing.T, h *helper.Helper, ruleID string) {
	t.Helper()

	h.Call(t, "ルールを元の状態 (無効・parameter なし) に戻す",
		"security-control-api", "evaluation-rule", "update", ruleID,
		"--request", `{"rule":{"evaluationRuleId":"`+ruleID+`"},"isEnabled":false}`,
	)
}
