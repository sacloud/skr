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
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// ipv6NetZone は IPv6 ネットワークを扱う E2E で使用するゾーンです。
const ipv6NetZone = "is1b"

// ipv6NetNamePrefix はテスト用ルータ＋スイッチの名前の接頭辞です。ランダムな接尾辞とともに
// 使い、過去の実行のリソースと衝突しないようにします。
const ipv6NetNamePrefix = "skr-e2e-ipv6net-"

// ipv6NetDesc はテスト用ルータ＋スイッチに付ける固定の説明です。後片て付けの識別にも使います。
const ipv6NetDesc = "Temporary Internet for skr ipv6net E2E"

// ipv6NetCreateTimeout はルータ＋スイッチの作成の期限です。
// 作成直後の404を最大30分リトライする SDK の待機を許容します。
const ipv6NetCreateTimeout = 35 * time.Minute

// internetItem はルータ＋スイッチの作成応答です。
type internetItem struct {
	ID             types.ID `json:"ID"`
	Name           string   `json:"Name"`
	Description    string   `json:"Description"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
	BandWidthMbps  int      `json:"BandWidthMbps"`
	Switch         struct {
		ID types.ID `json:"ID"`
	} `json:"Switch"`
}

// parseInternetItem はルータ＋スイッチの作成応答を解析します。ID が空の場合はエラーを返します。
func parseInternetItem(data []byte) (internetItem, error) {
	var item internetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return internetItem{}, fmt.Errorf("decode Internet: %w", err)
	}
	if item.ID.IsEmpty() {
		return internetItem{}, errors.New("Internet ID is empty")
	}
	return item, nil
}

// checkInternetIdentity はルータ＋スイッチの応答が、作成時に指定した内容と一致することを
// 確認します。
func checkInternetIdentity(item internetItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != ipv6NetDesc {
		return fmt.Errorf("Internet identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	if item.NetworkMaskLen != 28 || item.BandWidthMbps != 100 {
		return fmt.Errorf("Internet identity mismatch for ID %s (expected network mask len 28 and bandwidth 100 Mbps)", id)
	}
	if item.Switch.ID.IsEmpty() || item.Switch.ID != switchID {
		return fmt.Errorf("Internet %s is not connected to the expected Switch %s", id, switchID)
	}
	return nil
}

// lookupInternets は SDK で名前部分一致検索を行い、結果を作成応答の型で返します。
func lookupInternets(t *testing.T, internets *internetSDK.Service, name string) ([]internetItem, error) {
	t.Helper()

	found, err := internets.FindWithContext(t.Context(), &internetSDK.FindRequest{
		Zone:  ipv6NetZone,
		Names: []string{name},
	})
	if err != nil {
		return nil, fmt.Errorf("find Internet by name: %w", err)
	}
	items := make([]internetItem, 0, len(found))
	for _, f := range found {
		if f == nil {
			continue
		}
		items = append(items, internetItem{
			ID:             f.ID,
			Name:           f.Name,
			Description:    f.Description,
			NetworkMaskLen: f.NetworkMaskLen,
			BandWidthMbps:  f.BandWidthMbps,
		})
		if f.Switch != nil {
			items[len(items)-1].Switch.ID = f.Switch.ID
		}
	}
	return items, nil
}

// requireNoInternetNameCollision は作成前に、指定した名前を含む名前を持つルータ＋スイッチが
// 存在しないことを SDK で確認します。確認の実行は証跡に残らず、既存リソースを変更しません。
// 衝突が見つかった場合は自動削除せず、手動確認を求めて失敗します。
func requireNoInternetNameCollision(t *testing.T, internets *internetSDK.Service, name string) {
	t.Helper()

	matches, err := lookupInternets(t, internets, name)
	require.NoError(t, err, "verify no Internet name collision before create")
	require.Empty(t, matches, "an Internet matching the E2E name already exists; manual cleanup required")
}

// cleanupInternet はテストで作成したルータ＋スイッチを後片て付けします。
// 削除前に SDK で識別情報を再照合し、一致した場合のみ skr CLI で削除します。
// 削除後は名前検索で不在を確認します。ルータ＋スイッチの削除は IPv6 の無効化も行うため、
// IPv6 を有効化したままで削除します。
func cleanupInternet(t *testing.T, h *helper.Helper, internets *internetSDK.Service, name string, id, switchID types.ID) error {
	t.Helper()

	if id.IsEmpty() {
		return nil
	}
	found, err := lookupInternets(t, internets, name)
	if err != nil {
		return fmt.Errorf("cannot verify Internet before cleanup: %w", err)
	}
	if len(found) != 1 {
		return fmt.Errorf("found %d Internet matching the E2E name %q; refusing to delete; manual cleanup required", len(found), name)
	}
	current := found[0]
	if current.ID != id {
		return fmt.Errorf("found Internet ID %s does not match the created ID %s; refusing to delete", current.ID, id)
	}
	if err := checkInternetIdentity(current, id, name, switchID); err != nil {
		return fmt.Errorf("cleanup refused: %w; manual cleanup required", err)
	}
	_, _, err = h.CallAllowingFailure(t, "テストで作成したルータ＋スイッチを削除する",
		"iaas-api", "internet", "delete",
		"--zone", ipv6NetZone, "--id", id.String(),
		"--force", "--fail-if-not-found",
	)
	if err != nil {
		// 削除は非同期で、応答の喪失後にリトライが 404 になることがある。
		// 実際に削除されたかを SDK で確認し、残っている場合だけ削除の失敗として扱う。
		remaining, fetchErr := lookupInternets(t, internets, name)
		if fetchErr == nil && len(remaining) > 0 {
			return fmt.Errorf("delete Internet %s: %w", id, err)
		}
	}
	matches, err := lookupInternets(t, internets, name)
	if err != nil {
		return fmt.Errorf("verify Internet absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("Internet %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}

// TestParseInternetItem はルータ＋スイッチの作成応答の解析を確認します。
func TestParseInternetItem(t *testing.T) {
	item, err := parseInternetItem([]byte(`{"ID":12,"Name":"skr-e2e-ipv6net-test","Description":"d","NetworkMaskLen":28,"BandWidthMbps":100,"Switch":{"ID":13}}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(12), item.ID)
	require.Equal(t, "skr-e2e-ipv6net-test", item.Name)
	require.Equal(t, 28, item.NetworkMaskLen)
	require.Equal(t, 100, item.BandWidthMbps)
	require.Equal(t, types.ID(13), item.Switch.ID)

	_, err = parseInternetItem([]byte(`{"Name":"no-id"}`))
	require.Error(t, err, "response without ID should fail")
}
