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
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// computeRunbookV1 は計算を行うワークフローの Runbook です。公式マニュアルの
// Runbook 構文リファレンスに沿って、数値引数を2倍して返します。
const computeRunbookV1 = `meta:
  description: skr workflows e2e
  args:
    value:
      type: number
      description: e2e argument
steps:
  double:
    return: ${args.value * 2}
`

// computeRunbookV2 はリビジョン追加の検証に使う Runbook です。説明を変えています。
const computeRunbookV2 = `meta:
  description: skr workflows e2e (revision 2)
  args:
    value:
      type: number
      description: e2e argument
steps:
  double:
    return: ${args.value * 2}
`

// sleepRunbook はキャンセル検証に使うワークフローの Runbook です。
// sys.sleep で指定秒数待機してから終了します。
const sleepRunbook = `meta:
  description: skr workflows e2e sleep
steps:
  wait:
    call: sys.sleep
    args:
      seconds: 60
  done:
    return: "done"
`

// TestScenarioWorkflowsLifecycle は workflows-api コマンドのライブ E2E です。
// テスト専用のワークフロー2件 (計算用、キャンセル検証用) を作成し、リビジョンと
// エイリアスの操作、実行の開始・完了待ち・キャンセル・削除、履歴の参照までを検証します。
// Workflows の契約がなければ最安プランで契約し、後片付けで解約します。
// 既存の契約とワークフローは変更しません。
func TestScenarioWorkflowsLifecycle(t *testing.T) {
	h := helper.New(t, "workflows-api",
		"ワークフローとリビジョンの作成・参照・一覧・更新・エイリアス操作、実行の開始・完了待ち・キャンセル・削除・履歴参照が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	mainName := "skr-e2e-workflows-" + suffix
	sleepName := "skr-e2e-workflows-sleep-" + suffix
	revisionAlias := "stable"
	executionName := "skr-e2e-exec-" + suffix
	cancelExecutionName := "skr-e2e-cancel-" + suffix

	// 契約の有無を確認し、なければ最安プランで契約する。後片付けは ensure の前に defer で
	// 登録し、契約作成直後から検証が失敗しても解約されるようにする
	var subscriptionCleanup workflowsSubscriptionCleanup
	defer releaseWorkflowsSubscription(t, h, &subscriptionCleanup)
	ensureWorkflowsSubscription(t, h, &subscriptionCleanup)

	// 作成前に同名ワークフローが存在しないことを確認する
	before := listWorkflows(t, h, "作成前に同名ワークフローが存在しないことを確認する")
	for _, item := range before.Workflows {
		require.NotEqual(t, mainName, item.Name, "workflow name %q already exists; refusing to modify it", mainName)
		require.NotEqual(t, sleepName, item.Name, "workflow name %q already exists; refusing to modify it", sleepName)
	}

	// Runbook ファイルを用意する
	computeRunbookV1File := writeRunbookFile(t, "runbook-v1.yaml", computeRunbookV1)
	computeRunbookV2File := writeRunbookFile(t, "runbook-v2.yaml", computeRunbookV2)
	sleepRunbookFile := writeRunbookFile(t, "runbook-sleep.yaml", sleepRunbook)

	// ワークフローを作成する
	created := createWorkflowsWorkflow(t, h, mainName, "skr workflows e2e", computeRunbookV1File)
	require.NotEmpty(t, created.ID, "created workflow should have an ID: %+v", created)
	require.Equal(t, mainName, created.Name, "created workflow name should match the requested name")
	require.True(t, created.Publish, "created workflow should be published")
	require.Equal(t, "parallel", created.ConcurrencyMode, "created workflow concurrency mode should match the requested value")
	require.Len(t, created.Tags, 2, "created workflow should have the requested tags: %+v", created.Tags)
	require.Equal(t, "env:test", created.Tags[0].Name)
	require.Equal(t, "skr-e2e", created.Tags[1].Name)
	mainID := created.ID

	// 後片付け: 実行を削除してからワークフローを削除し、削除後に不在を確認する
	var mainExecutionID, cancelExecutionID string
	defer func() {
		cleanupWorkflowsExecutions(t, h, mainID, []string{mainExecutionID})
		deleteWorkflowsWorkflow(t, h, "後片付け: テスト専用ワークフロー "+mainName+" を削除する", mainID)
	}()

	// 作成したワークフローを参照する (参照系の出力はエンベロープを除いた本体)
	read := readWorkflowsWorkflow(t, h, "作成したワークフローを参照する", mainID)
	require.Equal(t, mainID, read.ID, "read should return the workflow created by this run")
	require.Equal(t, mainName, read.Name, "read workflow name should match the requested name")
	require.Equal(t, "skr workflows e2e", read.Description)

	// 一覧に作成したワークフローが含まれることを確認する (--name は部分一致検索)
	listed := listWorkflows(t, h, "作成したワークフローを名前で検索する", "--name", mainName)
	require.Equal(t, 1, listed.Count, "workflow list should contain exactly the workflow created by this run: %+v", listed.Workflows)
	require.Equal(t, mainID, listed.Workflows[0].ID)

	// 名前の候補検索に作成したワークフローが含まれることを確認する。
	// 実 API の Suggests 要素は Name と Count で、作成直後のワークフローも即座に反映される (2026-11-19 ライブ検証済み)。
	// 候補検索で作成したワークフローの名前と件数を確認する
	suggested := listWorkflowsSuggestions(t, h, "名前の候補検索で作成したワークフローを確認する", mainName)
	found := false
	for _, item := range suggested {
		if item.Name == mainName {
			require.Positive(t, item.Count, "suggestion %q should report its workflow count", item.Name)
			found = true
		}
	}
	require.True(t, found, "suggestions should contain the workflow created by this run")

	// リビジョン一覧で初回リビジョンを確認する
	revisions := listWorkflowsRevisions(t, h, "初回リビジョンを一覧で確認する", mainID)
	require.Equal(t, 1, revisions.Count, "workflow should have exactly one revision initially")
	require.Equal(t, 1, revisions.Revisions[0].RevisionId)
	require.Contains(t, revisions.Revisions[0].Runbook, "double", "initial revision should contain the runbook step")

	// 初回リビジョンを参照する
	revision := readWorkflowsRevision(t, h, "初回リビジョンを参照する", mainID, "1")
	require.Equal(t, mainID, revision.WorkflowId, "revision should belong to the workflow created by this run")
	require.Contains(t, revision.Runbook, "double")

	// リビジョンを追加する
	revision2 := createWorkflowsRevision(t, h, "リビジョンを追加する", mainID, computeRunbookV2File)
	require.Equal(t, 2, revision2.RevisionId, "created revision should be revision 2: %+v", revision2)
	require.Equal(t, mainID, revision2.WorkflowId)

	revisions = listWorkflowsRevisions(t, h, "追加後のリビジョン一覧を確認する", mainID)
	require.Equal(t, 2, revisions.Count, "workflow should have two revisions after create-revision")

	// リビジョン2にエイリアスを付ける
	aliased := updateWorkflowsRevisionAlias(t, h, "リビジョン2にエイリアスを設定する", mainID, "2", revisionAlias)
	require.Equal(t, 2, aliased.RevisionId, "updated alias should be applied to revision 2")
	require.Equal(t, revisionAlias, aliased.RevisionAlias, "updated revision should have the requested alias")

	// エイリアスの付いたリビジョンを参照する
	revision = readWorkflowsRevision(t, h, "エイリアス付きリビジョンを参照する", mainID, "2")
	require.Equal(t, revisionAlias, revision.RevisionAlias, "revision 2 should have the alias")

	// リビジョン2のエイリアスを削除する (応答はないため read-revision で確認する)
	deleteWorkflowsRevisionAlias(t, h, "リビジョン2のエイリアスを削除する", mainID, "2")
	revision = readWorkflowsRevision(t, h, "エイリアス削除後のリビジョンを参照する", mainID, "2")
	require.Empty(t, revision.RevisionAlias, "revision 2 should not have an alias after deletion")

	// ワークフローを実行する (リビジョン指定なし: 最新リビジョンで実行される)
	execution := createWorkflowsExecution(t, h, "ワークフローを実行する", mainID, executionName, `{"value": 21}`, 0, "")
	mainExecutionID = execution.ExecutionId
	require.NotEmpty(t, mainExecutionID, "created execution should have an ID: %+v", execution)
	require.Equal(t, executionName, execution.Name)
	require.Equal(t, `{"value": 21}`, execution.Args, "created execution should carry the requested args")
	require.Equal(t, 2, execution.Revision, "execution without revision option should run the latest revision")

	// 実行が完了するまで待つ
	execution = pollWorkflowsExecutionStatus(t, h, mainID, mainExecutionID, []string{"Succeeded"}, 2*time.Minute)
	require.Equal(t, "Succeeded", execution.Status)

	// 実行の結果を確認する
	execution = readWorkflowsExecution(t, h, "完了した実行を参照して結果を確認する", mainID, mainExecutionID)
	require.Contains(t, execution.Result, "42", "execution result should be 21 * 2: %q", execution.Result)

	// 実行一覧に完了した実行が含まれることを確認する
	executions := listWorkflowsExecutions(t, h, "実行一覧を確認する", mainID)
	require.GreaterOrEqual(t, executions.Count, 1, "execution list should contain at least the execution created by this run")
	executionFound := false
	for _, item := range executions.Executions {
		if item.ExecutionId == mainExecutionID {
			require.Equal(t, "Succeeded", item.Status, "listed execution should be succeeded")
			executionFound = true
		}
	}
	require.True(t, executionFound, "execution list should contain the execution created by this run")

	// 実行履歴を確認する
	histories := listWorkflowsExecutionHistories(t, h, "実行履歴を確認する", mainID, mainExecutionID)
	require.GreaterOrEqual(t, histories.Count, 1, "execution histories should not be empty")

	// 実行を削除する
	h.Call(t, "完了した実行を削除する", "workflows-api", "delete-execution", mainID, mainExecutionID)
	_, stderr, err := h.CallAllowingFailure(t, "削除した実行が参照できないことを確認する", "workflows-api", "read-execution", mainID, mainExecutionID)
	require.Error(t, err, "read-execution should fail after deletion")
	require.Contains(t, stderr, "not found", "deleted execution should not be found")
	mainExecutionID = ""

	// キャンセル検証用のワークフローを作成する
	sleepCreated := createWorkflowsWorkflow(t, h, sleepName, "skr workflows e2e (cancel)", sleepRunbookFile)
	sleepID := sleepCreated.ID
	require.NotEmpty(t, sleepID, "created sleep workflow should have an ID: %+v", sleepCreated)

	defer func() {
		cleanupWorkflowsExecutions(t, h, sleepID, []string{cancelExecutionID})
		deleteWorkflowsWorkflow(t, h, "後片付け: テスト専用ワークフロー "+sleepName+" を削除する", sleepID)
	}()

	// 待機するワークフローを実行する
	cancelExecution := createWorkflowsExecution(t, h, "待機するワークフローを実行する", sleepID, cancelExecutionName, "", 0, "")
	cancelExecutionID = cancelExecution.ExecutionId
	require.NotEmpty(t, cancelExecutionID, "created cancel execution should have an ID: %+v", cancelExecution)

	// 実行が Running になるまで待つ
	cancelExecution = pollWorkflowsExecutionStatus(t, h, sleepID, cancelExecutionID, []string{"Running"}, 2*time.Minute)

	// 実行をキャンセルする
	h.Call(t, "Running の実行をキャンセルする", "workflows-api", "cancel-execution", sleepID, cancelExecutionID)

	// キャンセルが完了するまで待つ
	cancelExecution = pollWorkflowsExecutionStatus(t, h, sleepID, cancelExecutionID, []string{"Canceled"}, 3*time.Minute)
	require.Equal(t, "Canceled", cancelExecution.Status)
}

// createWorkflowsWorkflow は create-workflow を実行して作成結果を返します。
// 作成系の出力は作成したリソースの本体です。
func createWorkflowsWorkflow(t *testing.T, h *helper.Helper, name, description, runbookFile string) workflowsWorkflowItem {
	t.Helper()
	args := []string{"workflows-api", "create-workflow", "--name", name, "--description", description,
		"--publish", "--concurrency-mode", "parallel", "--tags", "env:test", "--tags", "skr-e2e",
		"--runbook-file", runbookFile}
	data := h.Call(t, "ワークフロー "+name+" を作成する", args...)
	var result workflowsWorkflowItem
	require.NoError(t, json.Unmarshal(data, &result), "create-workflow output: %s", data)
	return result
}

// readWorkflowsWorkflow は read-workflow を実行して応答を返します。
// 参照系の出力はエンベロープを除いたワークフロー本体です。
func readWorkflowsWorkflow(t *testing.T, h *helper.Helper, description, workflowID string) workflowsWorkflowItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "read-workflow", workflowID)
	var result workflowsWorkflowItem
	require.NoError(t, json.Unmarshal(data, &result), "read-workflow output: %s", data)
	return result
}

// listWorkflowsSuggestions は list-suggestions を実行して候補を返します。
func listWorkflowsSuggestions(t *testing.T, h *helper.Helper, description, name string) []workflowsSuggestionItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "list-suggestions", "--name", name)
	var result struct {
		workflowsListResponse
		Suggests []workflowsSuggestionItem `json:"Suggests"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-suggestions output: %s", data)
	require.True(t, result.IsOk, "list-suggestions should report is_ok: %s", data)
	return result.Suggests
}

// listWorkflowsRevisions は list-revisions を実行して応答を返します。
func listWorkflowsRevisions(t *testing.T, h *helper.Helper, description, workflowID string, extraArgs ...string) struct {
	workflowsListResponse
	Revisions []workflowsRevisionItem `json:"Revisions"`
} {
	t.Helper()
	args := append([]string{"workflows-api", "list-revisions", workflowID}, extraArgs...)
	data := h.Call(t, description, args...)
	var result struct {
		workflowsListResponse
		Revisions []workflowsRevisionItem `json:"Revisions"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-revisions output: %s", data)
	require.True(t, result.IsOk, "list-revisions should report is_ok: %s", data)
	return result
}

// readWorkflowsRevision は read-revision を実行して応答を返します。
// 参照系の出力はエンベロープを除いたリビジョン本体です。
func readWorkflowsRevision(t *testing.T, h *helper.Helper, description, workflowID, revisionNumber string) workflowsRevisionItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "read-revision", workflowID, revisionNumber)
	var result workflowsRevisionItem
	require.NoError(t, json.Unmarshal(data, &result), "read-revision output: %s", data)
	return result
}

// createWorkflowsRevision は create-revision を実行して作成結果を返します。
func createWorkflowsRevision(t *testing.T, h *helper.Helper, description, workflowID, runbookFile string) workflowsRevisionItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "create-revision", workflowID, "--runbook-file", runbookFile)
	var result workflowsRevisionItem
	require.NoError(t, json.Unmarshal(data, &result), "create-revision output: %s", data)
	return result
}

// updateWorkflowsRevisionAlias は update-revision-alias を実行して応答を返します。
// 出力はエンベロープを除いたリビジョン本体です。
func updateWorkflowsRevisionAlias(t *testing.T, h *helper.Helper, description, workflowID, revisionNumber, alias string) workflowsRevisionItem {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "update-revision-alias", workflowID, revisionNumber, "--revision-alias", alias)
	var result workflowsRevisionItem
	require.NoError(t, json.Unmarshal(data, &result), "update-revision-alias output: %s", data)
	return result
}

// deleteWorkflowsRevisionAlias は delete-revision-alias を実行します。
// 出力を持たないため、エイリアスの削除は呼び出し元が read-revision で確認します。
func deleteWorkflowsRevisionAlias(t *testing.T, h *helper.Helper, description, workflowID, revisionNumber string) {
	t.Helper()
	h.Call(t, description, "workflows-api", "delete-revision-alias", workflowID, revisionNumber)
}

// createWorkflowsExecution は create-execution を実行して作成結果を返します。
// revisionID と revisionAlias はどちらか一方だけを指定します。
func createWorkflowsExecution(t *testing.T, h *helper.Helper, description, workflowID, name, args string, revisionID int, revisionAlias string) workflowsExecutionItem {
	t.Helper()
	cmdArgs := []string{"workflows-api", "create-execution", workflowID, "--name", name}
	if args != "" {
		cmdArgs = append(cmdArgs, "--args", args)
	}
	if revisionID > 0 {
		cmdArgs = append(cmdArgs, "--revision-id", strconv.Itoa(revisionID))
	}
	if revisionAlias != "" {
		cmdArgs = append(cmdArgs, "--revision-alias", revisionAlias)
	}
	data := h.Call(t, description, cmdArgs...)
	var result workflowsExecutionItem
	require.NoError(t, json.Unmarshal(data, &result), "create-execution output: %s", data)
	return result
}

// listWorkflowsExecutions は list-executions を実行して応答を返します。
func listWorkflowsExecutions(t *testing.T, h *helper.Helper, description, workflowID string, extraArgs ...string) struct {
	workflowsListResponse
	Executions []workflowsExecutionItem `json:"Executions"`
} {
	t.Helper()
	args := append([]string{"workflows-api", "list-executions", workflowID}, extraArgs...)
	data := h.Call(t, description, args...)
	var result struct {
		workflowsListResponse
		Executions []workflowsExecutionItem `json:"Executions"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-executions output: %s", data)
	require.True(t, result.IsOk, "list-executions should report is_ok: %s", data)
	return result
}

// listWorkflowsExecutionHistories は list-execution-histories を実行して応答を返します。
func listWorkflowsExecutionHistories(t *testing.T, h *helper.Helper, description, workflowID, executionID string) struct {
	workflowsListResponse
	Count int `json:"Count"`
} {
	t.Helper()
	data := h.Call(t, description, "workflows-api", "list-execution-histories", workflowID, executionID)
	var result struct {
		workflowsListResponse
		Count int `json:"Count"`
	}
	require.NoError(t, json.Unmarshal(data, &result), "list-execution-histories output: %s", data)
	require.True(t, result.IsOk, "list-execution-histories should report is_ok: %s", data)
	return result
}
