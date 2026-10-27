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

const (
	// zone はテスト用スイッチを作成するゾーンです。
	zone = "is1b"
	// resourceNamePrefix は今回の実行が作成するリソース名の接頭辞です。
	resourceNamePrefix = "skr-e2e-eventbus-"
	// resourceDescriptionPrefix は今回の実行が作成するリソース説明の接頭辞です。
	resourceDescriptionPrefix = "skr-e2e-eventbus/"
	// testMessagePrefix は SimpleMQ で受信を確認するメッセージ内容の接頭辞です。
	testMessagePrefix = "SKRE2ESWITCHCREATED"
	// eventSource はスイッチ作成イベントのイベントソースです。
	eventSource = "//eventbus.sakura.ad.jp/eventlog"
	// switchCreatedType は通常スイッチ作成イベントのイベントタイプです。
	switchCreatedType = "jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created"
	// eventWaitLimit はイベントメッセージを受信するまでの確認期限です。
	eventWaitLimit = 5 * time.Minute
	// eventPolling はメッセージ受信の再試行間隔です。
	eventPolling = 3 * time.Second
	// commandTimeout は1回の CLI 呼び出しのタイムアウトです。
	commandTimeout = 60 * time.Second
	// triggerSettleDelay はトリガーの設定が反映されるまでの待機時間です。
	triggerSettleDelay = 60 * time.Second
)

func TestScenarioEventBus(t *testing.T) {
	h := helper.New(t, "eventbus-api",
		"EventBus のプロセス設定、トリガー、スケジュールと SimpleMQ キューを作成し、イベント連携と各リソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	resourceDescription := resourceDescriptionPrefix + suffix
	expected := testMessagePrefix + strings.ToUpper(suffix)

	queue := &eventbusResource{kind: "queue", name: name, description: resourceDescription}
	config := &eventbusResource{kind: "process-configuration", name: name, description: resourceDescription}
	trigger := &eventbusResource{kind: "trigger", name: name, description: resourceDescription}
	schedule := &eventbusResource{kind: "schedule", name: name, description: resourceDescription}
	sw := &eventbusResource{kind: "switch", name: name, description: resourceDescription}

	// 作成前に同名リソースが存在しないことを確認する
	requireNoNameCollision(t, h, queue, config, trigger, schedule, sw)

	// 後片付け: 依存関係の逆順で再照合つき削除し、一覧からの消失を確認する
	defer func() {
		cleanupEventbusResources(t, h, schedule, trigger, config, sw, queue)
	}()

	// SimpleMQ キューを作成する
	queue.attempted = true
	created := parseEventbusItem(t, h.Call(t, "SimpleMQ キューを作成する",
		"simplemq-api", "queue", "create", "--name", queue.name, "--description", queue.description))
	queue.id = requireEventbusIdentity(t, created, queue)

	// SimpleMQ キューの API キーを発行する
	// 応答には秘密情報が含まれるため、実行記録に stdout を残さない
	apiKey, err := parseAPIKey(h.CallHidingStdout(t, "SimpleMQ キューの API キーを発行する",
		"simplemq-api", "queue", "rotate-api-key", queue.id))
	require.NoError(t, err, "decode rotated SimpleMQ API key: %v", err)

	// 秘密情報を CLI フラグ用の一時ファイルに書き込む
	keyFile, secretFile := writeSecretFiles(t, apiKey)

	// プロセス設定を作成する
	config.attempted = true
	createParameters, err := simplemqParameters(queue.name, expected)
	require.NoError(t, err, "encode SimpleMQ parameters: %v", err)
	createConfigRequest := h.WriteRequestJSONFile(t, "create-process-configuration", map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        config.name,
			"Description": config.description,
			"Settings": map[string]any{
				"Destination": "simplemq",
				"Parameters":  createParameters,
			},
		},
	})
	created = parseEventbusItem(t, h.Call(t, "プロセス設定を作成する",
		"eventbus-api", "process-configuration", "create", "--request", "@"+createConfigRequest))
	config.id = requireEventbusIdentity(t, created, config)

	// プロセス設定を更新して反映を確認する
	expected = updateConfigurationAndVerify(t, h, config, queue, expected)

	// プロセス設定へ SimpleMQ の API キーを登録する
	h.Call(t, "プロセス設定へ SimpleMQ の API キーを登録する",
		"eventbus-api", "process-configuration", "update-secret", config.id, "--secret-file", secretFile)

	// トリガーを作成する
	trigger.attempted = true
	createTriggerRequest := h.WriteRequestJSONFile(t, "create-trigger", map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        trigger.name,
			"Description": trigger.description,
			"Settings": map[string]any{
				"Source": eventSource,
				"Types":  []string{switchCreatedType},
				"Conditions": []any{
					map[string]any{"Key": "zone", "Op": "eq", "Values": []string{zone}},
				},
				"ProcessConfigurationID": config.id,
			},
		},
	})
	created = parseEventbusItem(t, h.Call(t, "トリガーを作成する",
		"eventbus-api", "trigger", "create", "--request", "@"+createTriggerRequest))
	trigger.id = requireEventbusIdentity(t, created, trigger)

	// トリガーを参照して設定を確認する
	readTrigger, err := lookupTypedItem[triggerItem](t, h, "トリガーを参照する", "trigger", trigger.id)
	require.NoError(t, err, "read trigger: %v", err)
	require.NoError(t, checkTriggerIdentity(readTrigger, trigger, config.id, trigger.description),
		"trigger check failed before updating it")

	// トリガーの説明を更新して元に戻す
	updateTriggerAndVerify(t, h, trigger, config.id)

	// スケジュールのライフサイクルを確認する
	runScheduleScenario(t, h, schedule, config.id)

	// トリガーの設定が反映されるまで待つ
	h.Sleep(t, "トリガーの設定が反映されるまで待つ", triggerSettleDelay)

	// テスト用のスイッチを作成する
	sw.attempted = true
	created = parseEventbusItem(t, h.Call(t, "テスト用のスイッチを作成する",
		"iaas-api", "switch", "create", "--zone", zone, "--name", sw.name, "--description", sw.description))
	sw.id = requireEventbusIdentity(t, created, sw)

	// スイッチ作成イベントのメッセージがキューへ届くまで待つ
	waitForEventMessage(t, h, queue.name, keyFile, expected)
}

// message は SimpleMQ から受信したメッセージです。
type message struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// waitForEventMessage はスイッチ作成イベントのメッセージがキューへ届くまで期限付きで待ちます。
// 受信したメッセージの内容が期待値と一致することと、メッセージ ID があることを確認します。
// イベント配送はベストエフォート型のため、期限まで再試行します。
func waitForEventMessage(t *testing.T, h *helper.Helper, queueName, keyFile, expected string) {
	t.Helper()

	deadline := time.Now().Add(eventWaitLimit)
	for attempt := 1; ; attempt++ {
		data := h.CallWithTimeout(t, commandTimeout,
			fmt.Sprintf("イベントメッセージを受信する（%d 回目）", attempt),
			"simplemq-api", "message", "receive", "--queue-name", queueName, "--api-key-file", keyFile)
		var received []message
		err := json.Unmarshal(data, &received)
		require.NoError(t, err, "decode received messages: %v", err)
		for _, m := range received {
			if m.Content == expected {
				require.NotEmpty(t, m.ID, "received EventBus message has no ID")
				return
			}
		}
		if time.Now().Add(eventPolling).After(deadline) {
			t.Fatalf("timed out after %s waiting for the event message", eventWaitLimit)
		}
		h.Sleep(t, "イベントメッセージの再受信まで待つ", eventPolling)
	}
}
