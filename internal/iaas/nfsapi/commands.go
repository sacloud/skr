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

// Package nfsapi はさくらのクラウドの NFS アプライアンスのコマンド実装を提供します。
package nfsapi

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/power"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/wait"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// nfsAPI は生成コマンドが使う API 実装です。
// すべての操作で api 層の NFSOp を直接呼び出し、service 層は使いません。
// 削除・起動・シャットダウンのワークフロー (強制終了と停止・起動待ち) もこの層で実装します。
type nfsAPI struct {
	op iaas.NFSAPI
}

// NewAPI は生成コマンドが使う API 実装を構築します。wiring の sdk_factory から呼ばれます。
func NewAPI(caller iaas.APICaller) API {
	return &nfsAPI{op: iaas.NewNFSOp(caller)}
}

// FindWithContext は api 層の NFSOp.Find を直接呼び出します。名前とタグの検索条件は
// api 層の FindCondition に変換します。単一名の部分一致検索は Names の1要素に変換します。
func (a *nfsAPI) FindWithContext(ctx context.Context, req *FindRequest) ([]*iaas.NFS, error) {
	condition := &iaas.FindCondition{
		Filter: map[search.FilterKey]any{},
	}
	if len(req.Names) > 0 {
		condition.Filter[search.Key("Name")] = search.AndEqual(req.Names...)
	}
	if len(req.Tags) > 0 {
		condition.Filter[search.Key("Tags.Name")] = search.TagsAndEqual(req.Tags...)
	}
	condition.Sort = req.Sort
	condition.Count = req.Count
	condition.From = req.From
	found, err := a.op.Find(ctx, req.Zone, condition)
	if err != nil {
		return nil, err
	}
	return found.NFS, nil
}

// ReadWithContext は api 層の NFSOp.Read を直接呼び出します。
func (a *nfsAPI) ReadWithContext(ctx context.Context, req *ReadRequest) (*iaas.NFS, error) {
	return a.op.Read(ctx, req.Zone, req.ID)
}

// TransformCreateRequest は create 操作の反復可能な --ip-address を検証して
// 作成リクエストの IPAddresses へ設定します。フラグ経路の入力だけが対象で、
// --request JSON 経路の IPAddresses はここを経由しません。
func TransformCreateRequest(c *CreateCommand, request *CreateRequest) error {
	if len(c.IPAddresses) == 0 {
		return fmt.Errorf("--ip-address が必要です")
	}
	if len(c.IPAddresses) > 2 {
		return fmt.Errorf("--ip-address は2個まで指定できます")
	}
	for i, ip := range c.IPAddresses {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			return fmt.Errorf("--ip-address の %d 個目は IPv4 アドレスで指定してください: %s", i+1, ip)
		}
		request.IPAddresses = append(request.IPAddresses, ip)
	}
	return nil
}

// CreateWithContext は api 層の NFSOp.Create を直接呼び出します。作成リクエストは
// 生成コマンドが組み立てた内容を api 層の NFSCreateRequest に変換して送信します。
// 作成リクエストの Class の補完 (nfs) は api 層の作成処理が行います。
func (a *nfsAPI) CreateWithContext(ctx context.Context, req *CreateRequest) (*iaas.NFS, error) {
	param := &iaas.NFSCreateRequest{
		SwitchID:       req.SwitchID,
		PlanID:         req.PlanID,
		IPAddresses:    req.IPAddresses,
		NetworkMaskLen: req.NetworkMaskLen,
		DefaultRoute:   req.DefaultRoute,
		Name:           req.Name,
		Description:    req.Description,
		Tags:           req.Tags,
		IconID:         req.IconID,
	}
	return a.op.Create(ctx, req.Zone, param)
}

// UpdateWithContext は api 層の NFSOp.Update を直接呼び出します。未指定の項目は
// 空の値を API の更新リクエストへ送信します。
func (a *nfsAPI) UpdateWithContext(ctx context.Context, req *UpdateRequest) (*iaas.NFS, error) {
	params := &iaas.NFSUpdateRequest{}
	if req.Name != nil {
		params.Name = *req.Name
	}
	if req.Description != nil {
		params.Description = *req.Description
	}
	if req.Tags != nil {
		params.Tags = *req.Tags
	}
	if req.IconID != nil {
		params.IconID = *req.IconID
	}
	return a.op.Update(ctx, req.Zone, req.ID, params)
}

// DeleteWithContext は削除のワークフローを含めて api 層の NFSOp を呼び出します。
// 電源が起動中で Force が指定されていない場合はエラーにし、Force の場合は
// 強制終了してから停止を待ち、その後削除します。FailIfNotFound を指定しない場合、
// 対象が見つからないときはエラーにしません。
func (a *nfsAPI) DeleteWithContext(ctx context.Context, req *DeleteRequest) error {
	target, err := a.op.Read(ctx, req.Zone, req.ID)
	if err != nil {
		return handleNotFoundError(err, !req.FailIfNotFound)
	}
	if !req.Force && target.InstanceStatus.IsUp() {
		return fmt.Errorf("target %s:%q has not yet shut down", req.Zone, req.ID)
	}
	if target.InstanceStatus.IsUp() {
		if err := a.op.Shutdown(ctx, req.Zone, req.ID, &iaas.ShutdownOption{Force: true}); err != nil {
			return err
		}
	}
	// 元の状態が Unknown でなければ停止を待ちます (SDK の service 層と同じ条件)。
	if target.InstanceStatus != types.ServerInstanceStatuses.Unknown {
		if _, err := wait.UntilNFSIsDown(ctx, a.op, req.Zone, req.ID); err != nil {
			return err
		}
	}
	if err := a.op.Delete(ctx, req.Zone, req.ID); err != nil {
		return handleNotFoundError(err, !req.FailIfNotFound)
	}
	return nil
}

// handleNotFoundError は対象が見つからない場合のエラーを、許可されているときは無視します。
func handleNotFoundError(err error, ignore bool) error {
	if ignore && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}

// BootWithContext は起動のワークフローを含めて api 層の NFSOp を呼び出します。
// NoWait を指定しない場合は起動の完了を待ちます。
func (a *nfsAPI) BootWithContext(ctx context.Context, req *BootRequest) error {
	if req.NoWait {
		return a.op.Boot(ctx, req.Zone, req.ID)
	}
	return power.BootNFS(ctx, a.op, req.Zone, req.ID)
}

// ShutdownWithContext は停止のワークフローを含めて api 層の NFSOp を呼び出します。
// NoWait を指定しない場合は停止の完了を待ちます。
func (a *nfsAPI) ShutdownWithContext(ctx context.Context, req *ShutdownRequest) error {
	if req.NoWait {
		return a.op.Shutdown(ctx, req.Zone, req.ID, &iaas.ShutdownOption{Force: req.ForceShutdown})
	}
	return power.ShutdownNFS(ctx, a.op, req.Zone, req.ID, req.ForceShutdown)
}

// ResetWithContext は api 層の NFSOp.Reset を直接呼び出します。
func (a *nfsAPI) ResetWithContext(ctx context.Context, req *ResetRequest) error {
	return a.op.Reset(ctx, req.Zone, req.ID)
}

// MonitorCPUWithContext は api 層の NFSOp.MonitorCPU を直接呼び出します。
func (a *nfsAPI) MonitorCPUWithContext(ctx context.Context, req *MonitorCPURequest) ([]*iaas.MonitorCPUTimeValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorCPU(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// MonitorFreeDiskSizeWithContext は api 層の NFSOp.MonitorFreeDiskSize を直接呼び出します。
// この API は日単位の集計で正時でない時刻を受け付けないため、期間の既定値は正時です。
func (a *nfsAPI) MonitorFreeDiskSizeWithContext(ctx context.Context, req *MonitorFreeDiskSizeRequest) ([]*iaas.MonitorFreeDiskSizeValue, error) {
	condition, err := freeDiskSizeCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorFreeDiskSize(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// MonitorInterfaceWithContext は api 層の NFSOp.MonitorInterface を直接呼び出します。
func (a *nfsAPI) MonitorInterfaceWithContext(ctx context.Context, req *MonitorInterfaceRequest) ([]*iaas.MonitorInterfaceValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorInterface(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// monitorCondition は取得期間の未指定値を既定に補完し、開始と終了の整合を検証します。
// 終了の未指定は現在時刻、開始の未指定は終了の 1 時間前です。
func monitorCondition(start, end time.Time) (*iaas.MonitorCondition, error) {
	e := end
	if e.IsZero() {
		e = time.Now()
	}
	s := start
	if s.IsZero() {
		s = e.Add(-1 * time.Hour)
	}
	if !(s.Unix() <= e.Unix()) {
		return nil, fmt.Errorf("start(%s) or end(%s) is invalid", start.String(), end.String())
	}
	return &iaas.MonitorCondition{Start: s, End: e}, nil
}

// freeDiskSizeCondition は日単位のモニタリング API に対する取得期間を組み立てます。
// この API は正時でない時刻を指定するとエラー (400) になるため、--end の省略は
// 当日の UTC 正時、--start の省略は --end の前日の UTC 正時を既定にします。
func freeDiskSizeCondition(start, end time.Time) (*iaas.MonitorCondition, error) {
	e := end
	if e.IsZero() {
		e = time.Now().UTC().Truncate(24 * time.Hour)
	}
	s := start
	if s.IsZero() {
		s = e.Add(-24 * time.Hour)
	}
	if !(s.Unix() <= e.Unix()) {
		return nil, fmt.Errorf("start(%s) or end(%s) is invalid", start.String(), end.String())
	}
	return &iaas.MonitorCondition{Start: s, End: e}, nil
}
