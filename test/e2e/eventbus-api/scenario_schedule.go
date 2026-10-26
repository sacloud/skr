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
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// scheduleLeadTime keeps the schedule's first execution one year away so that
// it never fires during the run.
const scheduleLeadTime = time.Hour * 24 * 365

// scheduleItem is the schedule read result with its Settings.
type scheduleItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Settings    struct {
		ProcessConfigurationID json.RawMessage `json:"ProcessConfigurationID"`
		StartsAt               string          `json:"StartsAt"`
		RecurringStep          int             `json:"RecurringStep"`
		RecurringUnit          string          `json:"RecurringUnit"`
	} `json:"Settings"`
}

func (i scheduleItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// scheduleScenario exercises schedule create, list, read, update, and delete
// against the created process-configuration. The schedule starts one year later
// and is deleted and verified absent within the run, so it never fires.
func (s *scenario) scheduleScenario(ctx context.Context) error {
	startsAt := time.Now().Add(scheduleLeadTime).UnixMilli()
	if err := s.createSchedule(ctx, startsAt); err != nil {
		return err
	}
	id := s.schedule.id
	found, ok, err := s.findByName(ctx, "list-schedule", s.schedule)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("schedule %q was not found after create", s.schedule.name)
	}
	foundID, err := found.id()
	if err != nil || foundID != id {
		return fmt.Errorf("created schedule ID mismatch: list returned %q, want %q", foundID, id)
	}
	read, err := s.readSchedule(ctx, "read-schedule", id)
	if err != nil {
		return err
	}
	if err := checkSchedule(read, s.schedule, id, s.config.id, startsAt, s.schedule.description); err != nil {
		return err
	}
	updatedDescription := s.schedule.description + " (updated)"
	if err := s.updateScheduleDescription(ctx, "update-schedule", id, updatedDescription); err != nil {
		return err
	}
	updated, err := s.readSchedule(ctx, "read-updated-schedule", id)
	if err != nil {
		return err
	}
	if err := checkSchedule(updated, s.schedule, id, s.config.id, startsAt, updatedDescription); err != nil {
		return err
	}
	if err := s.updateScheduleDescription(ctx, "restore-schedule", id, s.schedule.description); err != nil {
		return fmt.Errorf("restore schedule Description: %w", err)
	}
	restored, err := s.readSchedule(ctx, "read-restored-schedule", id)
	if err != nil {
		return fmt.Errorf("read restored schedule: %w", err)
	}
	if err := checkSchedule(restored, s.schedule, id, s.config.id, startsAt, s.schedule.description); err != nil {
		return err
	}
	if err := s.deleteSchedule(ctx, id); err != nil {
		return err
	}
	remaining, err := s.list(ctx, "verify-schedule-deleted", "schedule")
	if err != nil {
		return err
	}
	for _, candidate := range remaining {
		if candidate.Name == s.schedule.name {
			return fmt.Errorf("schedule %q remains after delete", s.schedule.name)
		}
	}
	s.schedule.deleted = true
	return nil
}

func (s *scenario) createSchedule(ctx context.Context, startsAt int64) error {
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":        s.schedule.name,
			"Description": s.schedule.description,
			"Settings": map[string]any{
				"ProcessConfigurationID": s.config.id,
				"StartsAt":               startsAt,
				"RecurringStep":          1,
				"RecurringUnit":          "day",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("encode schedule create: %w", err)
	}
	s.schedule.attempted = true
	data, err := s.call(ctx, "create-schedule", "eventbus-api", "schedule", "create", "--request", string(request))
	if err != nil {
		return err
	}
	created, err := decodeItem("create-schedule", data)
	if err != nil {
		return err
	}
	id, err := created.id()
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.schedule, id); err != nil {
		return err
	}
	s.schedule.id = id
	return nil
}

func (s *scenario) updateScheduleDescription(ctx context.Context, step, id, description string) error {
	request, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Description": description,
		},
	})
	if err != nil {
		return fmt.Errorf("encode schedule update: %w", err)
	}
	_, err = s.call(ctx, step, "eventbus-api", "schedule", "update", id, "--request", string(request))
	return err
}

func (s *scenario) readSchedule(ctx context.Context, step, id string) (scheduleItem, error) {
	data, err := s.call(ctx, step, "eventbus-api", "schedule", "read", id)
	if err != nil {
		return scheduleItem{}, err
	}
	var result scheduleItem
	if err := json.Unmarshal(data, &result); err != nil {
		return scheduleItem{}, fmt.Errorf("%s: decode schedule: %w", step, err)
	}
	return result, nil
}

func (s *scenario) deleteSchedule(ctx context.Context, id string) error {
	if _, err := s.call(ctx, "delete-schedule", "eventbus-api", "schedule", "delete", id); err != nil {
		return err
	}
	return nil
}

func checkSchedule(read scheduleItem, resource resource, id, configID string, startsAt int64, wantDescription string) error {
	readID, err := read.id()
	if err != nil {
		return err
	}
	if readID != id || read.Name != resource.name || read.Description != wantDescription {
		return fmt.Errorf("schedule identity mismatch for ID %s", id)
	}
	configReference, err := decodeResourceID(read.Settings.ProcessConfigurationID)
	if err != nil || configReference != configID {
		return fmt.Errorf("schedule ProcessConfigurationID mismatch for ID %s", id)
	}
	// The API returns StartsAt as a decimal string in responses even though
	// the request accepts an integer.
	wantStartsAt := fmt.Sprintf("%d", startsAt)
	if read.Settings.StartsAt != wantStartsAt {
		return fmt.Errorf("schedule StartsAt mismatch for ID %s: got %q, want %q", id, read.Settings.StartsAt, wantStartsAt)
	}
	if read.Settings.RecurringStep != 1 || read.Settings.RecurringUnit != "day" {
		return fmt.Errorf("schedule recurrence mismatch for ID %s: got %d %s", id, read.Settings.RecurringStep, read.Settings.RecurringUnit)
	}
	return nil
}
