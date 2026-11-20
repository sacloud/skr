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
	"strings"
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

// callMonitoringSuiteSecret は stdout にキーの値などの秘密情報が含まれる
// monitoring-suite-api コマンドを実行して JSON 応答を返します。
// 応答本文は実行記録に残らず、テスト内での解析だけに使われます。
func callMonitoringSuiteSecret(t *testing.T, h *helper.Helper, description string, args ...string) monitoringSuiteJSON {
	t.Helper()
	data := h.CallHidingStdout(t, description, args...)
	var out monitoringSuiteJSON
	require.NoError(t, json.Unmarshal(data, &out), "output of %v is not a JSON object: %s", args, data)
	return out
}

// callMonitoringSuiteSecretList は stdout に秘密情報が含まれ得る一覧コマンドを実行して
// 配列応答を返します。応答本文は実行記録に残りません。
func callMonitoringSuiteSecretList(t *testing.T, h *helper.Helper, description string, args ...string) []map[string]any {
	t.Helper()
	data := h.CallHidingStdout(t, description, args...)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(data, &out), "output of %v is not a JSON array: %s", args, data)
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

// monitoringSuiteNotFound はコマンドの失敗が対象不在の API 404 かどうかを判定します。
func monitoringSuiteNotFound(stderr string) bool {
	return strings.Contains(stderr, "API Error 404")
}

// monitoringSuiteWaitGone はリソースの削除後の不在を期限付きで確認します。
// API の 404 以外の失敗 (認証失敗・通信障害など) は不在とみなさず、
// 期限までに 404 が確認できなければテストを失敗させます。
func monitoringSuiteWaitGone(t *testing.T, h *helper.Helper, description string, timeout time.Duration, args ...string) {
	t.Helper()
	monitoringSuiteWaitGoneImpl(t, h, description, timeout, false, args...)
}

// monitoringSuiteWaitGoneHidingStdout は stdout にキーの値などの秘密情報が含まれ得る
// リソースの削除後の不在を期限付きで確認します。読み取り応答の本文は
// 成功・失敗とも実行記録に残しません。
func monitoringSuiteWaitGoneHidingStdout(t *testing.T, h *helper.Helper, description string, timeout time.Duration, args ...string) {
	t.Helper()
	monitoringSuiteWaitGoneImpl(t, h, description, timeout, true, args...)
}

// monitoringSuiteWaitGoneImpl は不在確認の共通実装です。
// API の 404 以外の失敗は不在とみなさず、期限までに 404 が確認できなければテストを失敗させます。
func monitoringSuiteWaitGoneImpl(t *testing.T, h *helper.Helper, description string, timeout time.Duration, hideStdout bool, args ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var stderr string
		var err error
		if hideStdout {
			_, stderr, err = h.CallAllowingFailureHidingStdout(t, description, args...)
		} else {
			_, stderr, err = h.CallAllowingFailure(t, description, args...)
		}
		if err != nil {
			if monitoringSuiteNotFound(stderr) {
				return
			}
			if time.Now().After(deadline) {
				require.Failf(t, "timed out waiting for resource deletion", "read %v did not report a 404: %s", args, stderr)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		if time.Now().After(deadline) {
			require.Failf(t, "timed out waiting for resource deletion", "read %v still succeeded", args)
		}
		time.Sleep(3 * time.Second)
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
		Code     string `json:"code"`
		Variants []struct {
			Storage string `json:"storage"`
		} `json:"variants"`
	}
	require.NoError(t, json.Unmarshal(publishers, &publisherList), "list-publishers output: %s", publishers)
	require.NotEmpty(t, publisherList, "list-publishers should return at least one publisher")
	// ルーティング作成でログとメトリクスの両方の種別を使うため、
	// 両方の種別を持つパブリッシャーを選ぶ
	publisherCode := ""
	for _, p := range publisherList {
		hasLogs, hasMetrics := false, false
		for _, v := range p.Variants {
			switch v.Storage {
			case "logs":
				hasLogs = true
			case "metrics":
				hasMetrics = true
			}
		}
		if hasLogs && hasMetrics {
			publisherCode = p.Code
			break
		}
	}
	require.NotEmpty(t, publisherCode, "list-publishers should include a publisher with both logs and metrics variants")

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

	// テスト専用リソースのため、作成直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: ダッシュボードプロジェクト "+dashboardID+" を削除する", "monitoring-suite-api", "delete-dashboard-project", dashboardID)
		monitoringSuiteWaitGone(t, h, "後片付け: ダッシュボードプロジェクト "+dashboardID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-dashboard-project", dashboardID)
	}()

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

	// 課金リソースのため、ID 取得直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: メトリクスストレージ "+metricStorageID+" を削除する", "monitoring-suite-api", "delete-metric-storage", metricStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: メトリクスストレージ "+metricStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-metric-storage", metricStorageID)
	}()

	metricStorageRead := pollMonitoringSuiteRead(t, h, "メトリクスストレージ "+metricStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-metric-storage", metricStorageID)
	require.Equal(t, metricStorageID, monitoringSuiteIDText(t, metricStorageRead, "id"))

	metricStoragePatched := callMonitoringSuite(t, h, "メトリクスストレージ "+metricStorageID+" の説明を更新する",
		"monitoring-suite-api", "patch-metric-storage", metricStorageID, "--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", metricStoragePatched["description"], "patch-metric-storage output: %v", metricStoragePatched)

	// --- メトリクスストレージの一覧と利用量 ---
	metricStorageList := h.Call(t, "メトリクスストレージをリソース ID で絞り込んで取得する",
		"monitoring-suite-api", "list-metric-storages", "--resource-id", metricStorageID)
	var metricStorages []struct {
		ID float64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(metricStorageList, &metricStorages), "list-metric-storages output: %s", metricStorageList)
	require.Len(t, metricStorages, 1, "list-metric-storages filtered by resource ID should match exactly the created storage")
	require.Equal(t, metricStorageID, monitoringSuiteIDFloatText(t, metricStorages[0].ID), "list-metric-storages should return the created storage ID")

	// 利用量はデータがなくても日次・月次とも正常応答することを確認する。
	// 月次は年を必須で指定する。
	metricDailyStats := h.Call(t, "メトリクスストレージ "+metricStorageID+" の日次利用量を取得する",
		"monitoring-suite-api", "read-metric-storage-stats-daily", metricStorageID)
	t.Logf("メトリクスストレージの日次利用量: %s", strings.TrimSpace(string(metricDailyStats)))
	metricMonthlyStats := h.Call(t, "メトリクスストレージ "+metricStorageID+" の月次利用量を取得する",
		"monitoring-suite-api", "read-metric-storage-stats-monthly", metricStorageID, "--year", strconv.Itoa(time.Now().Year()))
	t.Logf("メトリクスストレージの月次利用量: %s", strings.TrimSpace(string(metricMonthlyStats)))

	// --- メトリクスストレージのアクセスキー ---
	// 作成応答にはキーの値が含まれるため、応答本文は実行記録に残さない
	metricKey := callMonitoringSuiteSecret(t, h, "メトリクスストレージ "+metricStorageID+" のアクセスキーを作成する",
		"monitoring-suite-api", "create-metric-storage-key", metricStorageID, "--description", "skr e2e key")
	metricKeyUID := monitoringSuiteIDText(t, metricKey, "uid")

	// キーを取得した直後に削除と不在確認を defer で登録する。
	// 読み取り応答にキーの値が含まれるため、不在確認も応答本文を記録に残さない
	defer func() {
		h.Call(t, "後片付け: アクセスキー "+metricKeyUID+" を削除する", "monitoring-suite-api", "delete-metric-storage-key", metricStorageID, metricKeyUID)
		monitoringSuiteWaitGoneHidingStdout(t, h, "後片付け: アクセスキー "+metricKeyUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-metric-storage-key", metricStorageID, metricKeyUID)
	}()

	metricKeyPatched := callMonitoringSuiteSecret(t, h, "アクセスキー "+metricKeyUID+" の説明を更新する",
		"monitoring-suite-api", "patch-metric-storage-key", metricStorageID, metricKeyUID, "--description", "skr e2e key patched")
	require.Equal(t, "skr e2e key patched", metricKeyPatched["description"], "patch-metric-storage-key output: %v", metricKeyPatched)

	metricKeyRead := callMonitoringSuiteSecret(t, h, "アクセスキー "+metricKeyUID+" を参照する",
		"monitoring-suite-api", "read-metric-storage-key", metricStorageID, metricKeyUID)
	require.Equal(t, metricKeyUID, monitoringSuiteIDText(t, metricKeyRead, "uid"))

	metricKeyList := callMonitoringSuiteSecretList(t, h, "メトリクスストレージのアクセスキー一覧を取得する",
		"monitoring-suite-api", "list-metric-storage-keys", metricStorageID)
	require.Contains(t, jsonUIDs(metricKeyList), metricKeyUID, "list-metric-storage-keys should contain the created key")

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

	// ルールを取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: アラートルール "+ruleUID+" を削除する", "monitoring-suite-api", "delete-alert-project-rule", projectID, ruleUID)
		monitoringSuiteWaitGone(t, h, "後片付け: アラートルール "+ruleUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project-rule", projectID, ruleUID)
	}()

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
	var ruleHistoryList []map[string]any
	require.NoError(t, json.Unmarshal(ruleHistories, &ruleHistoryList), "list-alert-project-rule-histories output: %s", ruleHistories)
	t.Logf("アラートルールの履歴: %d 件", len(ruleHistoryList))
	// 履歴はアラート発生後に作られるため、存在する場合にのみ参照を確認する。
	// 履歴がない場合の参照は未ライブ検証 (command-coverage baseline を参照)
	for _, history := range ruleHistoryList {
		historyUID, ok := history["uid"].(string)
		require.True(t, ok, "history uid should be a string: %v", history)
		ruleHistoryRead := callMonitoringSuite(t, h, "アラートルールの履歴 "+historyUID+" を参照する",
			"monitoring-suite-api", "read-alert-project-rule-history", projectID, ruleUID, historyUID)
		require.Equal(t, historyUID, monitoringSuiteIDText(t, ruleHistoryRead, "uid"))
		break
	}

	// --- 通知先 ---
	target := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" に通知先を作成する",
		"monitoring-suite-api", "create-alert-project-notification-target", projectID,
		"--service-type", "SAKURA_SIMPLE_NOTICE",
		"--url", "https://example.test/hook",
		"--description", "skr e2e target",
	)
	targetUID := monitoringSuiteIDText(t, target, "uid")
	require.Equal(t, "SAKURA_SIMPLE_NOTICE", target["service_type"], "create-alert-project-notification-target output: %v", target)

	// 通知先を取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: 通知先 "+targetUID+" を削除する", "monitoring-suite-api", "delete-alert-project-notification-target", projectID, targetUID)
		monitoringSuiteWaitGone(t, h, "後片付け: 通知先 "+targetUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project-notification-target", projectID, targetUID)
	}()

	targetRead := callMonitoringSuite(t, h, "通知先 "+targetUID+" を参照する",
		"monitoring-suite-api", "read-alert-project-notification-target", projectID, targetUID)
	require.Equal(t, targetUID, monitoringSuiteIDText(t, targetRead, "uid"))
	require.Equal(t, "SAKURA_SIMPLE_NOTICE", targetRead["service_type"], "read-alert-project-notification-target output: %v", targetRead)

	targetPatched := callMonitoringSuite(t, h, "通知先 "+targetUID+" の URL を更新する",
		"monitoring-suite-api", "patch-alert-project-notification-target", projectID, targetUID,
		"--request", `{"URL":"https://example.test/hook2"}`)
	require.Equal(t, "https://example.test/hook2", targetPatched["url"], "patch-alert-project-notification-target output: %v", targetPatched)

	// --- ログストレージ ---
	logStorage := callMonitoringSuite(t, h, "ログストレージ "+logStorageName+" を作成する",
		"monitoring-suite-api", "create-log-storage", "--name", logStorageName, "--description", "skr e2e", "--classification", "shared")
	logStorageID := monitoringSuiteIDText(t, logStorage, "id")
	require.Equal(t, logStorageName, logStorage["name"], "create-log-storage output: %v", logStorage)

	// 課金リソースのため、ID 取得直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: ログストレージ "+logStorageID+" を削除する", "monitoring-suite-api", "delete-log-storage", logStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: ログストレージ "+logStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-log-storage", logStorageID)
	}()

	logStorageRead := pollMonitoringSuiteRead(t, h, "ログストレージ "+logStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-log-storage", logStorageID)
	require.Equal(t, logStorageID, monitoringSuiteIDText(t, logStorageRead, "id"))

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

	// 利用量はデータがなくても日次・月次とも正常応答することを確認する。月次は年を必須で指定する。
	logDailyStats := h.Call(t, "ログストレージ "+logStorageID+" の日次利用量を取得する",
		"monitoring-suite-api", "read-log-storage-stats-daily", logStorageID)
	t.Logf("ログストレージの日次利用量: %s", strings.TrimSpace(string(logDailyStats)))
	logMonthlyStats := h.Call(t, "ログストレージ "+logStorageID+" の月次利用量を取得する",
		"monitoring-suite-api", "read-log-storage-stats-monthly", logStorageID, "--year", strconv.Itoa(time.Now().Year()))
	t.Logf("ログストレージの月次利用量: %s", strings.TrimSpace(string(logMonthlyStats)))

	// --- ログストレージのアクセスキー ---
	// 作成応答にはキーの値が含まれるため、応答本文は実行記録に残さない
	logKey := callMonitoringSuiteSecret(t, h, "ログストレージ "+logStorageID+" のアクセスキーを作成する",
		"monitoring-suite-api", "create-log-storage-key", logStorageID, "--description", "skr e2e key")
	logKeyUID := monitoringSuiteIDText(t, logKey, "uid")

	// キーを取得した直後に削除と不在確認を defer で登録する。
	// 読み取り応答にキーの値が含まれるため、不在確認も応答本文を記録に残さない
	defer func() {
		h.Call(t, "後片付け: アクセスキー "+logKeyUID+" を削除する", "monitoring-suite-api", "delete-log-storage-key", logStorageID, logKeyUID)
		monitoringSuiteWaitGoneHidingStdout(t, h, "後片付け: アクセスキー "+logKeyUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-log-storage-key", logStorageID, logKeyUID)
	}()

	logKeyPatched := callMonitoringSuiteSecret(t, h, "アクセスキー "+logKeyUID+" の説明を更新する",
		"monitoring-suite-api", "patch-log-storage-key", logStorageID, logKeyUID, "--description", "skr e2e key patched")
	require.Equal(t, "skr e2e key patched", logKeyPatched["description"], "patch-log-storage-key output: %v", logKeyPatched)

	logKeyRead := callMonitoringSuiteSecret(t, h, "アクセスキー "+logKeyUID+" を参照する",
		"monitoring-suite-api", "read-log-storage-key", logStorageID, logKeyUID)
	require.Equal(t, logKeyUID, monitoringSuiteIDText(t, logKeyRead, "uid"))

	logKeyList := callMonitoringSuiteSecretList(t, h, "ログストレージのアクセスキー一覧を取得する",
		"monitoring-suite-api", "list-log-storage-keys", logStorageID)
	require.Contains(t, jsonUIDs(logKeyList), logKeyUID, "list-log-storage-keys should contain the created key")

	// --- 通知ルーティング (通知先は SAKURA_SIMPLE_NOTICE でプロジェクトに 1 つのため、作成済みの通知先を使う) ---
	routing := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" に通知ルーティングを作成する",
		"monitoring-suite-api", "create-alert-project-notification-routing", projectID,
		"--request", `{"NotificationTargetUID":"`+targetUID+`","MatchLabels":[{"name":"severity","value":"critical"}]}`,
	)
	routingUID := monitoringSuiteIDText(t, routing, "uid")

	// ルーティングを取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: 通知ルーティング "+routingUID+" を削除する",
			"monitoring-suite-api", "delete-alert-project-notification-routing", projectID, routingUID)
		monitoringSuiteWaitGone(t, h, "後片付け: 通知ルーティング "+routingUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project-notification-routing", projectID, routingUID)
	}()

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

	// 通知先 UID を同値で部分更新し、部分更新 API が正常応答することを確認する
	routingPatched := callMonitoringSuite(t, h, "通知ルーティング "+routingUID+" の通知先を同値で更新する",
		"monitoring-suite-api", "patch-alert-project-notification-routing", projectID, routingUID,
		"--request", `{"NotificationTargetUID":"`+targetUID+`","ResendIntervalMinutes":30}`)
	require.Equal(t, routingUID, monitoringSuiteIDText(t, routingPatched, "uid"), "patch-alert-project-notification-routing output: %v", routingPatched)

	// --- ログ計測ルール ---
	measureRule := callMonitoringSuite(t, h, "アラートプロジェクト "+projectID+" にログ計測ルールを作成する",
		"monitoring-suite-api", "create-alert-project-log-measure-rule", projectID,
		"--request", `{"LogStorageID":"`+logStorageID+`","MetricsStorageID":"`+metricStorageID+`","Name":"skr-e2e-measure-`+suffix+`",`+
			`"Rule":{"version":"v1","query":{"matchers":[{"type":"string","operator":"eq","field":"log_name","value":"skr-e2e","value_list":[]}]}}}`,
	)
	measureRuleUID := monitoringSuiteIDText(t, measureRule, "uid")

	// ルールを取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: ログ計測ルール "+measureRuleUID+" を削除する",
			"monitoring-suite-api", "delete-alert-project-log-measure-rule", projectID, measureRuleUID)
		monitoringSuiteWaitGone(t, h, "後片付け: ログ計測ルール "+measureRuleUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-alert-project-log-measure-rule", projectID, measureRuleUID)
	}()

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

	// ログ計測ルールの履歴はアラート発生後に作られるため、一覧が空でも正常応答を確認する
	measureRuleHistories := h.Call(t, "ログ計測ルールの履歴を一覧表示する",
		"monitoring-suite-api", "list-alert-project-log-measure-rule-histories", projectID)
	var measureRuleHistoryList []map[string]any
	require.NoError(t, json.Unmarshal(measureRuleHistories, &measureRuleHistoryList), "list-alert-project-log-measure-rule-histories output: %s", measureRuleHistories)
	t.Logf("ログ計測ルールの履歴: %d 件", len(measureRuleHistoryList))
	// 履歴が存在する場合のみ参照を確認する。履歴がない場合の参照は未ライブ検証 (command-coverage baseline を参照)
	for _, history := range measureRuleHistoryList {
		historyUID, ok := history["uid"].(string)
		require.True(t, ok, "history uid should be a string: %v", history)
		measureRuleHistoryRead := callMonitoringSuite(t, h, "ログ計測ルールの履歴 "+historyUID+" を参照する",
			"monitoring-suite-api", "read-alert-project-log-measure-rule-history", projectID, historyUID)
		require.Equal(t, historyUID, monitoringSuiteIDText(t, measureRuleHistoryRead, "uid"))
		break
	}

	// --- ログルーティングとメトリクスルーティング ---
	logRouting := callMonitoringSuite(t, h, "ログルーティングを作成する (publisher="+publisherCode+" log_storage="+logStorageID+")",
		"monitoring-suite-api", "create-log-routing",
		"--publisher-code", publisherCode,
		"--variant", "logs",
		"--log-storage-id", logStorageID,
	)
	logRoutingUID := monitoringSuiteIDText(t, logRouting, "uid")

	// ルーティングを取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: ログルーティング "+logRoutingUID+" を削除する", "monitoring-suite-api", "delete-log-routing", logRoutingUID)
		monitoringSuiteWaitGone(t, h, "後片付け: ログルーティング "+logRoutingUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-log-routing", logRoutingUID)
	}()

	logRoutingRead := callMonitoringSuite(t, h, "ログルーティング "+logRoutingUID+" を参照する",
		"monitoring-suite-api", "read-log-routing", logRoutingUID)
	require.Equal(t, logRoutingUID, monitoringSuiteIDText(t, logRoutingRead, "uid"))

	logRoutingList := h.Call(t, "ログルーティングの一覧を取得する", "monitoring-suite-api", "list-log-routings")
	var logRoutings []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(logRoutingList, &logRoutings), "list-log-routings output: %s", logRoutingList)
	require.Contains(t, uids(logRoutings), logRoutingUID, "list-log-routings should contain the created routing")

	logRoutingPatched := callMonitoringSuite(t, h, "ログルーティング "+logRoutingUID+" の転送先を同値で更新する",
		"monitoring-suite-api", "patch-log-routing", logRoutingUID,
		"--log-storage-id", logStorageID)
	require.Equal(t, logRoutingUID, monitoringSuiteIDText(t, logRoutingPatched, "uid"), "patch-log-routing output: %v", logRoutingPatched)

	metricRouting := callMonitoringSuite(t, h, "メトリクスルーティングを作成する (publisher="+publisherCode+" metrics_storage="+metricStorageID+")",
		"monitoring-suite-api", "create-metric-routing",
		"--publisher-code", publisherCode,
		"--variant", "metrics",
		"--metrics-storage-id", metricStorageID,
	)
	metricRoutingUID := monitoringSuiteIDText(t, metricRouting, "uid")

	// ルーティングを取得した直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: メトリクスルーティング "+metricRoutingUID+" を削除する", "monitoring-suite-api", "delete-metric-routing", metricRoutingUID)
		monitoringSuiteWaitGone(t, h, "後片付け: メトリクスルーティング "+metricRoutingUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-metric-routing", metricRoutingUID)
	}()

	metricRoutingRead := callMonitoringSuite(t, h, "メトリクスルーティング "+metricRoutingUID+" を参照する",
		"monitoring-suite-api", "read-metric-routing", metricRoutingUID)
	require.Equal(t, metricRoutingUID, monitoringSuiteIDText(t, metricRoutingRead, "uid"))

	metricRoutingList := h.Call(t, "メトリクスルーティングの一覧を取得する", "monitoring-suite-api", "list-metric-routings")
	var metricRoutings []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(metricRoutingList, &metricRoutings), "list-metric-routings output: %s", metricRoutingList)
	require.Contains(t, uids(metricRoutings), metricRoutingUID, "list-metric-routings should contain the created routing")

	metricRoutingPatched := callMonitoringSuite(t, h, "メトリクスルーティング "+metricRoutingUID+" の転送先を同値で更新する",
		"monitoring-suite-api", "patch-metric-routing", metricRoutingUID,
		"--metrics-storage-id", metricStorageID)
	require.Equal(t, metricRoutingUID, monitoringSuiteIDText(t, metricRoutingPatched, "uid"), "patch-metric-routing output: %v", metricRoutingPatched)

	// --- トレースストレージ ---
	traceStorage := callMonitoringSuite(t, h, "トレースストレージ "+traceStorageName+" を作成する",
		"monitoring-suite-api", "create-trace-storage", "--name", traceStorageName, "--description", "skr e2e", "--classification", "shared")
	traceStorageID := monitoringSuiteIDText(t, traceStorage, "id")
	require.Equal(t, traceStorageName, traceStorage["name"], "create-trace-storage output: %v", traceStorage)

	// 課金リソースのため、ID 取得直後に削除と不在確認を defer で登録する
	defer func() {
		h.Call(t, "後片付け: トレースストレージ "+traceStorageID+" を削除する", "monitoring-suite-api", "delete-trace-storage", traceStorageID)
		monitoringSuiteWaitGone(t, h, "後片付け: トレースストレージ "+traceStorageID+" の削除後の不在を確認する", 3*time.Minute,
			"monitoring-suite-api", "read-trace-storage", traceStorageID)
	}()

	traceStorageRead := pollMonitoringSuiteRead(t, h, "トレースストレージ "+traceStorageID+" の反映を確認する", 3*time.Minute,
		"monitoring-suite-api", "read-trace-storage", traceStorageID)
	require.Equal(t, traceStorageID, monitoringSuiteIDText(t, traceStorageRead, "id"))

	traceStorageExpired := callMonitoringSuite(t, h, "トレースストレージ "+traceStorageID+" の保存期間を 60 日に変更する",
		"monitoring-suite-api", "set-trace-storage-expire", traceStorageID, "--days", "60")
	retention, ok := traceStorageExpired["retention_period_days"].(float64)
	require.True(t, ok, "set-trace-storage-expire should return retention_period_days: %v", traceStorageExpired)
	require.Equal(t, float64(60), retention, "set-trace-storage-expire should apply the new retention period")

	traceStoragePatched := callMonitoringSuite(t, h, "トレースストレージ "+traceStorageID+" の説明を更新する",
		"monitoring-suite-api", "patch-trace-storage", traceStorageID, "--request", `{"Description":"skr e2e patched"}`)
	require.Equal(t, "skr e2e patched", traceStoragePatched["description"], "patch-trace-storage output: %v", traceStoragePatched)

	// 利用量はデータがなくても日次・月次とも正常応答することを確認する
	traceDailyStats := h.Call(t, "トレースストレージ "+traceStorageID+" の日次利用量を取得する",
		"monitoring-suite-api", "read-trace-storage-stats-daily", traceStorageID)
	t.Logf("トレースストレージの日次利用量: %s", strings.TrimSpace(string(traceDailyStats)))
	traceMonthlyStats := h.Call(t, "トレースストレージ "+traceStorageID+" の月次利用量を取得する",
		"monitoring-suite-api", "read-trace-storage-stats-monthly", traceStorageID, "--year", strconv.Itoa(time.Now().Year()))
	t.Logf("トレースストレージの月次利用量: %s", strings.TrimSpace(string(traceMonthlyStats)))

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

	// --- トレースストレージのアクセスキー ---
	// 作成応答にはキーの値が含まれるため、応答本文は実行記録に残さない
	traceKey := callMonitoringSuiteSecret(t, h, "トレースストレージ "+traceStorageID+" のアクセスキーを作成する",
		"monitoring-suite-api", "create-trace-storage-key", traceStorageID, "--description", "skr e2e key")
	traceKeyUID := monitoringSuiteIDText(t, traceKey, "uid")

	// キーを取得した直後に削除と不在確認を defer で登録する。
	// 読み取り応答にキーの値が含まれるため、不在確認も応答本文を記録に残さない
	defer func() {
		h.Call(t, "後片付け: トレースストレージのアクセキー "+traceKeyUID+" を削除する",
			"monitoring-suite-api", "delete-trace-storage-key", traceStorageID, traceKeyUID)
		monitoringSuiteWaitGoneHidingStdout(t, h, "後片付け: トレースストレージのアクセキー "+traceKeyUID+" の削除後の不在を確認する", 2*time.Minute,
			"monitoring-suite-api", "read-trace-storage-key", traceStorageID, traceKeyUID)
	}()

	traceKeyPatched := callMonitoringSuiteSecret(t, h, "トレースストレージのアクセスキー "+traceKeyUID+" の説明を更新する",
		"monitoring-suite-api", "patch-trace-storage-key", traceStorageID, traceKeyUID, "--description", "skr e2e key patched")
	require.Equal(t, "skr e2e key patched", traceKeyPatched["description"], "patch-trace-storage-key output: %v", traceKeyPatched)

	traceKeyRead := callMonitoringSuiteSecret(t, h, "トレースストレージのアクセスキー "+traceKeyUID+" を参照する",
		"monitoring-suite-api", "read-trace-storage-key", traceStorageID, traceKeyUID)
	require.Equal(t, traceKeyUID, monitoringSuiteIDText(t, traceKeyRead, "uid"))

	traceKeyList := callMonitoringSuiteSecretList(t, h, "トレースストレージのアクセキー一覧を取得する",
		"monitoring-suite-api", "list-trace-storage-keys", traceStorageID)
	require.Contains(t, jsonUIDs(traceKeyList), traceKeyUID, "list-trace-storage-keys should contain the created key")

	// --- 通知先一覧 ---
	targetList := h.Call(t, "アラートプロジェクトの通知先一覧を取得する",
		"monitoring-suite-api", "list-alert-project-notification-targets", projectID)
	var targets []struct {
		UID string `json:"uid"`
	}
	require.NoError(t, json.Unmarshal(targetList, &targets), "list-alert-project-notification-targets output: %s", targetList)
	require.Contains(t, uids(targets), targetUID, "list-alert-project-notification-targets should contain the routing target")

	// --- アラートプロジェクトの履歴 ---
	projectHistories := h.Call(t, "アラートプロジェクトの履歴を一覧表示する",
		"monitoring-suite-api", "list-alert-project-histories", projectID, "--severity", "critical", "--open")
	var historyList []map[string]any
	require.NoError(t, json.Unmarshal(projectHistories, &historyList), "list-alert-project-histories output: %s", projectHistories)
	t.Logf("アラートプロジェクトの履歴: %d 件", len(historyList))
	// 履歴はアラート発生後に作られるため、存在する場合にのみ参照を確認する。
	// 履歴がない場合の参照は未ライブ検証 (command-coverage baseline を参照)
	for _, history := range historyList {
		historyUID, ok := history["uid"].(string)
		require.True(t, ok, "history uid should be a string: %v", history)
		historyRead := callMonitoringSuite(t, h, "アラート履歴 "+historyUID+" を参照する",
			"monitoring-suite-api", "read-alert-project-history", projectID, historyUID)
		require.Equal(t, historyUID, monitoringSuiteIDText(t, historyRead, "uid"))
		break
	}
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

// jsonUIDs は map 形式の一覧応答から UID 集合を作ります。
func jsonUIDs(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if uid, ok := item["uid"].(string); ok {
			out = append(out, uid)
		}
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
