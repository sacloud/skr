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
	"testing"
	"time"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestProjectProcessConfiguration(t *testing.T) {
	output, err := ProjectProcessConfiguration(processItem())
	require.NoError(t, err)
	require.Equal(t, "123", output.ID)
	require.Equal(t, "simplemq", output.Destination)
	require.Equal(t, map[string]string{"queue_name": "queue", "content": "old"}, output.Parameters)
	require.Nil(t, output.Description, "未設定の Description は null で出力します")
	require.Equal(t, []string{}, output.Tags, "Tags の null は空配列で出力します")

	// 未知のキーは公開しません。
	require.NotContains(t, output.Parameters, "future")

	data, err := json.Marshal(output)
	require.NoError(t, err)
	require.Contains(t, string(data), `"queue_name":"queue"`)
	require.Contains(t, string(data), `"Description":null`)
}

func TestProjectProcessConfigurationDescriptionAndDestination(t *testing.T) {
	item := processItem()
	item.Description = v1.NewOptNilString("説明")
	output, err := ProjectProcessConfiguration(item)
	require.NoError(t, err)
	require.NotNil(t, output.Description)
	require.Equal(t, "説明", *output.Description)

	// 宛先が変わっても既存 Parameters は出力対象のキーだけを射影します。
	item.Settings.ProcessConfigurationSettings.Destination = v1.ProcessConfigurationSettingsDestinationSimplenotification
	output, err = ProjectProcessConfiguration(item)
	require.NoError(t, err)
	require.Equal(t, "simplenotification", output.Destination)
	require.Equal(t, map[string]string{}, output.Parameters)
}

func TestProjectProcessConfigurationRejectsInvalidItems(t *testing.T) {
	for _, test := range []struct {
		name string
		item *v1.CommonServiceItem
		want string
	}{
		{name: "nil", item: nil, want: "一致しません"},
		{name: "empty ID", item: &v1.CommonServiceItem{Provider: v1.Provider{Class: v1.ProviderClassEventbusprocessconfiguration}}, want: "一致しません"},
		{name: "class mismatch", item: scheduleItem(), want: "一致しません"},
		{name: "settings mismatch", item: &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbusprocessconfiguration}}, want: "Settings の種別"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectProcessConfiguration(test.item)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestProjectProcessConfigurationRejectsInvalidParameters(t *testing.T) {
	for _, test := range []struct {
		name       string
		parameters string
		want       string
	}{
		{name: "not object", parameters: `[]`, want: "JSON オブジェクトではありません"},
		{name: "not JSON", parameters: `oops`, want: "JSON オブジェクトではありません"},
		{name: "non string known field", parameters: `{"queue_name":1,"content":"x"}`, want: "queue_name は文字列"},
		{name: "null known field", parameters: `{"queue_name":null,"content":"x"}`, want: "queue_name は文字列"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := processItem()
			item.Settings.ProcessConfigurationSettings.Parameters = test.parameters
			_, err := ProjectProcessConfiguration(item)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}

	item := processItem()
	item.Settings.ProcessConfigurationSettings.Destination = "unknown"
	_, err := ProjectProcessConfiguration(item)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Destination は")
}

func TestProjectSchedule(t *testing.T) {
	output, err := ProjectSchedule(scheduleItem())
	require.NoError(t, err)
	require.Equal(t, "123", output.ID)
	require.Equal(t, "456", output.ProcessConfigurationID)
	require.Equal(t, "2030-03-17T17:46:40.123Z", output.StartsAt)
	require.NotNil(t, output.RecurringStep)
	require.Equal(t, 3, *output.RecurringStep)
	require.NotNil(t, output.RecurringUnit)
	require.Equal(t, "hour", *output.RecurringUnit)
	require.Nil(t, output.Crontab)
	require.Equal(t, []string{}, output.Tags)
}

func TestProjectScheduleCrontabMode(t *testing.T) {
	item := &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbusschedule},
		Settings: v1.NewScheduleSettingsSettings(v1.ScheduleSettings{
			ProcessConfigurationID: "456",
			StartsAt:               v1.NewInt64ScheduleSettingsStartsAt(1893456000000),
			Crontab:                v1.NewOptString("0 9 * * *"),
		})}
	output, err := ProjectSchedule(item)
	require.NoError(t, err)
	require.Equal(t, "2030-01-01T00:00:00Z", output.StartsAt)
	require.NotNil(t, output.Crontab)
	require.Equal(t, "0 9 * * *", *output.Crontab)
	require.Nil(t, output.RecurringStep)
	require.Nil(t, output.RecurringUnit)
}

func TestProjectScheduleRejectsInvalidStartsAt(t *testing.T) {
	item := scheduleItem()
	item.Settings.ScheduleSettings.StartsAt = v1.NewStringScheduleSettingsStartsAt("not-a-number")
	_, err := ProjectSchedule(item)
	require.Error(t, err)
	require.Contains(t, err.Error(), "StartsAt")
}

func TestProjectTrigger(t *testing.T) {
	output, err := ProjectTrigger(triggerItem())
	require.NoError(t, err)
	require.Equal(t, "123", output.ID)
	require.Equal(t, "//eventbus.sakura.ad.jp/eventlog", output.Source)
	require.Nil(t, output.Types, "未設定の Types は null で出力します")
	require.Nil(t, output.Conditions, "未設定の Conditions は null で出力します")
	require.Equal(t, "456", output.ProcessConfigurationID)

	item := triggerItem()
	item.Settings.TriggerSettings.Types = v1.NewOptNilStringArray([]string{"zone.updated"})
	item.Settings.TriggerSettings.Conditions = v1.NewOptNilTriggerSettingsConditionsItemArray([]v1.TriggerSettingsConditionsItem{
		v1.NewTriggerConditionEqTriggerSettingsConditionsItem(v1.TriggerConditionEq{
			Key: "zone", Op: v1.TriggerConditionEqOpEq, Values: []string{"is1b"},
		}),
		v1.NewTriggerConditionInTriggerSettingsConditionsItem(v1.TriggerConditionIn{
			Key: "host", Op: v1.TriggerConditionInOpIn, Values: []string{"www1", "www2"},
		}),
	})
	output, err = ProjectTrigger(item)
	require.NoError(t, err)
	require.Equal(t, []string{"zone.updated"}, output.Types)
	require.Equal(t, []TriggerConditionOutput{
		{Key: "zone", Op: "eq", Values: []string{"is1b"}},
		{Key: "host", Op: "in", Values: []string{"www1", "www2"}},
	}, output.Conditions)

	data, err := json.Marshal(output)
	require.NoError(t, err)
	require.Contains(t, string(data), `"Key":"zone"`)
	require.Contains(t, string(data), `"Op":"eq"`)
	require.Contains(t, string(data), `"Values":["is1b"]`)
}

func TestProjectTriggerNullArrays(t *testing.T) {
	item := triggerItem()
	item.Settings.TriggerSettings.Types = v1.NewOptNilStringArray([]string{})
	item.Settings.TriggerSettings.Conditions = v1.NewOptNilTriggerSettingsConditionsItemArray([]v1.TriggerSettingsConditionsItem{})
	output, err := ProjectTrigger(item)
	require.NoError(t, err)
	require.Equal(t, []string{}, output.Types)
	require.Equal(t, []TriggerConditionOutput{}, output.Conditions)
}

func TestProjectTriggerRejectsInvalidItems(t *testing.T) {
	for _, test := range []struct {
		name string
		item *v1.CommonServiceItem
		want string
	}{
		{name: "nil", item: nil, want: "一致しません"},
		{name: "class mismatch", item: processItem(), want: "一致しません"},
		{name: "settings mismatch", item: &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbustrigger}}, want: "Settings の種別"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectTrigger(test.item)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestOutputTimestampsArePreserved(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	item := processItem()
	item.CreatedAt = created
	item.ModifiedAt = modified
	output, err := ProjectProcessConfiguration(item)
	require.NoError(t, err)
	require.Equal(t, created, output.CreatedAt)
	require.Equal(t, modified, output.ModifiedAt)
}
