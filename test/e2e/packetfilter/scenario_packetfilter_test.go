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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// zone はパケットフィルタの CRUD を確認するゾーンです。
	zone = "is1b"
	// namePrefix はテスト用パケットフィルタの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	namePrefix = "skr-e2e-packetfilter-"
	// description はテスト用パケットフィルタに付ける説明です。後片付けの識別にも使います。
	description = "Temporary PacketFilter for skr API E2E"
	// findPageSize は SDK の安全確認で使う1ページの件数です。
	findPageSize = 100
)

// packetFilterExpression はパケットフィルタのルールで確認する項目です。
type packetFilterExpression struct {
	Protocol        string
	SourceNetwork   string
	SourcePort      string
	DestinationPort string
	Action          string
	Description     string
}

// packetFilterItem はパケットフィルタの参照結果のうち、確認に使う項目です。
type packetFilterItem struct {
	ID             types.ID
	Name           string
	Description    string
	Expression     []packetFilterExpression
	ExpressionHash string
}

// TestScenarioPacketFilter は packetfilter コマンドのライブ E2E です。is1b で、
// パケットフィルタの作成、名前検索、--query での射影、参照、ルールと名前の更新、
// 削除と不在確認まで行います。パケットフィルタは NIC に接続しません。
func TestScenarioPacketFilter(t *testing.T) {
	h := helper.New(t, "packetfilter-api", "IaaS パケットフィルタを作成・検索・参照・更新・削除し、リソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := namePrefix + suffix
	updatedName := name + "-updated"
	firstExpression := packetFilterExpression{
		Protocol:        "tcp",
		SourceNetwork:   "192.0.2.0/24",
		DestinationPort: "443",
		Action:          "allow",
		Description:     "E2E initial rule",
	}
	updatedExpression := packetFilterExpression{
		Protocol:        "tcp",
		SourceNetwork:   "192.0.2.0/24",
		DestinationPort: "8443",
		Action:          "allow",
		Description:     "E2E updated rule",
	}

	// 作成前の同名確認を実施する
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	require.Empty(t, lookupPacketFilters(t, client, name), "packet filter name %q already exists; refusing to modify it", name)
	require.Empty(t, lookupPacketFilters(t, client, updatedName), "packet filter name %q already exists; refusing to modify it", updatedName)

	// パケットフィルタを作成する
	data := h.Call(t, "パケットフィルタを作成する",
		"iaas-api", "packetfilter", "create",
		"--zone", zone,
		"--name", name,
		"--description", description,
		"--expressions", expressionsJSON(t, firstExpression))
	created := parsePacketFilter(t, data)
	require.Equal(t, name, created.Name)
	require.Equal(t, description, created.Description)
	require.Len(t, created.Expression, 1, "create should return the requested rule")
	require.Equal(t, firstExpression, created.Expression[0])

	defer func() {
		// 削除前に保持した ID と説明を再確認する
		current := readPacketFilter(t, h, "削除前にパケットフィルタを再確認する", created.ID)
		require.Equal(t, description, current.Description, "packet filter identity changed before cleanup")
		// パケットフィルタを削除し、作成時と更新後の名前のどちらでも残っていないことを確認する
		_ = h.Call(t, "パケットフィルタを削除する", "iaas-api", "packetfilter", "delete", "--zone", zone, "--id", created.ID.String(), "--fail-if-not-found")
		require.Empty(t, findPacketFilters(t, h, "削除後に作成時の名前が残っていないことを確認する", name), "packet filter %s should not remain after deletion", created.ID)
		require.Empty(t, findPacketFilters(t, h, "削除後に更新後の名前が残っていないことを確認する", updatedName), "packet filter %s should not remain after deletion", created.ID)
	}()

	// 作成したパケットフィルタを名前で検索する。Names はフラグで指定できないため --request で JSON を渡す
	found := findPacketFilters(t, h, "作成したパケットフィルタを名前で検索する", name)
	require.Len(t, found, 1, "packet filter find should return exactly the created filter %s", created.ID)
	require.Equal(t, created.ID, found[0].ID)

	// --query で ID と名前を射影した一覧を作成結果と照合する
	projected := findPacketFilterNames(t, h, "--query で ID と名前を射影して検索する", name)
	require.Len(t, projected, 1, "the projected list should contain only the created packet filter")
	require.Equal(t, created.ID, projected[0].ID)
	require.Equal(t, name, projected[0].Name)

	// 作成したパケットフィルタを参照する
	read := readPacketFilter(t, h, "作成したパケットフィルタを参照する", created.ID)
	require.Equal(t, name, read.Name)
	require.Equal(t, []packetFilterExpression{firstExpression}, read.Expression)

	// ルールと名前を更新する。ルールを更新するため read で取得した ExpressionHash を指定する
	data = h.Call(t, "パケットフィルタのルールと名前を更新する",
		"iaas-api", "packetfilter", "update",
		"--zone", zone, "--id", created.ID.String(),
		"--name", updatedName,
		"--original-expression-hash", read.ExpressionHash,
		"--expressions", expressionsJSON(t, updatedExpression))
	updated := parsePacketFilter(t, data)
	require.Equal(t, updatedName, updated.Name)
	require.Equal(t, []packetFilterExpression{updatedExpression}, updated.Expression)

	// 更新後のパケットフィルタを参照する
	readUpdated := readPacketFilter(t, h, "更新後のパケットフィルタを参照する", created.ID)
	require.Equal(t, updatedName, readUpdated.Name)
	require.Equal(t, []packetFilterExpression{updatedExpression}, readUpdated.Expression)
}

// findPacketFilterRequestJSON は名前検索のリクエスト JSON を組み立てます。
// Names はフラグで指定できないため、JSON 経路で渡します。
func findPacketFilterRequestJSON(t *testing.T, name string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{"Zone": zone, "Names": []string{name}})
	require.NoError(t, err, "encode packet filter find request")
	return string(data)
}

// expressionsJSON はルール 1 件の JSON 配列を組み立てます。
func expressionsJSON(t *testing.T, expr packetFilterExpression) string {
	t.Helper()

	data, err := json.Marshal([]packetFilterExpression{expr})
	require.NoError(t, err, "encode packet filter expressions")
	return string(data)
}

// findPacketFilters は名前でパケットフィルタを検索して応答を解析します。
func findPacketFilters(t *testing.T, h *helper.Helper, description, name string) []packetFilterItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "packetfilter", "find", "--request", findPacketFilterRequestJSON(t, name))
	var items []packetFilterItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode packet filters: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON packet filter array", description)
	return items
}

// findPacketFilterNames は --query で ID と名前を射影した一覧を取得します。
func findPacketFilterNames(t *testing.T, h *helper.Helper, description, name string) []packetFilterItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "packetfilter", "find", "--request", findPacketFilterRequestJSON(t, name), "--query", "map({ID,Name})")
	var items []packetFilterItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode projected packet filters: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON packet filter array", description)
	return items
}

// readPacketFilter は指定した ID のパケットフィルタを参照して応答を解析します。
func readPacketFilter(t *testing.T, h *helper.Helper, description string, id types.ID) packetFilterItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "packetfilter", "read", "--zone", zone, "--id", id.String())
	return parsePacketFilter(t, data)
}

// parsePacketFilter はパケットフィルタの応答を解析します。ID が空の場合はテストを失敗させます。
func parsePacketFilter(t *testing.T, data []byte) packetFilterItem {
	t.Helper()

	var item packetFilterItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode packet filter: %v", err)
	require.NotZero(t, item.ID, "packet filter ID is missing")
	return item
}

// lookupPacketFilters は SDK で名前でパケットフィルタを検索します。
func lookupPacketFilters(t *testing.T, client *iaas.Client, name string) []packetFilterItem {
	t.Helper()

	filters, err := packetfilter.New(client).FindWithContext(t.Context(), &packetfilter.FindRequest{
		Zone:  zone,
		Names: []string{name},
		Count: findPageSize,
	})
	require.NoError(t, err, "find packet filters by name %q", name)
	items := make([]packetFilterItem, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			continue
		}
		items = append(items, packetFilterItem{ID: f.ID, Name: f.Name})
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), findPageSize, "packet filter find result page is full; cannot conclude absence of %q", name)
	return items
}
