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

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// switchZone はチュートリアルに沿って Sandbox ゾーンで確認するためのゾーンです。
	switchZone = "tk1v"
	// liveZone は実際のゾーンでも同じライフサイクルが動作することを確認するゾーンです。
	liveZone = "is1b"
	// switchNamePrefix はテスト用スイッチの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	switchNamePrefix = "skr-e2e-switch-"
	// switchDescription はテスト用スイッチに付ける説明です。後片付けの識別にも使います。
	switchDescription = "Temporary switch for skr API E2E"
)

// switchItem はスイッチの参照結果のうち、確認に使う項目です。
type switchItem struct {
	ID          types.ID
	Name        string
	Description string
	Tags        []string
}

// TestScenarioSwitch は switch コマンドのライブ E2E です。tk1v の Sandbox ゾーンで、
// スイッチの作成、名前検索、--query での射影、参照、名前とタグの更新、削除と不在確認まで行います。
// チュートリアル (docs/manual/tutorials/iaas-api/switch.md) と同じ手順を確認します。
func TestScenarioSwitch(t *testing.T) {
	h := helper.New(t, "switch-api",
		"IaaS API の switch コマンドでスイッチの作成、検索、参照、更新、削除が正しく動作することを確認する")
	runSwitchScenario(t, h, switchZone, switchNamePrefix+helper.RandomSuffix(t))
}

// TestScenarioSwitchIs1b は is1b でも同じライフサイクルが動作することを確認します。
func TestScenarioSwitchIs1b(t *testing.T) {
	h := helper.New(t, "switch-api",
		"IaaS API の switch コマンドで is1b でもスイッチの作成、検索、参照、更新、削除が正しく動作することを確認する")
	runSwitchScenario(t, h, liveZone, switchNamePrefix+helper.RandomSuffix(t))
}

// runSwitchScenario は1回の実行で、作成前の同名確認、作成、名前検索、--query での射影、
// 参照、名前とタグの更新、削除と不在確認まで行います。作成した ID を保持し、後片付けでは
// 削除前に再参照して識別情報を確認してから削除します。
func runSwitchScenario(t *testing.T, h *helper.Helper, zone, name string) {
	updatedName := name + "-updated"
	createdTags := []string{"tutorial", "managed"}
	updatedTags := []string{"tutorial", "updated"}

	// 作成前に同名スイッチが存在しないことを確認する
	require.Empty(t, findSwitches(t, h, "作成前に同名スイッチが存在しないことを確認する", zone, name),
		"switch name %q already exists; refusing to modify it", name)
	require.Empty(t, findSwitches(t, h, "作成前に更新後の同名スイッチが存在しないことを確認する", zone, updatedName),
		"switch name %q already exists; refusing to modify it", updatedName)

	// スイッチを作成する
	data := h.Call(t, "スイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", zone,
		"--name", name,
		"--description", switchDescription,
		"--tags", "tutorial,managed")
	created := parseSwitch(t, data)
	require.Equal(t, name, created.Name)
	require.Equal(t, switchDescription, created.Description)
	require.ElementsMatch(t, createdTags, created.Tags, "create should return the requested tags")

	defer func() {
		// 削除前に再参照して、今回作成したスイッチであることを確認する
		current := readSwitch(t, h, "削除前にスイッチを再参照する", zone, created.ID)
		require.Equal(t, switchDescription, current.Description, "cleanup should only delete the test switch")
		require.Contains(t, []string{name, updatedName}, current.Name,
			"switch %s has an unexpected name; refusing cleanup", created.ID)
		// スイッチを削除し、作成時と更新後の名前のどちらでも残っていないことを確認する
		_ = h.Call(t, "スイッチを削除する",
			"iaas-api", "switch", "delete", "--zone", zone, "--id", created.ID.String(), "--fail-if-not-found")
		require.Empty(t, findSwitches(t, h, "削除後に作成時の名前のスイッチが残っていないことを確認する", zone, name),
			"switch %s should not remain after deletion", created.ID)
		require.Empty(t, findSwitches(t, h, "削除後に更新後の名前のスイッチが残っていないことを確認する", zone, updatedName),
			"switch %s should not remain after deletion", created.ID)
	}()

	// 作成したスイッチを名前で検索する。Names はフラグで指定できないため --request で JSON を渡す
	found := findSwitches(t, h, "作成したスイッチを名前で検索する", zone, name)
	require.Len(t, found, 1, "switch find should return exactly the created switch %s", created.ID)
	require.Equal(t, created.ID, found[0].ID)

	// --query で ID と名前を射影した一覧を作成結果と照合する
	projected := findSwitchNames(t, h, "--query で ID と名前を射影して検索する", zone, name)
	require.Len(t, projected, 1, "the projected list should contain only the created switch")
	require.Equal(t, created.ID, projected[0].ID)
	require.Equal(t, name, projected[0].Name)

	// 作成したスイッチを参照する
	read := readSwitch(t, h, "作成したスイッチを参照する", zone, created.ID)
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
	readUpdated := readSwitch(t, h, "更新後のスイッチを参照する", zone, created.ID)
	require.Equal(t, updatedName, readUpdated.Name)
	require.Equal(t, switchDescription, readUpdated.Description, "update should not change the omitted description")
	require.ElementsMatch(t, updatedTags, readUpdated.Tags, "read should return the updated tags")
}

// findSwitchRequestJSON は名前検索のリクエスト JSON を組み立てます。
// Names はフラグで指定できないため、JSON 経路で渡します。
func findSwitchRequestJSON(t *testing.T, zone, name string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{"Zone": zone, "Names": []string{name}})
	require.NoError(t, err, "encode switch find request")
	return string(data)
}

// findSwitches は名前でスイッチを検索して応答を解析します。
func findSwitches(t *testing.T, h *helper.Helper, description, zone, name string) []switchItem {
	t.Helper()

	data := h.Call(t, description, "iaas-api", "switch", "find",
		"--request", findSwitchRequestJSON(t, zone, name))
	var items []switchItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode switch list", description)
	require.NotNil(t, items, "%s: expected a JSON array", description)
	return items
}

// findSwitchNames は --query で ID と名前を射影した一覧を取得します。
func findSwitchNames(t *testing.T, h *helper.Helper, description, zone, name string) []switchItem {
	t.Helper()

	data := h.Call(t, description, "iaas-api", "switch", "find",
		"--request", findSwitchRequestJSON(t, zone, name), "--query", "map({ID,Name})")
	var items []switchItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode projected switch list", description)
	require.NotNil(t, items, "%s: expected a JSON array", description)
	return items
}

// readSwitch は指定した ID のスイッチを参照して応答を解析します。
func readSwitch(t *testing.T, h *helper.Helper, description, zone string, id types.ID) switchItem {
	t.Helper()

	data := h.Call(t, description, "iaas-api", "switch", "read", "--zone", zone, "--id", id.String())
	return parseSwitch(t, data)
}

// parseSwitch はスイッチの応答を解析します。ID が空の場合はテストを失敗させます。
func parseSwitch(t *testing.T, data []byte) switchItem {
	t.Helper()

	var item switchItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode switch")
	require.NotZero(t, item.ID, "switch ID is missing")
	return item
}
