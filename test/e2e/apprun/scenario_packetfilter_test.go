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
	"strings"
	"testing"
)

func TestPacketFilterScenarioEnablesAndRestores(t *testing.T) {
	fake := &fakeCLI{}
	s := testScenarioForAdditions(t, fake)
	if err := s.packetFilterScenario(context.Background(), "app-id"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{
		"read-packet-filter", "update-packet-filter", "read-updated-packet-filter",
		"restore-packet-filter", "read-restored-packet-filter",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
	if fake.filter.IsEnabled {
		t.Error("packet filter must be restored to the disabled state")
	}
}

func TestPacketFilterScenarioReportsFailures(t *testing.T) {
	t.Run("already enabled", func(t *testing.T) {
		fake := &fakeCLI{}
		fake.filter.IsEnabled = true
		s := testScenarioForAdditions(t, fake)
		err := s.packetFilterScenario(context.Background(), "app-id")
		if err == nil || !strings.Contains(err.Error(), "already enabled") {
			t.Fatalf("error = %v, want already enabled refusal", err)
		}
	})
	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeCLI{}
		s := testScenarioForAdditions(t, fake)
		s.client = &filterOverrideCLI{base: fake, updateStale: true}
		err := s.packetFilterScenario(context.Background(), "app-id")
		if err == nil || !strings.Contains(err.Error(), "is_enabled = false, want true") {
			t.Fatalf("error = %v, want update reflection failure", err)
		}
	})
	t.Run("restore ignored", func(t *testing.T) {
		fake := &fakeCLI{}
		s := testScenarioForAdditions(t, fake)
		s.client = &filterOverrideCLI{base: fake, restoreStale: true}
		err := s.packetFilterScenario(context.Background(), "app-id")
		if err == nil || !strings.Contains(err.Error(), "is_enabled = true, want false") {
			t.Fatalf("error = %v, want restore reflection failure", err)
		}
	})
	t.Run("api failure", func(t *testing.T) {
		fake := &fakeCLI{failStep: "update-packet-filter"}
		s := testScenarioForAdditions(t, fake)
		err := s.packetFilterScenario(context.Background(), "app-id")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want injected CLI failure", err)
		}
	})
}

// filterOverrideCLI returns a stale packet-filter state for the reflection
// reads while delegating everything else to the base fake.
type filterOverrideCLI struct {
	base         *fakeCLI
	updateStale  bool
	restoreStale bool
}

func (o *filterOverrideCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	data, err := o.base.call(ctx, step, args...)
	if err != nil {
		return data, err
	}
	switch step {
	case "read-updated-packet-filter":
		if o.updateStale {
			return []byte(`{"is_enabled":false,"settings":[]}`), nil
		}
	case "read-restored-packet-filter":
		if o.restoreStale {
			return []byte(`{"is_enabled":true,"settings":[{"from_ip":"192.0.2.1","from_ip_prefix_length":32}]}`), nil
		}
	}
	return data, nil
}

func (o *filterOverrideCLI) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return o.base.callRoot(ctx, step, args...)
}
