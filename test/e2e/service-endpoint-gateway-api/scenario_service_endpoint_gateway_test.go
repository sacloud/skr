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
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	swytch "github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// switchNamePrefix はテスト用スイッチの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	switchNamePrefix = "skr-e2e-seg-switch-"
	// switchDescription はテスト用スイッチに付ける説明です。後片付けの識別にも使います。
	switchDescription = "Temporary switch for skr service endpoint gateway E2E"
	// networkMaskLen はゲートウェイに割り当てるサブネットマスク長です。
	networkMaskLen = 24
	// gatewayIP はゲートウェイに割り当てる IPv4 アドレス (ゲートウェイアドレス) です。
	// スイッチで構成するサブネット内のアドレスを指定します。
	gatewayIP = "192.168.2.110"
	// findPageSize は SDK の安全確認で使う1ページの件数です。
	findPageSize = 100
	// availableTimeout はゲートウェイの作成処理の完了を待つ期限です。
	availableTimeout = 10 * time.Minute
	// powerStatusTimeout は電源状態の反映を待つ期限です。
	powerStatusTimeout = 10 * time.Minute
	// pollInterval は状態確認のポーリング間隔です。
	pollInterval = 30 * time.Second
	// segZone はテストで使うゾーンです。is1b はスイッチのゾーン内上限が 1/1 のため、
	// 上限が緩い is1c を使います。ゲートウェイはゾーン引数を持たず、
	// SDK 認証設定のゾーンを使うため、SAKURA_ZONE でスイッチと同じゾーンに合わせます。
	segZone = "is1c"
)

// TestScenarioServiceEndpointGateway は service-endpoint-gateway-api コマンドのライブ E2E です。
// テスト専用のスイッチを作成し、ゲートウェイの作成から参照、一覧、インターフェース参照、
// 接続先マネージドサービス設定の更新と反映、電源操作、削除と不在確認までの一連の操作を検証します。
// 後片付けで作成したゲートウェイとスイッチを削除し、既存リソースは変更・削除しません。
func TestScenarioServiceEndpointGateway(t *testing.T) {
	t.Setenv("SAKURA_ZONE", segZone)

	h := helper.New(t, "service-endpoint-gateway-api", "サービスエンドポイントゲートウェイの作成・参照・一覧・設定更新・反映・電源操作・削除が正しく動作することを確認する")

	// ゲートウェイは SAKURA_ZONE で指定したゾーンを使うため、
	// スイッチの作成先ゾーンも同じ値に合わせる
	zone := segZone
	switchName := switchNamePrefix + helper.RandomSuffix(t)

	// 作成前の同名確認を実施する (SDK)
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	require.Empty(t, lookupSwitches(t, client, switchName), "switch name %q already exists; refusing to modify it", switchName)

	// 接続先スイッチを作成する
	data := h.Call(t, "ゲートウェイの接続先となるスイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", zone,
		"--name", switchName,
		"--description", switchDescription)
	createdSwitch := parseSwitch(t, data)
	switchID := createdSwitch.ID.String()
	require.NotEmpty(t, switchID, "created switch should have an ID")

	// 後片付け: ゲートウェイの後片付けが先に実行されるよう、テスト関数の defer でスイッチを削除する。
	// スイッチは接続済みのリソースがあると削除できないため、ゲートウェイの削除後に実行する。
	defer func() {
		deleteSwitch(t, h, zone, switchID)
		require.Empty(t, findSwitchesByName(t, h, "削除後にスイッチが残っていないことを確認する", switchName), "switch %s should not remain after deletion", switchID)
	}()

	// ゲートウェイを作成する
	data = h.Call(t, "スイッチにサービスエンドポイントゲートウェイを作成する",
		"service-endpoint-gateway-api", "appliance", "create",
		"--switch-id", switchID,
		"--network-mask-len", strconv.Itoa(networkMaskLen),
		"--server-ip", gatewayIP)
	var created struct {
		Appliance struct {
			ID           string `json:"ID"`
			Class        string `json:"Class"`
			Availability string `json:"Availability"`
			Remark       struct {
				Switch struct {
					ID string `json:"ID"`
				} `json:"Switch"`
				Network struct {
					NetworkMaskLen int `json:"NetworkMaskLen"`
				} `json:"Network"`
				Servers []struct {
					IPAddress string `json:"IPAddress"`
				} `json:"Servers"`
			} `json:"Remark"`
		} `json:"Appliance"`
	}
	require.NoError(t, json.Unmarshal(data, &created), "decode create response: %s", string(data))
	require.NotEmpty(t, created.Appliance.ID, "created gateway should have an ID: %s", string(data))
	require.Equal(t, "serviceendpointgateway", created.Appliance.Class, "unexpected class")
	require.Equal(t, switchID, created.Appliance.Remark.Switch.ID, "created gateway should reference the scenario switch")
	require.Equal(t, networkMaskLen, created.Appliance.Remark.Network.NetworkMaskLen, "unexpected network mask len")
	require.Equal(t, gatewayIP, created.Appliance.Remark.Servers[0].IPAddress, "unexpected gateway address")
	applianceID := created.Appliance.ID

	// 後片付け: 作成したゲートウェイを削除し、一覧に残っていないことを確認する
	defer func() {
		deleteGateway(t, h, applianceID)
		require.NotContains(t, listGatewayIDs(t, h, "削除後にゲートウェイが一覧に残っていないことを確認する"), applianceID, "gateway %s should not remain after deletion", applianceID)
	}()

	// 利用可能になるまで待つ (作成処理の進行中は参照や変更が行えない)
	waitGatewayAvailable(t, h, "ゲートウェイの作成処理の完了を待つ", applianceID)

	// 作成したゲートウェイを参照する
	read := readGateway(t, h, "作成したゲートウェイを参照する", applianceID)
	require.Equal(t, applianceID, read.ID, "read should return the gateway created by this run")
	require.Equal(t, switchID, read.SwitchID, "read gateway should reference the scenario switch")
	require.NotEmpty(t, read.Interfaces, "read gateway should have interfaces: %+v", read)

	// 作成したゲートウェイが一覧に含まれることを確認する
	listed := listGatewayIDs(t, h, "作成したゲートウェイが一覧に含まれることを確認する")
	matches := 0
	for _, id := range listed {
		if id == applianceID {
			matches++
		}
	}
	require.Equal(t, 1, matches, "gateway list should contain exactly one match for %q", applianceID)

	// 接続先マネージドサービス設定を更新する (Settings は全体置換のため --request で渡す)
	settingsPath := h.WriteRequestJSONFile(t, "seg-settings", map[string]any{
		"Appliance": map[string]any{
			"Settings": map[string]any{
				"ServiceEndpointGateway": map[string]any{
					"EnabledServices": []any{
						map[string]any{
							"Type": "ObjectStorage",
							"Config": map[string]any{
								"Endpoints": []string{"s3.isk01.sakurastorage.jp"},
							},
						},
					},
				},
			},
		},
	})
	data = h.Call(t, "接続先マネージドサービス設定を更新する",
		"service-endpoint-gateway-api", "appliance", "update", applianceID,
		"--request", "@"+settingsPath)
	var updated struct {
		Appliance struct {
			ID       string `json:"ID"`
			Settings struct {
				ServiceEndpointGateway struct {
					EnabledServices []struct {
						Type string `json:"Type"`
					} `json:"EnabledServices"`
				} `json:"ServiceEndpointGateway"`
			} `json:"Settings"`
		} `json:"Appliance"`
	}
	require.NoError(t, json.Unmarshal(data, &updated), "decode update response: %s", string(data))
	require.Equal(t, applianceID, updated.Appliance.ID, "update should return the gateway created by this run")
	require.Len(t, updated.Appliance.Settings.ServiceEndpointGateway.EnabledServices, 1, "update should register the enabled service: %s", string(data))
	require.Equal(t, "ObjectStorage", updated.Appliance.Settings.ServiceEndpointGateway.EnabledServices[0].Type, "unexpected enabled service type")

	// 設定を反映する (応答本文は出力されない)
	// update 直後の反映は一時的に失敗することがある (SDK が 4xx 応答の内容を破権するため詳細は不可視) ため、
	// 失敗してもリトライする
	applyGateway(t, h, applianceID)

	// 反映後の設定を参照する
	read = readGateway(t, h, "反映後の設定を参照する", applianceID)
	require.NotEmpty(t, read.SettingsHash, "apply should set the settings hash: %+v", read)

	// 電源状態を確認する
	status := readPowerStatus(t, h, "電源状態を確認する", applianceID)
	require.Equal(t, "up", status, "the gateway should be up after creation")

	// 電源を停止する (応答本文は出力されない)
	h.Call(t, "ゲートウェイの電源を停止する", "service-endpoint-gateway-api", "appliance", "shutdown", applianceID)
	waitPowerStatus(t, h, "停止後の電源状態を確認する", applianceID, "down")

	// 電源を起動する
	data = h.Call(t, "ゲートウェイの電源を起動する", "service-endpoint-gateway-api", "appliance", "power-on", applianceID)
	var poweredOn struct {
		Success bool `json:"Success"`
	}
	require.NoError(t, json.Unmarshal(data, &poweredOn), "decode power-on response: %s", string(data))
	require.True(t, poweredOn.Success, "power-on should succeed: %s", string(data))
	waitPowerStatus(t, h, "起動後の電源状態を確認する", applianceID, "up")

	// reset も実行できることを確認する (応答本文は出力されない)
	h.Call(t, "ゲートウェイを再起動する", "service-endpoint-gateway-api", "appliance", "reset", applianceID)
	waitPowerStatus(t, h, "再起動後の電源状態を確認する", applianceID, "up")
}

// applyGateway は設定を反映します。update 直後の反映は一時的に失敗することがあるため、
// 期限までリトライします。
func applyGateway(t *testing.T, h *helper.Helper, id string) {
	t.Helper()

	deadline := time.Now().Add(availableTimeout)
	for {
		_, stderr, err := h.CallAllowingFailure(t, "接続先マネージドサービス設定を反映する",
			"service-endpoint-gateway-api", "appliance", "apply", id)
		if err == nil {
			return
		}
		if !time.Now().Before(deadline) {
			require.NoError(t, err, "apply did not succeed: %s", stderr)
		}
		h.Sleep(t, "接続先マネージドサービス設定を反映する", pollInterval)
	}
}

// gatewayItem はゲートウェイの参照結果のうち、確認に使う項目です。
// Interfaces[].SwitchID は生の応答では数値のため json.Number で受けます。
type gatewayItem struct {
	ID           string `json:"ID"`
	SettingsHash string `json:"SettingsHash"`
	SwitchID     string `json:"SwitchID"`
	Interfaces   []struct {
		SwitchID json.Number `json:"SwitchID"`
		Scope    string      `json:"Scope"`
	} `json:"Interfaces"`
}

// readGateway は skr CLI でゲートウェイを参照し、必要な項目に絞って返します。
func readGateway(t *testing.T, h *helper.Helper, description, id string) gatewayItem {
	t.Helper()

	data := h.Call(t, description,
		"service-endpoint-gateway-api", "appliance", "read", id,
		"--query", "{ID: .Appliance.ID,SettingsHash: .Appliance.SettingsHash,SwitchID: .Appliance.Remark.Switch.ID,Interfaces: [.Appliance.Interfaces[] | {SwitchID: .Switch.ID,Scope: .Switch.Scope}]}")
	var item gatewayItem
	require.NoError(t, json.Unmarshal(data, &item), "%s: decode read response: %s", description, string(data))
	require.NotEmpty(t, item.ID, "%s: read should return the gateway ID: %s", description, string(data))
	return item
}

// listGatewayIDs は skr CLI でゲートウェイを一覧し、ID の一覧を返します。
func listGatewayIDs(t *testing.T, h *helper.Helper, description string) []string {
	t.Helper()

	data := h.Call(t, description, "service-endpoint-gateway-api", "appliance", "list")
	var resp struct {
		Appliances []struct {
			ID string `json:"ID"`
		} `json:"Appliances"`
	}
	require.NoError(t, json.Unmarshal(data, &resp), "%s: decode list response: %s", description, string(data))
	ids := make([]string, 0, len(resp.Appliances))
	for _, item := range resp.Appliances {
		ids = append(ids, item.ID)
	}
	return ids
}

// readAvailability は skr CLI でゲートウェイの稼働状態を取得し、Availability を返します。
func readAvailability(t *testing.T, h *helper.Helper, description, id string) string {
	t.Helper()

	data := h.Call(t, description,
		"service-endpoint-gateway-api", "appliance", "read", id,
		"--query", "{Availability: .Appliance.Availability}")
	var item struct {
		Availability string `json:"Availability"`
	}
	require.NoError(t, json.Unmarshal(data, &item), "%s: decode availability: %s", description, string(data))
	return item.Availability
}

// waitGatewayAvailable はゲートウェイの稼働状態が利用可能になるまで待ちます。
// 作成処理の進行中は削除などの操作が行えないため、操作前に待ちます。
// 期限を過ぎても利用可能にならない場合はテストを失敗させます。
func waitGatewayAvailable(t *testing.T, h *helper.Helper, description, id string) string {
	t.Helper()

	deadline := time.Now().Add(availableTimeout)
	availability := readAvailability(t, h, description, id)
	for availability != "available" && time.Now().Before(deadline) {
		h.Sleep(t, description, pollInterval)
		availability = readAvailability(t, h, description, id)
	}
	require.Equal(t, "available", availability, "gateway %s did not become available", id)
	return availability
}

// readPowerStatus は skr CLI で電源状態を取得し、Status を返します。
func readPowerStatus(t *testing.T, h *helper.Helper, description, id string) string {
	t.Helper()

	data := h.Call(t, description,
		"service-endpoint-gateway-api", "appliance", "read-power-status", id,
		"--query", "{Status: .Instance.Status}")
	var item struct {
		Status string `json:"Status"`
	}
	require.NoError(t, json.Unmarshal(data, &item), "%s: decode power status: %s", description, string(data))
	return item.Status
}

// waitPowerStatus は電源状態が期待する値になるまで電源状態の取得を繰り返します。
// 期限を過ぎても期待する状態にならない場合はテストを失敗させます。
func waitPowerStatus(t *testing.T, h *helper.Helper, description, id, want string) string {
	t.Helper()

	deadline := time.Now().Add(powerStatusTimeout)
	status := readPowerStatus(t, h, description, id)
	for status != want && time.Now().Before(deadline) {
		h.Sleep(t, description, pollInterval)
		status = readPowerStatus(t, h, description, id)
	}
	require.Equal(t, want, status, "power status did not reach the expected state")
	return status
}

// deleteGateway は作成したゲートウェイを削除します。
// 起動中は削除できないため、電源を停止してから削除します。
// 途中の失敗でも後片付けを試み、不在確認まで行います。
func deleteGateway(t *testing.T, h *helper.Helper, id string) {
	t.Helper()

	// 稼働状態を確認する。作成処理の進行中は待ってから後片付けに進む
	availability := readAvailability(t, h, "後片付けの前にゲートウェイの稼働状態を確認する", id)
	if availability != "available" {
		waitGatewayAvailable(t, h, "後片付けの前にゲートウェイの作成処理の完了を待つ", id)
	}

	// 起動中は電源を停止する
	if status := readPowerStatus(t, h, "後片付けの前にゲートウェイの電源状態を確認する", id); status == "up" {
		h.Call(t, "後片付けとしてゲートウェイの電源を停止する", "service-endpoint-gateway-api", "appliance", "shutdown", id)
		waitPowerStatus(t, h, "後片付けとしてゲートウェイの停止を待つ", id, "down")
	}

	// ゲートウェイを削除する
	h.Call(t, "後片付けとしてゲートウェイを削除する", "service-endpoint-gateway-api", "appliance", "delete", id)
}

// deleteSwitch は skr CLI で作成したスイッチを削除します。
func deleteSwitch(t *testing.T, h *helper.Helper, zone, id string) {
	t.Helper()

	h.Call(t, "後片付けとしてスイッチを削除する", "iaas-api", "switch", "delete", "--zone", zone, "--id", id, "--fail-if-not-found")
}

// switchItem はスイッチの検索結果のうち、確認に使う項目です。
type switchItem struct {
	ID          types.ID
	Name        string
	Description string
}

// findSwitchesByName は名前でスイッチを検索して応答を解析します。
// Names はフラグで指定できないため、JSON 経路で渡します。
func findSwitchesByName(t *testing.T, h *helper.Helper, description, name string) []switchItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "switch", "find", "--request", findSwitchRequestJSON(t, name))
	var items []switchItem
	require.NoError(t, json.Unmarshal(data, &items), "%s: decode switches: %s", description, string(data))
	return items
}

// findSwitchRequestJSON は名前検索のリクエスト JSON を組み立てます。
func findSwitchRequestJSON(t *testing.T, name string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{"Zone": segZone, "Names": []string{name}})
	require.NoError(t, err, "encode switch find request")
	return string(data)
}

// parseSwitch はスイッチの応答を解析します。ID が空の場合はテストを失敗させます。
func parseSwitch(t *testing.T, data []byte) switchItem {
	t.Helper()

	var item switchItem
	require.NoError(t, json.Unmarshal(data, &item), "decode switch: %s", string(data))
	require.NotZero(t, item.ID, "switch ID is missing: %s", string(data))
	return item
}

// lookupSwitches は SDK で名前でスイッチを検索します。
func lookupSwitches(t *testing.T, client *iaas.Client, name string) []switchItem {
	t.Helper()

	switches, err := swytch.New(client).FindWithContext(t.Context(), &swytch.FindRequest{
		Zone:  segZone,
		Names: []string{name},
		Count: findPageSize,
	})
	require.NoError(t, err, "find switches by name %q", name)
	items := make([]switchItem, 0, len(switches))
	for _, s := range switches {
		if s == nil {
			continue
		}
		items = append(items, switchItem{ID: s.ID, Name: s.Name, Description: s.Description})
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), findPageSize, "switch find result page is full; cannot conclude absence of %q", name)
	return items
}
