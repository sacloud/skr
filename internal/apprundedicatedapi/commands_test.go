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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
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
