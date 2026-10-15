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

package diskapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
)

func TestTypedRequestValidators(t *testing.T) {
	tests := []struct {
		name      string
		validate  func() error
		wantError string
	}{
		{name: "find", validate: func() error { return ValidateFindRequest(&disk.FindRequest{Zone: "tk1v"}) }},
		{name: "find missing zone", validate: func() error { return ValidateFindRequest(&disk.FindRequest{}) }, wantError: "Zone が必要"},
		{name: "find rejects all zone", validate: func() error { return ValidateFindRequest(&disk.FindRequest{Zone: zonesAll}) }, wantError: `Zone に "all"`},
		{name: "read", validate: func() error { return ValidateReadRequest(&disk.ReadRequest{Zone: "tk1v", ID: 123}) }},
		{name: "read rejects all", validate: func() error { return ValidateReadRequest(&disk.ReadRequest{Zone: zonesAll, ID: 123}) }, wantError: `Zone に "all"`},
		{name: "create", validate: func() error {
			return ValidateCreateRequest(&disk.CreateRequest{
				Zone: "tk1v", Name: "disk", DiskPlanID: 4, Connection: types.EDiskConnection("virtio"), SizeGB: 20,
			})
		}},
		{name: "create missing size", validate: func() error {
			return ValidateCreateRequest(&disk.CreateRequest{
				Zone: "tk1v", Name: "disk", DiskPlanID: 4, Connection: types.EDiskConnection("virtio"),
			})
		}, wantError: "SizeGB"},
		{name: "update", validate: func() error {
			return ValidateUpdateRequest(&disk.UpdateRequest{Zone: "tk1v", ID: 123, Name: stringPointer("updated")})
		}},
		{name: "update no changes", validate: func() error { return ValidateUpdateRequest(&disk.UpdateRequest{Zone: "tk1v", ID: 123}) }, wantError: "更新するフィールド"},
		{name: "delete", validate: func() error { return ValidateDeleteRequest(&disk.DeleteRequest{Zone: "tk1v", ID: 123}) }},
		{name: "delete missing id", validate: func() error { return ValidateDeleteRequest(&disk.DeleteRequest{Zone: "tk1v"}) }, wantError: "ID が必要"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validator() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validator() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

const zonesAll = "all"

func stringPointer(value string) *string {
	return &value
}
