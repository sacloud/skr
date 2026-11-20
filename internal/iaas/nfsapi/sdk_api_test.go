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
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/defaults"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/power"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// fakeNFSOp は iaas.NFSAPI を満たすテスト用実装です。
// 操作の引数とワークフローの操作順序を検証します。
type fakeNFSOp struct {
	nfs         *iaas.NFS
	readErr     error
	readCount   int
	shutdownErr error
	deleteErr   error
	operations  []string

	findZone      string
	findCondition *iaas.FindCondition
	findResult    *iaas.NFSFindResult
	findErr       error

	createZone  string
	createParam *iaas.NFSCreateRequest
	createErr   error

	updateZone  string
	updateID    types.ID
	updateParam *iaas.NFSUpdateRequest
	updateErr   error

	bootErr  error
	resetErr error

	monitorCondition *iaas.MonitorCondition
	cpuValues        *iaas.CPUTimeActivity
	cpuErr           error
	diskValues       *iaas.FreeDiskSizeActivity
	diskErr          error
	ifaceValues      *iaas.InterfaceActivity
	ifaceErr         error
}

func (f *fakeNFSOp) record(operation string) {
	f.operations = append(f.operations, operation)
}

func (f *fakeNFSOp) Find(_ context.Context, zone string, conditions *iaas.FindCondition) (*iaas.NFSFindResult, error) {
	f.record("find")
	f.findZone = zone
	f.findCondition = conditions
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findResult, nil
}

func (f *fakeNFSOp) Create(_ context.Context, zone string, param *iaas.NFSCreateRequest) (*iaas.NFS, error) {
	f.record("create")
	f.createZone = zone
	f.createParam = param
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &iaas.NFS{ID: 123, Name: param.Name}, nil
}

func (f *fakeNFSOp) Read(context.Context, string, types.ID) (*iaas.NFS, error) {
	f.record("read")
	f.readCount++
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.nfs, nil
}

func (f *fakeNFSOp) Update(_ context.Context, zone string, id types.ID, param *iaas.NFSUpdateRequest) (*iaas.NFS, error) {
	f.record("update")
	f.updateZone = zone
	f.updateID = id
	f.updateParam = param
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &iaas.NFS{ID: id, Name: param.Name}, nil
}

func (f *fakeNFSOp) Delete(_ context.Context, _ string, _ types.ID) error {
	f.record("delete")
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.nfs = nil
	return nil
}

func (f *fakeNFSOp) Boot(context.Context, string, types.ID) error {
	f.record("boot")
	if f.bootErr != nil {
		return f.bootErr
	}
	return nil
}

func (f *fakeNFSOp) Shutdown(_ context.Context, _ string, _ types.ID, _ *iaas.ShutdownOption) error {
	f.record("shutdown")
	if f.shutdownErr != nil {
		return f.shutdownErr
	}
	// 強制終了を受け付けた状態を模擬する
	if f.nfs != nil {
		f.nfs.InstanceStatus = types.ServerInstanceStatuses.Down
	}
	return nil
}

func (f *fakeNFSOp) Reset(context.Context, string, types.ID) error {
	f.record("reset")
	if f.resetErr != nil {
		return f.resetErr
	}
	return nil
}

func (f *fakeNFSOp) MonitorCPU(_ context.Context, _ string, _ types.ID, condition *iaas.MonitorCondition) (*iaas.CPUTimeActivity, error) {
	f.record("monitor-cpu")
	f.monitorCondition = condition
	if f.cpuErr != nil {
		return nil, f.cpuErr
	}
	return f.cpuValues, nil
}

func (f *fakeNFSOp) MonitorFreeDiskSize(_ context.Context, _ string, _ types.ID, condition *iaas.MonitorCondition) (*iaas.FreeDiskSizeActivity, error) {
	f.record("monitor-free-disk-size")
	f.monitorCondition = condition
	if f.diskErr != nil {
		return nil, f.diskErr
	}
	return f.diskValues, nil
}

func (f *fakeNFSOp) MonitorInterface(_ context.Context, _ string, _ types.ID, condition *iaas.MonitorCondition) (*iaas.InterfaceActivity, error) {
	f.record("monitor-interface")
	f.monitorCondition = condition
	if f.ifaceErr != nil {
		return nil, f.ifaceErr
	}
	return f.ifaceValues, nil
}

// speedUpWaiters は待ち処理のポーリング間隔とタイムアウトをテスト用に短くします。戻り値で元の値を復元します。
func speedUpWaiters() func() {
	originalInterval := defaults.DefaultStatePollingInterval
	originalTimeout := defaults.DefaultStatePollingTimeout
	originalShutdownSpan := power.ShutdownRetrySpan
	originalInitialTimeout := power.InitialRequestTimeout
	originalInitialSpan := power.InitialRequestRetrySpan
	defaults.DefaultStatePollingInterval = time.Millisecond
	defaults.DefaultStatePollingTimeout = 50 * time.Millisecond
	power.ShutdownRetrySpan = time.Millisecond
	power.InitialRequestTimeout = time.Second
	power.InitialRequestRetrySpan = time.Millisecond
	return func() {
		defaults.DefaultStatePollingInterval = originalInterval
		defaults.DefaultStatePollingTimeout = originalTimeout
		power.ShutdownRetrySpan = originalShutdownSpan
		power.InitialRequestTimeout = originalInitialTimeout
		power.InitialRequestRetrySpan = originalInitialSpan
	}
}

// notFoundError は API の 404 エラーを模擬します。
func notFoundError() error {
	return iaas.NewAPIError(http.MethodGet, &url.URL{Path: "/appliance/999"}, http.StatusNotFound, &iaas.APIErrorResponse{})
}

func TestFindWithContextBuildsCondition(t *testing.T) {
	op := &fakeNFSOp{findResult: &iaas.NFSFindResult{NFS: []*iaas.NFS{{ID: 123}}}}
	api := &nfsAPI{op: op}

	found, err := api.FindWithContext(context.Background(), &FindRequest{
		Zone:  "is1b",
		Names: []string{"example"},
		Tags:  []string{"tag1", "tag2"},
		Count: 5,
		From:  10,
	})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "is1b", op.findZone)
	require.Equal(t, search.AndEqual("example"), op.findCondition.Filter[search.Key("Name")])
	require.Equal(t, search.TagsAndEqual("tag1", "tag2"), op.findCondition.Filter[search.Key("Tags.Name")])
	require.Equal(t, 5, op.findCondition.Count)
	require.Equal(t, 10, op.findCondition.From)
}

func TestFindWithContextOmittedConditions(t *testing.T) {
	op := &fakeNFSOp{findResult: &iaas.NFSFindResult{}}
	api := &nfsAPI{op: op}

	_, err := api.FindWithContext(context.Background(), &FindRequest{Zone: "is1b"})
	require.NoError(t, err)
	require.Empty(t, op.findCondition.Filter)
	require.Equal(t, 0, op.findCondition.Count)
	require.Equal(t, 0, op.findCondition.From)
}

func TestFindWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{findErr: errors.New("find failed")}
	api := &nfsAPI{op: op}

	_, err := api.FindWithContext(context.Background(), &FindRequest{Zone: "is1b"})
	require.Error(t, err)
}

func TestCreateWithContextConvertsRequest(t *testing.T) {
	op := &fakeNFSOp{}
	api := &nfsAPI{op: op}

	created, err := api.CreateWithContext(context.Background(), &CreateRequest{
		Zone:           "is1b",
		SwitchID:       112900500001,
		PlanID:         100001,
		IPAddresses:    []string{"192.168.0.11"},
		NetworkMaskLen: 24,
		DefaultRoute:   "192.168.0.1",
		Name:           "nfs-01",
		Description:    "desc",
		Tags:           types.Tags{"tag1"},
		IconID:         123,
	})
	require.NoError(t, err)
	require.Equal(t, "is1b", op.createZone)
	require.Equal(t, types.ID(112900500001), op.createParam.SwitchID)
	require.EqualValues(t, 100001, op.createParam.PlanID)
	require.Equal(t, []string{"192.168.0.11"}, op.createParam.IPAddresses)
	require.Equal(t, 24, op.createParam.NetworkMaskLen)
	require.Equal(t, "192.168.0.1", op.createParam.DefaultRoute)
	require.Equal(t, "nfs-01", op.createParam.Name)
	require.Equal(t, "desc", op.createParam.Description)
	require.Equal(t, types.Tags{"tag1"}, op.createParam.Tags)
	require.EqualValues(t, 123, op.createParam.IconID)
	require.Equal(t, types.ID(123), created.ID)
}

func TestCreateWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{createErr: errors.New("create failed")}
	api := &nfsAPI{op: op}

	_, err := api.CreateWithContext(context.Background(), &CreateRequest{Zone: "is1b"})
	require.Error(t, err)
}

func TestTransformCreateRequestValidatesIPAddresses(t *testing.T) {
	t.Run("single ipv4", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{IPAddresses: []string{"192.168.0.11"}}
		require.NoError(t, TransformCreateRequest(command, request))
		require.Equal(t, []string{"192.168.0.11"}, request.IPAddresses)
	})

	t.Run("two ipv4 addresses", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{IPAddresses: []string{"192.168.0.11", "192.168.0.12"}}
		require.NoError(t, TransformCreateRequest(command, request))
		require.Equal(t, []string{"192.168.0.11", "192.168.0.12"}, request.IPAddresses)
	})

	t.Run("missing ip address", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{}
		err := TransformCreateRequest(command, request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--ip-address が必要です")
	})

	t.Run("three ip addresses", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{IPAddresses: []string{"192.168.0.11", "192.168.0.12", "192.168.0.13"}}
		err := TransformCreateRequest(command, request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--ip-address は2個まで指定できます")
	})

	t.Run("ipv6 address", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{IPAddresses: []string{"2001:db8::1"}}
		err := TransformCreateRequest(command, request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "IPv4 アドレスで指定してください")
	})

	t.Run("not an address", func(t *testing.T) {
		request := &CreateRequest{}
		command := &CreateCommand{IPAddresses: []string{"not-an-ip"}}
		err := TransformCreateRequest(command, request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "IPv4 アドレスで指定してください")
	})
}

func TestUpdateWithContextConvertsRequest(t *testing.T) {
	op := &fakeNFSOp{}
	api := &nfsAPI{op: op}

	name := "updated-nfs"
	description := "updated desc"
	tags := types.Tags{"updated"}
	iconID := types.ID(123)
	updated, err := api.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone:        "is1b",
		ID:          123,
		Name:        &name,
		Description: &description,
		Tags:        &tags,
		IconID:      &iconID,
	})
	require.NoError(t, err)
	require.Equal(t, "is1b", op.updateZone)
	require.Equal(t, types.ID(123), op.updateID)
	require.Equal(t, "updated-nfs", op.updateParam.Name)
	require.Equal(t, "updated desc", op.updateParam.Description)
	require.Equal(t, types.Tags{"updated"}, op.updateParam.Tags)
	require.EqualValues(t, 123, op.updateParam.IconID)
	require.Equal(t, "updated-nfs", updated.Name)
}

func TestUpdateWithContextOmittedFields(t *testing.T) {
	op := &fakeNFSOp{}
	api := &nfsAPI{op: op}

	_, err := api.UpdateWithContext(context.Background(), &UpdateRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	// 未指定の項目は空の値として API の更新リクエストへ送信します
	require.Equal(t, "", op.updateParam.Name)
	require.Equal(t, "", op.updateParam.Description)
	require.Empty(t, op.updateParam.Tags)
	require.EqualValues(t, 0, op.updateParam.IconID)
}

func TestUpdateWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{updateErr: errors.New("update failed")}
	api := &nfsAPI{op: op}

	_, err := api.UpdateWithContext(context.Background(), &UpdateRequest{Zone: "is1b", ID: 123})
	require.Error(t, err)
}

func TestDeleteWorkflowRejectsRunningNFSWithoutForce(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123})
	require.Error(t, err)
	require.Contains(t, err.Error(), "has not yet shut down")
	// 起動中でも Force がなければ停止も削除もしない
	require.Equal(t, []string{"read"}, op.operations)
}

func TestDeleteWorkflowForcesShutdownAndDeletesRunningNFS(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123, Force: true})
	require.NoError(t, err)
	require.Equal(t, []string{"read", "shutdown", "read", "delete"}, op.operations)
}

func TestDeleteWorkflowDeletesStoppedNFSWithoutForce(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down}}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Equal(t, []string{"read", "read", "delete"}, op.operations)
}

func TestDeleteWorkflowDeletesUnknownStatusNFSWithoutForce(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Unknown}}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Equal(t, []string{"read", "delete"}, op.operations)
}

func TestDeleteWorkflowToleratesMissingNFSWithoutFailIfNotFound(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{readErr: notFoundError()}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 999})
	require.NoError(t, err)
	require.Equal(t, []string{"read"}, op.operations)
}

func TestDeleteWorkflowFailsOnMissingNFSWithFailIfNotFound(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{readErr: notFoundError()}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 999, FailIfNotFound: true})
	require.Error(t, err)
	var apiErr iaas.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.ResponseCode(), "error response code")
}

func TestDeleteWorkflowKeepsNFSWhenShutdownFails(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{
		nfs:         &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up},
		shutdownErr: errors.New("shutdown failed"),
	}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123, Force: true})
	require.Error(t, err)
	require.Equal(t, []string{"read", "shutdown"}, op.operations)
}

func TestDeleteWorkflowToleratesMissingNFSOnDeleteWithoutFailIfNotFound(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{
		nfs:       &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down},
		deleteErr: notFoundError(),
	}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
}

func TestDeleteWorkflowFailsOnMissingNFSOnDeleteWithFailIfNotFound(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{
		nfs:       &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down},
		deleteErr: notFoundError(),
	}
	api := &nfsAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123, FailIfNotFound: true})
	require.Error(t, err)
}

func TestBootWithContextWaitsByDefault(t *testing.T) {
	defer speedUpWaiters()()
	// 起動後の状態を段階的に変化させ、起動完了待ちが行われることを確認します
	op := &fakeNFSOp{
		nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down},
	}
	api := &nfsAPI{op: op}
	go func() {
		time.Sleep(10 * time.Millisecond)
		op.nfs.InstanceStatus = types.ServerInstanceStatuses.Up
	}()

	err := api.BootWithContext(context.Background(), &BootRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Contains(t, op.operations, "boot")
	require.Equal(t, types.ServerInstanceStatuses.Up, op.nfs.InstanceStatus)
}

func TestBootWithContextNoWait(t *testing.T) {
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, InstanceStatus: types.ServerInstanceStatuses.Down}}
	api := &nfsAPI{op: op}

	err := api.BootWithContext(context.Background(), &BootRequest{Zone: "is1b", ID: 123, NoWait: true})
	require.NoError(t, err)
	require.Equal(t, []string{"boot"}, op.operations)
}

func TestBootWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{bootErr: errors.New("boot failed")}
	api := &nfsAPI{op: op}

	err := api.BootWithContext(context.Background(), &BootRequest{Zone: "is1b", ID: 123, NoWait: true})
	require.Error(t, err)
}

func TestShutdownWithContextWaitsByDefault(t *testing.T) {
	defer speedUpWaiters()()
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &nfsAPI{op: op}

	err := api.ShutdownWithContext(context.Background(), &ShutdownRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Contains(t, op.operations, "shutdown")
	require.Equal(t, types.ServerInstanceStatuses.Down, op.nfs.InstanceStatus)
}

func TestShutdownWithContextNoWaitWithForce(t *testing.T) {
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &nfsAPI{op: op}

	err := api.ShutdownWithContext(context.Background(), &ShutdownRequest{Zone: "is1b", ID: 123, NoWait: true, ForceShutdown: true})
	require.NoError(t, err)
	require.Equal(t, []string{"shutdown"}, op.operations)
}

func TestShutdownWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{shutdownErr: errors.New("shutdown failed")}
	api := &nfsAPI{op: op}

	err := api.ShutdownWithContext(context.Background(), &ShutdownRequest{Zone: "is1b", ID: 123, NoWait: true})
	require.Error(t, err)
}

func TestResetWithContext(t *testing.T) {
	op := &fakeNFSOp{}
	api := &nfsAPI{op: op}

	err := api.ResetWithContext(context.Background(), &ResetRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Equal(t, []string{"reset"}, op.operations)
}

func TestMonitorConditionComplementsPeriod(t *testing.T) {
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	start := end.Add(-1 * time.Hour)

	condition, err := monitorCondition(time.Time{}, end)
	require.NoError(t, err)
	// 終了のみ指定した場合は開始が終了の1時間前に補完されます
	require.Equal(t, start, condition.Start)
	require.Equal(t, end, condition.End)

	condition, err = monitorCondition(start, time.Time{})
	require.NoError(t, err)
	// 開始のみ指定した場合は終了が現在時刻に補完されます
	require.Equal(t, start, condition.Start)
	require.False(t, condition.End.IsZero())
}

func TestMonitorConditionRejectsInvalidPeriod(t *testing.T) {
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := monitorCondition(start, end)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is invalid")
}

func TestMonitorCPUWithContextReturnsValues(t *testing.T) {
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	start := end.Add(-1 * time.Hour)
	op := &fakeNFSOp{cpuValues: &iaas.CPUTimeActivity{Values: []*iaas.MonitorCPUTimeValue{{CPUTime: 10}}}}
	api := &nfsAPI{op: op}

	values, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1b", ID: 123, Start: start, End: end})
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, start, op.monitorCondition.Start)
	require.Equal(t, end, op.monitorCondition.End)
}

func TestMonitorCPUWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{cpuErr: errors.New("monitor failed")}
	api := &nfsAPI{op: op}

	_, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1b", ID: 123})
	require.Error(t, err)
}

func TestMonitorFreeDiskSizeWithContextReturnsValues(t *testing.T) {
	op := &fakeNFSOp{diskValues: &iaas.FreeDiskSizeActivity{Values: []*iaas.MonitorFreeDiskSizeValue{{FreeDiskSize: 100}}}}
	api := &nfsAPI{op: op}

	values, err := api.MonitorFreeDiskSizeWithContext(context.Background(), &MonitorFreeDiskSizeRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Len(t, values, 1)
}

func TestMonitorFreeDiskSizeWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{diskErr: errors.New("monitor failed")}
	api := &nfsAPI{op: op}

	_, err := api.MonitorFreeDiskSizeWithContext(context.Background(), &MonitorFreeDiskSizeRequest{Zone: "is1b", ID: 123})
	require.Error(t, err)
}

func TestMonitorInterfaceWithContextReturnsValues(t *testing.T) {
	op := &fakeNFSOp{ifaceValues: &iaas.InterfaceActivity{Values: []*iaas.MonitorInterfaceValue{{Receive: 10}}}}
	api := &nfsAPI{op: op}

	values, err := api.MonitorInterfaceWithContext(context.Background(), &MonitorInterfaceRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Len(t, values, 1)
}

func TestMonitorInterfaceWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeNFSOp{ifaceErr: errors.New("monitor failed")}
	api := &nfsAPI{op: op}

	_, err := api.MonitorInterfaceWithContext(context.Background(), &MonitorInterfaceRequest{Zone: "is1b", ID: 123})
	require.Error(t, err)
}

func TestReadWithContext(t *testing.T) {
	op := &fakeNFSOp{nfs: &iaas.NFS{ID: 123, Name: "nfs-01"}}
	api := &nfsAPI{op: op}

	found, err := api.ReadWithContext(context.Background(), &ReadRequest{Zone: "is1b", ID: 123})
	require.NoError(t, err)
	require.Equal(t, "nfs-01", found.Name)
}
