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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	updatedMaxCPU     = "1"
	updatedMaxMemory  = "2Gi"
	allTrafficPercent = 100
	versionWaitLimit  = 3 * time.Minute
	versionPolling    = 10 * time.Second
)

type versionSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type versionDetail struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Port       int    `json:"port"`
	MinScale   int    `json:"min_scale"`
	MaxScale   int    `json:"max_scale"`
	Components []struct {
		Name         string `json:"name"`
		MaxCPU       string `json:"max_cpu"`
		MaxMemory    string `json:"max_memory"`
		DeploySource struct {
			ContainerRegistry struct {
				Image string `json:"image"`
			} `json:"container_registry"`
		} `json:"deploy_source"`
	} `json:"components"`
	CreatedAt string `json:"created_at"`
}

type trafficEntry struct {
	VersionName     string `json:"version_name"`
	IsLatestVersion bool   `json:"is_latest_version"`
	Percent         int    `json:"percent"`
}

// versionScenario は application の更新、version の一覧/読み込み/ステータス/削除、
// および traffic の一覧/更新を実行します。application の更新でコンポーネントメモリが変更され、
// 新しい version が作成されます。シナリオは新しい version が Healthy になるのを待ち、
// すべてのトラフィックをそちらに振り分けてから、前の version を削除します。
func (s *scenario) versionScenario(ctx context.Context, appID, appName string) error {
	updatePath, err := s.writeUpdateRequest()
	if err != nil {
		return err
	}
	if _, err := s.call(ctx, "update-application", "application", "update", appID, "--request", "@"+updatePath); err != nil {
		return err
	}
	updated, err := s.read(ctx, "read-updated-application", appID)
	if err != nil {
		return err
	}
	if err := checkUpdatedApplication(updated, appID, appName); err != nil {
		return err
	}

	versions, err := s.listVersions(ctx, "list-versions", appID)
	if err != nil {
		return err
	}
	if len(versions) != 2 {
		return fmt.Errorf("version list returned %d versions after the update; want 2", len(versions))
	}
	latest, previous, err := newestAndPrevious(versions)
	if err != nil {
		return err
	}

	if err := s.waitVersionHealthy(ctx, appID, latest.ID); err != nil {
		return err
	}
	detail, err := s.readVersion(ctx, "read-version", appID, latest.ID)
	if err != nil {
		return err
	}
	if err := checkVersionDetail(detail, latest.ID); err != nil {
		return err
	}

	if _, err := s.call(ctx, "list-traffic", "traffic", "list", appID, "--query", ".data"); err != nil {
		return err
	}
	if _, err := s.call(ctx, "update-traffic", "traffic", "update", appID,
		"--request", fmt.Sprintf(`[{"is_latest_version":true,"percent":%d}]`, allTrafficPercent)); err != nil {
		return err
	}
	traffics, err := s.listTraffic(ctx, "list-traffic-after-update", appID)
	if err != nil {
		return err
	}
	total := 0
	latestPercent := 0
	for _, entry := range traffics {
		total += entry.Percent
		if entry.IsLatestVersion {
			latestPercent += entry.Percent
		}
	}
	if total != allTrafficPercent {
		return fmt.Errorf("traffic percentages sum to %d after the update; want %d", total, allTrafficPercent)
	}
	if latestPercent != allTrafficPercent {
		return fmt.Errorf("latest version receives %d%% of the traffic after the update; want %d%%", latestPercent, allTrafficPercent)
	}

	if _, err := s.call(ctx, "delete-version", "version", "delete", "--application-id", appID, "--version-id", previous.ID); err != nil {
		return err
	}
	remaining, err := s.listVersions(ctx, "list-versions-after-delete", appID)
	if err != nil {
		return err
	}
	if len(remaining) != 1 || remaining[0].ID != latest.ID {
		return fmt.Errorf("version list returned %d versions after deleting %s; want only %s", len(remaining), previous.ID, latest.ID)
	}
	return nil
}

func (s *scenario) writeUpdateRequest() (string, error) {
	request := map[string]any{
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
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode AppRun update request: %w", err)
	}
	path := s.requestDir + "/update.json"
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write AppRun update request: %w", err)
	}
	return path, nil
}

func (s *scenario) listVersions(ctx context.Context, step, appID string) ([]versionSummary, error) {
	data, err := s.call(ctx, step, "version", "list", "--application-id", appID, "--query", ".data")
	if err != nil {
		return nil, err
	}
	var versions []versionSummary
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("%s: decode AppRun version list: %w", step, err)
	}
	if versions == nil {
		return nil, fmt.Errorf("%s: expected a JSON version array", step)
	}
	return versions, nil
}

func (s *scenario) listTraffic(ctx context.Context, step, appID string) ([]trafficEntry, error) {
	data, err := s.call(ctx, step, "traffic", "list", appID, "--query", ".data")
	if err != nil {
		return nil, err
	}
	var entries []trafficEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: decode AppRun traffic list: %w", step, err)
	}
	return entries, nil
}

func (s *scenario) readVersion(ctx context.Context, step, appID, versionID string) (versionDetail, error) {
	data, err := s.call(ctx, step, "version", "read", "--application-id", appID, "--version-id", versionID)
	if err != nil {
		return versionDetail{}, err
	}
	var result versionDetail
	if err := json.Unmarshal(data, &result); err != nil {
		return versionDetail{}, fmt.Errorf("%s: decode AppRun version: %w", step, err)
	}
	return result, nil
}

func (s *scenario) waitVersionHealthy(ctx context.Context, appID, versionID string) error {
	waitCtx, cancel := context.WithTimeout(ctx, versionWaitLimit)
	defer cancel()
	ticker := time.NewTicker(versionPolling)
	defer ticker.Stop()

	for {
		statusData, err := s.call(waitCtx, "read-version-status", "version", "status", "--application-id", appID, "--version-id", versionID)
		if err != nil {
			if waitCtx.Err() != nil {
				return fmt.Errorf("waiting for AppRun version to become Healthy: %w", waitCtx.Err())
			}
			return err
		}
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(statusData, &status); err != nil {
			return fmt.Errorf("decode AppRun version status: %w", err)
		}
		switch status.Status {
		case "Healthy":
			return nil
		case "Deploying":
		case "UnHealthy":
			return fmt.Errorf("AppRun version %s status is UnHealthy", versionID)
		default:
			return fmt.Errorf("unexpected AppRun version status %q", status.Status)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for AppRun version to become Healthy: %w", ctx.Err())
		case <-waitCtx.Done():
			return fmt.Errorf("timed out after %s waiting for AppRun version to become Healthy", versionWaitLimit)
		case <-ticker.C:
		}
	}
}

// newestAndPrevious は最も新しい created_at を持つ version ともう一方の version を返します。
// 更新は初回作成後に最も新しい version を作成するため、
// 最新のものが更新後の version です。
func newestAndPrevious(versions []versionSummary) (versionSummary, versionSummary, error) {
	if len(versions) < 2 {
		return versionSummary{}, versionSummary{}, fmt.Errorf("need two versions to identify the updated one; got %d", len(versions))
	}
	first, err := parseVersionCreatedAt(versions[0])
	if err != nil {
		return versionSummary{}, versionSummary{}, err
	}
	second, err := parseVersionCreatedAt(versions[1])
	if err != nil {
		return versionSummary{}, versionSummary{}, err
	}
	if second.After(first) {
		return versions[1], versions[0], nil
	}
	return versions[0], versions[1], nil
}

func parseVersionCreatedAt(version versionSummary) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, version.CreatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse created_at %q of version %s: %w", version.CreatedAt, version.ID, err)
	}
	return parsed, nil
}

func checkUpdatedApplication(value app, id, name string) error {
	if value.ID != id || value.Name != name {
		return fmt.Errorf("updated AppRun application identity mismatch for ID %q", id)
	}
	if len(value.Components) != 1 || value.Components[0].MaxCPU != updatedMaxCPU || value.Components[0].MaxMemory != updatedMaxMemory {
		return fmt.Errorf("updated AppRun application components mismatch for ID %q", id)
	}
	return nil
}

func checkVersionDetail(value versionDetail, versionID string) error {
	if value.ID != versionID || value.Port != port || value.MinScale != 0 || value.MaxScale != 1 {
		return fmt.Errorf("AppRun version %s identity or configuration mismatch", versionID)
	}
	if len(value.Components) != 1 || value.Components[0].MaxCPU != updatedMaxCPU || value.Components[0].MaxMemory != updatedMaxMemory ||
		value.Components[0].DeploySource.ContainerRegistry.Image != image {
		return fmt.Errorf("AppRun version %s components mismatch", versionID)
	}
	return nil
}
