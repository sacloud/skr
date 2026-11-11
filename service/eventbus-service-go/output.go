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

// ProcessConfigurationOutput は実行設定の出力です。
// ProjectProcessConfiguration が API の応答から変換します。
type ProcessConfigurationOutput struct {
	ID          string
	Name        string
	Description *string
	// Destination はジョブの宛先です。simplenotification、simplemq、autoscale の
	// いずれかです。
	Destination string
	// Parameters は宛先対応表にある非秘密フィールドだけを含みます。
	// simplenotification は group_id と message、simplemq は queue_name と content、
	// autoscale は action と resource_id です。対応表にない既存のキーは公開しません。
	Parameters map[string]string
	Tags       []string
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// ScheduleOutput はスケジュールの出力です。
// ProjectSchedule が API の応答から変換します。
type ScheduleOutput struct {
	ID          string
	Name        string
	Description *string
	// ProcessConfigurationID は実行する実行設定の ID です。
	ProcessConfigurationID string
	// StartsAt は実行を開始する日時です。API はエポックミリ秒で保持するため、
	// ミリ秒精度の UTC です。
	StartsAt      time.Time
	RecurringStep *int
	RecurringUnit *string
	Crontab       *string
	Tags          []string
	CreatedAt     time.Time
	ModifiedAt    time.Time
}

// TriggerOutput はトリガーの出力です。
// ProjectTrigger が API の応答から変換します。
type TriggerOutput struct {
	ID          string
	Name        string
	Description *string
	// Source は送信元イベントの URI です。
	Source string
	// Types は対象のイベントタイプです。未設定または null のときは nil になり、
	// それ以外は指定した値全体です。
	Types      []string
	Conditions []TriggerConditionOutput
	// ProcessConfigurationID は実行する実行設定の ID です。
	ProcessConfigurationID string
	Tags                   []string
	CreatedAt              time.Time
	ModifiedAt             time.Time
}

// TriggerConditionOutput はトリガー条件の出力です。
// API の条件の sum 型を等号 (eq) または包含 (in) の形へ射影します。
// 対応していない種別を API が返した場合は ProjectTrigger がエラーを返します。
type TriggerConditionOutput struct {
	Key    string
	Op     string
	Values []string
}

// ProjectProcessConfiguration は実行設定の応答を出力用へ変換します。
//
// 応答の ID・リソース種別・Settings の種別を検証し、宛先対応表にある非秘密フィールド
// だけを出力します。未知のキーや秘密情報のおそれのある値は公開しません。ID が空、
// 種別が一致しない、Parameters が JSON オブジェクトでない、宛先が不明な場合は
// エラーを返します。
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
//
// 応答の ID・リソース種別・Settings の種別を検証します。ID が空、種別が一致しない、
// StartsAt をエポックミリ秒として解釈できない場合はエラーを返します。
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
		StartsAt:               time.UnixMilli(millis).UTC(),
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
//
// 応答の ID・リソース種別・Settings の種別を検証します。ID が空、種別が一致しない、
// Conditions に対応していない条件種別が含まれる場合はエラーを返します。Types と
// Conditions が未設定または null の場合は nil になります。
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
	copied := value.Value
	return &copied
}

// normalizeTags は Tags の null を空配列として出力します。
func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
