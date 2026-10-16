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
	"image"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type fakeCLI struct {
	item           *iconItem
	failAt         string
	existing       []iconItem
	deleted        []types.ID
	calls          []string
	badCreate      bool
	changeIdentity bool
}

func (f *fakeCLI) call(_ context.Context, step string, request map[string]any) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.failAt == step {
		return nil, errors.New("injected CLI failure")
	}
	if step == "profile-current" {
		return []byte("test-profile\n"), nil
	}
	switch step {
	case "preflight-find-query", "test-find-query", "verify-delete-query", "cleanup-find-query":
		name := request["Names"].([]string)[0]
		items := make([]iconItem, 0, len(f.existing)+1)
		if f.item != nil && f.item.Name == name {
			items = append(items, *f.item)
		}
		for _, existing := range f.existing {
			if existing.Name == name {
				items = append(items, existing)
			}
		}
		return json.Marshal(items)
	case "test-create":
		f.item = &iconItem{ID: 1234, Name: request["Name"].(string), Tags: request["Tags"].([]string)}
		if f.badCreate {
			return []byte("{"), nil
		}
		return json.Marshal(f.item)
	case "test-read", "test-read-updated", "cleanup-read":
		id := request["ID"].(types.ID)
		if f.item == nil || f.item.ID != id {
			return nil, errors.New("missing Icon")
		}
		item := *f.item
		if step == "cleanup-read" && f.changeIdentity {
			item.Name = "unexpected"
		}
		return json.Marshal(item)
	case "test-update":
		if f.item == nil || f.item.ID != request["ID"] {
			return nil, errors.New("missing Icon")
		}
		f.item.Name = request["Name"].(string)
		f.item.Tags = request["Tags"].([]string)
		return json.Marshal(f.item)
	case "cleanup-delete":
		if f.item == nil || f.item.ID != request["ID"].(types.ID) {
			return nil, errors.New("unsafe delete")
		}
		f.deleted = append(f.deleted, f.item.ID)
		f.item = nil
		return nil, nil
	default:
		return nil, errors.New("unexpected E2E step " + step)
	}
}

func TestScenarioCreatesUpdatesAndCleansIcon(t *testing.T) {
	fake := &fakeCLI{}
	if err := (scenario{client: fake}).run(context.Background(), "skr-e2e-icon-test", "/tmp/icon.png"); err != nil {
		t.Fatal(err)
	}
	if fake.item != nil || len(fake.deleted) != 1 || fake.deleted[0] != 1234 {
		t.Fatalf("Icon was not cleaned up safely: item=%+v deleted=%v", fake.item, fake.deleted)
	}
	if got, want := strings.Join(fake.calls, " "), "profile-current preflight-find-query test-create test-read test-update test-read-updated test-find-query cleanup-read cleanup-delete verify-delete-query"; got != want {
		t.Fatalf("scenario calls = %q, want %q", got, want)
	}
}

func TestScenarioRefusesExistingIconAndCleansAfterFailure(t *testing.T) {
	t.Run("pre-existing", func(t *testing.T) {
		fake := &fakeCLI{existing: []iconItem{{ID: 9876, Name: "skr-e2e-icon-test"}}}
		err := (scenario{client: fake}).run(context.Background(), "skr-e2e-icon-test", "/tmp/icon.png")
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("error = %v, want existing Icon refusal", err)
		}
		if len(fake.deleted) != 0 || fake.item != nil {
			t.Fatal("scenario modified a pre-existing Icon")
		}
	})
	t.Run("update failure", func(t *testing.T) {
		fake := &fakeCLI{failAt: "test-update"}
		err := (scenario{client: fake}).run(context.Background(), "skr-e2e-icon-test", "/tmp/icon.png")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("error = %v, want update error", err)
		}
		if fake.item != nil || len(fake.deleted) != 1 {
			t.Fatal("created Icon was not cleaned up after update failure")
		}
	})
	t.Run("bad create output", func(t *testing.T) {
		fake := &fakeCLI{badCreate: true}
		err := (scenario{client: fake}).run(context.Background(), "skr-e2e-icon-test", "/tmp/icon.png")
		if err == nil || !strings.Contains(err.Error(), "decode Icon") {
			t.Fatalf("error = %v, want create output decode error", err)
		}
		if fake.item != nil || len(fake.deleted) != 1 {
			t.Fatal("Icon with undecodable create output was not recovered and deleted")
		}
	})
	t.Run("cleanup identity mismatch", func(t *testing.T) {
		fake := &fakeCLI{failAt: "test-update", changeIdentity: true}
		err := (scenario{client: fake}).run(context.Background(), "skr-e2e-icon-test", "/tmp/icon.png")
		if err == nil || !strings.Contains(err.Error(), "refusing to delete") {
			t.Fatalf("error = %v, want unsafe cleanup refusal", err)
		}
		if len(fake.deleted) != 0 || fake.item == nil {
			t.Fatal("Icon with mismatched identity was deleted")
		}
	})
}

func TestIconArgsUsesImageFileAndRequestForFind(t *testing.T) {
	create, err := iconArgs("test-create", map[string]any{
		"Name": "skr-e2e-icon-test", "ImageFile": "/tmp/icon.png", "Tags": []string{"skr-e2e", "temporary"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(create, " "), "iaas-api icon create --name skr-e2e-icon-test --image-file /tmp/icon.png --tags skr-e2e,temporary"; got != want {
		t.Fatalf("create args = %q, want %q", got, want)
	}

	find, err := iconArgs("test-find-query", map[string]any{"Names": []string{"skr-e2e-icon-test"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(find, " "), `iaas-api icon find --request {"Names":["skr-e2e-icon-test"]} --query map({ID,Name,Tags})`; got != want {
		t.Fatalf("find args = %q, want %q", got, want)
	}

	verifyDelete, err := iconArgs("verify-delete-query", map[string]any{"Names": []string{"skr-e2e-icon-test"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(verifyDelete, " "), `iaas-api icon find --request {"Names":["skr-e2e-icon-test"]} --query map({ID,Name,Tags})`; got != want {
		t.Fatalf("delete verification args = %q, want %q", got, want)
	}
}

func TestCreateImageFileProducesPNG(t *testing.T) {
	path, err := createImageFile()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
	}()
	file, err := os.Open(path) //nolint:gosec // createImageFile returns a path created in the system temp directory.
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close generated image file: %v", err)
		}
	}()
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" || config.Width != 60 || config.Height != 56 {
		t.Fatalf("image = %s %dx%d, want PNG 60x56", format, config.Width, config.Height)
	}
}
