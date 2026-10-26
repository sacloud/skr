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
	"time"
)

func testScenarioForAdditions(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	return &scenario{
		client:     fake,
		requestDir: t.TempDir(),
		checkURL:   func(context.Context, string) error { return nil },
	}
}

func TestVersionScenarioCoversLifecycle(t *testing.T) {
	fake := &fakeCLI{}
	s := testScenarioForAdditions(t, fake)
	fake.value = newFakeApp(namePrefix + "test")
	if err := s.versionScenario(context.Background(), "app-id", namePrefix+"test"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{
		"update-application", "read-updated-application", "list-versions", "read-version-status",
		"read-version", "list-traffic", "update-traffic", "list-traffic-after-update",
		"delete-version", "list-versions-after-delete",
	} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s: %v", step, fake.calls)
		}
	}
}

func TestVersionScenarioReportsFailures(t *testing.T) {
	t.Run("update api failure", func(t *testing.T) {
		fake := &fakeCLI{failStep: "update-application"}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		err := s.versionScenario(context.Background(), "app-id", namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want injected CLI failure", err)
		}
	})
	t.Run("unexpected version count", func(t *testing.T) {
		fake := &fakeCLI{}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		fake.dropVersions = 1
		err := s.versionScenario(context.Background(), "app-id", namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "want 2") {
			t.Fatalf("error = %v, want unexpected version count", err)
		}
	})
	t.Run("traffic percentage mismatch", func(t *testing.T) {
		fake := &fakeCLI{ignoreTrafficUpdate: true}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		fake.traffic = []trafficEntry{{VersionName: "updated", IsLatestVersion: true, Percent: 50}}
		err := s.versionScenario(context.Background(), "app-id", namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "traffic percentages") {
			t.Fatalf("error = %v, want traffic percentage mismatch", err)
		}
	})
	t.Run("version remains after delete", func(t *testing.T) {
		fake := &fakeCLI{ignoreVersionDelete: true}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		err := s.versionScenario(context.Background(), "app-id", namePrefix+"test")
		if err == nil || !strings.Contains(err.Error(), "want only") {
			t.Fatalf("error = %v, want version remains after delete", err)
		}
	})
}

func TestVersionScenarioWaitsForHealthyAndRejectsUnHealthy(t *testing.T) {
	t.Run("unhealthy", func(t *testing.T) {
		fake := &fakeCLI{status: "UnHealthy"}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		err := s.waitVersionHealthy(context.Background(), "app-id", "version-new")
		if err == nil || !strings.Contains(err.Error(), "UnHealthy") {
			t.Fatalf("error = %v, want UnHealthy", err)
		}
	})
	t.Run("unexpected status", func(t *testing.T) {
		fake := &fakeCLI{status: "Deleting"}
		s := testScenarioForAdditions(t, fake)
		fake.value = newFakeApp(namePrefix + "test")
		err := s.waitVersionHealthy(context.Background(), "app-id", "version-new")
		if err == nil || !strings.Contains(err.Error(), "unexpected AppRun version status") {
			t.Fatalf("error = %v, want unexpected status", err)
		}
	})
}

func TestNewestAndPreviousIdentifiesUpdatedVersion(t *testing.T) {
	now := time.Now().UTC()
	versions := []versionSummary{
		{ID: "old", CreatedAt: now.Add(-time.Minute).Format(time.RFC3339)},
		{ID: "new", CreatedAt: now.Format(time.RFC3339)},
	}
	latest, previous, err := newestAndPrevious(versions)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "new" || previous.ID != "old" {
		t.Fatalf("latest = %s, previous = %s; want new and old", latest.ID, previous.ID)
	}
	if _, _, err := newestAndPrevious(versions[:1]); err == nil {
		t.Fatal("single version must be rejected")
	}
	if _, _, err := newestAndPrevious([]versionSummary{{ID: "x", CreatedAt: "not-a-time"}, {ID: "y"}}); err == nil {
		t.Fatal("invalid created_at must be rejected")
	}
}

func TestCheckUpdatedApplicationRejectsMismatch(t *testing.T) {
	valid := app{ID: "app-id", Name: "name"}
	valid.Components = append(valid.Components, struct {
		MaxCPU       string `json:"max_cpu"`
		MaxMemory    string `json:"max_memory"`
		DeploySource struct {
			ContainerRegistry struct {
				Image string `json:"image"`
			} `json:"container_registry"`
		} `json:"deploy_source"`
	}{MaxCPU: updatedMaxCPU, MaxMemory: updatedMaxMemory})
	if err := checkUpdatedApplication(valid, "app-id", "name"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := checkUpdatedApplication(valid, "another-id", "name"); err == nil {
		t.Fatal("identity mismatch must be rejected")
	}
	wrongMemory := valid
	wrongMemory.Components[0].MaxMemory = "1Gi"
	if err := checkUpdatedApplication(wrongMemory, "app-id", "name"); err == nil {
		t.Fatal("memory mismatch must be rejected")
	}
	wrongCPU := valid
	wrongCPU.Components[0].MaxCPU = "0.5"
	if err := checkUpdatedApplication(wrongCPU, "app-id", "name"); err == nil {
		t.Fatal("cpu mismatch must be rejected")
	}
}

func TestCheckVersionDetailRejectsMismatch(t *testing.T) {
	detail := versionDetail{ID: "version-new", Port: port, MinScale: 0, MaxScale: 1}
	detail.Components = append(detail.Components, struct {
		Name         string `json:"name"`
		MaxCPU       string `json:"max_cpu"`
		MaxMemory    string `json:"max_memory"`
		DeploySource struct {
			ContainerRegistry struct {
				Image string `json:"image"`
			} `json:"container_registry"`
		} `json:"deploy_source"`
	}{MaxCPU: updatedMaxCPU, MaxMemory: updatedMaxMemory, DeploySource: validDeploySourceForTest()})
	if err := checkVersionDetail(detail, "version-new"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := checkVersionDetail(detail, "version-other"); err == nil {
		t.Fatal("identity mismatch must be rejected")
	}
	detail.Components[0].MaxMemory = "1Gi"
	if err := checkVersionDetail(detail, "version-new"); err == nil {
		t.Fatal("memory mismatch must be rejected")
	}
	detail.Components[0].MaxMemory = updatedMaxMemory
	detail.Components[0].MaxCPU = "0.5"
	if err := checkVersionDetail(detail, "version-new"); err == nil {
		t.Fatal("cpu mismatch must be rejected")
	}
}

func validDeploySourceForTest() struct {
	ContainerRegistry struct {
		Image string `json:"image"`
	} `json:"container_registry"`
} {
	var result struct {
		ContainerRegistry struct {
			Image string `json:"image"`
		} `json:"container_registry"`
	}
	result.ContainerRegistry.Image = image
	return result
}

func TestUpdateRequestUsesProtectedFile(t *testing.T) {
	fake := &fakeCLI{}
	s := testScenarioForAdditions(t, fake)
	path, err := s.writeUpdateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "update.json") {
		t.Fatalf("update request path = %q, want update.json", path)
	}
	// fake は保護されたファイルを使わないリクエストを拒否する
	fake.value = newFakeApp("app")
	fake.calls = nil
	if _, err := fake.call(context.Background(), "update-application", "application", "update", "app-id", "--request", "@"+path); err != nil {
		t.Fatalf("update request with @path must be accepted: %v", err)
	}
	if _, err := fake.call(context.Background(), "update-application", "application", "update", "app-id", "--request", "{}"); err == nil {
		t.Fatal("inline update request must be rejected in favor of the protected file")
	}
}
