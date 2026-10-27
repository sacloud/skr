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

package main

import (
	"errors"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// fakeServerCommands はシナリオの単体テストで使う偽の Server・Disk 操作です。
// フィールドで状態と失敗の注入を切り替え、シナリオの各経路を検証します。
type fakeServerCommands struct {
	// server は現在管理している Server です。nil の場合は存在しません。
	server *serverItem
	// disk はテスト用の Disk が存在するかどうかです。
	disk bool

	// createDiskOnly は Server なしで Disk だけを作成する作成失敗を注入します。
	createDiskOnly bool
	// failCreateAfterSideEffect は Server と Disk を作成した後に作成 API を失敗させます。
	failCreateAfterSideEffect bool
	// failUpdate は1回目の更新 API を失敗させます。
	failUpdate bool
	// failRestore は2回目の更新 API (復元) を失敗させます。
	failRestore bool
	// ignoreUpdate は1回目の更新を無視して成功を装います。
	ignoreUpdate bool
	// ignoreRestore は2回目の更新を無視して成功を装います。
	ignoreRestore bool
	// deleteServerError は Server の削除処理後に削除 API を失敗させます。
	deleteServerError bool
	// leaveDisk は Server 削除後も Disk を残します。
	leaveDisk bool
	// multipleOrphans は同名の Disk を複数残します。
	multipleOrphans bool
	// keepOrphan は孤立ディスクの削除後も Disk を残します。
	keepOrphan bool
	// orphanDisk は孤立ディスクの参照結果です。nil の場合は既定の Disk を返します。
	orphanDisk *diskItem
	// changeIdentityAtCleanup は後片付けの再照合で識別情報を変更します。
	changeIdentityAtCleanup bool
	// failMethod は指定した操作を失敗させます。
	failMethod string
	// failLookupDisksCall は N 回目の Disk 検索を失敗させます。事前確認と後片付けの
	// 検索を区別するために使います。
	failLookupDisksCall int
	// sshErr は SSH ログイン確認が毎回返すエラーです。
	sshErr error

	// deleted は削除した Server の ID の記録です。
	deleted []types.ID
	// deletedDisks は削除した Disk の ID の記録です。
	deletedDisks []types.ID
	// calls は実行した操作の記録です。CLI 操作と SDK による確認の両方を記録します。
	calls []string
	// lookupDisksCalls は Disk 検索の呼び出し回数です。
	lookupDisksCalls int
	// updateCalls は更新 API の呼び出し回数です。更新と復元を区別するために使います。
	updateCalls int
}

// record は操作を記録し、failMethod が一致する場合は失敗を注入します。
func (f *fakeServerCommands) record(name string) error {
	f.calls = append(f.calls, name)
	if f.failMethod == name {
		return errors.New("injected failure")
	}
	return nil
}

func (f *fakeServerCommands) createServer(t *testing.T, label, name string, params serverDiskParams) (serverItem, error) {
	f.calls = append(f.calls, "createServer")
	if f.createDiskOnly {
		// Server の作成に失敗したが Disk だけが作成された状態を再現する
		f.disk = true
		return serverItem{}, errors.New("injected partial create failure")
	}
	f.server = &serverItem{
		ID: 123, Name: name, Description: serverDescription,
		Disks: []diskItem{{ID: 45, Name: name}},
	}
	f.disk = true
	if f.failCreateAfterSideEffect {
		// リソースを作成した後に作成 API が失敗した状態を再現する
		return serverItem{}, errors.New("injected failure")
	}
	return *f.server, nil
}

func (f *fakeServerCommands) bootServer(t *testing.T, label string, id types.ID, userDataPath string) error {
	f.calls = append(f.calls, "bootServer")
	if f.server == nil || f.server.ID != id {
		return errors.New("missing Server")
	}
	f.server.InstanceStatus = "up"
	f.server.Interfaces = []serverInterface{{IPAddress: "203.0.113.1", UpstreamType: "shared"}}
	return nil
}

func (f *fakeServerCommands) readServer(t *testing.T, label string, id types.ID) (serverItem, error) {
	f.calls = append(f.calls, "readServer")
	if f.server == nil || f.server.ID != id {
		return serverItem{}, errors.New("missing Server")
	}
	return *f.server, nil
}

func (f *fakeServerCommands) findServers(t *testing.T, label, name string) ([]serverItem, error) {
	if err := f.record("findServers"); err != nil {
		return nil, err
	}
	if f.server == nil || f.server.Name != name {
		return nil, nil
	}
	return []serverItem{*f.server}, nil
}

func (f *fakeServerCommands) updateServer(t *testing.T, label string, id types.ID, name, desc string) (serverItem, error) {
	f.calls = append(f.calls, "updateServer")
	if f.server == nil || f.server.ID != id {
		return serverItem{}, errors.New("missing Server")
	}
	if name == "" || desc == "" {
		return serverItem{}, errors.New("missing Name or Description")
	}
	f.updateCalls++
	// 更新と復元を区別するため、呼び出し回数で判定する
	if (f.failUpdate && f.updateCalls == 1) || (f.failRestore && f.updateCalls == 2) {
		return serverItem{}, errors.New("injected failure")
	}
	if (f.ignoreUpdate && f.updateCalls == 1) || (f.ignoreRestore && f.updateCalls == 2) {
		// 更新を無視して成功を装う。応答が更新前の値のままなので反映の有無を検出できる。
		return *f.server, nil
	}
	f.server.Name = name
	f.server.Description = desc
	return *f.server, nil
}

func (f *fakeServerCommands) deleteServer(t *testing.T, label string, id types.ID) error {
	f.calls = append(f.calls, "deleteServer")
	if f.server == nil || f.server.ID != id {
		return errors.New("missing Server")
	}
	f.deleted = append(f.deleted, f.server.ID)
	f.server = nil
	if !f.leaveDisk {
		f.disk = false
	}
	if f.deleteServerError {
		// リソースを削除した後に削除 API が失敗した状態を再現する
		return errors.New("injected partial delete failure")
	}
	return nil
}

func (f *fakeServerCommands) deleteDisk(t *testing.T, label string, id types.ID) error {
	if err := f.record("deleteDisk"); err != nil {
		return err
	}
	f.deletedDisks = append(f.deletedDisks, id)
	if !f.keepOrphan {
		f.disk = false
	}
	return nil
}

func (f *fakeServerCommands) connectSSH(t *testing.T, label, ip string) error {
	f.calls = append(f.calls, "connectSSH")
	return f.sshErr
}

// lookupServers は Server の事前確認と後片付けの検索を再現します。
func (f *fakeServerCommands) lookupServers(t *testing.T, name string) ([]serverItem, error) {
	if err := f.record("lookupServers"); err != nil {
		return nil, err
	}
	if f.server == nil || f.server.Name != name {
		return nil, nil
	}
	return []serverItem{*f.server}, nil
}

// lookupDisks は Disk の事前確認と後片付けの検索を再現します。
func (f *fakeServerCommands) lookupDisks(t *testing.T, name string) ([]diskItem, error) {
	if err := f.record("lookupDisks"); err != nil {
		return nil, err
	}
	f.lookupDisksCalls++
	if f.failLookupDisksCall == f.lookupDisksCalls {
		return nil, errors.New("injected failure")
	}
	if !f.disk {
		return nil, nil
	}
	if f.multipleOrphans {
		return []diskItem{{ID: 45, Name: name}, {ID: 46, Name: name}}, nil
	}
	return []diskItem{{ID: 45, Name: name}}, nil
}

// fetchServer は後片付けの再照合と起動待ちのポーリングを再現します。
func (f *fakeServerCommands) fetchServer(t *testing.T, id types.ID) (serverItem, error) {
	f.calls = append(f.calls, "fetchServer")
	if f.server == nil || f.server.ID != id {
		return serverItem{}, errors.New("missing Server")
	}
	item := *f.server
	if f.changeIdentityAtCleanup && f.updateCalls >= 2 {
		// 更新と復元の後の再照合でのみ識別情報が変わった状態を再現する
		item.Description = "changed identity"
	}
	return item, nil
}

// fetchDisk は孤立ディスクの参照を再現します。
func (f *fakeServerCommands) fetchDisk(t *testing.T, id types.ID) (diskItem, error) {
	if err := f.record("fetchDisk"); err != nil {
		return diskItem{}, err
	}
	disk := diskItem{ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024}
	if f.orphanDisk != nil {
		disk = *f.orphanDisk
	}
	return disk, nil
}

// noSleep は単体テストで待機を省略する sleep 関数です。
func noSleep(time.Duration) {}

// testServerRun は単体テスト用の serverRun を組み立てます。
func testServerRun() *serverRun {
	return &serverRun{
		name:       namePrefix + "test",
		params:     serverDiskParams{ArchiveID: 234, DiskPlanID: 4, SizeGB: 20},
		material:   helper.SSHMaterial{UserDataPath: "/tmp/user-data.yaml"},
		sshTimeout: 20 * time.Millisecond,
	}
}

// callIndex は指定した操作が最初に実行された位置を返します。実行されていない場合は -1 を返します。
func callIndex(calls []string, want string) int {
	for i, call := range calls {
		if call == want {
			return i
		}
	}
	return -1
}
