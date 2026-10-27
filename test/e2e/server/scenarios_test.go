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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// TestScenarioCreatesBootsSSHAndCleansServerAndDisk はシナリオが
// 作成前の同名確認、作成、起動、SSH 接続確認、更新、後片付けの順に実行されることを確認する。
func TestScenarioCreatesBootsSSHAndCleansServerAndDisk(t *testing.T) {
	fake := &fakeServerCommands{}
	require.NoError(t, runServerScenario(t, fake, noSleep, testServerRun()))

	require.Nil(t, fake.server, "the Server should be deleted after the scenario")
	require.False(t, fake.disk, "the Disk should be deleted after the scenario")
	require.Equal(t, []int64{123}, toInts(fake.deleted), "the Server should be deleted once")

	// 同名確認 → 作成 → 起動 → SSH 接続確認 → 更新 → 後片付けの順に実行される
	require.Greater(t, callIndex(fake.calls, "findServers"), -1, "the preflight should find existing Servers")
	require.Greater(t, callIndex(fake.calls, "createServer"), callIndex(fake.calls, "findServers"), "the create should follow the preflight")
	require.Greater(t, callIndex(fake.calls, "bootServer"), callIndex(fake.calls, "createServer"), "the boot should follow the create")
	require.Greater(t, callIndex(fake.calls, "connectSSH"), callIndex(fake.calls, "bootServer"), "the SSH login check should follow the boot")
	require.Greater(t, callIndex(fake.calls, "updateServer"), callIndex(fake.calls, "connectSSH"), "the update should follow the SSH login check")
	require.Greater(t, callIndex(fake.calls, "deleteServer"), callIndex(fake.calls, "updateServer"), "the cleanup should follow the update")
}

// TestScenarioRefusesExistingServer は同名リソースが既にある場合に作成しないことを確認する。
func TestScenarioRefusesExistingServer(t *testing.T) {
	fake := &fakeServerCommands{}
	fake.server = &serverItem{ID: 456, Name: namePrefix + "test", Description: "unrelated Server"}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "already exists", "the name collision should be refused")
	// 既存リソースは変更せず、作成も削除もしない
	require.Equal(t, "unrelated Server", fake.server.Description, "the existing Server should not be modified")
	require.Equal(t, []string{"findServers"}, fake.calls, "only the preflight server find should run")
	require.Empty(t, fake.deleted, "no Server should be deleted")
}

// TestScenarioRefusesCleanupAfterIdentityChange は後片付けの再照合で識別情報が
// 変わっていた場合に削除しないことを確認する。
func TestScenarioRefusesCleanupAfterIdentityChange(t *testing.T) {
	fake := &fakeServerCommands{changeIdentityAtCleanup: true}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "cleanup refused", "the identity change should be refused")
	require.NotNil(t, fake.server, "the Server with changed identity should not be deleted")
	require.Empty(t, fake.deleted, "no Server should be deleted")
}

// TestScenarioTimesOutWhenSSHLoginFailsAndCleansUp は SSH ログイン確認が期限を過ぎた場合に
// エラーにし、後片付けまで行うことを確認する。
func TestScenarioTimesOutWhenSSHLoginFailsAndCleansUp(t *testing.T) {
	fake := &fakeServerCommands{sshErr: errors.New("SSH unavailable")}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "SSH login check timed out", "the SSH timeout should be reported")
	require.Nil(t, fake.server, "the Server should be cleaned up after the SSH timeout")
	require.False(t, fake.disk, "the Disk should be cleaned up after the SSH timeout")
	require.Equal(t, []int64{123}, toInts(fake.deleted), "the Server should be deleted after the SSH timeout")
}

// TestScenarioKeepsServerWhenRestoreFails は復元に失敗した場合に、後片付けが
// Server を識別できず削除しないことを確認する。
func TestScenarioKeepsServerWhenRestoreFails(t *testing.T) {
	fake := &fakeServerCommands{failRestore: true}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "restore Server Name and Description", "the restore failure should be reported")
	require.ErrorContains(t, err, "cleanup refused", "the cleanup should refuse a Server whose name was not restored")
	require.NotNil(t, fake.server, "the Server should not be deleted when the restore failed")
	require.Empty(t, fake.deleted, "no Server should be deleted")
}

// TestFindReadsAllPages は一括検索が全ページを取得することを確認する。
func TestFindReadsAllPages(t *testing.T) {
	calls := 0
	items, err := paginateFind(t, "ディスクを検索する", func(from int) ([]diskItem, error) {
		calls++
		if from != (calls-1)*findPageSize {
			return nil, fmt.Errorf("unexpected from %d", from)
		}
		if calls == 2 {
			return nil, nil
		}
		return make([]diskItem, findPageSize), nil
	})

	require.NoError(t, err)
	require.Len(t, items, findPageSize, "the first page should be collected")
	require.Equal(t, 2, calls, "the second page should be requested")
}

// TestFindDoesNotTreatPageFailureAsMissing は途中ページの取得に失敗した場合に
// 不在として扱わないことを確認する。
func TestFindDoesNotTreatPageFailureAsMissing(t *testing.T) {
	calls := 0
	_, err := paginateFind(t, "ディスクを検索する", func(int) ([]diskItem, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("injected CLI failure")
		}
		return make([]diskItem, findPageSize), nil
	})

	require.ErrorContains(t, err, "injected CLI failure", "a page failure should not be treated as no results")
}

// TestFindPageLabelAddsPageInfo は2ページ目以降の検索にページ位置を示す説明が
// 追加されることを確認する。
func TestFindPageLabelAddsPageInfo(t *testing.T) {
	require.Equal(t, "ディスクを検索する", findPageLabel("ディスクを検索する", 0), "the first page should use the original label")
	require.Equal(t, "ディスクを検索する（101 件目以降）", findPageLabel("ディスクを検索する", 100), "the second page should note the position")
}

// toInts は ID の記録を検証しやすい int64 のスライスに変換します。
func toInts(ids []types.ID) []int64 {
	values := make([]int64, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.Int64())
	}
	return values
}
