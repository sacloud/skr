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

// cleanupServerResources は後片付けです。作成に成功した Server の ID を優先して再照合し、
// ID がない場合は名前検索で回収を試みます。識別できない場合は削除しません。
// 削除後は同名リソースの不在を確認し、孤立した Disk の回収まで行います。
func cleanupServerResources(t *testing.T, c serverCommands, run *serverRun) error {
	t.Helper()

	id := run.serverID
	if id.IsEmpty() {
		matches, err := c.findServers(t, "後片付け前に同名サーバーを検索する", run.name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Server: %w", err)
		}
		if len(matches) == 0 {
			return cleanupOrphanDisks(t, c, run, nil)
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != serverDescription {
			return errors.New("cannot safely identify created Server; manual cleanup required")
		}
		id = matches[0].ID
	}
	current, err := c.readServer(t, "削除前にサーバーを再照合する", id)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	if err := checkServer(current, id, run.name); err != nil {
		return fmt.Errorf("cleanup refused: Server identity changed; manual cleanup required: %w", err)
	}
	deleteErr := c.deleteServer(t, "サーバーをディスクとともに削除する", id)
	matches, err := c.findServers(t, "削除後にサーバーが一覧に残っていないことを確認する", run.name)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	if len(matches) != 0 {
		return errors.Join(deleteErr, fmt.Errorf("Server %q remains after cleanup; manual cleanup required", run.name))
	}
	return errors.Join(deleteErr, cleanupOrphanDisks(t, c, run, current.Disks))
}

// cleanupOrphanDisks は Server 削除後に残った同名 Disk を回収します。
// 既知の Disk と一致する場合のみ削除し、削除後は不在を確認します。
// 識別できない Disk は変更せず、手動での回収が必要なことを報告します。
func cleanupOrphanDisks(t *testing.T, c serverCommands, run *serverRun, knownDisks []diskItem) error {
	t.Helper()

	matches, err := c.findDisks(t, "後片付け前に同名ディスクを検索する", run.name)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 || len(knownDisks) != 1 || matches[0].ID.IsEmpty() ||
		matches[0].ID != knownDisks[0].ID || matches[0].Name != run.name {
		return errors.New("cannot safely identify orphan Disk; manual cleanup required")
	}
	id := matches[0].ID
	current, err := c.readDisk(t, "削除前に孤立ディスクを再照合する", id)
	if err != nil {
		return fmt.Errorf("cannot verify orphan Disk: %w", err)
	}
	if !matchesCreatedDisk(current, id, run.name, run.params) {
		return errors.New("orphan Disk identity or attachment changed; manual cleanup required")
	}
	if err := c.deleteDisk(t, "孤立ディスクを削除する", id); err != nil {
		return fmt.Errorf("delete orphan Disk %s: %w", id, err)
	}
	matches, err = c.findDisks(t, "削除後にディスクが一覧に残っていないことを確認する", run.name)
	if err != nil {
		return err
	}
	if len(matches) != 0 {
		return errors.New("Disk remains after cleanup; manual cleanup required")
	}
	return nil
}

// TestScenarioDeletesVerifiedOrphanDisk は Server 削除後に残った Disk が
// 検証つきで回収されることを確認する。
func TestScenarioDeletesVerifiedOrphanDisk(t *testing.T) {
	fake := &fakeServerCommands{leaveDisk: true}
	require.NoError(t, runServerScenario(t, fake, noSleep, testServerRun()))

	require.False(t, fake.disk, "the orphan Disk should be deleted")
	require.Equal(t, []types.ID{45}, fake.deletedDisks, "the verified orphan Disk should be deleted once")
	// 孤立ディスクは検証つきで削除され、削除後の不在確認まで行われる
	require.Greater(t, callIndex(fake.calls, "readDisk"), callIndex(fake.calls, "deleteServer"), "the orphan Disk should be verified before deletion")
	require.Greater(t, callIndex(fake.calls, "deleteDisk"), callIndex(fake.calls, "readDisk"), "the orphan Disk should be deleted after verification")
}

// TestScenarioRefusesUnverifiedOrphanDisk は識別できない孤立ディスクを削除しないことを確認する。
func TestScenarioRefusesUnverifiedOrphanDisk(t *testing.T) {
	for name, disk := range map[string]diskItem{
		"other ID":      {ID: 46, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		"other name":    {ID: 45, Name: "different", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		"attached":      {ID: 45, Name: namePrefix + "test", ServerID: 999, DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 20 * 1024},
		"other plan":    {ID: 45, Name: namePrefix + "test", DiskPlanID: 5, SourceArchiveID: 234, SizeMB: 20 * 1024},
		"other archive": {ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 235, SizeMB: 20 * 1024},
		"other size":    {ID: 45, Name: namePrefix + "test", DiskPlanID: 4, SourceArchiveID: 234, SizeMB: 100 * 1024},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeServerCommands{leaveDisk: true, orphanDisk: &disk}
			err := runServerScenario(t, fake, noSleep, testServerRun())
			require.ErrorContains(t, err, "manual cleanup required", "unverified orphan Disks should be refused")
			require.True(t, fake.disk, "the unverified orphan Disk should not be deleted")
			require.Empty(t, fake.deletedDisks, "no Disk should be deleted")
		})
	}
}

// TestScenarioReportsDiskWhenPartialCreateHasNoServer は Server なしで Disk だけが作成された場合に、
// 識別できない Disk が報告されることを確認する。
func TestScenarioReportsDiskWhenPartialCreateHasNoServer(t *testing.T) {
	fake := &fakeServerCommands{createDiskOnly: true}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "cannot safely identify orphan Disk", "the unidentified Disk should be reported")
	require.True(t, fake.disk, "the Disk should not be deleted")
	require.Empty(t, fake.deletedDisks, "no Disk should be deleted")
}

// TestScenarioReportsOrphanCleanupFailures は孤立ディスクの回収の失敗が報告されることを確認する。
func TestScenarioReportsOrphanCleanupFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		failMethod        string
		failFindDisksCall int
	}{
		// 後片付けの Disk 検索は全体で2回目 (削除後の不在確認) と3回目 (孤立ディスク削除後) です。
		"findDisks after server delete": {failFindDisksCall: 2},
		"findDisks after disk delete":   {failFindDisksCall: 3},
		"readDisk":                      {failMethod: "readDisk"},
		"deleteDisk":                    {failMethod: "deleteDisk"},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeServerCommands{leaveDisk: true, failMethod: tc.failMethod, failFindDisksCall: tc.failFindDisksCall}
			err := runServerScenario(t, fake, noSleep, testServerRun())
			require.ErrorContains(t, err, "injected CLI failure", "the orphan cleanup failure should be reported")
		})
	}
}

// TestScenarioRejectsAmbiguousOrRemainingOrphans は曖昧な Disk や削除後も残る Disk を報告することを確認する。
func TestScenarioRejectsAmbiguousOrRemainingOrphans(t *testing.T) {
	t.Run("multiple orphans", func(t *testing.T) {
		fake := &fakeServerCommands{leaveDisk: true, multipleOrphans: true}
		err := runServerScenario(t, fake, noSleep, testServerRun())
		require.ErrorContains(t, err, "manual cleanup required", "ambiguous orphans should be reported")
		require.True(t, fake.disk, "the Disk should not be deleted")
		require.Empty(t, fake.deletedDisks, "no Disk should be deleted")
	})

	t.Run("disk remains after cleanup", func(t *testing.T) {
		fake := &fakeServerCommands{leaveDisk: true, keepOrphan: true}
		err := runServerScenario(t, fake, noSleep, testServerRun())
		require.ErrorContains(t, err, "manual cleanup required", "a remaining Disk should be reported")
		require.True(t, fake.disk, "the Disk should not be deleted")
		require.Equal(t, []types.ID{45}, fake.deletedDisks, "the Disk should be deleted once")
	})
}

// TestScenarioCleansDiskAfterPartialServerDelete は Server の削除 API が失敗しても、
// 削除された Disk の回収と不在確認が行われることを確認する。
func TestScenarioCleansDiskAfterPartialServerDelete(t *testing.T) {
	fake := &fakeServerCommands{leaveDisk: true, deleteServerError: true}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "injected partial delete failure", "the partial delete failure should be reported")
	require.False(t, fake.disk, "the Disk should be deleted after the partial server delete")
	require.Equal(t, []types.ID{45}, fake.deletedDisks, "the orphan Disk should be deleted once")
}

// TestScenarioCleansUpAfterCreateReturnsError は作成 API が失敗しても、
// 作成済みのリソースが後片付けで回収されることを確認する。
func TestScenarioCleansUpAfterCreateReturnsError(t *testing.T) {
	fake := &fakeServerCommands{failCreateAfterSideEffect: true}
	err := runServerScenario(t, fake, noSleep, testServerRun())

	require.ErrorContains(t, err, "injected CLI failure", "the create failure should be reported")
	require.Nil(t, fake.server, "the partially created Server should be cleaned up")
	require.False(t, fake.disk, "the partially created Disk should be cleaned up")
	require.Equal(t, []types.ID{123}, fake.deleted, "the Server should be recovered by name and deleted once")
}
