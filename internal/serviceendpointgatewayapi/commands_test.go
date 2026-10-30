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

package serviceendpointgatewayapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"

	v1 "github.com/sacloud/sacloud-sdk-go/api/service-endpoint-gateway/apis/v1"
)

// fakeApplianceAPI は ApplianceAPI の動作を置き換えるテスト用実装です。
type fakeApplianceAPI struct {
	createRequest v1.ModelsApplianceApplianceCreateRequest
	createErr     error
}

func (f *fakeApplianceAPI) List(context.Context) (*v1.ModelsApplianceApplianceListResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) Read(context.Context, string) (*v1.ModelsApplianceApplianceGetResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) Create(_ context.Context, request v1.ModelsApplianceApplianceCreateRequest) (*v1.ModelsApplianceApplianceCreateResponseBody, error) {
	f.createRequest = request
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &v1.ModelsApplianceApplianceCreateResponseBody{}, nil
}

func (f *fakeApplianceAPI) Update(context.Context, string, v1.ModelsApplianceApplianceUpdateRequest) (*v1.ModelsApplianceApplianceGetResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) Apply(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeApplianceAPI) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeApplianceAPI) ReadInterface(context.Context, string, string) (*v1.ModelsApplianceApplianceGetInterfaceResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) ReadPowerStatus(context.Context, string) (*v1.ModelsPowerApplianceGetPowerStatusResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) PowerOn(context.Context, string) (*v1.ModelsPowerApplianceUpdatePowerStatusResponseBody, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeApplianceAPI) Shutdown(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeApplianceAPI) Reset(context.Context, string) error {
	return errors.New("not implemented")
}

// testRuntime は出力処理をスタブに差し替えた ApplianceRuntime です。
func testRuntime() ApplianceRuntime {
	return ApplianceRuntime{
		// decodeRequest と同じ共有処理の動作を模擬します (@path 形式に対応)
		DecodeRequest:  decodeTestRequest,
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput: func(ctx *kong.Context, value any) error {
			encoder := json.NewEncoder(ctx.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(value)
		},
	}
}

// decodeTestRequest は --request JSON を復号するテスト用ヘルパーです。inline JSON と
// @path 形式に対応し、internal/cli の decodeRequest と同じ動作を模擬します。
func decodeTestRequest(input string, destination any) error {
	data := []byte(input)
	if len(input) > 0 && input[0] == '@' {
		read, err := os.ReadFile(input[1:])
		if err != nil {
			return fmt.Errorf("read request file: %w", err)
		}
		data = read
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func parseCommand(t *testing.T, command any, args ...string) *kong.Context {
	t.Helper()
	parser, err := kong.New(command, kong.Exit(func(int) {}))
	require.NoError(t, err)
	ctx, err := parser.Parse(args)
	require.NoError(t, err)
	return ctx
}

func TestApplianceCreateCommandRun(t *testing.T) {
	t.Run("creates appliance with flags", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--switch-id", "112900500001", "--network-mask-len", "24", "--server-ip", "192.168.2.100", "--server-ip", "192.168.2.101")
		require.NoError(t, command.Run(kongCtx))
		remark := fake.createRequest.Appliance.Remark
		require.Equal(t, "112900500001", remark.Switch.ID)
		require.EqualValues(t, 24, remark.Network.NetworkMaskLen)
		require.Equal(t, []v1.ModelsRemarkServerRemark{{IPAddress: "192.168.2.100"}, {IPAddress: "192.168.2.101"}}, remark.Servers)
	})

	t.Run("creates appliance without server ip", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--switch-id", "112900500001", "--network-mask-len", "24")
		require.NoError(t, command.Run(kongCtx))
		remark := fake.createRequest.Appliance.Remark
		require.Equal(t, "112900500001", remark.Switch.ID)
		require.EqualValues(t, 24, remark.Network.NetworkMaskLen)
		// Servers は API のリクエストで必須のため、未指定でも空配列を送る
		require.Equal(t, []v1.ModelsRemarkServerRemark{}, remark.Servers)
	})

	t.Run("creates appliance from request json", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		requestJSON := `{"Appliance":{"Remark":{"Switch":{"ID":"112900500001"},"Network":{"NetworkMaskLen":28},"Servers":[{"IPAddress":"192.168.2.100"}]}}}`
		kongCtx := parseCommand(t, command, "--request", requestJSON)
		require.NoError(t, command.Run(kongCtx))
		remark := fake.createRequest.Appliance.Remark
		require.Equal(t, "112900500001", remark.Switch.ID)
		require.EqualValues(t, 28, remark.Network.NetworkMaskLen)
		require.Equal(t, []v1.ModelsRemarkServerRemark{{IPAddress: "192.168.2.100"}}, remark.Servers)
	})

	t.Run("creates appliance from request file", func(t *testing.T) {
		requestFile := "testdata/create-request.json"
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+requestFile)
		require.NoError(t, command.Run(kongCtx))
		remark := fake.createRequest.Appliance.Remark
		require.Equal(t, "112900500001", remark.Switch.ID)
		require.EqualValues(t, 24, remark.Network.NetworkMaskLen)
		require.Equal(t, []v1.ModelsRemarkServerRemark{{IPAddress: "192.168.2.100"}}, remark.Servers)
	})

	t.Run("request and flags are mutually exclusive", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "{}", "--switch-id", "112900500001")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request と個別フラグは併用できません")
	})

	t.Run("missing switch id", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--network-mask-len", "24")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--switch-id が必要です")
	})

	t.Run("missing network mask len", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--switch-id", "112900500001")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--network-mask-len が必要です")
	})

	t.Run("no input", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request または個別フラグが必要です")
	})

	t.Run("missing request file", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@missing-request.json")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "read request file")
	})

	t.Run("invalid request json", func(t *testing.T) {
		fake := &fakeApplianceAPI{}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "{invalid")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "decode request JSON")
	})

	t.Run("factory error", func(t *testing.T) {
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return nil, errors.New("client error") },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--switch-id", "112900500001", "--network-mask-len", "24")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "client error")
	})

	t.Run("api error", func(t *testing.T) {
		fake := &fakeApplianceAPI{createErr: errors.New("api error")}
		command := &ApplianceCreateCommand{
			factory: func() (ApplianceAPI, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--switch-id", "112900500001", "--network-mask-len", "24")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "api error")
	})
}
