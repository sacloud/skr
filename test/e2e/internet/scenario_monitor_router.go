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
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// monitorWindow is the length of the observed time range, ending at the current time.
const monitorWindow = 10 * time.Minute

// monitorRouter reads Router + Switch monitor values for a recent time range and
// verifies that the command returns well-formed monitor data.
func (s scenario) monitorRouter(ctx context.Context, id types.ID) error {
	end := time.Now().UTC()
	output, err := s.call(ctx, "test-monitor-router", map[string]any{
		"Zone":  zone,
		"ID":    id,
		"Start": end.Add(-monitorWindow).Format(time.RFC3339),
		"End":   end.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return checkMonitorValues(output)
}

func monitorRouterArgs(request map[string]any) ([]string, error) {
	id, err := requestID(request)
	if err != nil {
		return nil, err
	}
	start, err := requestString(request, "Start")
	if err != nil {
		return nil, err
	}
	end, err := requestString(request, "End")
	if err != nil {
		return nil, err
	}
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return nil, fmt.Errorf("invalid Start: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return nil, fmt.Errorf("invalid End: %w", err)
	}
	if !endTime.After(startTime) {
		return nil, errors.New("End must be after Start")
	}
	idValue, err := strconv.ParseInt(id, 10, 64)
	if err != nil || idValue <= 0 {
		return nil, errors.New("invalid ID")
	}
	payload := map[string]any{"Zone": zone, "ID": idValue, "Start": start, "End": end}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Router monitor request: %w", err)
	}
	return []string{"iaas-api", "internet", "monitor-router", "--request", string(data)}, nil
}

type monitorValue struct {
	Time string   `json:"Time"`
	In   *float64 `json:"In"`
	Out  *float64 `json:"Out"`
}

func checkMonitorValues(output []byte) error {
	var values []monitorValue
	if err := json.Unmarshal(output, &values); err != nil {
		return fmt.Errorf("decode Router monitor values: %w", err)
	}
	for i, value := range values {
		if _, err := time.Parse(time.RFC3339, value.Time); err != nil {
			return fmt.Errorf("monitor value %d has invalid Time %q", i, value.Time)
		}
		if value.In == nil || value.Out == nil {
			return fmt.Errorf("monitor value %d lacks numeric In/Out values", i)
		}
	}
	return nil
}
