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

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// updateFunc adapts a test function to the scenario's cli interface.
type updateFunc func(context.Context, string, map[string]any) ([]byte, error)

func (f updateFunc) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	return f(ctx, step, request)
}

func TestServerUpdateArgsUseUpdateOptions(t *testing.T) {
	args, err := serverArgs("test-update", map[string]any{
		"Zone": zone, "ID": types.ID(123), "Name": "updated-name", "Description": "updated desc",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); joined != "iaas-api server update --zone is1b --id 123 --name updated-name --description updated desc" {
		t.Fatalf("update args = %q", joined)
	}
	args, err = serverArgs("test-update-restore", map[string]any{
		"Zone": zone, "ID": types.ID(123), "Name": "original", "Description": "original desc",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); joined != "iaas-api server update --zone is1b --id 123 --name original --description original desc" {
		t.Fatalf("restore args = %q", joined)
	}
}

func TestServerUpdateArgsRejectInvalidRequests(t *testing.T) {
	for _, request := range []map[string]any{
		{"Zone": "tk1v", "ID": types.ID(123), "Name": "n", "Description": "d"},
		{"Zone": zone, "Name": "n", "Description": "d"},
		{"Zone": zone, "ID": types.ID(123), "Description": "d"},
		{"Zone": zone, "ID": types.ID(123), "Name": "n"},
		{"Zone": zone, "ID": types.ID(0), "Name": "n", "Description": "d"},
	} {
		if _, err := serverArgs("test-update", request, ""); err == nil {
			t.Errorf("serverArgs accepted invalid request: %+v", request)
		}
	}
}

func TestUpdateAndVerifyUpdatesRestoresAndReads(t *testing.T) {
	fake := &fakeCLI{}
	fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: description}
	test := testScenario(fake)
	if err := test.updateAndVerify(context.Background(), 123, namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	want := []string{"test-update", "test-read", "test-update-restore", "test-read"}
	if len(fake.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", fake.calls, want)
	}
	for i, step := range want {
		if fake.calls[i] != step {
			t.Fatalf("step %d = %s, want %s; calls=%v", i, fake.calls[i], step, fake.calls)
		}
	}
	if fake.server.Name != namePrefix+"test" || fake.server.Description != description {
		t.Fatalf("Server was not restored: %+v", fake.server)
	}
}

func TestUpdateAndVerifyReportsFailures(t *testing.T) {
	t.Run("update api error", func(t *testing.T) {
		fake := &fakeCLI{failStep: "test-update"}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: description}
		err := testScenario(fake).updateAndVerify(context.Background(), 123, namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want update API failure", err)
		}
	})
	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeCLI{}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: description}
		test := testScenario(fake)
		test.client = updateFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-update" {
				// Pretend success while ignoring the update; the next Read must detect it.
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.updateAndVerify(context.Background(), 123, namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "updated Server identity mismatch") {
			t.Fatalf("error = %v, want update reflection check failure", err)
		}
	})
	t.Run("restore api error", func(t *testing.T) {
		fake := &fakeCLI{failStep: "test-update-restore"}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: description}
		err := testScenario(fake).updateAndVerify(context.Background(), 123, namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "restore Server Name and Description") {
			t.Fatalf("error = %v, want restore failure", err)
		}
	})
	t.Run("restore ignored", func(t *testing.T) {
		fake := &fakeCLI{}
		fake.server = &serverItem{ID: 123, Name: namePrefix + "test", Description: description}
		test := testScenario(fake)
		test.client = updateFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-update-restore" {
				// Pretend success while ignoring the restore; the next Read must detect it.
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.updateAndVerify(context.Background(), 123, namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("error = %v, want restore reflection check failure", err)
		}
	})
}

func TestScenarioKeepsServerWhenRestoreFails(t *testing.T) {
	fake := &fakeCLI{failStep: "test-update-restore"}
	test := testScenario(fake)
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "restore Server Name and Description") {
		t.Fatalf("scenario error = %v, want restore failure", err)
	}
	if !strings.Contains(err.Error(), "cleanup refused") || fake.server == nil || len(fake.deleted) != 0 {
		t.Fatalf("cleanup deleted a Server whose identity changed: error=%v", err)
	}
}
