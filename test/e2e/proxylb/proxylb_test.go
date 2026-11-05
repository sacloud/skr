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
	proxylbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// proxylbDescription はテスト用 ProxyLB に付ける固定の説明です。後片付けの識別にも使います。
	proxylbDescription = "Temporary ProxyLB created by skr live E2E"
	// updatedProxylbDescription は更新確認中に使用する一時的な説明です。
	// 更新確認は説明を元に戻さないため、後片付けはこの説明も許容します。
	updatedProxylbDescription = "Temporary ProxyLB updated by skr live E2E"
)

// proxylbItem は ProxyLB の参照結果です。
type proxylbItem struct {
	ID          types.ID
	Name        string
	Description string
	FQDN        string
	Plan        int
	Region      types.EProxyLBRegion
	VirtualIP   string `json:"VirtualIPAddress"`
	HealthCheck *iaas.ProxyLBHealthCheck
	BindPorts   []*iaas.ProxyLBBindPort
	Servers     []*iaas.ProxyLBServer
}

// healthStatusItem は health-status の応答の配下サーバーです。
type healthStatusItem struct {
	IPAddress string
	Port      types.StringNumber
	Status    types.EServerInstanceStatus
}

// decodeProxylbList は ProxyLB の一括検索の応答を解析します。
func decodeProxylbList(data []byte) ([]proxylbItem, error) {
	var items []proxylbItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode ProxyLB list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of ProxyLBs")
	}
	return items, nil
}

// parseProxylb は ProxyLB の参照応答を解析します。ID が空の場合はエラーを返します。
func parseProxylb(data []byte) (proxylbItem, error) {
	var item proxylbItem
	if err := json.Unmarshal(data, &item); err != nil {
		return proxylbItem{}, fmt.Errorf("decode ProxyLB: %w", err)
	}
	if item.ID.IsEmpty() {
		return proxylbItem{}, errors.New("ProxyLB ID is empty")
	}
	return item, nil
}

// checkProxylbIdentity は参照結果が期待する ProxyLB (ID・名前・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func checkProxylbIdentity(item proxylbItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("ProxyLB identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// requireNoProxylbNameCollision は作成前に同名の ProxyLB が存在しないことを SDK で確認します。
// 確認の実行は証跡に残しません。既存リソースは変更しません。
func requireNoProxylbNameCollision(t *testing.T, proxylbs *proxylbSDK.Service, name string) {
	t.Helper()

	matches, err := lookupProxylbs(t, proxylbs, name)
	require.NoError(t, err, "find existing ProxyLBs")
	require.Empty(t, matches, "test ProxyLB name %q already exists; refusing to modify it", name)
}

// lookupProxylbs は SDK で名前の部分一致検索を行い、結果を返します。
// 事前確認と後片付けの確認など、証跡に残さない安全確認で使います。
func lookupProxylbs(t *testing.T, proxylbs *proxylbSDK.Service, name string) ([]proxylbItem, error) {
	t.Helper()

	found, err := proxylbs.FindWithContext(t.Context(), &proxylbSDK.FindRequest{Names: []string{name}})
	if err != nil {
		return nil, fmt.Errorf("find ProxyLBs by name: %w", err)
	}
	items := make([]proxylbItem, 0, len(found))
	for _, p := range found {
		if p == nil {
			continue
		}
		items = append(items, proxylbItem{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			FQDN:        p.FQDN,
			Plan:        int(p.Plan),
			Region:      p.Region,
			VirtualIP:   p.VirtualIPAddress,
			HealthCheck: p.HealthCheck,
			BindPorts:   p.BindPorts,
			Servers:     p.Servers,
		})
	}
	return items, nil
}

// findProxylbs は skr CLI で ProxyLB を名前で検索し、確認に必要な項目へ絞り込んだ応答を返します。
func findProxylbs(t *testing.T, h *helper.Helper, name string) []proxylbItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ProxyLB を名前で検索する",
		"proxylb-api", "find", "--name", name,
		"--query", "map({ID,Name,Description,FQDN,Plan,Region,HealthCheck,BindPorts,Servers})",
	)
	require.NoError(t, err, "find ProxyLB by name")
	items, err := decodeProxylbList(data)
	require.NoError(t, err)
	return items
}

// readProxylb は skr CLI で ProxyLB を参照し、応答を返します。
func readProxylb(t *testing.T, h *helper.Helper, id types.ID) proxylbItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ProxyLB を参照する",
		"proxylb-api", "read", "--id", id.String(),
	)
	require.NoError(t, err, "read ProxyLB")
	item, err := parseProxylb(data)
	require.NoError(t, err)
	return item
}

// healthStatus は skr CLI で配下サーバーの稼働状態を取得します。
func healthStatus(t *testing.T, h *helper.Helper, id types.ID) []healthStatusItem {
	t.Helper()

	// health-status の応答は ActiveConn、CPS、CurrentVIP、Servers を持つオブジェクトです。
	// Servers を取り出して配下サーバーの稼働状態を確認します
	data, _, err := h.CallAllowingFailure(t, "ProxyLB の配下サーバーの稼働状態を取得する",
		"proxylb-api", "health-status", "--id", id.String(),
		"--query", ".Servers",
	)
	require.NoError(t, err, "get ProxyLB health status")
	var items []healthStatusItem
	require.NoError(t, json.Unmarshal(data, &items), "decode health status")
	return items
}

// cleanupProxylb は後片付けです。作成直後に確認した ProxyLB の ID を優して SDK で再照合し、
// ID と許容する識別情報が一致する場合だけ削除します。保持した ID がないのに同名リソースが
// 見つかった場合は、今回の作成物と確定できないため自動削除せず手動確認を要求します。
// 削除は skr CLI で行い、削除後は SDK で不在を確認します。
// 更新確認は説明を元に戻さないため、更新後の説明も許容します。
func cleanupProxylb(t *testing.T, h *helper.Helper, proxylbs *proxylbSDK.Service, name string, knownID types.ID) error {
	t.Helper()

	id := knownID
	if id.IsEmpty() {
		matches, err := lookupProxylbs(t, proxylbs, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created ProxyLB: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		// 保持した作成 ID がないのに同名リソースが見つかった場合は、
		// 今回の作成物と確定できないため削除せず手動確認を要求します
		return errors.New("cannot confirm the created ProxyLB ID; manual cleanup required")
	}

	// 削除前に SDK で再照合し、保持した ID と許容する識別情報が一致する場合だけ削除する
	current, err := proxylbs.ReadWithContext(t.Context(), &proxylbSDK.ReadRequest{ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify ProxyLB before cleanup: %w", err)
	}
	if current.ID != id || current.Name != name ||
		(current.Description != proxylbDescription && current.Description != updatedProxylbDescription) {
		return errors.New("cleanup refused: ProxyLB identity changed; manual cleanup required")
	}
	// 削除などリソースを実際に変更する操作は、実行記録に残る skr CLI で行います
	if _, _, err := h.CallAllowingFailure(t, "テスト用の ProxyLB を削除する",
		"proxylb-api", "delete", "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test ProxyLB %s: %w", id, err)
	}
	matches, err := lookupProxylbs(t, proxylbs, name)
	if err != nil {
		return fmt.Errorf("verify ProxyLB absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("ProxyLB %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}
