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

package apprundedicatedapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/stretchr/testify/require"
)

func TestReadRequest(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		for _, source := range []string{"inline", "file", "stdin"} {
			t.Run(source+"/"+map[bool]string{false: "ordinary", true: "sensitive"}[sensitive], func(t *testing.T) {
				const content = `{"registryPassword":"test-only-secret"}`
				input := content
				switch source {
				case "file":
					input = "@" + writeRequestFile(t, content)
				case "stdin":
					setStdin(t, content, false)
					input = "-"
				}
				data, err := readRequest(input, sensitive)
				if sensitive && source == "inline" {
					if err == nil || !strings.Contains(err.Error(), "@path.json") || strings.Contains(err.Error(), "test-only-secret") {
						t.Fatalf("error = %v, want safe private-input guidance", err)
					}
					if data != nil {
						t.Fatal("rejected input returned data")
					}
					return
				}
				if err != nil || string(data) != content {
					t.Fatalf("readRequest() = %q, %v", data, err)
				}
			})
		}
	}
}

func TestReadRequestErrors(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "sensitive"}[sensitive], func(t *testing.T) {
			_, err := readRequest("@"+filepath.Join(t.TempDir(), "missing"), sensitive)
			if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "リクエストファイル") {
				t.Fatalf("file error = %v", err)
			}
			setStdin(t, "", true)
			_, err = readRequest("-", sensitive)
			if !errors.Is(err, os.ErrClosed) || !strings.Contains(err.Error(), "標準入力") {
				t.Fatalf("stdin error = %v", err)
			}
		})
	}
}

func TestDecodeRequest(t *testing.T) {
	const content = `{"name":"cluster","servicePrincipalID":"123456789012"}`
	for _, input := range []string{content, "@" + writeRequestFile(t, content), "-"} {
		t.Run(input, func(t *testing.T) {
			if input == "-" {
				setStdin(t, content, false)
			}
			var request cluster.CreateParams
			require.NoError(t, decodeRequest(input, false, &request))
			require.Equal(t, "cluster", request.Name)
			require.Equal(t, "123456789012", request.ServicePrincipalID)
		})
	}
	for _, input := range []string{"@" + writeRequestFile(t, `{"image":"private-image","registryPassword":"test-only-secret"}`), "-"} {
		t.Run("sensitive/"+input, func(t *testing.T) {
			if input == "-" {
				setStdin(t, `{"image":"private-image","registryPassword":"test-only-secret"}`, false)
			}
			var request version.CreateParams
			require.NoError(t, decodeRequest(input, true, &request))
			require.Equal(t, "private-image", request.Image)
			require.NotNil(t, request.RegistryPassword)
			require.Equal(t, "test-only-secret", *request.RegistryPassword)
		})
	}
	t.Run("envVars を環境変数として読み込む", func(t *testing.T) {
		var request version.CreateParams
		require.NoError(t, decodeRequest(`{"image":"nginx:latest","envVars":[{"key":"EXAMPLE","value":"value1","secret":false}]}`, false, &request))
		require.Len(t, request.EnvVars, 1)
		require.Equal(t, "EXAMPLE", request.EnvVars[0].Key)
		require.NotNil(t, request.EnvVars[0].Value)
		require.Equal(t, "value1", *request.EnvVars[0].Value)
	})
}

func TestDecodeRequestErrorsDoNotExposeSecrets(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		sensitive bool
		want      string
	}{
		{"inline secret", `{"registryPassword":"test-only-secret"}`, true, "@path.json"},
		{"malformed inline", `{"name":"test-only-secret"`, false, "リクエスト JSON"},
		{"malformed file", "@" + writeRequestFile(t, `{"registryPassword":"test-only-secret"`), true, "リクエスト JSON"},
		{"wrong type", "@" + writeRequestFile(t, `{"registryPassword":{"value":"test-only-secret"}}`), true, "リクエスト JSON"},
		{"trailing JSON", "@" + writeRequestFile(t, `{"registryPassword":"test-only-secret"} {}`), true, "リクエスト JSON"},
		{"missing file", "@" + filepath.Join(t.TempDir(), "missing"), true, "リクエストファイル"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var request version.CreateParams
			err := decodeRequest(test.input, test.sensitive, &request)
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "test-only-secret") {
				t.Fatalf("error = %v, want safe error containing %q", err, test.want)
			}
		})
	}
	t.Run("malformed stdin", func(t *testing.T) {
		setStdin(t, `{"registryPassword":"test-only-secret"`, false)
		var request version.CreateParams
		err := decodeRequest("-", true, &request)
		if err == nil || !strings.Contains(err.Error(), "リクエスト JSON") || strings.Contains(err.Error(), "test-only-secret") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stdin read failure", func(t *testing.T) {
		setStdin(t, "", true)
		var request version.CreateParams
		if err := decodeRequest("-", true, &request); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("error = %v, want os.ErrClosed", err)
		}
	})
}

func TestDecodeRequestRejectsUnknownField(t *testing.T) {
	// 未知のフィールドは黙って無視せずエラーにします。リクエストの入力 JSON は
	// 環境変数を envVars で受け付けるため、API のフィールド名 env と書いても
	// 環境変数が黙って失われないことを、特にこの名前で確認します。
	for _, tc := range []struct {
		name   string
		input  string
		target any
		field  string
	}{
		{name: "未知のフィールド", input: `{"name":"cluster","unknown":"ignored"}`, target: new(cluster.CreateParams), field: "unknown"},
		{name: "API のフィールド名 env", input: `{"image":"nginx:latest","env":[{"key":"A","value":"B"}]}`, target: new(version.CreateParams), field: "env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeRequest(tc.input, false, tc.target)
			require.Error(t, err)
			require.Contains(t, err.Error(), "リクエスト JSON")
			require.Contains(t, err.Error(), tc.field)
		})
	}
}

func TestRunValue(t *testing.T) {
	sentinel := errors.New("callback failed")
	for _, stage := range []string{"success", "missing validate", "missing write", "validate", "missing client", "client", "API", "write"} {
		t.Run(stage, func(t *testing.T) {
			client := new(v1.Client)
			ctx := new(kong.Context)
			var calls []string
			runtime := Runtime{
				ValidateOutput: func(got *kong.Context) error {
					calls = append(calls, "validate")
					if got != ctx {
						t.Fatal("ValidateOutput context changed")
					}
					if stage == "validate" {
						return sentinel
					}
					return nil
				},
				NewClient: func() (*v1.Client, error) {
					calls = append(calls, "client")
					if stage == "client" {
						return nil, sentinel
					}
					return client, nil
				},
				WriteOutput: func(got *kong.Context, result any) error {
					calls = append(calls, "write")
					if got != ctx || result != "result" {
						t.Fatalf("WriteOutput args = %v, %v", got, result)
					}
					if stage == "write" {
						return sentinel
					}
					return nil
				},
			}
			switch stage {
			case "missing validate":
				runtime.ValidateOutput = nil
			case "missing write":
				runtime.WriteOutput = nil
			case "missing client":
				runtime.NewClient = nil
			}
			err := runValue(ctx, runtime, func(ctx context.Context, got *v1.Client) (string, error) {
				calls = append(calls, "API")
				if ctx == nil || got != client {
					t.Fatal("API callback args changed")
				}
				if stage == "API" {
					return "", sentinel
				}
				return "result", nil
			})
			wantCalls := []string{"validate", "client", "API", "write"}
			switch stage {
			case "missing validate", "missing write":
				wantCalls = nil
				if err == nil || !strings.Contains(err.Error(), "API 出力処理") {
					t.Fatalf("error = %v", err)
				}
			case "validate":
				wantCalls = wantCalls[:1]
			case "missing client":
				wantCalls = wantCalls[:1]
				if err == nil || !strings.Contains(err.Error(), "API クライアント") {
					t.Fatalf("error = %v", err)
				}
			case "client":
				wantCalls = wantCalls[:2]
			case "API":
				wantCalls = wantCalls[:3]
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, wantCalls)
			}
			if stage == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !strings.HasPrefix(stage, "missing") && !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want sentinel", err)
			}
		})
	}
}

func TestRunNoOutput(t *testing.T) {
	sentinel := errors.New("callback failed")
	for _, stage := range []string{"success", "missing client", "client", "API"} {
		t.Run(stage, func(t *testing.T) {
			client := new(v1.Client)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			runtime := Runtime{NewClient: func() (*v1.Client, error) {
				calls = append(calls, "client")
				if stage == "client" {
					return nil, sentinel
				}
				return client, nil
			}}
			if stage == "missing client" {
				runtime.NewClient = nil
			}
			err := runNoOutput(ctx, runtime, func(gotCtx context.Context, got *v1.Client) error {
				calls = append(calls, "API")
				if gotCtx != ctx || got != client {
					t.Fatal("API callback args changed")
				}
				if stage == "API" {
					return sentinel
				}
				return nil
			})
			wantCalls := []string{"client", "API"}
			switch stage {
			case "missing client":
				wantCalls = nil
				if err == nil || !strings.Contains(err.Error(), "API クライアント") {
					t.Fatalf("error = %v", err)
				}
			case "client":
				wantCalls = wantCalls[:1]
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, wantCalls)
			}
			if stage == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if stage != "missing client" && !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want sentinel", err)
			}
		})
	}
}

func TestParseIDAndCursor(t *testing.T) {
	const value = "fd16da34-68dc-4ccb-a3db-391207b5dd40"
	id, err := parseID[v1.ClusterID](value)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := parseCursor[v1.ClusterID](new(value))
	if err != nil || cursor == nil || *cursor != id {
		t.Fatalf("cursor = %v, %v", cursor, err)
	}
	cursor, err = parseCursor[v1.ClusterID](nil)
	if err != nil || cursor != nil {
		t.Fatalf("nil cursor = %v, %v", cursor, err)
	}
	if _, err := parseID[v1.ClusterID]("invalid"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err := parseCursor[v1.ClusterID](new("invalid")); err == nil || !strings.Contains(err.Error(), "カーソル") {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestParseVersionNumber(t *testing.T) {
	for _, value := range []string{"1", "2147483647", "-2147483648"} {
		if _, err := parseVersionNumber(value); err != nil {
			t.Fatalf("parseVersionNumber(%q): %v", value, err)
		}
	}
	for _, value := range []string{"", "one", "1.5", "2147483648", "-2147483649"} {
		if _, err := parseVersionNumber(value); err == nil || !strings.Contains(err.Error(), "32ビット整数") {
			t.Fatalf("parseVersionNumber(%q) error = %v", value, err)
		}
	}
}

func TestApplicationUpdateRejectsConflictingInputs(t *testing.T) {
	for _, command := range []ApplicationUpdateCommand{
		{ID: "invalid"},
		{ID: "invalid", ActiveVersion: new(int32(1)), Deactivate: true},
	} {
		if err := command.Run(nil); err == nil || !strings.Contains(err.Error(), "いずれか一方") {
			t.Fatalf("error = %v", err)
		}
	}
}

func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordedListRequest は一覧取得のテストサーバが受け取ったリクエストの記録です。
type recordedListRequest struct {
	Path     string
	Cursor   string
	MaxItems string
}

// setAppRunTestEndpoint はダミー認証情報とテスト用 API サーバのエンドポイントを環境変数に設定します。
func setAppRunTestEndpoint(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", serverURL)
}

// testApplicationJSON は ReadApplication の必須フィールドを含む応答フラグメントを返します。
func testApplicationJSON(id, clusterID, name string) string {
	return fmt.Sprintf(`{"applicationID":%q,"name":%q,"clusterID":%q,"clusterName":"cluster-1","activeVersion":null,"desiredCount":null,"scalingCooldownSeconds":300,"enoughResources":{"cpu":null,"memory":null}}`, id, name, clusterID)
}

// testLoadBalancerNodeJSON は LoadBalancerNode の必須フィールドを含む応答フラグメントを返します。
func testLoadBalancerNodeJSON(id string) string {
	return fmt.Sprintf(`{"loadBalancerNodeID":%q,"resourceID":null,"status":"healthy","interfaces":[],"created":1735689600}`, id)
}

// application list の型付きカーソルが、応答の nextCursor を次ページ取得へ引き渡せることを公開 SDK 経由で確認します。
func TestApplicationListCommandCursorRoundtrip(t *testing.T) {
	const (
		clusterID = "13b9ea83-ddb0-4385-9533-3d693a6a310f"
		app1ID    = "3f3f6f61-6df3-4f0e-8b9f-63d4d24b31a1"
		app2ID    = "8b0b8980-8f52-4b21-9be2-2a4f4bd49a2e"
	)
	var (
		requests []recordedListRequest
		captured []page[v1.ReadApplication]
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, recordedListRequest{
			Path:     r.URL.Path,
			Cursor:   r.URL.Query().Get("cursor"),
			MaxItems: r.URL.Query().Get("maxItems"),
		})
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"applications":[%s],"nextCursor":%q}`, testApplicationJSON(app1ID, clusterID, "app-1"), app1ID)))
		case 2:
			_, _ = w.Write([]byte(`{"applications":[` + testApplicationJSON(app2ID, clusterID, "app-2") + `]}`))
		}
	}))
	defer server.Close()
	setAppRunTestEndpoint(t, server.URL)

	command := ApplicationListCommand{
		MaxItems: 1,
		runtime: Runtime{
			ValidateOutput: func(*kong.Context) error { return nil },
			NewClient:      func() (*v1.Client, error) { return NewClient(false) },
			WriteOutput: func(_ *kong.Context, result any) error {
				captured = append(captured, result.(page[v1.ReadApplication]))
				return nil
			},
		},
	}
	require.NoError(t, command.Run(new(kong.Context)))
	require.Len(t, captured, 1)
	require.Len(t, captured[0].Items, 1)
	require.Equal(t, "app-1", captured[0].Items[0].Name)
	nextCursor, ok := captured[0].NextCursor.(*v1.ApplicationID)
	require.True(t, ok, "1ページ目では nextCursor が設定されるべきです")
	require.Equal(t, app1ID, uuid.UUID(*nextCursor).String())

	// 応答の nextCursor を次ページ取得に引き渡す。
	command.Cursor = new(app1ID)
	require.NoError(t, command.Run(new(kong.Context)))
	require.Len(t, captured, 2)
	require.Len(t, captured[1].Items, 1)
	require.Equal(t, "app-2", captured[1].Items[0].Name)
	require.Nil(t, captured[1].NextCursor, "最終ページでは nextCursor が空になるべきです")

	require.Len(t, requests, 2)
	require.Equal(t, "/applications", requests[0].Path)
	require.Empty(t, requests[0].Cursor)
	require.Equal(t, "1", requests[0].MaxItems)
	require.Equal(t, app1ID, requests[1].Cursor)
	require.Equal(t, "1", requests[1].MaxItems)
}

// load-balancer node list の新しい --cursor と nextCursor が往復できることを公開 SDK 経由で確認します。
func TestLoadBalancerNodeListCommandCursorRoundtrip(t *testing.T) {
	const (
		clusterID = "13b9ea83-ddb0-4385-9533-3d693a6a310f"
		asgID     = "e2f66e35-a1ba-47e3-a68c-9f4d2a0e0d01"
		lbID      = "c8b1e34f-4b27-4e9f-8d38-6c2a2f0a7e02"
		node1ID   = "5d3f9f61-9af3-4a1e-9c8f-14e5e34c32a3"
		node2ID   = "9c1c9980-3f52-5c21-8ce2-3b5f5ce59b3e"
	)
	wantPath := fmt.Sprintf("/clusters/%s/asg/%s/load_balancers/%s/load_balancer_nodes", clusterID, asgID, lbID)
	var (
		requests []recordedListRequest
		captured []page[v1.LoadBalancerNode]
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, recordedListRequest{
			Path:     r.URL.Path,
			Cursor:   r.URL.Query().Get("cursor"),
			MaxItems: r.URL.Query().Get("maxItems"),
		})
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"loadBalancerNodes":[%s],"nextCursor":%q}`, testLoadBalancerNodeJSON(node1ID), node1ID)))
		case 2:
			_, _ = w.Write([]byte(`{"loadBalancerNodes":[` + testLoadBalancerNodeJSON(node2ID) + `]}`))
		}
	}))
	defer server.Close()
	setAppRunTestEndpoint(t, server.URL)

	command := LoadBalancerNodeListCommand{
		ClusterID:          clusterID,
		AutoScalingGroupID: asgID,
		LoadBalancerID:     lbID,
		MaxItems:           1,
		runtime: Runtime{
			ValidateOutput: func(*kong.Context) error { return nil },
			NewClient:      func() (*v1.Client, error) { return NewClient(false) },
			WriteOutput: func(_ *kong.Context, result any) error {
				captured = append(captured, result.(page[v1.LoadBalancerNode]))
				return nil
			},
		},
	}
	require.NoError(t, command.Run(new(kong.Context)))
	require.Len(t, captured, 1)
	require.Len(t, captured[0].Items, 1)
	require.Equal(t, node1ID, uuid.UUID(captured[0].Items[0].LoadBalancerNodeID).String())
	nextCursor, ok := captured[0].NextCursor.(*v1.LoadBalancerNodeID)
	require.True(t, ok, "1ページ目では nextCursor が設定されるべきです")
	require.Equal(t, node1ID, uuid.UUID(*nextCursor).String())

	// 応答の nextCursor を次ページ取得に引き渡す。
	command.Cursor = new(node1ID)
	require.NoError(t, command.Run(new(kong.Context)))
	require.Len(t, captured, 2)
	require.Len(t, captured[1].Items, 1)
	require.Equal(t, node2ID, uuid.UUID(captured[1].Items[0].LoadBalancerNodeID).String())
	require.Nil(t, captured[1].NextCursor, "最終ページでは nextCursor が空になるべきです")

	require.Len(t, requests, 2)
	require.Equal(t, wantPath, requests[0].Path)
	require.Empty(t, requests[0].Cursor)
	require.Equal(t, "1", requests[0].MaxItems)
	require.Equal(t, node1ID, requests[1].Cursor)
	require.Equal(t, "1", requests[1].MaxItems)
}

// 不正なカーソルは API クライアントを生成する前に拒否されることを確認します。
func TestListCommandsRejectInvalidCursorBeforeClientCreation(t *testing.T) {
	const id = "13b9ea83-ddb0-4385-9533-3d693a6a310f"
	tests := map[string]func(Runtime) error{
		"application list": func(rt Runtime) error {
			command := ApplicationListCommand{Cursor: new("invalid"), runtime: rt}
			return command.Run(new(kong.Context))
		},
		"load-balancer node list": func(rt Runtime) error {
			command := LoadBalancerNodeListCommand{
				ClusterID:          id,
				AutoScalingGroupID: id,
				LoadBalancerID:     id,
				Cursor:             new("invalid"),
				runtime:            rt,
			}
			return command.Run(new(kong.Context))
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			clientCreated := false
			rt := Runtime{
				ValidateOutput: func(*kong.Context) error { return nil },
				NewClient: func() (*v1.Client, error) {
					clientCreated = true
					return nil, errors.New("クライアントは生成されません")
				},
				WriteOutput: func(*kong.Context, any) error { return nil },
			}
			err := run(rt)
			require.Error(t, err)
			require.Contains(t, err.Error(), "カーソル")
			require.False(t, clientCreated, "不正なカーソルはクライアント生成前に拒否されるべきです")
		})
	}
}

func setStdin(t *testing.T, content string, closed bool) {
	t.Helper()
	file, err := os.Open(writeRequestFile(t, content))
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = previous
		if !closed {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	if closed {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// listResponseJSON は一覧操作のパスに対応する空の一覧応答を返します。
// 一覧応答の配列フィールドは必須のため、応答キーを切り替えます。
// パスはリソース ID を含むため、末尾のセグメントで判定します。
func listResponseJSON(path string) string {
	switch {
	case strings.HasSuffix(path, "/load_balancer_nodes"):
		return `{"loadBalancerNodes":[]}`
	case strings.HasSuffix(path, "/worker_nodes"):
		return `{"workerNodes":[]}`
	case strings.HasSuffix(path, "/load_balancers"):
		return `{"loadBalancers":[]}`
	case strings.HasSuffix(path, "/asg"):
		return `{"autoScalingGroups":[]}`
	case strings.HasSuffix(path, "/certificates"):
		return `{"certificates":[]}`
	case strings.HasSuffix(path, "/versions"):
		return `{"versions":[]}`
	case strings.HasSuffix(path, "/applications"):
		return `{"applications":[]}`
	default:
		return `{"clusters":[]}`
	}
}

// TestListCommandsSendDefaultMaxItems は、一覧操作を引数なしで実行したときに
// kong の既定値 (20) が API リクエストの maxItems として送られることを確認します。
// 明示した値がそのまま送られることも合わせて確認します。
func TestListCommandsSendDefaultMaxItems(t *testing.T) {
	const (
		clusterID = "13b9ea83-ddb0-4385-9533-3d693a6a310f"
		appID     = "3f3f6f61-6df3-4f0e-8b9f-63d4d24b31a1"
		asgID     = "9f4b8b62-9a45-4b7e-9b1e-2f5cf0a5a1b2"
		lbID      = "57f7a1a0-6e5a-4b1f-9c0e-3c5e6e0f0e01"
	)

	grammar := struct {
		Cluster          ClusterListCommand          `cmd:"" name:"list-clusters"`
		Application      ApplicationListCommand      `cmd:"" name:"list-applications"`
		Version          VersionListCommand          `cmd:"" name:"list-application-versions"`
		AutoScalingGroup AutoScalingGroupListCommand `cmd:"" name:"list-auto-scaling-groups"`
		WorkerNode       WorkerNodeListCommand       `cmd:"" name:"list-worker-nodes"`
		LoadBalancer     LoadBalancerListCommand     `cmd:"" name:"list-load-balancers"`
		LoadBalancerNode LoadBalancerNodeListCommand `cmd:"" name:"list-load-balancer-nodes"`
		Certificate      CertificateListCommand      `cmd:"" name:"list-certificates"`
	}{}

	// run は grammar の各コマンドフィールドへのメソッド値です。パース後に
	// runtime を差し込んで呼び出すため、レシーバのフィールド値を参照します。
	tests := map[string]struct {
		args []string
		run  func(*kong.Context) error
		// wantQuery は送信される maxItems の期待値です。既定では kong の
		// 既定値 (20) を確認します。
		wantQuery string
	}{
		"list-clusters": {
			args: []string{"list-clusters"},
			run:  grammar.Cluster.Run,
		},
		"list-applications": {
			args: []string{"list-applications"},
			run:  grammar.Application.Run,
		},
		"list-application-versions": {
			args: []string{"list-application-versions", "--application-id", appID},
			run:  grammar.Version.Run,
		},
		"list-auto-scaling-groups": {
			args: []string{"list-auto-scaling-groups", "--cluster-id", clusterID},
			run:  grammar.AutoScalingGroup.Run,
		},
		"list-worker-nodes": {
			args: []string{"list-worker-nodes", "--cluster-id", clusterID, "--auto-scaling-group-id", asgID},
			run:  grammar.WorkerNode.Run,
		},
		"list-load-balancers": {
			args: []string{"list-load-balancers", "--cluster-id", clusterID, "--auto-scaling-group-id", asgID},
			run:  grammar.LoadBalancer.Run,
		},
		"list-load-balancer-nodes": {
			args: []string{"list-load-balancer-nodes", "--cluster-id", clusterID, "--auto-scaling-group-id", asgID, "--load-balancer-id", lbID},
			run:  grammar.LoadBalancerNode.Run,
		},
		"list-certificates": {
			args: []string{"list-certificates", "--cluster-id", clusterID},
			run:  grammar.Certificate.Run,
		},
		"明示した --max-items で既定を上書きする": {
			args:      []string{"list-clusters", "--max-items", "5"},
			run:       grammar.Cluster.Run,
			wantQuery: "5",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var maxItems []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				maxItems = append(maxItems, r.URL.Query().Get("maxItems"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(listResponseJSON(r.URL.Path)))
			}))
			defer server.Close()
			setAppRunTestEndpoint(t, server.URL)

			want := test.wantQuery
			if want == "" {
				want = "20"
			}

			parser := kong.Must(&grammar, kong.Name("skr"), kong.Writers(io.Discard, io.Discard),
				kong.Exit(func(int) { t.Fatal("kong.Exit が呼ばれました") }))
			kctx, err := parser.Parse(test.args)
			require.NoError(t, err)

			rt := Runtime{
				ValidateOutput: func(*kong.Context) error { return nil },
				NewClient:      func() (*v1.Client, error) { return NewClient(false) },
				WriteOutput:    func(*kong.Context, any) error { return nil },
			}
			grammar.Cluster.runtime = rt
			grammar.Application.runtime = rt
			grammar.Version.runtime = rt
			grammar.AutoScalingGroup.runtime = rt
			grammar.WorkerNode.runtime = rt
			grammar.LoadBalancer.runtime = rt
			grammar.LoadBalancerNode.runtime = rt
			grammar.Certificate.runtime = rt

			require.NoError(t, test.run(kctx))
			require.Equal(t, []string{want}, maxItems,
				"maxItems を期待値どおり 1 回だけ送ること")
		})
	}
}

// TestListWorkerNodesPreservesAPIAddress は、list-worker-nodes の出力が
// API 応答の addresses の値をそのまま出力することを確認します。
// 実 API は CIDR 付きのアドレス (例: 192.0.2.10/24) を返すことが
// 2026-11-12 のライブ検証で確認されているため、低レベルコマンドが
// 応答の値を加工しない契約をこのテストで固定します。
func TestListWorkerNodesPreservesAPIAddress(t *testing.T) {
	const (
		clusterID = "13b9ea83-ddb0-4385-9533-3d693a6a310f"
		asgID     = "9f4b8b62-9a45-4b7e-9b1e-2f5cf0a5a1b2"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"workerNodes":[{
			"workerNodeID":"01a1cb07-5678-9abc-def0-1234567890ab",
			"resourceID":"123456789012",
			"draining":false,
			"status":"healthy",
			"networkInterfaces":[{"interfaceIndex":0,"addresses":[{"address":"192.0.2.10/24"}]}],
			"created":1731500000
		}]}`))
	}))
	defer server.Close()
	setAppRunTestEndpoint(t, server.URL)

	grammar := struct {
		WorkerNode WorkerNodeListCommand `cmd:"" name:"list-worker-nodes"`
	}{}
	parser := kong.Must(&grammar, kong.Name("skr"), kong.Writers(io.Discard, io.Discard),
		kong.Exit(func(int) { t.Fatal("kong.Exit が呼ばれました") }))
	kctx, err := parser.Parse([]string{"list-worker-nodes", "--cluster-id", clusterID, "--auto-scaling-group-id", asgID})
	require.NoError(t, err)

	var raw []byte
	rt := Runtime{
		ValidateOutput: func(*kong.Context) error { return nil },
		NewClient:      func() (*v1.Client, error) { return NewClient(false) },
		WriteOutput: func(_ *kong.Context, value any) error {
			var err error
			raw, err = json.Marshal(value)
			return err
		},
	}
	grammar.WorkerNode.runtime = rt

	require.NoError(t, grammar.WorkerNode.Run(kctx))

	var out struct {
		Items []struct {
			NetworkInterfaces []struct {
				Addresses []string `json:"addresses"`
			} `json:"networkInterfaces"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotEmpty(t, out.Items, "list-worker-nodes should return one node")
	require.NotEmpty(t, out.Items[0].NetworkInterfaces, "the node should have one network interface")
	require.Equal(t, []string{"192.0.2.10/24"}, out.Items[0].NetworkInterfaces[0].Addresses,
		"list-worker-nodes は API の addresses を CIDR 付きのまま加工せず出力すること")
}
