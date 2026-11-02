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
	"os"
	"path/filepath"
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
	// dbZone はデータベースの CRUD を確認するゾーンです。is1b と is1c はスイッチの
	// ゾーン内上限が 1/1 のため、上限が緩い is1c を使います。
	dbZone = "is1c"
	// dbNamePrefix はテスト用データベースの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	dbNamePrefix = "skr-e2e-database-"
	// dbDescription はテスト用データベースに付ける説明です。後片付けの識別にも使います。
	dbDescription = "Temporary database for skr API E2E"
	// dbPlanID はテストで使うデータベースプランの ID です。10 は標準の 10GB プラン
	// (1core-2gb-10gb) に対応し、ゾーンによらず同じ ID を使えます。
	dbPlanID = 10
	// dbIPAddress はテスト用データベースに割り当てる IP アドレスです。
	dbIPAddress = "192.168.0.11"
	// dbNetworkMaskLen はテスト用ネットワークのプレフィックス長です。
	dbNetworkMaskLen = 24
	// dbUsername はデータベースの管理ユーザー名です。
	dbUsername = "e2euser"
	// dbCreateTimeout はデータベースの作成処理 (起動待ちを含む) の期限です。
	dbCreateTimeout = 25 * time.Minute
	// dbPowerTimeout は電源状態の反映を待つ期限です。
	dbPowerTimeout = 25 * time.Minute
	// dbUpdateTimeout は更新の反映を待つ期限です。
	dbUpdateTimeout = 10 * time.Minute
	// dbDeleteTimeout は削除の完了を待つ期限です。
	dbDeleteTimeout = 10 * time.Minute
	// dbFindPageSize は SDK の安全確認で使う1ページの件数です。
	dbFindPageSize = 100
)

// databaseItem はデータベースの参照結果のうち、確認に使う項目です。
// SDK の Database モデルの JSON は Go のフィールド名で出力されます。
type databaseItem struct {
	ID             types.ID
	Name           string
	Description    string
	Tags           []string
	PlanID         types.ID
	SwitchID       types.ID
	NetworkMaskLen int
	IPAddresses    []string
	CommonSetting  *databaseCommonSetting
}

// databaseCommonSetting はデータベースの共通設定のうち、確認に使う項目です。
// API 応答の既定ユーザーは DefaultUser フィールドで返ります。
type databaseCommonSetting struct {
	DefaultUser string
}

// ipAddress は割り当てた最初の IP アドレスを返します。
func (item databaseItem) ipAddress() string {
	if len(item.IPAddresses) == 0 {
		return ""
	}
	return item.IPAddresses[0]
}

// TestScenarioDatabase は database コマンドのライブ E2E です。is1c でテスト用スイッチと
// データベース (MariaDB、10GB プラン) を作成し、作成から参照、パラメータ一覧、名前検索、
// モニタリング、更新、電源操作 (shutdown/boot/reset)、削除と不在確認までを検証します。
// パスワードは一時ファイル経由で渡し、実行記録に秘密情報を残しません。
func TestScenarioDatabase(t *testing.T) {
	h := helper.New(t, "database-api", "IaaS データベースを作成・参照・検索・監視・更新・電源操作・削除し、データベースリソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := dbNamePrefix + suffix
	updatedDescription := dbDescription + " (updated)"
	password := "e2e-db-" + suffix + "-pass"
	// created は後片てつきの中で ID 照合に使うため、作成前に宣言する
	var created databaseItem

	// 作成前の同名確認を実施する
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	require.Empty(t, lookupDatabases(t, client, name), "database name %q already exists; refusing to modify it", name)

	// テスト用のスイッチを作成する。データベースはスイッチへの接続が必須のため
	switchName := "skr-e2e-database-switch-" + suffix
	switchData := h.Call(t, "データベースの接続先となるスイッチを作成する",
		"iaas-api", "switch", "create",
		"--zone", dbZone,
		"--name", switchName,
		"--description", dbDescription)
	var createdSwitch struct {
		ID          types.ID
		Name        string
		Description string
	}
	require.NoError(t, json.Unmarshal(switchData, &createdSwitch), "decode created switch")
	require.Equal(t, switchName, createdSwitch.Name, "created switch name should match the request")
	require.Equal(t, dbDescription, createdSwitch.Description, "created switch description should match the request")

	defer func() {
		// スイッチを削除する前に、保持した ID と識別情報を再照合する。
		// データベースの削除はデータベースの後片付けで行う
		switchOp := iaas.NewSwitchOp(client)
		current, err := switchOp.Read(t.Context(), dbZone, createdSwitch.ID)
		if iaas.IsNotFoundError(err) {
			return // 不在なら変更せず終了する
		}
		require.NoError(t, err, "re-verify switch %s before cleanup", createdSwitch.ID)
		require.NotNil(t, current, "re-verified switch should not be nil")
		require.Equal(t, createdSwitch.ID, current.ID, "cleanup target switch ID mismatch")
		require.Equal(t, switchName, current.Name, "cleanup target switch name mismatch")
		require.Equal(t, dbDescription, current.Description, "cleanup target switch description mismatch")
		// スイッチに接続するデータベースが残っていないことを確認する。
		// 接続が残る場合や照合に失敗した場合はスイッチを削除しない (背景は ADR 0064)
		remaining := lookupDatabases(t, client, name)
		for _, item := range remaining {
			if item.ID == created.ID {
				t.Errorf("database %s still exists; refusing to delete the switch %s", created.ID, createdSwitch.ID)
				return
			}
			require.NotEqual(t, createdSwitch.ID, item.SwitchID,
				"database %s still connects to the switch %s; refusing to delete it", item.ID, createdSwitch.ID)
		}
		_ = h.Call(t, "テスト用スイッチを削除する",
			"iaas-api", "switch", "delete", "--zone", dbZone, "--id", createdSwitch.ID.String(), "--fail-if-not-found")
		// 削除後の不在確認を行う
		_, err = switchOp.Read(t.Context(), dbZone, createdSwitch.ID)
		require.True(t, iaas.IsNotFoundError(err), "switch %s should not remain after deletion", createdSwitch.ID)
	}()

	// テストで使うプラン ID は dbPlanID (10GB プラン) を使う
	planID := types.ID(dbPlanID)
	require.NotZero(t, planID, "database plan ID is not set")

	// パスワードを一時ファイルに書き込む。実行記録にはファイルパスだけが残る
	passwordFile := filepath.Join(t.TempDir(), "db-password.txt")
	require.NoError(t, os.WriteFile(passwordFile, []byte(password), 0o600))

	// データベースを作成する。作成リクエストは起動の完了を待たずに応答を返すため、
	// 作成後に SDK で起動状態を待ち合わせる
	createArgs := []string{
		"iaas-api", "database", "create",
		"--zone", dbZone,
		"--name", name,
		"--description", dbDescription,
		"--tags", "tutorial,managed",
		"--plan-id", planID.String(),
		"--switch-id", createdSwitch.ID.String(),
		"--ip-address", dbIPAddress,
		"--network-mask-len", strconv.Itoa(dbNetworkMaskLen),
		"--database-type", "mariadb",
		"--username", dbUsername,
		"--password", "@" + passwordFile,
	}
	data := h.CallWithTimeout(t, dbCreateTimeout, "データベースを作成する", createArgs...)
	created = parseDatabase(t, data)
	require.NotZero(t, created.ID, "created database ID is missing")

	defer func() {
		// データベースを削除する前に、保持した ID と識別情報を再照合する
		dbOp := iaas.NewDatabaseOp(client)
		current, err := dbOp.Read(t.Context(), dbZone, created.ID)
		if iaas.IsNotFoundError(err) {
			return // 不在なら変更せず終了する
		}
		require.NoError(t, err, "re-verify database %s before cleanup", created.ID)
		require.NotNil(t, current, "re-verified database should not be nil")
		require.Equal(t, created.ID, current.ID, "cleanup target database ID mismatch")
		require.Equal(t, name, current.Name, "cleanup target database name mismatch")
		// 説明はこのシナリオ後半で更新するため、作成直後の説明と更新後の説明のどちらも今回設定した値として扱う
		require.Contains(t, []string{dbDescription, updatedDescription}, current.Description,
			"cleanup target database description mismatch")
		// 接続先と IP 設定を再照合する。保持したスイッチ ID、指定した IP アドレスとマスク長が
		// 現在値と一致しない場合は削除を呼ばない (背景は ADR 0064)
		require.Equal(t, createdSwitch.ID, current.SwitchID, "cleanup target database switch mismatch")
		require.NotEmpty(t, current.IPAddresses, "cleanup target database should have an IP address")
		require.Equal(t, dbIPAddress, current.IPAddresses[0], "cleanup target database IP address mismatch")
		require.Equal(t, dbNetworkMaskLen, current.NetworkMaskLen, "cleanup target database network mask length mismatch")
		// データベースを削除し、名前でも残っていないことを確認する
		_ = h.CallWithTimeout(t, dbDeleteTimeout, "データベースを削除する",
			"iaas-api", "database", "delete",
			"--zone", dbZone, "--id", created.ID.String(), "--force", "--fail-if-not-found")
		remaining := lookupDatabases(t, client, name)
		for _, item := range remaining {
			require.NotEqual(t, created.ID, item.ID, "database %s should not remain after deletion", created.ID)
		}
	}()

	// 作成応答の内容を確認する。データベース API は Tags に RDBMS バージョンのタグ (@MariaDB-x.y など) を
	// 追加するため、要求したタグが含まれることだけを確認する
	require.Equal(t, name, created.Name)
	require.Equal(t, dbDescription, created.Description)
	require.Subset(t, created.Tags, []string{"tutorial", "managed"}, "create should return the requested tags")

	// 作成リクエストは起動の完了を待たないため、SDK で起動状態を待ち合わせる。
	// リソースを変更しない安全確認のため SDK 直呼びとする (背景は ADR 0064)
	waitForDatabaseReady(t, client, created.ID)

	// 作成したデータベースを参照し、接続先と割り当てた IP を確認する
	read := readDatabase(t, h, "作成したデータベースを参照する", created.ID)
	require.Equal(t, name, read.Name)
	require.Equal(t, planID, read.PlanID, "read should return the requested plan")
	require.Equal(t, createdSwitch.ID, read.SwitchID, "read should return the connected switch")
	require.Equal(t, dbIPAddress, read.ipAddress(), "read should return the assigned IP address")
	require.Equal(t, dbNetworkMaskLen, read.NetworkMaskLen, "read should return the assigned network mask length")
	require.NotNil(t, read.CommonSetting, "read should return the common setting")
	require.Equal(t, dbUsername, read.CommonSetting.DefaultUser, "read should return the database username")

	// データベースの設定パラメータ一覧を取得する
	parameters := h.CallWithTimeout(t, dbPowerTimeout, "データベースの設定パラメータ一覧を取得する",
		"iaas-api", "database", "list-parameter", "--zone", dbZone, "--id", created.ID.String())
	var parameterList []map[string]any
	require.NoError(t, json.Unmarshal(parameters, &parameterList), "decode database parameters")
	require.NotNil(t, parameterList, "list-parameter should return a JSON array")

	// 名前の部分一致検索 (--name は1要素の Names に変換される) で作成したデータベースを検索する
	data = h.CallWithTimeout(t, dbPowerTimeout, "名前で部分一致検索する",
		"iaas-api", "database", "find", "--zone", dbZone, "--name", name, "--count", strconv.Itoa(dbFindPageSize))
	var found []databaseItem
	require.NoError(t, json.Unmarshal(data, &found), "decode database find result")
	require.NotEmpty(t, found, "database find by name should return the created database %s", created.ID)
	var matched *databaseItem
	for i := range found {
		if found[i].ID == created.ID {
			matched = &found[i]
		}
	}
	require.NotNil(t, matched, "database find should contain the created database %s", created.ID)

	// モニタリング系のコマンドを実行する。起動直後はデータが存在しないことがあるため、
	// エラーでなく正常な応答が返ることを確認する
	for _, op := range []string{"monitor-database", "monitor-cpu", "monitor-disk", "monitor-interface"} {
		monitorData := h.CallWithTimeout(t, dbPowerTimeout, op+" のモニタリングデータを取得する",
			"iaas-api", "database", op, "--zone", dbZone, "--id", created.ID.String())
		var values []map[string]any
		require.NoError(t, json.Unmarshal(monitorData, &values), "decode %s result", op)
		require.NotNil(t, values, "%s should return a JSON array", op)
	}

	// 説明を更新する。update は送信した内容で設定が置き換わるため、現在の設定を
	// 含めて更新リクエストを組み立てる。read の応答のパスワードはマスク済みで返り、
	// API はマスク済みの値を受け付けないため、作成時に使った値で置き換える
	current := readDatabaseModel(t, h, "更新前の設定を取得する", created.ID)
	if current.CommonSetting != nil {
		current.CommonSetting.UserPassword = password
	}
	update := &iaas.DatabaseUpdateRequest{
		Name:               current.Name,
		Description:        updatedDescription,
		Tags:               current.Tags,
		CommonSetting:      current.CommonSetting,
		BackupSetting:      current.BackupSetting,
		ReplicationSetting: current.ReplicationSetting,
		MonitoringSuite:    current.MonitoringSuite,
		// Backupv2Setting は読み取りモデルが表示用の型のため、このシナリオでは無効のまま送らない
	}
	updateJSON, err := json.Marshal(update)
	require.NoError(t, err, "encode database update request")
	updateFile := filepath.Join(t.TempDir(), "db-update.json")
	require.NoError(t, os.WriteFile(updateFile, updateJSON, 0o600))
	h.CallWithTimeout(t, dbUpdateTimeout, "データベースの説明を更新する",
		"iaas-api", "database", "update", "--zone", dbZone, "--id", created.ID.String(), "--request", "@"+updateFile)

	// 更新した説明を参照で確認する。反映待ちは期限付きで行う
	var updated databaseItem
	require.Eventually(t, func() bool {
		item, err := readDatabaseForRetry(t, h, created.ID)
		if err != nil {
			return false
		}
		updated = item
		return item.Description == updatedDescription
	}, dbUpdateTimeout, 30*time.Second, "update should change the description to %q", updatedDescription)
	require.Equal(t, updatedDescription, updated.Description, "update should change the description")

	// 電源操作を確認する。shutdown は電源 OFF まで、boot は起動完了まで待つ
	h.CallWithTimeout(t, dbPowerTimeout, "データベースをシャットダウンする (電源 OFF まで待つ)",
		"iaas-api", "database", "shutdown", "--zone", dbZone, "--id", created.ID.String())
	h.CallWithTimeout(t, dbPowerTimeout, "データベースを起動する (起動完了まで待つ)",
		"iaas-api", "database", "boot", "--zone", dbZone, "--id", created.ID.String())
	h.CallWithTimeout(t, dbPowerTimeout, "データベースをリセットする",
		"iaas-api", "database", "reset", "--zone", dbZone, "--id", created.ID.String())

	// リセット後も参照できることを確認する
	readAfterReset := readDatabase(t, h, "リセット後にデータベースを参照する", created.ID)
	require.Equal(t, name, readAfterReset.Name)
}

// readDatabase は指定した ID のデータベースを参照して応答を解析します。
func readDatabase(t *testing.T, h *helper.Helper, description string, id types.ID) databaseItem {
	t.Helper()

	data := h.CallWithTimeout(t, dbPowerTimeout, description,
		"iaas-api", "database", "read", "--zone", dbZone, "--id", id.String())
	return parseDatabase(t, data)
}

// readDatabaseForRetry は更新の反映待ちで使う参照です。エラーを値として返します。
func readDatabaseForRetry(t *testing.T, h *helper.Helper, id types.ID) (databaseItem, error) {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "データベースを参照する (更新の反映待ち)",
		"iaas-api", "database", "read", "--zone", dbZone, "--id", id.String())
	if err != nil {
		return databaseItem{}, err
	}
	var item databaseItem
	if err := json.Unmarshal(data, &item); err != nil {
		return databaseItem{}, err
	}
	return item, nil
}

// parseDatabase はデータベースの応答を解析します。ID が空の場合はテストを失敗させます。
func parseDatabase(t *testing.T, data []byte) databaseItem {
	t.Helper()

	var item databaseItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode database: %v", err)
	require.NotZero(t, item.ID, "database ID is missing")
	return item
}

// waitForDatabaseReady は SDK でデータベースの起動状態を待ち合わせます。
// 作成リクエストは起動の完了を待たないため、起動確認が必要なステップの前に呼び出します。
// リソースを変更しない安全確認のため SDK 直呼びとします (背景は ADR 0064)。
func waitForDatabaseReady(t *testing.T, client *iaas.Client, id types.ID) {
	t.Helper()

	op := iaas.NewDatabaseOp(client)
	require.Eventually(t, func() bool {
		db, err := op.Read(t.Context(), dbZone, id)
		if err != nil {
			return false
		}
		return db != nil && db.InstanceStatus == types.ServerInstanceStatuses.Up
	}, dbCreateTimeout, 15*time.Second, "database %s should become up", id)
}

// readDatabaseModel は CLI の read で参照結果を SDK の Database モデルとして取得します。
// update は送信した内容で設定が置き換わるため、現在の設定を含む更新リクエストの
// 組み立てに使います。
func readDatabaseModel(t *testing.T, h *helper.Helper, description string, id types.ID) *iaas.Database {
	t.Helper()

	data := h.CallWithTimeout(t, dbPowerTimeout, description,
		"iaas-api", "database", "read", "--zone", dbZone, "--id", id.String())
	var model iaas.Database
	require.NoError(t, json.Unmarshal(data, &model), "decode database read result")
	require.NotZero(t, model.ID, "database ID is missing")
	return &model
}

// lookupDatabases は SDK で名前でデータベースを検索します。
// 作成前の同名確認と削除後の不在確認に使います。
func lookupDatabases(t *testing.T, client *iaas.Client, name string) []databaseItem {
	t.Helper()

	op := iaas.NewDatabaseOp(client)
	result, err := op.Find(t.Context(), dbZone, &iaas.FindCondition{
		Count:  dbFindPageSize,
		Filter: map[search.FilterKey]any{search.Key("Name"): search.AndEqual(name)},
	})
	require.NoError(t, err, "find databases by name %q", name)
	items := make([]databaseItem, 0, len(result.Databases))
	for _, db := range result.Databases {
		if db == nil {
			continue
		}
		items = append(items, databaseItem{
			ID:       db.ID,
			Name:     db.Name,
			SwitchID: db.SwitchID,
		})
	}
	// 1ページで取りきれない結果は不在の判断に使わない
	require.Less(t, len(items), dbFindPageSize, "database find result page is full; cannot conclude absence of %q", name)
	return items
}
