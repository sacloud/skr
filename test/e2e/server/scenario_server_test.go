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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// updatedServerDescription は更新確認中に使用する一時的な説明です。
const updatedServerDescription = helper.ServerDescription + " (updated)"

// updatedServerName は更新確認中に使用する一時的な名前です。
func updatedServerName(name string) string {
	return name + "-upd"
}

// TestScenarioServer は Server E2E です。cloud-init ディスク付きの Server を is1b に作成し、
// user-data で起動して SSH 接続を確認し、名前と説明の更新・復元と後片付けまで行います。
// cloud-init アーカイブとディスクプランは SDK で自動選択します。
// テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認と起動待ちのポーリングは
// SDK で静かに実行します。途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioServer(t *testing.T) {
	h := helper.New(t, "server-api", "IaaS サーバーを cloud-init ディスク付きで作成・起動し、SSH 接続・更新・削除と後片付けが正しく動作することを確認する")

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	servers, disks := serverSDK.New(client), diskSDK.New(client)

	// cloud-init アーカイブとディスクプランを SDK で自動選択します。
	// アーカイブは Ubuntu Server 24.04 LTS (cloudimg) に限定して選択します
	params := helper.RequireServerDiskParams(t, client)

	// 一時的な SSH 鍵と cloud-init の user-data を作成します。秘密鍵の内容は実行記録に残りません
	material := helper.WriteSSHMaterial(t, helper.ServerSSHUser, false)

	name := helper.ServerNamePrefix + helper.RandomSuffix(t)

	// 作成前に Server と Disk の両方で同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	helper.RequireNoServerNameCollision(t, servers, disks, name)

	// 後片付け: Server を識別情報の再照合つきで削除し、孤立ディスクの回収と不在確認まで行う。
	// 更新フェーズで復元が完了していない場合は更新後の識別情報も許容する。
	var serverID types.ID
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, serverID, []helper.ServerIdentity{
			{Name: name, Description: helper.ServerDescription},
			{Name: updatedServerName(name), Description: updatedServerDescription},
		}); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// テスト用の Server を cloud-init ディスクと共有 NIC 付きで作成し、作成した Disk も含めて確認する
	item := helper.CreateCloudInitServer(t, h, disks, name, helper.ServerDescription, params)
	serverID = item.ID

	// server read が作成済みの Server を返すことを確認する。--query で確認に必要な項目へ絞り込みます
	data, _, err := h.CallAllowingFailure(t, "作成したサーバーを参照する",
		"iaas-api", "server", "read", "--zone", helper.ServerZone, "--id", serverID.String(),
		"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}",
	)
	require.NoError(t, err, "read the created Server")
	readItem, err := helper.ParseServerOutput(data)
	require.NoError(t, err)
	require.NoError(t, helper.CheckServerIdentity(readItem, serverID, name, helper.ServerDescription),
		"server read should return the created Server")

	// server find --name が作成した Server を見つけられることを確認します。--name は部分一致検索です
	data, _, err = h.CallAllowingFailure(t, "作成したサーバーを名前で検索する",
		"iaas-api", "server", "find",
		"--zone", helper.ServerZone, "--name", name,
		"--query", "map({ID,Name,Description})",
	)
	require.NoError(t, err, "find the created Server by name")
	matches, err := helper.DecodeServerList(data)
	require.NoError(t, err)
	require.Len(t, matches, 1, "server find --name should return only the created Server")
	require.Equal(t, serverID, matches[0].ID, "server find --name should return the created Server")

	// cloud-init の user-data を指定して起動する。user-data は @path 形式でのみ渡し、内容を引数に含めません
	helper.BootServerWithUserData(t, h, serverID, material.UserDataPath)

	// 起動を確認しながら SSH ログインできるまで待つ
	helper.WaitForServerSSH(t, h, servers, serverID, name, helper.ServerDescription, material)

	// サーバーの名前と説明を更新して元に戻す
	updateAndVerifyServer(t, h, serverID, name)
}

// updateAndVerifyServer は Server の名前と説明を skr CLI で更新し、応答で反映を確認してから
// 元の値に戻し、復元を確認します。復元により後片付けが Server を再び識別できるようになります。
func updateAndVerifyServer(t *testing.T, h *helper.Helper, id types.ID, name string) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "サーバーの名前と説明を更新する",
		"iaas-api", "server", "update",
		"--zone", helper.ServerZone, "--id", id.String(),
		"--name", updatedServerName(name), "--description", updatedServerDescription,
		// 応答は単一オブジェクトのため、--query はオブジェクト構築で確認に必要な項目へ絞り込みます
		"--query", "{ID,Name,Description}",
	)
	require.NoError(t, err, "update Server Name and Description")
	item, err := helper.ParseServerOutput(data)
	require.NoError(t, err)
	require.NoError(t, helper.CheckServerIdentity(item, id, updatedServerName(name), updatedServerDescription),
		"the update should be reflected in the response")

	data, _, err = h.CallAllowingFailure(t, "サーバーの名前と説明を元に戻す",
		"iaas-api", "server", "update",
		"--zone", helper.ServerZone, "--id", id.String(),
		"--name", name, "--description", helper.ServerDescription,
		"--query", "{ID,Name,Description}",
	)
	require.NoError(t, err, "restore Server Name and Description")
	item, err = helper.ParseServerOutput(data)
	require.NoError(t, err)
	require.NoError(t, helper.CheckServerIdentity(item, id, name, helper.ServerDescription),
		"the restore should be reflected in the response")
}
