package eventbus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	sdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	eventbusmock "github.com/sacloud/sakumock/eventbus"
	"github.com/stretchr/testify/require"
)

func TestServiceWithSDKAndSakumock(t *testing.T) {
	server := eventbusmock.NewTestServer(eventbusmock.Config{})
	t.Cleanup(server.Close)
	var client saclient.Client
	require.NoError(t, client.SetEnviron([]string{
		"SAKURA_PROFILE_DIR=" + t.TempDir(),
		"SAKURA_ACCESS_TOKEN=dummy-token",
		"SAKURA_ACCESS_TOKEN_SECRET=dummy-secret",
		"SAKURA_ENDPOINTS_EVENTBUS=" + server.TestURL() + "/",
	}))
	var mu sync.Mutex
	var lastBody []byte
	require.NoError(t, client.SetWith(saclient.WithMiddleware(
		func(request *http.Request, pull func() (saclient.Middleware, bool)) (*http.Response, error) {
			if request.Body != nil {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				if err := request.Body.Close(); err != nil {
					return nil, err
				}
				request.Body = io.NopCloser(bytes.NewReader(body))
				mu.Lock()
				lastBody = body
				mu.Unlock()
			}
			next, ok := pull()
			if !ok {
				return nil, fmt.Errorf("HTTP middleware がありません")
			}
			return next(request, pull)
		},
	)))
	caller, err := sdk.NewClient(&client)
	require.NoError(t, err)
	s := New(caller)
	ctx := t.Context()
	wire := func() map[string]json.RawMessage {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		var request struct{ CommonServiceItem map[string]json.RawMessage }
		require.NoError(t, json.Unmarshal(lastBody, &request))
		return request.CommonServiceItem
	}

	process, err := s.CreateProcessConfiguration(ctx, &CreateProcessConfigurationRequest{Name: "sdk-process",
		Settings: ProcessConfigurationSettings{Destination: "simplemq", QueueName: ptr("queue"), Content: ptr("before")},
	})
	require.NoError(t, err)
	require.NotEmpty(t, process.ID)
	require.Equal(t, v1.ProviderClassEventbusprocessconfiguration, process.Provider.Class)
	require.Contains(t, string(wire()["Settings"]), `"Parameters":"`)
	list, err := s.ListProcessConfigurations(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	readProcess, err := s.ReadProcessConfiguration(ctx, &ReadRequest{ID: process.ID})
	require.NoError(t, err)
	require.Equal(t, process.ID, readProcess.ID)

	_, err = s.UpdateProcessConfiguration(ctx, &UpdateProcessConfigurationRequest{ID: process.ID, Content: ptr("after")})
	require.NoError(t, err)
	readProcess, err = s.ReadProcessConfiguration(ctx, &ReadRequest{ID: process.ID})
	require.NoError(t, err)
	require.JSONEq(t, `{"queue_name":"queue","content":"after"}`, readProcess.Settings.ProcessConfigurationSettings.Parameters)
	_, err = s.UpdateProcessConfiguration(ctx, &UpdateProcessConfigurationRequest{ID: process.ID, Description: ptr("changed")})
	require.NoError(t, err)
	require.NotContains(t, wire(), "Settings")
	require.NoError(t, s.SetProcessConfigurationSecret(ctx, &SetProcessConfigurationSecretRequest{
		ID: process.ID, Secret: v1.NewSimpleMQSecretSetSecretRequestSecret(v1.SimpleMQSecret{APIKey: "test-key"}),
	}))

	schedule, err := s.CreateSchedule(ctx, &CreateScheduleRequest{Name: "sdk-schedule", Settings: ScheduleSettings{
		ProcessConfigurationID: process.ID, StartsAt: time.UnixMilli(1900000000123), RecurringStep: ptr(1), RecurringUnit: ptr("day"),
	}})
	require.NoError(t, err)
	schedules, err := s.ListSchedules(ctx)
	require.NoError(t, err)
	require.Len(t, schedules, 1)
	readSchedule, err := s.ReadSchedule(ctx, &ReadRequest{ID: schedule.ID})
	require.NoError(t, err)
	require.Equal(t, process.ID, readSchedule.Settings.ScheduleSettings.ProcessConfigurationID)
	_, err = s.UpdateSchedule(ctx, &UpdateScheduleRequest{ID: schedule.ID, Crontab: ptr("0 * * * *")})
	require.NoError(t, err)
	require.Contains(t, string(wire()["Settings"]), `"StartsAt":1900000000123`)
	readSchedule, err = s.ReadSchedule(ctx, &ReadRequest{ID: schedule.ID})
	require.NoError(t, err)
	require.Equal(t, "0 * * * *", readSchedule.Settings.ScheduleSettings.Crontab.Value)
	require.False(t, readSchedule.Settings.ScheduleSettings.RecurringUnit.IsSet())
	_, err = s.UpdateSchedule(ctx, &UpdateScheduleRequest{ID: schedule.ID, RecurringStep: ptr(2), RecurringUnit: ptr("hour")})
	require.NoError(t, err)
	readSchedule, err = s.ReadSchedule(ctx, &ReadRequest{ID: schedule.ID})
	require.NoError(t, err)
	require.False(t, readSchedule.Settings.ScheduleSettings.Crontab.IsSet())
	require.NoError(t, s.DeleteSchedule(ctx, &DeleteRequest{ID: schedule.ID}))
	_, err = s.ReadSchedule(ctx, &ReadRequest{ID: schedule.ID})
	require.Error(t, err)

	trigger, err := s.CreateTrigger(ctx, &CreateTriggerRequest{Name: "sdk-trigger", Settings: TriggerSettings{
		Source: "//eventbus.sakura.ad.jp/eventlog", ProcessConfigurationID: process.ID,
		Types:      []string{"jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created"},
		Conditions: []TriggerCondition{{Key: "zone", Op: "eq", Values: []string{"is1b"}}},
	}})
	require.NoError(t, err)
	triggers, err := s.ListTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, triggers, 1)
	readTrigger, err := s.ReadTrigger(ctx, &ReadRequest{ID: trigger.ID})
	require.NoError(t, err)
	require.Equal(t, process.ID, readTrigger.Settings.TriggerSettings.ProcessConfigurationID)
	_, err = s.UpdateTrigger(ctx, &UpdateTriggerRequest{ID: trigger.ID, Source: ptr("//eventbus.sakura.ad.jp/monitoringsuite/alert")})
	require.NoError(t, err)
	readTrigger, err = s.ReadTrigger(ctx, &ReadRequest{ID: trigger.ID})
	require.NoError(t, err)
	require.Equal(t, readTrigger.Settings.TriggerSettings.Types, trigger.Settings.TriggerSettings.Types)
	require.Equal(t, readTrigger.Settings.TriggerSettings.Conditions, trigger.Settings.TriggerSettings.Conditions)
	require.NoError(t, s.DeleteTrigger(ctx, &DeleteRequest{ID: trigger.ID}))
	require.NoError(t, s.DeleteProcessConfiguration(ctx, &DeleteRequest{ID: process.ID}))
	list, err = s.ListProcessConfigurations(ctx)
	require.NoError(t, err)
	require.Empty(t, list)
}
