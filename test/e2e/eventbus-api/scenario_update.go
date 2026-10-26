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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// decodeResourceID は JSON 文字列または数値としてエンコードされたリソース ID をデコードする。
// または数値の場合がある。
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
		if value == "0" {
			return "", errors.New("resource ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("resource ID has unsupported JSON type %T", value)
	}
}

// configurationItem は process-configuration の読み込み結果とその Settings である。
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

// triggerItem は trigger の読み込み結果とその Settings である。
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

// updatedExpected は、process-configuration の更新確認後に設定された SimpleMQ メッセージの内容です。
// この内容のメッセージを待つことで、
// ライブ API がイベント発火前に更新を適用したことを確認します。
func (s *scenario) updatedExpected() string {
	return s.expected + "UPDATED"
}

// updateConfigurationAndVerify は、作成した process-configuration の Settings を更新後のメッセージ内容で更新し、
// Read で反映を確認します。
// その後のメッセージ待ちで、ライブ API が更新を適用したことを確認します。
// 更新する。
func (s *scenario) updateConfigurationAndVerify(ctx context.Context) error {
	parameters, err := json.Marshal(struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}{QueueName: s.queue.name, Content: s.updatedExpected()})
	if err != nil {
		return fmt.Errorf("encode updated SimpleMQ parameters: %w", err)
	}
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Settings": map[string]string{
				"Destination": "simplemq",
				"Parameters":  string(parameters),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("encode process-configuration update: %w", err)
	}
	if _, err := s.call(ctx, "update-process-configuration", "eventbus-api", "process-configuration", "update",
		s.config.id, "--request", string(request)); err != nil {
		return err
	}
	updated, err := s.readConfiguration(ctx, "read-updated-process-configuration", s.config.id)
	if err != nil {
		return err
	}
	if err := checkUpdatedConfiguration(updated, s.config, s.updatedExpected()); err != nil {
		return err
	}
	s.expected = s.updatedExpected()
	return nil
}

// updateTriggerAndVerify は、作成した trigger の Description を更新し、
// Read で反映を確認します。その後、既存の identity チェックが trigger と一致し続けられるよう
// 元の Description に戻します。
func (s *scenario) updateTriggerAndVerify(ctx context.Context) error {
	updatedDescription := s.trigger.description + " (updated)"
	if err := s.updateTriggerDescription(ctx, "update-trigger", s.trigger.id, updatedDescription); err != nil {
		return err
	}
	updated, err := s.readTrigger(ctx, "read-updated-trigger", s.trigger.id)
	if err != nil {
		return err
	}
	if err := checkUpdatedTrigger(updated, s.trigger, s.config.id, updatedDescription); err != nil {
		return err
	}
	if err := s.updateTriggerDescription(ctx, "restore-trigger", s.trigger.id, s.trigger.description); err != nil {
		return fmt.Errorf("restore trigger Description: %w", err)
	}
	restored, err := s.readTrigger(ctx, "read-restored-trigger", s.trigger.id)
	if err != nil {
		return fmt.Errorf("read restored trigger: %w", err)
	}
	return checkTriggerIdentity(restored, s.trigger, s.config.id, s.trigger.description)
}

func (s *scenario) updateTriggerDescription(ctx context.Context, step, id, description string) error {
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Description": description,
		},
	})
	if err != nil {
		return fmt.Errorf("encode trigger update: %w", err)
	}
	_, err = s.call(ctx, step, "eventbus-api", "trigger", "update", id, "--request", string(request))
	return err
}

func (s *scenario) readConfiguration(ctx context.Context, step, id string) (configurationItem, error) {
	data, err := s.call(ctx, step, "eventbus-api", "process-configuration", "read", id)
	if err != nil {
		return configurationItem{}, err
	}
	var result configurationItem
	if err := json.Unmarshal(data, &result); err != nil {
		return configurationItem{}, fmt.Errorf("%s: decode process-configuration: %w", step, err)
	}
	return result, nil
}

func (s *scenario) readTrigger(ctx context.Context, step, id string) (triggerItem, error) {
	data, err := s.call(ctx, step, "eventbus-api", "trigger", "read", id)
	if err != nil {
		return triggerItem{}, err
	}
	var result triggerItem
	if err := json.Unmarshal(data, &result); err != nil {
		return triggerItem{}, fmt.Errorf("%s: decode trigger: %w", step, err)
	}
	return result, nil
}

func checkUpdatedConfiguration(updated configurationItem, resource resource, wantContent string) error {
	id, err := updated.id()
	if err != nil {
		return err
	}
	if id != resource.id || updated.Name != resource.name || updated.Description != resource.description {
		return fmt.Errorf("updated process-configuration identity mismatch for ID %s", resource.id)
	}
	if updated.Settings.Destination != "simplemq" {
		return fmt.Errorf("updated process-configuration Destination mismatch for ID %s: %q", resource.id, updated.Settings.Destination)
	}
	var parameters struct {
		QueueName string `json:"queue_name"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(updated.Settings.Parameters), &parameters); err != nil {
		return fmt.Errorf("updated process-configuration Parameters are not valid JSON for ID %s: %w", resource.id, err)
	}
	if parameters.QueueName != resource.name || parameters.Content != wantContent {
		return fmt.Errorf("updated process-configuration Parameters mismatch for ID %s", resource.id)
	}
	return nil
}

func checkUpdatedTrigger(updated triggerItem, resource resource, configID, wantDescription string) error {
	return checkTriggerIdentity(updated, resource, configID, wantDescription)
}

func checkTriggerIdentity(read triggerItem, resource resource, configID, wantDescription string) error {
	id, err := read.id()
	if err != nil {
		return err
	}
	if id != resource.id || read.Name != resource.name || read.Description != wantDescription {
		return fmt.Errorf("trigger identity mismatch for ID %s", resource.id)
	}
	if read.Settings.Source != eventSource {
		return fmt.Errorf("trigger Source mismatch for ID %s: %q", resource.id, read.Settings.Source)
	}
	if len(read.Settings.Types) != 1 || read.Settings.Types[0] != switchCreatedType {
		return fmt.Errorf("trigger Types mismatch for ID %s: %v", resource.id, read.Settings.Types)
	}
	if len(read.Settings.Conditions) != 1 ||
		read.Settings.Conditions[0].Key != "zone" ||
		read.Settings.Conditions[0].Op != "eq" ||
		len(read.Settings.Conditions[0].Values) != 1 ||
		read.Settings.Conditions[0].Values[0] != zone {
		return fmt.Errorf("trigger Conditions mismatch for ID %s: %+v", resource.id, read.Settings.Conditions)
	}
	configReference, err := decodeResourceID(read.Settings.ProcessConfigurationID)
	if err != nil || configReference != configID {
		return fmt.Errorf("trigger ProcessConfigurationID mismatch for ID %s", resource.id)
	}
	return nil
}
