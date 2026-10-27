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
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// updatedMaxCPU は今回の実行が application の更新で設定する CPU です。
	updatedMaxCPU = "1"
	// updatedMaxMemory は今回の実行が application の更新で設定するメモリです。
	updatedMaxMemory = "2Gi"
	// allTrafficPercent は新しいバージョンへ振り分けるトラフィックの割合です。
	allTrafficPercent = 100
	// versionWaitLimit はバージョンが Healthy になるまでの確認期限です。
	versionWaitLimit = 3 * time.Minute
	// versionPolling はバージョンステータスの再試行間隔です。
	versionPolling = 10 * time.Second
)

// versionSummary はバージョン一覧の照合に使う項目です。
type versionSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// versionDetail は CLI の JSON 出力をバージョン詳細として解釈するための項目です。
type versionDetail struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Port       int            `json:"port"`
	MinScale   int            `json:"min_scale"`
	MaxScale   int            `json:"max_scale"`
	Components []appComponent `json:"components"`
}

// trafficEntry は CLI の JSON 出力をトラフィック分散設定として解釈するための項目です。
type trafficEntry struct {
	VersionName     string `json:"version_name"`
	IsLatestVersion bool   `json:"is_latest_version"`
	Percent         int    `json:"percent"`
}

// runVersionScenario は application の更新と、version・traffic の各操作を実行します。
// application の更新でコンポーネントのメモリが変更され、新しいバージョンが作成されます。
// シナリオは新しいバージョンが Healthy になるのを待ち、すべてのトラフィックを
// そちらに振り分けてから、前のバージョンを削除します。
func runVersionScenario(t *testing.T, h *helper.Helper, appID, appName string) {
	t.Helper()

	// アプリケーションを更新して新しいバージョンを作成する
	updateRequest := h.WriteRequestJSONFile(t, "update", map[string]any{
		"all_traffic_available": true,
		"components": []any{
			map[string]any{
				"name":       "web",
				"max_cpu":    updatedMaxCPU,
				"max_memory": updatedMaxMemory,
				"deploy_source": map[string]any{
					"container_registry": map[string]any{"image": image},
				},
			},
		},
	})
	updatedApp := parseApp(t, h.Call(t, "アプリケーションを更新する",
		"apprun-api", "application", "update", appID, "--request", "@"+updateRequest))
	require.Equal(t, appID, updatedApp.ID, "updated application ID should match the application created by this run")
	require.Equal(t, appName, updatedApp.Name, "updated application name should match the name requested by this run")
	require.Len(t, updatedApp.Components, 1, "updated application should have one component")
	require.Equal(t, updatedMaxCPU, updatedApp.Components[0].MaxCPU, "updated application max_cpu should match the value requested by this run")
	require.Equal(t, updatedMaxMemory, updatedApp.Components[0].MaxMemory, "updated application max_memory should match the value requested by this run")

	// 更新後のアプリケーションを参照する
	readApp := parseApp(t, h.Call(t, "更新後のアプリケーションを参照する",
		"apprun-api", "application", "read", appID))
	require.Equal(t, updatedMaxCPU, readApp.Components[0].MaxCPU, "read application max_cpu should match the value requested by this run")
	require.Equal(t, updatedMaxMemory, readApp.Components[0].MaxMemory, "read application max_memory should match the value requested by this run")

	// 更新でバージョンが2件になることを確認する
	versions := listVersions(t, h, "バージョン一覧を取得する", appID)
	require.Len(t, versions, 2, "version list should contain the initial and updated versions after the update")
	latest, previous := newestAndPrevious(t, versions)

	// 新しいバージョンが Healthy になるまで待つ
	waitVersionHealthy(t, h, appID, latest.ID)

	// バージョン詳細を参照する
	detail := parseVersionDetail(t, h.Call(t, "バージョンを参照する",
		"apprun-api", "version", "read", "--application-id", appID, "--version-id", latest.ID), latest.ID)
	require.Equal(t, port, detail.Port, "version port should match the port requested by this run")
	require.Equal(t, 0, detail.MinScale, "version min_scale should match the value requested by this run")
	require.Equal(t, 1, detail.MaxScale, "version max_scale should match the value requested by this run")
	require.Len(t, detail.Components, 1, "version should have one component")
	require.Equal(t, updatedMaxCPU, detail.Components[0].MaxCPU, "version max_cpu should match the value requested by this run")
	require.Equal(t, updatedMaxMemory, detail.Components[0].MaxMemory, "version max_memory should match the value requested by this run")
	require.Equal(t, image, detail.Components[0].DeploySource.ContainerRegistry.Image, "version deploy source image should match the image requested by this run")

	// トラフィック分散設定を参照する
	listTraffic(t, h, "トラフィック分散設定を参照する", appID)

	// すべてのトラフィックを新しいバージョンへ振り分ける
	trafficRequest := h.WriteRequestJSONFile(t, "traffic-update", []any{
		map[string]any{"is_latest_version": true, "percent": allTrafficPercent},
	})
	_ = h.Call(t, "トラフィックを新しいバージョンへ振り分ける",
		"apprun-api", "traffic", "update", appID, "--request", "@"+trafficRequest)
	entries := listTraffic(t, h, "更新後のトラフィック分散設定を参照する", appID)
	total := 0
	latestPercent := 0
	for _, entry := range entries {
		total += entry.Percent
		if entry.IsLatestVersion {
			latestPercent += entry.Percent
		}
	}
	require.Equal(t, allTrafficPercent, total, "traffic percentages should sum to %d after the update", allTrafficPercent)
	require.Equal(t, allTrafficPercent, latestPercent, "the latest version should receive %d%% of the traffic after the update", allTrafficPercent)

	// 前のバージョンを削除する
	_ = h.Call(t, "前のバージョンを削除する",
		"apprun-api", "version", "delete", "--application-id", appID, "--version-id", previous.ID)

	// 削除後に残りのバージョンを確認する
	remaining := listVersions(t, h, "削除後にバージョン一覧を取得する", appID)
	require.Len(t, remaining, 1, "version list should contain only the latest version after deletion")
	require.Equal(t, latest.ID, remaining[0].ID, "the remaining version should be the updated version")
}

// listVersions はバージョン一覧を取得します。
func listVersions(t *testing.T, h *helper.Helper, description, appID string) []versionSummary {
	t.Helper()

	data := h.Call(t, description,
		"apprun-api", "version", "list", "--application-id", appID, "--query", ".data")
	var versions []versionSummary
	err := json.Unmarshal(data, &versions)
	require.NoError(t, err, "%s: decode AppRun version list: %v", description, err)
	require.NotNil(t, versions, "%s: expected a JSON version array", description)
	return versions
}

// listTraffic はトラフィック分散設定を取得します。
func listTraffic(t *testing.T, h *helper.Helper, description, appID string) []trafficEntry {
	t.Helper()

	data := h.Call(t, description,
		"apprun-api", "traffic", "list", appID, "--query", ".data")
	var entries []trafficEntry
	err := json.Unmarshal(data, &entries)
	require.NoError(t, err, "%s: decode AppRun traffic list: %v", description, err)
	return entries
}

// parseVersionDetail は CLI の JSON 出力をバージョン詳細として解釈します。
func parseVersionDetail(t *testing.T, data []byte, versionID string) versionDetail {
	t.Helper()

	var value versionDetail
	err := json.Unmarshal(data, &value)
	require.NoError(t, err, "decode AppRun version %s: %v", versionID, err)
	require.Equal(t, versionID, value.ID, "version ID should match the requested version")
	return value
}

// waitVersionHealthy はバージョンのステータスが Healthy になるまで期限付きで待ちます。
// UnHealthy や想定外のステータスは失敗として扱います。
func waitVersionHealthy(t *testing.T, h *helper.Helper, appID, versionID string) {
	t.Helper()

	deadline := time.Now().Add(versionWaitLimit)
	for {
		data := h.CallWithTimeout(t, 60*time.Second, "バージョンのステータスを取得する",
			"apprun-api", "version", "status", "--application-id", appID, "--version-id", versionID)
		var status struct {
			Status string `json:"status"`
		}
		err := json.Unmarshal(data, &status)
		require.NoError(t, err, "decode version status: %v", err)
		switch status.Status {
		case "Healthy":
			return
		case "Deploying":
		case "UnHealthy":
			t.Fatalf("version %s status is UnHealthy", versionID)
		default:
			t.Fatalf("unexpected version status %q", status.Status)
		}
		if time.Now().Add(versionPolling).After(deadline) {
			t.Fatalf("timed out after %s waiting for the version to become Healthy", versionWaitLimit)
		}
		h.Sleep(t, "バージョンのステータス確認まで待つ", versionPolling)
	}
}

// newestAndPrevious は最も新しい created_at を持つバージョンと、もう一方のバージョンを返します。
// application の更新は初回作成後に最も新しいバージョンを作成するため、
// 最新のものが更新後のバージョンです。
func newestAndPrevious(t *testing.T, versions []versionSummary) (latest, previous versionSummary) {
	t.Helper()

	require.Len(t, versions, 2, "need two versions to identify the updated one")
	first := parseVersionCreatedAt(t, versions[0])
	second := parseVersionCreatedAt(t, versions[1])
	if second.After(first) {
		return versions[1], versions[0]
	}
	return versions[0], versions[1]
}

// parseVersionCreatedAt はバージョンの created_at を解釈します。
func parseVersionCreatedAt(t *testing.T, version versionSummary) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, version.CreatedAt)
	require.NoError(t, err, "parse created_at %q of version %s: %v", version.CreatedAt, version.ID, err)
	return parsed
}
