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

	"github.com/stretchr/testify/require"
)

func TestProcessCreateParameters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings ProcessConfigurationSettings
		want     string
	}{
		{"simplemq", ProcessConfigurationSettings{Destination: "simplemq", QueueName: ptr("queue"), Content: ptr("body")}, `{"queue_name":"queue","content":"body"}`},
		{"simplenotification", ProcessConfigurationSettings{Destination: "simplenotification", GroupID: ptr("group"), Message: ptr("body")}, `{"group_id":"group","message":"body"}`},
		{"autoscale", ProcessConfigurationSettings{Destination: "autoscale", Action: ptr("scale_up"), ResourceID: ptr("resource")}, `{"action":"scale_up","resource_id":"resource"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{item: processItem()}
			s := &Service{process: api}
			_, err := s.CreateProcessConfiguration(t.Context(), &CreateProcessConfigurationRequest{Name: "example", Settings: tc.settings})
			require.NoError(t, err)
			require.JSONEq(t, tc.want, api.createRequest.CommonServiceItem.Settings.ProcessConfigurationSettings.Parameters)
			require.True(t, api.createRequest.CommonServiceItem.Description.IsNull())
		})
	}
}

func TestProcessUpdatePreservesParameters(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	s := &Service{process: api}
	_, err := s.UpdateProcessConfiguration(t.Context(), &UpdateProcessConfigurationRequest{ID: "123", Content: ptr("")})
	require.NoError(t, err)
	settings := api.updateRequest.CommonServiceItem.Settings.Value.ProcessConfigurationSettings
	require.JSONEq(t, `{"queue_name":"queue","content":"","future":{"number":9007199254740993}}`, settings.Parameters)
	encoded, err := json.Marshal(&api.updateRequest)
	require.NoError(t, err)
	var wire struct {
		CommonServiceItem struct{ Settings struct{ Parameters string } }
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Equal(t, settings.Parameters, wire.CommonServiceItem.Settings.Parameters)

	_, err = s.UpdateProcessConfiguration(t.Context(), &UpdateProcessConfigurationRequest{
		ID: "123", Destination: ptr("simplenotification"), GroupID: ptr("group"),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"group_id":"group","message":""}`, api.updateRequest.CommonServiceItem.Settings.Value.ProcessConfigurationSettings.Parameters)
}

func TestProcessMetadataUpdateOmitsSettings(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	api.item.Settings.ProcessConfigurationSettings.Parameters = "invalid"
	s := &Service{process: api}
	_, err := s.UpdateProcessConfiguration(t.Context(), &UpdateProcessConfigurationRequest{ID: "123", Name: ptr("new"), Description: ptr("")})
	require.NoError(t, err)
	require.False(t, api.updateRequest.CommonServiceItem.Settings.IsSet())
	require.Equal(t, "", api.updateRequest.CommonServiceItem.Description.Value)
	require.True(t, api.updateRequest.CommonServiceItem.Description.IsSet())
	require.False(t, api.updateRequest.CommonServiceItem.Description.IsNull())
}

func TestProcessInvalidInputDoesNotMutate(t *testing.T) {
	for _, settings := range []ProcessConfigurationSettings{
		{}, {Destination: "unknown"}, {Destination: "simplemq", Message: ptr("")}, {Destination: "autoscale", QueueName: ptr("")},
	} {
		api := &fakeAPI{item: processItem()}
		s := &Service{process: api}
		_, err := s.CreateProcessConfiguration(t.Context(), &CreateProcessConfigurationRequest{Name: "example", Settings: settings})
		require.Error(t, err)
		require.Zero(t, api.mutations)
	}
	for _, input := range []string{"", "null", "[]", "{", `{"queue_name":12}`, `{"content":null}`} {
		t.Run(input, func(t *testing.T) {
			api := &fakeAPI{item: processItem()}
			api.item.Settings.ProcessConfigurationSettings.Parameters = input
			s := &Service{process: api}
			_, err := s.UpdateProcessConfiguration(t.Context(), &UpdateProcessConfigurationRequest{ID: "123", Content: ptr("updated")})
			require.Error(t, err)
			require.Zero(t, api.mutations)
		})
	}
	api := &fakeAPI{item: processItem()}
	_, err := (&Service{process: api}).CreateProcessConfiguration(t.Context(), &CreateProcessConfigurationRequest{Settings: ProcessConfigurationSettings{Destination: "simplemq"}})
	require.ErrorContains(t, err, "Name")
	require.Zero(t, api.mutations)
}
