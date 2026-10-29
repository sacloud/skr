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

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// internetZone はルータ＋スイッチを扱う E2E で使用するゾーンです。
const internetZone = "is1b"

// internetNamePrefix はテスト用ルータ＋スイッチの名前の接頭辞です。ランダムな接尾辞とともに
// 使い、過去の実行のリソースと衝突しないようにします。
const internetNamePrefix = "skr-e2e-internet-"

// internetDesc はテスト用ルータ＋スイッチに付ける固定の説明です。後片て付けの識別にも使います。
const internetDesc = "Temporary Internet for skr router-switch SSH E2E"

// serverDesc はテスト用 Server に付ける固定の説明です。後片て付けの識別にも使います。
const serverDesc = "Temporary Server for skr router-switch SSH E2E"

// internetItem はルータ＋スイッチの参照結果です。
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

// parseInternet はルータ＋スイッチの参照応答を解析します。ID が空の場合はエラーを返します。
func parseInternet(data []byte) (internetItem, error) {
	var item internetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return internetItem{}, fmt.Errorf("decode Internet: %w", err)
	}
	if item.ID.IsEmpty() {
		return internetItem{}, errors.New("Internet ID is empty")
	}
	return item, nil
}

// decodeInternetList はルータ＋スイッチの検索応答を解析します。
// 応答が配列でない場合はエラーを返します。
func decodeInternetList(data []byte) ([]internetItem, error) {
	var items []internetItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode Internet list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of Internet")
	}
	return items, nil
}

// checkInternetIdentity は参照結果が期待するルータ＋スイッチ (ID・名前・説明・プレフィクス長・
// 帯域幅・接続先 Switch) と一致することを確認します。
func checkInternetIdentity(item internetItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != internetDesc {
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

// internetFromSDK は SDK のルータ＋スイッチを参照結果の型へ変換します。
func internetFromSDK(item *iaas.Internet) internetItem {
	var converted internetItem
	if item == nil {
		return converted
	}
	converted.ID = item.ID
	converted.Name = item.Name
	converted.Description = item.Description
	converted.NetworkMaskLen = item.NetworkMaskLen
	converted.BandWidthMbps = item.BandWidthMbps
	if item.Switch != nil {
		converted.Switch.ID = item.Switch.ID
	}
	return converted
}

// fetchInternet は SDK でルータ＋スイッチを参照します。後片て付けの再照合など、
// 証跡に残さない安全確認で使います。
func fetchInternet(t *testing.T, internets *internetSDK.Service, id types.ID) (internetItem, error) {
	t.Helper()

	item, err := internets.ReadWithContext(t.Context(), &internetSDK.ReadRequest{Zone: internetZone, ID: id})
	if err != nil {
		return internetItem{}, fmt.Errorf("read Internet %s: %w", id, err)
	}
	return internetFromSDK(item), nil
}

// lookupInternets は SDK で名前部分一致検索を行い、結果を参照結果の型で返します。
func lookupInternets(t *testing.T, internets *internetSDK.Service, name string) ([]internetItem, error) {
	t.Helper()

	found, err := internets.FindWithContext(t.Context(), &internetSDK.FindRequest{
		Zone:  internetZone,
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
		items = append(items, internetFromSDK(f))
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
// Server と Disk の後片て付けのあとに呼び出します。削除前に SDK で識別情報を再照合し、
// 一致した場合のみ skr CLI で削除します。削除後は名前検索で不在を確認します。
// ルータ＋スイッチの削除は IPv6 の無効化と NextHop 付き追加サブネットの削除も行うため、
// IPv6 を有効化したままで削除します。
func cleanupInternet(t *testing.T, h *helper.Helper, internets *internetSDK.Service, name string, id, switchID types.ID) error {
	t.Helper()

	if id.IsEmpty() {
		return nil
	}
	current, err := fetchInternet(t, internets, id)
	if err != nil {
		return fmt.Errorf("cannot verify Internet before cleanup: %w", err)
	}
	if err := checkInternetIdentity(current, id, name, switchID); err != nil {
		return fmt.Errorf("cleanup refused: %w; manual cleanup required", err)
	}
	_, _, err = h.CallAllowingFailure(t, "テストで作成したルータ＋スイッチを削除する",
		"iaas-api", "internet", "delete",
		"--zone", internetZone, "--id", id.String(),
		"--force", "--fail-if-not-found",
	)
	if err != nil {
		// 削除は非同期で、応答の喪失後にリトライが 404 になることがある。
		// 実際に削除されたかを SDK で確認し、残っている場合だけ削除の失敗として扱う。
		if _, fetchErr := fetchInternet(t, internets, id); fetchErr == nil {
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

// TestParseInternet はルータ＋スイッチの参照応答の解析を確認します。
func TestParseInternet(t *testing.T) {
	item, err := parseInternet([]byte(`{"ID":12,"Name":"skr-e2e-internet-test","Description":"d","NetworkMaskLen":28,"BandWidthMbps":100,"Switch":{"ID":13}}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(12), item.ID)
	require.Equal(t, "skr-e2e-internet-test", item.Name)
	require.Equal(t, 28, item.NetworkMaskLen)
	require.Equal(t, 100, item.BandWidthMbps)
	require.Equal(t, types.ID(13), item.Switch.ID)

	_, err = parseInternet([]byte(`{"Name":"no-id"}`))
	require.Error(t, err, "Internet ID is required")

	_, err = parseInternet([]byte("{"))
	require.Error(t, err, "invalid JSON should fail")
}

// TestDecodeInternetList はルータ＋スイッチの検索応答の解析を確認します。
func TestDecodeInternetList(t *testing.T) {
	items, err := decodeInternetList([]byte(`[{"ID":12,"Name":"skr-e2e-internet-test"}]`))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, types.ID(12), items[0].ID)

	_, err = decodeInternetList([]byte(`{"ID":12}`))
	require.Error(t, err, "expected a JSON array")

	_, err = decodeInternetList([]byte("{"))
	require.Error(t, err, "invalid JSON should fail")
}

// TestCheckInternetIdentity はルータ＋スイッチの識別情報の確認を確認します。
func TestCheckInternetIdentity(t *testing.T) {
	item := internetItem{ID: 12, Name: "skr-e2e-internet-test", Description: internetDesc, NetworkMaskLen: 28, BandWidthMbps: 100}
	item.Switch.ID = 13

	require.NoError(t, checkInternetIdentity(item, 12, "skr-e2e-internet-test", 13))

	mismatch := item
	mismatch.BandWidthMbps = 200
	require.Error(t, checkInternetIdentity(mismatch, 12, "skr-e2e-internet-test", 13),
		"bandwidth should match the create request")

	noSwitch := item
	noSwitch.Switch.ID = 0
	require.Error(t, checkInternetIdentity(noSwitch, 12, "skr-e2e-internet-test", 13),
		"the Switch connection should match the create request")

	differentName := item
	differentName.Name = "skr-e2e-internet-other"
	require.Error(t, checkInternetIdentity(differentName, 12, "skr-e2e-internet-test", 13),
		"the name should match the create request")
}

// TestInternetFromSDK は SDK のルータ＋スイッチから参照結果の型への変換を確認します。
func TestInternetFromSDK(t *testing.T) {
	require.Equal(t, internetItem{}, internetFromSDK(nil))

	item := internetFromSDK(&iaas.Internet{
		ID: 12, Name: "skr-e2e-internet-test", Description: internetDesc,
		NetworkMaskLen: 28, BandWidthMbps: 100,
		Switch: &iaas.SwitchInfo{ID: 13},
	})
	require.Equal(t, types.ID(12), item.ID)
	require.Equal(t, types.ID(13), item.Switch.ID)
}
