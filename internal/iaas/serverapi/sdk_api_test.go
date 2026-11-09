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
	"fmt"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

func TestCreateUsesServerAndDiskAPIs(t *testing.T) {
	events := []string{}
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123}, events: &events}
	diskOp := &diskAPIFake{
		createResult: &iaas.Disk{ID: 456},
		readResults:  []*iaas.Disk{{ID: 456, ServerID: 123, Availability: types.Availabilities.Available}},
		events:       &events,
	}
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
	require.Equal(t, []string{"create", "read"}, diskOp.calls)
	require.Equal(t, []string{"server-create", "disk-create", "disk-read", "server-boot"}, events)
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

func TestFindPassesSortToAPI(t *testing.T) {
	serverOp := &serverAPIFake{}
	sdk := &sdkAPI{op: serverOp}
	sort := search.SortKeys{search.SortKeyDesc("CreatedAt")}
	var request FindRequest
	require.NoError(t, json.Unmarshal([]byte(`{"Zone":"is1b","Sort":[{"Key":"CreatedAt","Order":1}],"Count":10,"From":20}`), &request))

	_, err := sdk.FindWithContext(context.Background(), &request)

	require.NoError(t, err)
	require.NotNil(t, serverOp.findCondition)
	require.Equal(t, sort, serverOp.findCondition.Sort)
	require.Equal(t, 10, serverOp.findCondition.Count)
	require.Equal(t, 20, serverOp.findCondition.From)
}

func TestCreateWaitsForArchiveDiskBeforeBoot(t *testing.T) {
	events := []string{}
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123}, events: &events}
	diskOp := &diskAPIFake{
		createResult: &iaas.Disk{ID: 456},
		readResults: []*iaas.Disk{
			{ID: 456, ServerID: 123, Availability: types.Availabilities.Transferring},
			{ID: 456, ServerID: 123, Availability: types.Availabilities.Available},
		},
		events: &events,
	}
	sdk := &sdkAPI{op: serverOp, diskOp: diskOp}

	_, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:            "is1b",
		Name:            "test-server",
		CPU:             2,
		MemoryGB:        4,
		BootAfterCreate: true,
		Disks: []DiskRequest{{
			Name:            "test-disk",
			DiskPlanID:      7,
			SizeGB:          20,
			SourceArchiveID: 99,
			Connection:      types.DiskConnections.VirtIO,
		}},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"server-create", "disk-create", "disk-read", "disk-read", "server-boot"}, events)
}

func TestCreateCompletesResponseDisksWhenServerResponseOmitsThem(t *testing.T) {
	// サーバ作成 API の応答にディスクが含まれない場合でも、CLI が実行した
	// ディスク作成の応答で Disks を補完して、作成したディスクの ID を応答から
	// 確定できるようにします
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123}}
	diskOp := &diskAPIFake{
		createResults: []*iaas.Disk{
			{
				ID:                  456,
				Name:                "test-disk",
				Availability:        types.Availabilities.Migrating,
				Connection:          types.DiskConnections.VirtIO,
				ConnectionOrder:     1,
				SizeMB:              20480,
				DiskPlanID:          7,
				EncryptionAlgorithm: types.DiskEncryptionAlgorithms.None,
			},
			{
				ID:                  457,
				Name:                "test-disk-2",
				Availability:        types.Availabilities.Migrating,
				Connection:          types.DiskConnections.VirtIO,
				ConnectionOrder:     2,
				SizeMB:              40960,
				DiskPlanID:          7,
				EncryptionAlgorithm: types.DiskEncryptionAlgorithms.AES256XTS,
			},
		},
	}
	sdk := &sdkAPI{op: serverOp, diskOp: diskOp}

	result, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:     "is1b",
		Name:     "test-server",
		CPU:      2,
		MemoryGB: 4,
		Disks: []DiskRequest{{
			Name:       "test-disk",
			DiskPlanID: 7,
			SizeGB:     20,
		}, {
			Name:       "test-disk-2",
			DiskPlanID: 7,
			SizeGB:     40,
		}},
	})

	require.NoError(t, err)
	require.Len(t, result.Disks, 2,
		"the create result should include the disks created by the CLI")
	require.EqualValues(t, 456, result.Disks[0].ID)
	require.Equal(t, "test-disk", result.Disks[0].Name)
	require.Equal(t, types.Availabilities.Migrating, result.Disks[0].Availability)
	require.Equal(t, types.DiskConnections.VirtIO, result.Disks[0].Connection)
	require.Equal(t, 1, result.Disks[0].ConnectionOrder)
	require.EqualValues(t, 20480, result.Disks[0].SizeMB)
	require.EqualValues(t, 7, result.Disks[0].DiskPlanID)
	require.Equal(t, types.DiskEncryptionAlgorithms.None, result.Disks[0].EncryptionAlgorithm)
	require.EqualValues(t, 457, result.Disks[1].ID)
	require.Equal(t, "test-disk-2", result.Disks[1].Name)
	require.Equal(t, 2, result.Disks[1].ConnectionOrder,
		"the connection order of the created disk should be preserved")
	require.Equal(t, types.DiskEncryptionAlgorithms.AES256XTS, result.Disks[1].EncryptionAlgorithm)
}

func TestCreateKeepsServerResponseDisksWhenPresent(t *testing.T) {
	// サーバ作成 API の応答にディスクが含まれる場合は API 応答をそのまま返します
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123, Disks: []*iaas.ServerConnectedDisk{
		{ID: 999, Name: "from-server-response"},
	}}}
	diskOp := &diskAPIFake{
		createResult: &iaas.Disk{ID: 456, Name: "from-disk-create-response"},
	}
	sdk := &sdkAPI{op: serverOp, diskOp: diskOp}

	result, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:     "is1b",
		Name:     "test-server",
		CPU:      2,
		MemoryGB: 4,
		Disks: []DiskRequest{{
			Name:       "test-disk",
			DiskPlanID: 7,
			SizeGB:     20,
		}},
	})

	require.NoError(t, err)
	require.Len(t, result.Disks, 1)
	require.EqualValues(t, 999, result.Disks[0].ID)
	require.Equal(t, "from-server-response", result.Disks[0].Name)
}

func TestCreateDoesNotBootWhenDiskCannotBecomeAvailable(t *testing.T) {
	serverOp := &serverAPIFake{createResult: &iaas.Server{ID: 123}}
	diskOp := &diskAPIFake{
		createResult: &iaas.Disk{ID: 456},
		readResults: []*iaas.Disk{
			{ID: 456, ServerID: 123, Availability: types.Availabilities.Failed},
		},
	}
	sdk := &sdkAPI{op: serverOp, diskOp: diskOp}

	_, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:            "is1b",
		Name:            "test-server",
		CPU:             2,
		MemoryGB:        4,
		BootAfterCreate: true,
		Disks:           []DiskRequest{{DiskPlanID: 7, SizeGB: 20}},
	})

	require.ErrorContains(t, err, "利用可能化に失敗")
	require.Equal(t, []string{"create"}, serverOp.calls)
}

func TestCreatePassesDiskSourceAndEncryptionFieldsFromJSON(t *testing.T) {
	var request CreateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"Zone":"is1b","Name":"test-server","CPU":2,"MemoryGB":4,"Disks":[{"Name":"test-disk","DiskPlanID":7,"SizeGB":20,"SourceDiskID":321,"EncryptionAlgorithm":"aes256_xts","KMSKeyID":789}]}`), &request))
	caller := &serverDiskCreateAPICallerFake{}
	sdk := &sdkAPI{op: iaas.NewServerOp(caller), diskOp: iaas.NewDiskOp(caller)}

	result, err := sdk.CreateWithContext(context.Background(), &request)

	require.NoError(t, err)
	require.Equal(t, 2, caller.calls)
	// 実際の SDK のデコード経路を通った応答でも、作成したディスクの情報が
	// Disks に保持されることを確認します
	require.Len(t, result.Disks, 1)
	require.EqualValues(t, 456, result.Disks[0].ID)
	require.Equal(t, "test-disk", result.Disks[0].Name)
	var body struct {
		Disk struct {
			EncryptionAlgorithm types.EDiskEncryptionAlgorithm
			SourceDisk          struct {
				ID types.ID
			}
		}
		KMSKey struct {
			ID types.ID
		}
	}
	require.NoError(t, json.Unmarshal(caller.diskRequest, &body))
	require.Equal(t, types.EDiskEncryptionAlgorithm("aes256_xts"), body.Disk.EncryptionAlgorithm)
	require.EqualValues(t, 321, body.Disk.SourceDisk.ID)
	require.EqualValues(t, 789, body.KMSKey.ID)
}

func TestCreateConnectsPacketFilterFromJSON(t *testing.T) {
	var request CreateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"Zone":"is1b","Name":"test-server","CPU":2,"MemoryGB":4,"BootAfterCreate":true,"NetworkInterfaces":[{"Upstream":"shared","PacketFilterID":789}]}`), &request))
	caller := &serverPacketFilterAPICallerFake{}
	sdk := NewAPI(caller)

	_, err := sdk.CreateWithContext(context.Background(), &request)

	require.NoError(t, err)
	require.Len(t, caller.calls, 5)
	require.Equal(t, "POST", caller.calls[0].method)
	require.Contains(t, caller.calls[0].path, "/server")
	require.Equal(t, "GET", caller.calls[1].method)
	require.Contains(t, caller.calls[1].path, "/server/123")
	require.Equal(t, "GET", caller.calls[2].method)
	require.Contains(t, caller.calls[2].path, "/interface/456")
	require.Equal(t, "PUT", caller.calls[3].method)
	require.Contains(t, caller.calls[3].path, "/interface/456/to/packetfilter/789")
	require.Equal(t, "PUT", caller.calls[4].method)
	require.Contains(t, caller.calls[4].path, "/server/123/power")
}

func TestCreateDoesNotBootAndCleansUpWhenPacketFilterConnectionFails(t *testing.T) {
	var request CreateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"Zone":"is1b","Name":"test-server","CPU":2,"MemoryGB":4,"BootAfterCreate":true,"NetworkInterfaces":[{"Upstream":"shared","PacketFilterID":789}]}`), &request))
	caller := &serverPacketFilterAPICallerFake{failPacketFilter: true}
	sdk := NewAPI(caller)

	_, err := sdk.CreateWithContext(context.Background(), &request)

	require.ErrorContains(t, err, "パケットフィルター")
	require.Len(t, caller.calls, 5)
	require.Contains(t, caller.calls[3].path, "/interface/456/to/packetfilter/789")
	require.Equal(t, "DELETE", caller.calls[4].method)
	require.Contains(t, caller.calls[4].path, "/server/123")
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
	findCondition          *iaas.FindCondition
	createRequest          *iaas.ServerCreateRequest
	createResult           *iaas.Server
	readResult             *iaas.Server
	readResults            []*iaas.Server
	changePlanRequest      *iaas.ServerChangePlanRequest
	deleteWithDisksRequest *iaas.ServerDeleteWithDisksRequest
	updateCalls            int
	changePlanCalls        int
	events                 *[]string
}

func (api *serverAPIFake) Create(_ context.Context, _ string, request *iaas.ServerCreateRequest) (*iaas.Server, error) {
	api.calls = append(api.calls, "create")
	if api.events != nil {
		*api.events = append(*api.events, "server-create")
	}
	api.createRequest = request
	return api.createResult, nil
}

func (api *serverAPIFake) Find(_ context.Context, _ string, condition *iaas.FindCondition) (*iaas.ServerFindResult, error) {
	api.findCondition = condition
	return &iaas.ServerFindResult{}, nil
}

func (api *serverAPIFake) Boot(_ context.Context, _ string, _ types.ID) error {
	api.calls = append(api.calls, "boot")
	if api.events != nil {
		*api.events = append(*api.events, "server-boot")
	}
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
	calls         []string
	createRequest *iaas.DiskCreateRequest
	createResult  *iaas.Disk
	createResults []*iaas.Disk
	readResults   []*iaas.Disk
	events        *[]string
}

func (api *diskAPIFake) Create(_ context.Context, _ string, request *iaas.DiskCreateRequest, _ []types.ID, _ types.ID) (*iaas.Disk, error) {
	api.calls = append(api.calls, "create")
	if api.events != nil {
		*api.events = append(*api.events, "disk-create")
	}
	api.createRequest = request
	if len(api.createResults) > 0 {
		result := api.createResults[0]
		api.createResults = api.createResults[1:]
		return result, nil
	}
	return api.createResult, nil
}

func (api *diskAPIFake) Read(_ context.Context, _ string, _ types.ID) (*iaas.Disk, error) {
	api.calls = append(api.calls, "read")
	if api.events != nil {
		*api.events = append(*api.events, "disk-read")
	}
	if len(api.readResults) == 0 {
		return nil, nil
	}
	result := api.readResults[0]
	api.readResults = api.readResults[1:]
	return result, nil
}

type serverAPICallerFake struct {
	body []byte
}

type serverDiskCreateAPICallerFake struct {
	calls       int
	diskRequest []byte
}

type serverPacketFilterAPICallerFake struct {
	calls []struct {
		method string
		path   string
	}
	failPacketFilter bool
}

func (caller *serverPacketFilterAPICallerFake) Do(_ context.Context, method, path string, _ any) ([]byte, error) {
	caller.calls = append(caller.calls, struct {
		method string
		path   string
	}{method: method, path: path})
	switch {
	case method == "POST" && strings.HasSuffix(path, "/server"):
		return []byte(`{"Server":{"ID":123}}`), nil
	case method == "GET" && strings.HasSuffix(path, "/server/123"):
		return []byte(`{"Server":{"ID":123,"Interfaces":[{"ID":456,"Switch":{"Scope":"shared"}}]}}`), nil
	case method == "GET" && strings.HasSuffix(path, "/interface/456"):
		return []byte(`{"Interface":{"ID":456,"Server":{"ID":123}}}`), nil
	case method == "PUT" && strings.HasSuffix(path, "/interface/456/to/packetfilter/789"):
		if caller.failPacketFilter {
			return nil, fmt.Errorf("packet filter rejected")
		}
		return []byte(`{}`), nil
	case method == "PUT" && strings.HasSuffix(path, "/server/123/power"):
		return []byte(`{}`), nil
	case method == "DELETE" && strings.HasSuffix(path, "/server/123"):
		return []byte(`{}`), nil
	default:
		return nil, fmt.Errorf("unexpected API call: %s %s", method, path)
	}
}

func (caller *serverDiskCreateAPICallerFake) Do(_ context.Context, _, _ string, body any) ([]byte, error) {
	caller.calls++
	switch caller.calls {
	case 1:
		return []byte(`{"Server":{"ID":123}}`), nil
	case 2:
		var err error
		caller.diskRequest, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return []byte(`{"Disk":{"ID":456,"Name":"test-disk","Availability":"migrating","Connection":"virtio","ConnectionOrder":1,"SizeMB":20480,"Plan":{"ID":7}}}`), nil
	default:
		return nil, fmt.Errorf("unexpected API call %d", caller.calls)
	}
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
