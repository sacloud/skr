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
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// 以下のリクエスト型は生成コマンドが組み立てる入力モデルです。
// Zone と操作ごとの条件を1つの構造体にまとめ、API 実装が api 層の
// NFSOp の操作に変換して呼び出します。service 層は使いません。

// FindRequest は find 操作のリクエストです。名前とタグの検索条件は
// api 層の FindCondition に変換されます。
type FindRequest struct {
	Zone string

	Names []string
	Tags  []string

	Sort  search.SortKeys
	Count int
	From  int
}

// ReadRequest は read 操作のリクエストです。
type ReadRequest struct {
	Zone string
	ID   types.ID
}

// CreateRequest は create 操作のリクエストです。Zone は作成対象のゾーンで、
// それ以外の項目は api 層の NFSCreateRequest に変換して送信します。
type CreateRequest struct {
	Zone string

	SwitchID       types.ID
	PlanID         types.ID
	IPAddresses    []string
	NetworkMaskLen int
	DefaultRoute   string

	Name        string
	Description string
	Tags        types.Tags
	IconID      types.ID
}

// UpdateRequest は update 操作のリクエストです。ポインタ型のフィールドは
// 個別フラグの指定有無を表し、未指定時は空の値を API の更新リクエストへ送信します。
type UpdateRequest struct {
	Zone string
	ID   types.ID

	Name        *string
	Description *string
	Tags        *types.Tags
	IconID      *types.ID
}

// DeleteRequest は delete 操作のリクエストです。
// Force が true の場合は電源 OFF (強制終了) してから削除します。
type DeleteRequest struct {
	Zone string
	ID   types.ID

	FailIfNotFound bool
	Force          bool
}

// BootRequest は boot 操作のリクエストです。
type BootRequest struct {
	Zone string
	ID   types.ID

	NoWait bool
}

// ShutdownRequest は shutdown 操作のリクエストです。
type ShutdownRequest struct {
	Zone string
	ID   types.ID

	NoWait        bool
	ForceShutdown bool
}

// ResetRequest は reset 操作のリクエストです。
type ResetRequest struct {
	Zone string
	ID   types.ID
}

// MonitorCPURequest は CPU モニタリング操作のリクエストです。
type MonitorCPURequest struct {
	Zone string
	ID   types.ID

	Start time.Time
	End   time.Time
}

// MonitorFreeDiskSizeRequest は空きディスク容量モニタリング操作のリクエストです。
type MonitorFreeDiskSizeRequest struct {
	Zone string
	ID   types.ID

	Start time.Time
	End   time.Time
}

// MonitorInterfaceRequest はインターフェースモニタリング操作のリクエストです。
type MonitorInterfaceRequest struct {
	Zone string
	ID   types.ID

	Start time.Time
	End   time.Time
}
