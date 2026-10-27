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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// namePrefix は今回の実行が作成するアプリケーション名の接頭辞です。
	namePrefix = "skr-e2e-apprun-"
	// image はアプリケーションのデプロイ元コンテナイメージです。
	image = "ghcr.io/sacloud/apprun-samples/tutorial:latest"
	// port はアプリケーションが待ち受けるポートです。
	port = 8080
	// appStatusWaitLimit はアプリケーションが Healthy になるまでの確認期限です。
	appStatusWaitLimit = 3 * time.Minute
	// statusPolling はステータスの再試行間隔です。
	statusPolling = 10 * time.Second
)

// appComponent は後片付けの照合に使うアプリケーションコンポーネントの項目です。
type appComponent struct {
	MaxCPU       string `json:"max_cpu"`
	MaxMemory    string `json:"max_memory"`
	DeploySource struct {
		ContainerRegistry struct {
			Image string `json:"image"`
		} `json:"container_registry"`
	} `json:"deploy_source"`
}

// app は CLI の JSON 出力をアプリケーションとして解釈するための項目です。
type app struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Port       int            `json:"port"`
	PublicURL  string         `json:"public_url"`
	Components []appComponent `json:"components"`
}

// appSummary は一覧照合に使うアプリケーションの要約です。
type appSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestScenarioAppRun(t *testing.T) {
	h := helper.New(t, "apprun-api", "AppRun アプリケーションのライフサイクルと、パケットフィルター・ユーザー・バージョン・トラフィックの各操作が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := namePrefix + suffix

	// 作成前に同名アプリケーションが存在しないことを確認する
	require.Empty(t, findApplicationsByName(t, h, "作成前に同名アプリケーションが存在しないことを確認する", name), "application name %q already exists; refusing to modify it", name)

	// アプリケーションを作成する
	createRequest := h.WriteRequestJSONFile(t, "create", map[string]any{
		"name":                     name,
		"timeout_seconds":          60,
		"port":                     port,
		"min_scale":                0,
		"max_scale":                1,
		"scale_target_concurrency": 100,
		"components": []any{
			map[string]any{
				"name":       "web",
				"max_cpu":    "0.5",
				"max_memory": "1Gi",
				"deploy_source": map[string]any{
					"container_registry": map[string]any{"image": image},
				},
			},
		},
	})
	data := h.Call(t, "アプリケーションを作成する",
		"apprun-api", "application", "create", "--request", "@"+createRequest)
	createdApp := parseApp(t, data)
	requireAppIdentity(t, createdApp, createdApp.ID, name)

	defer func() {
		// アプリケーションを削除し、一覧に残っていないことを確認する
		// 削除前に今回の作成物と設定を再照合し、一致しない場合は削除しない
		data = h.Call(t, "削除前にアプリケーションを再取得する", "apprun-api", "application", "read", createdApp.ID)
		currentApp := parseApp(t, data)
		requireAppIdentity(t, currentApp, createdApp.ID, name)
		_ = h.Call(t, "アプリケーションを削除する", "apprun-api", "application", "delete", createdApp.ID)
		remaining := findApplicationsByID(t, h, "削除後にアプリケーションが一覧に残っていないことを確認する", createdApp.ID)
		require.Empty(t, remaining, "application %s should not remain after deletion", createdApp.ID)
	}()

	// 作成したアプリケーションが一覧に含まれることを確認する
	listed := findApplicationsByID(t, h, "作成したアプリケーションが一覧に含まれることを確認する", createdApp.ID)
	require.Len(t, listed, 1, "application list should contain exactly one match for ID %s", createdApp.ID)
	require.Equal(t, name, listed[0].Name, "the listed application name should match the name requested by this run")

	// アプリケーションを参照する
	data = h.Call(t, "アプリケーションを参照する", "apprun-api", "application", "read", createdApp.ID)
	readApp := parseApp(t, data)
	requireAppIdentity(t, readApp, createdApp.ID, name)

	// ステータスが Healthy になるまで待つ
	waitApplicationHealthy(t, h, createdApp.ID)

	// 公開 URL がサンプル応答を返すことを確認する
	checkPublicURL(t, readApp.PublicURL)

	// パケットフィルターを有効化して確認し、無効状態に戻す
	runPacketFilterScenario(t, h, createdApp.ID)

	// AppRun ユーザーを確認する
	runUserScenario(t, h)

	// アプリケーションを更新して、新しいバージョンへトラフィックを振り分ける
	runVersionScenario(t, h, createdApp.ID, name)
}

// listApplicationSummaries はページングしながらアプリケーション一覧の id と name を取得します。
// 途中ページの取得に失敗した場合はテストを停止し、不在として扱いません。
func listApplicationSummaries(t *testing.T, h *helper.Helper, description string) []appSummary {
	t.Helper()

	items := make([]appSummary, 0)
	for page := 1; ; page++ {
		data := h.CallWithTimeout(t, 60*time.Second, fmt.Sprintf("%s（%d ページ目）", description, page),
			"apprun-api", "application", "list",
			"--page-num", strconv.Itoa(page), "--page-size", "100",
			"--query", ".data | map({id,name})")
		var batch []appSummary
		err := json.Unmarshal(data, &batch)
		require.NoError(t, err, "%s: decode application list page %d: %v", description, page, err)
		require.NotNil(t, batch, "%s: expected a JSON application array on page %d", description, page)
		items = append(items, batch...)
		if len(batch) < 100 {
			return items
		}
	}
}

// findApplicationsByName は全ページの一覧から、指定した名前のアプリケーションを検索します。
func findApplicationsByName(t *testing.T, h *helper.Helper, description, name string) []appSummary {
	t.Helper()

	matches := make([]appSummary, 0)
	for _, item := range listApplicationSummaries(t, h, description) {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches
}

// findApplicationsByID は全ページの一覧から、指定した ID のアプリケーションを検索します。
func findApplicationsByID(t *testing.T, h *helper.Helper, description, id string) []appSummary {
	t.Helper()

	matches := make([]appSummary, 0)
	for _, item := range listApplicationSummaries(t, h, description) {
		if item.ID == id {
			matches = append(matches, item)
		}
	}
	return matches
}

// parseApp は CLI の JSON 出力をアプリケーションとして解釈します。
func parseApp(t *testing.T, data []byte) app {
	t.Helper()

	var value app
	err := json.Unmarshal(data, &value)
	require.NoError(t, err, "decode application: %v", err)
	require.NotEmpty(t, value.ID, "application ID is missing")
	return value
}

// requireAppIdentity は参照結果の ID・名前・ポート・公開 URL・デプロイ元イメージが、
// 今回の実行で作成した内容と一致することを確認します。
func requireAppIdentity(t *testing.T, value app, id, name string) {
	t.Helper()

	require.Equal(t, id, value.ID, "application ID should match the application created by this run")
	require.Equal(t, name, value.Name, "application name should match the name requested by this run")
	require.Equal(t, port, value.Port, "application port should match the port requested by this run")
	require.NotEmpty(t, value.PublicURL, "application should have a public URL")
	require.Len(t, value.Components, 1, "application should have one component")
	require.Equal(t, image, value.Components[0].DeploySource.ContainerRegistry.Image, "application deploy source image should match the image requested by this run")
}

// waitApplicationHealthy はステータスが Healthy になるまで期限付きで待ちます。
// UnHealthy や想定外のステータスは失敗として扱います。
func waitApplicationHealthy(t *testing.T, h *helper.Helper, appID string) {
	t.Helper()

	deadline := time.Now().Add(appStatusWaitLimit)
	for {
		data := h.CallWithTimeout(t, 60*time.Second, "アプリケーションのステータスを取得する",
			"apprun-api", "application", "status", appID)
		var status struct {
			Status string `json:"status"`
		}
		err := json.Unmarshal(data, &status)
		require.NoError(t, err, "decode application status: %v", err)
		switch status.Status {
		case "Healthy":
			return
		case "Deploying":
		case "UnHealthy":
			t.Fatalf("application %s status is UnHealthy", appID)
		default:
			t.Fatalf("unexpected application status %q", status.Status)
		}
		if time.Now().Add(statusPolling).After(deadline) {
			t.Fatalf("timed out after %s waiting for the application to become Healthy", appStatusWaitLimit)
		}
		h.Sleep(t, "アプリケーションのステータス確認まで待つ", statusPolling)
	}
}

// checkPublicURL は公開 URL に GET し、HTTP 200 とサンプル本文が返ることを期限付きで確認します。
// 公開 URL の反映には時間がかかるため、期限付きで再試行します。
func checkPublicURL(t *testing.T, publicURL string) {
	t.Helper()

	parsed, err := url.Parse(publicURL)
	require.NoError(t, err, "parse application public URL: %v", err)
	require.Equal(t, "https", parsed.Scheme, "application public URL should be HTTPS")
	require.NotEmpty(t, parsed.Host, "application public URL should have a host")

	require.NoError(t, checkPublicURLWithClient(t, t.Context(), defaultPublicURLClient(), publicURLPolling, publicURL),
		"application public URL did not return the expected sample response")
}

// defaultPublicURLClient は公開 URL の確認に使う HTTP クライアントを返します。
// リダイレクトは追従せず、応答の本文を確認します。
func defaultPublicURLClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// publicURLPolling は公開 URL の再試行間隔です。
const publicURLPolling = 10 * time.Second

// publicURLWaitLimit は公開 URL の確認期限です。
const publicURLWaitLimit = 5 * time.Minute

// checkPublicURLWithClient は公開 URL の確認を再試行付きで実行します。
// HTTP 200 と本文 Hello from AppRun! が揃った時点で成功とし、期限までに確認できない場合はエラーを返します。
func checkPublicURLWithClient(t *testing.T, ctx context.Context, client *http.Client, interval time.Duration, publicURL string) error {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(ctx, publicURLWaitLimit)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastErr error
	for {
		status, body, requestErr := getPublicURL(ctx, client, publicURL)
		if requestErr == nil && status == http.StatusOK && strings.Contains(body, "Hello from AppRun!") {
			t.Logf("public URL returned HTTP %d with the sample body", status)
			return nil
		}
		lastErr = requestErr
		if lastErr == nil {
			lastErr = fmt.Errorf("unexpected HTTP status or sample response: HTTP %d", status)
		}
		t.Logf("public URL is not ready yet: %v", lastErr)
		select {
		case <-ctx.Done():
			return fmt.Errorf("application public URL check was canceled: %w", lastErr)
		case <-waitCtx.Done():
			return fmt.Errorf("timed out after %s waiting for the expected sample response: %w", publicURLWaitLimit, lastErr)
		case <-ticker.C:
		}
	}
}

// getPublicURL は公開 URL に GET し、HTTP ステータスと本文を返します。
// 応答本文は 1MiB を上限として読み込みます。
func getPublicURL(ctx context.Context, client *http.Client, publicURL string) (int, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL, nil)
	if err != nil {
		return 0, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, "", err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if err != nil {
		return response.StatusCode, string(data), errors.Join(err, closeErr)
	}
	if closeErr != nil {
		return response.StatusCode, string(data), fmt.Errorf("close public URL response: %w", closeErr)
	}
	return response.StatusCode, string(data), nil
}

// TestCheckPublicURLRetriesUntilSampleResponse は公開 URL の確認が、
// 一時的な失敗の後にサンプル応答を確認できることを確認します。
func TestCheckPublicURLRetriesUntilSampleResponse(t *testing.T) {
	attempt := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporarily unavailable"))
			return
		}
		_, _ = w.Write([]byte("Hello from AppRun!"))
	}))
	defer server.Close()

	err := checkPublicURLWithClient(t, t.Context(), server.Client(), time.Millisecond, server.URL)
	require.NoError(t, err, "the sample response should be confirmed after the retry")
	require.Equal(t, 2, attempt, "the check should retry until the sample response")
}

// TestCheckPublicURLFailsWhenSampleResponseIsMissing はサンプル応答が確認できない場合に、
// 期限付きでエラーになることを確認します。
func TestCheckPublicURLFailsWhenSampleResponseIsMissing(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected body"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := checkPublicURLWithClient(t, ctx, server.Client(), time.Millisecond, server.URL)
	require.Error(t, err, "an unexpected body should fail the check")
}
