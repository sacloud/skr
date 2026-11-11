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
	"context"
	"errors"
	"strings"
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	item          *v1.CommonServiceItem
	readErr       error
	afterRead     func()
	createRequest v1.CreateCommonServiceItemRequest
	updateRequest v1.UpdateCommonServiceItemRequest
	mutations     int
}

func (f *fakeAPI) List(context.Context) ([]v1.CommonServiceItem, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return []v1.CommonServiceItem{*f.item}, nil
}
func (f *fakeAPI) Read(context.Context, string) (*v1.CommonServiceItem, error) {
	if f.afterRead != nil {
		f.afterRead()
	}
	return f.item, f.readErr
}
func (f *fakeAPI) Create(_ context.Context, req v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	f.mutations++
	f.createRequest = req
	return f.item, nil
}
func (f *fakeAPI) Update(_ context.Context, _ string, req v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	f.mutations++
	f.updateRequest = req
	return f.item, nil
}
func (f *fakeAPI) Delete(context.Context, string) error { f.mutations++; return nil }
func (f *fakeAPI) UpdateSecret(context.Context, string, v1.SetSecretRequest) error {
	f.mutations++
	return nil
}

func ptr[T any](v T) *T { return &v }

func processItem() *v1.CommonServiceItem {
	return &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbusprocessconfiguration},
		Settings: v1.NewProcessConfigurationSettingsSettings(v1.ProcessConfigurationSettings{
			Destination: v1.ProcessConfigurationSettingsDestinationSimplemq,
			Parameters:  `{"queue_name":"queue","content":"old","future":{"number":9007199254740993}}`,
		})}
}

func scheduleItem() *v1.CommonServiceItem {
	return &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbusschedule},
		Settings: v1.NewScheduleSettingsSettings(v1.ScheduleSettings{
			ProcessConfigurationID: "456", StartsAt: v1.NewStringScheduleSettingsStartsAt("1900000000123"),
			RecurringStep: v1.NewOptInt(3), RecurringUnit: v1.NewOptScheduleSettingsRecurringUnit(v1.ScheduleSettingsRecurringUnitHour),
		})}
}

func triggerItem() *v1.CommonServiceItem {
	return &v1.CommonServiceItem{ID: "123", Provider: v1.Provider{Class: v1.ProviderClassEventbustrigger},
		Settings: v1.NewTriggerSettingsSettings(v1.TriggerSettings{
			Source: "//eventbus.sakura.ad.jp/eventlog", ProcessConfigurationID: "456",
		})}
}

func TestMutationSafety(t *testing.T) {
	operations := map[string]func(context.Context, *Service) error{
		"process-update": func(ctx context.Context, s *Service) error {
			_, err := s.UpdateProcessConfiguration(ctx, &UpdateProcessConfigurationRequest{ID: "123", Name: ptr("new")})
			return err
		},
		"process-delete": func(ctx context.Context, s *Service) error {
			return s.DeleteProcessConfiguration(ctx, &DeleteRequest{ID: "123"})
		},
		"process-set-secret": func(ctx context.Context, s *Service) error {
			return s.SetProcessConfigurationSecret(ctx, &SetProcessConfigurationSecretRequest{ID: "123"})
		},
		"schedule-update": func(ctx context.Context, s *Service) error {
			_, err := s.UpdateSchedule(ctx, &UpdateScheduleRequest{ID: "123", Name: ptr("new")})
			return err
		},
		"schedule-delete": func(ctx context.Context, s *Service) error { return s.DeleteSchedule(ctx, &DeleteRequest{ID: "123"}) },
		"trigger-update": func(ctx context.Context, s *Service) error {
			_, err := s.UpdateTrigger(ctx, &UpdateTriggerRequest{ID: "123", Name: ptr("new")})
			return err
		},
		"trigger-delete": func(ctx context.Context, s *Service) error { return s.DeleteTrigger(ctx, &DeleteRequest{ID: "123"}) },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			for _, failure := range []string{"read-error", "nil-response", "wrong-id", "wrong-provider", "wrong-settings", "cancelled"} {
				t.Run(failure, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					item := processItem()
					if strings.HasPrefix(name, "schedule") {
						item = scheduleItem()
					} else if strings.HasPrefix(name, "trigger") {
						item = triggerItem()
					}
					api := &fakeAPI{item: item}
					switch failure {
					case "read-error":
						api.readErr = errors.New("read failed")
					case "nil-response":
						api.item = nil
					case "wrong-id":
						api.item.ID = "other"
					case "wrong-provider":
						api.item.Provider.Class = ""
					case "wrong-settings":
						api.item.Settings = v1.Settings{}
					case "cancelled":
						api.afterRead = cancel
					}
					s := &Service{process: api, schedule: api, trigger: api}
					require.Error(t, operation(ctx, s))
					require.Zero(t, api.mutations)
				})
			}
		})
	}
}

func TestReadChecksIdentity(t *testing.T) {
	for _, item := range []*v1.CommonServiceItem{nil, scheduleItem(), {ID: "other"}, {ID: "123"}} {
		s := &Service{process: &fakeAPI{item: item}}
		_, err := s.ReadProcessConfiguration(t.Context(), &ReadRequest{ID: "123"})
		require.Error(t, err)
	}
}
