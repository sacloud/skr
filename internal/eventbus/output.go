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

package eventbus

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// ProcessConfigurationOutput は実行設定の応答を宛先別の個別入力と対応づけて出力します。
type ProcessConfigurationOutput struct {
	ID          string
	Name        string
	Description *string
	Destination string
	Parameters  map[string]string
	Tags        []string
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

// ScheduleOutput はスケジュールの応答を出力します。
// StartsAt はエポックミリ秒をミリ秒精度を失わない RFC3339 形式 (UTC) の文字列へ変換します。
type ScheduleOutput struct {
	ID                     string
	Name                   string
	Description            *string
	ProcessConfigurationID string
	StartsAt               string
	RecurringStep          *int
	RecurringUnit          *string
	Crontab                *string
	Tags                   []string
	CreatedAt              time.Time
	ModifiedAt             time.Time
}

// TriggerOutput はトリガーの応答を出力します。
// Conditions は条件の sum 型を等号・包含の形へ射影します。
type TriggerOutput struct {
	ID                     string
	Name                   string
	Description            *string
	Source                 string
	Types                  []string
	Conditions             []TriggerConditionOutput
	ProcessConfigurationID string
	Tags                   []string
	CreatedAt              time.Time
	ModifiedAt             time.Time
}

// TriggerConditionOutput はトリガー条件の等号または包含を表します。
type TriggerConditionOutput struct {
	Key    string
	Op     string
	Values []string
}

// ProjectProcessConfiguration は実行設定の応答を出力用へ変換します。
// 宛先ごとの対応表にある非秘密フィールドだけを出力し、未知のキーや秘密情報の
// おそれのある値は公開しません。
func ProjectProcessConfiguration(item *v1.CommonServiceItem) (*ProcessConfigurationOutput, error) {
	if err := checkItem(item, "", v1.ProviderClassEventbusprocessconfiguration); err != nil {
		return nil, err
	}
	settings := item.Settings.ProcessConfigurationSettings
	keys, err := parameterKeys(string(settings.Destination))
	if err != nil {
		return nil, err
	}
	parameters, err := projectParameters(settings.Parameters, keys)
	if err != nil {
		return nil, err
	}
	return &ProcessConfigurationOutput{
		ID:          item.ID,
		Name:        item.Name,
		Description: optNilStringPointer(item.Description),
		Destination: string(settings.Destination),
		Parameters:  parameters,
		Tags:        normalizeTags(item.Tags),
		CreatedAt:   item.CreatedAt,
		ModifiedAt:  item.ModifiedAt,
	}, nil
}

// ProjectSchedule はスケジュールの応答を出力用へ変換します。
func ProjectSchedule(item *v1.CommonServiceItem) (*ScheduleOutput, error) {
	if err := checkItem(item, "", v1.ProviderClassEventbusschedule); err != nil {
		return nil, err
	}
	settings := item.Settings.ScheduleSettings
	millis, err := startsAtMillis(settings.StartsAt)
	if err != nil {
		return nil, err
	}
	output := &ScheduleOutput{
		ID:                     item.ID,
		Name:                   item.Name,
		Description:            optNilStringPointer(item.Description),
		ProcessConfigurationID: settings.ProcessConfigurationID,
		StartsAt:               time.UnixMilli(millis).UTC().Format(time.RFC3339Nano),
		Tags:                   normalizeTags(item.Tags),
		CreatedAt:              item.CreatedAt,
		ModifiedAt:             item.ModifiedAt,
	}
	if settings.RecurringStep.IsSet() {
		value := settings.RecurringStep.Value
		output.RecurringStep = &value
	}
	if settings.RecurringUnit.IsSet() {
		value := string(settings.RecurringUnit.Value)
		output.RecurringUnit = &value
	}
	if settings.Crontab.IsSet() {
		value := settings.Crontab.Value
		output.Crontab = &value
	}
	return output, nil
}

// ProjectTrigger はトリガーの応答を出力用へ変換します。
func ProjectTrigger(item *v1.CommonServiceItem) (*TriggerOutput, error) {
	if err := checkItem(item, "", v1.ProviderClassEventbustrigger); err != nil {
		return nil, err
	}
	settings := item.Settings.TriggerSettings
	output := &TriggerOutput{
		ID:                     item.ID,
		Name:                   item.Name,
		Description:            optNilStringPointer(item.Description),
		Source:                 settings.Source,
		ProcessConfigurationID: settings.ProcessConfigurationID,
		Tags:                   normalizeTags(item.Tags),
		CreatedAt:              item.CreatedAt,
		ModifiedAt:             item.ModifiedAt,
	}
	if settings.Types.IsSet() && !settings.Types.IsNull() {
		output.Types = settings.Types.Value
	}
	if settings.Conditions.IsSet() && !settings.Conditions.IsNull() {
		conditions := make([]TriggerConditionOutput, 0, len(settings.Conditions.Value))
		for _, condition := range settings.Conditions.Value {
			switch {
			case condition.IsTriggerConditionEq():
				conditions = append(conditions, TriggerConditionOutput{
					Key:    condition.TriggerConditionEq.Key,
					Op:     string(condition.TriggerConditionEq.Op),
					Values: condition.TriggerConditionEq.Values,
				})
			case condition.IsTriggerConditionIn():
				conditions = append(conditions, TriggerConditionOutput{
					Key:    condition.TriggerConditionIn.Key,
					Op:     string(condition.TriggerConditionIn.Op),
					Values: condition.TriggerConditionIn.Values,
				})
			default:
				return nil, fmt.Errorf("応答の Conditions が対応していない条件です")
			}
		}
		output.Conditions = conditions
	}
	return output, nil
}

// projectParameters は Parameters の JSON 文字列をオブジェクトへ変換し、
// 指定された非秘密フィールドだけを出力します。
func projectParameters(parameters string, keys []string) (map[string]string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(parameters), &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("応答の Parameters が JSON オブジェクトではありません")
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || string(raw) == "null" {
			return nil, fmt.Errorf("応答の Parameters の %s は文字列である必要があります", key)
		}
		values[key] = value
	}
	return values, nil
}

// optNilStringPointer は Description を未設定・null 以外のときだけポインターにします。
// 未設定も null も JSON の null として出力します。
func optNilStringPointer(value v1.OptNilString) *string {
	if !value.IsSet() || value.IsNull() {
		return nil
	}
	copyed := value.Value
	return &copyed
}

// normalizeTags は Tags の null を空配列として出力します。
func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
