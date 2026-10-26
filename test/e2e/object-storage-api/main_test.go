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
	"time"
)

type fakeCLI struct {
	failStep     string
	createOutput []byte
	preflight    []byte
	verifyCreate []byte
	cleanupCheck []byte
	calls        []string
	args         map[string][]string
	permission   bool
	key          bool
	updated      bool
	bucketGone   bool
	responses    map[string][]byte
}

func (f *fakeCLI) call(_ context.Context, step, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = make(map[string][]string)
	}
	f.args[step] = append([]string(nil), args...)
	if output, ok := f.responses[step]; ok {
		return output, nil
	}
	if step == f.failStep {
		return nil, errors.New("injected CLI failure")
	}
	switch step {
	case "site-list":
		return []byte(`[{"id":"isk01"},{"id":"tky01"},{"id":"arc02"}]`), nil
	case "site-read":
		return json.Marshal(site{ID: args[3]})
	case "site-plans":
		return []byte(`[{"service_class_path":"objectstorage/arc02/bucket/1t","type":"archive","cluster_id":"arc02"}]`), nil
	case "site-status-read":
		return []byte(`{"accept_new":true,"status_code":{"id":1,"status":"ok"}}`), nil
	case "site-quota":
		return []byte(`{"num_buckets":20,"num_permissions":30}`), nil
	case "bucket-penalty", "bucket-usage", "bucket-quota":
		return []byte(`{"num_objects_per_bucket":0,"amount_gib_per_bucket":0}`), nil
	case "bucket-metering":
		now := time.Now().UTC()
		current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return json.Marshal([]map[string]string{{"year_month": current.AddDate(0, -1, 0).Format("2006-01")}, {"year_month": current.Format("2006-01")}})
	case "profile-current":
		return []byte("test-profile\n"), nil
	case "site-account-read":
		return []byte(`{"resource_id":"account-1"}`), nil
	case "bucket-preflight":
		if f.preflight != nil {
			return f.preflight, nil
		}
		return []byte(`[]`), nil
	case "bucket-cleanup-check", "bucket-cleanup-retry-check":
		if f.cleanupCheck != nil {
			return f.cleanupCheck, nil
		}
		if f.bucketGone {
			return []byte(`[]`), nil
		}
		return []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"bucket-resource"}]`), nil
	case "bucket-verify-delete", "bucket-delete-verify", "bucket-delete-retry-verify":
		return []byte(`[]`), nil
	case "bucket-create":
		if f.createOutput != nil {
			return f.createOutput, nil
		}
		return []byte(`{"name":"skr-e2e-0123456789abcdef","cluster_id":"isk01","plan":{"type":"standard","service_class_path":"objectstorage/isk01/bucket"}}`), nil
	case "bucket-verify-create":
		if f.verifyCreate != nil {
			return f.verifyCreate, nil
		}
		return []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"bucket-resource","plan":{"type":"standard","service_class_path":"objectstorage/isk01/bucket"}}]`), nil
	case "bucket-delete", "bucket-delete-retry":
		f.bucketGone = true
		return nil, nil
	case "permission-schema-preflight", "permission-preflight", "permission-verify-create", "permission-verify-update", "permission-cleanup-presence", "permission-cleanup-check", "permission-verify-delete":
		if !f.permission {
			return []byte(`[]`), nil
		}
		return []byte(`[` + f.permissionOutput() + `]`), nil
	case "permission-create":
		f.permission = true
		return []byte(fakePermission), nil
	case "permission-update":
		f.updated = true
		return []byte(f.permissionOutput()), nil
	case "permission-verify-create-read", "permission-verify-update-read", "permission-cleanup-check-read":
		return []byte(f.permissionOutput()), nil
	case "permission-delete":
		f.permission = false
		return nil, nil
	case "permission-key-preflight", "permission-key-cleanup-check", "permission-key-verify-delete", "permission-cleanup-keys":
		if !f.key {
			return []byte(`[]`), nil
		}
		return []byte(`[{"id":"key-1"}]`), nil
	case "permission-key-create":
		f.key = true
		return []byte(`{"id":"key-1","secret":"test-only-secret"}`), nil
	case "permission-key-read":
		return []byte(`{"id":"key-1","secret":""}`), nil
	case "permission-key-delete":
		f.key = false
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

const fakePermission = `{"id":1001,"display_name":"skr-e2e-0123456789abcdef","bucket_controls":[{"bucket_name":"skr-e2e-0123456789abcdef","can_read":true,"can_write":true}]}`

func (f *fakeCLI) permissionOutput() string {
	if f.updated {
		return strings.ReplaceAll(fakePermission, `"can_write":true`, `"can_write":false`)
	}
	return fakePermission
}

func TestScenarioCreatesAndCleansUpOneTemporaryBucket(t *testing.T) {
	fake := &fakeCLI{}
	err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"site-list", "site-read", "permission-create", "permission-update", "permission-verify-create-read", "permission-key-create", "permission-key-read", "permission-key-delete", "permission-delete", "bucket-delete", "bucket-verify-delete"} {
		if _, ok := fake.args[required]; !ok {
			t.Errorf("scenario did not execute %s", required)
		}
	}
	if fake.permission || fake.key || !fake.bucketGone {
		t.Fatalf("resources remain: permission %v, key %v, bucket %v", fake.permission, fake.key, !fake.bucketGone)
	}
	if got, want := strings.Join(fake.args["bucket-create"], " "), "object-storage-api bucket create isk01 skr-e2e-0123456789abcdef"; got != want {
		t.Errorf("create args = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.args["bucket-delete"], " "), "object-storage-api bucket delete isk01 skr-e2e-0123456789abcdef"; got != want {
		t.Errorf("delete args = %q, want %q", got, want)
	}
}

func TestScenarioRefusesExistingBucketAndMismatchedCreateIdentity(t *testing.T) {
	for _, test := range []struct {
		name         string
		preflight    []byte
		createOutput []byte
		wantErr      string
	}{
		{name: "existing bucket", preflight: []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"existing-resource"}]`), wantErr: "already exists"},
		{name: "mismatched create identity", createOutput: []byte(`{"name":"other","cluster_id":"isk01"}`), wantErr: "identity does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{preflight: test.preflight, createOutput: test.createOutput}
			err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
			for _, step := range fake.calls {
				if step == "bucket-delete" || step == "bucket-delete-retry" {
					t.Fatalf("scenario deleted a bucket after identity mismatch: calls %v", fake.calls)
				}
			}
		})
	}
}

func TestScenarioRequiresAndPreservesCreatedBucketResourceID(t *testing.T) {
	t.Run("missing created ID refuses deletion", func(t *testing.T) {
		fake := &fakeCLI{
			verifyCreate: []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":""}]`),
			cleanupCheck: []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"bucket-resource"}]`),
		}
		err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
		if err == nil || !strings.Contains(err.Error(), "no resource ID") {
			t.Fatalf("scenario error = %v, want missing resource ID", err)
		}
		for _, step := range fake.calls {
			if step == "bucket-delete" || step == "bucket-delete-retry" {
				t.Fatalf("scenario deleted a bucket without a known resource ID: calls %v", fake.calls)
			}
		}
	})

	t.Run("changed ID refuses deletion", func(t *testing.T) {
		fake := &fakeCLI{
			cleanupCheck: []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"different-resource"}]`),
		}
		err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
		if err == nil || !strings.Contains(err.Error(), "resource ID does not match") {
			t.Fatalf("scenario error = %v, want resource ID mismatch", err)
		}
		for _, step := range fake.calls {
			if step == "bucket-delete" || step == "bucket-delete-retry" {
				t.Fatalf("scenario deleted a bucket with a changed resource ID: calls %v", fake.calls)
			}
		}
	})
}

func TestScenarioRetriesCleanupAfterDeleteFailure(t *testing.T) {
	fake := &fakeCLI{failStep: "bucket-delete"}
	err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
	if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
		t.Fatalf("scenario error = %v, want the initial deletion failure", err)
	}
	for _, step := range fake.calls {
		if step == "bucket-delete-retry" {
			return
		}
	}
	t.Fatalf("scenario did not retry cleanup after deletion failed: calls %v", fake.calls)
}

func TestValidateSiteIDAllowsOnlyStandardSites(t *testing.T) {
	for _, test := range []struct {
		siteID  string
		wantErr bool
	}{
		{siteID: "isk01"},
		{siteID: "tky01"},
		{siteID: "arc02", wantErr: true},
		{siteID: "unknown", wantErr: true},
	} {
		t.Run(test.siteID, func(t *testing.T) {
			err := validateSiteID(test.siteID)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSiteID(%q) error = %v, wantErr %t", test.siteID, err, test.wantErr)
			}
		})
	}
}

func TestScenarioStopsWhenProfileOrAccountIsMissing(t *testing.T) {
	for _, test := range []struct {
		name     string
		failStep string
		wantErr  string
	}{
		{name: "profile", failStep: "profile-current", wantErr: "selected SDK profile"},
		{name: "site account", failStep: "site-account-read", wantErr: "site account must already exist"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCLI{failStep: test.failStep}
			err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("scenario error = %v, want %q", err, test.wantErr)
			}
			for _, step := range fake.calls {
				if step == "bucket-create" {
					t.Fatalf("scenario created a bucket after prerequisite failure: calls %v", fake.calls)
				}
			}
		})
	}
}
