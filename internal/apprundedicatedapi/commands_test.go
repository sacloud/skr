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
	"errors"
	"fmt"
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
			if err := decodeRequest(input, false, &request); err != nil {
				t.Fatal(err)
			}
			if request.Name != "cluster" || request.ServicePrincipalID != "123456789012" {
				t.Fatalf("request = %#v", request)
			}
		})
	}
	for _, input := range []string{"@" + writeRequestFile(t, `{"image":"private-image","registryPassword":"test-only-secret"}`), "-"} {
		t.Run("sensitive/"+input, func(t *testing.T) {
			if input == "-" {
				setStdin(t, `{"image":"private-image","registryPassword":"test-only-secret"}`, false)
			}
			var request version.CreateParams
			if err := decodeRequest(input, true, &request); err != nil {
				t.Fatal(err)
			}
			if request.Image != "private-image" || request.RegistryPassword == nil || *request.RegistryPassword != "test-only-secret" {
				t.Fatalf("sensitive request fields were not decoded")
			}
		})
	}
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

func TestDecodeRequestPreservesUnknownFieldBehavior(t *testing.T) {
	var request cluster.CreateParams
	if err := decodeRequest(`{"name":"cluster","unknown":"ignored"}`, false, &request); err != nil {
		t.Fatal(err)
	}
	if request.Name != "cluster" {
		t.Fatalf("Name = %q", request.Name)
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
