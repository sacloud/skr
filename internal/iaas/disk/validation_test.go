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

package diskresource

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name      string
		validator string
		request   any
		wantError string
	}{
		{name: "find", validator: "validateDiskFindRequest", request: &disk.FindRequest{Zone: "tk1v"}},
		{name: "find missing zone", validator: "validateDiskFindRequest", request: &disk.FindRequest{}, wantError: "Zone が必要"},
		{name: "find rejects all zone", validator: "validateDiskFindRequest", request: &disk.FindRequest{Zone: zonesAll}, wantError: `Zone に "all"`},
		{name: "read", validator: "validateDiskReadRequest", request: &disk.ReadRequest{Zone: "tk1v", ID: 123}},
		{name: "read rejects all", validator: "validateDiskReadRequest", request: &disk.ReadRequest{Zone: zonesAll, ID: 123}, wantError: `Zone に "all"`},
		{name: "create", validator: "validateDiskCreateRequest", request: &disk.CreateRequest{
			Zone: "tk1v", Name: "disk", DiskPlanID: 4, Connection: types.EDiskConnection("virtio"), SizeGB: 20,
		}},
		{name: "create missing size", validator: "validateDiskCreateRequest", request: &disk.CreateRequest{
			Zone: "tk1v", Name: "disk", DiskPlanID: 4, Connection: types.EDiskConnection("virtio"),
		}, wantError: "SizeGB"},
		{name: "update", validator: "validateDiskUpdateRequest", request: &disk.UpdateRequest{
			Zone: "tk1v", ID: 123, Name: stringPointer("updated"),
		}},
		{name: "update no changes", validator: "validateDiskUpdateRequest", request: &disk.UpdateRequest{
			Zone: "tk1v", ID: 123,
		}, wantError: "更新するフィールド"},
		{name: "delete", validator: "validateDiskDeleteRequest", request: &disk.DeleteRequest{Zone: "tk1v", ID: 123}},
		{name: "delete missing id", validator: "validateDiskDeleteRequest", request: &disk.DeleteRequest{Zone: "tk1v"}, wantError: "ID が必要"},
		{name: "unknown validator", validator: "unknown", request: &disk.FindRequest{}, wantError: "未知"},
		{name: "wrong request type", validator: "validateDiskFindRequest", request: &disk.ReadRequest{}, wantError: "型が不正"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.validator, test.request)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("ValidateRequest() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ValidateRequest() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

const zonesAll = "all"

func stringPointer(value string) *string {
	return &value
}
