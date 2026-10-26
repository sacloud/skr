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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

func TestMonitorRouterArgsEncodeJSONRequest(t *testing.T) {
	args, err := monitorRouterArgs(map[string]any{
		"Zone": zone, "ID": types.ID(12),
		"Start": "2026-01-01T00:00:00Z", "End": "2026-01-01T00:10:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 5 || args[0] != "iaas-api" || args[1] != "internet" || args[2] != "monitor-router" || args[3] != "--request" {
		t.Fatalf("monitor-router args = %v", args)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args[4]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["Zone"] != zone || payload["ID"] != float64(12) ||
		payload["Start"] != "2026-01-01T00:00:00Z" || payload["End"] != "2026-01-01T00:10:00Z" {
		t.Fatalf("monitor-router request = %+v", payload)
	}
}

func TestMonitorRouterArgsRejectInvalidRequests(t *testing.T) {
	for _, request := range []map[string]any{
		{"Zone": zone, "Start": "2026-01-01T00:00:00Z", "End": "2026-01-01T00:10:00Z"},
		{"Zone": zone, "ID": types.ID(0), "Start": "2026-01-01T00:00:00Z", "End": "2026-01-01T00:10:00Z"},
		{"Zone": zone, "ID": types.ID(12), "End": "2026-01-01T00:10:00Z"},
		{"Zone": zone, "ID": types.ID(12), "Start": "2026-01-01T00:00:00Z"},
		{"Zone": zone, "ID": types.ID(12), "Start": "not-a-time", "End": "2026-01-01T00:10:00Z"},
		{"Zone": zone, "ID": types.ID(12), "Start": "2026-01-01T00:00:00Z", "End": "not-a-time"},
		{"Zone": zone, "ID": types.ID(12), "Start": "2026-01-01T00:10:00Z", "End": "2026-01-01T00:00:00Z"},
		{"Zone": zone, "ID": types.ID(12), "Start": "2026-01-01T00:00:00Z", "End": "2026-01-01T00:00:00Z"},
	} {
		if _, err := monitorRouterArgs(request); err == nil {
			t.Errorf("monitorRouterArgs accepted invalid request: %+v", request)
		}
	}
}

func TestCheckMonitorValuesAcceptsWellFormedData(t *testing.T) {
	for _, output := range []string{
		`[]`,
		`[{"Time":"2026-01-01T00:00:00Z","In":0,"Out":0}]`,
		`[{"Time":"2026-01-01T00:00:00Z","In":12.5,"Out":0.25},{"Time":"2026-01-01T00:05:00Z","In":0,"Out":3}]`,
	} {
		if err := checkMonitorValues([]byte(output)); err != nil {
			t.Errorf("checkMonitorValues(%s) = %v, want nil", output, err)
		}
	}
	// データポイントがないことは、トラフィックのない新しい Router + Switch に対する有効な応答である。
	if err := checkMonitorValues([]byte(`null`)); err != nil {
		t.Errorf("checkMonitorValues(null) = %v, want nil", err)
	}
}

func TestCheckMonitorValuesRejectsMalformedData(t *testing.T) {
	for _, output := range []string{
		`{`,
		`{"Time":"2026-01-01T00:00:00Z","In":0,"Out":0}`,
		`[{"Time":"not-a-time","In":0,"Out":0}]`,
		`[{"Time":"2026-01-01T00:00:00Z","Out":0}]`,
		`[{"Time":"2026-01-01T00:00:00Z","In":0}]`,
		`[{"Time":"2026-01-01T00:00:00Z"}]`,
	} {
		if err := checkMonitorValues([]byte(output)); err == nil {
			t.Errorf("checkMonitorValues(%s) accepted malformed monitor data", output)
		}
	}
}

func TestMonitorRouterCallsCommandWithVerifiedInternet(t *testing.T) {
	fake := &fakeCLI{}
	fake.internet = &internetItem{
		ID: 12, Name: namePrefix + "test", Description: internetDesc,
		NetworkMaskLen: 28, BandWidthMbps: 100,
	}
	fake.internet.Switch.ID = 13
	test := testScenario(fake, t.TempDir())
	if err := test.monitorRouter(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "test-monitor-router" {
		t.Fatalf("calls = %v, want test-monitor-router", fake.calls)
	}
}
