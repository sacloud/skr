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

package containerregistryapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	sdk "github.com/sacloud/sacloud-sdk-go/service/iaas/containerregistry"
)

type apiCall struct {
	method string
	path   string
	body   any
}

type apiCaller struct {
	calls  []apiCall
	onCall func(apiCall)
}

func (c *apiCaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	call := apiCall{method: method, path: parsed.Path, body: body}
	c.calls = append(c.calls, call)
	if c.onCall != nil {
		c.onCall(call)
	}

	switch {
	case strings.HasSuffix(parsed.Path, "/containerregistry/users") && method == "GET":
		return []byte(`{"ContainerRegistry":{"Users":[{"UserName":"builder","Permission":"readwrite","Password":"not-returned"}]}}`), nil
	case strings.HasSuffix(parsed.Path, "/containerregistry/users") && method == "POST":
		return nil, nil
	case strings.HasSuffix(parsed.Path, "/containerregistry/users/builder") && method == "PUT":
		return nil, nil
	case strings.HasSuffix(parsed.Path, "/containerregistry/users/builder") && method == "DELETE":
		return nil, nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem") && method == "GET":
		return []byte(`{"Total":1,"Count":1,"CommonServiceItems":[{"ID":123,"Name":"registry","Description":"test"}]}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem") && method == "POST":
		return []byte(`{"CommonServiceItem":{"ID":123,"Name":"registry","Description":"test"}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "GET":
		return []byte(`{"CommonServiceItem":{"ID":123,"Name":"registry","Description":"test","Tags":["keep"],"Icon":{"ID":456},"SettingsHash":"current-hash","Settings":{"ContainerRegistry":{"public":"none","virtual_domain":"registry.example.test"}}}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "PUT":
		return []byte(`{"CommonServiceItem":{"ID":123,"Name":"registry","Description":"updated"}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "DELETE":
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected request %s %s", method, parsed.Path)
	}
}

func TestMetadataUpdatePreservesConcurrentUsersAndCurrentFields(t *testing.T) {
	users := map[string]bool{"builder": true}
	caller := &apiCaller{onCall: func(call apiCall) {
		if call.method == "GET" && strings.HasSuffix(call.path, "/commonserviceitem/123") {
			users["concurrent-user"] = true
		}
		if strings.Contains(call.path, "/containerregistry/users") {
			t.Errorf("metadata update accessed user API: %s %s", call.method, call.path)
			if call.method == "DELETE" {
				delete(users, filepath.Base(call.path))
			}
		}
	}}
	description := "updated"
	if _, err := newSDKAPI(caller).Update(context.Background(), &sdk.UpdateRequest{
		ID: 123, Description: &description,
	}); err != nil {
		t.Fatal(err)
	}
	if !users["builder"] || !users["concurrent-user"] {
		t.Fatalf("metadata update removed users: %v", users)
	}
	if len(caller.calls) != 2 || caller.calls[1].method != "PUT" {
		t.Fatalf("metadata update calls = %+v", caller.calls)
	}
	body, err := json.Marshal(caller.calls[1].body)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"Name":"registry"`, `"Description":"updated"`, `"Tags":["keep"]`,
		`"ID":456`, `"SettingsHash":"current-hash"`,
		`"public":"none"`, `"virtual_domain":"registry.example.test"`,
	} {
		if !strings.Contains(string(body), field) {
			t.Errorf("update body missing %s: %s", field, body)
		}
	}
}

func TestSDKAPIUsesPublicSDKAndSanitizesUsers(t *testing.T) {
	caller := &apiCaller{}
	api := newSDKAPI(caller)
	ctx := context.Background()

	registries, err := api.Find(ctx, &sdk.FindRequest{Names: []string{"registry"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(registries) != 1 || registries[0].ID != 123 || registries[0].Name != "registry" {
		t.Fatalf("Find result = %+v", registries)
	}

	created, err := api.Create(ctx, &sdk.CreateRequest{Name: "registry", SubDomainLabel: "registry"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 123 || created.Name != "registry" {
		t.Fatalf("Create result = %+v", created)
	}
	createBody, err := json.Marshal(caller.calls[1].body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(createBody), `"Name":"registry"`) ||
		!strings.Contains(string(createBody), `"registry_name":"registry"`) {
		t.Fatalf("Create SDK request is missing the registry name: %s", createBody)
	}

	read, err := api.Read(ctx, &sdk.ReadRequest{ID: 123})
	if err != nil {
		t.Fatal(err)
	}
	if read.ID != 123 || read.Name != "registry" {
		t.Fatalf("Read result = %+v", read)
	}

	description := "updated"
	updated, err := api.Update(ctx, &sdk.UpdateRequest{ID: 123, Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != 123 || updated.Description != description {
		t.Fatalf("Update result = %+v", updated)
	}

	users, err := api.ListUsers(ctx, 123)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].UserName != "builder" || users[0].Permission != types.ContainerRegistryPermissions.ReadWrite {
		t.Fatalf("ListUsers result = %+v", users)
	}
	userJSON, err := json.Marshal(users)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(userJSON), "Password") || strings.Contains(string(userJSON), "not-returned") {
		t.Fatalf("user output contains a password: %s", userJSON)
	}

	const password = "test-only-secret"
	if err := api.AddUser(ctx, 123, &iaas.ContainerRegistryUserCreateRequest{
		UserName:   "builder",
		Password:   password,
		Permission: types.ContainerRegistryPermissions.ReadWrite,
	}); err != nil {
		t.Fatal(err)
	}
	if err := api.UpdateUser(ctx, 123, "builder", &iaas.ContainerRegistryUserUpdateRequest{
		Permission: types.ContainerRegistryPermissions.ReadOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteUser(ctx, 123, "builder"); err != nil {
		t.Fatal(err)
	}
	if err := api.Delete(ctx, &sdk.DeleteRequest{ID: 123}); err != nil {
		t.Fatal(err)
	}

	if len(caller.calls) != 10 {
		t.Fatalf("API calls = %d; want 10: %+v", len(caller.calls), caller.calls)
	}
	if !strings.HasSuffix(caller.calls[0].path, "/commonserviceitem") || caller.calls[0].method != "GET" {
		t.Fatalf("Find request = %+v", caller.calls[0])
	}
	if !strings.HasSuffix(caller.calls[1].path, "/commonserviceitem") || caller.calls[1].method != "POST" {
		t.Fatalf("Create request = %+v", caller.calls[1])
	}
	var addCall *apiCall
	for i := range caller.calls {
		if caller.calls[i].method == "POST" && strings.HasSuffix(caller.calls[i].path, "/containerregistry/users") {
			addCall = &caller.calls[i]
			break
		}
	}
	if addCall == nil {
		t.Fatal("AddUser API call was not made")
	}
	addBody, err := json.Marshal(addCall.body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(addBody), password) {
		t.Fatalf("AddUser SDK request did not contain the provided password: %s", addBody)
	}
}
