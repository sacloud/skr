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
	gslbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/gslb"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// gslbDescription はテスト用 GSLB に付ける固定の説明です。後片付けの識別にも使います。
	gslbDescription = "Temporary GSLB created by skr live E2E"
	// updatedGSLBDescription は更新確認中に使用する一時的な説明です。
	// 更新確認は説明を元に戻さないため、後片付けはこの説明も許容します。
	updatedGSLBDescription = "Temporary GSLB updated by skr live E2E"
)

// gslbItem は GSLB の参照結果です。
type gslbItem struct {
	ID                 types.ID
	Name               string
	Description        string
	FQDN               string
	DelayLoop          int
	SorryServer        string
	HealthCheck        *iaas.GSLBHealthCheck
	DestinationServers iaas.GSLBServers
}

// decodeGSLBList は GSLB の一括検索の応答を解析します。
func decodeGSLBList(data []byte) ([]gslbItem, error) {
	var items []gslbItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode GSLB list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of GSLBs")
	}
	return items, nil
}

// parseGSLB は GSLB の参照応答を解析します。ID が空の場合はエラーを返します。
func parseGSLB(data []byte) (gslbItem, error) {
	var item gslbItem
	if err := json.Unmarshal(data, &item); err != nil {
		return gslbItem{}, fmt.Errorf("decode GSLB: %w", err)
	}
	if item.ID.IsEmpty() {
		return gslbItem{}, errors.New("GSLB ID is empty")
	}
	return item, nil
}

// checkGSLBIdentity は参照結果が期待する GSLB (ID・名前・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func checkGSLBIdentity(item gslbItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("GSLB identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// requireNoGSLBNameCollision は作成前に同名の GSLB が存在しないことを SDK で確認します。
// 確認の実行は証跡に残しません。既存リソースは変更しません。
func requireNoGSLBNameCollision(t *testing.T, gslbs *gslbSDK.Service, name string) {
	t.Helper()

	matches, err := lookupGSLBs(t, gslbs, name)
	require.NoError(t, err, "find existing GSLBs")
	require.Empty(t, matches, "test GSLB name %q already exists; refusing to modify it", name)
}

// lookupGSLBs は SDK で名前の完全一致検索を行い、結果を返します。
// 事前確認と後片付けの確認など、証跡に残さない安全確認で使います。
func lookupGSLBs(t *testing.T, gslbs *gslbSDK.Service, name string) ([]gslbItem, error) {
	t.Helper()

	found, err := gslbs.FindWithContext(t.Context(), &gslbSDK.FindRequest{Names: []string{name}})
	if err != nil {
		return nil, fmt.Errorf("find GSLBs by name: %w", err)
	}
	items := make([]gslbItem, 0, len(found))
	for _, g := range found {
		if g == nil {
			continue
		}
		items = append(items, gslbItem{
			ID:                 g.ID,
			Name:               g.Name,
			Description:        g.Description,
			FQDN:               g.FQDN,
			DelayLoop:          g.DelayLoop,
			SorryServer:        g.SorryServer,
			HealthCheck:        g.HealthCheck,
			DestinationServers: g.DestinationServers,
		})
	}
	return items, nil
}

// findGSLBs は skr CLI で GSLB を名前で検索し、確認に必要な項目へ絞り込んだ応答を返します。
func findGSLBs(t *testing.T, h *helper.Helper, name string) []gslbItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "GSLB を名前で検索する",
		"gslb-api", "find", "--name", name,
		"--query", "map({ID,Name,Description,FQDN,DelayLoop,HealthCheck,DestinationServers,SorryServer})",
	)
	require.NoError(t, err, "find GSLB by name")
	items, err := decodeGSLBList(data)
	require.NoError(t, err)
	return items
}

// readGSLB は skr CLI で GSLB を参照し、応答を返します。
func readGSLB(t *testing.T, h *helper.Helper, id types.ID) gslbItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "GSLB を参照する",
		"gslb-api", "read", "--id", id.String(),
	)
	require.NoError(t, err, "read GSLB")
	item, err := parseGSLB(data)
	require.NoError(t, err)
	return item
}

// cleanupGSLB は後片付けです。作成に成功した GSLB の ID を優先して SDK で再照合し、
// ID がない場合は名前検索で回収を試みます。識別できない場合は削除しません。
// 削除は skr CLI で行い、削除後は SDK で不在を確認します。
// 更新確認は説明を元に戻さないため、更新後の説明も許容します。
func cleanupGSLB(t *testing.T, h *helper.Helper, gslbs *gslbSDK.Service, name string, knownID types.ID) error {
	t.Helper()

	id := knownID
	if id.IsEmpty() {
		matches, err := lookupGSLBs(t, gslbs, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created GSLB: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() ||
			(matches[0].Description != gslbDescription && matches[0].Description != updatedGSLBDescription) {
			return errors.New("cannot safely identify created GSLB; manual cleanup required")
		}
		id = matches[0].ID
	}

	// 削除前に SDK で再照合し、保持した ID と許容する識別情報が一致する場合だけ削除する
	current, err := gslbs.ReadWithContext(t.Context(), &gslbSDK.ReadRequest{ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify GSLB before cleanup: %w", err)
	}
	if current.ID != id || current.Name != name ||
		(current.Description != gslbDescription && current.Description != updatedGSLBDescription) {
		return errors.New("cleanup refused: GSLB identity changed; manual cleanup required")
	}
	if _, _, err := h.CallAllowingFailure(t, "テスト用の GSLB を削除する",
		"gslb-api", "delete", "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test GSLB %s: %w", id, err)
	}
	matches, err := lookupGSLBs(t, gslbs, name)
	if err != nil {
		return fmt.Errorf("verify GSLB absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("GSLB %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}
