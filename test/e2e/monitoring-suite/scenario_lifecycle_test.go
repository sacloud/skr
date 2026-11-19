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

// monitoringSuiteJSON は monitoring-suite-api の任意の JSON 応答を保持します。
type monitoringSuiteJSON map[string]any

// callMonitoringSuite は monitoring-suite-api コマンドを実行して JSON 応答を返します。
func callMonitoringSuite(t *testing.T, h *helper.Helper, description string, args ...string) monitoringSuiteJSON {
	t.Helper()
	data := h.Call(t, description, args...)
	var out monitoringSuiteJSON
	require.NoError(t, json.Unmarshal(data, &out), "output of %v is not a JSON object: %s", args, data)
	return out
}

// monitoringSuiteIDText は応答から ID を文字列として取り出します。
func monitoringSuiteIDText(t *testing.T, data monitoringSuiteJSON, key string) string {
	t.Helper()
	raw, ok := data[key]
	require.True(t, ok, "output does not contain %s: %s", key, data)
	switch v := raw.(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case string:
		return v
	default:
		t.Fatalf("unexpected type for %s: %T (%v)", key, raw, raw)
		return ""
	}
}

// pollMonitoringSuiteRead はリソースの反映を待ちながら read コマンドを実行します。
// 作成直後の読み取りは 404 になることがあるため、期限付きで再試行します。
func pollMonitoringSuiteRead(t *testing.T, h *helper.Helper, description string, timeout time.Duration, args ...string) monitoringSuiteJSON {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		data, _, err := h.CallAllowingFailure(t, description, args...)
		if err == nil {
			var out monitoringSuiteJSON
			require.NoError(t, json.Unmarshal(data, &out), "output of %v is not a JSON object: %s", args, data)
			return out
		}
		if time.Now().After(deadline) {
			require.Failf(t, "timed out waiting for resource", "read %v timed out: %s", args, err)
		}
		time.Sleep(3 * time.Second)
	}
}

// monitoringSuiteWaitGone はリソースの削除後の不在を期限付きで確認します。
func monitoringSuiteWaitGone(t *testing.T, h *helper.Helper, description string, timeout time.Duration, args ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		_, _, err := h.CallAllowingFailure(t, description, args...)
		if err == nil {
			if time.Now().After(deadline) {
				require.Failf(t, "timed out waiting for resource deletion", "read %v still succeeded", args)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		// 読み取りが失敗すれば不在とみなす (失敗内容は実行記録に残る)
		return
	}
}

// TestScenarioMonitoringSuiteLifecycle は monitoring-suite-api コマンドのライブ E2E です。
// テスト専用のアラートプロジェクト、メトリクスストレージ、ログストレージ、トレースストレージを
// 作成し、ルールや通知、ルーティング、アクセスキーの一連の操作を検証します。
// 作成したリソースはすべてテスト内で削除します。既存のリソースとプロビジョニング状態は変更しません。
func TestScenarioMonitoringSuiteLifecycle(t *testing.T) {
	h := helper.New(t, "monitoring-suite-api",
		"モニタリングスイートのストレージ・アラート・通知・ルーティングのライフサイクルと読み取り系コマンドが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	projectName := "skr-e2e-ms-" + suffix
	logStorageName := "skr-e2e-ms-logs-" + suffix
	metricStorageName := "skr-e2e-ms-metrics-" + suffix
	traceStorageName := "skr-e2e-ms-traces-" + suffix
	dashboardName := "skr-e2e-ms-dashboard-" + suffix

	// --- 読み取り系コマンド ---
	publishers := h.Call(t, "パブリッシャーの一覧を取得する", "monitoring-suite-api", "list-publishers")
	var publisherList []struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(publishers, &publisherList), "list-publishers output: %s", publishers)
	require.NotEmpty(t, publisherList, "list-publishers should return at least one publisher")
	publisherCode := publisherList[0].Code

	publisher := callMonitoringSuite(t, h, "パブリッシャー "+publisherCode+" を参照する", "monitoring-suite-api", "read-publisher", publisherCode)
	require.Equal(t, publisherCode, publisher["code"], "read-publisher output: %v", publisher)

	limits := callMonitoringSuite(t, h, "リソース上限を参照する", "monitoring-suite-api", "read-resource-limits")
	require.NotEmpty(t, limits, "read-resource-limits should return limits: %v", limits)

	provisioning := callMonitoringSuite(t, h, "プロビジョニング状態を参照する", "monitoring-suite-api", "read-provisioning-state")
	t.Logf("プロビジョニング状態: %v", provisioning)

	// --- ダッシュボードプロジェクト ---
	dashboard := callMonitoringSuite(t, h, "ダッシュボードプロジェクト "+dashboardName+" を作成する",
		"monitoring-suite-api", "create-dashboard-project", "--name", dashboardName, "--description", "skr e2e")
	dashboardID := monitoringSuiteIDText(t, dashboard, "id")
	require.Equal(t, dashboardName, dashboard["name"], "create-dashboard-project output: %v", dashboard)

	dashboardRead := callMonitoringSuite(t, h, "ダッシュボードプロジェクト "+dashboardID+" を参照する",
		"monitoring-suite-api", "read-dashboard-project", dashboardID)
	require.Equal(t, dashboardID, monitoringSuiteIDText(t, dashboardRead, "id"))

	dashboardPatched := callMonitoringSuite(t, h, "ダッシュボードプロジェクト "+dashboardID+" の説明を更新する",
		"monitoring-suite-api", "patch-dashboard-project", dashboardID, "--request", `{"Description":"skr e2e"}`)
	require.Equal(t, "skr e2e", dashboardPatched["description"], "patch-dashboard-project output: %v", dashboardPatched)

	dashboardList := h.Call(t, "ダッシュボードプロジェクトの一覧を取得する", "monitoring-suite-api", "list-dashboard-projects")
	var dashboards []struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	require.NoError(t, json.Unmarshal(dashboardList, &dashboards), "list-dashboard-projects output: %s", dashboardList)
	require.Contains(t, dashboardNames(dashboards), dashboardName, "list-dashboard-projects should contain the created dashboard")
	require.Contains(t, dashboardIDs(t, dashboards), dashboardID, "list-dashboard-projects should contain the created dashboard ID")

	h.Call(t, "後片付け: ダッシュボードプロジェクト "+dashboardID+" を削除する", "monitoring-suite-api", "delete-dashboard-project", dashboardID)
	monitoringSuiteWaitGone(t, h, "ダッシュボードプロジェクト "+dashboardID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-dashboard-project", dashboardID)

	// --- アラートプロジェクト ---
	project := callMonitoringSuite(t, h, "アラートプロジェクト "+projectName+" を作成する",
		"monitoring-suite-api", "create-alert-project", "--name", projectName, "--description", "skr e2e")
	projectID := monitoringSuiteIDText(t, project, "id")
	require.Equal(t, projectName, project["name"], "create-alert-project output: %v", project)

	// テスト全体を通して使うため、削除は defer で最後に実行する
	defer func() {
		h.Call(t, "後片付け: アラートプロジェクト "+projectID+" を削除する", "monitoring-suite-api", "delete-alert-project", projectID)
		monitoringSuiteWaitGone(t, h, "後片付け: アラートプロジェクト "+projectID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project", projectID)
	}()

	projectRead := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" を参照する",
		"monitoring-suite-api", "read-alert-project", projectID)
	require.Equal(t, projectID, monitoringSuiteIDText(t, projectRead, "id"))

	projectPatched := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" の説明を更新する",
		"monitoring-suite-api", "patch-alert-project", projectID, "--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", projectPatched["description"], "patch-alert-project output: %v", projectPatched)

	projectList := h.Call(t, "アラートプロジェクトの一覧を取得する", "monitoring-suite-api", "list-alert-projects")
	var projects []struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	require.NoError(t, json.Unmarshal(projectList, &projects), "list-alert-projects output: %s", projectList)
	require.Contains(t, projectNames(projects), projectName, "list-alert-projects should contain the created project")
	require.Contains(t, projectIDs(t, projects), projectID, "list-alert-projects should contain the created project ID")

	// --- メトリクスストレージ ---
	metricStorage := callMonitoringSuite(t, h, "メトリクスストレージ "+metricStorageName+" を作成する",
		"monitoring-suite-api", "create-metric-storage", "--name", metricStorageName, "--description", "skr e2e")
	metricStorageID := monitoringSuiteIDText(t, metricStorage, "id")
	require.Equal(t, metricStorageName, metricStorage["name"], "create-metric-storage output: %v", metricStorage)

	metricStorageRead := pollMonitoringSuiteRead(t, h, "メトリクスストレージ "+metricStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-metric-storage", metricStorageID)
	require.Equal(t, metricStorageID, monitoringSuiteIDText(t, metricStorageRead, "id"))

	defer func() {
		h.Call(t, "後片付け: メトリクスストレージ "+metricStorageID+" を削除する", "monitoring-suite-api", "delete-metric-storage", metricStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: メトリクスストレージ "+metricStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-metric-storage", metricStorageID)
	}()

	metricStoragePatched := callMonitoringSuite(t, h, "メトリクスストレージ "+metricStorageID+" の説明を更新する",
		"monitoring-suite-api", "patch-metric-storage", metricStorageID, "--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", metricStoragePatched["description"], "patch-metric-storage output: %v", metricStoragePatched)

	// --- メトリクスストレージのアクセスキー ---
	metricKey := callMonitoringSuite(t, h, "メトリクスストレージ "+metricStorageID+" のアクセスキーを作成する",
		"monitoring-suite-api", "create-metric-storage-key", metricStorageID, "--description", "skr e2e key")
	metricKeyUID := monitoringSuiteIDText(t, metricKey, "uid")

	metricKeyPatched := callMonitoringSuite(t, h, "アクセスキー "+metricKeyUID+" の説明を更新する",
		"monitoring-suite-api", "patch-metric-storage-key", metricStorageID, metricKeyUID, "--description", "skr e2e key patched")
	require.Equal(t, "skr e2e key patched", metricKeyPatched["description"], "patch-metric-storage-key output: %v", metricKeyPatched)

	metricKeyRead := callMonitoringSuite(t, h, "アクセスキー "+metricKeyUID+" を参照する",
		"monitoring-suite-api", "read-metric-storage-key", metricStorageID, metricKeyUID)
	require.Equal(t, metricKeyUID, monitoringSuiteIDText(t, metricKeyRead, "uid"))

	metricKeys := h.Call(t, "メトリクスストレージのアクセスキー一覧を取得する", "monitoring-suite-api", "list-metric-storage-keys", metricStorageID)
	var metricKeyList []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(metricKeys, &metricKeyList), "list-metric-storage-keys output: %s", metricKeys)
	require.Contains(t, uids(metricKeyList), metricKeyUID, "list-metric-storage-keys should contain the created key")

	h.Call(t, "アクセスキー "+metricKeyUID+" を削除する", "monitoring-suite-api", "delete-metric-storage-key", metricStorageID, metricKeyUID)
	monitoringSuiteWaitGone(t, h, "アクセスキー "+metricKeyUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-metric-storage-key", metricStorageID, metricKeyUID)

	// --- アラートルール ---
	// Query は PromQL 形式、しきい値は「演算子+値」の文字列形式
	// (参考: https://manual.sakura.ad.jp/cloud/appliance/monitoring-suite/tutorial.html 手順7)
	ruleQuery := `100 * (1 - avg(system_cpu_utilization_ratio{state="idle"}))`
	rule := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" にアラートルールを作成する",
		"monitoring-suite-api", "create-alert-project-rule", projectID,
		"--metrics-storage-id", metricStorageID,
		"--alert-query", ruleQuery,
		"--name", "skr-e2e-rule-"+suffix,
		"--threshold-warning", ">=10",
		"--threshold-critical", ">=20",
		"--enabled-warning",
		"--enabled-critical",
	)
	ruleUID := monitoringSuiteIDText(t, rule, "uid")
	require.Equal(t, "skr-e2e-rule-"+suffix, rule["name"], "create-alert-project-rule output: %v", rule)

	ruleRead := callMonitoringSuite(t, h, "アラートルール "+ruleUID+" を参照する",
		"monitoring-suite-api", "read-alert-project-rule", projectID, ruleUID)
	require.Equal(t, ruleUID, monitoringSuiteIDText(t, ruleRead, "uid"))

	rulePatched := callMonitoringSuite(t, h, "アラートルール "+ruleUID+" の critical しきい値を更新する",
		"monitoring-suite-api", "patch-alert-project-rule", projectID, ruleUID, "--threshold-critical", ">=30")
	require.Equal(t, ">=30", rulePatched["threshold_critical"], "patch-alert-project-rule output: %v", rulePatched)

	ruleList := h.Call(t, "アラートプロジェクトのルール一覧を取得する", "monitoring-suite-api", "list-alert-project-rules", projectID)
	var rules []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(ruleList, &rules), "list-alert-project-rules output: %s", ruleList)
	require.Contains(t, uids(rules), ruleUID, "list-alert-project-rules should contain the created rule")

	ruleHistories := h.Call(t, "アラートルール "+ruleUID+" の履歴を一覧表示する",
		"monitoring-suite-api", "list-alert-project-rule-histories", projectID, ruleUID)
	var ruleHistoryList []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(ruleHistories, &ruleHistoryList), "list-alert-project-rule-histories output: %s", ruleHistories)
	t.Logf("アラートルールの履歴: %d 件", len(ruleHistoryList))

	h.Call(t, "アラートルール "+ruleUID+" を削除する", "monitoring-suite-api", "delete-alert-project-rule", projectID, ruleUID)
	monitoringSuiteWaitGone(t, h, "アラートルール "+ruleUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-alert-project-rule", projectID, ruleUID)

	// --- 通知先 ---
	target := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" に通知先を作成する",
		"monitoring-suite-api", "create-alert-project-notification-target", projectID,
		"--service-type", "SAKURA_SIMPLE_NOTICE",
		"--url", "https://example.test/hook",
		"--description", "skr e2e target",
	)
	targetUID := monitoringSuiteIDText(t, target, "uid")
	require.Equal(t, "SAKURA_SIMPLE_NOTICE", target["service_type"], "create-alert-project-notification-target output: %v", target)

	targetPatched := callMonitoringSuite(t, h, "通知先 "+targetUID+" の URL を更新する",
		"monitoring-suite-api", "patch-alert-project-notification-target", projectID, targetUID,
		"--request", `{"URL":"https://example.test/hook2"}`)
	require.Equal(t, "https://example.test/hook2", targetPatched["url"], "patch-alert-project-notification-target output: %v", targetPatched)

	h.Call(t, "通知先 "+targetUID+" を削除する", "monitoring-suite-api", "delete-alert-project-notification-target", projectID, targetUID)
	monitoringSuiteWaitGone(t, h, "通知先 "+targetUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-alert-project-notification-target", projectID, targetUID)

	// --- ログストレージ ---
	logStorage := callMonitoringSuite(t, h, "ログストレージ "+logStorageName+" を作成する",
		"monitoring-suite-api", "create-log-storage", "--name", logStorageName, "--description", "skr e2e", "--classification", "shared")
	logStorageID := monitoringSuiteIDText(t, logStorage, "id")
	require.Equal(t, logStorageName, logStorage["name"], "create-log-storage output: %v", logStorage)

	logStorageRead := pollMonitoringSuiteRead(t, h, "ログストレージ "+logStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-log-storage", logStorageID)
	require.Equal(t, logStorageID, monitoringSuiteIDText(t, logStorageRead, "id"))

	defer func() {
		h.Call(t, "後片付け: ログストレージ "+logStorageID+" を削除する", "monitoring-suite-api", "delete-log-storage", logStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: ログストレージ "+logStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-log-storage", logStorageID)
	}()

	logStoragePatched := callMonitoringSuite(t, h, "ログストレージ "+logStorageID+" の説明を更新する",
		"monitoring-suite-api", "patch-log-storage", logStorageID, "--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", logStoragePatched["description"], "patch-log-storage output: %v", logStoragePatched)

	logStorageExpired := callMonitoringSuite(t, h, "ログストレージ "+logStorageID+" の保存期間を 90 日に変更する",
		"monitoring-suite-api", "set-log-storage-expire", logStorageID, "--days", "90")
	expireDay, ok := logStorageExpired["expire_day"].(float64)
	require.True(t, ok, "set-log-storage-expire should return expire_day: %v", logStorageExpired)
	require.Equal(t, float64(90), expireDay, "set-log-storage-expire should apply the new retention period")

	logStorages := h.Call(t, "ログストレージの一覧を名前で絞り込んで取得する",
		"monitoring-suite-api", "list-log-storages",
		"--query-raw", `[.[] | select(.name == "`+logStorageName+`")]`)
	var logStorageList []struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	require.NoError(t, json.Unmarshal(logStorages, &logStorageList), "list-log-storages output: %s", logStorages)
	require.Len(t, logStorageList, 1, "list-log-storages filtered by name should match exactly the created storage")
	require.Equal(t, logStorageID, monitoringSuiteIDFloatText(t, logStorageList[0].ID), "list-log-storages should return the created storage ID")

	// --- ログストレージのアクセスキー ---
	logKey := callMonitoringSuite(t, h, "ログストレージ "+logStorageID+" のアクセスキーを作成する",
		"monitoring-suite-api", "create-log-storage-key", logStorageID, "--description", "skr e2e key")
	logKeyUID := monitoringSuiteIDText(t, logKey, "uid")

	logKeyPatched := callMonitoringSuite(t, h, "アクセスキー "+logKeyUID+" の説明を更新する",
		"monitoring-suite-api", "patch-log-storage-key", logStorageID, logKeyUID, "--description", "skr e2e key patched")
	require.Equal(t, "skr e2e key patched", logKeyPatched["description"], "patch-log-storage-key output: %v", logKeyPatched)

	logKeyRead := callMonitoringSuite(t, h, "アクセスキー "+logKeyUID+" を参照する",
		"monitoring-suite-api", "read-log-storage-key", logStorageID, logKeyUID)
	require.Equal(t, logKeyUID, monitoringSuiteIDText(t, logKeyRead, "uid"))

	logKeys := h.Call(t, "ログストレージのアクセスキー一覧を取得する", "monitoring-suite-api", "list-log-storage-keys", logStorageID)
	var logKeyList []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(logKeys, &logKeyList), "list-log-storage-keys output: %s", logKeys)
	require.Contains(t, uids(logKeyList), logKeyUID, "list-log-storage-keys should contain the created key")

	h.Call(t, "アクセスキー "+logKeyUID+" を削除する", "monitoring-suite-api", "delete-log-storage-key", logStorageID, logKeyUID)
	monitoringSuiteWaitGone(t, h, "アクセスキー "+logKeyUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-log-storage-key", logStorageID, logKeyUID)

	// --- 通知ルーティング (通知先を再作成して使う) ---
	target2 := callMonitoringSuite(t, h, "通知ルーティングの検証用に通知先を作成する",
		"monitoring-suite-api", "create-alert-project-notification-target", projectID,
		"--service-type", "SAKURA_SIMPLE_NOTICE",
		"--url", "https://example.test/routing-hook",
	)
	target2UID := monitoringSuiteIDText(t, target2, "uid")
	defer func() {
		h.Call(t, "後片付け: 通知先 "+target2UID+" を削除する", "monitoring-suite-api", "delete-alert-project-notification-target", projectID, target2UID)
		monitoringSuiteWaitGone(t, h, "後片付け: 通知先 "+target2UID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project-notification-target", projectID, target2UID)
	}()

	routing := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" に通知ルーティングを作成する",
		"monitoring-suite-api", "create-alert-project-notification-routing", projectID,
		"--request", `{"NotificationTargetUID":"`+target2UID+`","MatchLabels":[{"name":"severity","value":"critical"}]}`,
	)
	routingUID := monitoringSuiteIDText(t, routing, "uid")

	routingRead := callMonitoringSuite(t, h, "通知ルーティング "+routingUID+" を参照する",
		"monitoring-suite-api", "read-alert-project-notification-routing", projectID, routingUID)
	require.Equal(t, routingUID, monitoringSuiteIDText(t, routingRead, "uid"))

	h.Call(t, "通知ルーティングの評価順序を並べ替える",
		"monitoring-suite-api", "reorder-alert-project-notification-routings", projectID,
		"--request", `[{"notification_routing_uid":"`+routingUID+`","order":3}]`)

	routingList := h.Call(t, "通知ルーティングの一覧を取得する", "monitoring-suite-api", "list-alert-project-notification-routings", projectID)
	var routings []struct {
		UID   string  `json:"uid"`
		Order float64 `json:"order"`
	}
	require.NoError(t, json.Unmarshal(routingList, &routings), "list-alert-project-notification-routings output: %s", routingList)
	require.Len(t, routings, 1, "list-alert-project-notification-routings should return the created routing")
	require.Equal(t, float64(3), routings[0].Order, "reorder should apply the new order: %v", routings)

	h.Call(t, "通知ルーティング "+routingUID+" を削除する",
		"monitoring-suite-api", "delete-alert-project-notification-routing", projectID, routingUID)
	monitoringSuiteWaitGone(t, h, "通知ルーティング "+routingUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-alert-project-notification-routing", projectID, routingUID)

	// --- ログ計測ルール ---
	measureRule := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" にログ計測ルールを作成する",
		"monitoring-suite-api", "create-alert-project-log-measure-rule", projectID,
		"--request", `{"LogStorageID":"`+logStorageID+`","MetricsStorageID":"`+metricStorageID+`","Name":"skr-e2e-measure-`+suffix+`",`+
			`"Rule":{"version":"v1","query":{"matchers":[{"type":"string","operator":"eq","field":"log_name","value":"skr-e2e","value_list":[]}]}}}`,
	)
	measureRuleUID := monitoringSuiteIDText(t, measureRule, "uid")

	measureRuleRead := callMonitoringSuite(t, h, "ログ計測ルール "+measureRuleUID+" を参照する",
		"monitoring-suite-api", "read-alert-project-log-measure-rule", projectID, measureRuleUID)
	require.Equal(t, measureRuleUID, monitoringSuiteIDText(t, measureRuleRead, "uid"))

	measureRulePatched := callMonitoringSuite(t, h, "ログ計測ルール "+measureRuleUID+" の説明を更新する",
		"monitoring-suite-api", "patch-alert-project-log-measure-rule", projectID, measureRuleUID,
		"--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", measureRulePatched["description"], "patch-alert-project-log-measure-rule output: %v", measureRulePatched)

	measureRules := h.Call(t, "ログ計測ルールの一覧を取得する", "monitoring-suite-api", "list-alert-project-log-measure-rules", projectID)
	var measureRuleList []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(measureRules, &measureRuleList), "list-alert-project-log-measure-rules output: %s", measureRules)
	require.Contains(t, uids(measureRuleList), measureRuleUID, "list-alert-project-log-measure-rules should contain the created rule")

	h.Call(t, "ログ計測ルール "+measureRuleUID+" を削除する",
		"monitoring-suite-api", "delete-alert-project-log-measure-rule", projectID, measureRuleUID)
	monitoringSuiteWaitGone(t, h, "ログ計測ルール "+measureRuleUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-alert-project-log-measure-rule", projectID, measureRuleUID)

	// --- ログルーティングとメトリクスルーティング ---
	logRouting := callMonitoringSuite(t, h, "ログルーティングを作成する (publisher="+publisherCode+" log_storage="+logStorageID+")",
		"monitoring-suite-api", "create-log-routing",
		"--publisher-code", publisherCode,
		"--variant", "logs",
		"--log-storage-id", logStorageID,
	)
	logRoutingUID := monitoringSuiteIDText(t, logRouting, "uid")

	logRoutingRead := callMonitoringSuite(t, h, "ログルーティング "+logRoutingUID+" を参照する",
		"monitoring-suite-api", "read-log-routing", logRoutingUID)
	require.Equal(t, logRoutingUID, monitoringSuiteIDText(t, logRoutingRead, "uid"))

	h.Call(t, "後片付け: ログルーティング "+logRoutingUID+" を削除する", "monitoring-suite-api", "delete-log-routing", logRoutingUID)
	monitoringSuiteWaitGone(t, h, "後片付け: ログルーティング "+logRoutingUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-log-routing", logRoutingUID)

	metricRouting := callMonitoringSuite(t, h, "メトリクスルーティングを作成する (publisher="+publisherCode+" metrics_storage="+metricStorageID+")",
		"monitoring-suite-api", "create-metric-routing",
		"--publisher-code", publisherCode,
		"--variant", "metrics",
		"--metrics-storage-id", metricStorageID,
	)
	metricRoutingUID := monitoringSuiteIDText(t, metricRouting, "uid")

	metricRoutingRead := callMonitoringSuite(t, h, "メトリクスルーティング "+metricRoutingUID+" を参照する",
		"monitoring-suite-api", "read-metric-routing", metricRoutingUID)
	require.Equal(t, metricRoutingUID, monitoringSuiteIDText(t, metricRoutingRead, "uid"))

	h.Call(t, "後片付け: メトリクスルーティング "+metricRoutingUID+" を削除する", "monitoring-suite-api", "delete-metric-routing", metricRoutingUID)
	monitoringSuiteWaitGone(t, h, "後片付け: メトリクスルーティング "+metricRoutingUID+" の削除後の不在を確認する", 2*time.Minute,
		"monitoring-suite-api", "read-metric-routing", metricRoutingUID)

	// --- トレースストレージ ---
	traceStorage := callMonitoringSuite(t, h, "トレースストレージ "+traceStorageName+" を作成する",
		"monitoring-suite-api", "create-trace-storage", "--name", traceStorageName, "--description", "skr e2e", "--classification", "shared")
	traceStorageID := monitoringSuiteIDText(t, traceStorage, "id")
	require.Equal(t, traceStorageName, traceStorage["name"], "create-trace-storage output: %v", traceStorage)

	traceStorageRead := pollMonitoringSuiteRead(t, h, "トレースストレージ "+traceStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-trace-storage", traceStorageID)
	require.Equal(t, traceStorageID, monitoringSuiteIDText(t, traceStorageRead, "id"))

	defer func() {
		h.Call(t, "後片付け: トレースストレージ "+traceStorageID+" を削除する", "monitoring-suite-api", "delete-trace-storage", traceStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: トレースストレージ "+traceStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-trace-storage", traceStorageID)
	}()

	traceStorageExpired := callMonitoringSuite(t, h, "トレースストレージ "+traceStorageID+" の保存期間を 60 日に変更する",
		"monitoring-suite-api", "set-trace-storage-expire", traceStorageID, "--days", "60")
	retention, ok := traceStorageExpired["retention_period_days"].(float64)
	require.True(t, ok, "set-trace-storage-expire should return retention_period_days: %v", traceStorageExpired)
	require.Equal(t, float64(60), retention, "set-trace-storage-expire should apply the new retention period")

	traceStorages := h.Call(t, "トレースストレージの一覧を名前で絞り込んで取得する",
		"monitoring-suite-api", "list-trace-storages",
		"--query-raw", `[.[] | select(.name == "`+traceStorageName+`")]`)
	var traceStorageList []struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	require.NoError(t, json.Unmarshal(traceStorages, &traceStorageList), "list-trace-storages output: %s", traceStorages)
	require.Len(t, traceStorageList, 1, "list-trace-storages filtered by name should match exactly the created storage")
	require.Equal(t, traceStorageID, monitoringSuiteIDFloatText(t, traceStorageList[0].ID), "list-trace-storages should return the created storage ID")

	// --- アラートプロジェクトの履歴 ---
	projectHistories := h.Call(t, "アラートプロジェクトの履歴を一覧表示する",
		"monitoring-suite-api", "list-alert-project-histories", projectID, "--severity", "critical", "--open")
	var historyList []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(projectHistories, &historyList), "list-alert-project-histories output: %s", projectHistories)
	t.Logf("アラートプロジェクトの履歴: %d 件", len(historyList))
}

// dashboardNames はダッシュボード一覧から名前の集合を作ります。
func dashboardNames(items []struct {
	ID   float64 `json:"id"`
	Name string  `json:"name"`
}) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

// projectNames はアラートプロジェクト一覧から名前の集合を作ります。
func projectNames(items []struct {
	ID   float64 `json:"id"`
	Name string  `json:"name"`
}) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

// monitoringSuiteIDFloatText は数値 ID を文字列に変換します。
func monitoringSuiteIDFloatText(t *testing.T, id float64) string {
	t.Helper()
	return strconv.FormatInt(int64(id), 10)
}

// uids は UID 集合を作ります。
func uids(items []struct {
	UID string `json:"uid"`
}) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.UID)
	}
	return out
}

// dashboardIDs はダッシュボード一覧から ID の集合を作ります。
func dashboardIDs(t *testing.T, items []struct {
	ID   float64 `json:"id"`
	Name string  `json:"name"`
}) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, monitoringSuiteIDFloatText(t, item.ID))
	}
	return ids
}

// projectIDs はアラートプロジェクト一覧から ID の集合を作ります。
func projectIDs(t *testing.T, items []struct {
	ID   float64 `json:"id"`
	Name string  `json:"name"`
}) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, monitoringSuiteIDFloatText(t, item.ID))
	}
	return ids
}
