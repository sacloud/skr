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

package databaseapi

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
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// fakeDatabaseOp は iaas.DatabaseAPI を満たすテスト用実装です。
// 削除ワークフローの操作順序と失敗時の削除抑止を検証します。
type fakeDatabaseOp struct {
	database    *iaas.Database
	readErr     error
	shutdownErr error
	deleteErr   error
	operations  []string
}

func (f *fakeDatabaseOp) record(operation string) {
	f.operations = append(f.operations, operation)
}

func (f *fakeDatabaseOp) Find(context.Context, string, *iaas.FindCondition) (*iaas.DatabaseFindResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) Create(context.Context, string, *iaas.DatabaseCreateRequest) (*iaas.Database, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) Read(context.Context, string, types.ID) (*iaas.Database, error) {
	f.record("read")
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.database, nil
}

func (f *fakeDatabaseOp) Update(context.Context, string, types.ID, *iaas.DatabaseUpdateRequest) (*iaas.Database, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) UpdateSettings(context.Context, string, types.ID, *iaas.DatabaseUpdateSettingsRequest) (*iaas.Database, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) Delete(_ context.Context, _ string, _ types.ID) error {
	f.record("delete")
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.database = nil
	return nil
}

func (f *fakeDatabaseOp) Config(context.Context, string, types.ID) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseOp) Boot(context.Context, string, types.ID) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseOp) Shutdown(_ context.Context, _ string, _ types.ID, _ *iaas.ShutdownOption) error {
	f.record("shutdown")
	if f.shutdownErr != nil {
		return f.shutdownErr
	}
	// 強制終了を受け付けた状態を模擬する
	if f.database != nil {
		f.database.InstanceStatus = types.ServerInstanceStatuses.Down
	}
	return nil
}

func (f *fakeDatabaseOp) Reset(context.Context, string, types.ID) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseOp) MonitorCPU(context.Context, string, types.ID, *iaas.MonitorCondition) (*iaas.CPUTimeActivity, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) MonitorDisk(context.Context, string, types.ID, *iaas.MonitorCondition) (*iaas.DiskActivity, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) MonitorInterface(context.Context, string, types.ID, *iaas.MonitorCondition) (*iaas.InterfaceActivity, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) MonitorDatabase(context.Context, string, types.ID, *iaas.MonitorCondition) (*iaas.DatabaseActivity, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) Status(context.Context, string, types.ID) (*iaas.DatabaseStatus, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) GetParameter(context.Context, string, types.ID) (*iaas.DatabaseParameter, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseOp) SetParameter(context.Context, string, types.ID, map[string]any) error {
	return errors.New("not implemented")
}

// speedUpWaiters は待ち処理のポーリング間隔をテスト用に短くします。戻り値で元の値を復元します。
func speedUpWaiters() func() {
	originalInterval := defaults.DefaultStatePollingInterval
	originalShutdownSpan := power.ShutdownRetrySpan
	originalInitialTimeout := power.InitialRequestTimeout
	originalInitialSpan := power.InitialRequestRetrySpan
	defaults.DefaultStatePollingInterval = time.Millisecond
	power.ShutdownRetrySpan = time.Millisecond
	power.InitialRequestTimeout = time.Second
	power.InitialRequestRetrySpan = time.Millisecond
	return func() {
		defaults.DefaultStatePollingInterval = originalInterval
		power.ShutdownRetrySpan = originalShutdownSpan
		power.InitialRequestTimeout = originalInitialTimeout
		power.InitialRequestRetrySpan = originalInitialSpan
	}
}

// notFoundError は API の 404 エラーを模擬します。
func notFoundError() error {
	return iaas.NewAPIError(http.MethodGet, &url.URL{Path: "/appliance/999"}, http.StatusNotFound, &iaas.APIErrorResponse{})
}

func TestDeleteWorkflowRejectsRunningDatabaseWithoutForce(t *testing.T) {
	op := &fakeDatabaseOp{database: &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &databaseAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123, FailIfNotFound: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "has not yet shut down")
	require.Equal(t, []string{"read"}, op.operations, "delete must not be called for a running database without --force")
}

func TestDeleteWorkflowForcesShutdownAndDeletesRunningDatabase(t *testing.T) {
	restore := speedUpWaiters()
	defer restore()

	op := &fakeDatabaseOp{database: &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up}}
	api := &databaseAPI{op: op}

	require.NoError(t, api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123, Force: true, FailIfNotFound: true}))

	// 起動中のデータベースは強制終了して停止を待ち、その後削除します
	require.Equal(t, []string{"read", "shutdown", "read", "delete"}, op.operations)
}

func TestDeleteWorkflowDeletesStoppedDatabaseWithoutForce(t *testing.T) {
	op := &fakeDatabaseOp{database: &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down}}
	api := &databaseAPI{op: op}

	require.NoError(t, api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123, FailIfNotFound: true}))

	require.Equal(t, []string{"read", "delete"}, op.operations)
}

func TestDeleteWorkflowToleratesMissingDatabaseWithoutFailIfNotFound(t *testing.T) {
	op := &fakeDatabaseOp{readErr: notFoundError()}
	api := &databaseAPI{op: op}

	require.NoError(t, api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 999}))

	require.Equal(t, []string{"read"}, op.operations, "delete must not be called when the target is missing")
}

func TestDeleteWorkflowFailsOnMissingDatabaseWithFailIfNotFound(t *testing.T) {
	op := &fakeDatabaseOp{readErr: notFoundError()}
	api := &databaseAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 999, FailIfNotFound: true})

	require.Error(t, err)
	require.Equal(t, []string{"read"}, op.operations)
}

func TestDeleteWorkflowKeepsDatabaseWhenShutdownFails(t *testing.T) {
	restore := speedUpWaiters()
	defer restore()

	op := &fakeDatabaseOp{
		database:    &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Up},
		shutdownErr: errors.New("shutdown failed"),
	}
	api := &databaseAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123, Force: true})

	require.Error(t, err)
	require.Equal(t, []string{"read", "shutdown"}, op.operations, "delete must not be called when the shutdown fails")
}

func TestDeleteWorkflowToleratesMissingDatabaseOnDeleteWithoutFailIfNotFound(t *testing.T) {
	op := &fakeDatabaseOp{
		database:  &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down},
		deleteErr: notFoundError(),
	}
	api := &databaseAPI{op: op}

	require.NoError(t, api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123}))
}

func TestDeleteWorkflowFailsOnMissingDatabaseOnDeleteWithFailIfNotFound(t *testing.T) {
	op := &fakeDatabaseOp{
		database:  &iaas.Database{ID: 123, Availability: types.Availabilities.Available, InstanceStatus: types.ServerInstanceStatuses.Down},
		deleteErr: notFoundError(),
	}
	api := &databaseAPI{op: op}

	err := api.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1c", ID: 123, FailIfNotFound: true})

	require.Error(t, err)
}
