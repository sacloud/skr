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

func TestScheduleCreateAndModes(t *testing.T) {
	date := time.Date(2030, 1, 1, 12, 0, 0, 123000000, time.FixedZone("offset", 9*60*60))
	api := &fakeAPI{item: scheduleItem()}
	s := &Service{schedule: api}
	_, err := s.CreateSchedule(t.Context(), &CreateScheduleRequest{Name: "periodic", Settings: ScheduleSettings{
		ProcessConfigurationID: "456", StartsAt: date, RecurringStep: ptr(2), RecurringUnit: ptr("day"),
	}})
	require.NoError(t, err)
	require.Equal(t, date.UnixMilli(), api.createRequest.CommonServiceItem.Settings.ScheduleSettings.StartsAt.Int64)

	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", Crontab: ptr("0 * * * *")})
	require.NoError(t, err)
	settings := api.updateRequest.CommonServiceItem.Settings.Value.ScheduleSettings
	require.Equal(t, "0 * * * *", settings.Crontab.Value)
	require.False(t, settings.RecurringStep.IsSet())
	require.False(t, settings.RecurringUnit.IsSet())
	require.True(t, settings.StartsAt.IsInt64())
	require.EqualValues(t, 1900000000123, settings.StartsAt.Int64)
	api.item.Settings = api.updateRequest.CommonServiceItem.Settings.Value

	api.mutations = 0
	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", RecurringStep: ptr(4)})
	require.Error(t, err)
	require.Zero(t, api.mutations)
	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", RecurringStep: ptr(4), RecurringUnit: ptr("hour")})
	require.NoError(t, err)
	require.False(t, api.updateRequest.CommonServiceItem.Settings.Value.ScheduleSettings.Crontab.IsSet())
	require.Equal(t, 4, api.updateRequest.CommonServiceItem.Settings.Value.ScheduleSettings.RecurringStep.Value)
}

func TestSchedulePartialUpdateAndWireTimestamp(t *testing.T) {
	api := &fakeAPI{item: scheduleItem()}
	s := &Service{schedule: api}
	_, err := s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", RecurringStep: ptr(5)})
	require.NoError(t, err)
	settings := api.updateRequest.CommonServiceItem.Settings.Value.ScheduleSettings
	require.Equal(t, v1.ScheduleSettingsRecurringUnitHour, settings.RecurringUnit.Value)
	require.Equal(t, 5, settings.RecurringStep.Value)
	data, err := json.Marshal(&api.updateRequest)
	require.NoError(t, err)
	require.Contains(t, string(data), `"StartsAt":1900000000123`)

	api.item.Settings.ScheduleSettings.StartsAt = v1.NewStringScheduleSettingsStartsAt("invalid")
	api.mutations = 0
	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", StartsAt: ptr(time.Now())})
	require.Error(t, err)
	require.Zero(t, api.mutations)
	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{ID: "123", Name: ptr("name")})
	require.NoError(t, err)
	require.False(t, api.updateRequest.CommonServiceItem.Settings.IsSet())
}

func TestScheduleRejectsIncompleteModes(t *testing.T) {
	startsAt := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, input := range []ScheduleSettings{
		{},
		{ProcessConfigurationID: "456", StartsAt: startsAt},
		{ProcessConfigurationID: "456", StartsAt: startsAt, RecurringStep: ptr(1)},
		{ProcessConfigurationID: "456", StartsAt: startsAt, RecurringUnit: ptr("min")},
		{ProcessConfigurationID: "456", StartsAt: startsAt, RecurringStep: ptr(1), RecurringUnit: ptr("week")},
		{ProcessConfigurationID: "456", StartsAt: startsAt, Crontab: ptr(""), RecurringStep: ptr(1), RecurringUnit: ptr("min")},
		{ProcessConfigurationID: "456", StartsAt: startsAt, Crontab: ptr("")},
	} {
		api := &fakeAPI{item: scheduleItem()}
		_, err := (&Service{schedule: api}).CreateSchedule(t.Context(), &CreateScheduleRequest{Name: "example", Settings: input})
		require.Error(t, err)
		require.Zero(t, api.mutations)
	}
}

func TestScheduleRejectsSubMillisecondStartsAt(t *testing.T) {
	// API は StartsAt をエポックミリ秒で保持するため、ミリ秒未満の小数秒は
	// 切り捨てずにエラーにします。
	api := &fakeAPI{item: scheduleItem()}
	s := &Service{schedule: api}
	_, err := s.CreateSchedule(t.Context(), &CreateScheduleRequest{Name: "example", Settings: ScheduleSettings{
		ProcessConfigurationID: "456", StartsAt: time.Date(2030, 1, 1, 12, 0, 0, 123456789, time.UTC),
		RecurringStep: ptr(2), RecurringUnit: ptr("day"),
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ミリ秒")
	require.Zero(t, api.mutations)

	_, err = s.UpdateSchedule(t.Context(), &UpdateScheduleRequest{
		ID: "123", StartsAt: ptr(time.Date(2030, 1, 1, 12, 0, 0, 123456789, time.UTC)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ミリ秒")
	require.Zero(t, api.mutations)
}
