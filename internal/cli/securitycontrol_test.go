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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunSecurityControlAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"security-control-api", "--help"},
			want: []string{"activation", "evaluation-rule", "automated-action", "manual.sakura.ad.jp/cloud/controlpanel/eventlog/security-control.html"},
		},
		{
			args: []string{"security-control-api", "activation", "--help"},
			want: []string{"create", "read", "update"},
		},
		{
			args: []string{"security-control-api", "activation", "create", "--help"},
			want: []string{"--service-principal-id", "サービスプリンシパル", "評価対象リソース"},
		},
		{
			args: []string{"security-control-api", "activation", "update", "--help"},
			want: []string{"--service-principal-id", "--is-active", "両方とも API に送信"},
		},
		{
			args: []string{"security-control-api", "evaluation-rule", "--help"},
			want: []string{"list", "read", "update"},
		},
		{
			args: []string{"security-control-api", "evaluation-rule", "list", "--help"},
			want: []string{"--page-size", "--next", "--prev"},
		},
		{
			args: []string{"security-control-api", "evaluation-rule", "read", "--help"},
			want: []string{"rule-id", "iamRolesRequired", "isEnabled"},
		},
		{
			args: []string{"security-control-api", "evaluation-rule", "update", "--help"},
			want: []string{"--request", "@path.json", "--service-principal-id", "--is-enabled", "--targets", "個別フラグと併用できません"},
		},
		{
			args: []string{"security-control-api", "automated-action", "--help"},
			want: []string{"list", "read", "create", "update", "delete"},
		},
		{
			args: []string{"security-control-api", "automated-action", "create", "--help"},
			want: []string{"--name", "--execution-condition", "--is-enabled", "--service-principal-id", "--notification-group-id", "--workflow-id", "--revision-id", "--revision-alias", "--workflow-name", "--args", "--request", "simpleNotification または workflows", "@path で保護したファイルから、- で標準入力から読み込みます", "JSON を直接指定することはできません"},
		},
		{
			args: []string{"security-control-api", "automated-action", "update", "--help"},
			want: []string{"--request", "@path.json または - (標準入力)", "全体を置き換える", "JSON を直接指定することはできません"},
		},
		{
			args: []string{"security-control-api", "automated-action", "delete", "--help"},
			want: []string{"action-id", "削除"},
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

func TestRunSecurityControlAPIRejectsMissingServicePrincipalID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"security-control-api", "activation", "create"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "missing flags: --service-principal-id", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsActivationUpdateWithSingleFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"security-control-api", "activation", "update", "--service-principal-id", "112700012345"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "missing flags: --is-active", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsUnknownEvaluationRuleID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"security-control-api", "evaluation-rule", "update", "unknown-rule", "--is-enabled=true"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "unknown-rule", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsEvaluationRuleRequestAndFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"security-control-api", "evaluation-rule", "update", "server-no-public-ip", "--request", `{"isEnabled":true}`, "--is-enabled=true"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsAutomatedActionBothTargets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "automated-action", "create",
		"--name", "my-action", "--execution-condition", "event.evaluationResult.status == \"REJECTED\"",
		"--is-enabled=true", "--service-principal-id", "112700012345",
		"--notification-group-id", "112700056789", "--workflow-id", "workflow-1",
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--notification-group-id と --workflow-id は併用できません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsAutomatedActionRequestAndFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "automated-action", "create",
		"--request", `{"name":"my-action"}`, "--name", "my-action",
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsInlineAutomatedActionRequest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"security-control-api", "automated-action", "create", "--request", `{"name":"my-action"}`}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request には @path または - (標準入力) を指定してください", "error = %q", stderr.String())
	require.Contains(t, stderr.String(), "JSON を直接指定することはできません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsInlineAutomatedActionArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "automated-action", "create",
		"--name", "workflow-action", "--execution-condition", "condition",
		"--is-enabled=false", "--service-principal-id", "112700012345",
		"--workflow-id", "workflow-1", "--args", `{"server_id": 12345}`,
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--args には @path または - (標準入力) を指定してください", "error = %q", stderr.String())
	require.Contains(t, stderr.String(), "値を直接指定することはできません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIRejectsEvaluationRuleRequestWithEmptyTargets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "evaluation-rule", "update", "server-no-public-ip",
		"--request", `{"isEnabled":true}`, "--targets=",
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません", "error = %q", stderr.String())
}

func TestRunSecurityControlAPIAutomatedActionCreateWithRequestFile(t *testing.T) {
	var gotCreateBody map[string]any
	setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/automated_actions", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&gotCreateBody))
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"automatedActionId":"action-1","name":"my-action","action":{"actionType":"simpleNotification","actionParameter":{"servicePrincipalId":"112700012345","notificationGroupId":"112700056789"}},"executionCondition":"condition","isEnabled":false,"createdAt":"2026-01-01T00:00:00Z"}`))
	})

	requestFile := filepath.Join(t.TempDir(), "request.json")
	requestJSON := `{"name":"my-action","action":{"actionType":"simpleNotification","actionParameter":{"servicePrincipalId":"112700012345","notificationGroupId":"112700056789"}},"executionCondition":"condition","isEnabled":false}`
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"security-control-api", "automated-action", "create", "--request", "@" + requestFile}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Contains(t, stdout.String(), `"automatedActionId": "action-1"`)

	body, err := json.Marshal(gotCreateBody)
	require.NoError(t, err)
	require.JSONEq(t, requestJSON, string(body))
}

const securityControlAPIRootPath = "/cloud/zone/tk1b/api/securitycontrol/1.0"

// setSecurityControlTestEndpoint はテスト用の API サーバーをエンドポイント環境変数に設定します。
func setSecurityControlTestEndpoint(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_SECURITY_CONTROL", server.URL+securityControlAPIRootPath+"/")
	return server
}

func TestRunSecurityControlAPIActivationLifecycle(t *testing.T) {
	const servicePrincipalID = "112700012345"
	var gotActivationCreate, gotActivationUpdate map[string]any
	server := setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/activation", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			require.NoError(t, json.NewDecoder(request.Body).Decode(&gotActivationCreate))
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"servicePrincipalId":"` + servicePrincipalID + `","isActive":true,"automatedActionLimit":1}`))
		case http.MethodGet:
			_, _ = writer.Write([]byte(`{"servicePrincipalId":"` + servicePrincipalID + `","isActive":true,"automatedActionLimit":1}`))
		case http.MethodPut:
			require.NoError(t, json.NewDecoder(request.Body).Decode(&gotActivationUpdate))
			_, _ = writer.Write([]byte(`{"servicePrincipalId":"` + servicePrincipalID + `","isActive":false,"automatedActionLimit":1}`))
		default:
			http.Error(writer, "unexpected method", http.StatusNotFound)
		}
	})
	_ = server

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"security-control-api", "activation", "create", "--service-principal-id", servicePrincipalID}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Equal(t, map[string]any{"servicePrincipalId": servicePrincipalID}, gotActivationCreate)
	require.Contains(t, stdout.String(), `"servicePrincipalId": "`+servicePrincipalID+`"`)
	require.Contains(t, stdout.String(), `"isActive": true`)

	stdout.Reset()
	require.Equal(t, 0, run([]string{"security-control-api", "activation", "read"}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Contains(t, stdout.String(), `"automatedActionLimit": 1`)

	stdout.Reset()
	require.Equal(t, 0, run([]string{"security-control-api", "activation", "update", "--service-principal-id", servicePrincipalID, "--is-active=false"}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Equal(t, map[string]any{"servicePrincipalId": servicePrincipalID, "isActive": false}, gotActivationUpdate)
	require.Contains(t, stdout.String(), `"isActive": false`)
}

func TestRunSecurityControlAPIEvaluationRuleUpdateWithFlags(t *testing.T) {
	var gotUpdateBody map[string]any
	setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/evaluation_rules/server-no-public-ip", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPut {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&gotUpdateBody))
		_, _ = writer.Write([]byte(`{"rule":{"evaluationRuleId":"server-no-public-ip","parameter":{"servicePrincipalId":"112700012345","zones":["is1b"]}},"description":"desc","iamRolesRequired":["role"],"isEnabled":true}`))
	})

	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "evaluation-rule", "update", "server-no-public-ip",
		"--service-principal-id", "112700012345", "--targets", "is1b", "--is-enabled=true",
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Contains(t, stdout.String(), `"evaluationRuleId": "server-no-public-ip"`)
	require.Contains(t, stdout.String(), `"zones": [`)
	require.Contains(t, stdout.String(), `"isEnabled": true`)

	body, err := json.Marshal(gotUpdateBody)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"rule": {"evaluationRuleId": "server-no-public-ip", "parameter": {"servicePrincipalId": "112700012345", "zones": ["is1b"]}},
		"isEnabled": true
	}`, string(body))
}

func TestRunSecurityControlAPIEvaluationRuleList(t *testing.T) {
	setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/evaluation_rules", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"items":[{"rule":{"evaluationRuleId":"server-no-public-ip","parameter":{}},"iamRolesRequired":[],"isEnabled":true}],"totalSize":1}`))
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"security-control-api", "evaluation-rule", "list"}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Contains(t, stdout.String(), `"evaluationRuleId": "server-no-public-ip"`)
}

func TestRunSecurityControlAPIAutomatedActionCreateWithFlags(t *testing.T) {
	var gotCreateBody map[string]any
	setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/automated_actions", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&gotCreateBody))
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"automatedActionId":"action-1","name":"evaluation-rejected","action":{"actionType":"simpleNotification","actionParameter":{"servicePrincipalId":"112700012345","notificationGroupId":"112700056789"}},"executionCondition":"event.evaluationResult.status == \"REJECTED\"","isEnabled":true,"createdAt":"2026-01-01T00:00:00Z"}`))
	})

	var stdout, stderr bytes.Buffer
	args := []string{
		"security-control-api", "automated-action", "create",
		"--name", "evaluation-rejected",
		"--execution-condition", `event.evaluationResult.status == "REJECTED"`,
		"--is-enabled=true", "--service-principal-id", "112700012345",
		"--notification-group-id", "112700056789",
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Contains(t, stdout.String(), `"automatedActionId": "action-1"`)

	body, err := json.Marshal(gotCreateBody)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"name": "evaluation-rejected",
		"action": {"actionType": "simpleNotification", "actionParameter": {"servicePrincipalId": "112700012345", "notificationGroupId": "112700056789"}},
		"executionCondition": "event.evaluationResult.status == \"REJECTED\"",
		"isEnabled": true
	}`, string(body))
}

func TestRunSecurityControlAPIAutomatedActionDelete(t *testing.T) {
	setSecurityControlTestEndpoint(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, securityControlAPIRootPath+"/automated_actions/action-1", request.URL.Path)
		if request.Method != http.MethodDelete {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"security-control-api", "automated-action", "delete", "action-1"}, &stdout, &stderr), "stderr: %s", stderr.String())
}
