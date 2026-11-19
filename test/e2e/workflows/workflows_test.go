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
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// workflowsWorkflowItem は workflows-api のワークフロー応答の共通項目です。
// Workflows API の応答フィールドは PascalCase です。
type workflowsWorkflowItem struct {
	ID              string             `json:"Id"`
	Name            string             `json:"Name"`
	Description     string             `json:"Description"`
	Publish         bool               `json:"Publish"`
	Logging         bool               `json:"Logging"`
	Tags            []workflowsTagItem `json:"Tags"`
	ConcurrencyMode string             `json:"ConcurrencyMode"`
}

// workflowsTagItem は Tags 配列の要素です。要素は Name のみを持ちます。
type workflowsTagItem struct {
	Name string `json:"Name"`
}

// workflowsListResponse は一覧系コマンドの共通応答です。
type workflowsListResponse struct {
	IsOk  bool `json:"is_ok"`
	Total int  `json:"Total"`
	From  int  `json:"From"`
	Count int  `json:"Count"`
}

// workflowsRevisionItem はリビジョン応答の共通項目です。
type workflowsRevisionItem struct {
	RevisionId    int    `json:"RevisionId"`
	WorkflowId    string `json:"WorkflowId"`
	RevisionAlias string `json:"RevisionAlias"`
	Runbook       string `json:"Runbook"`
}

// workflowsExecutionItem は実行応答の共通項目です。
type workflowsExecutionItem struct {
	ExecutionId string `json:"ExecutionId"`
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	Revision    int    `json:"Revision"`
	Args        string `json:"Args"`
	StepCount   int    `json:"StepCount"`
	Result      string `json:"Result"`
}

// workflowsPlanItem は list-plans の応答項目です。
type workflowsPlanItem struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	BasePrice     int    `json:"basePrice"`
	IncludedSteps int    `json:"includedSteps"`
}

// workflowsCurrentPlan は read-subscription の現在の契約です。
type workflowsCurrentPlan struct {
	ID       string `json:"id"`
	PlanId   int    `json:"planId"`
	PlanName string `json:"planName"`
}

// workflowsSuggestionItem は list-suggestions の応答項目です。
type workflowsSuggestionItem struct {
	Id    string `json:"Id"`
	Name  string `json:"Name"`
	Count int    `json:"Count"`
}

// listWorkflows は list-workflows を実行して応答をデコードします。
func listWorkflows(t *testing.T, h *helper.Helper, description string, extraArgs ...string) struct {
	workflowsListResponse
	Workflows []workflowsWorkflowItem `json:"Workflows"`
} {
	t.Helper()
	args := append([]string{"workflows-api", "list-workflows"}, extraArgs...)
	data := h.Call(t, description, args...)
	var result struct {
		workflowsListResponse
		Workflows []workflowsWorkflowItem `json:"Workflows"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-workflows output: %s", data)
	require.True(t, result.IsOk, "list-workflows should report is_ok: %s", data)
	return result
}

// listWorkflowsPlans は list-plans を実行してプラン一覧を返します。
func listWorkflowsPlans(t *testing.T, h *helper.Helper) []workflowsPlanItem {
	t.Helper()
	data := h.Call(t, "Workflows の利用可能なプラン一覧を取得する", "workflows-api", "list-plans")
	var result struct {
		workflowsListResponse
		Plans []workflowsPlanItem `json:"Plans"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-plans output: %s", data)
	require.True(t, result.IsOk, "list-plans should report is_ok: %s", data)
	require.NotEmpty(t, result.Plans, "list-plans should return at least one plan")
	return result.Plans
}

// cheapestWorkflowsPlan はプラン一覧から基本料金が最も低いプランを返します。
func cheapestWorkflowsPlan(t *testing.T, plans []workflowsPlanItem) workflowsPlanItem {
	t.Helper()
	cheapest := plans[0]
	for _, p := range plans[1:] {
		if p.BasePrice < cheapest.BasePrice {
			cheapest = p
		}
	}
	return cheapest
}

// readWorkflowsSubscription は read-subscription を実行して現在の契約を返します。
// 契約がなければ nil を返します。
func readWorkflowsSubscription(t *testing.T, h *helper.Helper, description string) *workflowsCurrentPlan {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "read-subscription")
	var result struct {
		workflowsListResponse
		CurrentPlan *workflowsCurrentPlan `json:"CurrentPlan"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "read-subscription output: %s", data)
	return result.CurrentPlan
}

// ensureWorkflowsSubscription は Workflows の契約がなければ最安プランで契約します。
// 戻り値は (契約を作成したか, 契約したプラン ID) です。created が true の場合は呼び出し元が
// defer で releaseWorkflowsSubscription を実行して契約を解約してください。
// 既存の契約は変更・解約しません。
func ensureWorkflowsSubscription(t *testing.T, h *helper.Helper) (bool, int) {
	t.Helper()
	current := readWorkflowsSubscription(t, h, "Workflows の契約状態を確認する")
	if current != nil {
		t.Logf("Workflows の契約が既に存在します (planId=%d planName=%s)。契約を作成・解約しません", current.PlanId, current.PlanName)
		return false, 0
	}
	cheapest := cheapestWorkflowsPlan(t, listWorkflowsPlans(t, h))
	h.Call(t, "契約がないため最安プランで契約を開始する (planId="+strconv.Itoa(cheapest.ID)+" name="+cheapest.Name+")",
		"workflows-api", "create-subscription", "--plan-id", strconv.Itoa(cheapest.ID))
	after := readWorkflowsSubscription(t, h, "契約開始後に契約状態を確認する")
	require.NotNil(t, after, "subscription should exist after create-subscription")
	require.Equal(t, cheapest.ID, after.PlanId, "current plan should be the plan created by this run")
	return true, cheapest.ID
}

// releaseWorkflowsSubscription は契約を解約します。解約直前に現在値を再取得し、
// 契約が存在しない場合は変更せずに終了します。現在の契約が expectedPlanID と
// 一致しない場合は変更 API を呼ばずに失敗します。
func releaseWorkflowsSubscription(t *testing.T, h *helper.Helper, expectedPlanID int) {
	t.Helper()
	current := readWorkflowsSubscription(t, h, "後片付け: 契約状態を再確認する")
	if current == nil {
		t.Logf("後片付け: 契約は既に存在しません。解約しません")
		return
	}
	require.Equal(t, expectedPlanID, current.PlanId, "subscription plan should be the plan created by this run; refusing to delete a different plan")
	h.Call(t, "後片付け: テストで開始した契約 (planId="+strconv.Itoa(current.PlanId)+") を解約する", "workflows-api", "delete-subscription")
	after := readWorkflowsSubscription(t, h, "後片付け: 解約後に契約が存在しないことを確認する")
	require.Nil(t, after, "subscription should not remain after delete-subscription")
}

// writeRunbookFile は Runbook YAML を一時ディレクトリに書き込み、パスを返します。
func writeRunbookFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600), "could not write runbook file: %s", name)
	return path
}

// pollWorkflowsExecutionStatus は read-execution を繰り返し実行し、
// 実行ステータスが wanted のいずれかになるまで待ちます。
// 終端状態 (Succeeded、Failed、Canceled) に到達して wanted に一致しない場合は失敗します。
// 待ち受けはリソースを変更しないため、応答ごとに実行記録へ残します。
func pollWorkflowsExecutionStatus(t *testing.T, h *helper.Helper, workflowID, executionID string, wanted []string, timeout time.Duration) workflowsExecutionItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		data := h.Call(t, "実行のステータスを確認する (wanted: "+strings.Join(wanted, "/")+")",
			"workflows-api", "read-execution", workflowID, executionID, "--query-raw", ".Status")
		status := strings.TrimSpace(string(data))
		for _, want := range wanted {
			if status == want {
				return workflowsExecutionItem{ExecutionId: executionID, Status: status}
			}
		}
		switch status {
		case "Succeeded", "Failed", "Canceled":
			t.Fatalf("execution %s reached terminal status %q, wanted %v", executionID, status, wanted)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for execution %s to reach %v; last status %q", executionID, wanted, status)
		}
		time.Sleep(3 * time.Second)
	}
}

// readWorkflowsExecution は read-execution を実行して実行の詳細を返します。
// 参照系コマンドの出力はエンベロープを除いた本体オブジェクトです。
func readWorkflowsExecution(t *testing.T, h *helper.Helper, description, workflowID, executionID string) workflowsExecutionItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "read-execution", workflowID, executionID)
	var result workflowsExecutionItem
	require.NoError(t, json.Unmarshal(data, &result), "read-execution output: %s", data)
	return result
}

// cleanupWorkflowsExecutions はワークフローの実行を削除します。存在しない実行は変更せず終了し、
// 終端状態の実行だけを削除します。ワークフロー削除の前に実行を掃除するために使います。
func cleanupWorkflowsExecutions(t *testing.T, h *helper.Helper, workflowID string, executionIDs []string) {
	t.Helper()
	for _, executionID := range executionIDs {
		if executionID == "" {
			continue
		}
		// 現在値を再取得し、存在と状態を確認してから削除する
		execution := readWorkflowsExecutionAllowingFailure(t, h, "後片付け: 実行 "+executionID+" の残存を確認する", workflowID, executionID)
		if execution == nil {
			t.Logf("後片付け: 実行 %s は既に存在しません。削除しません", executionID)
			continue
		}
		switch execution.Status {
		case "Succeeded", "Failed", "Canceled":
			h.Call(t, "後片付け: 実行 "+executionID+" (status="+execution.Status+") を削除する", "workflows-api", "delete-execution", workflowID, executionID)
		default:
			// Queued、Running、Canceling は削除できないため、終端状態まで待ってから削除する
			pollWorkflowsExecutionStatus(t, h, workflowID, executionID, []string{"Succeeded", "Failed", "Canceled"}, 3*time.Minute)
			h.Call(t, "後片付け: 実行 "+executionID+" を削除する", "workflows-api", "delete-execution", workflowID, executionID)
		}
	}
}

// readWorkflowsExecutionAllowingFailure は read-execution を実行し、
// 存在しない場合 (not found) は nil を返します。その他の失敗はテストを失敗させます。
func readWorkflowsExecutionAllowingFailure(t *testing.T, h *helper.Helper, description, workflowID, executionID string) *workflowsExecutionItem {
	t.Helper()
	data, stderr, err := h.CallAllowingFailure(t, description, "workflows-api", "read-execution", workflowID, executionID)
	if err != nil {
		require.Contains(t, stderr, "not found", "unexpected error reading execution %s: %s", executionID, stderr)
		return nil
	}
	var result workflowsExecutionItem
	require.NoError(t, json.Unmarshal(data, &result), "read-execution output: %s", data)
	return &result
}

// deleteWorkflowsWorkflow はワークフローを削除し、削除後に不在であることを確認します。
func deleteWorkflowsWorkflow(t *testing.T, h *helper.Helper, description, workflowID string) {
	t.Helper()
	// 削除直前にワークフローを再照合する (参照系の出力はエンベロープを除いた本体オブジェクト)
	data := h.Call(t, description+" (削除前に再照合)", "workflows-api", "read-workflow", workflowID)
	var read workflowsWorkflowItem
	require.NoError(t, json.Unmarshal(data, &read), "read-workflow output: %s", data)
	require.Equal(t, workflowID, read.ID, "workflow %q should be read back before deletion", workflowID)

	h.Call(t, description, "workflows-api", "delete-workflow", workflowID)
	// 削除後の不在確認 (参照系の出力はエンベロープを除いた本体オブジェクト)
	_, stderr, err := h.CallAllowingFailure(t, description+" (削除後の不在確認)", "workflows-api", "read-workflow", workflowID)
	require.Error(t, err, "read-workflow should fail after deletion")
	require.Contains(t, stderr, "not found", "workflow %s should not be found after deletion", workflowID)
}
