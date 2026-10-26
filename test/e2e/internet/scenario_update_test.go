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

func TestInternetUpdateArgsUseUpdateOptions(t *testing.T) {
	args, err := internetArgs("test-update-internet", map[string]any{
		"Zone": zone, "ID": types.ID(12), "Name": "updated-name", "Description": "updated desc",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); joined != "iaas-api internet update --zone is1b --id 12 --name updated-name --description updated desc" {
		t.Fatalf("update args = %q", joined)
	}
	args, err = internetArgs("test-update-internet-restore", map[string]any{
		"Zone": zone, "ID": types.ID(12), "Name": "original", "Description": "original desc",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); joined != "iaas-api internet update --zone is1b --id 12 --name original --description original desc" {
		t.Fatalf("restore args = %q", joined)
	}
}

func TestInternetUpdateArgsRejectInvalidRequests(t *testing.T) {
	for _, request := range []map[string]any{
		{"Zone": "tk1v", "ID": types.ID(12), "Name": "n", "Description": "d"},
		{"Zone": zone, "Name": "n", "Description": "d"},
		{"Zone": zone, "ID": types.ID(12), "Description": "d"},
		{"Zone": zone, "ID": types.ID(12), "Name": "n"},
		{"Zone": zone, "ID": types.ID(0), "Name": "n", "Description": "d"},
	} {
		if _, err := internetArgs("test-update-internet", request, ""); err == nil {
			t.Errorf("internetArgs accepted invalid request: %+v", request)
		}
	}
}

func TestUpdateInternetAndVerifyUpdatesRestoresAndReads(t *testing.T) {
	fake := &fakeCLI{}
	fake.internet = &internetItem{
		ID: 12, Name: namePrefix + "test", Description: internetDesc,
		NetworkMaskLen: 28, BandWidthMbps: 100,
	}
	fake.internet.Switch.ID = 13
	test := testScenario(fake, t.TempDir())
	if err := test.updateInternetAndVerify(context.Background(), 12, namePrefix+"test", 13); err != nil {
		t.Fatal(err)
	}
	want := []string{"test-update-internet", "test-read-internet", "test-update-internet-restore", "test-read-internet"}
	if len(fake.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", fake.calls, want)
	}
	for i, step := range want {
		if fake.calls[i] != step {
			t.Fatalf("step %d = %s, want %s; calls=%v", i, fake.calls[i], step, fake.calls)
		}
	}
	if fake.internet.Name != namePrefix+"test" || fake.internet.Description != internetDesc {
		t.Fatalf("Internet was not restored: %+v", fake.internet)
	}
}

func TestUpdateInternetAndVerifyReportsFailures(t *testing.T) {
	newInternet := func() *internetItem {
		item := &internetItem{
			ID: 12, Name: namePrefix + "test", Description: internetDesc,
			NetworkMaskLen: 28, BandWidthMbps: 100,
		}
		item.Switch.ID = 13
		return item
	}
	t.Run("update api error", func(t *testing.T) {
		fake := &fakeCLI{failStep: "test-update-internet", internet: newInternet()}
		err := testScenario(fake, t.TempDir()).updateInternetAndVerify(context.Background(), 12, namePrefix+"test", 13)
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want update API failure", err)
		}
	})
	t.Run("update ignored", func(t *testing.T) {
		fake := &fakeCLI{internet: newInternet()}
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-update-internet" {
				// 更新を無視して成功を装う。次の Read でそれを検出する必要がある。
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.updateInternetAndVerify(context.Background(), 12, namePrefix+"test", 13)
		if err == nil || !strings.Contains(err.Error(), "updated Internet identity mismatch") {
			t.Fatalf("error = %v, want update reflection check failure", err)
		}
	})
	t.Run("restore api error", func(t *testing.T) {
		fake := &fakeCLI{failStep: "test-update-internet-restore", internet: newInternet()}
		err := testScenario(fake, t.TempDir()).updateInternetAndVerify(context.Background(), 12, namePrefix+"test", 13)
		if err == nil || !strings.Contains(err.Error(), "restore Internet Name and Description") {
			t.Fatalf("error = %v, want restore failure", err)
		}
	})
	t.Run("restore ignored", func(t *testing.T) {
		fake := &fakeCLI{internet: newInternet()}
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-update-internet-restore" {
				// 復元を無視して成功を装う。次の Read でそれを検出する必要がある。
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.updateInternetAndVerify(context.Background(), 12, namePrefix+"test", 13)
		if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("error = %v, want restore reflection check failure", err)
		}
	})
}

func TestScenarioKeepsInternetWhenRestoreFails(t *testing.T) {
	fake := &fakeCLI{failStep: "test-update-internet-restore"}
	test := testScenario(fake, t.TempDir())
	err := test.run(context.Background(), namePrefix+"test")
	if err == nil || !strings.Contains(err.Error(), "restore Internet Name and Description") {
		t.Fatalf("scenario error = %v, want restore failure", err)
	}
	if !strings.Contains(err.Error(), "cleanup refused") || fake.internet == nil || fake.server != nil {
		t.Fatalf("cleanup deleted an Internet whose identity changed: error=%v", err)
	}
	if containsStep(fake.calls, "cleanup-delete-internet") {
		t.Fatal("Internet whose restore failed was deleted")
	}
}
