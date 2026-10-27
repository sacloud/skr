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

package iaas

import (
	"context"
	"errors"
	"net/http"
	"testing"

	sdk "github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	iconSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
)

func TestIconSearchAndPaging(t *testing.T) {
	server := NewTestServer(Config{})
	t.Cleanup(server.Close)
	ctx := context.Background()
	service := iconSDK.New(server)
	for _, item := range []struct {
		name string
		tags []string
	}{
		{"test alpha", []string{"test", "first"}},
		{"test beta", []string{"test", "second"}},
		{"other", []string{"other"}},
	} {
		if _, err := service.Create(&iconSDK.CreateRequest{
			Name: item.name, Image: "aW1hZ2U=", Tags: item.tags,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name    string
		request iconSDK.FindRequest
		total   int
		names   []string
	}{
		{"name", iconSDK.FindRequest{Names: []string{"test"}}, 2, []string{"test alpha", "test beta"}},
		{"name and tags", iconSDK.FindRequest{Names: []string{"test"}, Tags: []string{"second"}}, 1, []string{"test beta"}},
		{"all tags", iconSDK.FindRequest{Tags: []string{"test", "first"}}, 1, []string{"test alpha"}},
		{"missing", iconSDK.FindRequest{Names: []string{"missing"}}, 0, nil},
		{"page", iconSDK.FindRequest{Names: []string{"test"}, Count: 1, From: 1}, 2, []string{"test beta"}},
		{"past end", iconSDK.FindRequest{From: 10}, 3, nil},
		{"sort", iconSDK.FindRequest{Sort: search.SortKeys{search.SortKeyDesc("Name")}}, 3, []string{"test beta", "test alpha", "other"}},
		{"scope", iconSDK.FindRequest{Scope: types.Scopes.User}, 3, []string{"test alpha", "test beta", "other"}},
		{"shared", iconSDK.FindRequest{Scope: types.Scopes.Shared}, 0, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition, err := test.request.ToRequestParameter()
			if err != nil {
				t.Fatal(err)
			}
			result, err := sdk.NewIconOp(server).Find(ctx, condition)
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != test.total || result.Count != len(test.names) || len(result.Icons) != len(test.names) || result.From != test.request.From {
				t.Fatalf("result = %+v; want total=%d count=%d from=%d", result, test.total, len(test.names), test.request.From)
			}
			for i, name := range test.names {
				if result.Icons[i].Name != name {
					t.Fatalf("result[%d] = %q, want %q", i, result.Icons[i].Name, name)
				}
			}
		})
	}
}

func TestIconMissingResourceReturnsNotFound(t *testing.T) {
	server := NewTestServer(Config{})
	t.Cleanup(server.Close)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			_, err := server.Do(context.Background(), method, "https://example.invalid/icon/9999", nil)
			var apiErr sdk.APIError
			if !errors.As(err, &apiErr) || apiErr.ResponseCode() != http.StatusNotFound {
				t.Fatalf("error = %v, want API 404", err)
			}
		})
	}
}

func TestZoneFindReturnsConfiguredZonesWithIDs(t *testing.T) {
	server := NewTestServer(Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)
	ctx := context.Background()

	result, err := sdk.NewZoneOp(server).Find(ctx, &sdk.FindCondition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Zones) != 2 || result.Zones[0].ID != types.ID(1) || result.Zones[0].Name != "test-zone-a" ||
		result.Zones[1].ID != types.ID(2) || result.Zones[1].Name != "test-zone-b" {
		t.Fatalf("zone find = %#v, want configured zones with IDs 1 and 2", result.Zones)
	}
}

func TestZoneReadReturnsConfiguredZone(t *testing.T) {
	server := NewTestServer(Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	found, err := sdk.NewZoneOp(server).Read(context.Background(), types.ID(2))
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != types.ID(2) || found.Name != "test-zone-b" {
		t.Fatalf("zone read = %#v, want ID 2 and name test-zone-b", found)
	}
}

func TestZoneMissingResourceReturnsNotFound(t *testing.T) {
	server := NewTestServer(Config{Zones: []string{"test-zone-a"}})
	t.Cleanup(server.Close)
	_, err := sdk.NewZoneOp(server).Read(context.Background(), types.ID(99))
	var apiErr sdk.APIError
	if !errors.As(err, &apiErr) || apiErr.ResponseCode() != http.StatusNotFound {
		t.Fatalf("error = %v, want API 404", err)
	}
}
