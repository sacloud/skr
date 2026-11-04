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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

func TestCreateUsesServerAndDiskAPIs(t *testing.T) {
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123}}
	diskOp := &diskAPIFake{createResult: &iaas.Disk{ID: 456}}
	sdk := &sdkAPI{op: serverOp, diskOp: diskOp}

	result, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:            "is1b",
		Name:            "test-server",
		CPU:             2,
		MemoryGB:        4,
		BootAfterCreate: true,
		Disks: []DiskRequest{{
			Name:            "test-server",
			DiskPlanID:      7,
			SizeGB:          20,
			SourceArchiveID: 99,
			Connection:      types.DiskConnections.VirtIO,
		}},
		NetworkInterfaces: []NetworkInterface{
			{Upstream: "shared"},
			{Upstream: "disconnected"},
			{Upstream: "321"},
		},
	})
	require.NoError(t, err)
	require.EqualValues(t, 123, result.ID)
	require.Equal(t, []string{"create", "boot"}, serverOp.calls)
	require.EqualValues(t, 4096, serverOp.createRequest.MemoryMB)
	require.Len(t, serverOp.createRequest.ConnectedSwitches, 3)
	require.Equal(t, types.Scopes.Shared, serverOp.createRequest.ConnectedSwitches[0].Scope)
	require.Nil(t, serverOp.createRequest.ConnectedSwitches[1])
	require.Equal(t, types.EScope(""), serverOp.createRequest.ConnectedSwitches[2].Scope)
	require.EqualValues(t, 321, serverOp.createRequest.ConnectedSwitches[2].ID)
	require.Equal(t, types.InterfaceDrivers.VirtIO, serverOp.createRequest.InterfaceDriver)
	require.EqualValues(t, 7, diskOp.createRequest.DiskPlanID)
	require.EqualValues(t, 123, diskOp.createRequest.ServerID)
	require.EqualValues(t, 99, diskOp.createRequest.SourceArchiveID)
	require.EqualValues(t, 20480, diskOp.createRequest.SizeMB)
}

func TestCreateSerializesNICsAsServerAPIRequests(t *testing.T) {
	caller := &serverAPICallerFake{}
	sdk := NewAPI(caller)

	_, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:     "is1b",
		Name:     "test-server",
		CPU:      1,
		MemoryGB: 1,
		NetworkInterfaces: []NetworkInterface{
			{Upstream: "shared"},
			{Upstream: "disconnected"},
			{Upstream: "321"},
		},
	})
	require.NoError(t, err)
	var body struct {
		Server struct {
			InterfaceDriver   string            `json:"InterfaceDriver"`
			ConnectedSwitches []json.RawMessage `json:"ConnectedSwitches"`
		} `json:"Server"`
	}
	require.NoError(t, json.Unmarshal(caller.body, &body))
	require.Equal(t, "virtio", body.Server.InterfaceDriver)
	require.Len(t, body.Server.ConnectedSwitches, 3)
	require.JSONEq(t, `{"Scope":"shared"}`, string(body.Server.ConnectedSwitches[0]))
	require.JSONEq(t, `null`, string(body.Server.ConnectedSwitches[1]))
	require.JSONEq(t, `{"ID":321}`, string(body.Server.ConnectedSwitches[2]))
}

func TestUpdatePlanOnlyDoesNotSendEmptyMetadataUpdate(t *testing.T) {
	serverOp := &serverAPIFake{readResult: &iaas.Server{ID: 123, CPU: 2, MemoryMB: 4096}}
	sdk := &sdkAPI{op: serverOp}
	cpu, memory := 2, 4

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone:     "is1b",
		ID:       123,
		CPU:      &cpu,
		MemoryGB: &memory,
	})
	require.NoError(t, err)
	require.Equal(t, 0, serverOp.updateCalls)
	require.Equal(t, 1, serverOp.changePlanCalls)
	require.EqualValues(t, 2, serverOp.changePlanRequest.CPU)
	require.EqualValues(t, 4096, serverOp.changePlanRequest.MemoryMB)
}

func TestUpdateDoesNotMutateWhenReadReturnsNoServer(t *testing.T) {
	serverOp := &serverAPIFake{}
	sdk := &sdkAPI{op: serverOp}
	name := "updated"

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		Name: &name,
	})
	require.ErrorContains(t, err, "読み込み結果が空")
	require.Zero(t, serverOp.updateCalls)
	require.Zero(t, serverOp.changePlanCalls)
}

func TestUpdatePreservesUnspecifiedFieldsInAPIRequest(t *testing.T) {
	caller := &serverAPICallerFake{}
	sdk := &sdkAPI{op: iaas.NewServerOp(caller)}
	name := "updated"

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		Name: &name,
	})
	require.NoError(t, err)
	var body struct {
		Server map[string]json.RawMessage
	}
	require.NoError(t, json.Unmarshal(caller.body, &body))
	require.JSONEq(t, `"updated"`, string(body.Server["Name"]))
	require.JSONEq(t, `"preserve me"`, string(body.Server["Description"]))
	require.JSONEq(t, `["keep"]`, string(body.Server["Tags"]))
	require.JSONEq(t, `{"ID":42,"Tags":[]}`, string(body.Server["Icon"]))
	require.JSONEq(t, `{"ID":43,"Description":"","Tags":[]}`, string(body.Server["PrivateHost"]))
	require.JSONEq(t, `"virtio"`, string(body.Server["InterfaceDriver"]))
}

func TestUpdatePreservesUnspecifiedPlanFieldsInAPIRequest(t *testing.T) {
	caller := &serverAPICallerFake{}
	sdk := &sdkAPI{op: iaas.NewServerOp(caller)}
	cpu := 8

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		CPU:  &cpu,
	})
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(caller.body, &body))
	require.JSONEq(t, `8`, string(body["CPU"]))
	require.JSONEq(t, `8192`, string(body["MemoryMB"]))
	require.JSONEq(t, `1`, string(body["GPU"]))
}

func TestCreateRejectsUnsupportedFieldsBeforeCallingAPI(t *testing.T) {
	serverOp := &serverAPIFake{}
	sdk := &sdkAPI{op: serverOp, diskOp: &diskAPIFake{}}
	cdromID := types.ID(456)

	_, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:    "is1b",
		Name:    "test-server",
		NoWait:  true,
		CDROMID: &cdromID,
	})
	require.ErrorContains(t, err, "NoWait")
	require.Empty(t, serverOp.calls)
}

func TestUpdateRejectsUnsupportedFieldsBeforeCallingAPI(t *testing.T) {
	serverOp := &serverAPIFake{}
	sdk := &sdkAPI{op: serverOp}

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		Disks: []DiskRequest{{
			DiskPlanID: 7,
			SizeGB:     20,
		}},
	})
	require.ErrorContains(t, err, "Disks")
	require.Equal(t, 0, serverOp.updateCalls)
	require.Equal(t, 0, serverOp.changePlanCalls)
}

func TestDeleteWithDisksUsesCurrentDiskIDs(t *testing.T) {
	serverOp := &serverAPIFake{
		readResults: []*iaas.Server{
			{
				ID:             123,
				InstanceStatus: types.ServerInstanceStatuses.Up,
				Disks:          []*iaas.ServerConnectedDisk{{ID: 456}, {ID: 789}},
			},
			{ID: 123, InstanceStatus: types.ServerInstanceStatuses.Down},
		},
	}
	sdk := &sdkAPI{op: serverOp}

	err := sdk.DeleteWithContext(context.Background(), &DeleteRequest{
		Zone:      "is1b",
		ID:        123,
		WithDisks: true,
		Force:     true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"read", "shutdown", "read", "delete-with-disks"}, serverOp.calls)
	require.Equal(t, []types.ID{456, 789}, serverOp.deleteWithDisksRequest.IDs)
}

func TestDeleteRejectsRunningServerWithoutForce(t *testing.T) {
	serverOp := &serverAPIFake{
		readResult: &iaas.Server{ID: 123, InstanceStatus: types.ServerInstanceStatuses.Up},
	}
	sdk := &sdkAPI{op: serverOp}

	err := sdk.DeleteWithContext(context.Background(), &DeleteRequest{Zone: "is1b", ID: 123})
	require.ErrorContains(t, err, "起動中")
	require.Equal(t, []string{"read"}, serverOp.calls)
}

type serverAPIFake struct {
	iaas.ServerAPI
	calls                  []string
	createRequest          *iaas.ServerCreateRequest
	createResult           *iaas.Server
	readResult             *iaas.Server
	readResults            []*iaas.Server
	changePlanRequest      *iaas.ServerChangePlanRequest
	deleteWithDisksRequest *iaas.ServerDeleteWithDisksRequest
	updateCalls            int
	changePlanCalls        int
}

func (api *serverAPIFake) Create(_ context.Context, _ string, request *iaas.ServerCreateRequest) (*iaas.Server, error) {
	api.calls = append(api.calls, "create")
	api.createRequest = request
	return api.createResult, nil
}

func (api *serverAPIFake) Boot(_ context.Context, _ string, _ types.ID) error {
	api.calls = append(api.calls, "boot")
	return nil
}

func (api *serverAPIFake) Update(_ context.Context, _ string, id types.ID, _ *iaas.ServerUpdateRequest) (*iaas.Server, error) {
	api.updateCalls++
	return &iaas.Server{ID: id}, nil
}

func (api *serverAPIFake) ChangePlan(_ context.Context, _ string, id types.ID, request *iaas.ServerChangePlanRequest) (*iaas.Server, error) {
	api.changePlanCalls++
	api.changePlanRequest = request
	return &iaas.Server{ID: id}, nil
}

func (api *serverAPIFake) Shutdown(_ context.Context, zone string, id types.ID, _ *iaas.ShutdownOption) error {
	api.calls = append(api.calls, "shutdown")
	return nil
}

func (api *serverAPIFake) Read(_ context.Context, _ string, _ types.ID) (*iaas.Server, error) {
	api.calls = append(api.calls, "read")
	if len(api.readResults) != 0 {
		result := api.readResults[0]
		api.readResults = api.readResults[1:]
		return result, nil
	}
	return api.readResult, nil
}

func (api *serverAPIFake) DeleteWithDisks(_ context.Context, _ string, _ types.ID, request *iaas.ServerDeleteWithDisksRequest) error {
	api.calls = append(api.calls, "delete-with-disks")
	api.deleteWithDisksRequest = request
	return nil
}

type diskAPIFake struct {
	iaas.DiskAPI
	createRequest *iaas.DiskCreateRequest
	createResult  *iaas.Disk
}

func (api *diskAPIFake) Create(_ context.Context, _ string, request *iaas.DiskCreateRequest, _ []types.ID, _ types.ID) (*iaas.Disk, error) {
	api.createRequest = request
	return api.createResult, nil
}

type serverAPICallerFake struct {
	body []byte
}

func (caller *serverAPICallerFake) Do(_ context.Context, _, _ string, body any) ([]byte, error) {
	if body != nil {
		var err error
		caller.body, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	return []byte(`{"Server":{"ID":123,"Name":"current","Description":"preserve me","Tags":["keep"],"Icon":{"ID":42},"PrivateHost":{"ID":43},"InterfaceDriver":"virtio","ServerPlan":{"CPU":2,"MemoryMB":8192,"GPU":1}}}`), nil
}
