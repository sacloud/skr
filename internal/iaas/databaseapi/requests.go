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
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// 以下のリクエスト型は生成コマンドが組み立てる入力モデルです。
// Zone と操作ごとの条件を1つの構造体にまとめ、API 実装が api 層の
// DatabaseOp の操作に変換して呼び出します。service 層は使いません。

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

// MonitorDiskRequest はディスクモニタリング操作のリクエストです。
type MonitorDiskRequest struct {
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

// MonitorDatabaseRequest はデータベースモニタリング操作のリクエストです。
type MonitorDatabaseRequest struct {
	Zone string
	ID   types.ID

	Start time.Time
	End   time.Time
}

// ListParameterRequest は list-parameter 操作のリクエストです。
type ListParameterRequest struct {
	Zone string
	ID   types.ID
}

// Parameter は list-parameter 操作の出力要素です。
// Key はパラメータの表示名、Value は現在の設定値、Meta はパラメータのメタ情報です。
type Parameter struct {
	Key   string
	Value any
	Meta  *iaas.DatabaseParameterMeta
}
