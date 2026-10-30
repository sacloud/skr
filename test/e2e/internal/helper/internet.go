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

package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	"github.com/stretchr/testify/require"
)

const (
	// InternetZone はルータ＋スイッチを扱う E2E で使用するゾーンです。
	InternetZone = "is1b"
	// internetNetworkMaskLen はテスト用ルータ＋スイッチに割り当てるネットワークマスク長です。
	internetNetworkMaskLen = 28
	// internetBandWidthMbps はテスト用ルータ＋スイッチの回線帯域 (Mbps) です。
	internetBandWidthMbps = 100
	// InternetCreateTimeout はルータ＋スイッチの作成の期限です。
	// 作成直後の404を最大30分リトライする SDK の待機を許容します。
	InternetCreateTimeout = 35 * time.Minute
)

// InternetItem はルータ＋スイッチの参照結果です。
type InternetItem struct {
	ID             types.ID `json:"ID"`
	Name           string   `json:"Name"`
	Description    string   `json:"Description"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
	BandWidthMbps  int      `json:"BandWidthMbps"`
	Switch         struct {
		ID types.ID `json:"ID"`
	} `json:"Switch"`
}

// IPv6NetItem はルータ＋スイッチの IPv6 アドレス割り当ての参照結果です。
type IPv6NetItem struct {
	ID            types.ID `json:"ID"`
	IPv6Prefix    string   `json:"IPv6Prefix"`
	IPv6PrefixLen int      `json:"IPv6PrefixLen"`
}

// ParseInternetOutput はルータ＋スイッチの作成応答を解析します。ID が空の場合はエラーを返します。
func ParseInternetOutput(data []byte) (InternetItem, error) {
	var item InternetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return InternetItem{}, fmt.Errorf("decode Internet: %w", err)
	}
	if item.ID.IsEmpty() {
		return InternetItem{}, errors.New("Internet ID is empty")
	}
	return item, nil
}

// CheckInternetIdentity はルータ＋スイッチの応答が、作成時に指定した内容と一致することを
// 確認します。
func CheckInternetIdentity(item InternetItem, id types.ID, name, description string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("Internet identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	if item.NetworkMaskLen != internetNetworkMaskLen || item.BandWidthMbps != internetBandWidthMbps {
		return fmt.Errorf("Internet identity mismatch for ID %s (expected network mask len %d and bandwidth %d Mbps)", id, internetNetworkMaskLen, internetBandWidthMbps)
	}
	if item.Switch.ID.IsEmpty() || item.Switch.ID != switchID {
		return fmt.Errorf("Internet %s is not connected to the expected Switch %s", id, switchID)
	}
	return nil
}

// LookupInternets は SDK で名前部分一致検索を行い、結果を作成応答の型で返します。
// 後片て付けでの残存確認など、証跡に残さない安全確認で使います。
func LookupInternets(t *testing.T, internets *internetSDK.Service, zone, name string) ([]InternetItem, error) {
	t.Helper()

	found, err := internets.FindWithContext(t.Context(), &internetSDK.FindRequest{
		Zone:  zone,
		Names: []string{name},
	})
	if err != nil {
		return nil, fmt.Errorf("find Internet by name: %w", err)
	}
	items := make([]InternetItem, 0, len(found))
	for _, f := range found {
		if f == nil {
			continue
		}
		items = append(items, InternetItem{
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

// RequireNoInternetNameCollision は作成前に、指定した名前を含む名前を持つルータ＋スイッチが
// 存在しないことを SDK で確認します。確認の実行は証跡に残らず、既存リソースを変更しません。
// 衝突が見つかった場合は自動削除せず、手動確認を求めて失敗します。
func RequireNoInternetNameCollision(t *testing.T, internets *internetSDK.Service, zone, name string) {
	t.Helper()

	matches, err := LookupInternets(t, internets, zone, name)
	require.NoError(t, err, "verify no Internet name collision before create")
	require.Empty(t, matches, "an Internet matching the E2E name already exists; manual cleanup required")
}

// CreateInternet はテスト用ルータ＋スイッチを skr CLI で作成し、作成応答が期待する内容と
// 一致することを確認します。ネットワークマスク長と回線帯域は internetNetworkMaskLen と
// internetBandWidthMbps を使います。
func CreateInternet(t *testing.T, h *Helper, zone, name, description string) InternetItem {
	t.Helper()

	// ルータ＋スイッチを作成します。作成直後の404を最大30分リトライする SDK の待機を
	// 許容して InternetCreateTimeout の期限を設けます
	data := h.CallWithTimeout(t, InternetCreateTimeout, "ルータ＋スイッチを作成する",
		"iaas-api", "internet", "create",
		"--zone", zone, "--name", name, "--description", description,
		"--network-mask-len", strconv.Itoa(internetNetworkMaskLen),
		"--band-width-mbps", strconv.Itoa(internetBandWidthMbps),
	)
	router, err := ParseInternetOutput(data)
	require.NoError(t, err, "decode created Internet")
	require.NoError(t, CheckInternetIdentity(router, router.ID, name, description, router.Switch.ID),
		"the create response should contain the created Internet")
	return router
}

// EnableIPv6 は skr CLI でルータ＋スイッチの IPv6 アドレス割り当てを有効化します。
// 応答は --query で必要な項目に絞って解析します。
func EnableIPv6(t *testing.T, h *Helper, zone string, internetID types.ID) (IPv6NetItem, error) {
	t.Helper()

	data := h.Call(t, "ルータ＋スイッチの IPv6 アドレス割り当てを有効化する",
		"iaas-api", "internet", "enable-ipv6",
		"--zone", zone, "--id", internetID.String(),
		"--query", "{ID,IPv6Prefix,IPv6PrefixLen}",
	)
	var item IPv6NetItem
	if err := json.Unmarshal(data, &item); err != nil {
		return IPv6NetItem{}, fmt.Errorf("decode IPv6Net: %w", err)
	}
	return item, nil
}

// CheckIPv6NetItem は IPv6 アドレス割り当ての応答が有効な内容を持つことを確認します。
func CheckIPv6NetItem(item IPv6NetItem, internetID types.ID) error {
	if item.ID.IsEmpty() {
		return fmt.Errorf("IPv6Net ID is empty (expected for Internet %s)", internetID)
	}
	if item.IPv6Prefix == "" || item.IPv6PrefixLen == 0 {
		return fmt.Errorf("IPv6Net %s has empty prefix (expected for Internet %s)", item.ID, internetID)
	}
	return nil
}

// CleanupInternet はテストで作成したルータ＋スイッチを後片て付けします。
// 削除前に SDK で識別情報を再照合し、一致した場合のみ skr CLI で削除します。
// 削除後は名前検索で不在を確認します。ルータ＋スイッチの削除は IPv6 の無効化も行うため、
// IPv6 を有効化したままで削除します。
func CleanupInternet(t *testing.T, h *Helper, internets *internetSDK.Service, zone, name, description string, id, switchID types.ID) error {
	t.Helper()

	if id.IsEmpty() {
		return nil
	}
	found, err := LookupInternets(t, internets, zone, name)
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
	if err := CheckInternetIdentity(current, id, name, description, switchID); err != nil {
		return fmt.Errorf("cleanup refused: %w; manual cleanup required", err)
	}
	_, _, err = h.CallAllowingFailure(t, "テストで作成したルータ＋スイッチを削除する",
		"iaas-api", "internet", "delete",
		"--zone", zone, "--id", id.String(),
		"--force", "--fail-if-not-found",
	)
	if err != nil {
		// 削除は非同期で、応答の喪失後にリトライが 404 になることがある。
		// 実際に削除されたかを SDK で確認し、残っている場合だけ削除の失敗として扱う。
		remaining, fetchErr := LookupInternets(t, internets, zone, name)
		if fetchErr == nil && len(remaining) > 0 {
			return fmt.Errorf("delete Internet %s: %w", id, err)
		}
	}
	matches, err := LookupInternets(t, internets, zone, name)
	if err != nil {
		return fmt.Errorf("verify Internet absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("Internet %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}
