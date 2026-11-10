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

// このファイルは eventbus コマンド E2E の共有ヘルパーです。
// 高レベルコマンドの JSON 出力の解釈、識別情報の照合、秘密情報ファイルの作成、
// イベントメッセージの受信確認、後片付けを提供します。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// resourceNamePrefix は今回の実行が作成するリソース名の接頭辞です。
	resourceNamePrefix = "skr-e2e-eventbus-"
	// resourceDescriptionPrefix は今回の実行が作成するリソース説明の接頭辞です。
	resourceDescriptionPrefix = "skr-e2e-eventbus/"
)

// eventbusResource は今回の実行が扱うリソースの識別情報と後片付けの状態です。
type eventbusResource struct {
	kind        string
	name        string
	description string
	// zone はスイッチの作成先ゾーンです。スイッチ以外では空です。
	zone string
	id   string
	// attempted は作成 API を呼び出したかどうかです。呼び出しが失敗しても後片付けの対象にします。
	attempted bool
	// deleted はシナリオ内で削除済みかどうかです。削除済みのリソースは後片付けで変更しません。
	deleted bool
	// 以下は削除前の再照合で設定値ごと確認するための、今回の実行で設定した構成です。
	// テスト中に変更する項目 (説明、本文、開始日時) は、更新の CLI 呼び出しが成功した直後に追跡します。
	processConfigurationID string
	destination            string
	parameters             map[string]string
	startsAt               string
	recurringStep          int
	recurringUnit          string
	source                 string
	types                  []string
	conditions             []triggerConditionItem
}

// queueItem は SimpleMQ キュー コマンドの JSON 出力を解釈するための構造体です。
// SimpleMQ キューの ID は実環境では数値、sakumock では文字列で返ります。
type queueItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Status      struct {
		QueueName string `json:"QueueName"`
	} `json:"Status"`
}

func (i queueItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// processConfigurationItem は eventbus process-configuration コマンドの JSON 出力を解釈するための構造体です。
// リソース ID は実環境では数値、sakumock では文字列で返る可能性があります。
type processConfigurationItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description *string         `json:"Description"`
	Destination string          `json:"Destination"`
	Parameters  map[string]string
	Tags        []string
}

func (i processConfigurationItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// scheduleItem は eventbus schedule コマンドの JSON 出力を解釈するための構造体です。
// リソース ID は実環境では数値、sakumock では文字列で返る可能性があります。
type scheduleItem struct {
	ID                     json.RawMessage `json:"ID"`
	Name                   string          `json:"Name"`
	Description            *string         `json:"Description"`
	ProcessConfigurationID json.RawMessage `json:"ProcessConfigurationID"`
	StartsAt               string          `json:"StartsAt"`
	RecurringStep          *int            `json:"RecurringStep"`
	RecurringUnit          *string         `json:"RecurringUnit"`
	Crontab                *string         `json:"Crontab"`
	Tags                   []string
}

func (i scheduleItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// triggerItem は eventbus trigger コマンドの JSON 出力を解釈するための構造体です。
// リソース ID は実環境では数値、sakumock では文字列で返る可能性があります。
type triggerItem struct {
	ID                     json.RawMessage `json:"ID"`
	Name                   string          `json:"Name"`
	Description            *string         `json:"Description"`
	Source                 string          `json:"Source"`
	Types                  []string
	Conditions             []triggerConditionItem
	ProcessConfigurationID json.RawMessage `json:"ProcessConfigurationID"`
	Tags                   []string
}

func (i triggerItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// triggerConditionItem は eventbus trigger コマンドの出力の条件です。
type triggerConditionItem struct {
	Key    string   `json:"Key"`
	Op     string   `json:"Op"`
	Values []string `json:"Values"`
}

// switchItem は IaaS スイッチ コマンドの JSON 出力を解釈するための構造体です。
// IaaS のリソース ID は数値で返ります。
type switchItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
}

// decodeResourceID は JSON 文字列または数値としてエンコードされたリソース ID をデコードします。
// EventBus と SimpleMQ のリソース ID は文字列、IaaS のリソース ID は数値です。
func decodeResourceID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("resource ID is missing")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode resource ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("resource ID is empty")
		}
		return value, nil
	case json.Number:
		// IaaS の未割り当て ID は 0 として返るため、空の ID として扱う。
		if value == "0" {
			return "", errors.New("resource ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("resource ID has unsupported JSON type %T", value)
	}
}

// decodeSwitchItem はスイッチ コマンドの JSON 出力を解釈します。
func decodeSwitchItem(data []byte) (switchItem, error) {
	var item switchItem
	if err := json.Unmarshal(data, &item); err != nil {
		return switchItem{}, fmt.Errorf("decode switch: %w", err)
	}
	if _, err := decodeResourceID(item.ID); err != nil {
		return switchItem{}, err
	}
	return item, nil
}

// kindName はリソース種別の利用者向けの名称を返します。
func kindName(kind string) string {
	switch kind {
	case "queue":
		return "SimpleMQ キュー"
	case "process-configuration":
		return "実行設定"
	case "trigger":
		return "トリガー"
	case "schedule":
		return "スケジュール"
	case "switch":
		return "スイッチ"
	default:
		return kind
	}
}

// listProcessConfigurations は実行設定の一覧を取得します。
func listProcessConfigurations(t *testing.T, h *helper.Helper, callDescription string) []processConfigurationItem {
	t.Helper()

	var items []processConfigurationItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "process-configuration", "list"), &items, true), "%s", callDescription)
	return items
}

// readProcessConfiguration は ID で実行設定を参照します。
func readProcessConfiguration(t *testing.T, h *helper.Helper, callDescription, id string) processConfigurationItem {
	t.Helper()

	var item processConfigurationItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "process-configuration", "read", id), &item, false), "%s", callDescription)
	return item
}

// listSchedules はスケジュールの一覧を取得します。
func listSchedules(t *testing.T, h *helper.Helper, callDescription string) []scheduleItem {
	t.Helper()

	var items []scheduleItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "schedule", "list"), &items, true), "%s", callDescription)
	return items
}

// readSchedule は ID でスケジュールを参照します。
func readSchedule(t *testing.T, h *helper.Helper, callDescription, id string) scheduleItem {
	t.Helper()

	var item scheduleItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "schedule", "read", id), &item, false), "%s", callDescription)
	return item
}

// listTriggers はトリガーの一覧を取得します。
func listTriggers(t *testing.T, h *helper.Helper, callDescription string) []triggerItem {
	t.Helper()

	var items []triggerItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "trigger", "list"), &items, true), "%s", callDescription)
	return items
}

// readTrigger は ID でトリガーを参照します。
func readTrigger(t *testing.T, h *helper.Helper, callDescription, id string) triggerItem {
	t.Helper()

	var item triggerItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"eventbus", "trigger", "read", id), &item, false), "%s", callDescription)
	return item
}

// createSimpleMQQueue は宛先として使う SimpleMQ キューを作成します。
func createSimpleMQQueue(t *testing.T, h *helper.Helper, callDescription string, queue *eventbusResource) queueItem {
	t.Helper()

	var item queueItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"simplemq-api", "create-queue", "--name", queue.name, "--description", queue.description), &item, false),
		"%s", callDescription)
	return item
}

// readSimpleMQQueue は ID で SimpleMQ キューを参照します。
func readSimpleMQQueue(t *testing.T, h *helper.Helper, callDescription, id string) queueItem {
	t.Helper()

	var item queueItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"simplemq-api", "read-queue", id), &item, false), "%s", callDescription)
	return item
}

// listSimpleMQQueues は SimpleMQ キューの一覧を取得します。
func listSimpleMQQueues(t *testing.T, h *helper.Helper, callDescription string) []queueItem {
	t.Helper()

	var items []queueItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"simplemq-api", "list-queues"), &items, true), "%s", callDescription)
	return items
}

// createSwitch はテスト用の未接続スイッチを作成します。
func createSwitch(t *testing.T, h *helper.Helper, callDescription string, sw *eventbusResource) switchItem {
	t.Helper()

	item, err := decodeSwitchItem(h.Call(t, callDescription,
		"iaas-api", "switch", "create", "--zone", sw.zone, "--name", sw.name, "--description", sw.description))
	require.NoError(t, err, "%s", callDescription)
	return item
}

// listSwitches はゾーンのスイッチ一覧を取得します。
func listSwitches(t *testing.T, h *helper.Helper, callDescription, zone string) []switchItem {
	t.Helper()

	var items []switchItem
	require.NoError(t, decodeJSON(h.Call(t, callDescription,
		"iaas-api", "switch", "find", "--zone", zone), &items, true), "%s", callDescription)
	return items
}

// checkProcessConfigurationIdentity は実行設定の参照結果が、今回の実行で作成したリソースと一致することを確認します。
func checkProcessConfigurationIdentity(candidate processConfigurationItem, r *eventbusResource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name {
		return fmt.Errorf("実行設定の識別情報が一致しません (ID %s, name %q)", id, candidate.Name)
	}
	if candidate.Description == nil || *candidate.Description != r.description {
		return fmt.Errorf("実行設定の説明が一致しません (ID %s)", id)
	}
	// 宛先と宛先パラメーターも、今回設定した値と照合します。
	if r.destination != "" && candidate.Destination != r.destination {
		return fmt.Errorf("実行設定の宛先が一致しません (ID %s)", id)
	}
	if r.parameters != nil && !maps.Equal(candidate.Parameters, r.parameters) {
		return fmt.Errorf("実行設定の宛先パラメーターが一致しません (ID %s)", id)
	}
	return nil
}

// checkScheduleIdentity はスケジュールの参照結果が、今回の実行で作成したリソースと一致することを確認します。
func checkScheduleIdentity(candidate scheduleItem, r *eventbusResource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name {
		return fmt.Errorf("スケジュールの識別情報が一致しません (ID %s, name %q)", id, candidate.Name)
	}
	if candidate.Description == nil || *candidate.Description != r.description {
		return fmt.Errorf("スケジュールの説明が一致しません (ID %s)", id)
	}
	// 参照先・開始日時・繰り返し設定も、今回設定した値と照合します。
	if r.processConfigurationID != "" {
		referenced, err := decodeResourceID(candidate.ProcessConfigurationID)
		if err != nil || referenced != r.processConfigurationID {
			return fmt.Errorf("スケジュールの実行設定の参照先が一致しません (ID %s)", id)
		}
	}
	if r.startsAt != "" {
		want, err := time.Parse(time.RFC3339, r.startsAt)
		if err != nil {
			return fmt.Errorf("期待する開始日時 %q を解析できません: %w", r.startsAt, err)
		}
		got, err := time.Parse(time.RFC3339Nano, candidate.StartsAt)
		if err != nil || got.Sub(want).Abs() > time.Second {
			return fmt.Errorf("スケジュールの開始日時が一致しません (ID %s)", id)
		}
	}
	if r.recurringStep != 0 && (candidate.RecurringStep == nil || *candidate.RecurringStep != r.recurringStep) {
		return fmt.Errorf("スケジュールの繰り返し間隔が一致しません (ID %s)", id)
	}
	if r.recurringUnit != "" && (candidate.RecurringUnit == nil || *candidate.RecurringUnit != r.recurringUnit) {
		return fmt.Errorf("スケジュールの繰り返し単位が一致しません (ID %s)", id)
	}
	return nil
}

// checkTriggerIdentity はトリガーの参照結果が、今回の実行で作成したリソースと一致することを確認します。
func checkTriggerIdentity(candidate triggerItem, r *eventbusResource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name {
		return fmt.Errorf("トリガーの識別情報が一致しません (ID %s, name %q)", id, candidate.Name)
	}
	if candidate.Description == nil || *candidate.Description != r.description {
		return fmt.Errorf("トリガーの説明が一致しません (ID %s)", id)
	}
	// 送信元・イベントタイプ・条件・参照先も、今回設定した値と照合します。
	if r.source != "" && candidate.Source != r.source {
		return fmt.Errorf("トリガーの送信元が一致しません (ID %s)", id)
	}
	if r.types != nil && !slices.Equal(candidate.Types, r.types) {
		return fmt.Errorf("トリガーのイベントタイプが一致しません (ID %s)", id)
	}
	if r.conditions != nil && !slices.EqualFunc(candidate.Conditions, r.conditions, equalTriggerConditions) {
		return fmt.Errorf("トリガーの条件が一致しません (ID %s)", id)
	}
	if r.processConfigurationID != "" {
		referenced, err := decodeResourceID(candidate.ProcessConfigurationID)
		if err != nil || referenced != r.processConfigurationID {
			return fmt.Errorf("トリガーの実行設定の参照先が一致しません (ID %s)", id)
		}
	}
	return nil
}

// equalTriggerConditions はトリガーの条件のスライス要素を、Key・Op・Values で比較します。
// triggerConditionItem は Values (スライス) を含むため == で比較できません。
func equalTriggerConditions(a, b triggerConditionItem) bool {
	return a.Key == b.Key && a.Op == b.Op && slices.Equal(a.Values, b.Values)
}

// checkQueueIdentity は SimpleMQ キューの参照結果が、今回の実行で作成したリソースと一致することを確認します。
func checkQueueIdentity(candidate queueItem, r *eventbusResource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name || candidate.Description != r.description {
		return fmt.Errorf("SimpleMQ キューの識別情報が一致しません (ID %s, name %q)", id, candidate.Name)
	}
	if candidate.Status.QueueName != r.name {
		return fmt.Errorf("SimpleMQ キュー名が一致しません (ID %s)", id)
	}
	return nil
}

// checkSwitchIdentity はスイッチの参照結果が、今回の実行で作成したリソースと一致することを確認します。
func checkSwitchIdentity(candidate switchItem, r *eventbusResource, id string) error {
	candidateID, err := decodeResourceID(candidate.ID)
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name || candidate.Description != r.description {
		return fmt.Errorf("スイッチの識別情報が一致しません (ID %s, name %q)", id, candidate.Name)
	}
	return nil
}

// requireNoNameCollision は作成前に同名リソースが存在しないことを確認します。
// 一覧の取得に失敗した場合はテストを停止し、不在として扱いません。
func requireNoNameCollision(t *testing.T, h *helper.Helper, resources ...*eventbusResource) {
	t.Helper()

	for _, r := range resources {
		var names []string
		callDescription := fmt.Sprintf("作成前に同名%sが存在しないことを確認する", kindName(r.kind))
		switch r.kind {
		case "queue":
			for _, item := range listSimpleMQQueues(t, h, callDescription) {
				names = append(names, item.Name)
			}
		case "process-configuration":
			for _, item := range listProcessConfigurations(t, h, callDescription) {
				names = append(names, item.Name)
			}
		case "schedule":
			for _, item := range listSchedules(t, h, callDescription) {
				names = append(names, item.Name)
			}
		case "trigger":
			for _, item := range listTriggers(t, h, callDescription) {
				names = append(names, item.Name)
			}
		case "switch":
			for _, item := range listSwitches(t, h, callDescription, r.zone) {
				names = append(names, item.Name)
			}
		default:
			t.Fatalf("unknown resource kind %q", r.kind)
		}
		for _, name := range names {
			require.NotEqual(t, r.name, name,
				"%s name %q already exists; refusing to modify it", kindName(r.kind), r.name)
		}
	}
}

// cleanupEventbusResource は1件のリソースを後片付けします。
// 作成 API を呼び出していないリソースは変更しません。作成結果から ID を確定できて
// いない対象は同名・同説明でも今回の作成物と断定できないため、自動削除せずに
// 候補を報告して手動確認を求めます。
// 削除直前に今回の作成物と識別情報を再照合し、一致しない場合は削除しません。
func cleanupEventbusResource(t *testing.T, h *helper.Helper, r *eventbusResource) {
	t.Helper()

	label := kindName(r.kind)
	if !r.attempted || r.deleted {
		return
	}
	id := r.id
	if id == "" {
		// 作成 API がエラーを返しても、サーバー側で作成済みの可能性があります。
		// 削除は行わず、同名・同説明の候補を報告して手動確認を求めます。
		if candidate := findResourceIDByName(t, h, r); candidate != "" {
			t.Errorf("cleanup refused: %s %s の ID が作成結果で確定していないため自動削除しません。同名・同説明の %s を手動で確認してください", label, r.name, candidate)
		}
		return
	}

	// 削除前に今回の作成物と識別情報を再照合する
	switch r.kind {
	case "queue":
		current := readSimpleMQQueue(t, h, fmt.Sprintf("削除前に%sを再照合する", label), id)
		if err := checkQueueIdentity(current, r, id); err != nil {
			t.Errorf("cleanup refused: %v", err)
			return
		}
	case "process-configuration":
		current := readProcessConfiguration(t, h, fmt.Sprintf("削除前に%sを再照合する", label), id)
		if err := checkProcessConfigurationIdentity(current, r, id); err != nil {
			t.Errorf("cleanup refused: %v", err)
			return
		}
	case "schedule":
		current := readSchedule(t, h, fmt.Sprintf("削除前に%sを再照合する", label), id)
		if err := checkScheduleIdentity(current, r, id); err != nil {
			t.Errorf("cleanup refused: %v", err)
			return
		}
	case "trigger":
		current := readTrigger(t, h, fmt.Sprintf("削除前に%sを再照合する", label), id)
		if err := checkTriggerIdentity(current, r, id); err != nil {
			t.Errorf("cleanup refused: %v", err)
			return
		}
	case "switch":
		var current switchItem
		require.NoError(t, decodeJSON(h.Call(t, fmt.Sprintf("削除前に%sを再照合する", label),
			"iaas-api", "switch", "read", "--zone", r.zone, "--id", id), &current, false),
			"read %s before cleanup", label)
		if err := checkSwitchIdentity(current, r, id); err != nil {
			t.Errorf("cleanup refused: %v", err)
			return
		}
	}

	if r.kind == "queue" {
		// メッセージが残っていても削除できるように、先にメッセージを削除します。
		h.Call(t, "キューのメッセージをすべて削除する", "simplemq-api", "clear-queue", id)
	}

	var deleteErr error
	switch r.kind {
	case "queue":
		deleteErr = callAllowingFailure(t, h, fmt.Sprintf("%sを削除する", label), "simplemq-api", "delete-queue", id)
	case "process-configuration":
		deleteErr = callAllowingFailure(t, h, fmt.Sprintf("%sを削除する", label), "eventbus", "process-configuration", "delete", id)
	case "schedule":
		deleteErr = callAllowingFailure(t, h, fmt.Sprintf("%sを削除する", label), "eventbus", "schedule", "delete", id)
	case "trigger":
		deleteErr = callAllowingFailure(t, h, fmt.Sprintf("%sを削除する", label), "eventbus", "trigger", "delete", id)
	case "switch":
		// スイッチの削除は対象が見つからない場合にエラーにします。
		deleteErr = callAllowingFailure(t, h, fmt.Sprintf("%sを削除する", label),
			"iaas-api", "switch", "delete", "--zone", r.zone, "--id", id, "--fail-if-not-found=true")
	}
	if deleteErr != nil {
		t.Errorf("delete %s: %v", label, deleteErr)
		return
	}
	r.deleted = true

	// 削除後に一覧から消失したことを確認する
	switch r.kind {
	case "queue":
		for _, item := range listSimpleMQQueues(t, h, fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label)) {
			remaining, err := item.id()
			require.NoError(t, err, "decode remaining queue ID: %v", err)
			require.NotEqual(t, id, remaining, "%s %s remains after delete", label, id)
		}
	case "process-configuration":
		for _, item := range listProcessConfigurations(t, h, fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label)) {
			remaining, err := item.id()
			require.NoError(t, err, "decode remaining process-configuration ID: %v", err)
			require.NotEqual(t, id, remaining, "%s %s remains after delete", label, id)
		}
	case "schedule":
		for _, item := range listSchedules(t, h, fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label)) {
			remaining, err := item.id()
			require.NoError(t, err, "decode remaining schedule ID: %v", err)
			require.NotEqual(t, id, remaining, "%s %s remains after delete", label, id)
		}
	case "trigger":
		for _, item := range listTriggers(t, h, fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label)) {
			remaining, err := item.id()
			require.NoError(t, err, "decode remaining trigger ID: %v", err)
			require.NotEqual(t, id, remaining, "%s %s remains after delete", label, id)
		}
	case "switch":
		for _, item := range listSwitches(t, h, fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label), r.zone) {
			remaining, err := decodeResourceID(item.ID)
			require.NoError(t, err, "decode remaining switch ID: %v", err)
			require.NotEqual(t, id, remaining, "%s %s remains after delete", label, id)
		}
	}
}

// findResourceIDByName は名前でリソースを検索し、名前と説明が完全一致する1件の
// 今回の実行の作成物の ID を返します。確定できない場合は空文字列を返します。
func findResourceIDByName(t *testing.T, h *helper.Helper, r *eventbusResource) string {
	t.Helper()

	var matches []string
	collect := func(id string, name string, description *string) {
		if name == r.name && description != nil && *description == r.description {
			matches = append(matches, id)
		}
	}
	callDescription := fmt.Sprintf("後片付け前に%sを検索する", kindName(r.kind))
	switch r.kind {
	case "queue":
		for _, item := range listSimpleMQQueues(t, h, callDescription) {
			d := item.Description
			itemID, err := item.id()
			require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
			collect(itemID, item.Name, &d)
		}
	case "process-configuration":
		for _, item := range listProcessConfigurations(t, h, callDescription) {
			itemID, err := item.id()
			require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
			collect(itemID, item.Name, item.Description)
		}
	case "schedule":
		for _, item := range listSchedules(t, h, callDescription) {
			itemID, err := item.id()
			require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
			collect(itemID, item.Name, item.Description)
		}
	case "trigger":
		for _, item := range listTriggers(t, h, callDescription) {
			itemID, err := item.id()
			require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
			collect(itemID, item.Name, item.Description)
		}
	case "switch":
		for _, item := range listSwitches(t, h, callDescription, r.zone) {
			d := item.Description
			if id, err := decodeResourceID(item.ID); err == nil {
				collect(id, item.Name, &d)
			}
		}
	default:
		t.Fatalf("unknown resource kind %q", r.kind)
	}
	if len(matches) == 0 {
		return ""
	}
	if len(matches) != 1 {
		t.Errorf("cleanup refused: %s %q identity is not unique and marked for this E2E", kindName(r.kind), r.name)
		return ""
	}
	return matches[0]
}

// writeSecretFiles は発行済みの API キーを 0600 の一時ファイルへ書き込みます。
// SimpleMQ メッセージ API 用のキーファイルと、実行設定の secret 登録用の JSON ファイルのパスを返します。
func writeSecretFiles(t *testing.T, apiKey string) (keyFile, secretFile string) {
	t.Helper()

	dir := t.TempDir()
	keyFile = filepath.Join(dir, "simplemq.key")
	if err := os.WriteFile(keyFile, []byte(apiKey), 0o600); err != nil {
		t.Fatalf("write SimpleMQ API key file: %v", err)
	}
	secretFile = filepath.Join(dir, "eventbus-secret.json")
	//nolint:gosec // 発行済みの API キーは所有者のみ読める 0600 の一時ファイルにのみ書き込まれる。
	data, err := json.Marshal(struct {
		APIKey string `json:"APIKey"`
	}{APIKey: apiKey})
	if err != nil {
		t.Fatalf("encode EventBus secret: %v", err)
	}
	if err := os.WriteFile(secretFile, data, 0o600); err != nil {
		t.Fatalf("write EventBus secret file: %v", err)
	}
	return keyFile, secretFile
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

// message は SimpleMQ から受信したメッセージです。
type message struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// decodeJSON は CLI の JSON 出力を value へ解釈します。配列の出力では items が nil にならないことを確認します。
func decodeJSON(data []byte, value any, wantArray bool) error {
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode JSON output: %w", err)
	}
	if wantArray {
		switch items := value.(type) {
		case *[]processConfigurationItem:
			if *items == nil {
				return errors.New("expected a JSON array")
			}
		case *[]scheduleItem:
			if *items == nil {
				return errors.New("expected a JSON array")
			}
		case *[]triggerItem:
			if *items == nil {
				return errors.New("expected a JSON array")
			}
		case *[]queueItem:
			if *items == nil {
				return errors.New("expected a JSON array")
			}
		case *[]switchItem:
			if *items == nil {
				return errors.New("expected a JSON array")
			}
		}
	}
	return nil
}

// callAllowingFailure は後片付けの CLI 呼び出しを行い、失敗をエラーとして返します。
func callAllowingFailure(t *testing.T, h *helper.Helper, callDescription string, args ...string) error {
	t.Helper()

	_, stderr, err := h.CallAllowingFailure(t, callDescription, args...)
	if err != nil {
		return fmt.Errorf("%v: %s", err, stderr)
	}
	return nil
}

// TestDecodeResourceID は JSON 文字列または数値としてエンコードされたリソース ID の解釈を確認する。
func TestDecodeResourceID(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		id, err := decodeResourceID(json.RawMessage(`"test-only-id"`))
		require.NoError(t, err)
		require.Equal(t, "test-only-id", id)
	})
	t.Run("number", func(t *testing.T) {
		id, err := decodeResourceID(json.RawMessage(`123456789012`))
		require.NoError(t, err)
		require.Equal(t, "123456789012", id)
	})
	t.Run("zero number", func(t *testing.T) {
		_, err := decodeResourceID(json.RawMessage(`0`))
		require.Error(t, err, "a zero IaaS ID should be treated as an empty ID")
	})
	t.Run("empty string", func(t *testing.T) {
		_, err := decodeResourceID(json.RawMessage(`""`))
		require.Error(t, err)
	})
	t.Run("missing", func(t *testing.T) {
		_, err := decodeResourceID(nil)
		require.Error(t, err)
	})
}

// TestDecodeJSONRejectsNonArray は一覧の JSON 出力が配列でないときの拒否を確認します。
func TestDecodeJSONRejectsNonArray(t *testing.T) {
	var items []processConfigurationItem
	err := decodeJSON([]byte(`null`), &items, true)
	require.Error(t, err, "a null list should not be treated as an empty list")

	err = decodeJSON([]byte(`[]`), &items, true)
	require.NoError(t, err)
	require.NotNil(t, items, "an empty JSON array should not become nil")
}

// TestParseAPIKey は rotate-api-key の応答の解釈を確認する。
func TestParseAPIKey(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		key, err := parseAPIKey([]byte(`{"APIKey":"test-only-api-key"}`))
		require.NoError(t, err)
		require.Equal(t, "test-only-api-key", key)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`{"APIKey":""}`))
		require.Error(t, err)
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`not-json`))
		require.Error(t, err)
	})
}

// TestWriteSecretFiles は秘密情報を受け取る CLI フラグ用ファイルの作成を確認する。
func TestWriteSecretFiles(t *testing.T) {
	keyFile, secretFile := writeSecretFiles(t, "test-only-api-key")

	// SimpleMQ API キーは 0600 のファイルに、鍵の値そのまま書かれる
	info, err := os.Stat(keyFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	data, err := os.ReadFile(keyFile) //nolint:gosec // テスト専用ディレクトリ内のファイルを読み込む。
	require.NoError(t, err)
	require.Equal(t, "test-only-api-key", string(data))

	// EventBus のシークレット登録用 JSON も 0600 で作成される
	secretInfo, err := os.Stat(secretFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), secretInfo.Mode().Perm())
	secretData, err := os.ReadFile(secretFile) //nolint:gosec // テスト専用ディレクトリ内のファイルを読み込む。
	require.NoError(t, err)
	require.JSONEq(t, `{"APIKey":"test-only-api-key"}`, string(secretData))
}
