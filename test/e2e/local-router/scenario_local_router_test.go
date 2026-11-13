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
	localrouterSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
	swytch "github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// localRouterZone は接続先スイッチを作成するゾーンです。ローカルルータ自体はグローバルリソースです。
	localRouterZone = "is1b"
	// localRouterNamePrefix はテスト用ローカルルータの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	localRouterNamePrefix = "skr-e2e-local-router-"
	// switchNamePrefix はテスト用スイッチの名前の接頭辞です。
	switchNamePrefix = "skr-e2e-local-router-switch-"
	// localRouterDescription はテスト用ローカルルータに付ける説明です。後片付けの識別にも使います。
	localRouterDescription = "Temporary local router created by skr live E2E"
	// updatedLocalRouterDescription は更新確認中に使用する一時的な説明です。
	// 更新確認は説明を元に戻さないため、後片付けはこの説明も許容します。
	updatedLocalRouterDescription = "Temporary local router updated by skr live E2E"
	// switchDescription はテスト用スイッチに付ける説明です。
	switchDescription = "Temporary switch for skr local-router-api live E2E"
	// localRouterFindPageSize は SDK の安全確認で使う1ページの件数です。
	localRouterFindPageSize = 100
)

// localRouterItem はローカルルータの参照結果のうち、確認に使う項目です。
type localRouterItem struct {
	ID           types.ID
	Name         string
	Description  string
	Switch       *iaas.LocalRouterSwitch    `json:"Switch"`
	Interface    *iaas.LocalRouterInterface `json:"Interface"`
	Peers        []*iaas.LocalRouterPeer    `json:"Peers"`
	StaticRoutes []*iaas.LocalRouterStaticRoute
}

// TestScenarioLocalRouter は local-router-api コマンドのライブ E2E です。
// is1b に接続先スイッチを作成し、スイッチ接続とスタティックルート付きの
// ローカルルータを作成して、名前検索、参照、説明の更新、接続設定の部分更新、
// ピアの稼働状態、トラフィックモニタリング、削除と不在確認まで行います。
// ローカルルータ自体はグローバルリソースで、--zone を指定しません。
// テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認は SDK で静かに実行します。
// 途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioLocalRouter(t *testing.T) {
	h := helper.New(t, "local-router-api", "ローカルルータを作成・検索・参照・更新し、接続設定の部分更新、稼働状態、モニタリング、削除が正しく動作することを確認する")

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	switches := swytch.New(client)
	localRouters := localrouterSDK.New(client)

	suffix := helper.RandomSuffix(t)
	switchName := switchNamePrefix + suffix
	name := localRouterNamePrefix + suffix

	// 作成前に同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、既存リソースは変更しないまま終了する
	require.Empty(t, lookupSwitches(t, switches, switchName),
		"switch name %q already exists; refusing to modify it", switchName)
	existingLocalRouters, err := lookupLocalRouters(t, localRouters, name)
	require.NoError(t, err, "find existing local routers")
	require.Empty(t, existingLocalRouters,
		"local router name %q already exists; refusing to modify it", name)

	// 接続先スイッチを作成する。スイッチの作成・削除は実行記録に残る skr CLI で行います
	switchData := h.Call(t, "接続先スイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", localRouterZone,
		"--name", switchName,
		"--description", switchDescription)
	createdSwitch := parseSwitchResource(t, switchData)
	switchID := createdSwitch.ID

	// 後片付け。defer の呼び出し順 (LIFO) により、ローカルルータの削除がスイッチの削除より先に実行されます
	var localRouterID types.ID
	defer func() {
		if err := cleanupSwitch(t, h, switches, switchName, switchID); err != nil {
			t.Errorf("switch cleanup failed: %v", err)
		}
	}()
	defer func() {
		if err := cleanupLocalRouter(t, h, localRouters, name, localRouterID); err != nil {
			t.Errorf("local router cleanup failed: %v", err)
		}
	}()

	// スイッチ接続とスタティックルート付きのローカルルータを作成します。
	// スイッチのカテゴリとゾーンは既定値の補完がないため明示します
	data := h.Call(t, "スイッチ接続とスタティックルート付きのローカルルータを作成する",
		"local-router-api", "create",
		"--name", name,
		"--description", localRouterDescription,
		"--switch-id", switchID.String(), "--switch-category", "cloud", "--switch-zone-id", localRouterZone,
		"--interface-virtual-ip-address", "192.168.1.1",
		"--interface-ip-address", "192.168.1.2", "--interface-ip-address", "192.168.1.3",
		"--interface-network-mask-len", "24", "--interface-vrid", "100",
		"--static-route", "prefix=192.168.10.0/24,next-hop=192.168.1.254",
	)
	created, err := parseLocalRouter(data)
	require.NoError(t, err)
	localRouterID = created.ID
	require.NoError(t, checkLocalRouterIdentity(created, localRouterID, name, localRouterDescription),
		"the create response should contain the created local router")
	verifyLocalRouterConnection(t, created, switchID.String(), "192.168.1.1", []string{"192.168.1.2", "192.168.1.3"}, 24, 100,
		[]*iaas.LocalRouterStaticRoute{{Prefix: "192.168.10.0/24", NextHop: "192.168.1.254"}})

	// find が名前の部分一致で作成したローカルルータを見つけられることを確認します
	found := findLocalRouters(t, h, name)
	require.Len(t, found, 1, "local-router-api find --name should return only the created local router")
	require.NoError(t, checkLocalRouterIdentity(found[0], localRouterID, name, localRouterDescription),
		"local-router-api find should return the created local router")

	// --query で ID と名前を射影した一覧も同じ結果になることを確認します
	projected := findLocalRouterIDAndName(t, h, name)
	require.Len(t, projected, 1)
	require.Equal(t, localRouterID, projected[0].ID)
	require.Equal(t, name, projected[0].Name)

	// read が作成済みのローカルルータを返すことを確認します
	read := readLocalRouter(t, h, localRouterID)
	require.NoError(t, checkLocalRouterIdentity(read, localRouterID, name, localRouterDescription),
		"local-router-api read should return the created local router")
	verifyLocalRouterConnection(t, read, switchID.String(), "192.168.1.1", []string{"192.168.1.2", "192.168.1.3"}, 24, 100,
		[]*iaas.LocalRouterStaticRoute{{Prefix: "192.168.10.0/24", NextHop: "192.168.1.254"}})

	// 説明だけを更新します。省略した接続設定が変更されないことを応答と参照で確認します
	data, _, err = h.CallAllowingFailure(t, "ローカルルータの説明を更新する",
		"local-router-api", "update", "--id", localRouterID.String(),
		"--description", updatedLocalRouterDescription,
	)
	require.NoError(t, err, "update local router description")
	updated, err := parseLocalRouter(data)
	require.NoError(t, err)
	require.NoError(t, checkLocalRouterIdentity(updated, localRouterID, name, updatedLocalRouterDescription),
		"the update should be reflected in the response")
	verifyLocalRouterConnection(t, updated, switchID.String(), "192.168.1.1", []string{"192.168.1.2", "192.168.1.3"}, 24, 100,
		[]*iaas.LocalRouterStaticRoute{{Prefix: "192.168.10.0/24", NextHop: "192.168.1.254"}})

	// read でも更新後の説明と接続設定の維持を確認します
	read = readLocalRouter(t, h, localRouterID)
	require.NoError(t, checkLocalRouterIdentity(read, localRouterID, name, updatedLocalRouterDescription),
		"local-router-api read should return the updated local router")
	verifyLocalRouterConnection(t, read, switchID.String(), "192.168.1.1", []string{"192.168.1.2", "192.168.1.3"}, 24, 100,
		[]*iaas.LocalRouterStaticRoute{{Prefix: "192.168.10.0/24", NextHop: "192.168.1.254"}})

	// update-settings でスタティックルートとインターフェースを置き換えます。
	// 省略したピア設定が変更されないことを応答と参照で確認します
	data, _, err = h.CallAllowingFailure(t, "ローカルルータの接続設定を部分更新する",
		"local-router-api", "update-settings", "--id", localRouterID.String(),
		"--switch-id", switchID.String(), "--switch-category", "cloud", "--switch-zone-id", localRouterZone,
		"--interface-virtual-ip-address", "192.168.1.1",
		"--interface-ip-address", "192.168.1.4,192.168.1.5",
		"--interface-network-mask-len", "24", "--interface-vrid", "200",
		"--static-route", "prefix=192.168.20.0/24,next-hop=192.168.1.253",
		"--static-route", "prefix=192.168.21.0/24,next-hop=192.168.1.253",
	)
	require.NoError(t, err, "update local router settings")
	settingsUpdated, err := parseLocalRouter(data)
	require.NoError(t, err)
	require.NoError(t, checkLocalRouterIdentity(settingsUpdated, localRouterID, name, updatedLocalRouterDescription),
		"update-settings should not change the identity")
	verifyLocalRouterConnection(t, settingsUpdated, switchID.String(), "192.168.1.1", []string{"192.168.1.4", "192.168.1.5"}, 24, 200,
		[]*iaas.LocalRouterStaticRoute{
			{Prefix: "192.168.20.0/24", NextHop: "192.168.1.253"},
			{Prefix: "192.168.21.0/24", NextHop: "192.168.1.253"},
		})

	read = readLocalRouter(t, h, localRouterID)
	verifyLocalRouterConnection(t, read, switchID.String(), "192.168.1.1", []string{"192.168.1.4", "192.168.1.5"}, 24, 200,
		[]*iaas.LocalRouterStaticRoute{
			{Prefix: "192.168.20.0/24", NextHop: "192.168.1.253"},
			{Prefix: "192.168.21.0/24", NextHop: "192.168.1.253"},
		})

	// ピアの稼働状態を取得します。ピアを設定していないため Peers は空配列です
	getLocalRouterHealth(t, h, localRouterID)

	// トラフィックモニタリングを取得します。トラフィックがないため値は空でも、
	// 期間を指定した応答が JSON 配列で返ることを確認します
	getLocalRouterMonitor(t, h, localRouterID)

	// ローカルルータとスイッチの削除と不在確認は後片付けの defer で行います
}

// parseLocalRouter はローカルルータの応答を解析します。ID が空の場合はエラーを返します。
func parseLocalRouter(data []byte) (localRouterItem, error) {
	var item localRouterItem
	if err := json.Unmarshal(data, &item); err != nil {
		return localRouterItem{}, fmt.Errorf("decode local router: %w", err)
	}
	if item.ID.IsEmpty() {
		return localRouterItem{}, errors.New("local router ID is empty")
	}
	return item, nil
}

// checkLocalRouterIdentity は参照結果が期待するローカルルータ (ID・名前・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func checkLocalRouterIdentity(item localRouterItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("local router identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// verifyLocalRouterConnection はローカルルータの応答が期待する接続設定
// (スイッチ・インターフェース・スタティックルート) と一致することを確認します。
func verifyLocalRouterConnection(t *testing.T, item localRouterItem, switchCode, virtualIPAddress string,
	ipAddress []string, networkMaskLen, vrid int, staticRoutes []*iaas.LocalRouterStaticRoute) {
	t.Helper()

	require.NotNil(t, item.Switch, "the local router should be connected to a switch")
	require.Equal(t, switchCode, item.Switch.Code, "the local router should be connected to the created switch")
	require.Equal(t, "cloud", item.Switch.Category, "the switch category should be cloud")
	require.Equal(t, localRouterZone, item.Switch.ZoneID, "the switch zone should be %s", localRouterZone)
	require.NotNil(t, item.Interface, "the local router should have interface settings")
	require.Equal(t, virtualIPAddress, item.Interface.VirtualIPAddress, "the interface virtual IP should match")
	require.Equal(t, ipAddress, item.Interface.IPAddress, "the interface IP addresses should match")
	require.Equal(t, networkMaskLen, item.Interface.NetworkMaskLen, "the interface network mask length should match")
	require.Equal(t, vrid, item.Interface.VRID, "the interface VRID should match")
	require.Len(t, item.StaticRoutes, len(staticRoutes), "the local router should have the requested static routes")
	for i, want := range staticRoutes {
		require.Equal(t, want.Prefix, item.StaticRoutes[i].Prefix, "static routes[%d] prefix should match", i)
		require.Equal(t, want.NextHop, item.StaticRoutes[i].NextHop, "static routes[%d] next hop should match", i)
	}
}

// findLocalRouters は skr CLI でローカルルータを名前で検索し、確認に必要な項目へ絞り込んだ応答を返します。
func findLocalRouters(t *testing.T, h *helper.Helper, name string) []localRouterItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ローカルルータを名前で検索する",
		"local-router-api", "find", "--name", name,
		"--query", "map({ID,Name,Description,Switch,Interface,Peers,StaticRoutes})",
	)
	require.NoError(t, err, "find local router by name")
	items, err := decodeLocalRouterList(data)
	require.NoError(t, err)
	return items
}

// findLocalRouterIDAndName は --query で ID と名前を射影した一覧を取得します。
func findLocalRouterIDAndName(t *testing.T, h *helper.Helper, name string) []localRouterItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "--query で ID と名前を射影してローカルルータを検索する",
		"local-router-api", "find", "--name", name, "--query", "map({ID,Name})",
	)
	require.NoError(t, err, "find local router with projection")
	items, err := decodeLocalRouterList(data)
	require.NoError(t, err)
	return items
}

// decodeLocalRouterList はローカルルータの一括検索の応答を解析します。
func decodeLocalRouterList(data []byte) ([]localRouterItem, error) {
	var items []localRouterItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode local router list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of local routers")
	}
	return items, nil
}

// readLocalRouter は skr CLI でローカルルータを参照し、応答を返します。
func readLocalRouter(t *testing.T, h *helper.Helper, id types.ID) localRouterItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ローカルルータを参照する",
		"local-router-api", "read", "--id", id.String(),
	)
	require.NoError(t, err, "read local router")
	item, err := parseLocalRouter(data)
	require.NoError(t, err)
	return item
}

// getLocalRouterHealth は skr CLI でピアの稼働状態を取得します。
// ピアを設定していないローカルルータでは Peers が空配列で返ることを確認します。
func getLocalRouterHealth(t *testing.T, h *helper.Helper, id types.ID) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ローカルルータのピアの稼働状態を取得する",
		"local-router-api", "health-status", "--id", id.String(),
	)
	require.NoError(t, err, "get local router health status")
	var health struct {
		Peers []*iaas.LocalRouterHealthPeer `json:"Peers"`
	}
	require.NoError(t, json.Unmarshal(data, &health), "decode local router health: %s", string(data))
	require.Empty(t, health.Peers, "the local router without peers should have no peers in the health status")
}

// getLocalRouterMonitor は skr CLI でトラフィックモニタリングを取得します。
// 現在時刻を終端とする観測期間を指定し、応答が JSON 配列で返ることを確認します。
// トラフィックがない間は空配列になるため、実測サンプルの存在までは要求しません。
func getLocalRouterMonitor(t *testing.T, h *helper.Helper, id types.ID) {
	t.Helper()

	window := h.Call(t, "ローカルルータのトラフィックモニタリングを取得する",
		"local-router-api", "monitor-local-router", "--id", id.String(),
		"--start", "2026-11-13T00:00:00Z", "--end", "2036-11-13T00:00:00Z")
	var values []json.RawMessage
	require.NoError(t, json.Unmarshal(window, &values), "decode monitor values: %s", string(window))
	require.NotNil(t, values, "monitor-local-router should return a JSON array")
}

// cleanupLocalRouter は後片付けです。作成直後に確認したローカルルータの ID を優して SDK で再照合し、
// ID と許容する識別情報が一致する場合だけ削除します。保持した ID がないのに同名リソースが
// 見つかった場合は、今回の作成物と確定できないため自動削除せず手動確認を要求します。
// 削除は skr CLI で行い、削除後は SDK で不在を確認します。
// 更新確認は説明を元に戻さないため、更新後の説明も許容します。
func cleanupLocalRouter(t *testing.T, h *helper.Helper, localRouters *localrouterSDK.Service, name string, knownID types.ID) error {
	t.Helper()

	id := knownID
	if id.IsEmpty() {
		matches, err := lookupLocalRouters(t, localRouters, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created local router: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		// 保持した作成 ID がないのに同名リソースが見つかった場合は、
		// 今回の作成物と確定できないため削除せず手動確認を要求します
		return errors.New("cannot confirm the created local router ID; manual cleanup required")
	}

	// 削除前に SDK で再照合し、保持した ID と許容する識別情報が一致する場合だけ削除する
	current, err := localRouters.ReadWithContext(t.Context(), &localrouterSDK.ReadRequest{ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify local router before cleanup: %w", err)
	}
	if current.ID != id || current.Name != name ||
		(current.Description != localRouterDescription && current.Description != updatedLocalRouterDescription) {
		return errors.New("cleanup refused: local router identity changed; manual cleanup required")
	}
	// 削除などリソースを実際に変更する操作は、実行記録に残る skr CLI で行います
	if _, _, err := h.CallAllowingFailure(t, "テスト用のローカルルータを削除する",
		"local-router-api", "delete", "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test local router %s: %w", id, err)
	}
	matches, err := lookupLocalRouters(t, localRouters, name)
	if err != nil {
		return fmt.Errorf("verify local router absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("local router %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}

// lookupLocalRouters は SDK で名前の部分一致検索を行い、結果を返します。
// 事前確認と後片付けの確認など、証跡に残さない安全確認で使います。
func lookupLocalRouters(t *testing.T, localRouters *localrouterSDK.Service, name string) ([]localRouterItem, error) {
	t.Helper()

	found, err := localRouters.FindWithContext(t.Context(), &localrouterSDK.FindRequest{
		Names: []string{name},
		Count: localRouterFindPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("find local routers by name: %w", err)
	}
	items := make([]localRouterItem, 0, len(found))
	for _, r := range found {
		if r == nil {
			continue
		}
		items = append(items, localRouterItem{
			ID:           r.ID,
			Name:         r.Name,
			Description:  r.Description,
			Switch:       r.Switch,
			Interface:    r.Interface,
			Peers:        r.Peers,
			StaticRoutes: r.StaticRoutes,
		})
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), localRouterFindPageSize,
		"local router find result page is full; cannot conclude absence of %q", name)
	return items, nil
}

// cleanupSwitch は後片付けです。作成直後に確認したスイッチの ID を優して SDK で再照合し、
// ID と名前が一致する場合だけ削除します。削除は skr CLI で行い、削除後は SDK で不在を確認します。
func cleanupSwitch(t *testing.T, h *helper.Helper, switches *swytch.Service, name string, knownID types.ID) error {
	t.Helper()

	id := knownID
	if id.IsEmpty() {
		matches := lookupSwitches(t, switches, name)
		if len(matches) == 0 {
			return nil
		}
		return errors.New("cannot confirm the created switch ID; manual cleanup required")
	}

	// 削除前に SDK で再照合し、保持した ID と名前が一致する場合だけ削除する
	current, err := switches.ReadWithContext(t.Context(), &swytch.ReadRequest{Zone: localRouterZone, ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify switch before cleanup: %w", err)
	}
	if current.ID != id || current.Name != name {
		return errors.New("cleanup refused: switch identity changed; manual cleanup required")
	}
	// 削除などリソースを実際に変更する操作は、実行記録に残る skr CLI で行います
	if _, _, err := h.CallAllowingFailure(t, "テスト用のスイッチを削除する",
		"iaas-api", "switch", "delete", "--zone", localRouterZone, "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test switch %s: %w", id, err)
	}
	matches := lookupSwitches(t, switches, name)
	if len(matches) != 0 {
		return fmt.Errorf("switch %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}

// lookupSwitches は SDK で名前でスイッチを検索します。
func lookupSwitches(t *testing.T, switches *swytch.Service, name string) []switchResource {
	t.Helper()

	found, err := switches.FindWithContext(t.Context(), &swytch.FindRequest{
		Zone:  localRouterZone,
		Names: []string{name},
		Count: localRouterFindPageSize,
	})
	require.NoError(t, err, "find switches by name %q", name)
	items := make([]switchResource, 0, len(found))
	for _, s := range found {
		if s == nil {
			continue
		}
		items = append(items, switchResource{ID: s.ID, Name: s.Name, Description: s.Description})
	}
	return items
}

// switchResource はスイッチの参照結果のうち、確認に使う項目です。
type switchResource struct {
	ID          types.ID
	Name        string
	Description string
}

// parseSwitchResource はスイッチの応答を解析します。ID が空の場合はテストを失敗させます。
func parseSwitchResource(t *testing.T, data []byte) switchResource {
	t.Helper()

	var item switchResource
	require.NoError(t, json.Unmarshal(data, &item), "decode switch: %v", errString(data))
	require.NotZero(t, item.ID, "switch ID is missing")
	return item
}

// errString はデコード失敗時の応答本文を文字列化します。
func errString(data []byte) string {
	return string(data)
}
