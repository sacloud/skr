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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type fakeCLI struct {
	failStep string
	find     map[string][]byte
	read     map[string][]byte
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
	switch step {
	case "profile-current":
		return []byte("test-profile\n"), nil
	case "internetplan-find":
		if data, ok := f.find[step]; ok {
			return data, nil
		}
		return []byte(`[{"ID":11,"Name":"internet-plan","BandWidthMbps":100,"Availability":"available"}]`), nil
	case "internetplan-read":
		if data, ok := f.read[step]; ok {
			return data, nil
		}
		return []byte(`{"ID":11,"Name":"internet-plan","BandWidthMbps":100,"Availability":"available"}`), nil
	case "licenseinfo-find":
		if data, ok := f.find[step]; ok {
			return data, nil
		}
		return []byte(`{"Total":1,"From":0,"Count":1,"LicenseInfo":[{"ID":42,"Name":"license-info"}]}`), nil
	case "privatehostplan-find":
		if data, ok := f.find[step]; ok {
			return data, nil
		}
		return []byte(`[{"ID":8,"Name":"private-host-plan","Class":"standard","CPU":20,"CPUModel":"model","Dedicated":true,"MemoryMB":229376,"Availability":"available"}]`), nil
	case "privatehostplan-read":
		if data, ok := f.read[step]; ok {
			return data, nil
		}
		return []byte(`{"ID":8,"Name":"private-host-plan","Class":"standard","CPU":20,"CPUModel":"model","Dedicated":true,"MemoryMB":229376,"Availability":"available"}`), nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioFindsAndReadsProductPlans(t *testing.T) {
	fake := &fakeCLI{}
	if err := (scenario{client: fake}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		step string
		want string
	}{
		{step: "internetplan-find", want: "iaas-api product internet find --zone is1b --count 1 --from 0 --query " + internetPlanQuery},
		{step: "internetplan-read", want: "iaas-api product internet read --zone is1b --id 11"},
		{step: "licenseinfo-find", want: "iaas-api product license find --count 1 --from 0 --query " + licenseInfoQuery},
		{step: "privatehostplan-find", want: "iaas-api product privatehost find --zone is1b --count 1 --from 0 --query " + privateHostPlanQuery},
		{step: "privatehostplan-read", want: "iaas-api product privatehost read --zone is1b --id 8"},
	}
	for _, check := range checks {
		if got := strings.Join(fake.args[check.step], " "); got != check.want {
			t.Errorf("%s args = %q, want %q", check.step, got, check.want)
		}
	}
	if got, want := strings.Join(fake.calls, " "),
		"profile-current internetplan-find internetplan-read licenseinfo-find privatehostplan-find privatehostplan-read"; got != want {
		t.Errorf("scenario calls = %q, want %q", got, want)
	}
}

func TestScenarioRejectsInvalidIdentityOrMismatchedReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		find    map[string][]byte
		read    map[string][]byte
		wantErr string
	}{
		{name: "empty InternetPlan", find: map[string][]byte{"internetplan-find": []byte(`[]`)}, wantErr: "InternetPlan Find returned"},
		{name: "invalid InternetPlan JSON", find: map[string][]byte{"internetplan-find": []byte("not-json")}, wantErr: "decode InternetPlan find"},
		{name: "mismatched InternetPlan Read", read: map[string][]byte{"internetplan-read": []byte(`{"ID":12,"Name":"other","BandWidthMbps":100,"Availability":"available"}`)}, wantErr: "InternetPlan Read did not match"},
		{name: "inconsistent empty LicenseInfo", find: map[string][]byte{"licenseinfo-find": []byte(`{"Total":1,"From":0,"Count":1,"LicenseInfo":[]}`)}, wantErr: "inconsistent result"},
		{name: "valid empty LicenseInfo", find: map[string][]byte{"licenseinfo-find": []byte(`{"Total":0,"From":0,"Count":0,"LicenseInfo":[]}`)}},
		{name: "empty PrivateHostPlan", find: map[string][]byte{"privatehostplan-find": []byte(`[]`)}, wantErr: "PrivateHostPlan Find returned"},
		{name: "mismatched PrivateHostPlan Read", read: map[string][]byte{"privatehostplan-read": []byte(`{"ID":9,"Name":"other","MemoryMB":229376}`)}, wantErr: "PrivateHostPlan Read did not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := (scenario{client: &fakeCLI{find: test.find, read: test.read}}).run(context.Background())
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestScenarioPropagatesProfileAndAPIErrors(t *testing.T) {
	for _, test := range []struct {
		step    string
		wantErr string
	}{
		{step: "profile-current", wantErr: "selected SDK profile"},
		{step: "internetplan-find", wantErr: "injected CLI failure"},
		{step: "internetplan-read", wantErr: "injected CLI failure"},
		{step: "licenseinfo-find", wantErr: "injected CLI failure"},
		{step: "privatehostplan-find", wantErr: "injected CLI failure"},
		{step: "privatehostplan-read", wantErr: "injected CLI failure"},
	} {
		t.Run(test.step, func(t *testing.T) {
			err := (scenario{client: &fakeCLI{failStep: test.step}}).run(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCLIRunnerRecordsPrivateEvidence(t *testing.T) {
	binary, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo executable not found")
	}
	recorder, err := evidence.CreateAt(filepath.Join(t.TempDir(), "tmp", "iaas-product-plans"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: binary, evidence: recorder}
	output, err := runner.call(context.Background(), "internetplan-find", "iaas-api", "product", "internet", "find")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "product internet") {
		t.Fatalf("command output = %q, want echoed API route command", output)
	}
	info, err := os.Stat(filepath.Join(recorder.Dir(), "001-internetplan-find.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence file permissions = %04o, want 0600", info.Mode().Perm())
	}
}
