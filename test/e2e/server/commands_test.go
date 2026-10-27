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
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// serverCommands は Server シナリオが使う Server・Disk の操作と SSH ログイン確認です。
// ライブ実行では helper 経由の skr CLI と SSH、単体テストでは偽の実装を使います。
// すべてのメソッドはエラーを返し、後片付けが途中の失敗でも続行できるようにします。
type serverCommands interface {
	// 検証対象の CLI 操作です。実行記録に説明付きで残ります。

	// createServer は cloud-init ディスクと共有 NIC を持つ Server を作成します。
	createServer(t *testing.T, label, name string, params serverDiskParams) (serverItem, error)
	// bootServer は cloud-init の user-data を指定して Server を起動します。
	bootServer(t *testing.T, label string, id types.ID, userDataPath string) error
	// readServer は Server を参照します。
	readServer(t *testing.T, label string, id types.ID) (serverItem, error)
	// findServers は名前で Server を検索します。
	findServers(t *testing.T, label, name string) ([]serverItem, error)
	// updateServer は Server の名前と説明を更新し、更新結果を返します。
	updateServer(t *testing.T, label string, id types.ID, name, desc string) (serverItem, error)
	// deleteServer は Server をディスクとともに削除します。
	deleteServer(t *testing.T, label string, id types.ID) error
	// deleteDisk は Disk を削除します。
	deleteDisk(t *testing.T, label string, id types.ID) error
	// connectSSH は一時的な鍵でリモートの hostname を取得し、ログインできることを確認します。
	connectSSH(t *testing.T, label, ip string) error

	// SDK で静かに実行する確認です。事前確認と後片付けの確認で使い、証跡には残しません。

	// lookupServers は名前部分一致で Server を検索し、全ページを返します。
	lookupServers(t *testing.T, name string) ([]serverItem, error)
	// lookupDisks は名前部分一致で Disk を検索し、全ページを返します。
	lookupDisks(t *testing.T, name string) ([]diskItem, error)
	// fetchServer は Server を参照します。
	fetchServer(t *testing.T, id types.ID) (serverItem, error)
	// fetchDisk は Disk を参照します。
	fetchDisk(t *testing.T, id types.ID) (diskItem, error)
}

// skrServerCommands は helper 経由で skr CLI と SSH を実行し、確認系は SDK で直接実行する操作です。
type skrServerCommands struct {
	h        *helper.Helper
	material helper.SSHMaterial
	user     string
	// servers と disks は事前確認と後片付けの確認で使う SDK のサービスです。
	// helper.Helper が設定した skr-e2e プロファイルの認証を使います。
	servers *serverSDK.Service
	disks   *diskSDK.Service
}

// newSKRServerCommands は skr CLI と SDK の両方を使う操作を組み立てます。
// SDK クライアントは skr CLI と同じ認証設定 (プロファイル) を使います。
func newSKRServerCommands(h *helper.Helper, material helper.SSHMaterial, user string) (skrServerCommands, error) {
	client, err := iaasclient.New(false)
	if err != nil {
		return skrServerCommands{}, fmt.Errorf("create IaaS API client for verification: %w", err)
	}
	return skrServerCommands{
		h:        h,
		material: material,
		user:     user,
		servers:  serverSDK.New(client),
		disks:    diskSDK.New(client),
	}, nil
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

func (c skrServerCommands) findServers(t *testing.T, label, name string) ([]serverItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, findServerArgs(name)...)
	if err != nil {
		return nil, err
	}
	return decodeServerList(data)
}

func (c skrServerCommands) updateServer(t *testing.T, label string, id types.ID, name, desc string) (serverItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, updateServerArgs(id, name, desc)...)
	if err != nil {
		return serverItem{}, err
	}
	return parseServer(data)
}

func (c skrServerCommands) deleteServer(t *testing.T, label string, id types.ID) error {
	_, _, err := c.h.CallAllowingFailure(t, label, deleteServerArgs(id)...)
	return err
}

func (c skrServerCommands) deleteDisk(t *testing.T, label string, id types.ID) error {
	_, _, err := c.h.CallAllowingFailure(t, label, deleteDiskArgs(id)...)
	return err
}

func (c skrServerCommands) connectSSH(t *testing.T, label, ip string) error {
	return c.h.ConnectSSH(t, c.material, c.user, ip, label)
}

// lookupServers は SDK で名前部分一致検索を行い、全ページの結果を返します。
func (c skrServerCommands) lookupServers(t *testing.T, name string) ([]serverItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]serverItem, error) {
		servers, err := c.servers.FindWithContext(ctx, &serverSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]serverItem, 0, len(servers))
		for _, s := range servers {
			if s == nil {
				continue
			}
			items = append(items, serverFromSDK(s))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Servers by name: %w", err)
	}
	return items, nil
}

// lookupDisks は SDK で名前部分一致検索を行い、全ページの結果を返します。
func (c skrServerCommands) lookupDisks(t *testing.T, name string) ([]diskItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]diskItem, error) {
		disks, err := c.disks.FindWithContext(ctx, &diskSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]diskItem, 0, len(disks))
		for _, d := range disks {
			if d == nil {
				continue
			}
			items = append(items, diskFromSDK(d))
		}
		return items, nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Disks by name: %w", err)
	}
	return items, nil
}

// fetchServer は SDK で Server を参照します。
func (c skrServerCommands) fetchServer(t *testing.T, id types.ID) (serverItem, error) {
	t.Helper()

	server, err := c.servers.ReadWithContext(t.Context(), &serverSDK.ReadRequest{Zone: zone, ID: id})
	if err != nil {
		return serverItem{}, fmt.Errorf("read Server %s: %w", id, err)
	}
	return serverFromSDK(server), nil
}

// fetchDisk は SDK で Disk を参照します。
func (c skrServerCommands) fetchDisk(t *testing.T, id types.ID) (diskItem, error) {
	t.Helper()

	disk, err := c.disks.ReadWithContext(t.Context(), &diskSDK.ReadRequest{Zone: zone, ID: id})
	if err != nil {
		return diskItem{}, fmt.Errorf("read Disk %s: %w", id, err)
	}
	return diskFromSDK(disk), nil
}

// paginateFind は Count/From のページングで全ページを取得します。
// 途中ページの取得に失敗した場合はエラーを返し、不在として扱いません。
func paginateFind[T any](t *testing.T, fetch func(ctx context.Context, from int) ([]T, error)) ([]T, error) {
	t.Helper()

	var all []T
	for from := 0; ; from += findPageSize {
		items, err := fetch(t.Context(), from)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < findPageSize {
			return all, nil
		}
	}
}

// TestFindReadsAllPages は一括検索が全ページを取得することを確認する。
func TestFindReadsAllPages(t *testing.T) {
	calls := 0
	items, err := paginateFind(t, func(ctx context.Context, from int) ([]diskItem, error) {
		calls++
		if from != (calls-1)*findPageSize {
			return nil, fmt.Errorf("unexpected from %d", from)
		}
		if calls == 2 {
			return nil, nil
		}
		return make([]diskItem, findPageSize), nil
	})

	require.NoError(t, err)
	require.Len(t, items, findPageSize, "the first page should be collected")
	require.Equal(t, 2, calls, "the second page should be requested")
}

// TestFindDoesNotTreatPageFailureAsMissing は途中ページの取得に失敗した場合に
// 不在として扱わないことを確認する。
func TestFindDoesNotTreatPageFailureAsMissing(t *testing.T) {
	calls := 0
	_, err := paginateFind(t, func(context.Context, int) ([]diskItem, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("injected failure")
		}
		return make([]diskItem, findPageSize), nil
	})

	require.ErrorContains(t, err, "injected failure", "a page failure should not be treated as no results")
}
