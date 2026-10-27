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
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioServer は Server E2E です。cloud-init ディスク付きの Server を is1b に作成し、
// user-data で起動して SSH 接続を確認し、名前と説明の更新・復元と後片付けまで行います。
// ライブ実行には is1b で利用可能な cloud-init アーカイブとディスクプランの環境変数が必要です。
func TestScenarioServer(t *testing.T) {
	h := helper.New(t, "server-api", "IaaS サーバーを cloud-init ディスク付きで作成・起動し、SSH 接続・更新・削除と後片付けが正しく動作することを確認する")

	// ライブ実行には is1b で利用可能な cloud-init アーカイブとディスクプランの設定が必要です
	params := requireServerParams(t)

	// 一時的な SSH 鍵と cloud-init の user-data を作成します。秘密鍵の内容は実行記録に残りません
	material := helper.WriteSSHMaterial(t, sshUser, false)
	c, err := newSKRServerCommands(h, material, sshUser)
	require.NoError(t, err, "build Server E2E commands")

	run := &serverRun{
		name:       namePrefix + helper.RandomSuffix(t),
		params:     params,
		material:   material,
		sshTimeout: sshWaitTimeout,
	}

	err = runServerScenario(t, c, time.Sleep, run)
	require.NoError(t, err, "Server E2E scenario failed: %v", err)
}

// runServerScenario は1回の実行で、作成前の同名確認、Server と cloud-init ディスクの作成、
// user-data による起動、SSH 接続確認、名前と説明の更新と復元、後片付けまで行います。
// テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認と起動待ちのポーリングは
// SDK で静かに実行します。途中の失敗でも後片付けは実行され、その失敗はエラーに含まれます。
func runServerScenario(t *testing.T, c serverCommands, sleep func(time.Duration), run *serverRun) (result error) {
	// 作成前に Server と Disk の両方で同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	if err := requireNoServerNameCollision(t, c, run.name); err != nil {
		return err
	}

	// 後片付け: Server を識別情報の再照合つきで削除し、孤立ディスクの回収と不在確認まで行う
	defer func() {
		if err := cleanupServerResources(t, c, run); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", err))
		}
	}()

	// テスト用の Server を cloud-init ディスクと共有 NIC 付きで作成し、参照と名前検索で確認する
	if err := createServer(t, c, run); err != nil {
		return err
	}

	// cloud-init の user-data を指定して起動する
	if err := c.bootServer(t, "cloud-init の user-data を指定してサーバーを起動する", run.serverID, run.material.UserDataPath); err != nil {
		return err
	}

	// 起動を確認しながら SSH ログインできるまで待つ
	if err := waitForServerSSH(t, c, sleep, run); err != nil {
		return err
	}

	// サーバーの名前と説明を更新して元に戻す
	return updateAndVerifyServer(t, c, run.serverID, run.name)
}

// requireNoServerNameCollision は作成前に Server と Disk の両方で同名リソースが存在しないことを
// SDK で確認します。確認の実行は証跡に残しません。既存リソースは変更せず、存在する場合はエラーを返します。
func requireNoServerNameCollision(t *testing.T, c serverCommands, name string) error {
	t.Helper()

	servers, err := c.lookupServers(t, name)
	if err != nil {
		return fmt.Errorf("find existing servers: %w", err)
	}
	if len(servers) != 0 {
		return fmt.Errorf("test server name %q already exists; refusing to modify it", name)
	}
	disks, err := c.lookupDisks(t, name)
	if err != nil {
		return fmt.Errorf("find existing disks: %w", err)
	}
	if len(disks) != 0 {
		return fmt.Errorf("test disk name %q already exists; refusing to modify it", name)
	}
	return nil
}

// createServer は cloud-init ディスクと共有 NIC を持つテスト用 Server を skr CLI で作成し、
// 参照と名前検索で作成結果を確認します。作成に成功した ID を run に記録し、
// 後片付けが確実に回収できるようにします。
func createServer(t *testing.T, c serverCommands, run *serverRun) error {
	t.Helper()

	// 作成応答に作成結果が含まれる
	item, err := c.createServer(t, "テスト用のサーバーを cloud-init ディスク付きで作成する", run.name, run.params)
	if err != nil {
		return err
	}
	if item.ID.IsEmpty() {
		return errors.New("created Server ID is empty")
	}
	if err := checkServer(item, item.ID, run.name); err != nil {
		return err
	}
	run.serverID = item.ID

	// server read が作成済みの Server を返すことを確認する
	item, err = c.readServer(t, "作成したサーバーを参照する", run.serverID)
	if err != nil {
		return err
	}
	if err := checkServer(item, run.serverID, run.name); err != nil {
		return err
	}

	// server find --name が作成した Server を見つけられることを確認する
	matches, err := c.findServers(t, "作成したサーバーを名前で検索する", run.name)
	if err != nil {
		return err
	}
	if len(matches) != 1 || matches[0].ID != run.serverID {
		return fmt.Errorf("server find --name should return the created Server %s", run.serverID)
	}
	return nil
}

// waitForServerSSH は SDK で起動と共有 NIC の IP アドレスを確認し、SSH ログインが
// 成功するまで期限付きで待ちます。起動待ちのポーリングは証跡に残しません。
// cloud-init の反映はベストエフォートで待ち、期限を過ぎたらエラーにします。
// SSH の接続試行は実行記録に残り、成功の判断は hostname の取得で行います。
func waitForServerSSH(t *testing.T, c serverCommands, sleep func(time.Duration), run *serverRun) error {
	t.Helper()

	deadline := time.Now().Add(run.sshTimeout)
	for {
		item, err := c.fetchServer(t, run.serverID)
		if err != nil {
			return err
		}
		if err := checkServer(item, run.serverID, run.name); err != nil {
			return err
		}
		if item.InstanceStatus.IsUp() {
			if ip := sharedIPOfServer(item); ip != "" {
				err := c.connectSSH(t, "SSH ログインを確認する", ip)
				if err == nil {
					return nil
				}
				t.Logf("SSH login attempt failed, retrying: %v", err)
			}
		}
		if time.Now().Add(sshPolling).After(deadline) {
			return fmt.Errorf("SSH login check timed out after %s", run.sshTimeout)
		}
		sleep(sshPolling)
	}
}

// sharedIPOfServer は Server の参照結果から共有 NIC の IPv4 アドレスを返します。ない場合は空文字列を返します。
func sharedIPOfServer(item serverItem) string {
	for _, iface := range item.Interfaces {
		if iface.UpstreamType != "shared" || net.ParseIP(iface.IPAddress) == nil {
			continue
		}
		return iface.IPAddress
	}
	return ""
}
