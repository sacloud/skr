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
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

const (
	serverShutdownTimeout   = 20 * time.Minute
	serverShutdownInterval  = 5 * time.Second
	serverDiskReadyTimeout  = 20 * time.Minute
	serverDiskReadyInterval = 5 * time.Second
)

type sdkAPI struct {
	op     iaas.ServerAPI
	diskOp iaas.DiskAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{
		op:     iaas.NewServerOp(caller),
		diskOp: iaas.NewDiskOp(caller),
	}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Server, error) {
	condition := &iaas.FindCondition{
		Count: request.Count,
		From:  request.From,
		Sort:  request.Sort,
	}
	filter := search.Filter{}
	if len(request.Names) != 0 {
		filter[search.Key("Name")] = search.AndEqual(request.Names...)
	}
	if len(request.Tags) != 0 {
		filter[search.Key("Tags.Name")] = search.TagsAndEqual(request.Tags...)
	}
	if len(filter) != 0 {
		condition.Filter = filter
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.Servers, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Server, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	return api.op.Read(ctx, request.Zone, request.ID)
}

func (api *sdkAPI) CreateWithContext(ctx context.Context, request *CreateRequest) (*iaas.Server, error) {
	if request.NoWait || request.CDROMID != nil {
		return nil, fmt.Errorf("NoWait と CDROMID はこの作成操作では受け付けません")
	}
	interfaceDriver := request.InterfaceDriver
	if interfaceDriver == "" {
		interfaceDriver = types.InterfaceDrivers.VirtIO
	}
	params := &iaas.ServerCreateRequest{
		CPU:             request.CPU,
		GPU:             request.GPU,
		GPUModel:        request.GPUModel,
		CPUModel:        request.CPUModel,
		Commitment:      request.Commitment,
		Generation:      request.Generation,
		ConfidentialVM:  request.ConfidentialVM,
		InterfaceDriver: interfaceDriver,
		Name:            request.Name,
		Description:     request.Description,
		Tags:            request.Tags,
		IconID:          request.IconID,
		PrivateHostID:   request.PrivateHostID,
	}
	params.SetMemoryMB(request.MemoryGB * 1024)
	for _, nic := range request.NetworkInterfaces {
		connection, err := connectedSwitch(nic.Upstream)
		if err != nil {
			return nil, err
		}
		params.ConnectedSwitches = append(params.ConnectedSwitches, connection)
	}
	server, err := api.op.Create(ctx, request.Zone, params)
	if err != nil {
		return nil, err
	}
	if server == nil || server.ID.IsEmpty() {
		return nil, fmt.Errorf("サーバ作成 API の応答に ID がありません")
	}
	createdDisks := make([]types.ID, 0, len(request.Disks))
	for _, disk := range request.Disks {
		diskParams := &iaas.DiskCreateRequest{
			DiskPlanID:      disk.DiskPlanID,
			Connection:      disk.Connection,
			SourceArchiveID: disk.SourceArchiveID,
			ServerID:        server.ID,
			Name:            disk.Name,
		}
		diskParams.SetSizeGB(disk.SizeGB)
		createdDisk, err := api.diskOp.Create(ctx, request.Zone, diskParams, nil, 0)
		if err != nil {
			cleanupErr := api.cleanupCreatedServer(ctx, request.Zone, server.ID, createdDisks)
			return nil, errors.Join(fmt.Errorf("サーバ %d のディスク作成に失敗しました: %w", server.ID, err), cleanupErr)
		}
		createdDisks = append(createdDisks, createdDisk.ID)
	}
	if request.BootAfterCreate {
		waitCtx, cancel := context.WithTimeout(ctx, serverDiskReadyTimeout)
		defer cancel()
		for _, diskID := range createdDisks {
			if err := api.waitForDiskReady(waitCtx, request.Zone, server.ID, diskID); err != nil {
				return nil, fmt.Errorf("サーバ %d は作成されましたが、ディスク %d の起動準備完了を確認できませんでした: %w", server.ID, diskID, err)
			}
		}
		if err := api.op.Boot(ctx, request.Zone, server.ID); err != nil {
			return nil, fmt.Errorf("サーバ %d は作成されましたが、起動に失敗しました: %w", server.ID, err)
		}
	}
	return server, nil
}

func (api *sdkAPI) waitForDiskReady(ctx context.Context, zone string, serverID, diskID types.ID) error {
	ticker := time.NewTicker(serverDiskReadyInterval)
	defer ticker.Stop()

	for {
		disk, err := api.diskOp.Read(ctx, zone, diskID)
		if err == nil {
			if disk == nil {
				return fmt.Errorf("ディスク %d の読み込み結果が空です", diskID)
			}
			if disk.ID != diskID || disk.ServerID != serverID {
				return fmt.Errorf("ディスク %d の所属確認に失敗しました", diskID)
			}
			if disk.Availability.IsAvailable() {
				return nil
			}
			if disk.Availability.IsFailed() || disk.Availability.IsDiscontinued() {
				return fmt.Errorf("ディスク %d の利用可能化に失敗しました: %s", diskID, disk.Availability)
			}
		} else if !iaas.IsNotFoundError(err) {
			return fmt.Errorf("ディスク %d の状態を確認できません: %w", diskID, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("ディスク %d が %s 以内に利用可能になりませんでした: %w", diskID, serverDiskReadyTimeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func connectedSwitch(upstream string) (*iaas.ConnectedSwitch, error) {
	switch upstream {
	case "shared":
		return &iaas.ConnectedSwitch{Scope: types.Scopes.Shared}, nil
	case "disconnected":
		return nil, nil
	default:
		id, err := strconv.ParseInt(upstream, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("upstream は shared、disconnected、または正の整数のスイッチ ID で指定してください")
		}
		return &iaas.ConnectedSwitch{ID: types.ID(id)}, nil
	}
}

func (api *sdkAPI) cleanupCreatedServer(ctx context.Context, zone string, serverID types.ID, diskIDs []types.ID) error {
	if len(diskIDs) == 0 {
		if err := api.op.Delete(ctx, zone, serverID); err != nil {
			return fmt.Errorf("作成途中のサーバ %d の削除にも失敗しました: %w", serverID, err)
		}
		return nil
	}
	if err := api.op.DeleteWithDisks(ctx, zone, serverID, &iaas.ServerDeleteWithDisksRequest{IDs: diskIDs}); err != nil {
		return fmt.Errorf("作成途中のサーバ %d とディスクの削除にも失敗しました: %w", serverID, err)
	}
	return nil
}

func (api *sdkAPI) BootWithContext(ctx context.Context, request *BootRequest) error {
	if request.ID == 0 {
		return fmt.Errorf("ID is required")
	}
	if request.UserData == "" {
		return api.op.Boot(ctx, request.Zone, request.ID)
	}
	return api.op.BootWithVariables(ctx, request.Zone, request.ID, &iaas.ServerBootVariables{UserData: request.UserData})
}

func (api *sdkAPI) UpdateWithContext(ctx context.Context, request *UpdateRequest) (*iaas.Server, error) {
	if request.ID == 0 {
		return nil, fmt.Errorf("ID is required")
	}
	if request.CDROMID != nil || len(request.Disks) != 0 || len(request.NetworkInterfaces) != 0 {
		return nil, fmt.Errorf("CDROMID、Disks、NetworkInterfaces はこの更新操作では受け付けません")
	}
	hasPlanChange := request.CPU != nil || request.MemoryGB != nil || request.GPU != nil ||
		request.GPUModel != nil || request.CPUModel != nil ||
		request.Commitment != nil || request.Generation != nil
	hasMetadataChange := request.Name != nil || request.Description != nil || request.Tags != nil ||
		request.IconID != nil || request.PrivateHostID != nil || request.InterfaceDriver != nil
	current, err := api.op.Read(ctx, request.Zone, request.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("サーバ %d の読み込み結果が空です", request.ID)
	}
	if !hasMetadataChange && !hasPlanChange {
		return current, nil
	}
	var result *iaas.Server
	if hasMetadataChange {
		update := &iaas.ServerUpdateRequest{
			Name:            current.Name,
			Description:     current.Description,
			Tags:            current.Tags,
			IconID:          current.IconID,
			PrivateHostID:   current.PrivateHostID,
			InterfaceDriver: current.InterfaceDriver,
		}
		if request.Name != nil {
			update.Name = *request.Name
		}
		if request.Description != nil {
			update.Description = *request.Description
		}
		if request.Tags != nil {
			update.Tags = *request.Tags
		}
		if request.IconID != nil {
			update.IconID = *request.IconID
		}
		if request.PrivateHostID != nil {
			update.PrivateHostID = *request.PrivateHostID
		}
		if request.InterfaceDriver != nil {
			update.InterfaceDriver = *request.InterfaceDriver
		}
		updated, err := api.op.Update(ctx, request.Zone, request.ID, update)
		if err != nil {
			return nil, err
		}
		result = updated
	}
	if !hasPlanChange {
		return result, nil
	}

	plan := &iaas.ServerChangePlanRequest{
		CPU:        current.CPU,
		MemoryMB:   current.MemoryMB,
		GPU:        current.GPU,
		GPUModel:   current.GPUModel,
		CPUModel:   current.CPUModel,
		Commitment: current.Commitment,
		Generation: current.Generation,
	}
	if request.CPU != nil {
		plan.CPU = *request.CPU
	}
	if request.MemoryGB != nil {
		plan.SetMemoryGB(*request.MemoryGB)
	}
	if request.GPU != nil {
		plan.GPU = *request.GPU
	}
	if request.GPUModel != nil {
		plan.GPUModel = *request.GPUModel
	}
	if request.CPUModel != nil {
		plan.CPUModel = *request.CPUModel
	}
	if request.Commitment != nil {
		plan.Commitment = *request.Commitment
	}
	if request.Generation != nil {
		plan.Generation = *request.Generation
	}
	return api.op.ChangePlan(ctx, request.Zone, request.ID, plan)
}

func (api *sdkAPI) DeleteWithContext(ctx context.Context, request *DeleteRequest) error {
	if request.ID == 0 {
		return fmt.Errorf("ID is required")
	}
	server, err := api.op.Read(ctx, request.Zone, request.ID)
	if err != nil {
		if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if server == nil {
		return fmt.Errorf("サーバ %d の読み込み結果が空です", request.ID)
	}
	if !request.Force && server.InstanceStatus.IsUp() {
		return fmt.Errorf("サーバ %d は起動中です。停止してから削除してください", request.ID)
	}
	if server.InstanceStatus.IsUp() {
		if err := api.op.Shutdown(ctx, request.Zone, request.ID, &iaas.ShutdownOption{Force: true}); err != nil {
			if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
				return nil
			}
			return err
		}
	}
	if server.InstanceStatus != types.ServerInstanceStatuses.Unknown {
		if err := api.waitUntilDown(ctx, request.Zone, request.ID); err != nil {
			return err
		}
	}
	if request.WithDisks {
		diskIDs := make([]types.ID, 0, len(server.Disks))
		for _, disk := range server.Disks {
			if disk == nil || disk.ID.IsEmpty() {
				return fmt.Errorf("サーバ %d の接続ディスク応答に ID がありません", request.ID)
			}
			diskIDs = append(diskIDs, disk.ID)
		}
		err = api.op.DeleteWithDisks(ctx, request.Zone, request.ID, &iaas.ServerDeleteWithDisksRequest{IDs: diskIDs})
	} else {
		err = api.op.Delete(ctx, request.Zone, request.ID)
	}
	if !request.FailIfNotFound && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}

func (api *sdkAPI) waitUntilDown(ctx context.Context, zone string, id types.ID) error {
	waitCtx, cancel := context.WithTimeout(ctx, serverShutdownTimeout)
	defer cancel()

	ticker := time.NewTicker(serverShutdownInterval)
	defer ticker.Stop()
	for {
		server, err := api.op.Read(waitCtx, zone, id)
		if err != nil {
			return fmt.Errorf("サーバ %d の停止状態を確認できません: %w", id, err)
		}
		if server == nil {
			return fmt.Errorf("サーバ %d の読み込み結果が空です", id)
		}
		if server.InstanceStatus.IsDown() {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("サーバ %d が %s 以内に停止しませんでした: %w", id, serverShutdownTimeout, waitCtx.Err())
		case <-ticker.C:
		}
	}
}
