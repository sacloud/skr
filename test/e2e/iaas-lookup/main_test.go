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
	"errors"
	"strings"
	"testing"
)

type fakeCLI struct {
	failStep string
	outputs  map[string][]byte
	calls    []string
	args     map[string][]string
}

func (f *fakeCLI) call(_ context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = make(map[string][]string)
	}
	f.args[step] = append([]string(nil), args...)
	if f.failStep == step {
		return nil, errors.New("injected CLI failure")
	}
	if output, exists := f.outputs[step]; exists {
		return output, nil
	}
	switch step {
	case "profile-current":
		return []byte("test-profile\n"), nil
	case "archive-find":
		return []byte(`[{"ID":123,"Name":"archive","Description":"test","DiskPlanID":4,"DiskPlanName":"ssd","SizeMB":20480,"Tags":["test"]}]`), nil
	case "archive-read":
		return []byte(`{"ID":123,"Name":"archive","Description":"test","DiskPlanID":4,"DiskPlanName":"ssd","SizeMB":20480,"Tags":["test"]}`), nil
	case "disk-plan-find":
		return []byte(`[{"ID":4,"Name":"ssd","StorageClass":"ssd","Availability":"available","Size":[{"Availability":"available","DisplaySize":20,"DisplaySuffix":"GB","SizeMB":20480}]}]`), nil
	case "disk-plan-read":
		return []byte(`{"ID":4,"Name":"ssd","StorageClass":"ssd","Availability":"available","Size":[{"Availability":"available","DisplaySize":20,"DisplaySuffix":"GB","SizeMB":20480}]}`), nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioFindsAndReadsArchiveAndDiskPlan(t *testing.T) {
	fake := &fakeCLI{}
	if err := (scenario{client: fake, zone: "is1b"}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fake.calls, " "), "profile-current archive-find archive-read disk-plan-find disk-plan-read"; got != want {
		t.Errorf("scenario calls = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["archive-find"], " "), "iaas-api archive find --zone is1b --query "+archiveQuery; got != want {
		t.Errorf("Archive find args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["archive-read"], " "), "iaas-api archive read --zone is1b --id 123"; got != want {
		t.Errorf("Archive read args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["disk-plan-find"], " "), "iaas-api disk-plan find --zone is1b --query "+diskPlanQuery; got != want {
		t.Errorf("DiskPlan find args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["disk-plan-read"], " "), "iaas-api disk-plan read --zone is1b --id 4"; got != want {
		t.Errorf("DiskPlan read args = %q, want %q", got, want)
	}
}

func TestScenarioRejectsInvalidProfileZoneAndResults(t *testing.T) {
	for _, test := range []struct {
		name     string
		zone     string
		failStep string
		outputs  map[string][]byte
		wantErr  string
	}{
		{name: "profile failure", zone: "is1b", failStep: "profile-current", wantErr: "selected SDK profile"},
		{name: "empty profile", zone: "is1b", outputs: map[string][]byte{"profile-current": []byte(" \n")}, wantErr: "profile name is empty"},
		{name: "all zones", zone: "all", wantErr: "single target zone"},
		{name: "invalid archive JSON", zone: "is1b", outputs: map[string][]byte{"archive-find": []byte("{")}, wantErr: "decode Archive find"},
		{name: "empty archive result", zone: "is1b", outputs: map[string][]byte{"archive-find": []byte("[]")}, wantErr: "no item"},
		{name: "archive identity mismatch", zone: "is1b", outputs: map[string][]byte{"archive-read": []byte(`{"ID":124,"Name":"other"}`)}, wantErr: "does not match"},
		{name: "disk plan API error", zone: "is1b", failStep: "disk-plan-find", wantErr: "injected CLI failure"},
		{name: "empty disk plan result", zone: "is1b", outputs: map[string][]byte{"disk-plan-find": []byte("[]")}, wantErr: "no item"},
		{name: "disk plan missing size data", zone: "is1b", outputs: map[string][]byte{"disk-plan-find": []byte(`[{"ID":4,"Name":"ssd"}]`)}, wantErr: "no size data"},
		{name: "disk plan identity mismatch", zone: "is1b", outputs: map[string][]byte{"disk-plan-read": []byte(`{"ID":5,"Name":"other"}`)}, wantErr: "does not match"},
		{name: "disk plan size mismatch", zone: "is1b", outputs: map[string][]byte{"disk-plan-read": []byte(`{"ID":4,"Name":"ssd","StorageClass":"ssd","Availability":"available","Size":[{"Availability":"available","DisplaySize":40,"DisplaySuffix":"GB","SizeMB":40960}]}`)}, wantErr: "does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{failStep: test.failStep, outputs: test.outputs}
			err := (scenario{client: fake, zone: test.zone}).run(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
