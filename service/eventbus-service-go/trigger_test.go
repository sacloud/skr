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

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestTriggerCreateAndReplacement(t *testing.T) {
	api := &fakeAPI{item: triggerItem()}
	s := &Service{trigger: api}
	_, err := s.CreateTrigger(t.Context(), &CreateTriggerRequest{Name: "trigger", Settings: TriggerSettings{
		Source: "//eventbus.sakura.ad.jp/eventlog", ProcessConfigurationID: "456",
	}})
	require.NoError(t, err)
	require.True(t, api.createRequest.CommonServiceItem.Settings.TriggerSettings.Types.IsNull())
	require.True(t, api.createRequest.CommonServiceItem.Settings.TriggerSettings.Conditions.IsNull())
	_, err = s.UpdateTrigger(t.Context(), &UpdateTriggerRequest{ID: "123", Types: []string{"created"},
		Conditions: []TriggerCondition{{Key: "zone", Op: "in", Values: []string{"is1a", "is1b"}}},
	})
	require.NoError(t, err)
	settings := api.updateRequest.CommonServiceItem.Settings.Value.TriggerSettings
	require.Equal(t, []string{"created"}, settings.Types.Value)
	require.Len(t, settings.Conditions.Value, 1)
	require.Equal(t, []string{"is1a", "is1b"}, settings.Conditions.Value[0].TriggerConditionIn.Values)
}

func TestTriggerPreservesOptionalArrays(t *testing.T) {
	for _, state := range []string{"missing", "null", "empty", "values"} {
		t.Run(state, func(t *testing.T) {
			api := &fakeAPI{item: triggerItem()}
			settings := &api.item.Settings.TriggerSettings
			switch state {
			case "null":
				settings.Types.SetToNull()
				settings.Conditions.SetToNull()
			case "empty":
				settings.Types.SetTo([]string{})
				settings.Conditions.SetTo([]v1.TriggerSettingsConditionsItem{})
			case "values":
				settings.Types.SetTo([]string{"original"})
				conditions, err := convertConditions([]TriggerCondition{{Key: "zone", Op: "eq", Values: []string{"is1b"}}})
				require.NoError(t, err)
				settings.Conditions = conditions
			}
			original, err := json.Marshal(settings)
			require.NoError(t, err)
			_, err = (&Service{trigger: api}).UpdateTrigger(t.Context(), &UpdateTriggerRequest{
				ID: "123", ProcessConfigurationID: ptr("456"),
			})
			require.NoError(t, err)
			merged, err := json.Marshal(&api.updateRequest.CommonServiceItem.Settings.Value.TriggerSettings)
			require.NoError(t, err)
			require.JSONEq(t, string(original), string(merged))
		})
	}
}

func TestTriggerInvalidInputDoesNotMutate(t *testing.T) {
	for _, condition := range []TriggerCondition{
		{Key: "data", Op: "eq", Values: []string{"x"}},
		{Key: "Zone", Op: "eq", Values: []string{"x"}},
		{Key: "zone", Op: "eq", Values: []string{"x", "y"}},
		{Key: "zone", Op: "in"},
		{Key: "zone", Op: "unknown", Values: []string{"x"}},
	} {
		api := &fakeAPI{item: triggerItem()}
		_, err := (&Service{trigger: api}).UpdateTrigger(t.Context(), &UpdateTriggerRequest{ID: "123", Conditions: []TriggerCondition{condition}})
		require.Error(t, err)
		require.Zero(t, api.mutations)
	}
	for _, req := range []*UpdateTriggerRequest{
		{ID: "123", Source: ptr("unknown")},
		{ID: "123", ProcessConfigurationID: ptr("")},
		{ID: "123", Types: []string{}},
		{ID: "123", Types: []string{""}},
		{ID: "123", Conditions: []TriggerCondition{}},
	} {
		api := &fakeAPI{item: triggerItem()}
		_, err := (&Service{trigger: api}).UpdateTrigger(t.Context(), req)
		require.Error(t, err)
		require.Zero(t, api.mutations)
	}
}
