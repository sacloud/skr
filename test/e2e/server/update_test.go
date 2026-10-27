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
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// updatedServerDescription は更新確認中に使用する一時的な説明です。
const updatedServerDescription = serverDescription + " (updated)"

// updatedServerName は更新確認中に使用する一時的な名前です。
func updatedServerName(name string) string {
	return name + "-upd"
}

// updateAndVerifyServer は Server の名前と説明を更新し、参照で反映を確認してから
// 元の値に戻し、復元を確認します。復元により後片付けが Server を再び識別できるようになります。
func updateAndVerifyServer(t *testing.T, c serverCommands, id types.ID, name string) error {
	t.Helper()

	if err := c.updateServer(t, "サーバーの名前と説明を更新する", id, updatedServerName(name), updatedServerDescription); err != nil {
		return err
	}
	item, err := c.readServer(t, "更新後のサーバーを参照する", id)
	if err != nil {
		return fmt.Errorf("read updated Server: %w", err)
	}
	if err := checkUpdatedServer(item, id, name); err != nil {
		return err
	}
	if err := c.updateServer(t, "サーバーの名前と説明を元に戻す", id, name, serverDescription); err != nil {
		return fmt.Errorf("restore Server Name and Description: %w", err)
	}
	item, err = c.readServer(t, "元に戻ったサーバーを参照する", id)
	if err != nil {
		return fmt.Errorf("read restored Server: %w", err)
	}
	return checkServer(item, id, name)
}

// TestUpdateAndVerifyUpdatesRestoresAndReads は更新と復元が参照を挟んで実行されることを確認する。
func TestUpdateAndVerifyUpdatesRestoresAndReads(t *testing.T) {
	fake := &fakeServerCommands{}
	fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: serverDescription}

	run := testServerRun()
	require.NoError(t, updateAndVerifyServer(t, fake, 123, run.name))

	// 更新 → 参照 → 復元 → 参照の順に実行される
	want := []string{"updateServer", "readServer", "updateServer", "readServer"}
	require.Equal(t, want, fake.calls)
	require.Equal(t, run.name, fake.server.Name, "the Server Name should be restored")
	require.Equal(t, serverDescription, fake.server.Description, "the Server Description should be restored")
}

// TestUpdateAndVerifyReportsFailures は更新と復元の失敗が検出されることを確認する。
func TestUpdateAndVerifyReportsFailures(t *testing.T) {
	t.Run("update api error", func(t *testing.T) {
		fake := &fakeServerCommands{failUpdate: true}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: serverDescription}

		err := updateAndVerifyServer(t, fake, 123, namePrefix+"test")
		require.ErrorContains(t, err, "injected CLI failure", "the update API failure should be reported")
	})

	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeServerCommands{ignoreUpdate: true}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: serverDescription}

		err := updateAndVerifyServer(t, fake, 123, namePrefix+"test")
		require.ErrorContains(t, err, "updated Server identity mismatch", "the update reflection should be verified by Read")
	})

	t.Run("restore api error", func(t *testing.T) {
		fake := &fakeServerCommands{failRestore: true}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: serverDescription}

		err := updateAndVerifyServer(t, fake, 123, namePrefix+"test")
		require.ErrorContains(t, err, "restore Server Name and Description", "the restore failure should be reported")
	})

	t.Run("restore ignored", func(t *testing.T) {
		fake := &fakeServerCommands{ignoreRestore: true}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: serverDescription}

		err := updateAndVerifyServer(t, fake, 123, namePrefix+"test")
		require.ErrorContains(t, err, "identity mismatch", "the restore reflection should be verified by Read")
	})
}
