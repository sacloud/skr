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

// このファイルは eventbus コマンド E2E のシナリオです。
// 実行設定、トリガー、スケジュールのライフサイクルと、スイッチ作成イベントによる
// SimpleMQ キューへのメッセージ配送を確認します。
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
	// testMessagePrefix は SimpleMQ で受信を確認するメッセージ内容の接頭辞です。
	testMessagePrefix = "SKRE2EHIHGHLVL"
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
	// triggerSettleDelay はトリガーや実行設定の変更が反映されるまでの待機時間です。
	triggerSettleDelay = 60 * time.Second
)

// TestScenarioEventbus は eventbus の高レベルコマンドのライブ E2E です。
// SimpleMQ キュー、実行設定、トリガー、将来開始のスケジュールを作成し、
// スイッチ作成イベントによるメッセージ配送を確認してからすべて削除します。
func TestScenarioEventbus(t *testing.T) {
	h := helper.New(t, "eventbus",
		"EventBus の高レベルコマンドで実行設定、トリガー、スケジュールと SimpleMQ キューを作成し、イベント連携と各リソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	description := resourceDescriptionPrefix + suffix
	expected := testMessagePrefix + strings.ToUpper(suffix)

	queue := &eventbusResource{kind: "queue", name: name, description: description}
	process := &eventbusResource{kind: "process-configuration", name: name, description: description}
	trigger := &eventbusResource{kind: "trigger", name: name, description: description}
	schedule := &eventbusResource{kind: "schedule", name: name, description: description}
	sw1 := &eventbusResource{kind: "switch", name: name, description: description, zone: zone}
	sw2 := &eventbusResource{kind: "switch", name: name + "-2", description: description, zone: zone}

	// 作成前に同名リソースが存在しないことを確認する
	requireNoNameCollision(t, h, queue, process, trigger, schedule, sw1, sw2)

	// 後片付け: 依存関係の逆順で再照合つき削除し、一覧からの消失を確認する
	defer func() {
		cleanupEventbusResources(t, h, schedule, trigger, process, sw2, sw1, queue)
	}()

	// SimpleMQ キューを作成する
	queue.attempted = true
	createdQueue := createSimpleMQQueue(t, h, "SimpleMQ キューを作成する", queue)
	queueID, err := createdQueue.id()
	require.NoError(t, err, "decode created SimpleMQ queue ID: %v", err)
	require.NotEmpty(t, queueID, "SimpleMQ queue create should return ID")
	queue.id = queueID
	require.NoError(t, checkQueueIdentity(createdQueue, queue, queue.id), "created SimpleMQ queue identity mismatch")

	// SimpleMQ キューの API キーを発行する
	// 応答には秘密情報が含まれるため、実行記録に stdout を残さない
	apiKey, err := parseAPIKey(h.CallHidingStdout(t, "SimpleMQ キューの API キーを発行する",
		"simplemq-api", "rotate-api-key", queue.id))
	require.NoError(t, err, "decode rotated SimpleMQ API key: %v", err)
	keyFile, secretFile := writeSecretFiles(t, apiKey)

	// 実行設定を作成する (simplemq 宛先、キュー名とメッセージ本文を指定する)
	process.attempted = true
	var created processConfigurationItem
	data := h.Call(t, "実行設定を作成する",
		"eventbus", "process-configuration", "create",
		"--name", process.name, "--description", description,
		"--destination", "simplemq",
		"--queue-name", queue.name,
		"--content", expected,
	)
	require.NoError(t, decodeJSON(data, &created, false), "decode created process configuration")
	createdID, err := created.id()
	require.NoError(t, err, "decode created process-configuration ID: %v", err)
	require.NotEmpty(t, createdID, "process-configuration create should return ID")
	process.id = createdID
	require.NoError(t, checkProcessConfigurationIdentity(created, process, process.id), "created process configuration identity mismatch")
	require.Equal(t, "simplemq", created.Destination, "destination should be simplemq")
	require.Equal(t, map[string]string{"queue_name": queue.name, "content": expected},
		created.Parameters, "parameters should match the given flags")

	// 実行設定を参照して一覧に含まれることを確認する
	read := readProcessConfiguration(t, h, "実行設定を ID で参照する", process.id)
	require.NoError(t, checkProcessConfigurationIdentity(read, process, process.id), "read process configuration identity mismatch")
	require.Equal(t, created.Parameters, read.Parameters, "read parameters should match the create result")
	requireContainsProcessConfiguration(t, listProcessConfigurations(t, h, "実行設定の一覧を取得する"), process.id, "process configuration list")

	// 実行設定へ SimpleMQ の API キーを登録する
	// secret は write-only のため応答では確認できません。API が受け付けたこと (200) を確認します。
	h.Call(t, "実行設定へ SimpleMQ の API キーを登録する",
		"eventbus", "process-configuration", "set-secret", process.id, "--secret-file", "@"+secretFile)

	// トリガーを作成する (イベントログのスイッチ作成タイプ、ゾーン条件を指定する)
	trigger.attempted = true
	var createdTrigger triggerItem
	data = h.Call(t, "トリガーを作成する",
		"eventbus", "trigger", "create",
		"--name", trigger.name, "--description", description,
		"--source", eventSource,
		"--type", switchCreatedType,
		"--condition", "key=zone,op=eq,value="+zone,
		"--process-configuration-id", process.id,
	)
	require.NoError(t, decodeJSON(data, &createdTrigger, false), "decode created trigger")
	createdTriggerID, err := createdTrigger.id()
	require.NoError(t, err, "decode created trigger ID: %v", err)
	require.NotEmpty(t, createdTriggerID, "trigger create should return ID")
	trigger.id = createdTriggerID
	require.NoError(t, checkTriggerIdentity(createdTrigger, trigger, trigger.id), "created trigger identity mismatch")
	require.Equal(t, eventSource, createdTrigger.Source, "source should match the given flag")
	require.Equal(t, []string{switchCreatedType}, createdTrigger.Types, "types should match the given flag")
	require.Equal(t, []triggerConditionItem{{Key: "zone", Op: "eq", Values: []string{zone}}},
		createdTrigger.Conditions, "conditions should match the given flags")
	triggeredProcessID, err := decodeResourceID(createdTrigger.ProcessConfigurationID)
	require.NoError(t, err, "decode created trigger ProcessConfigurationID: %v", err)
	require.Equal(t, process.id, triggeredProcessID,
		"trigger should reference the created process configuration")

	// トリガーを参照して一覧に含まれることを確認する
	readTriggerItem := readTrigger(t, h, "トリガーを ID で参照する", trigger.id)
	require.NoError(t, checkTriggerIdentity(readTriggerItem, trigger, trigger.id), "read trigger identity mismatch")
	require.Equal(t, createdTrigger.Conditions, readTriggerItem.Conditions, "read conditions should match the create result")
	requireContainsTrigger(t, listTriggers(t, h, "トリガーの一覧を取得する"), trigger.id, "trigger list")

	// トリガーの説明を更新して読み戻す
	// 今回作成して削除するリソースのため、説明を元に戻す手順は設けません。
	updatedDescription := description + " (updated)"
	var updatedTrigger triggerItem
	data = h.Call(t, "トリガーの説明を更新する",
		"eventbus", "trigger", "update", trigger.id,
		"--name", trigger.name, "--description", updatedDescription)
	require.NoError(t, decodeJSON(data, &updatedTrigger, false), "decode updated trigger")
	require.Equal(t, updatedDescription, *updatedTrigger.Description, "description should be updated")
	require.Equal(t, createdTrigger.Types, updatedTrigger.Types, "types should be kept after a description-only update")
	readTriggerItem = readTrigger(t, h, "更新後のトリガーを再参照する", trigger.id)
	require.Equal(t, updatedDescription, *readTriggerItem.Description, "read description should keep the update result")
	// 後片付けの再照合に備えて、現在の説明を追跡する。説明の復元は行わない。
	trigger.description = updatedDescription

	// スケジュールを作成する (開始日時は 24 時間後、1 時間ごとの繰り返し)
	schedule.attempted = true
	startsAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	var createdSchedule scheduleItem
	data = h.Call(t, "スケジュールを作成する",
		"eventbus", "schedule", "create",
		"--name", schedule.name, "--description", description,
		"--process-configuration-id", process.id,
		"--starts-at", startsAt,
		"--recurring-step", "1",
		"--recurring-unit", "hour",
	)
	require.NoError(t, decodeJSON(data, &createdSchedule, false), "decode created schedule")
	createdScheduleID, err := createdSchedule.id()
	require.NoError(t, err, "decode created schedule ID: %v", err)
	require.NotEmpty(t, createdScheduleID, "schedule create should return ID")
	schedule.id = createdScheduleID
	require.NoError(t, checkScheduleIdentity(createdSchedule, schedule, schedule.id), "created schedule identity mismatch")
	scheduledProcessID, err := decodeResourceID(createdSchedule.ProcessConfigurationID)
	require.NoError(t, err, "decode created schedule ProcessConfigurationID: %v", err)
	require.Equal(t, process.id, scheduledProcessID, "schedule should reference the created process configuration")
	requireStartsAtEqual(t, startsAt, createdSchedule.StartsAt)
	require.NotNil(t, createdSchedule.RecurringStep, "recurring step should be set")
	require.Equal(t, 1, *createdSchedule.RecurringStep, "recurring step should match the given flag")
	require.NotNil(t, createdSchedule.RecurringUnit, "recurring unit should be set")
	require.Equal(t, "hour", *createdSchedule.RecurringUnit, "recurring unit should match the given flag")

	// スケジュールを参照して一覧に含まれることを確認する
	readScheduleItem := readSchedule(t, h, "スケジュールを ID で参照する", schedule.id)
	require.NoError(t, checkScheduleIdentity(readScheduleItem, schedule, schedule.id), "read schedule identity mismatch")
	requireStartsAtEqual(t, startsAt, readScheduleItem.StartsAt)
	requireContainsSchedule(t, listSchedules(t, h, "スケジュールの一覧を取得する"), schedule.id, "schedule list")

	// スケジュールの開始日時を変更して読み戻す
	// 開始日時だけの更新で、周期指定が維持されることを確認します。
	updatedStartsAt := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	var updatedSchedule scheduleItem
	data = h.Call(t, "スケジュールの開始日時を更新する",
		"eventbus", "schedule", "update", schedule.id,
		"--name", schedule.name,
		"--starts-at", updatedStartsAt)
	require.NoError(t, decodeJSON(data, &updatedSchedule, false), "decode updated schedule")
	requireStartsAtEqual(t, updatedStartsAt, updatedSchedule.StartsAt)
	require.NotNil(t, updatedSchedule.RecurringStep, "recurring step should be kept after a starts-at-only update")
	require.Equal(t, 1, *updatedSchedule.RecurringStep, "recurring step should keep the created value")
	readScheduleItem = readSchedule(t, h, "更新後のスケジュールを再参照する", schedule.id)
	requireStartsAtEqual(t, updatedStartsAt, readScheduleItem.StartsAt)

	// スケジュールを削除して、一覧から消失することを確認する
	h.Call(t, "スケジュールを削除する", "eventbus", "schedule", "delete", schedule.id)
	schedule.deleted = true
	for _, item := range listSchedules(t, h, "削除後にスケジュールが一覧に残っていないことを確認する") {
		remaining, err := item.id()
		require.NoError(t, err, "decode remaining schedule ID: %v", err)
		require.NotEqual(t, schedule.id, remaining, "schedule %s remains after delete", schedule.id)
	}

	// トリガーの設定が反映されるまで待つ
	h.Sleep(t, "トリガーの設定が反映されるまで待つ", triggerSettleDelay)

	// テスト用のスイッチを作成する (1 個目)
	sw1.attempted = true
	createdSwitch := createSwitch(t, h, "テスト用のスイッチを作成する", sw1)
	sw1.id = requireSwitchID(t, createdSwitch, sw1)
	require.NoError(t, checkSwitchIdentity(createdSwitch, sw1, sw1.id), "created switch identity mismatch")

	// スイッチ作成イベントのメッセージがキューへ届くまで待つ
	waitForEventMessage(t, h, queue.name, keyFile, expected)

	// 実行設定の本文を更新して読み戻す
	updatedExpected := expected + "UPDATED"
	var reconfigured processConfigurationItem
	data = h.Call(t, "実行設定の本文を更新する",
		"eventbus", "process-configuration", "update", process.id,
		"--name", process.name,
		"--content", updatedExpected)
	require.NoError(t, decodeJSON(data, &reconfigured, false), "decode reconfigured process configuration")
	require.Equal(t, map[string]string{"queue_name": queue.name, "content": updatedExpected},
		reconfigured.Parameters, "content should be updated and queue_name kept")

	// 変更が反映されるまで待つ
	h.Sleep(t, "実行設定の変更が反映されるまで待つ", triggerSettleDelay)

	// ゾーンのリソース数上限を避けるため、1 個目のスイッチを削除してから 2 個目を作成する。
	// 削除は再照合つきの安全な手順で行い、以降の後片付けではスキップされる。
	cleanupEventbusResource(t, h, sw1)

	// テスト用のスイッチを作成する (2 個目)
	sw2.attempted = true
	createdSwitch2 := createSwitch(t, h, "変更後の本文を確認するためスイッチを作成する", sw2)
	sw2.id = requireSwitchID(t, createdSwitch2, sw2)
	require.NoError(t, checkSwitchIdentity(createdSwitch2, sw2, sw2.id), "created switch identity mismatch")

	// 変更後の本文がキューへ届くまで待つ
	waitForEventMessage(t, h, queue.name, keyFile, updatedExpected)
}

// requireSwitchID はスイッチの作成結果の ID を解釈して返します。
func requireSwitchID(t *testing.T, item switchItem, r *eventbusResource) string {
	t.Helper()

	id, err := decodeResourceID(item.ID)
	require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
	require.NotEmpty(t, id, "%s create should return ID", kindName(r.kind))
	return id
}

// requireContainsProcessConfiguration は実行設定の一覧に指定した ID の要素が含まれることを確認します。
func requireContainsProcessConfiguration(t *testing.T, items []processConfigurationItem, id, label string) {
	t.Helper()

	for _, item := range items {
		itemID, err := item.id()
		require.NoError(t, err, "decode process-configuration list ID: %v", err)
		if itemID == id {
			return
		}
	}
	t.Fatalf("%s should contain %s", label, id)
}

// requireContainsSchedule はスケジュールの一覧に指定した ID の要素が含まれることを確認します。
func requireContainsSchedule(t *testing.T, items []scheduleItem, id, label string) {
	t.Helper()

	for _, item := range items {
		itemID, err := item.id()
		require.NoError(t, err, "decode schedule list ID: %v", err)
		if itemID == id {
			return
		}
	}
	t.Fatalf("%s should contain %s", label, id)
}

// requireContainsTrigger はトリガーの一覧に指定した ID の要素が含まれることを確認します。
func requireContainsTrigger(t *testing.T, items []triggerItem, id, label string) {
	t.Helper()

	for _, item := range items {
		itemID, err := item.id()
		require.NoError(t, err, "decode trigger list ID: %v", err)
		if itemID == id {
			return
		}
	}
	t.Fatalf("%s should contain %s", label, id)
}

// requireStartsAtEqual は出力の StartsAt が、指定した RFC3339 文字列と同じ時刻であることを確認します。
// 出力はミリ秒精度の RFC3339 (UTC) のため、時刻として比較します。
func requireStartsAtEqual(t *testing.T, want, got string, msgAndArgs ...any) {
	t.Helper()

	wantTime, err := time.Parse(time.RFC3339, want)
	require.NoError(t, err, "parse expected starts-at %q", want)
	gotTime, err := time.Parse(time.RFC3339Nano, got)
	require.NoError(t, err, "parse output starts-at %q", got)
	require.WithinDuration(t, wantTime, gotTime, time.Second, "starts-at should match the given instant", msgAndArgs)
}

// waitForEventMessage は指定した本文のメッセージがキューへ届くまで期限付きで待ちます。
// 受信したメッセージの内容が期待値と一致することと、メッセージ ID があることを確認します。
// イベント配送はベストエフォート型のため、期限まで再試行します。通信失敗は不在とみなしません。
func waitForEventMessage(t *testing.T, h *helper.Helper, queueName, keyFile, expected string) {
	t.Helper()

	deadline := time.Now().Add(eventWaitLimit)
	for attempt := 1; ; attempt++ {
		data := h.CallWithTimeout(t, commandTimeout,
			fmt.Sprintf("イベントメッセージを受信する（%d 回目）", attempt),
			"simplemq-api", "receive-message", "--queue-name", queueName, "--api-key-file", keyFile)
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
