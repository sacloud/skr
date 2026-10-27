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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// scheduleLeadTime はスケジュールの初回実行を1年後に設定し、実行中に発火しないようにします。
const scheduleLeadTime = time.Hour * 24 * 365

// scheduleItem はスケジュールの読み込み結果とその Settings です。
type scheduleItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Settings    struct {
		ProcessConfigurationID json.RawMessage `json:"ProcessConfigurationID"`
		StartsAt               string          `json:"StartsAt"`
		RecurringStep          int             `json:"RecurringStep"`
		RecurringUnit          string          `json:"RecurringUnit"`
	} `json:"Settings"`
}

func (i scheduleItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// runScheduleScenario はプロセス設定を参照するスケジュールの作成、一覧、参照、更新、削除を実行します。
// スケジュールは1年後に1日周期で開始されるため、実行中に発火しません。削除後に不在を確認します。
func runScheduleScenario(t *testing.T, h *helper.Helper, schedule *eventbusResource, configID string) {
	t.Helper()

	startsAt := time.Now().Add(scheduleLeadTime).UnixMilli()
	schedule.attempted = true
	createRequest := h.WriteRequestJSONFile(t, "create-schedule", map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        schedule.name,
			"Description": schedule.description,
			"Settings": map[string]any{
				"ProcessConfigurationID": configID,
				"StartsAt":               startsAt,
				"RecurringStep":          1,
				"RecurringUnit":          "day",
			},
		},
	})
	created := parseEventbusItem(t, h.Call(t, "スケジュールを作成する",
		"eventbus-api", "schedule", "create", "--request", "@"+createRequest))
	id := requireEventbusIdentity(t, created, schedule)
	schedule.id = id

	// 作成したスケジュールが一覧に含まれることを確認する
	matches := findEventbusItemsByName(t, h, "作成したスケジュールが一覧に含まれることを確認する", schedule)
	require.Len(t, matches, 1, "schedule %q should be listed once after create", schedule.name)
	foundID, err := matches[0].id()
	require.NoError(t, err, "decode listed schedule ID: %v", err)
	require.Equal(t, id, foundID, "the listed schedule ID should match the created one")

	// スケジュールを参照する
	read := readTypedItem[scheduleItem](t, h, "スケジュールを参照する", "schedule", id)
	require.NoError(t, checkSchedule(read, schedule, id, configID, startsAt, schedule.description),
		"created schedule check failed")

	// 説明を更新して反映を確認する
	updatedDescription := schedule.description + " (updated)"
	updateScheduleDescription(t, h, "スケジュールの説明を更新する", id, updatedDescription)
	updated := readTypedItem[scheduleItem](t, h, "更新後のスケジュールを参照する", "schedule", id)
	require.NoError(t, checkSchedule(updated, schedule, id, configID, startsAt, updatedDescription),
		"updated schedule check failed")

	// 説明を元に戻して反映を確認する
	updateScheduleDescription(t, h, "スケジュールの説明を元に戻す", id, schedule.description)
	restored := readTypedItem[scheduleItem](t, h, "元に戻ったスケジュールを参照する", "schedule", id)
	require.NoError(t, checkSchedule(restored, schedule, id, configID, startsAt, schedule.description),
		"restored schedule check failed")

	// スケジュールを削除する
	h.Call(t, "スケジュールを削除する", "eventbus-api", "schedule", "delete", id)

	// 削除後に一覧に残っていないことを確認する
	remaining := listEventbusItems(t, h, "削除後にスケジュールが一覧に残っていないことを確認する", "schedule")
	if len(itemsNamed(remaining, schedule.name)) != 0 {
		t.Errorf("schedule %q remains after delete", schedule.name)
	}
	schedule.deleted = true
}

// updateScheduleDescription はスケジュールの説明を更新します。
func updateScheduleDescription(t *testing.T, h *helper.Helper, description, id, newDescription string) {
	t.Helper()

	request := h.WriteRequestJSONFile(t, "update-schedule-description", map[string]any{
		"CommonServiceItem": map[string]any{
			"Description": newDescription,
		},
	})
	h.Call(t, description, "eventbus-api", "schedule", "update", id, "--request", "@"+request)
}

// checkSchedule はスケジュールの識別情報と設定が、
// 今回の実行で作成した内容と一致することを確認します。
func checkSchedule(read scheduleItem, r *eventbusResource, id, configID string, startsAt int64, wantDescription string) error {
	readID, err := read.id()
	if err != nil {
		return err
	}
	if readID != id || read.Name != r.name || read.Description != wantDescription {
		return fmt.Errorf("schedule identity mismatch for ID %s", id)
	}
	configReference, err := decodeResourceID(read.Settings.ProcessConfigurationID)
	if err != nil || configReference != configID {
		return fmt.Errorf("schedule ProcessConfigurationID mismatch for ID %s", id)
	}
	// API はリクエストで整数を受け入れますが、レスポンスの StartsAt は10進文字列です。
	wantStartsAt := fmt.Sprintf("%d", startsAt)
	if read.Settings.StartsAt != wantStartsAt {
		return fmt.Errorf("schedule StartsAt mismatch for ID %s: got %q, want %q", id, read.Settings.StartsAt, wantStartsAt)
	}
	if read.Settings.RecurringStep != 1 || read.Settings.RecurringUnit != "day" {
		return fmt.Errorf("schedule recurrence mismatch for ID %s: got %d %s", id, read.Settings.RecurringStep, read.Settings.RecurringUnit)
	}
	return nil
}

// TestCheckSchedule はスケジュールの識別情報と設定の照合を確認する。
func TestCheckSchedule(t *testing.T) {
	r := &eventbusResource{
		kind:        "schedule",
		name:        "test-only-name",
		description: "test-only-description",
		id:          "test-only-id",
	}
	configID := "test-only-config-id"
	startsAt := time.Now().UnixMilli()
	ok := fmt.Sprintf(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Settings":{"ProcessConfigurationID":%q,"StartsAt":"%d","RecurringStep":1,"RecurringUnit":"day"}}`,
		configID, startsAt)

	t.Run("ok", func(t *testing.T) {
		var item scheduleItem
		require.NoError(t, json.Unmarshal([]byte(ok), &item))
		require.NoError(t, checkSchedule(item, r, r.id, configID, startsAt, r.description))
	})
	t.Run("description mismatch", func(t *testing.T) {
		var item scheduleItem
		require.NoError(t, json.Unmarshal([]byte(ok), &item))
		require.Error(t, checkSchedule(item, r, r.id, configID, startsAt, "other-description"))
	})
	t.Run("process configuration mismatch", func(t *testing.T) {
		var item scheduleItem
		data := strings.Replace(ok, configID, "other-config-id", 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkSchedule(item, r, r.id, configID, startsAt, r.description))
	})
	t.Run("starts at mismatch", func(t *testing.T) {
		var item scheduleItem
		require.NoError(t, json.Unmarshal([]byte(ok), &item))
		require.Error(t, checkSchedule(item, r, r.id, configID, startsAt+1, r.description))
	})
	t.Run("recurrence mismatch", func(t *testing.T) {
		var item scheduleItem
		data := strings.Replace(ok, `"RecurringUnit":"day"`, `"RecurringUnit":"hour"`, 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkSchedule(item, r, r.id, configID, startsAt, r.description))
	})
}
