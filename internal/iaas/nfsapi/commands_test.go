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

package nfsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// fakeNFSAPI は生成コマンドが使う API のテスト用実装です。
// コマンドの入力から API へ渡されるリクエストの内容を検証します。
type fakeNFSAPI struct {
	findZone      string
	findRequest   *FindRequest
	findResult    []*iaas.NFS
	findErr       error
	readZone      string
	readRequest   *ReadRequest
	readResult    *iaas.NFS
	readErr       error
	createZone    string
	createRequest *CreateRequest
	createResult  *iaas.NFS
	createErr     error
	updateZone    string
	updateRequest *UpdateRequest
	updateResult  *iaas.NFS
	updateErr     error
	deleteZone    string
	deleteRequest *DeleteRequest
	deleteErr     error
	bootZone      string
	bootRequest   *BootRequest
	bootErr       error
	shutdownZone  string
	shutdownReq   *ShutdownRequest
	shutdownErr   error
	resetZone     string
	resetRequest  *ResetRequest
	resetErr      error
	cpuZone       string
	cpuRequest    *MonitorCPURequest
	cpuValues     []*iaas.MonitorCPUTimeValue
	cpuErr        error
	diskZone      string
	diskRequest   *MonitorFreeDiskSizeRequest
	diskValues    []*iaas.MonitorFreeDiskSizeValue
	diskErr       error
	ifaceZone     string
	ifaceRequest  *MonitorInterfaceRequest
	ifaceValues   []*iaas.MonitorInterfaceValue
	ifaceErr      error
}

func (f *fakeNFSAPI) FindWithContext(_ context.Context, request *FindRequest) ([]*iaas.NFS, error) {
	f.findZone = request.Zone
	f.findRequest = request
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findResult, nil
}

func (f *fakeNFSAPI) ReadWithContext(_ context.Context, request *ReadRequest) (*iaas.NFS, error) {
	f.readZone = request.Zone
	f.readRequest = request
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.readResult, nil
}

func (f *fakeNFSAPI) CreateWithContext(_ context.Context, request *CreateRequest) (*iaas.NFS, error) {
	f.createZone = request.Zone
	f.createRequest = request
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResult, nil
}

func (f *fakeNFSAPI) UpdateWithContext(_ context.Context, request *UpdateRequest) (*iaas.NFS, error) {
	f.updateZone = request.Zone
	f.updateRequest = request
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResult, nil
}

func (f *fakeNFSAPI) DeleteWithContext(_ context.Context, request *DeleteRequest) error {
	f.deleteZone = request.Zone
	f.deleteRequest = request
	return f.deleteErr
}

func (f *fakeNFSAPI) BootWithContext(_ context.Context, request *BootRequest) error {
	f.bootZone = request.Zone
	f.bootRequest = request
	return f.bootErr
}

func (f *fakeNFSAPI) ShutdownWithContext(_ context.Context, request *ShutdownRequest) error {
	f.shutdownZone = request.Zone
	f.shutdownReq = request
	return f.shutdownErr
}

func (f *fakeNFSAPI) ResetWithContext(_ context.Context, request *ResetRequest) error {
	f.resetZone = request.Zone
	f.resetRequest = request
	return f.resetErr
}

func (f *fakeNFSAPI) MonitorCPUWithContext(_ context.Context, request *MonitorCPURequest) ([]*iaas.MonitorCPUTimeValue, error) {
	f.cpuZone = request.Zone
	f.cpuRequest = request
	if f.cpuErr != nil {
		return nil, f.cpuErr
	}
	return f.cpuValues, nil
}

func (f *fakeNFSAPI) MonitorFreeDiskSizeWithContext(_ context.Context, request *MonitorFreeDiskSizeRequest) ([]*iaas.MonitorFreeDiskSizeValue, error) {
	f.diskZone = request.Zone
	f.diskRequest = request
	if f.diskErr != nil {
		return nil, f.diskErr
	}
	return f.diskValues, nil
}

func (f *fakeNFSAPI) MonitorInterfaceWithContext(_ context.Context, request *MonitorInterfaceRequest) ([]*iaas.MonitorInterfaceValue, error) {
	f.ifaceZone = request.Zone
	f.ifaceRequest = request
	if f.ifaceErr != nil {
		return nil, f.ifaceErr
	}
	return f.ifaceValues, nil
}

// testRuntime は出力処理をスタブに差し替えた Runtime です。
func testRuntime() Runtime {
	return Runtime{
		DecodeRequest:          decodeTestRequest,
		ValidateOutput:         func(*kong.Context) error { return nil },
		WriteOutput:            writeTestOutput,
		TransformCreateRequest: TransformCreateRequest,
	}
}

func writeTestOutput(ctx *kong.Context, value any) error {
	encoder := json.NewEncoder(ctx.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
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

func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNFSCreateCommandRunWithFlags(t *testing.T) {
	fake := &fakeNFSAPI{createResult: &iaas.NFS{ID: 123, Name: "nfs-01"}}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	kongCtx := parseCommand(t, command,
		"--zone", "is1b", "--name", "nfs-01",
		"--plan-id", "100001", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11",
		"--network-mask-len", "24",
		"--default-route", "192.168.0.1",
		"--description", "desc", "--tags", "production,managed",
		"--icon-id", "112300012345",
	)
	require.NoError(t, command.Run(kongCtx))
	require.Equal(t, "is1b", fake.createZone)
	request := fake.createRequest
	require.Equal(t, "nfs-01", request.Name)
	require.EqualValues(t, 100001, request.PlanID)
	require.EqualValues(t, 112900500001, request.SwitchID)
	require.Equal(t, []string{"192.168.0.11"}, request.IPAddresses)
	require.Equal(t, 24, request.NetworkMaskLen)
	require.Equal(t, "192.168.0.1", request.DefaultRoute)
	require.Equal(t, "desc", request.Description)
	require.Equal(t, types.Tags{"production", "managed"}, request.Tags)
	require.EqualValues(t, 112300012345, request.IconID)
}

func TestNFSCreateCommandRunWithTwoIPAddresses(t *testing.T) {
	fake := &fakeNFSAPI{createResult: &iaas.NFS{ID: 123}}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	kongCtx := parseCommand(t, command,
		"--zone", "is1b", "--name", "nfs-02",
		"--plan-id", "100001", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11", "--ip-address", "192.168.0.12",
		"--network-mask-len", "24",
	)
	require.NoError(t, command.Run(kongCtx))
	require.Equal(t, []string{"192.168.0.11", "192.168.0.12"}, fake.createRequest.IPAddresses)
}

func TestNFSCreateCommandRequestJSON(t *testing.T) {
	fake := &fakeNFSAPI{createResult: &iaas.NFS{ID: 123}}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	kongCtx := parseCommand(t, command, "--request", `{"Zone":"is1b","Name":"nfs-03","PlanID":100001,"SwitchID":112900500001,"IPAddresses":["192.168.0.11","192.168.0.12"],"NetworkMaskLen":24}`)
	require.NoError(t, command.Run(kongCtx))
	require.Equal(t, "is1b", fake.createZone)
	request := fake.createRequest
	require.Equal(t, "nfs-03", request.Name)
	require.EqualValues(t, 100001, request.PlanID)
	require.EqualValues(t, 112900500001, request.SwitchID)
	require.Equal(t, []string{"192.168.0.11", "192.168.0.12"}, request.IPAddresses)
	require.Equal(t, 24, request.NetworkMaskLen)
}

func TestNFSCreateCommandRequestFile(t *testing.T) {
	fake := &fakeNFSAPI{createResult: &iaas.NFS{ID: 123}}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	path := writeRequestFile(t, `{"Zone":"is1b","Name":"nfs-04","PlanID":100001,"SwitchID":112900500001,"IPAddresses":["192.168.0.11"],"NetworkMaskLen":24}`)
	kongCtx := parseCommand(t, command, "--request", "@"+path)
	require.NoError(t, command.Run(kongCtx))
	require.Equal(t, "nfs-04", fake.createRequest.Name)
}

func TestNFSCreateCommandRejectsInvalidInput(t *testing.T) {
	t.Run("missing ip address flag", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "nfs-05",
			"--plan-id", "100001", "--switch-id", "112900500001",
			"--network-mask-len", "24",
		)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--ip-address が必要です")
	})

	t.Run("three ip addresses", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "nfs-06",
			"--plan-id", "100001", "--switch-id", "112900500001",
			"--ip-address", "192.168.0.11", "--ip-address", "192.168.0.12", "--ip-address", "192.168.0.13",
			"--network-mask-len", "24",
		)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--ip-address は2個まで指定できます")
	})

	t.Run("invalid ip address", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "nfs-07",
			"--plan-id", "100001", "--switch-id", "112900500001",
			"--ip-address", "not-an-ip",
			"--network-mask-len", "24",
		)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "IPv4 アドレスで指定してください")
	})

	t.Run("request and flags are exclusive", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--request", `{"Zone":"is1b","Name":"nfs-08","PlanID":100001,"SwitchID":112900500001,"IPAddresses":["192.168.0.11"],"NetworkMaskLen":24}`,
			"--name", "flag-wins",
		)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request と個別フラグは併用できません")
	})

	t.Run("invalid request json", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", `{"Zone":}`)
		err := command.Run(kongCtx)
		require.Error(t, err)
	})
}

func TestNFSCreateCommandPropagatesAPIErrors(t *testing.T) {
	fake := &fakeNFSAPI{createErr: errors.New("create failed")}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	kongCtx := parseCommand(t, command,
		"--zone", "is1b", "--name", "nfs-09",
		"--plan-id", "100001", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11",
		"--network-mask-len", "24",
	)
	err := command.Run(kongCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create failed")
}

func TestNFSUpdateCommandRun(t *testing.T) {
	t.Run("flags", func(t *testing.T) {
		fake := &fakeNFSAPI{updateResult: &iaas.NFS{ID: 123, Name: "updated-nfs"}}
		command := &UpdateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--id", "123",
			"--name", "updated-nfs", "--description", "updated",
			"--tags", "updated,manual", "--icon-id", "112300012345",
		)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.updateZone)
		request := fake.updateRequest
		require.EqualValues(t, 123, request.ID)
		require.NotNil(t, request.Name)
		require.Equal(t, "updated-nfs", *request.Name)
		require.NotNil(t, request.Description)
		require.Equal(t, "updated", *request.Description)
		require.NotNil(t, request.Tags)
		require.Equal(t, types.Tags{"updated", "manual"}, *request.Tags)
		require.NotNil(t, request.IconID)
		require.EqualValues(t, 112300012345, *request.IconID)
	})

	t.Run("request json", func(t *testing.T) {
		fake := &fakeNFSAPI{updateResult: &iaas.NFS{ID: 123, Name: "updated-nfs"}}
		command := &UpdateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", `{"Zone":"is1b","ID":123,"Name":"updated-nfs"}`)
		require.NoError(t, command.Run(kongCtx))
		request := fake.updateRequest
		require.NotNil(t, request.Name)
		require.Equal(t, "updated-nfs", *request.Name)
		// JSON で未指定の項目は nil ポインタのまま API 実装へ渡ります
		require.Nil(t, request.Description)
		require.Nil(t, request.Tags)
	})
}

func TestNFSSimpleCommandsRequestJSON(t *testing.T) {
	findFile := writeRequestFile(t, `{"Zone":"is1b","Names":["example"],"Tags":["tag1"],"Count":10,"From":20}`)
	readFile := writeRequestFile(t, `{"Zone":"is1b","ID":123}`)
	deleteFile := writeRequestFile(t, `{"Zone":"is1b","ID":123,"FailIfNotFound":true,"Force":true}`)
	bootFile := writeRequestFile(t, `{"Zone":"is1b","ID":123,"NoWait":true}`)
	shutdownFile := writeRequestFile(t, `{"Zone":"is1b","ID":123,"NoWait":true,"ForceShutdown":true}`)
	resetFile := writeRequestFile(t, `{"Zone":"is1b","ID":123}`)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	monitorFile := writeRequestFile(t, fmt.Sprintf(`{"Zone":"is1b","ID":123,"Start":%q,"End":%q}`, start.Format(time.RFC3339), end.Format(time.RFC3339)))

	t.Run("find", func(t *testing.T) {
		fake := &fakeNFSAPI{findResult: []*iaas.NFS{{ID: 123, Name: "nfs-01"}}}
		command := &FindCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+findFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.findZone)
		require.Equal(t, []string{"example"}, fake.findRequest.Names)
		require.Equal(t, []string{"tag1"}, fake.findRequest.Tags)
		require.Equal(t, 10, fake.findRequest.Count)
		require.Equal(t, 20, fake.findRequest.From)
	})

	t.Run("read", func(t *testing.T) {
		fake := &fakeNFSAPI{readResult: &iaas.NFS{ID: 123, Name: "nfs-01"}}
		command := &ReadCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+readFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.readZone)
		require.EqualValues(t, 123, fake.readRequest.ID)
	})

	t.Run("delete", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &DeleteCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+deleteFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.deleteZone)
		require.True(t, fake.deleteRequest.FailIfNotFound)
		require.True(t, fake.deleteRequest.Force)
	})

	t.Run("boot", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &BootCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+bootFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.bootZone)
		require.True(t, fake.bootRequest.NoWait)
	})

	t.Run("shutdown", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &ShutdownCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+shutdownFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.shutdownZone)
		require.True(t, fake.shutdownReq.NoWait)
		require.True(t, fake.shutdownReq.ForceShutdown)
	})

	t.Run("reset", func(t *testing.T) {
		fake := &fakeNFSAPI{}
		command := &ResetCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+resetFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.resetZone)
		require.EqualValues(t, 123, fake.resetRequest.ID)
	})

	t.Run("monitor cpu", func(t *testing.T) {
		fake := &fakeNFSAPI{cpuValues: []*iaas.MonitorCPUTimeValue{{CPUTime: 10}}}
		command := &MonitorCPUCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+monitorFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.cpuZone)
		require.Equal(t, start, fake.cpuRequest.Start)
		require.Equal(t, end, fake.cpuRequest.End)
	})

	t.Run("monitor free disk size", func(t *testing.T) {
		fake := &fakeNFSAPI{diskValues: []*iaas.MonitorFreeDiskSizeValue{{FreeDiskSize: 100}}}
		command := &MonitorFreeDiskSizeCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+monitorFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.diskZone)
		require.Equal(t, start, fake.diskRequest.Start)
		require.Equal(t, end, fake.diskRequest.End)
	})

	t.Run("monitor interface", func(t *testing.T) {
		fake := &fakeNFSAPI{ifaceValues: []*iaas.MonitorInterfaceValue{{Receive: 10}}}
		command := &MonitorInterfaceCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--request", "@"+monitorFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.ifaceZone)
		require.Equal(t, start, fake.ifaceRequest.Start)
		require.Equal(t, end, fake.ifaceRequest.End)
	})
}
