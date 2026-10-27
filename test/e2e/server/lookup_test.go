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
	"github.com/stretchr/testify/require"
)

// このファイルは事前確認と後片付けの確認で使う SDK の操作です。
// これらの操作は証跡に残さず静かに実行します。

// lookupServers は SDK で名前部分一致検索を行い、全ページの結果を返します。
func lookupServers(t *testing.T, servers *serverSDK.Service, name string) ([]serverItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]serverItem, error) {
		found, err := servers.FindWithContext(ctx, &serverSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]serverItem, 0, len(found))
		for _, s := range found {
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
func lookupDisks(t *testing.T, disks *diskSDK.Service, name string) ([]diskItem, error) {
	t.Helper()

	items, err := paginateFind(t, func(ctx context.Context, from int) ([]diskItem, error) {
		found, err := disks.FindWithContext(ctx, &diskSDK.FindRequest{
			Zone:  zone,
			Names: []string{name},
			Count: findPageSize,
			From:  from,
		})
		if err != nil {
			return nil, err
		}
		items := make([]diskItem, 0, len(found))
		for _, d := range found {
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
func fetchServer(t *testing.T, servers *serverSDK.Service, id types.ID) (serverItem, error) {
	t.Helper()

	server, err := servers.ReadWithContext(t.Context(), &serverSDK.ReadRequest{Zone: zone, ID: id})
	if err != nil {
		return serverItem{}, fmt.Errorf("read Server %s: %w", id, err)
	}
	return serverFromSDK(server), nil
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
