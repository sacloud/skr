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
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

const (
	// nfsZone は NFS の CRUD を確認するゾーンです。tk1a はスイッチのゾーン内上限に
	// 余裕があり、database (is1c) や switch (is1b) の E2E とゾーンを共有しません。
	nfsZone = "tk1a"
	// nfsNamePrefix はテスト用 NFS の名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	nfsNamePrefix = "skr-e2e-nfs-"
	// nfsDescription はテスト用 NFS に付ける説明です。後片付けの識別にも使います。
	nfsDescription = "Temporary NFS appliance for skr API E2E"
	// nfsIPAddress はテスト用 NFS に割り当てる IP アドレスです。
	nfsIPAddress = "192.168.0.11"
	// nfsNetworkMaskLen はテスト用ネットワークのプレフィックス長です。
	nfsNetworkMaskLen = 24
	// nfsCreateTimeout は NFS の作成処理 (起動待ちを含む) の期限です。
	// アプライアンスの作成は起動まで時間がかかります。
	nfsCreateTimeout = 25 * time.Minute
	// nfsPowerTimeout は電源状態の反映を待つ期限です。
	nfsPowerTimeout = 25 * time.Minute
	// nfsUpdateTimeout は更新の反映を待つ期限です。
	nfsUpdateTimeout = 10 * time.Minute
	// nfsDeleteTimeout は削除の完了を待つ期限です。
	nfsDeleteTimeout = 10 * time.Minute
	// nfsFindPageSize は SDK の安全確認で使う1ページの件数です。
	nfsFindPageSize = 100
)

// nfsItem は NFS の参照結果のうち、確認に使う項目です。
// SDK の NFS モデルの JSON は Go のフィールド名で出力されます。
type nfsItem struct {
	ID             types.ID
	Name           string
	Description    string
	Tags           []string
	PlanID         types.ID
	SwitchID       types.ID
	NetworkMaskLen int
	IPAddresses    []string
	InstanceStatus types.EServerInstanceStatus
	Availability   types.EAvailability
}

// ipAddress は割り当てた最初の IP アドレスを返します。
func (item nfsItem) ipAddress() string {
	if len(item.IPAddresses) == 0 {
		return ""
	}
	return item.IPAddresses[0]
}

// TestScenarioNFS は nfs-api コマンドのライブ E2E です。tk1a でテスト用スイッチと
// NFS (SSD プランの最小サイズ) を作成し、作成から参照、名前検索、モニタリング、
// 更新、電源操作 (shutdown/boot/reset)、削除と不在確認までを検証します。
func TestScenarioNFS(t *testing.T) {
	h := helper.New(t, "nfs-api", "IaaS NFS を作成・参照・検索・監視・更新・電源操作・削除し、NFS リソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := nfsNamePrefix + suffix
	updatedDescription := nfsDescription + " (updated)"
	// created は後片付けの中で ID 照合に使うため、作成前に宣言する
	var created nfsItem

	// 作成前の同名確認を実施する
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	require.Empty(t, lookupNFS(t, client, name), "NFS name %q already exists; refusing to modify it", name)

	// テストで使うプラン ID は Note (sys-nfs) のプラン一覧から、SSD プランの
	// 最小サイズを解決します。リソースを変更しない安全確認のため SDK 直呼びとします
	planID, planSize := resolveNFSPlan(t, client, nfsZone)
	t.Logf("resolved NFS SSD plan: id=%d size=%dGB", planID, planSize)

	// プラン解決の結果を高レベルコマンド skr nfs plans の出力と照合します。
	// デフォルトは利用可能 (available) なプランのみで、解決したプラン ID が含まれます
	plansData := h.Call(t, "高レベルコマンドでゾーンの NFS プラン一覧を確認する",
		"nfs", "plans", "--zone", nfsZone)
	var plans []struct {
		Storage      string
		Size         int
		PlanID       types.ID
		Availability string
		Cloneable    bool
	}
	require.NoError(t, json.Unmarshal(plansData, &plans), "decode nfs plans output")
	require.NotEmpty(t, plans, "nfs plans output should not be empty")
	for _, plan := range plans {
		require.Equal(t, "available", plan.Availability,
			"default nfs plans output should contain available plans only")
	}
	require.True(t, slices.ContainsFunc(plans, func(plan struct {
		Storage      string
		Size         int
		PlanID       types.ID
		Availability string
		Cloneable    bool
	}) bool {
		return plan.PlanID == planID
	}),
		"resolved plan %d should be listed by skr nfs plans", planID)

	// --all は利用不可 (disabled) のプランも含め、デフォルト出力のプランを
	// すべて含みます
	allPlansData := h.Call(t, "--all で利用不可のプランも含めた NFS プラン一覧を確認する",
		"nfs", "plans", "--zone", nfsZone, "--all")
	var allPlans []struct {
		PlanID       types.ID
		Availability string
	}
	require.NoError(t, json.Unmarshal(allPlansData, &allPlans), "decode nfs plans --all output")
	for _, available := range plans {
		require.True(t, slices.ContainsFunc(allPlans, func(plan struct {
			PlanID       types.ID
			Availability string
		}) bool {
			return plan.PlanID == available.PlanID
		}),
			"plan %d listed by default output should also be listed with --all", available.PlanID)
	}

	// テスト用のスイッチを作成する。NFS はスイッチへの接続が必須のため
	switchName := "skr-e2e-nfs-switch-" + suffix
	switchData := h.Call(t, "NFS の接続先となるスイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", nfsZone,
		"--name", switchName,
		"--description", nfsDescription)
	var createdSwitch struct {
		ID          types.ID
		Name        string
		Description string
	}
	require.NoError(t, json.Unmarshal(switchData, &createdSwitch), "decode created switch")
	require.Equal(t, switchName, createdSwitch.Name, "created switch name should match the request")
	require.Equal(t, nfsDescription, createdSwitch.Description, "created switch description should match the request")

	defer func() {
		// スイッチを削除する前に、保持した ID と識別情報を再照合する。
		// NFS の削除は NFS の後片付けで行う
		switchOp := iaas.NewSwitchOp(client)
		current, err := switchOp.Read(t.Context(), nfsZone, createdSwitch.ID)
		if iaas.IsNotFoundError(err) {
			return // 不在なら変更せず終了する
		}
		require.NoError(t, err, "re-verify switch %s before cleanup", createdSwitch.ID)
		require.NotNil(t, current, "re-verified switch should not be nil")
		require.Equal(t, createdSwitch.ID, current.ID, "cleanup target switch ID mismatch")
		require.Equal(t, switchName, current.Name, "cleanup target switch name mismatch")
		require.Equal(t, nfsDescription, current.Description, "cleanup target switch description mismatch")
		// スイッチに接続する NFS が残っていないことを確認する。
		// 接続が残る場合や照合に失敗した場合はスイッチを削除しない (背景は ADR 0064)
		remaining := lookupNFS(t, client, name)
		for _, item := range remaining {
			if item.ID == created.ID {
				t.Errorf("NFS %s still exists; refusing to delete the switch %s", created.ID, createdSwitch.ID)
				return
			}
			require.NotEqual(t, createdSwitch.ID, item.SwitchID,
				"NFS %s still connects to the switch %s; refusing to delete it", item.ID, createdSwitch.ID)
		}
		_ = h.Call(t, "テスト用スイッチを削除する",
			"iaas-api", "switch", "delete", "--zone", nfsZone, "--id", createdSwitch.ID.String(), "--fail-if-not-found")
		// 削除後の不在確認を行う
		_, err = switchOp.Read(t.Context(), nfsZone, createdSwitch.ID)
		require.True(t, iaas.IsNotFoundError(err), "switch %s should not remain after deletion", createdSwitch.ID)
	}()

	// NFS を作成する。作成リクエストは起動の完了を待たずに応答を返すため、
	// 作成後に SDK で起動状態を待ち合わせる
	createArgs := []string{
		"nfs-api", "create",
		"--zone", nfsZone,
		"--name", name,
		"--description", nfsDescription,
		"--tags", "tutorial,managed",
		"--plan-id", planID.String(),
		"--switch-id", createdSwitch.ID.String(),
		"--ip-address", nfsIPAddress,
		"--network-mask-len", strconv.Itoa(nfsNetworkMaskLen),
	}
	data := h.CallWithTimeout(t, nfsCreateTimeout, "NFS を作成する (起動の完了は待たない)", createArgs...)
	created = parseNFS(t, data)
	require.NotZero(t, created.ID, "created NFS ID is missing")

	defer func() {
		// NFS を削除する前に、保持した ID と識別情報を再照合する
		nfsOp := iaas.NewNFSOp(client)
		current, err := nfsOp.Read(t.Context(), nfsZone, created.ID)
		if iaas.IsNotFoundError(err) {
			return // 不在なら変更せず終了する
		}
		require.NoError(t, err, "re-verify NFS %s before cleanup", created.ID)
		require.NotNil(t, current, "re-verified NFS should not be nil")
		require.Equal(t, created.ID, current.ID, "cleanup target NFS ID mismatch")
		require.Equal(t, name, current.Name, "cleanup target NFS name mismatch")
		// 説明はこのシナリオ後半で更新するため、作成直後の説明と更新後の説明のどちらも今回設定した値として扱う
		require.Contains(t, []string{nfsDescription, updatedDescription}, current.Description,
			"cleanup target NFS description mismatch")
		// 接続先と IP 設定を再照合する。保持したスイッチ ID、指定した IP アドレスとマスク長が
		// 現在値と一致しない場合は削除を呼ばない (背景は ADR 0064)
		require.Equal(t, createdSwitch.ID, current.SwitchID, "cleanup target NFS switch mismatch")
		require.NotEmpty(t, current.IPAddresses, "cleanup target NFS should have an IP address")
		require.Equal(t, nfsIPAddress, current.IPAddresses[0], "cleanup target NFS IP address mismatch")
		require.Equal(t, nfsNetworkMaskLen, current.NetworkMaskLen, "cleanup target NFS network mask length mismatch")
		// NFS を削除し、名前でも残っていないことを確認する
		_ = h.CallWithTimeout(t, nfsDeleteTimeout, "NFS を削除する (電源 OFF してから削除する)",
			"nfs-api", "delete",
			"--zone", nfsZone, "--id", created.ID.String(), "--force", "--fail-if-not-found")
		remaining := lookupNFS(t, client, name)
		for _, item := range remaining {
			require.NotEqual(t, created.ID, item.ID, "NFS %s should not remain after deletion", created.ID)
		}
	}()

	// 作成応答の内容を確認する
	require.Equal(t, name, created.Name)
	require.Equal(t, nfsDescription, created.Description)
	require.Subset(t, created.Tags, []string{"tutorial", "managed"}, "create should return the requested tags")
	require.Equal(t, planID, created.PlanID, "create should return the requested plan")
	require.Equal(t, createdSwitch.ID, created.SwitchID, "create should return the connected switch")
	require.Equal(t, nfsIPAddress, created.ipAddress(), "create should return the assigned IP address")
	require.Equal(t, nfsNetworkMaskLen, created.NetworkMaskLen, "create should return the assigned network mask length")

	// 作成リクエストは起動の完了を待たないため、SDK で起動状態を待ち合わせる。
	// リソースを変更しない安全確認のため SDK 直呼びとする (背景は ADR 0064)
	waitForNFSReady(t, client, created.ID)

	// 作成した NFS を参照し、接続先と割り当てた IP を確認する
	read := readNFS(t, h, "作成した NFS を参照する", created.ID)
	require.Equal(t, name, read.Name)
	require.Equal(t, planID, read.PlanID, "read should return the requested plan")
	require.Equal(t, createdSwitch.ID, read.SwitchID, "read should return the connected switch")
	require.Equal(t, nfsIPAddress, read.ipAddress(), "read should return the assigned IP address")
	require.Equal(t, nfsNetworkMaskLen, read.NetworkMaskLen, "read should return the assigned network mask length")
	require.Equal(t, types.ServerInstanceStatuses.Up, read.InstanceStatus, "read should return the up state")

	// 名前の部分一致検索 (--name は1要素の Names に変換される) で作成した NFS を検索する
	data = h.Call(t, "名前で部分一致検索する",
		"nfs-api", "find", "--zone", nfsZone, "--name", name, "--count", strconv.Itoa(nfsFindPageSize))
	var found []nfsItem
	require.NoError(t, json.Unmarshal(data, &found), "decode NFS find result")
	require.NotEmpty(t, found, "NFS find by name should return the created NFS %s", created.ID)
	var matched *nfsItem
	for i := range found {
		if found[i].ID == created.ID {
			matched = &found[i]
		}
	}
	require.NotNil(t, matched, "NFS find should contain the created NFS %s", created.ID)

	// タグでも検索する (--tags はカンマ区切りの Tags に変換される)
	data = h.Call(t, "タグで検索する",
		"nfs-api", "find", "--zone", nfsZone, "--tags", "tutorial,managed", "--count", strconv.Itoa(nfsFindPageSize))
	require.NoError(t, json.Unmarshal(data, &found), "decode NFS find result")
	matched = nil
	for i := range found {
		if found[i].ID == created.ID {
			matched = &found[i]
		}
	}
	require.NotNil(t, matched, "NFS find by tags should contain the created NFS %s", created.ID)

	// モニタリング系のコマンドを実行する。起動直後はデータが存在しないことがあるため、
	// エラーでなく正常な応答が返ることを確認する
	for _, op := range []string{"monitor-cpu", "monitor-interface"} {
		monitorData := h.Call(t, op+" のモニタリングデータを取得する",
			"nfs-api", op, "--zone", nfsZone, "--id", created.ID.String())
		var values []map[string]any
		require.NoError(t, json.Unmarshal(monitorData, &values), "decode %s result", op)
		require.NotNil(t, values, "%s should return a JSON array", op)
	}

	// 空きディスク容量は日単位のモニタリングのため、正時 (00:00:00) の期間を指定する。
	// 作成日のデータは日が変わるまで確定しないため、直近2日の期間で正常な応答を確認する
	monitorEnd := time.Now().UTC().Truncate(24 * time.Hour)
	monitorStart := monitorEnd.Add(-48 * time.Hour)
	monitorData := h.Call(t, "日単位の期間を指定して monitor-free-disk-size のモニタリングデータを取得する",
		"nfs-api", "monitor-free-disk-size",
		"--zone", nfsZone, "--id", created.ID.String(),
		"--start", monitorStart.Format(time.RFC3339),
		"--end", monitorEnd.Format(time.RFC3339))
	var freeDiskValues []map[string]any
	require.NoError(t, json.Unmarshal(monitorData, &freeDiskValues), "decode monitor-free-disk-size result")
	require.NotNil(t, freeDiskValues, "monitor-free-disk-size should return a JSON array")

	// 名前と説明とタグを更新する。update は未指定の項目を空の値として送信するため、
	// 既存の値を維持する項目もあわせて指定する
	h.Call(t, "NFS の名前・説明・タグを更新する",
		"nfs-api", "update",
		"--zone", nfsZone,
		"--id", created.ID.String(),
		"--name", name,
		"--description", updatedDescription,
		"--tags", "tutorial,managed,updated")

	// 更新した説明を参照で確認する。反映待ちは期限付きで行う
	var updated nfsItem
	require.Eventually(t, func() bool {
		item, err := readNFSForRetry(t, h, created.ID)
		if err != nil {
			return false
		}
		updated = item
		return item.Description == updatedDescription
	}, nfsUpdateTimeout, 30*time.Second, "update should change the description to %q", updatedDescription)
	require.Equal(t, updatedDescription, updated.Description, "update should change the description")
	// タグの順序は API 側で並べ替えられるため、要素の一致で照合する
	require.ElementsMatch(t, types.Tags{"tutorial", "managed", "updated"}, updated.Tags, "update should change the tags")

	// 電源操作を確認する。shutdown は電源 OFF まで、boot は起動完了まで待つ
	h.CallWithTimeout(t, nfsPowerTimeout, "NFS をシャットダウンする (電源 OFF まで待つ)",
		"nfs-api", "shutdown", "--zone", nfsZone, "--id", created.ID.String())
	read = readNFS(t, h, "シャットダウン後の NFS を参照する", created.ID)
	require.Equal(t, types.ServerInstanceStatuses.Down, read.InstanceStatus, "shutdown should turn the NFS down")

	h.CallWithTimeout(t, nfsPowerTimeout, "NFS を起動する (起動完了まで待つ)",
		"nfs-api", "boot", "--zone", nfsZone, "--id", created.ID.String())
	read = readNFS(t, h, "起動後の NFS を参照する", created.ID)
	require.Equal(t, types.ServerInstanceStatuses.Up, read.InstanceStatus, "boot should turn the NFS up")

	h.Call(t, "NFS をリセットする",
		"nfs-api", "reset", "--zone", nfsZone, "--id", created.ID.String())

	// リセット後も参照できることを確認する
	readAfterReset := readNFS(t, h, "リセット後に NFS を参照する", created.ID)
	require.Equal(t, name, readAfterReset.Name)
}

// readNFS は指定した ID の NFS を参照して応答を解析します。
func readNFS(t *testing.T, h *helper.Helper, description string, id types.ID) nfsItem {
	t.Helper()

	data := h.Call(t, description,
		"nfs-api", "read", "--zone", nfsZone, "--id", id.String())
	return parseNFS(t, data)
}

// readNFSForRetry は更新の反映待ちで使う参照です。エラーを値として返します。
func readNFSForRetry(t *testing.T, h *helper.Helper, id types.ID) (nfsItem, error) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "NFS を参照する (更新の反映待ち)",
		"nfs-api", "read", "--zone", nfsZone, "--id", id.String())
	if err != nil {
		return nfsItem{}, err
	}
	var item nfsItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nfsItem{}, err
	}
	return item, nil
}

// parseNFS は NFS の応答を解析します。ID が空の場合はテストを失敗させます。
func parseNFS(t *testing.T, data []byte) nfsItem {
	t.Helper()

	var item nfsItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode NFS: %v", err)
	require.NotZero(t, item.ID, "NFS ID is missing")
	return item
}

// waitForNFSReady は SDK で NFS の起動状態を待ち合わせます。
// 作成リクエストは起動の完了を待たないため、起動確認が必要なステップの前に呼び出します。
// リソースを変更しない安全確認のため SDK 直呼びとします (背景は ADR 0064)。
func waitForNFSReady(t *testing.T, client *iaas.Client, id types.ID) {
	t.Helper()

	op := iaas.NewNFSOp(client)
	require.Eventually(t, func() bool {
		nfs, err := op.Read(t.Context(), nfsZone, id)
		if err != nil {
			return false
		}
		return nfs != nil && nfs.InstanceStatus == types.ServerInstanceStatuses.Up
	}, nfsCreateTimeout, 15*time.Second, "NFS %s should become up", id)
}

// lookupNFS は SDK で名前で NFS を検索します。
// 作成前の同名確認と削除後の不在確認に使います。
func lookupNFS(t *testing.T, client *iaas.Client, name string) []nfsItem {
	t.Helper()

	op := iaas.NewNFSOp(client)
	result, err := op.Find(t.Context(), nfsZone, &iaas.FindCondition{
		Count:  nfsFindPageSize,
		Filter: map[search.FilterKey]any{search.Key("Name"): search.AndEqual(name)},
	})
	require.NoError(t, err, "find NFS by name %q", name)
	items := make([]nfsItem, 0, len(result.NFS))
	for _, nfs := range result.NFS {
		if nfs == nil {
			continue
		}
		items = append(items, nfsItem{
			ID:       nfs.ID,
			Name:     nfs.Name,
			SwitchID: nfs.SwitchID,
		})
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), nfsFindPageSize, "NFS find result page is full; cannot conclude absence of %q", name)
	return items
}
