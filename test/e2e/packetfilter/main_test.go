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
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type fakeCLI struct {
	item           *packetFilterItem
	failAt         string
	badCreate      bool
	changeIdentity bool
	projection     []byte
	deleted        []types.ID
	calls          []string
	existing       []packetFilterItem
}

func (f *fakeCLI) call(_ context.Context, step string, value any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if step == "profile-current" {
		return []byte("selected-profile\n"), nil
	}
	if f.failAt == step {
		return nil, errors.New("injected CLI failure")
	}
	request, ok := value.(map[string]any)
	if !ok || request["Zone"] != zone {
		return nil, errors.New("unexpected request or zone")
	}
	if step == "test-find-query" && f.projection != nil {
		return f.projection, nil
	}
	if strings.Contains("-"+step+"-", "-find-") {
		var items []packetFilterItem
		if f.item != nil {
			items = append(items, *f.item)
		}
		items = append(items, f.existing...)
		names, _ := request["Names"].([]string)
		if len(names) != 0 {
			var filtered []packetFilterItem
			for _, item := range items {
				if item.Name == names[0] {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		if items == nil {
			items = []packetFilterItem{}
		}
		return json.Marshal(items)
	}
	switch step {
	case "test-create":
		expressions, err := expressionRequest(request)
		if err != nil {
			return nil, err
		}
		f.item = &packetFilterItem{
			ID: 1234, Name: request["Name"].(string), Description: request["Description"].(string),
			Expression: expressions, ExpressionHash: "hash-before",
		}
		if f.badCreate {
			return []byte("{"), nil
		}
		return json.Marshal(f.item)
	case "test-read", "test-read-updated", "cleanup-read":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("missing PacketFilter")
		}
		item := *f.item
		if f.changeIdentity && step == "cleanup-read" {
			item.Description = "changed identity"
		}
		return json.Marshal(item)
	case "test-update":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("missing PacketFilter")
		}
		expressions, err := expressionRequest(request)
		if err != nil {
			return nil, err
		}
		f.item.Name = request["Name"].(string)
		f.item.Expression = expressions
		f.item.ExpressionHash = "hash-after"
		return json.Marshal(f.item)
	case "cleanup-delete":
		if f.item == nil || request["ID"] != f.item.ID {
			return nil, errors.New("unsafe delete")
		}
		f.deleted = append(f.deleted, f.item.ID)
		f.item = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func expressionRequest(request map[string]any) ([]packetFilterExpression, error) {
	data, err := json.Marshal(request["Expression"])
	if err != nil {
		return nil, err
	}
	var expressions []packetFilterExpression
	if err := json.Unmarshal(data, &expressions); err != nil {
		return nil, err
	}
	return expressions, nil
}

func testScenario(t *testing.T, fake *fakeCLI) error {
	t.Helper()
	return (scenario{client: fake}).run(context.Background(), namePrefix+"test")
}

func TestScenarioCreatesUpdatesAndDeletesPacketFilter(t *testing.T) {
	fake := &fakeCLI{}
	if err := testScenario(t, fake); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil || len(fake.deleted) != 1 || fake.deleted[0] != types.ID(1234) {
		t.Fatalf("PacketFilter after E2E: item=%+v deleted=%v", fake.item, fake.deleted)
	}
	for _, step := range []string{
		"test-create", "test-find", "test-find-query", "test-read", "test-update",
		"test-read-updated", "cleanup-delete", "cleanup-find-original-after", "cleanup-find-updated-after",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestScenarioCleansUpAfterUpdateFailure(t *testing.T) {
	fake := &fakeCLI{failAt: "test-update"}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want update failure", err)
	}
	if fake.item != nil || len(fake.deleted) != 1 {
		t.Fatalf("PacketFilter was not cleaned up: item=%+v deleted=%v", fake.item, fake.deleted)
	}
}

func TestScenarioRecoversAfterCreateOutputDecodeFailure(t *testing.T) {
	fake := &fakeCLI{badCreate: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "decode PacketFilter") {
		t.Fatalf("scenario error = %v, want response decode failure", err)
	}
	if fake.item != nil || len(fake.deleted) != 1 {
		t.Fatalf("created PacketFilter was not recovered and deleted")
	}
}

func TestScenarioRefusesExistingName(t *testing.T) {
	fake := &fakeCLI{existing: []packetFilterItem{{ID: 9876, Name: namePrefix + "test", Description: "existing filter"}}}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("scenario error = %v, want collision refusal", err)
	}
	if len(fake.deleted) != 0 || len(fake.calls) != 2 {
		t.Fatalf("existing PacketFilter was changed: calls=%v deleted=%v", fake.calls, fake.deleted)
	}
}

func TestScenarioRejectsInvalidProjectionAndCleansUp(t *testing.T) {
	for _, output := range []string{"null", "[]", `[{"ID":1234,"Name":"wrong"}]`, `[{"ID":9876,"Name":"skr-e2e-packetfilter-test"}]`} {
		t.Run(output, func(t *testing.T) {
			fake := &fakeCLI{projection: []byte(output)}
			if err := testScenario(t, fake); err == nil {
				t.Fatal("invalid projection accepted")
			}
			if fake.item != nil || len(fake.deleted) != 1 {
				t.Fatal("created PacketFilter was not cleaned up")
			}
		})
	}
}

func TestScenarioRefusesCleanupAfterIdentityChange(t *testing.T) {
	fake := &fakeCLI{failAt: "test-update", changeIdentity: true}
	err := testScenario(t, fake)
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("scenario error = %v, want cleanup refusal", err)
	}
	if fake.item == nil || len(fake.deleted) != 0 {
		t.Fatal("PacketFilter with changed identity was deleted")
	}
}

func TestPacketFilterArgsUseExpressionsJSON(t *testing.T) {
	expressions := []map[string]any{{"Protocol": "tcp", "DestinationPort": "443", "Action": "allow"}}
	tests := []struct {
		step    string
		request map[string]any
		want    []string
	}{
		{
			step: "test-create",
			request: map[string]any{
				"Zone": zone, "Name": namePrefix + "test", "Description": description, "Expression": expressions,
			},
			want: []string{"--zone is1b", "--name " + namePrefix + "test", "--expressions", `"Protocol":"tcp"`},
		},
		{
			step: "test-update",
			request: map[string]any{
				"Zone": zone, "ID": types.ID(123), "OriginalExpressionHash": "hash-before", "Expression": expressions,
			},
			want: []string{"--zone is1b", "--id 123", "--original-expression-hash hash-before", "--expressions"},
		},
		{
			step:    "test-find",
			request: map[string]any{"Zone": zone, "Names": []string{namePrefix + "test"}},
			want:    []string{"--request", `"Names":["skr-e2e-packetfilter-test"]`},
		},
	}
	for _, test := range tests {
		args, err := packetFilterArgs(test.step, test.request)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(args, " ")
		for _, want := range test.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s args %v do not contain %q", test.step, args, want)
			}
		}
		if strings.Contains(got, "--output") {
			t.Errorf("%s args contain removed flag: %v", test.step, args)
		}
	}
}

func containsStep(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}
