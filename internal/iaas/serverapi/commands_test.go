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

package serverapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func TestParseDevices(t *testing.T) {
	for _, test := range []struct {
		input   string
		plan    int64
		size    int
		archive int64
	}{
		{"archive-id=123,plan-id=4,size-gb=20", 4, 20, 123},
		{"size-gb=100,plan-id=4", 4, 100, 0},
	} {
		value, err := parseDisk(test.input, "server")
		if err != nil {
			t.Fatal(err)
		}
		if int64(value.DiskPlanID) != test.plan || value.SizeGB != test.size ||
			int64(value.SourceArchiveID) != test.archive || value.Name != "server" || value.Connection != "virtio" {
			t.Fatalf("unexpected disk: %+v", value)
		}
	}
	for _, upstream := range []string{"shared", "disconnected", "123", "9223372036854775807"} {
		value, err := parseNIC("upstream=" + upstream)
		if err != nil || value.Upstream != upstream {
			t.Fatalf("NIC = %+v, error = %v", value, err)
		}
	}
}

func TestParseDevicesRejectInvalidInputsWithoutEcho(t *testing.T) {
	for _, input := range []string{
		"", "plan-id=4", "size-gb=20", "archive-id=123",
		"plan-id=0,size-gb=20", "plan-id=-4,size-gb=20",
		"plan-id=4,size-gb=0", "plan-id=4,size-gb=-1",
		"plan-id=4,size-gb=20,archive-id=0",
		"plan-id=4,size-gb=20,archive-id=-1",
		"plan-id=9223372036854775808,size-gb=20",
		"plan-id=4,size-gb=9223372036854775808",
		"plan-id=4,size-gb=20,archive-id=9223372036854775808",
		"plan-id=4,size-gb=20,plan-id=4", "plan-id=4,size-gb=20,",
		"plan-id=4,,size-gb=20", "plan-id=,size-gb=20", "plan-id=4,size-gb",
		"plan-id=4,size-gb=20=secret-value", "password=secret-value",
		"plan-id=4,size-gb=secret-value", "plan-id=+4,size-gb=20",
		"plan-id=4, size-gb=20", "plan-id=4,size-gb=20\n",
	} {
		if _, err := parseDisk(input, "server"); err == nil || strings.Contains(err.Error(), "secret-value") {
			t.Errorf("input %q: error = %v", input, err)
		}
	}
	for _, input := range []string{
		"", "upstream=", "upstream=0", "upstream=-1", "upstream=+123",
		"upstream=9223372036854775808", "upstream=123\n", "upstream=secret-value",
		"upstream=shared,upstream=disconnected", "upstream=shared,",
		"password=secret-value", "upstream=shared=secret-value",
	} {
		if _, err := parseNIC(input); err == nil || strings.Contains(err.Error(), "secret-value") {
			t.Errorf("input %q: error = %v", input, err)
		}
	}
}

type createTestAPI struct {
	API
	request *server.CreateRequest
}

func (a *createTestAPI) CreateWithContext(_ context.Context, request *server.CreateRequest) (*iaas.Server, error) {
	a.request = request
	return &iaas.Server{Name: request.Name}, nil
}

func TestCreateCommandDevicesAndExclusivity(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{"multiple", []string{"--disk", "archive-id=123,plan-id=4,size-gb=20", "--disk", "plan-id=4,size-gb=100", "--nic", "upstream=shared", "--nic", "upstream=disconnected"}, false},
		{"switch NIC", []string{"--nic", "upstream=123", "--nic", "upstream=456"}, false},
		{"archive boot conflict", []string{"--disk", "archive-id=123,plan-id=4,size-gb=20", "--boot-after-create"}, true},
		{"empty disk boot", []string{"--disk", "plan-id=4,size-gb=20", "--boot-after-create"}, false},
		{"partial disk", []string{"--disk", "archive-id=123"}, true},
		{"nonpositive disk", []string{"--disk", "plan-id=0,size-gb=20"}, true},
		{"invalid second disk", []string{"--disk", "plan-id=4,size-gb=20", "--disk", ""}, true},
		{"invalid NIC", []string{"--nic", "upstream=wrong"}, true},
		{"shared second NIC", []string{"--nic", "upstream=disconnected", "--nic", "upstream=shared"}, true},
		{"repeated shared NIC", []string{"--nic", "upstream=shared", "--nic", "upstream=shared"}, true},
		{"request disk", []string{"--request", "{}", "--disk", "plan-id=4,size-gb=20"}, true},
		{"request empty disk", []string{"--request", "{}", "--disk", ""}, true},
		{"request NIC", []string{"--request", "{}", "--nic", ""}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CreateCommand{}
			parser, err := kong.New(&command)
			if err != nil {
				t.Fatal(err)
			}

			args := append([]string{"--zone", "is1b", "--name", "server", "--cpu", "1", "--memory-gb", "1"}, test.args...)
			if strings.HasPrefix(test.name, "request ") {
				args = test.args
			}
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			api := &createTestAPI{}
			command.factory = func() (API, error) { return api, nil }
			command.runtime = Runtime{
				TransformCreateRequest: TransformCreateRequest,
				ValidateOutput:         func(*kong.Context) error { return nil },
				WriteOutput:            func(*kong.Context, any) error { return nil },
			}
			err = command.Run(ctx)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
			if test.wantError && api.request != nil {
				t.Fatal("invalid request reached API")
			}
			if (test.name == "shared second NIC" || test.name == "repeated shared NIC") &&
				(err == nil || !strings.Contains(err.Error(), "--nic の 2 個目") || !strings.Contains(err.Error(), "先頭")) {
				t.Fatalf("expected NIC position error, got %v", err)
			}
			if test.name == "multiple" {
				if len(api.request.Disks) != 2 || api.request.Disks[0].SourceArchiveID != 123 ||
					api.request.Disks[1].SizeGB != 100 || api.request.Disks[0].Name != "server" ||
					api.request.Disks[1].Name != "server" || len(api.request.NetworkInterfaces) != 2 ||
					api.request.NetworkInterfaces[0].Upstream != "shared" || api.request.NetworkInterfaces[1].Upstream != "disconnected" {
					t.Fatalf("unexpected request: %+v", api.request)
				}
			}
			if test.name == "switch NIC" && (len(api.request.NetworkInterfaces) != 2 ||
				api.request.NetworkInterfaces[0].Upstream != "123" || api.request.NetworkInterfaces[1].Upstream != "456") {
				t.Fatalf("unexpected switch NIC request: %+v", api.request)
			}
		})
	}
}

func TestCreateCommandRequestTransformLifecycle(t *testing.T) {
	baseArgs := []string{"--zone", "is1b", "--name", "server", "--cpu", "1", "--memory-gb", "1"}
	transformError := errors.New("transform failed")
	for _, test := range []struct {
		name string
		args []string
		mode string
		want string
	}{
		{name: "flag transform", args: append(append([]string{}, baseArgs...), "--disk", "plan-id=4,size-gb=20"), mode: "normal"},
		{name: "without devices", args: baseArgs, mode: "normal"},
		{name: "missing callback", args: baseArgs, mode: "missing", want: "入力変換が設定されていません"},
		{name: "callback error", args: baseArgs, mode: "error", want: transformError.Error()},
		{name: "validator rejects flags", args: []string{"--zone", "all", "--name", "server", "--cpu", "1", "--memory-gb", "1"}, mode: "normal", want: `Zone に "all"`},
		{name: "validator rejects JSON", args: []string{"--request", `{"Zone":"all","Name":"json","CPU":2,"MemoryGB":4}`}, mode: "missing", want: `Zone に "all"`},
		{name: "JSON bypasses transform", args: []string{"--request", `{"Zone":"is1b","Name":"json","CPU":2,"MemoryGB":4,"NetworkInterfaces":[{"Upstream":"disconnected"}]}`}, mode: "missing"},
		{name: "explicit empty disk", args: []string{"--request", "{}", "--disk", ""}, mode: "normal", want: "併用できません"},
		{name: "explicit empty nic", args: []string{"--request", "{}", "--nic", ""}, mode: "normal", want: "併用できません"},
		{name: "missing name", args: []string{"--zone", "is1b", "--cpu", "1", "--memory-gb", "1", "--disk", "plan-id=4,size-gb=20"}, mode: "normal", want: "--name が必要です"},
		{name: "no input", mode: "normal", want: "--request または個別フラグが必要です"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CreateCommand{}
			parser, err := kong.New(&command)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(test.args)
			if err != nil {
				t.Fatal(err)
			}
			api := &createTestAPI{}
			command.factory = func() (API, error) { return api, nil }
			transformed := false
			command.runtime = Runtime{
				DecodeRequest:  func(input string, target any) error { return json.Unmarshal([]byte(input), target) },
				ValidateOutput: func(*kong.Context) error { return nil },
				WriteOutput:    func(*kong.Context, any) error { return nil },
			}
			if test.mode != "missing" {
				command.runtime.TransformCreateRequest = func(c *CreateCommand, request *server.CreateRequest) error {
					transformed = true
					if request.Name != "server" || request.Zone == "" || request.CPU != 1 || request.MemoryGB != 1 {
						t.Fatalf("scalars were not assigned before transformation: %+v", request)
					}
					if test.mode == "error" {
						return transformError
					}
					return TransformCreateRequest(c, request)
				}
			}
			err = command.Run(ctx)
			if test.want != "" {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Run() = %v, want %q", err, test.want)
				}
				if api.request != nil {
					t.Fatal("failed input reached API")
				}
				if test.mode == "error" && !errors.Is(err, transformError) {
					t.Fatal("transform error identity was lost")
				}
				return
			}
			if err != nil || api.request == nil {
				t.Fatalf("Run() = %v, request = %+v", err, api.request)
			}
			if command.Request != nil && (transformed || api.request.Name != "json" || len(api.request.NetworkInterfaces) != 1) {
				t.Fatal("JSON request was transformed or lost fields")
			}
		})
	}
}

type bootTestAPI struct {
	API
	request *server.BootRequest
}

func (a *bootTestAPI) BootWithContext(_ context.Context, request *server.BootRequest) error {
	a.request = request
	return nil
}

func TestBootCommandOptionalUserData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-data.yaml")
	data := "#cloud-config\nhostname: example\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		args      []string
		want      string
		wantError bool
	}{
		{name: "omitted"},
		{name: "file", args: []string{"--user-data", "@" + path}, want: data},
		{name: "explicit empty", args: []string{"--user-data", ""}, wantError: true},
		{name: "inline", args: []string{"--user-data", "secret-value"}, wantError: true},
		{name: "missing file", args: []string{"--user-data", "@" + path + ".missing"}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := BootCommand{}
			parser, err := kong.New(&command)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(append([]string{"--zone", "is1b", "--id", "123"}, test.args...))
			if err != nil {
				t.Fatal(err)
			}
			api := &bootTestAPI{}
			command.factory = func() (API, error) { return api, nil }
			command.runtime = Runtime{}
			err = command.Run(ctx)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
			if test.wantError {
				if api.request != nil || strings.Contains(err.Error(), "secret-value") {
					t.Fatalf("invalid input reached API or was echoed: %v", err)
				}
				return
			}
			if api.request == nil || api.request.Zone != "is1b" || api.request.ID != 123 || api.request.UserData != test.want {
				t.Fatalf("unexpected Boot request: %+v", api.request)
			}
		})
	}
}

func TestReadUserData(t *testing.T) {
	userData := "#cloud-config\nssh_authorized_keys:\n  - ssh-ed25519 AAAA-example\n"
	path := filepath.Join(t.TempDir(), "user-data.yaml")
	if err := os.WriteFile(path, []byte(userData), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   string
		stdin   io.Reader
		want    string
		wantErr string
	}{
		{name: "file", input: "@" + path, want: userData},
		{name: "standard input", input: "-", stdin: strings.NewReader(userData), want: userData},
		{name: "inline rejected", input: userData, wantErr: "--user-data は @path.yaml または -"},
		{name: "empty input", input: "-", stdin: strings.NewReader(" \n"), wantErr: "user-data is empty"},
		{name: "missing file", input: "@missing", wantErr: "read user-data file"},
		{name: "empty file path", input: "@", wantErr: "--user-data は @path.yaml または -"},
		{name: "stdin read failure", input: "-", stdin: errorReader{}, wantErr: "read user-data from standard input"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readUserData(test.input, test.stdin)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("readUserData() error = %v, want containing %q", err, test.wantErr)
				}
				if strings.Contains(err.Error(), "AAAA-example") {
					t.Fatalf("readUserData() error leaked user-data: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("readUserData() error = %v", err)
			}
			if got != test.want {
				t.Errorf("readUserData() = %q, want %q", got, test.want)
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
