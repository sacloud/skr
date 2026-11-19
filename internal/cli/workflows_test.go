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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	workflowsmock "github.com/sacloud/sakumock/workflows"
)

// testRunbookYAML は sakumock の Runbook 検証を通る最小の Runbook です。
const testRunbookYAML = `
meta:
  description: test
steps:
  done:
    return: "hello"
`

func setupWorkflowsMock(t *testing.T) {
	t.Helper()
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := workflowsmock.NewTestServer(workflowsmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_WORKFLOWS", server.TestURL())
}

func writeTempRunbook(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runbook.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestRunWorkflowsAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"workflows-api", "--help"},
			want: []string{
				"list-workflows", "list-suggestions", "create-workflow", "read-workflow", "patch-workflow", "delete-workflow",
				"create-revision", "list-revisions", "read-revision", "update-revision-alias", "delete-revision-alias",
				"create-execution", "list-executions", "read-execution", "cancel-execution", "delete-execution", "list-execution-histories",
				"list-plans", "read-subscription", "create-subscription", "delete-subscription",
				"Runbook", "--runbook-file", "SAKURA_ENDPOINTS_WORKFLOWS",
			},
		},
		{
			args: []string{"workflows-api", "create-workflow", "--help"},
			want: []string{"--name", "--runbook-file", "--description", "--publish", "--logging", "--tags", "--revision-alias", "--concurrency-mode", "parallel", "lock", "queue", "--request", "@path.json", "標準入力"},
		},
		{
			args: []string{"workflows-api", "patch-workflow", "--help"},
			want: []string{"--name", "--description", "--publish", "--logging", "--tags", "--concurrency-mode", "--request", "Tags をリクエストに含めません"},
		},
		{
			args: []string{"workflows-api", "create-revision", "--help"},
			want: []string{"workflow-id", "--runbook-file", "--revision-alias", "--request"},
		},
		{
			args: []string{"workflows-api", "create-execution", "--help"},
			want: []string{"workflow-id", "--revision-id", "--revision-alias", "--args", "@path.json", "--name", "同時に指定できません"},
		},
		{
			args: []string{"workflows-api", "list-workflows", "--help"},
			want: []string{"--page", "--page-limit", "--sort-by", "createdAt", "--order", "--published", "--name", "--name-match-type", "partial", "prefix"},
		},
		{
			args: []string{"workflows-api", "list-suggestions", "--help"},
			want: []string{"--name", "--page", "--page-limit", "--sort-by", "--order"},
		},
		{
			args: []string{"workflows-api", "create-subscription", "--help"},
			want: []string{"--plan-id", "--request", "list-plans", "課金"},
		},
		{
			args: []string{"workflows-api", "delete-subscription", "--help"},
			want: []string{"解約", "read-subscription"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				require.Contains(t, help, expected, "help does not contain %q:\n%s", expected, help)
			}
		})
	}
}

func TestRunWorkflowsAPILifecycle(t *testing.T) {
	setupWorkflowsMock(t)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	// create-workflow: 個別フラグ経路。Runbook はファイルから読み込む
	runbookPath := writeTempRunbook(t, testRunbookYAML)
	var created struct {
		Id          string `json:"Id"`
		Name        string `json:"Name"`
		Description string `json:"Description"`
		Publish     bool   `json:"Publish"`
		Logging     bool   `json:"Logging"`
		Tags        []struct {
			Name string `json:"Name"`
		} `json:"Tags"`
		ConcurrencyMode string `json:"ConcurrencyMode"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "create-workflow",
		"--name", "mock-wf",
		"--description", "test workflow",
		"--runbook-file", runbookPath,
		"--publish",
		"--logging",
		"--tags", "env:prod",
		"--tags", "team:infra",
		"--revision-alias", "v1",
		"--concurrency-mode", "queue",
	), &created), "output = %s", runbookPath)
	require.NotEmpty(t, created.Id, "created = %+v", created)
	require.Equal(t, "mock-wf", created.Name)
	require.Equal(t, "test workflow", created.Description)
	require.Equal(t, true, created.Publish)
	require.Equal(t, true, created.Logging)
	require.Len(t, created.Tags, 2)
	require.Equal(t, "env:prod", created.Tags[0].Name)
	require.Equal(t, "team:infra", created.Tags[1].Name)
	require.Equal(t, "queue", created.ConcurrencyMode)
	workflowID := created.Id

	// 非公開ワークフローの実行はエラーになる (公開必須)
	unpublishedRunbook := writeTempRunbook(t, testRunbookYAML)
	var unpublished struct {
		Id string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "create-workflow", "--name", "mock-wf-private", "--runbook-file", unpublishedRunbook,
	), &unpublished))
	require.NotEmpty(t, unpublished.Id)
	var stderrBuf bytes.Buffer
	require.NotEqual(t, 0, run([]string{"workflows-api", "create-execution", unpublished.Id}, &stdoutHolder{}, &stderrBuf))
	require.Contains(t, stderrBuf.String(), "not executable")

	// list-workflows: 一覧と名前での絞り込み
	var listed struct {
		IsOk      bool `json:"is_ok"`
		Workflows []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Workflows"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-workflows"), &listed))
	require.True(t, listed.IsOk)
	require.NotEmpty(t, listed.Workflows)
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-workflows", "--name", "mock-wf", "--name-match-type", "prefix"), &listed))
	require.Len(t, listed.Workflows, 2, "prefix filter should match both mock-wf and mock-wf-private")

	// list-suggestions: --name は必須。sakumock の候補応答は SDK が必須とする Count を
	// 含まないため、正常系は TestRunWorkflowsAPIListSuggestions で仕様に沿った応答を使って検証する
	require.NotEqual(t, 0, run([]string{"workflows-api", "list-suggestions"}, &stdoutHolder{}, &stderrBuf))

	// read-workflow
	var read struct {
		Id   string `json:"Id"`
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-workflow", workflowID), &read))
	require.Equal(t, workflowID, read.Id)
	require.Equal(t, "mock-wf", read.Name)

	// create-revision: リビジョンを追加する
	revision2Runbook := writeTempRunbook(t, testRunbookYAML+"\n# revision 2\n")
	var createdRevision struct {
		RevisionId    int    `json:"RevisionId"`
		WorkflowId    string `json:"WorkflowId"`
		RevisionAlias string `json:"RevisionAlias"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "create-revision", workflowID,
		"--runbook-file", revision2Runbook,
		"--revision-alias", "v2",
	), &createdRevision))
	require.Equal(t, 2, createdRevision.RevisionId)
	require.Equal(t, workflowID, createdRevision.WorkflowId)
	require.Equal(t, "v2", createdRevision.RevisionAlias)

	// list-revisions
	var revisions struct {
		Revisions []struct {
			RevisionId int `json:"RevisionId"`
		} `json:"Revisions"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-revisions", workflowID), &revisions))
	require.Len(t, revisions.Revisions, 2)

	// read-revision
	var readRevision struct {
		RevisionId int    `json:"RevisionId"`
		Runbook    string `json:"Runbook"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-revision", workflowID, "1"), &readRevision))
	require.Equal(t, 1, readRevision.RevisionId)
	require.Contains(t, readRevision.Runbook, "return:")

	// update-revision-alias / delete-revision-alias
	var renamedAlias struct {
		RevisionAlias string `json:"RevisionAlias"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "update-revision-alias", workflowID, "1", "--revision-alias", "stable",
	), &renamedAlias))
	require.Equal(t, "stable", renamedAlias.RevisionAlias)

	// create-execution: 最新リビジョン (2) が実行される
	var createdExecution struct {
		ExecutionId string `json:"ExecutionId"`
		Status      string `json:"Status"`
		Revision    int    `json:"Revision"`
		Args        string `json:"Args"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "create-execution", workflowID, "--name", "nightly-run"), &createdExecution))
	require.NotEmpty(t, createdExecution.ExecutionId)
	require.Equal(t, "Succeeded", createdExecution.Status)
	require.Equal(t, 2, createdExecution.Revision)
	require.Equal(t, "null", createdExecution.Args)
	executionID := createdExecution.ExecutionId

	// create-execution: --revision-id と --args を指定する
	var secondExecution struct {
		ExecutionId string `json:"ExecutionId"`
		Revision    int    `json:"Revision"`
		Args        string `json:"Args"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "create-execution", workflowID,
		"--revision-id", "1",
		"--args", `{"day":"mon"}`,
	), &secondExecution))
	require.Equal(t, 1, secondExecution.Revision)
	require.JSONEq(t, `{"day":"mon"}`, secondExecution.Args)
	secondExecutionID := secondExecution.ExecutionId

	// list-executions
	var executions struct {
		Executions []struct {
			ExecutionId string `json:"ExecutionId"`
		} `json:"Executions"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-executions", workflowID), &executions))
	require.Len(t, executions.Executions, 2)

	// read-execution
	var readExecution struct {
		ExecutionId string `json:"ExecutionId"`
		Status      string `json:"Status"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-execution", workflowID, executionID), &readExecution))
	require.Equal(t, executionID, readExecution.ExecutionId)
	require.Equal(t, "Succeeded", readExecution.Status)

	// list-execution-histories
	var histories struct {
		Histories []struct {
			WorkflowExecutionId string `json:"WorkflowExecutionId"`
		} `json:"Histories"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-execution-histories", workflowID, executionID), &histories))

	// delete-execution
	runCommand("workflows-api", "delete-execution", workflowID, secondExecutionID)
	require.NotEqual(t, 0, run([]string{"workflows-api", "read-execution", workflowID, secondExecutionID}, &stdoutHolder{}, &stderrBuf))
	require.Contains(t, stderrBuf.String(), "not found")

	// patch-workflow: 個別フラグは指定した項目だけを送る
	var patched struct {
		Name        string `json:"Name"`
		Description string `json:"Description"`
		Publish     bool   `json:"Publish"`
		Tags        []struct {
			Name string `json:"Name"`
		} `json:"Tags"`
		ConcurrencyMode string `json:"ConcurrencyMode"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "patch-workflow", workflowID,
		"--name", "mock-wf-renamed",
		"--tags", "env:staging",
		"--concurrency-mode", "lock",
	), &patched))
	require.Equal(t, "mock-wf-renamed", patched.Name)
	require.Equal(t, true, patched.Publish, "patch は既存値を保持する")
	require.Len(t, patched.Tags, 1)
	require.Equal(t, "env:staging", patched.Tags[0].Name)
	require.Equal(t, "lock", patched.ConcurrencyMode)

	// delete-revision-alias
	runCommand("workflows-api", "delete-revision-alias", workflowID, "1")

	// delete-workflow
	runCommand("workflows-api", "delete-workflow", unpublished.Id)
	runCommand("workflows-api", "delete-workflow", workflowID)
	require.NotEqual(t, 0, run([]string{"workflows-api", "read-workflow", workflowID}, &stdoutHolder{}, &stderrBuf))
	require.Contains(t, stderrBuf.String(), "not found")

	// サブスクリプション: 未契約状態の read-subscription
	var emptySubscription struct {
		IsOk        bool           `json:"is_ok"`
		CurrentPlan map[string]any `json:"CurrentPlan"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-subscription"), &emptySubscription))
	require.True(t, emptySubscription.IsOk)
	require.Nil(t, emptySubscription.CurrentPlan)

	// list-plans
	var plans struct {
		Plans []struct {
			Id   int    `json:"Id"`
			Name string `json:"Name"`
		} `json:"Plans"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "list-plans"), &plans))
	require.NotEmpty(t, plans.Plans)

	// create-subscription / read-subscription / delete-subscription
	runCommand("workflows-api", "create-subscription", "--plan-id", strconv.Itoa(plans.Plans[0].Id))
	var subscribed struct {
		CurrentPlan *struct {
			PlanId int `json:"PlanId"`
		} `json:"CurrentPlan"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-subscription"), &subscribed))
	require.NotNil(t, subscribed.CurrentPlan)
	require.Equal(t, plans.Plans[0].Id, subscribed.CurrentPlan.PlanId)
	runCommand("workflows-api", "delete-subscription")
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "read-subscription"), &emptySubscription))
	require.Nil(t, emptySubscription.CurrentPlan)
}

// TestRunWorkflowsAPICancelExecution はデータプレーン有効のモックで実行をキャンセルします。
func TestRunWorkflowsAPICancelExecution(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := workflowsmock.NewTestServer(workflowsmock.Config{EnableDataPlane: true})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_WORKFLOWS", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	// sys.sleep で実行を Running 状態に保つ
	runbookPath := writeTempRunbook(t, `
meta:
  description: test
steps:
  wait:
    call: sys.sleep
    args:
      seconds: "30"
  done:
    return: "hello"
`)
	var created struct {
		Id string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(runCommand(
		"workflows-api", "create-workflow", "--name", "cancel-wf", "--runbook-file", runbookPath, "--publish",
	), &created))
	require.NotEmpty(t, created.Id)

	var execution struct {
		ExecutionId string `json:"ExecutionId"`
		Status      string `json:"Status"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "create-execution", created.Id), &execution))
	require.NotEmpty(t, execution.ExecutionId)

	var canceled struct {
		ExecutionId string `json:"ExecutionId"`
		Status      string `json:"Status"`
	}
	require.NoError(t, json.Unmarshal(runCommand("workflows-api", "cancel-execution", created.Id, execution.ExecutionId), &canceled))
	require.Equal(t, execution.ExecutionId, canceled.ExecutionId)
	require.Equal(t, "Canceled", canceled.Status)
}

// TestRunWorkflowsAPIListSuggestions は sakumock の候補応答が SDK の必須フィールド (Count) を
// 含まないため、仕様に沿った応答を返すテストサーバーで list-suggestions を検証します。
func TestRunWorkflowsAPIListSuggestions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "private", r.URL.Query().Get("Name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_ok":true,"Total":1,"From":0,"Count":1,"Suggests":[{"Id":"wf-1","Name":"mock-wf-private","Count":3}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_WORKFLOWS", server.URL)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"workflows-api", "list-suggestions", "--name", "private"}, &stdout, &stderr), "stderr: %s", stderr.String())
	var result struct {
		IsOk     bool `json:"is_ok"`
		Suggests []struct {
			Id    string `json:"Id"`
			Name  string `json:"Name"`
			Count int    `json:"Count"`
		} `json:"Suggests"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), "output = %s", stdout.String())
	require.True(t, result.IsOk)
	require.Len(t, result.Suggests, 1)
	require.Equal(t, "mock-wf-private", result.Suggests[0].Name)
	require.Equal(t, 3, result.Suggests[0].Count)
}

// TestRunWorkflowsAPIPatchWorkflowWireBody は patch-workflow の送信内容を検証します。
// --tags を省略した場合は Tags をリクエストに含めません。
func TestRunWorkflowsAPIPatchWorkflowWireBody(t *testing.T) {
	type recordedRequest struct {
		method string
		path   string
		body   map[string]any
	}
	var recorded recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		recorded.method, recorded.path = r.Method, r.URL.Path
		recorded.body = map[string]any{}
		if len(data) > 0 {
			require.NoError(t, json.Unmarshal(data, &recorded.body), "request body = %s", data)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_ok":true,"Workflow":{"Id":"wf-1","Name":"wire-wf","Publish":false,"Logging":false,"Tags":[],"CreatedAt":"2026-11-18T00:00:00Z","UpdatedAt":"2026-11-18T00:00:00Z"}}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_WORKFLOWS", server.URL)

	runOK := func(args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	}

	// --tags を指定した場合は Tags 配列を送る
	runOK("workflows-api", "patch-workflow", "wf-1", "--tags", "env:prod", "--tags", "team:infra")
	require.Equal(t, "PATCH", recorded.method)
	require.Equal(t, []any{map[string]any{"Name": "env:prod"}, map[string]any{"Name": "team:infra"}}, recorded.body["Tags"], "wire body = %#v", recorded.body)

	// --tags を省略した場合は Tags を送らない (patch 意味論)
	recorded.body = nil
	runOK("workflows-api", "patch-workflow", "wf-1", "--name", "renamed")
	require.NotContains(t, recorded.body, "Tags", "wire body = %#v", recorded.body)
	require.Equal(t, "renamed", recorded.body["Name"])

	// --request JSON の Tags はそのまま送る
	recorded.body = nil
	runOK("workflows-api", "patch-workflow", "wf-1", "--request", `{"Name":"json-wf","Tags":[{"Name":"env:json"}]}`)
	require.Equal(t, []any{map[string]any{"Name": "env:json"}}, recorded.body["Tags"], "wire body = %#v", recorded.body)
}

func TestRunWorkflowsAPIRejectsFlagsWithRequest(t *testing.T) {
	setupWorkflowsMock(t)
	runbookPath := writeTempRunbook(t, testRunbookYAML)

	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "create-workflow",
			args: []string{"workflows-api", "create-workflow", "--request", `{"Name":"wf"}`, "--name", "wf"},
		},
		{
			name: "create-workflow tags",
			args: []string{"workflows-api", "create-workflow", "--request", `{"Name":"wf"}`, "--tags", "env:prod"},
		},
		{
			name: "create-revision",
			args: []string{"workflows-api", "create-revision", "wf-1", "--request", `{"Runbook":"steps: []"}`, "--runbook-file", runbookPath},
		},
		{
			name: "create-execution",
			args: []string{"workflows-api", "create-execution", "wf-1", "--request", `{"Name":"run"}`, "--revision-id", "1"},
		},
		{
			name: "update-revision-alias",
			args: []string{"workflows-api", "update-revision-alias", "wf-1", "1", "--request", `{"RevisionAlias":"v1"}`, "--revision-alias", "v2"},
		},
		{
			name: "create-subscription",
			args: []string{"workflows-api", "create-subscription", "--request", `{"PlanId":1}`, "--plan-id", "1"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.NotEqual(t, 0, run(test.args, &stdout, &stderr), "run should fail")
			require.Contains(t, stderr.String(), "--request と個別フラグは併用できません")
		})
	}
}

func TestRunWorkflowsAPIInputValidation(t *testing.T) {
	setupWorkflowsMock(t)
	runbookPath := writeTempRunbook(t, testRunbookYAML)

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "create-workflow without name",
			args: []string{"workflows-api", "create-workflow", "--runbook-file", runbookPath},
			want: "--name が必要です",
		},
		{
			name: "create-workflow without runbook",
			args: []string{"workflows-api", "create-workflow", "--name", "wf"},
			want: "--runbook-file が必要です",
		},
		{
			name: "create-workflow invalid concurrency mode",
			args: []string{"workflows-api", "create-workflow", "--name", "wf", "--runbook-file", runbookPath, "--concurrency-mode", "serial"},
			want: "parallel、lock、queue のいずれか",
		},
		{
			name: "create-workflow missing runbook file",
			args: []string{"workflows-api", "create-workflow", "--name", "wf", "--runbook-file", filepath.Join(t.TempDir(), "not-found.yaml")},
			want: "read runbook file",
		},
		{
			name: "create-execution both revision-id and alias",
			args: []string{"workflows-api", "create-execution", "wf-1", "--revision-id", "1", "--revision-alias", "v1"},
			want: "--revision-id と --revision-alias は同時に指定できません",
		},
		{
			name: "create-execution invalid args json",
			args: []string{"workflows-api", "create-execution", "wf-1", "--args", `{"day":`},
			want: "JSON として妥当な文字列",
		},
		{
			name: "list-workflows invalid sort-by",
			args: []string{"workflows-api", "list-workflows", "--sort-by", "name"},
			want: "--sort-by には createdAt、updatedAt、id のいずれか",
		},
		{
			name: "list-workflows invalid name-match-type",
			args: []string{"workflows-api", "list-workflows", "--name", "wf", "--name-match-type", "suffix"},
			want: "--name-match-type には partial または prefix",
		},
		{
			name: "list-suggestions invalid sort-by",
			args: []string{"workflows-api", "list-suggestions", "--name", "wf", "--sort-by", "createdAt"},
			want: "--sort-by には name または count",
		},
		{
			name: "list-revisions invalid order",
			args: []string{"workflows-api", "list-revisions", "wf-1", "--order", "ascending"},
			want: "--order には asc または desc",
		},
		{
			name: "list-executions invalid order",
			args: []string{"workflows-api", "list-executions", "wf-1", "--order", "ascending"},
			want: "--order には asc または desc",
		},
		{
			name: "list-execution-histories invalid sort-order",
			args: []string{"workflows-api", "list-execution-histories", "wf-1", "exec-1", "--sort-order", "ascending"},
			want: "--sort-order には asc または desc",
		},
		{
			name: "patch-workflow invalid concurrency mode",
			args: []string{"workflows-api", "patch-workflow", "wf-1", "--concurrency-mode", "serial"},
			want: "parallel、lock、queue のいずれか",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.NotEqual(t, 0, run(test.args, &stdout, &stderr), "run should fail")
			require.Contains(t, stderr.String(), test.want, "stderr = %q", stderr.String())
		})
	}
}

// TestRunWorkflowsAPICreateWorkflowWithRequest は --request JSON で ServicePrincipalId を
// 含めて作成できることを検証します。ServicePrincipalId 利用のワークフローは API トークン認証の
// 経路からは作成できないため、JSON でのみ指定できます。
func TestRunWorkflowsAPICreateWorkflowWithRequest(t *testing.T) {
	var createdBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &createdBody), "request body = %s", data)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"is_ok":true,"Workflow":{"Id":"wf-sp","Name":"sp-wf","Publish":false,"Logging":false,"Tags":[],"ServicePrincipalId":"123456789012","CreatedAt":"2026-11-18T00:00:00Z","UpdatedAt":"2026-11-18T00:00:00Z"}}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_WORKFLOWS", server.URL)

	requestJSON := `{"Name":"sp-wf","Runbook":` + quoteJSON(t, testRunbookYAML) + `,"Publish":false,"Logging":false,"ServicePrincipalId":"123456789012"}`
	requestPath := filepath.Join(t.TempDir(), "create-workflow.json")
	require.NoError(t, os.WriteFile(requestPath, []byte(requestJSON), 0o600))

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"workflows-api", "create-workflow", "--request", "@" + requestPath}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Equal(t, "123456789012", createdBody["ServicePrincipalId"], "wire body = %#v", createdBody)
	require.Equal(t, "sp-wf", createdBody["Name"])

	var created struct {
		Id                 string `json:"Id"`
		ServicePrincipalId string `json:"ServicePrincipalId"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &created))
	require.Equal(t, "wf-sp", created.Id)
	require.Equal(t, "123456789012", created.ServicePrincipalId)
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

// stdoutHolder は出力を破棄する io.Writer です。エラー系テストで使います。
type stdoutHolder struct{}

func (stdoutHolder) Write(p []byte) (int, error) { return len(p), nil }
