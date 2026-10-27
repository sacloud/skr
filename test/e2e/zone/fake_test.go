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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeZoneCommands はシナリオの正常系とエラー系を確認する偽の実装です。
type fakeZoneCommands struct {
	findErr    error
	namesErr   error
	readErr    error
	zones      []zoneItem
	names      []string
	readResult zoneItem
	readID     int64
	findCalls  int
	readCalls  int
}

func (f *fakeZoneCommands) findZones(_ *testing.T, _ string) ([]zoneItem, error) {
	f.findCalls++
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.zones, nil
}

func (f *fakeZoneCommands) findZoneNames(_ *testing.T, _ string) ([]string, error) {
	if f.namesErr != nil {
		return nil, f.namesErr
	}
	return f.names, nil
}

func (f *fakeZoneCommands) readZone(_ *testing.T, _ string, id int64) (zoneItem, error) {
	f.readCalls++
	f.readID = id
	if f.readErr != nil {
		return zoneItem{}, f.readErr
	}
	return f.readResult, nil
}

func TestZoneScenarioPassesFindNamesAndRead(t *testing.T) {
	zones := []zoneItem{
		{ID: 1, Name: "zone-a", Description: "zone a"},
		{ID: 2, Name: "zone-b", Description: "zone b"},
	}
	fake := &fakeZoneCommands{
		zones:      zones,
		names:      []string{"zone-a", "zone-b"},
		readResult: zones[0],
	}

	require.NoError(t, runZoneScenario(t, fake))
	require.Equal(t, 1, fake.findCalls, "find calls")
	require.Equal(t, 1, fake.readCalls, "read calls")
	require.Equal(t, int64(1), fake.readID, "read zone ID")
}

func TestZoneScenarioReportsFailures(t *testing.T) {
	zones := []zoneItem{{ID: 1, Name: "zone-a", Description: "zone a"}}

	for _, test := range []struct {
		name    string
		fake    *fakeZoneCommands
		wantErr string
	}{
		{"find failure", &fakeZoneCommands{findErr: errors.New("injected"), zones: zones}, "find zones"},
		{"empty zone list", &fakeZoneCommands{}, "zone list is empty"},
		{"zone without ID", &fakeZoneCommands{zones: []zoneItem{{Name: "zone-a"}}, names: []string{"zone-a"}}, "has no ID"},
		{"zone without name", &fakeZoneCommands{zones: []zoneItem{{ID: 1}}, names: []string{""}}, "has an empty name"},
		{"name count mismatch", &fakeZoneCommands{zones: zones, names: []string{"zone-a", "zone-b"}}, "zone names"},
		{"name order mismatch", &fakeZoneCommands{zones: zones, names: []string{"zone-b", "zone-a"}}, "zone names"},
		{"read failure", &fakeZoneCommands{zones: zones, names: []string{"zone-a"}, readErr: errors.New("injected")}, "read zone"},
		{"read ID mismatch", &fakeZoneCommands{zones: zones, names: []string{"zone-a"}, readResult: zoneItem{ID: 99, Name: "zone-a"}}, "read zone ID mismatch"},
		{"read name mismatch", &fakeZoneCommands{zones: zones, names: []string{"zone-a"}, readResult: zoneItem{ID: 1, Name: "other"}}, "read zone name mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := runZoneScenario(t, test.fake)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
