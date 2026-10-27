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
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// groupNamePrefix は通知シナリオが作成するグループ名の接頭辞です。
	groupNamePrefix = "skr-e2e-notify-"
	// groupDescriptionPrefix は通知シナリオが作成するグループ説明の接頭辞です。
	groupDescriptionPrefix = "skr-e2e-simple-notification/"
	// groupListQuery はグループの一覧を射影する --query です。
	groupListQuery = ".CommonServiceItems | map({ID,Name,Description,Settings})"
	// historyListQuery は通知履歴の一覧を射影する --query です。
	historyListQuery = ".NotificationHistories | map({request_id,message:.message.body,statuses:[.statuses[] | {group_id,destination_id,status}]})"
	// historyReadQuery は個別の通知履歴を射影する --query です。
	historyReadQuery = ".NotificationHistory | {request_id,message:.message.body,statuses:[.statuses[] | {group_id,destination_id,status}]}"
	// historyWaitLimit は通知処理の完了を確認する期限です。
	historyWaitLimit = 2 * time.Minute
	// historyPolling は通知履歴を再取得する間隔です。
	historyPolling = 5 * time.Second
	// commandTimeout は1回の CLI 呼び出しのタイムアウトです。
	commandTimeout = 60 * time.Second
)

// groupItem は --query で射影した通知先グループです。
type groupItem struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Settings    struct {
		Destinations []string `json:"Destinations"`
		Disabled     bool     `json:"Disabled"`
	} `json:"Settings"`
}

// groupResponse は CLI のグループ参照・作成応答です。
type groupResponse struct {
	CommonServiceItem groupItem `json:"CommonServiceItem"`
}

// notificationHistory は --query で射影した通知履歴です。
type notificationHistory struct {
	RequestID string               `json:"request_id"`
	Message   string               `json:"message"`
	Statuses  []notificationStatus `json:"statuses"`
}

// notificationStatus は履歴の通知先ごとの処理状態です。
type notificationStatus struct {
	GroupID       string `json:"group_id"`
	DestinationID string `json:"destination_id"`
	Status        int    `json:"status"`
}

// TestScenarioSimpleNotification は既存通知先へテスト通知を送信し、グループの作成・参照、
// 通知履歴の確認、削除と不在確認まで行います。既存通知先は変更・削除しません。
func TestScenarioSimpleNotification(t *testing.T) {
	h := helper.New(t, "simple-notification-api",
		"既存通知先へテスト通知を送信し、グループの作成・参照、通知履歴の確認と後片付けが正しく動作することを確認する")

	destinationID := destinationIDFromEnv(t)
	suffix := helper.RandomSuffix(t)
	name := groupNamePrefix + suffix
	description := groupDescriptionPrefix + suffix
	message := "skr E2E test notification " + suffix

	// 既存通知先の送信可否を確認する（既存通知先は変更・削除しない）
	data := h.Call(t, "既存通知先の送信可否を確認する",
		"simple-notification-api", "destination", "read-status", destinationID)
	var status struct {
		NotificationStatus struct {
			IsValid bool `json:"IsValid"`
		} `json:"NotificationStatus"`
	}
	err := json.Unmarshal(data, &status)
	require.NoError(t, err, "decode destination status: %v", err)
	require.True(t, status.NotificationStatus.IsValid,
		"notification destination %s is not valid for sending", destinationID)

	// 作成前に同名グループが存在しないことを確認する
	for _, item := range listGroups(t, h, "作成前に同名グループが存在しないことを確認する") {
		require.NotEqual(t, name, item.Name, "group name %q already exists; refusing to modify it", name)
	}

	// 一時グループを作成する
	requestPath := h.WriteRequestJSONFile(t, "create-group", map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        name,
			"Description": description,
			"Settings": map[string]any{
				"Destinations": []string{destinationID},
				"Disabled":     false,
			},
		},
	})
	created := parseGroupResponse(t, h.Call(t, "一時グループを作成する",
		"simple-notification-api", "group", "create", "--request", "@"+requestPath), "一時グループを作成する")
	require.NotEmpty(t, created.ID, "create response has no group ID")
	require.True(t, sameGroup(created, created.ID, name, description, destinationID),
		"create response does not match the requested group identity and destination")
	id := created.ID

	// 後片付け: グループを削除し、一覧に残っていないことを確認する
	defer func() {
		_ = h.Call(t, "一時グループを削除する", "simple-notification-api", "group", "delete", id)
		remaining := listGroups(t, h, "削除後にグループが一覧に残っていないことを確認する")
		for _, item := range remaining {
			require.NotEqual(t, id, item.ID, "E2E group %s remains after delete", id)
			require.NotEqual(t, name, item.Name, "E2E group %s remains after delete", id)
		}
	}()

	// 作成したグループが一覧に1件だけ含まれることを確認する
	listed := listGroups(t, h, "作成したグループが一覧に含まれることを確認する")
	var found []*groupItem
	for i := range listed {
		if listed[i].Name == name {
			found = append(found, &listed[i])
		}
	}
	require.Len(t, found, 1, "group list returned %d groups named %q; want one", len(found), name)
	require.True(t, sameGroup(*found[0], id, name, description, destinationID),
		"group list does not match the created group")

	// 作成したグループを参照する
	current := parseGroupResponse(t, h.Call(t, "作成したグループを参照する",
		"simple-notification-api", "group", "read", id), "作成したグループを参照する")
	require.True(t, sameGroup(current, id, name, description, destinationID),
		"group read does not match the created group")

	// テストメッセージを送信する
	data = h.Call(t, "テストメッセージを送信する",
		"simple-notification-api", "group", "send-message", id, "--message", message)
	var send struct {
		IsOK bool `json:"is_ok"`
	}
	err = json.Unmarshal(data, &send)
	require.NoError(t, err, "decode send response: %v", err)
	require.True(t, send.IsOK, "notification API did not accept the test message")

	// 通知履歴で処理完了を確認する
	expected := waitForNotificationHistory(t, h, id, destinationID, message)

	// 履歴を個別に参照する
	verifyNotificationHistory(t, h, expected)
}

// listGroups は射影済みのグループ一覧を取得します。
func listGroups(t *testing.T, h *helper.Helper, description string) []groupItem {
	t.Helper()

	data := h.Call(t, description, "simple-notification-api", "group", "list", "--query", groupListQuery)
	var items []groupItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode projected group list: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON group array", description)
	return items
}

// parseGroupResponse はグループの参照・作成応答をデコードします。
func parseGroupResponse(t *testing.T, data []byte, description string) groupItem {
	t.Helper()

	var response groupResponse
	err := json.Unmarshal(data, &response)
	require.NoError(t, err, "%s: decode group response: %v", description, err)
	return response.CommonServiceItem
}

// sameGroup はグループが今回の実行が作成した内容と一致するかを判定します。
func sameGroup(item groupItem, id, name, description, destinationID string) bool {
	return item.ID == id &&
		item.Name == name &&
		item.Description == description &&
		len(item.Settings.Destinations) == 1 &&
		item.Settings.Destinations[0] == destinationID &&
		!item.Settings.Disabled
}

// waitForNotificationHistory は通知履歴に今回のメッセージが処理完了（状態 2）で現れるまで期限付きで待ちます。
func waitForNotificationHistory(t *testing.T, h *helper.Helper, groupID, destinationID, message string) notificationHistory {
	t.Helper()

	deadline := time.Now().Add(historyWaitLimit)
	for attempt := 1; ; attempt++ {
		data := h.CallWithTimeout(t, commandTimeout,
			fmt.Sprintf("通知履歴を確認する（%d 回目）", attempt),
			"simple-notification-api", "history", "list", "--query", historyListQuery)
		var histories []notificationHistory
		err := json.Unmarshal(data, &histories)
		require.NoError(t, err, "decode notification history: %v", err)
		require.NotNil(t, histories, "notification history query did not return a JSON array")

		var matching *notificationHistory
		for i := range histories {
			if histories[i].Message != message {
				continue
			}
			require.Nil(t, matching, "notification history contains multiple entries for the unique E2E message")
			matching = &histories[i]
		}
		if matching != nil {
			require.NotEmpty(t, matching.RequestID, "matching notification history has no request ID")
			foundStatus := false
			for _, st := range matching.Statuses {
				if st.GroupID != groupID {
					continue
				}
				require.Equal(t, destinationID, st.DestinationID,
					"notification history destination does not match the supplied destination")
				foundStatus = true
				if st.Status == 2 {
					return *matching
				}
				require.Contains(t, []int{0, 1}, st.Status,
					"notification processing ended with status %d", st.Status)
			}
			require.True(t, foundStatus, "matching notification history has no status for the created group")
		}
		if time.Now().Add(historyPolling).After(deadline) {
			t.Fatalf("timed out after %s waiting for notification processing", historyWaitLimit)
		}
		h.Sleep(t, "通知処理の完了を待つ", historyPolling)
	}
}

// verifyNotificationHistory は個別取得した履歴が一覧の内容と一致することを確認します。
func verifyNotificationHistory(t *testing.T, h *helper.Helper, expected notificationHistory) {
	t.Helper()

	data := h.Call(t, "通知履歴を個別に参照する", "simple-notification-api", "history", "read", expected.RequestID,
		"--query", historyReadQuery)
	var actual notificationHistory
	err := json.Unmarshal(data, &actual)
	require.NoError(t, err, "decode individual notification history: %v", err)
	require.Equal(t, expected.RequestID, actual.RequestID, "individual notification history request ID mismatch")
	require.Equal(t, expected.Message, actual.Message, "individual notification history message mismatch")
	require.Len(t, expected.Statuses, 1, "list entry has no single status")
	require.Len(t, actual.Statuses, 1, "individual notification history has no single status")
	require.Equal(t, expected.Statuses[0], actual.Statuses[0],
		"individual notification history does not match the list entry")
}
