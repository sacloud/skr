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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	monitoringsuitemock "github.com/sacloud/sakumock/monitoringsuite"
)

func setupMonitoringSuiteMock(t *testing.T) {
	t.Helper()
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := monitoringsuitemock.NewTestServer(monitoringsuitemock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_MONITORING_SUITE", server.TestURL())
}

func TestRunMonitoringSuiteAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"monitoring-suite-api", "--help"},
			want: []string{
				"list-alert-projects", "create-alert-project", "read-alert-project", "patch-alert-project", "delete-alert-project",
				"list-alert-project-histories", "read-alert-project-history",
				"list-alert-project-rules", "create-alert-project-rule", "read-alert-project-rule", "patch-alert-project-rule", "delete-alert-project-rule",
				"list-alert-project-log-measure-rules", "create-alert-project-log-measure-rule", "patch-alert-project-log-measure-rule",
				"list-alert-project-notification-targets", "create-alert-project-notification-target", "patch-alert-project-notification-target",
				"list-alert-project-notification-routings", "create-alert-project-notification-routing", "reorder-alert-project-notification-routings",
				"list-dashboard-projects", "create-dashboard-project", "patch-dashboard-project",
				"list-log-routings", "create-log-routing", "patch-log-routing",
				"list-log-storages", "create-log-storage", "patch-log-storage", "set-log-storage-expire", "list-log-storage-keys", "create-log-storage-key",
				"list-metric-routings", "create-metric-routing", "patch-metric-routing",
				"list-metric-storages", "create-metric-storage", "patch-metric-storage", "list-metric-storage-keys", "create-metric-storage-key",
				"list-trace-storages", "create-trace-storage", "patch-trace-storage", "set-trace-storage-expire",
				"list-publishers", "read-publisher",
				"read-resource-limits", "read-provisioning-state", "initialize-provisioning",
				"SAKURA_ENDPOINTS_MONITORING_SUITE", "課金",
			},
		},
		{
			args: []string{"monitoring-suite-api", "create-alert-project", "--help"},
			want: []string{"--name", "--description", "--request", "@path.json"},
		},
		{
			args: []string{"monitoring-suite-api", "create-alert-project-rule", "--help"},
			want: []string{"project-id", "--metrics-storage-id", "--alert-query", "--threshold-warning", "--threshold-critical", "--request"},
		},
		{
			args: []string{"monitoring-suite-api", "create-alert-project-notification-target", "--help"},
			want: []string{"project-id", "--service-type", "SAKURA_SIMPLE_NOTICE", "--url", "--description", "--request"},
		},
		{
			args: []string{"monitoring-suite-api", "list-alert-project-histories", "--help"},
			want: []string{"project-id", "--open", "--severity", "critical", "warning", "--starts-at", "RFC3339"},
		},
		{
			args: []string{"monitoring-suite-api", "reorder-alert-project-notification-routings", "--help"},
			want: []string{"project-id", "--request", "NotificationRoutingUID", "Order"},
		},
		{
			args: []string{"monitoring-suite-api", "create-log-routing", "--help"},
			want: []string{"--publisher-code", "--variant", "--log-storage-id", "--resource-id", "--request"},
		},
		{
			args: []string{"monitoring-suite-api", "create-log-storage", "--help"},
			want: []string{"--name", "--classification", "shared", "dedicated", "--kms-key-id", "--service-principal-id", "--request"},
		},
		{
			args: []string{"monitoring-suite-api", "set-log-storage-expire", "--help"},
			want: []string{"id", "--days", "保存期間"},
		},
		{
			args: []string{"monitoring-suite-api", "initialize-provisioning", "--help"},
			want: []string{"--request", "Logs", "Metrics"},
		},
		{
			args: []string{"monitoring-suite-api", "list-log-storages", "--help"},
			want: []string{"--bucket-classification", "shared", "dedicated", "--status", "assigned", "free", "--count", "--from"},
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

// runMonitoringSuiteCommand は sakumock に対してコマンドを実行し、標準出力を JSON で返します。
func runMonitoringSuiteCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
	return stdout.Bytes()
}

func TestRunMonitoringSuiteAPIAlertProjectLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// create-alert-project: 個別フラグ経路
	var created struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		ResourceID  int64  `json:"resource_id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project",
		"--name", "mock-project",
		"--description", "test project",
	), &created), "create output")
	require.NotZero(t, created.ID, "created = %+v", created)
	require.Equal(t, "mock-project", created.Name)
	require.Equal(t, "test project", created.Description)
	projectID := created.ID

	// create-alert-project: --request 経路
	var createdJSON struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project",
		"--request", `{"Name":"json-project"}`,
	), &createdJSON), "create with --request")
	require.NotZero(t, createdJSON.ID)
	require.Equal(t, "json-project", createdJSON.Name)

	// list-alert-projects
	var listed []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "list-alert-projects"), &listed), "list output")
	require.Len(t, listed, 2)

	// read-alert-project
	var read struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-alert-project", int64String(projectID),
	), &read), "read output")
	require.Equal(t, projectID, read.ID)

	// patch-alert-project: 個別フラグ経路 (指定した項目だけを送る)
	var patched struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project", int64String(projectID),
		"--name", "renamed-project",
	), &patched), "patch output")
	require.Equal(t, "renamed-project", patched.Name)

	// patch-alert-project: --request 経路。説明だけを変更する
	var patchedDesc struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project", int64String(projectID),
		"--request", `{"Description":"updated desc"}`,
	), &patchedDesc), "patch with --request")
	require.Equal(t, "renamed-project", patchedDesc.Name)
	require.NotNil(t, patchedDesc.Description)
	require.Equal(t, "updated desc", *patchedDesc.Description)

	// delete-alert-project
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-alert-project", int64String(projectID)}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPIAlertProjectRequestFlagExclusive(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var stdout, stderr bytes.Buffer
	// --request と個別フラグの併用はできない
	require.NotEqual(t, 0, run([]string{
		"monitoring-suite-api", "create-alert-project",
		"--request", `{"Name":"x"}`, "--name", "y",
	}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません")
}

func TestRunMonitoringSuiteAPINotificationTargetValidation(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var stdout, stderr bytes.Buffer

	// 不正な URL は作成前にエラーになる
	require.NotEqual(t, 0, run([]string{
		"monitoring-suite-api", "create-alert-project-notification-target", "1",
		"--service-type", "SAKURA_SIMPLE_NOTICE", "--url", "://invalid",
	}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "--url")

	// --service-type は必須
	require.NotEqual(t, 0, run([]string{
		"monitoring-suite-api", "create-alert-project-notification-target", "1",
		"--url", "https://example.test/hook",
	}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "--service-type")

	// --url は必須
	require.NotEqual(t, 0, run([]string{
		"monitoring-suite-api", "create-alert-project-notification-target", "1",
		"--service-type", "SAKURA_SIMPLE_NOTICE",
	}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "--url")
}

// int64String は数値リソース ID をコマンド引数の文字列に変換します。
func int64String(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestRunMonitoringSuiteAPIAlertRuleLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// メトリクスストレージを先に作る (アラートルールの転送先)
	var storage struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-storage", "--name", "mock-metrics",
	), &storage), "create-metric-storage output")
	require.NotZero(t, storage.ID)
	storageID := storage.ID

	// アラートプロジェクトを作る
	var project struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project", "--name", "rule-project",
	), &project), "create-alert-project output")
	projectID := int64String(project.ID)

	// create-alert-project-rule: 個別フラグ経路
	var created struct {
		UID              string  `json:"uid"`
		Name             string  `json:"name"`
		Query            string  `json:"query"`
		MetricsStorageID *int64  `json:"metrics_storage_id"`
		ThresholdWarning *string `json:"threshold_warning"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project-rule", projectID,
		"--metrics-storage-id", int64String(storageID),
		"--alert-query", `{job="test"} | rate() > 1`,
		"--name", "mock-rule",
		"--threshold-warning", "10",
	), &created), "create-alert-project-rule output")
	require.NotEmpty(t, created.UID, "created = %+v", created)
	require.Equal(t, "mock-rule", created.Name)
	require.NotNil(t, created.MetricsStorageID)
	require.Equal(t, storageID, *created.MetricsStorageID)
	require.NotNil(t, created.ThresholdWarning)
	ruleUID := created.UID

	// patch-alert-project-rule: 臨界値だけを変更する
	var patched struct {
		Name              string  `json:"name"`
		ThresholdCritical *string `json:"threshold_critical"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project-rule", projectID, ruleUID,
		"--threshold-critical", "20",
	), &patched), "patch-alert-project-rule output")
	require.Equal(t, "mock-rule", patched.Name)
	require.NotNil(t, patched.ThresholdCritical)
	require.Equal(t, "20", *patched.ThresholdCritical)

	// list-alert-project-rules
	var listed []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-rules", projectID,
	), &listed), "list-alert-project-rules output")
	require.Len(t, listed, 1)

	// read-alert-project-rule
	var read struct {
		UID  string `json:"uid"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-alert-project-rule", projectID, ruleUID,
	), &read), "read-alert-project-rule output")
	require.Equal(t, ruleUID, read.UID)

	// delete-alert-project-rule
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-alert-project-rule", projectID, ruleUID}, &stdoutHolder{}, &stderr))
	require.Empty(t, stderr.String(), "stderr = %q", stderr.String())
}

func TestRunMonitoringSuiteAPINotificationTargetLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var project struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project", "--name", "target-project",
	), &project), "create-alert-project output")
	projectID := int64String(project.ID)

	// create-alert-project-notification-target: 個別フラグ経路
	var created struct {
		UID         string `json:"uid"`
		ServiceType string `json:"service_type"`
		URL         string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project-notification-target", projectID,
		"--service-type", "SAKURA_SIMPLE_NOTICE",
		"--url", "https://example.test/hook",
		"--description", "test target",
	), &created), "create-alert-project-notification-target output")
	require.NotEmpty(t, created.UID, "created = %+v", created)
	require.Equal(t, "https://example.test/hook", created.URL)
	targetUID := created.UID

	// patch-alert-project-notification-target: --request 経路で URL を変更する
	var patched struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project-notification-target", projectID, targetUID,
		"--request", `{"URL":"https://example.test/hook2"}`,
	), &patched), "patch-alert-project-notification-target output")
	require.Equal(t, "https://example.test/hook2", patched.URL)

	// list-alert-project-notification-targets
	var listed []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-notification-targets", projectID,
	), &listed), "list-alert-project-notification-targets output")
	require.Len(t, listed, 1)

	// delete-alert-project-notification-target
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-alert-project-notification-target", projectID, targetUID}, &stdoutHolder{}, &stderr))
	require.Empty(t, stderr.String(), "stderr = %q", stderr.String())
}

func TestRunMonitoringSuiteAPILogStorageLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// create-log-storage: 個別フラグ経路
	var created struct {
		ID             int64  `json:"id"`
		Name           string `json:"name"`
		Classification string `json:"classification"`
		ExpireDay      int    `json:"expire_day"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-log-storage",
		"--name", "mock-logs",
		"--classification", "shared",
	), &created), "create-log-storage output")
	require.NotZero(t, created.ID, "created = %+v", created)
	require.Equal(t, "mock-logs", created.Name)
	require.Equal(t, "shared", created.Classification)
	storageID := int64String(created.ID)

	// set-log-storage-expire: 保存期間を変更する
	var expired struct {
		ExpireDay int `json:"expire_day"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "set-log-storage-expire", storageID, "--days", "90",
	), &expired), "set-log-storage-expire output")
	require.Equal(t, 90, expired.ExpireDay)

	// アクセスキーを発行する
	var key struct {
		UID         string `json:"uid"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-log-storage-key", storageID,
		"--description", "agent key",
	), &key), "create-log-storage-key output")
	require.NotEmpty(t, key.UID, "key = %+v", key)
	require.Equal(t, "agent key", key.Description)

	// patch-log-storage-key: 説明を変更する
	var patchedKey struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-log-storage-key", storageID, key.UID,
		"--description", "renamed key",
	), &patchedKey), "patch-log-storage-key output")
	require.Equal(t, "renamed key", patchedKey.Description)

	// list-log-storage-keys
	var listedKeys []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-log-storage-keys", storageID,
	), &listedKeys), "list-log-storage-keys output")
	require.Len(t, listedKeys, 1)

	// read-log-storage-key
	var readKey struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-log-storage-key", storageID, key.UID,
	), &readKey), "read-log-storage-key output")
	require.Equal(t, key.UID, readKey.UID)

	// delete-log-storage-key
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-log-storage-key", storageID, key.UID}, &stdoutHolder{}, &stderr))

	// patch-log-storage: --request 経路で説明を変更する
	var patched struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-log-storage", storageID,
		"--request", `{"Description":"updated"}`,
	), &patched), "patch-log-storage output")
	require.Equal(t, "updated", patched.Description)

	// patch-log-storage: --request 経路で service_principal_id を null にできる (フラグでは数値のみ)
	var patchedNull struct {
		ServicePrincipalID *int64 `json:"service_principal_id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-log-storage", storageID,
		"--request", `{"ServicePrincipalID":null}`,
	), &patchedNull), "patch-log-storage with null ServicePrincipalID output")
	require.Nil(t, patchedNull.ServicePrincipalID, "service_principal_id should be null")

	// delete-log-storage
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-log-storage", storageID}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPIMisc(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// list-publishers / read-publisher
	var publishers []struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "list-publishers"), &publishers), "list-publishers output")
	require.NotEmpty(t, publishers)
	require.Equal(t, "appliance", publishers[0].Code)

	var publisher struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-publisher", "otel",
	), &publisher), "read-publisher output")
	require.Equal(t, "otel", publisher.Code)

	// read-resource-limits
	var limits map[string]any
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "read-resource-limits"), &limits), "read-resource-limits output")
	require.NotEmpty(t, limits)

	// read-provisioning-state
	var provisioning struct {
		Logs struct {
			SystemExist bool `json:"system_exist"`
			UserExist   bool `json:"user_exist"`
		} `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "read-provisioning-state"), &provisioning), "read-provisioning-state output")

	// initialize-provisioning: --request JSON で領域の有無を指定する
	var initialized struct {
		Logs struct {
			SystemExist bool `json:"system_exist"`
			UserExist   bool `json:"user_exist"`
		} `json:"logs"`
		Metrics struct {
			SystemExist bool `json:"system_exist"`
			UserExist   bool `json:"user_exist"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "initialize-provisioning",
		"--request", `{"Logs":{"system_exist":true,"user_exist":true},"Metrics":{"system_exist":true,"user_exist":true}}`,
	), &initialized), "initialize-provisioning output")
	require.True(t, initialized.Logs.SystemExist, "initialized = %+v", initialized)
	require.True(t, initialized.Metrics.UserExist, "initialized = %+v", initialized)
}

func TestRunMonitoringSuiteAPIDashboardLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// create-dashboard-project
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-dashboard-project", "--name", "mock-dashboard", "--description", "skr mock",
	), &created), "create-dashboard-project output")
	require.NotZero(t, created.ID, "created = %+v", created)
	dashboardID := int64String(created.ID)

	// patch-dashboard-project
	var patched struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-dashboard-project", dashboardID,
		"--request", `{"Name":"renamed-dashboard"}`,
	), &patched), "patch-dashboard-project output")
	require.Equal(t, "renamed-dashboard", patched.Name)

	// list-dashboard-projects
	var listed []struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "list-dashboard-projects"), &listed), "list-dashboard-projects output")
	require.Len(t, listed, 1)

	// delete-dashboard-project
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-dashboard-project", dashboardID}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPILogAndMetricRouting(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// 転送先のストレージを作る
	var logStorage struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-log-storage", "--name", "routing-logs",
	), &logStorage), "create-log-storage output")
	var metricStorage struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-storage", "--name", "routing-metrics",
	), &metricStorage), "create-metric-storage output")

	// create-log-routing: publisher と variant とストレージを指定する
	var created struct {
		UID       string `json:"uid"`
		Variant   string `json:"variant"`
		Publisher struct {
			Code string `json:"code"`
		} `json:"publisher"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-log-routing",
		"--publisher-code", "appliance",
		"--variant", "logs",
		"--log-storage-id", int64String(logStorage.ID),
	), &created), "create-log-routing output")
	require.NotEmpty(t, created.UID, "created = %+v", created)
	require.Equal(t, "logs", created.Variant)
	require.Equal(t, "appliance", created.Publisher.Code)

	// list-log-routings / read-log-routing
	var listed []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "list-log-routings"), &listed), "list-log-routings output")
	require.Len(t, listed, 1)

	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-log-routing", created.UID}, &stdoutHolder{}, &stderr))

	// メトリクスのルーティング (UID で扱う)
	var metricRouting struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-routing",
		"--publisher-code", "otel",
		"--variant", "metrics",
		"--metrics-storage-id", int64String(metricStorage.ID),
	), &metricRouting), "create-metric-routing output")
	require.NotEmpty(t, metricRouting.UID, "metricRouting = %+v", metricRouting)

	// read-metric-routing / patch-metric-routing
	var read struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-metric-routing", metricRouting.UID,
	), &read), "read-metric-routing output")
	require.Equal(t, metricRouting.UID, read.UID)

	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-metric-routing", metricRouting.UID}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPINotificationRoutingAndReorder(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var project struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project", "--name", "routing-project",
	), &project), "create-alert-project output")
	projectID := int64String(project.ID)

	// 通知先を作り、その UID をルーティングに使う
	var target struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project-notification-target", projectID,
		"--service-type", "SAKURA_SIMPLE_NOTICE",
		"--url", "https://example.test/hook",
	), &target), "create-alert-project-notification-target output")

	// create-alert-project-notification-routing: --request JSON 経路
	var created struct {
		UID   string `json:"uid"`
		Order int    `json:"order"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project-notification-routing", projectID,
		"--request", `{"NotificationTargetUID":"`+target.UID+`","MatchLabels":[{"name":"severity","value":"critical"}]}`,
	), &created), "create-alert-project-notification-routing output")
	require.NotEmpty(t, created.UID, "created = %+v", created)

	// reorder-alert-project-notification-routings: --request JSON 経路 (ogen 型のためキーは snake_case)
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{
		"monitoring-suite-api", "reorder-alert-project-notification-routings", projectID,
		"--request", `[{"notification_routing_uid":"` + created.UID + `","order":2}]`,
	}, &stdoutHolder{}, &stderr), "stderr: %s", stderr.String())

	// list-alert-project-notification-routings
	var listed []struct {
		UID   string `json:"uid"`
		Order int    `json:"order"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-notification-routings", projectID,
	), &listed), "list-alert-project-notification-routings output")
	require.Len(t, listed, 1)
	require.Equal(t, 2, listed[0].Order, "listed = %+v", listed)

	// patch-alert-project-notification-routing: --request JSON 経路で再送間隔を変更する
	var patched struct {
		UID                   string `json:"uid"`
		ResendIntervalMinutes int    `json:"resend_interval_minutes"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project-notification-routing", projectID, created.UID,
		"--request", `{"NotificationTargetUID":"`+target.UID+`","ResendIntervalMinutes":30}`,
	), &patched), "patch-alert-project-notification-routing output")
	require.Equal(t, created.UID, patched.UID)
	require.Equal(t, 30, patched.ResendIntervalMinutes)

	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-alert-project-notification-routing", projectID, created.UID}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPILogMeasureRule(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var logStorage struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-log-storage", "--name", "measure-logs",
	), &logStorage), "create-log-storage output")
	var metricStorage struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-storage", "--name", "measure-metrics",
	), &metricStorage), "create-metric-storage output")

	var project struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project", "--name", "measure-project",
	), &project), "create-alert-project output")
	projectID := int64String(project.ID)

	// create-alert-project-log-measure-rule: --request JSON 経路
	var created struct {
		UID  string `json:"uid"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project-log-measure-rule", projectID,
		// Rule の matcher は API の sum 型に従う
		"--request", `{"LogStorageID":"`+int64String(logStorage.ID)+`","MetricsStorageID":"`+int64String(metricStorage.ID)+`","Name":"measure-rule",`+
			`"Rule":{"version":"v1","query":{"matchers":[{"type":"string","operator":"eq","field":"log_name","value":"app","value_list":[]}]}}}`,
	), &created), "create-alert-project-log-measure-rule output")
	require.NotEmpty(t, created.UID, "created = %+v", created)
	require.Equal(t, "measure-rule", created.Name)
	ruleUID := created.UID

	// patch-alert-project-log-measure-rule: 説明だけを変更する
	var patched struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-alert-project-log-measure-rule", projectID, ruleUID,
		"--request", `{"Description":"updated"}`,
	), &patched), "patch-alert-project-log-measure-rule output")
	require.Equal(t, "measure-rule", patched.Name)

	// list-alert-project-log-measure-rules
	var listed []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-log-measure-rules", projectID,
	), &listed), "list-alert-project-log-measure-rules output")
	require.Len(t, listed, 1)

	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-alert-project-log-measure-rule", projectID, ruleUID}, &stdoutHolder{}, &stderr))
}

func TestRunMonitoringSuiteAPIHistories(t *testing.T) {
	setupMonitoringSuiteMock(t)

	var project struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-alert-project", "--name", "history-project",
	), &project), "create-alert-project output")
	projectID := int64String(project.ID)

	// list-alert-project-histories: sakumock は空の履歴を返す
	var listed []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-histories", projectID,
		"--severity", "critical", "--open",
	), &listed), "list-alert-project-histories output")
	require.Empty(t, listed, "listed = %+v", listed)

	// list-alert-project-log-measure-rule-histories: sakumock は空の履歴を返す
	var measureRuleHistories []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-alert-project-log-measure-rule-histories", projectID,
	), &measureRuleHistories), "list-alert-project-log-measure-rule-histories output")
	require.Empty(t, measureRuleHistories, "measureRuleHistories = %+v", measureRuleHistories)

	// 存在しない履歴の読み取りはエラーになる
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"read-alert-project-history", []string{"read-alert-project-history", projectID, "00000000-0000-0000-0000-000000000000"}},
		{"read-alert-project-rule-history", []string{"read-alert-project-rule-history", projectID, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000"}},
		{"read-alert-project-log-measure-rule-history", []string{"read-alert-project-log-measure-rule-history", projectID, "00000000-0000-0000-0000-000000000000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.NotEqual(t, 0, run(append([]string{"monitoring-suite-api"}, tc.args...), &stdout, &stderr))
			require.Contains(t, stderr.String(), "404")
		})
	}
}

func TestRunMonitoringSuiteAPIStorageStats(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// 各ストレージを作成して日次・月次利用量を取得する。データがなくても正常応答する。
	for _, tc := range []struct {
		kind       string
		createArgs []string
		dailyCmd   string
		monthlyCmd string
	}{
		{
			kind:       "log",
			createArgs: []string{"create-log-storage", "--name", "stats-logs"},
			dailyCmd:   "read-log-storage-stats-daily",
			monthlyCmd: "read-log-storage-stats-monthly",
		},
		{
			kind:       "metric",
			createArgs: []string{"create-metric-storage", "--name", "stats-metrics"},
			dailyCmd:   "read-metric-storage-stats-daily",
			monthlyCmd: "read-metric-storage-stats-monthly",
		},
		{
			kind:       "trace",
			createArgs: []string{"create-trace-storage", "--name", "stats-traces", "--classification", "shared"},
			dailyCmd:   "read-trace-storage-stats-daily",
			monthlyCmd: "read-trace-storage-stats-monthly",
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var created struct {
				ID int64 `json:"id"`
			}
			require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
				append([]string{"monitoring-suite-api"}, tc.createArgs...)...,
			), &created), "create output")
			storageID := int64String(created.ID)

			year := strconv.Itoa(time.Now().Year())
			for _, cmd := range []string{tc.dailyCmd, tc.monthlyCmd} {
				args := []string{"monitoring-suite-api", cmd, storageID}
				if strings.HasSuffix(cmd, "monthly") {
					// 月次利用量は年が必須 (実 API と同じ)
					args = append(args, "--year", year)
				}
				var usages []map[string]any
				require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, args...), &usages), cmd+" output")
				require.Empty(t, usages, cmd+" should return an empty usage list")
			}
		})
	}
}

func TestRunMonitoringSuiteAPITracesStorageLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// create-trace-storage
	var created struct {
		ID             int64  `json:"id"`
		Name           string `json:"name"`
		Classification string `json:"classification"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-trace-storage",
		"--name", "mock-traces",
		"--classification", "shared",
	), &created), "create-trace-storage output")
	require.NotZero(t, created.ID, "created = %+v", created)
	storageID := int64String(created.ID)

	// set-trace-storage-expire
	var expired struct {
		RetentionPeriodDays int `json:"retention_period_days"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "set-trace-storage-expire", storageID, "--days", "60",
	), &expired), "set-trace-storage-expire output")
	require.Equal(t, 60, expired.RetentionPeriodDays)

	// list-trace-storages / read-trace-storage
	var listed []struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t, "monitoring-suite-api", "list-trace-storages"), &listed), "list-trace-storages output")
	require.Len(t, listed, 1)

	// create-trace-storage-key: 応答にはキーの値が含まれるため、そのまま検証する
	var createdKey struct {
		UID         string `json:"uid"`
		Secret      string `json:"secret"`
		Token       string `json:"token"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-trace-storage-key", storageID,
		"--description", "mock-key",
	), &createdKey), "create-trace-storage-key output")
	require.NotEmpty(t, createdKey.UID, "createdKey = %+v", createdKey)
	require.NotEmpty(t, createdKey.Secret, "createdKey = %+v", createdKey)
	require.Equal(t, "mock-key", createdKey.Description)

	// patch-trace-storage-key
	var patchedKey struct {
		UID         string `json:"uid"`
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-trace-storage-key", storageID, createdKey.UID,
		"--description", "mock-key patched",
	), &patchedKey), "patch-trace-storage-key output")
	require.Equal(t, createdKey.UID, patchedKey.UID)
	require.Equal(t, "mock-key patched", patchedKey.Description)

	// read-trace-storage-key
	var readKey struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-trace-storage-key", storageID, createdKey.UID,
	), &readKey), "read-trace-storage-key output")
	require.Equal(t, createdKey.UID, readKey.UID)

	// list-trace-storage-keys
	var listedKeys []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-trace-storage-keys", storageID,
	), &listedKeys), "list-trace-storage-keys output")
	require.Contains(t, listedKeysUIDs(listedKeys), createdKey.UID)

	// delete-trace-storage-key / read で不在確認
	runMonitoringSuiteCommand(t, "monitoring-suite-api", "delete-trace-storage-key", storageID, createdKey.UID)
	var stderrAfterDelete bytes.Buffer
	require.NotEqual(t, 0, run([]string{"monitoring-suite-api", "read-trace-storage-key", storageID, createdKey.UID}, &stdoutHolder{}, &stderrAfterDelete))
	require.Contains(t, stderrAfterDelete.String(), "API Error 404")

	// delete-trace-storage
	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-trace-storage", storageID}, &stdoutHolder{}, &stderr))
}

// listedKeysUIDs はアクセスキー一覧から UID 集合を作ります。
func listedKeysUIDs(items []struct {
	UID string `json:"uid"`
}) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.UID)
	}
	return out
}

func TestRunMonitoringSuiteAPIMetricStorageLifecycle(t *testing.T) {
	setupMonitoringSuiteMock(t)

	// create-metric-storage
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-storage", "--name", "mock-metrics-storage",
	), &created), "create-metric-storage output")
	storageID := int64String(created.ID)

	// アクセスキーの一式
	var key struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "create-metric-storage-key", storageID,
		"--description", "collector key",
	), &key), "create-metric-storage-key output")
	require.NotEmpty(t, key.UID, "key = %+v", key)

	var listedKeys []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "list-metric-storage-keys", storageID,
	), &listedKeys), "list-metric-storage-keys output")
	require.Len(t, listedKeys, 1)

	var stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-metric-storage-key", storageID, key.UID}, &stdoutHolder{}, &stderr))

	// patch-metric-storage: 説明を変更する
	var patched struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "patch-metric-storage", storageID,
		"--request", `{"Description":"updated"}`,
	), &patched), "patch-metric-storage output")
	require.Equal(t, "updated", patched.Description)

	// read-metric-storage
	var read struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(runMonitoringSuiteCommand(t,
		"monitoring-suite-api", "read-metric-storage", storageID,
	), &read), "read-metric-storage output")
	require.Equal(t, created.ID, read.ID)

	require.Equal(t, 0, run([]string{"monitoring-suite-api", "delete-metric-storage", storageID}, &stdoutHolder{}, &stderr))
}
