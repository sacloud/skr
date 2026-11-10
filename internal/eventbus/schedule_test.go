package eventbus

import (
	"encoding/json"
	"testing"
	"time"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestScheduleCreateAndModes(t *testing.T) {
	date := time.Date(2030, 1, 1, 12, 0, 0, 123456789, time.FixedZone("offset", 9*60*60))
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
	for _, input := range []ScheduleSettings{
		{},
		{ProcessConfigurationID: "456", StartsAt: time.Now()},
		{ProcessConfigurationID: "456", StartsAt: time.Now(), RecurringStep: ptr(1)},
		{ProcessConfigurationID: "456", StartsAt: time.Now(), RecurringUnit: ptr("min")},
		{ProcessConfigurationID: "456", StartsAt: time.Now(), RecurringStep: ptr(1), RecurringUnit: ptr("week")},
		{ProcessConfigurationID: "456", StartsAt: time.Now(), Crontab: ptr(""), RecurringStep: ptr(1), RecurringUnit: ptr("min")},
		{ProcessConfigurationID: "456", StartsAt: time.Now(), Crontab: ptr("")},
	} {
		api := &fakeAPI{item: scheduleItem()}
		_, err := (&Service{schedule: api}).CreateSchedule(t.Context(), &CreateScheduleRequest{Name: "example", Settings: input})
		require.Error(t, err)
		require.Zero(t, api.mutations)
	}
}
