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
	"strconv"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// configurationItem はプロセス設定の読み込み結果とその Settings です。
type configurationItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Settings    struct {
		Destination string `json:"Destination"`
		Parameters  string `json:"Parameters"`
	} `json:"Settings"`
}

func (i configurationItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// triggerItem はトリガーの読み込み結果とその Settings です。
type triggerItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Settings    struct {
		Source     string   `json:"Source"`
		Types      []string `json:"Types"`
		Conditions []struct {
			Key    string   `json:"Key"`
			Op     string   `json:"Op"`
			Values []string `json:"Values"`
		} `json:"Conditions"`
		ProcessConfigurationID json.RawMessage `json:"ProcessConfigurationID"`
	} `json:"Settings"`
}

func (i triggerItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// simplemqParameters は SimpleMQ 実行先の Parameters JSON 文字列を返します。
func simplemqParameters(queueName, content string) (string, error) {
	parameters, err := json.Marshal(struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}{QueueName: queueName, Content: content})
	if err != nil {
		return "", fmt.Errorf("encode SimpleMQ parameters: %w", err)
	}
	return string(parameters), nil
}

// updateConfigurationAndVerify はプロセス設定の Parameters を更新後のメッセージ内容で更新し、
// 参照で反映を確認します。更新後のメッセージ内容を返します。
// その後のメッセージ待ちで、ライブ API がイベント発火前に更新を適用したことを確認します。
func updateConfigurationAndVerify(t *testing.T, h *helper.Helper, config, queue *eventbusResource, expected string) string {
	t.Helper()

	updatedExpected := expected + "UPDATED"
	parameters, err := simplemqParameters(queue.name, updatedExpected)
	require.NoError(t, err, "encode updated SimpleMQ parameters: %v", err)
	updateRequest := h.WriteRequestJSONFile(t, "update-process-configuration", map[string]any{
		"CommonServiceItem": map[string]any{
			"Settings": map[string]string{
				"Destination": "simplemq",
				"Parameters":  parameters,
			},
		},
	})
	h.Call(t, "プロセス設定を更新する",
		"eventbus-api", "process-configuration", "update", config.id, "--request", "@"+updateRequest)

	updated, err := lookupTypedItem[configurationItem](t, h, "更新後のプロセス設定を参照する", "process-configuration", config.id)
	require.NoError(t, err, "read updated process-configuration: %v", err)
	require.NoError(t, checkUpdatedConfiguration(updated, config, updatedExpected),
		"updated process-configuration check failed")
	return updatedExpected
}

// updateTriggerAndVerify はトリガーの説明を更新して参照で反映を確認した後、
// 元の説明に戻します。戻した説明も参照で確認します。
func updateTriggerAndVerify(t *testing.T, h *helper.Helper, trigger *eventbusResource, configID string) {
	t.Helper()

	updatedDescription := trigger.description + " (updated)"
	updateTriggerDescription(t, h, "トリガーの説明を更新する", trigger.id, updatedDescription)
	updated, err := lookupTypedItem[triggerItem](t, h, "更新後のトリガーを参照する", "trigger", trigger.id)
	require.NoError(t, err, "read updated trigger: %v", err)
	require.NoError(t, checkTriggerIdentity(updated, trigger, configID, updatedDescription),
		"updated trigger check failed")

	updateTriggerDescription(t, h, "トリガーの説明を元に戻す", trigger.id, trigger.description)
	restored, err := lookupTypedItem[triggerItem](t, h, "元に戻ったトリガーを参照する", "trigger", trigger.id)
	require.NoError(t, err, "read restored trigger: %v", err)
	require.NoError(t, checkTriggerIdentity(restored, trigger, configID, trigger.description),
		"restored trigger check failed")
}

// updateTriggerDescription はトリガーの説明を更新します。
func updateTriggerDescription(t *testing.T, h *helper.Helper, description, id, newDescription string) {
	t.Helper()

	request := h.WriteRequestJSONFile(t, "update-trigger-description", map[string]any{
		"CommonServiceItem": map[string]any{
			"Description": newDescription,
		},
	})
	h.Call(t, description, "eventbus-api", "trigger", "update", id, "--request", "@"+request)
}

// checkUpdatedConfiguration は更新後のプロセス設定が、
// 今回の実行で作成した設定と、更新後のメッセージ内容で一致することを確認します。
func checkUpdatedConfiguration(updated configurationItem, r *eventbusResource, wantContent string) error {
	id, err := updated.id()
	if err != nil {
		return err
	}
	if id != r.id || updated.Name != r.name || updated.Description != r.description {
		return fmt.Errorf("updated process-configuration identity mismatch for ID %s", r.id)
	}
	if updated.Settings.Destination != "simplemq" {
		return fmt.Errorf("updated process-configuration Destination mismatch for ID %s: %q", r.id, updated.Settings.Destination)
	}
	var parameters struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(updated.Settings.Parameters), &parameters); err != nil {
		return fmt.Errorf("updated process-configuration Parameters are not valid JSON for ID %s: %w", r.id, err)
	}
	if parameters.QueueName != r.name || parameters.Content != wantContent {
		return fmt.Errorf("updated process-configuration Parameters mismatch for ID %s", r.id)
	}
	return nil
}

// checkTriggerIdentity はトリガーの識別情報と設定が、
// 今回の実行で作成した内容と一致することを確認します。
func checkTriggerIdentity(read triggerItem, r *eventbusResource, configID, wantDescription string) error {
	id, err := read.id()
	if err != nil {
		return err
	}
	if id != r.id || read.Name != r.name || read.Description != wantDescription {
		return fmt.Errorf("trigger identity mismatch for ID %s", r.id)
	}
	if read.Settings.Source != eventSource {
		return fmt.Errorf("trigger Source mismatch for ID %s: %q", r.id, read.Settings.Source)
	}
	if len(read.Settings.Types) != 1 || read.Settings.Types[0] != switchCreatedType {
		return fmt.Errorf("trigger Types mismatch for ID %s: %v", r.id, read.Settings.Types)
	}
	if len(read.Settings.Conditions) != 1 ||
		read.Settings.Conditions[0].Key != "zone" ||
		read.Settings.Conditions[0].Op != "eq" ||
		len(read.Settings.Conditions[0].Values) != 1 ||
		read.Settings.Conditions[0].Values[0] != zone {
		return fmt.Errorf("trigger Conditions mismatch for ID %s: %+v", r.id, read.Settings.Conditions)
	}
	configReference, err := decodeResourceID(read.Settings.ProcessConfigurationID)
	if err != nil || configReference != configID {
		return fmt.Errorf("trigger ProcessConfigurationID mismatch for ID %s", r.id)
	}
	return nil
}

// TestCheckUpdatedConfiguration は更新後のプロセス設定の照合を確認する。
func TestCheckUpdatedConfiguration(t *testing.T) {
	r := &eventbusResource{
		kind:        "process-configuration",
		name:        "test-only-name",
		description: "test-only-description",
		id:          "test-only-id",
	}
	buildItem := func(t *testing.T, parameters string) configurationItem {
		t.Helper()

		var item configurationItem
		data := fmt.Sprintf(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Settings":{"Destination":"simplemq","Parameters":%s}}`, strconv.Quote(parameters))
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		return item
	}

	t.Run("ok", func(t *testing.T) {
		item := buildItem(t, `{"queue_name":"test-only-name","content":"test-only-content"}`)
		require.NoError(t, checkUpdatedConfiguration(item, r, "test-only-content"))
	})
	t.Run("identity mismatch", func(t *testing.T) {
		item := buildItem(t, `{"queue_name":"test-only-name","content":"test-only-content"}`)
		require.Error(t, checkUpdatedConfiguration(item, r, "other-content"))
	})
	t.Run("destination mismatch", func(t *testing.T) {
		var item configurationItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Settings":{"Destination":"other","Parameters":"{}"}}`), &item))
		require.Error(t, checkUpdatedConfiguration(item, r, "test-only-content"))
	})
	t.Run("invalid parameters", func(t *testing.T) {
		item := buildItem(t, `not-json`)
		require.Error(t, checkUpdatedConfiguration(item, r, "test-only-content"))
	})
	t.Run("parameters mismatch", func(t *testing.T) {
		item := buildItem(t, `{"queue_name":"other-queue","content":"test-only-content"}`)
		require.Error(t, checkUpdatedConfiguration(item, r, "test-only-content"))
	})
}

// TestCheckTriggerIdentity はトリガーの識別情報と設定の照合を確認する。
func TestCheckTriggerIdentity(t *testing.T) {
	r := &eventbusResource{
		kind:        "trigger",
		name:        "test-only-name",
		description: "test-only-description",
		id:          "test-only-id",
	}
	configID := "test-only-config-id"
	ok := fmt.Sprintf(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Settings":{"Source":%q,"Types":[%q],"Conditions":[{"Key":"zone","Op":"eq","Values":[%q]}],"ProcessConfigurationID":%q}}`,
		eventSource, switchCreatedType, zone, configID)

	t.Run("ok", func(t *testing.T) {
		var item triggerItem
		require.NoError(t, json.Unmarshal([]byte(ok), &item))
		require.NoError(t, checkTriggerIdentity(item, r, configID, r.description))
	})
	t.Run("description mismatch", func(t *testing.T) {
		var item triggerItem
		require.NoError(t, json.Unmarshal([]byte(ok), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, "other-description"))
	})
	t.Run("source mismatch", func(t *testing.T) {
		var item triggerItem
		data := strings.Replace(ok, eventSource, "//other.example.com/eventlog", 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, r.description))
	})
	t.Run("types mismatch", func(t *testing.T) {
		var item triggerItem
		data := strings.Replace(ok, switchCreatedType, "other.type", 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, r.description))
	})
	t.Run("conditions mismatch", func(t *testing.T) {
		var item triggerItem
		data := strings.Replace(ok, `"Op":"eq"`, `"Op":"neq"`, 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, r.description))
	})
	t.Run("process configuration mismatch", func(t *testing.T) {
		var item triggerItem
		data := strings.Replace(ok, configID, "other-config-id", 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, r.description))
	})
	t.Run("invalid process configuration", func(t *testing.T) {
		var item triggerItem
		data := strings.Replace(ok, strconv.Quote(configID), `{"bad":1}`, 1)
		require.NoError(t, json.Unmarshal([]byte(data), &item))
		require.Error(t, checkTriggerIdentity(item, r, configID, r.description))
	})
}
