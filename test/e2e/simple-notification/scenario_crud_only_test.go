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
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// crudNamePrefix は CRUD シナリオが作成する一時リソース名の接頭辞です。
	crudNamePrefix = "skr-e2e-crud-"
	// crudDescriptionPrefix は CRUD シナリオが作成する一時リソース説明の接頭辞です。
	crudDescriptionPrefix = "skr-e2e-simple-notification-crud/"
	// crudVerifyAttempts は設定変更の反映を確認する再試行回数です。
	crudVerifyAttempts = 10
	// crudVerifyPolling は設定変更の反映を待つ間隔です。
	crudVerifyPolling = time.Second
)

// crudResource は CRUD シナリオが作成した一時リソースを追跡します。
type crudResource struct {
	kind   string
	suffix string
	item   crudItem
}

// TestScenarioSimpleNotificationCRUDOnly は既存メール通知先のアドレスで一時通知先・グループ・ルーティングを
// 作成し、通知を送信せずに作成・参照・更新・削除と不在確認まで行います。既存リソースは変更しません。
func TestScenarioSimpleNotificationCRUDOnly(t *testing.T) {
	h := helper.New(t, "simple-notification-api",
		"既存メール通知先のアドレスで一時通知先・グループ・ルーティングを作成し、通知を送信せずに作成・参照・更新・削除が正しく動作することを確認する")

	destinationID := destinationIDFromEnv(t)
	suffix := helper.RandomSuffix(t)
	name := crudNamePrefix + suffix
	description := crudDescriptionPrefix + suffix

	// 書き込み前に既存のルーティング設定を取得し、後でその優先順位を確認できるようにする
	before := listCRUDItems(t, h, "書き込み前に既存のルーティング設定を取得する", "routing")
	for _, item := range before {
		rank, ok := item.Settings["PriorityRank"].(float64)
		require.True(t, ok, "existing routing %s has no priority rank", item.ID)
		require.Less(t, rank, float64(99),
			"CRUD preflight refused: priority ranks 99 and 100 must be unused")
	}

	// 通知元のリソースを確認する
	sourceID := notificationSourceID(t, h)

	// 既存通知先がメール通知先であることを確認する（既存通知先は変更・削除しない）
	source := readCRUDItem(t, h, "既存通知先を参照する", "destination", destinationID)
	require.Equal(t, "email", source.Settings["Type"], "CRUD requires an existing email destination")
	address, ok := source.Settings["Value"].(string)
	require.True(t, ok, "CRUD requires an existing email destination")
	require.NotEmpty(t, address, "CRUD requires an existing email destination")

	// 後片付け: ルーティング、グループ、通知先の順に削除し、一覧に残っていないことを確認する
	var created []*crudResource
	defer func() {
		cleanupCRUDItems(t, h, created, name)
	}()

	// 一時通知先を作成して設定を変更する
	destination := createCRUDItem(t, h, &created, "destination", name, description, suffix,
		map[string]any{"Type": "email", "Value": address, "Disabled": true})
	updateCRUDItem(t, h, destination, "一時通知先をメタデータのみ更新し、既存設定が維持されることを確認する", nil)
	updateCRUDItem(t, h, destination, "一時通知先を有効化する",
		map[string]any{"Type": "email", "Value": address, "Disabled": false})
	updateCRUDItem(t, h, destination, "一時通知先を無効化する",
		map[string]any{"Type": "email", "Value": address, "Disabled": true})
	updateCRUDItem(t, h, destination, "一時通知先に同じ設定を再指定し、宛先と有効状態が保持されることを確認する",
		destination.item.Settings)

	// 一時通知先の状態を確認する（確認メールへの応答は要求せず、IsValid の有無だけを確認する）
	data := h.Call(t, "一時通知先の状態を確認する",
		"simple-notification-api", "destination", "read-status", destination.item.ID)
	var status struct {
		NotificationStatus *struct {
			IsValid *bool `json:"IsValid"`
		} `json:"NotificationStatus"`
	}
	err := json.Unmarshal(data, &status)
	require.NoError(t, err, "decode destination status: %v", err)
	require.NotNil(t, status.NotificationStatus, "destination status response is missing NotificationStatus")
	require.NotNil(t, status.NotificationStatus.IsValid, "destination status response is missing IsValid")

	// 一時グループを作成して設定を変更する
	group := createCRUDItem(t, h, &created, "group", name, description, suffix,
		map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": true})
	updateCRUDItem(t, h, group, "一時グループをメタデータのみ更新し、既存設定が維持されることを確認する", nil)
	updateCRUDItem(t, h, group, "一時グループを有効化する",
		map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": false})
	updateCRUDItem(t, h, group, "一時グループを無効化する",
		map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": true})
	updateCRUDItem(t, h, group, "一時グループに同じ設定を再指定し、宛先と有効状態が保持されることを確認する",
		group.item.Settings)

	// 一時ルーティングを作成して設定を変更する
	routing := createCRUDItem(t, h, &created, "routing", name, description, suffix, map[string]any{
		"SourceID":      sourceID,
		"TargetGroupID": group.item.ID,
		"PriorityRank":  100,
		"MatchLabels":   []any{map[string]any{"Name": "skr-e2e", "Value": suffix}},
	})
	updateCRUDItem(t, h, routing, "一時ルーティングをメタデータのみ更新し、既存設定が維持されることを確認する", nil)
	updateCRUDItem(t, h, routing, "一時ルーティングの照合ラベルを変更する", map[string]any{
		"SourceID":      sourceID,
		"TargetGroupID": group.item.ID,
		"PriorityRank":  routing.item.Settings["PriorityRank"],
		"MatchLabels":   []any{map[string]any{"Name": "skr-e2e", "Value": suffix + "-updated"}},
	})
	updateCRUDItem(t, h, routing, "一時ルーティングに同じ設定を再指定し、設定が保持されることを確認する",
		routing.item.Settings)

	// 一時ルーティングの優先順位を 99 に変更してから元に戻す
	for _, rank := range []int{99, 100} {
		reorderRouting(t, h, routing, rank)
	}

	// 既存ルーティングが変更されていないことを確認する
	after := listCRUDItems(t, h, "既存ルーティングが変更されていないことを確認する", "routing")
	var existing []crudItem
	for _, item := range after {
		if item.ID != routing.item.ID {
			existing = append(existing, item)
		}
	}
	require.Len(t, existing, len(before), "existing routing list changed during CRUD test")
	for _, expected := range before {
		found := false
		for _, actual := range existing {
			if matchesExactCRUDItem(actual, expected) {
				found = true
			}
		}
		require.True(t, found, "existing routing identity or settings changed during CRUD test")
	}

	// 既存通知先が変更されていないことを確認する
	currentSource := readCRUDItem(t, h, "既存通知先が変更されていないことを確認する", "destination", source.ID)
	require.True(t, matchesExactCRUDItem(currentSource, source),
		"source destination identity or settings changed during CRUD test")
}

// notificationSourceID は通知元のリソース ID を取得します。
func notificationSourceID(t *testing.T, h *helper.Helper) string {
	t.Helper()

	data := h.Call(t, "通知元のリソースを確認する", "simple-notification-api", "routing", "list-source")
	var sources struct {
		Sources []struct {
			ID string `json:"id"`
		} `json:"Sources"`
	}
	err := json.Unmarshal(data, &sources)
	require.NoError(t, err, "decode notification sources: %v", err)
	require.NotEmpty(t, sources.Sources, "CRUD preflight refused: no notification source available")
	require.NotEmpty(t, sources.Sources[0].ID, "CRUD preflight refused: no notification source available")
	return sources.Sources[0].ID
}

// createCRUDItem は一時リソースを作成し、作成内容と一覧への反映を確認します。
// 作成に成功したリソースは created に追加され、後片付けで削除されます。
func createCRUDItem(t *testing.T, h *helper.Helper, created *[]*crudResource, kind, name, description, suffix string, settings map[string]any) *crudResource {
	t.Helper()

	label := kindName(kind)
	// 作成前に同名リソースが存在しないことを確認する
	for _, item := range listCRUDItems(t, h, fmt.Sprintf("作成前に同名%sが存在しないことを確認する", label), kind) {
		require.NotEqual(t, name, item.Name, "%s name %q already exists", label, name)
	}

	item := crudItem{Name: name, Description: description, Tags: []string{"skr-e2e", suffix}, Settings: settings}
	requestPath := h.WriteRequestJSONFile(t, "create-"+kind, crudResponse{CommonServiceItem: item})
	response := parseCRUDResponse(t, h.Call(t, fmt.Sprintf("一時%sを作成する", label),
		"simple-notification-api", kind, "create", "--request", "@"+requestPath),
		fmt.Sprintf("一時%sを作成する", label))

	if kind == "routing" {
		// 作成時の優先順位は API 側で割り当てられるため、応答から取得する
		rank, ok := response.Settings["PriorityRank"].(float64)
		require.True(t, ok, "routing create response has no priority rank")
		require.GreaterOrEqual(t, rank, float64(1), "routing create response has invalid priority rank")
		require.LessOrEqual(t, rank, float64(100), "routing create response has invalid priority rank")
		item.Settings["PriorityRank"] = rank
	}
	require.NotEmpty(t, response.ID, "%s create response has no ID", label)
	item.ID = response.ID
	require.True(t, matchesCRUDItem(response, item), "%s create response mismatch", label)

	r := &crudResource{kind: kind, suffix: suffix, item: item}
	// 作成したリソースを後片付けの対象として登録する
	*created = append(*created, r)

	// 設定の反映を待って個別参照する
	verifyCRUDItem(t, h, fmt.Sprintf("作成した一時%sを参照する", label), kind, r.item)

	// 一覧に含まれることを確認する
	found := false
	for _, actual := range listCRUDItems(t, h, fmt.Sprintf("作成した一時%sが一覧に含まれることを確認する", label), kind) {
		if matchesCRUDItem(actual, r.item) {
			found = true
		}
	}
	require.True(t, found, "created %s is missing from list", label)
	return r
}

// updateCRUDItem は一時リソースを更新し、反映後の状態を個別参照で確認します。
// settings が nil の場合はメタデータのみを更新し、既存の Settings が維持されることを確認します。
func updateCRUDItem(t *testing.T, h *helper.Helper, r *crudResource, description string, settings map[string]any) {
	t.Helper()

	label := kindName(r.kind)
	item := r.item
	item.Settings = settings
	item.Tags = []string{"skr-e2e", r.suffix, "updated"}
	requestPath := h.WriteRequestJSONFile(t, "update-"+r.kind, crudResponse{CommonServiceItem: item})
	_ = parseCRUDResponse(t, h.Call(t, description,
		"simple-notification-api", r.kind, "update", r.item.ID, "--request", "@"+requestPath), description)
	r.item.Tags = item.Tags
	if settings != nil {
		r.item.Settings = settings
	}
	verifyCRUDItem(t, h, fmt.Sprintf("更新後の一時%sを参照する", label), r.kind, r.item)
}

// reorderRouting は一時ルーティングの優先順位を変更し、反映後の状態を確認します。
func reorderRouting(t *testing.T, h *helper.Helper, r *crudResource, rank int) {
	t.Helper()

	requestPath := h.WriteRequestJSONFile(t, "reorder-routing", map[string]any{
		"Orders": []any{map[string]any{"RoutingID": r.item.ID, "PriorityRank": rank}},
	})
	data := h.Call(t, fmt.Sprintf("一時ルーティングの優先順位を %d に変更する", rank),
		"simple-notification-api", "routing", "reorder", "--request", "@"+requestPath)
	var result struct {
		IsOK bool `json:"is_ok"`
	}
	err := json.Unmarshal(data, &result)
	require.NoError(t, err, "decode reorder response: %v", err)
	require.True(t, result.IsOK, "routing reorder was not accepted")

	expected := r.item
	expected.Settings = maps.Clone(r.item.Settings)
	expected.Settings["PriorityRank"] = float64(rank)
	r.item = expected
	verifyCRUDItem(t, h, fmt.Sprintf("優先順位を %d に変更した一時ルーティングを参照する", rank), r.kind, r.item)
}

// verifyCRUDItem は個別参照の結果が期待値と一致するまで期限付きで再試行します。
// 設定の変更は API 側の反映に時間がかかることがあります。
func verifyCRUDItem(t *testing.T, h *helper.Helper, description, kind string, expected crudItem) {
	t.Helper()

	for attempt := 1; attempt <= crudVerifyAttempts; attempt++ {
		actual := readCRUDItem(t, h, fmt.Sprintf("%s（%d 回目）", description, attempt), kind, expected.ID)
		if matchesCRUDItem(actual, expected) {
			return
		}
		if attempt < crudVerifyAttempts {
			h.Sleep(t, "設定の反映を待つ", crudVerifyPolling)
		}
	}
	t.Fatalf("%s: resource identity or settings mismatch", description)
}

// cleanupCRUDItems はルーティング、グループ、通知先の順に一時リソースを後片付けします。
// 失敗しても後続のリソースの後片付けを続け、その失敗をテストの結果へ反映します。
func cleanupCRUDItems(t *testing.T, h *helper.Helper, created []*crudResource, name string) {
	t.Helper()

	for i := len(created) - 1; i >= 0; i-- {
		r := created[i]
		label := kindName(r.kind)
		_, stderr, deleteErr := h.CallAllowingFailure(t, fmt.Sprintf("一時%sを削除する", label),
			"simple-notification-api", r.kind, "delete", r.item.ID)
		if deleteErr != nil {
			t.Errorf("delete temporary %s: %v\n%s", r.kind, deleteErr, stderr)
		}
		remaining := listCRUDItemsAllowingFailure(t, h,
			fmt.Sprintf("削除後に一時%sが一覧に残っていないことを確認する", label), r.kind)
		for _, item := range remaining {
			if item.ID == r.item.ID || item.Name == name {
				t.Errorf("cleanup failed: temporary %s remains", r.kind)
			}
		}
	}
}
