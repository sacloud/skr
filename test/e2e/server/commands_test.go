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
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// serverCommands は Server シナリオが使う Server・Disk の操作と SSH ログイン確認です。
// ライブ実行では helper 経由の skr CLI と SSH を、単体テストでは偽の実装を使います。
// すべてのメソッドはエラーを返し、後片付けが途中の失敗でも続行できるようにします。
type serverCommands interface {
	// findServers は名前で Server を検索し、全ページを返します。
	findServers(t *testing.T, label, name string) ([]serverItem, error)
	// findDisks は名前で Disk を検索し、全ページを返します。
	findDisks(t *testing.T, label, name string) ([]diskItem, error)
	// createServer は cloud-init ディスクと共有 NIC を持つ Server を作成します。
	createServer(t *testing.T, label, name string, params serverDiskParams) (serverItem, error)
	// bootServer は cloud-init の user-data を指定して Server を起動します。
	bootServer(t *testing.T, label string, id types.ID, userDataPath string) error
	// readServer は Server を参照します。
	readServer(t *testing.T, label string, id types.ID) (serverItem, error)
	// updateServer は Server の名前と説明を更新します。
	updateServer(t *testing.T, label string, id types.ID, name, desc string) error
	// deleteServer は Server をディスクとともに削除します。
	deleteServer(t *testing.T, label string, id types.ID) error
	// readDisk は Disk を参照します。
	readDisk(t *testing.T, label string, id types.ID) (diskItem, error)
	// deleteDisk は Disk を削除します。
	deleteDisk(t *testing.T, label string, id types.ID) error
	// connectSSH は一時的な鍵でリモートの hostname を取得し、ログインできることを確認します。
	connectSSH(t *testing.T, label, ip string) error
}

// skrServerCommands は helper 経由で skr CLI と SSH を実行する Server・Disk 操作です。
type skrServerCommands struct {
	h        *helper.Helper
	material helper.SSHMaterial
	user     string
}

func newSKRServerCommands(h *helper.Helper, material helper.SSHMaterial, user string) skrServerCommands {
	return skrServerCommands{h: h, material: material, user: user}
}

func (c skrServerCommands) findServers(t *testing.T, label, name string) ([]serverItem, error) {
	return paginateFind(t, label, func(from int) ([]serverItem, error) {
		data, _, err := c.h.CallAllowingFailure(t, findPageLabel(label, from), findServerArgs(name, from)...)
		if err != nil {
			return nil, err
		}
		return decodeServerList(data)
	})
}

func (c skrServerCommands) findDisks(t *testing.T, label, name string) ([]diskItem, error) {
	return paginateFind(t, label, func(from int) ([]diskItem, error) {
		data, _, err := c.h.CallAllowingFailure(t, findPageLabel(label, from), findDiskArgs(name, from)...)
		if err != nil {
			return nil, err
		}
		return decodeDiskList(data)
	})
}

func (c skrServerCommands) createServer(t *testing.T, label, name string, params serverDiskParams) (serverItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, createServerArgs(name, params)...)
	if err != nil {
		return serverItem{}, err
	}
	return parseServer(data)
}

func (c skrServerCommands) bootServer(t *testing.T, label string, id types.ID, userDataPath string) error {
	_, _, err := c.h.CallAllowingFailure(t, label, bootServerArgs(id, userDataPath)...)
	return err
}

func (c skrServerCommands) readServer(t *testing.T, label string, id types.ID) (serverItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, readServerArgs(id)...)
	if err != nil {
		return serverItem{}, err
	}
	return parseServer(data)
}

func (c skrServerCommands) updateServer(t *testing.T, label string, id types.ID, name, desc string) error {
	_, _, err := c.h.CallAllowingFailure(t, label, updateServerArgs(id, name, desc)...)
	return err
}

func (c skrServerCommands) deleteServer(t *testing.T, label string, id types.ID) error {
	_, _, err := c.h.CallAllowingFailure(t, label, deleteServerArgs(id)...)
	return err
}

func (c skrServerCommands) readDisk(t *testing.T, label string, id types.ID) (diskItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, readDiskArgs(id)...)
	if err != nil {
		return diskItem{}, err
	}
	return parseDisk(data)
}

func (c skrServerCommands) deleteDisk(t *testing.T, label string, id types.ID) error {
	_, _, err := c.h.CallAllowingFailure(t, label, deleteDiskArgs(id)...)
	return err
}

func (c skrServerCommands) connectSSH(t *testing.T, label, ip string) error {
	return c.h.ConnectSSH(t, c.material, c.user, ip, label)
}

// findPageLabel は2ページ目以降の検索にページ位置を示す説明を追加します。
func findPageLabel(label string, from int) string {
	if from == 0 {
		return label
	}
	return fmt.Sprintf("%s（%d 件目以降）", label, from+1)
}

// paginateFind は Count/From のページングで全ページを取得します。
// 途中ページの取得に失敗した場合はエラーを返し、不在として扱いません。
func paginateFind[T any](t *testing.T, label string, fetch func(from int) ([]T, error)) ([]T, error) {
	t.Helper()

	var all []T
	for from := 0; ; from += findPageSize {
		items, err := fetch(from)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < findPageSize {
			return all, nil
		}
	}
}
