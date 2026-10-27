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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/message"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/queue"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// queueNamePrefix は今回の実行が作成するキュー名の接頭辞です。
	queueNamePrefix = "skr-e2e-sqm-"
	// queueDescriptionPrefix は今回の実行が設定するキュー説明の接頭辞です。
	queueDescriptionPrefix = "skr-e2e-simplemq/"
	// messageBody は送受信を確認するメッセージ本文です。
	messageBody = "HelloSimpleMQ"
	// visibilityTimeoutSeconds と expireSeconds は config で設定する値です。
	visibilityTimeoutSeconds = 30
	expireSeconds            = 345600
)

func TestScenarioSimpleMQ(t *testing.T) {
	h := helper.New(t, "simplemq-api",
		"SimpleMQ キューの作成・参照・設定更新・削除、API キー ローテーションとメッセージの送受信が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := queueNamePrefix + suffix
	description := queueDescriptionPrefix + suffix

	// 作成前に同名キューが存在しないことを確認する
	before := listQueues(t, h, "作成前に同名キューが存在しないことを確認する")
	for _, item := range before {
		require.NotEqual(t, name, item.Name, "queue name %q already exists; refusing to modify it", name)
	}

	// キューを作成する
	data := h.Call(t, "SimpleMQ キューを作成する",
		"simplemq-api", "queue", "create", "--name", name, "--description", description)
	created := parseQueue(t, data)
	id := queueIDString(t, created)
	require.Equal(t, name, created.Name, "created queue name should match the requested name")
	require.Equal(t, description, created.Description.Value, "created queue description should match the requested description")
	require.Equal(t, name, created.Status.QueueName, "created queue status name should match the requested name")

	// 後片付け: キュー内のメッセージを削除してからキューを削除し、一覧に残っていないことを確認する
	defer func() {
		_ = h.Call(t, "キュー内のメッセージをすべて削除する", "simplemq-api", "queue", "clear-messages", id)
		_ = h.Call(t, "作成したキューを削除する", "simplemq-api", "queue", "delete", id)
		remaining := listQueues(t, h, "削除後にキューが一覧に残っていないことを確認する")
		for _, item := range remaining {
			require.NotEqual(t, name, item.Name, "queue %q should not remain after deletion", name)
		}
	}()

	// 作成したキューを参照する
	data = h.Call(t, "作成したキューを参照する", "simplemq-api", "queue", "read", id)
	requireQueueIdentity(t, parseQueue(t, data), created.ID, name, description)

	// 作成したキューが一覧に含まれることを確認する
	listed := listQueues(t, h, "作成したキューが一覧に含まれることを確認する")
	matches := queuesNamed(listed, name)
	require.Len(t, matches, 1, "queue list should contain exactly one match for %q", name)
	requireQueueIdentity(t, *matches[0], created.ID, name, description)

	// --query で一覧を射影して取得する
	data = h.Call(t, "キュー一覧を射影して取得する",
		"simplemq-api", "queue", "list", "--query", "map({ID,Name})")
	var projected []queueSummary
	err := json.Unmarshal(data, &projected)
	require.NoError(t, err, "decode projected queue list: %v", err)
	require.NotNil(t, projected, "projected queue list should be a JSON array")
	projectedMatches := 0
	for _, item := range projected {
		if item.Name == name {
			require.Equal(t, created.ID, item.ID, "projected queue %q should be the queue created by this run", name)
			projectedMatches++
		}
	}
	require.Equal(t, 1, projectedMatches, "projected queue list should contain the queue created by this run")

	// キューの設定を更新する
	data = h.Call(t, "キューの設定を更新する",
		"simplemq-api", "queue", "config", id,
		"--visibility-timeout-seconds", strconv.Itoa(visibilityTimeoutSeconds),
		"--expire-seconds", strconv.Itoa(expireSeconds))
	configured := parseQueue(t, data)
	requireQueueIdentity(t, configured, created.ID, name, description)
	require.EqualValues(t, visibilityTimeoutSeconds, configured.Settings.VisibilityTimeoutSeconds,
		"queue visibility timeout should be the value set by this run")
	require.EqualValues(t, expireSeconds, configured.Settings.ExpireSeconds,
		"queue expire seconds should be the value set by this run")

	// キューの API キーを発行する
	// 応答には秘密情報が含まれるため、実行記録に stdout を残さない
	apiKey, err := parseAPIKey(h.CallHidingStdout(t, "SimpleMQ キューの API キーを発行する",
		"simplemq-api", "queue", "rotate-api-key", id))
	require.NoError(t, err, "decode rotated SimpleMQ API key: %v", err)
	keyFile := writeKeyFile(t, apiKey)

	// メッセージを送信する
	data = h.Call(t, "メッセージを送信する",
		"simplemq-api", "message", "send",
		"--queue-name", name, "--api-key-file", keyFile, "--content", messageBody)
	var sent message.NewMessage
	err = json.Unmarshal(data, &sent)
	require.NoError(t, err, "decode sent message: %v", err)
	require.NotEmpty(t, sent.ID, "send response should contain the submitted message ID")
	require.Equal(t, messageBody, string(sent.Content), "sent message content should match the submitted content")

	// 送信したメッセージを受信する
	data = h.Call(t, "メッセージを受信する",
		"simplemq-api", "message", "receive", "--queue-name", name, "--api-key-file", keyFile)
	var received []message.Message
	err = json.Unmarshal(data, &received)
	require.NoError(t, err, "decode received messages: %v", err)
	require.Len(t, received, 1, "receive should return the message sent by this run")
	require.Equal(t, sent.ID, received[0].ID, "received message ID should match the message sent by this run")
	require.Equal(t, messageBody, string(received[0].Content), "received message content should match the submitted content")

	// 受信したメッセージの可視性タイムアウトを延長する
	data = h.Call(t, "メッセージの可視性タイムアウトを延長する",
		"simplemq-api", "message", "extend-timeout",
		"--queue-name", name, "--api-key-file", keyFile, string(received[0].ID))
	var extended message.Message
	err = json.Unmarshal(data, &extended)
	require.NoError(t, err, "decode extended message: %v", err)
	require.Greater(t, extended.VisibilityTimeoutAt, received[0].VisibilityTimeoutAt,
		"extend-timeout should advance the message visibility timeout")

	// 受信したメッセージを削除する
	_ = h.Call(t, "メッセージを削除する",
		"simplemq-api", "message", "delete",
		"--queue-name", name, "--api-key-file", keyFile, string(sent.ID))

	// キュー内のメッセージ数が 0 であることを確認する
	data = h.Call(t, "キュー内のメッセージ数を取得する", "simplemq-api", "queue", "count-messages", id)
	var count struct {
		Count int `json:"Count"`
	}
	err = json.Unmarshal(data, &count)
	require.NoError(t, err, "decode message count: %v", err)
	require.Equal(t, 0, count.Count, "queue should have no messages after deleting the test message")
}

// queueSummary は --query で射影した一覧項目です。
type queueSummary struct {
	ID   queue.CommonServiceItemID `json:"ID"`
	Name string                    `json:"Name"`
}

// listQueues はキューの一覧を取得します。
func listQueues(t *testing.T, h *helper.Helper, description string) []queue.CommonServiceItem {
	t.Helper()

	data := h.Call(t, description, "simplemq-api", "queue", "list")
	var items []queue.CommonServiceItem
	err := json.Unmarshal(data, &items)
	require.NoError(t, err, "%s: decode queues: %v", description, err)
	require.NotNil(t, items, "%s: expected a JSON queue array", description)
	return items
}

// queuesNamed は一覧から指定した名前のキューを抽出します。
func queuesNamed(items []queue.CommonServiceItem, name string) []*queue.CommonServiceItem {
	matches := make([]*queue.CommonServiceItem, 0)
	for i := range items {
		if items[i].Name == name {
			matches = append(matches, &items[i])
		}
	}
	return matches
}

// parseQueue は CLI の JSON 出力をキューとして解釈します。
func parseQueue(t *testing.T, data []byte) queue.CommonServiceItem {
	t.Helper()

	var item queue.CommonServiceItem
	err := json.Unmarshal(data, &item)
	require.NoError(t, err, "decode queue: %v", err)
	require.True(t, item.ID.IsString() || item.ID.IsInt(), "queue ID is missing")
	return item
}

// queueIDString はキュー ID を位置引数に渡せる文字列に変換します。
// SimpleMQ キューの ID は文字列または数値のどちらかで返ります。
func queueIDString(t *testing.T, item queue.CommonServiceItem) string {
	t.Helper()

	if value, ok := item.ID.GetString(); ok {
		return value
	}
	value, ok := item.ID.GetInt()
	require.True(t, ok, "queue ID has unsupported type %q", item.ID.Type)
	return strconv.Itoa(value)
}

// requireQueueIdentity は参照結果の ID・名前・説明が、今回の実行で作成したキューと一致することを確認します。
func requireQueueIdentity(t *testing.T, item queue.CommonServiceItem, id queue.CommonServiceItemID, name, description string) {
	t.Helper()

	require.Equal(t, id, item.ID, "queue ID should match the queue created by this run")
	require.Equal(t, name, item.Name, "queue name should match the name requested by this run")
	require.Equal(t, name, item.Status.QueueName, "queue status name should match the name requested by this run")
	require.Equal(t, description, item.Description.Value, "queue description should match the description requested by this run")
}

// writeKeyFile は発行したキュー API キーを、秘密情報を受け取る CLI フラグ用の保護されたファイルに書き込みます。
func writeKeyFile(t *testing.T, apiKey string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "simplemq.key")
	err := os.WriteFile(path, []byte(apiKey), 0o600)
	require.NoError(t, err, "write private SimpleMQ API key file: %v", err)
	return path
}

// parseAPIKey は rotate-api-key の応答から API キーを取り出します。
func parseAPIKey(data []byte) (string, error) {
	var response struct {
		APIKey string `json:"APIKey"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode API key: %w", err)
	}
	if response.APIKey == "" {
		return "", errors.New("SimpleMQ API key is empty")
	}
	return response.APIKey, nil
}

// TestParseAPIKey は rotate-api-key の応答から API キーを取り出す処理を確認する。
func TestParseAPIKey(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		apiKey, err := parseAPIKey([]byte(`{"APIKey":"test-only-api-key"}`))
		require.NoError(t, err)
		require.Equal(t, "test-only-api-key", apiKey)
	})
	t.Run("empty key", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`{"APIKey":""}`))
		require.Error(t, err)
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`not-json`))
		require.Error(t, err)
	})
}

// TestQueueIDString はキュー ID の文字列変換を確認する。
func TestQueueIDString(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		item := queue.CommonServiceItem{ID: queue.CommonServiceItemID{Type: queue.StringCommonServiceItemID, String: "test-only-id"}}
		require.Equal(t, "test-only-id", queueIDString(t, item))
	})
	t.Run("number", func(t *testing.T) {
		item := queue.CommonServiceItem{ID: queue.CommonServiceItemID{Type: queue.IntCommonServiceItemID, Int: 123456789012}}
		require.Equal(t, "123456789012", queueIDString(t, item))
	})
}
