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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	swytch "github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// zone はスイッチの CRUD を確認するゾーンです。
	zone = "is1b"
	// namePrefix はテスト用スイッチの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	namePrefix = "skr-e2e-switch-"
	// description はテスト用スイッチに付ける説明です。後片付けの識別にも使います。
	description = "Temporary switch for skr API E2E"
	// findPageSize は SDK の安全確認で使う1ページの件数です。
	findPageSize = 100
)

// switchItem はスイッチの参照結果のうち、確認に使う項目です。
type switchItem struct {
	ID          types.ID
	Name        string
	Description string
	Tags        []string
}

// TestScenarioSwitch は switch コマンドのライブ E2E です。is1b で、スイッチの作成、
// 名前検索、--query での射影、参照、名前とタグの更新、削除と不在確認まで行います。
func TestScenarioSwitch(t *testing.T) {
	h := helper.New(t, "switch-api", "IaaS スイッチを作成・検索・参照・更新・削除し、スイッチリソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := namePrefix + suffix
	updatedName := name + "-updated"
	createdTags := []string{"tutorial", "managed"}
	updatedTags := []string{"tutorial", "updated"}

	// 作成前の同名確認を実施する
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	require.Empty(t, lookupSwitches(t, client, name), "switch name %q already exists; refusing to modify it", name)
	require.Empty(t, lookupSwitches(t, client, updatedName), "switch name %q already exists; refusing to modify it", updatedName)

	// スイッチを作成する
	data := h.Call(t, "スイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", zone,
		"--name", name,
		"--description", description,
		"--tags", "tutorial,managed")
	created := parseSwitch(t, data)
	require.Equal(t, name, created.Name)
	require.Equal(t, description, created.Description)
	require.ElementsMatch(t, createdTags, created.Tags, "create should return the requested tags")

	defer func() {
		// スイッチを削除し、作成時と更新後の名前のどちらでも残っていないことを確認する
		_ = h.Call(t, "スイッチを削除する", "iaas-api", "switch", "delete", "--zone", zone, "--id", created.ID.String(), "--fail-if-not-found")
		require.Empty(t, findSwitches(t, h, "削除後に作成時の名前のスイッチが残っていないことを確認する", name), "switch %s should not remain after deletion", created.ID)
		require.Empty(t, findSwitches(t, h, "削除後に更新後の名前のスイッチが残っていないことを確認する", updatedName), "switch %s should not remain after deletion", created.ID)
	}()

	// 作成したスイッチを名前で検索する。Names はフラグで指定できないため --request で JSON を渡す
	found := findSwitches(t, h, "作成したスイッチを名前で検索する", name)
	require.Len(t, found, 1, "switch find should return exactly the created switch %s", created.ID)
	require.Equal(t, created.ID, found[0].ID)

	// --query で ID と名前を射影した一覧を作成結果と照合する
	projected := findSwitchNames(t, h, "--query で ID と名前を射影して検索する", name)
	require.Len(t, projected, 1, "the projected list should contain only the created switch")
	require.Equal(t, created.ID, projected[0].ID)
	require.Equal(t, name, projected[0].Name)

	// 作成したスイッチを参照する
	read := readSwitch(t, h, "作成したスイッチを参照する", created.ID)
	require.Equal(t, name, read.Name)

	// スイッチの名前とタグを更新する。説明は指定しないため変更されない
	data = h.Call(t, "スイッチの名前とタグを更新する",
		"iaas-api", "switch", "update",
		"--zone", zone, "--id", created.ID.String(),
		"--name", updatedName,
		"--tags", "tutorial,updated")
	updated := parseSwitch(t, data)
	require.Equal(t, updatedName, updated.Name)
	require.ElementsMatch(t, updatedTags, updated.Tags, "update should return the requested tags")

	// 更新後のスイッチを参照する。省略した説明が変更されていないことも確認する
	readUpdated := readSwitch(t, h, "更新後のスイッチを参照する", created.ID)
	require.Equal(t, updatedName, readUpdated.Name)
	require.Equal(t, description, readUpdated.Description, "update should not change the omitted description")
	require.ElementsMatch(t, updatedTags, readUpdated.Tags, "read should return the updated tags")
}

// findSwitchRequestJSON は名前検索のリクエスト JSON を組み立てます。
// Names はフラグで指定できないため、JSON 経路で渡します。
func findSwitchRequestJSON(t *testing.T, name string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{"Zone": zone, "Names": []string{name}})
	require.NoError(t, err, "encode switch find request")
	return string(data)
}

// findSwitches は名前でスイッチを検索して応答を解析します。
func findSwitches(t *testing.T, h *helper.Helper, description, name string) []switchItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "switch", "find", "--request", findSwitchRequestJSON(t, name))
	var items []switchItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode switches: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON switch array", description)
	return items
}

// findSwitchNames は --query で ID と名前を射影した一覧を取得します。
func findSwitchNames(t *testing.T, h *helper.Helper, description, name string) []switchItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "switch", "find", "--request", findSwitchRequestJSON(t, name), "--query", "map({ID,Name})")
	var items []switchItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode projected switches: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON switch array", description)
	return items
}

// readSwitch は指定した ID のスイッチを参照して応答を解析します。
func readSwitch(t *testing.T, h *helper.Helper, description string, id types.ID) switchItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "switch", "read", "--zone", zone, "--id", id.String())
	return parseSwitch(t, data)
}

// parseSwitch はスイッチの応答を解析します。ID が空の場合はテストを失敗させます。
func parseSwitch(t *testing.T, data []byte) switchItem {
	t.Helper()

	var item switchItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode switch: %v", err)
	require.NotZero(t, item.ID, "switch ID is missing")
	return item
}

// lookupSwitches は SDK で名前でスイッチを検索します。
func lookupSwitches(t *testing.T, client *iaas.Client, name string) []switchItem {
	t.Helper()

	switches, err := swytch.New(client).FindWithContext(t.Context(), &swytch.FindRequest{
		Zone:  zone,
		Names: []string{name},
		Count: findPageSize,
	})
	require.NoError(t, err, "find switches by name %q", name)
	items := make([]switchItem, 0, len(switches))
	for _, s := range switches {
		if s == nil {
			continue
		}
		items = append(items, switchFromSDK(s))
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), findPageSize, "switch find result page is full; cannot conclude absence of %q", name)
	return items
}

// switchFromSDK は SDK の Switch を確認用の項目へ変換します。
func switchFromSDK(s *iaas.Switch) switchItem {
	return switchItem{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Tags:        []string(s.Tags),
	}
}
