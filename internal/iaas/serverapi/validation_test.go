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

package serverapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func TestTypedRequestValidators(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
		wantErr  string
	}{
		{name: "find", validate: func() error { return ValidateFindRequest(&server.FindRequest{Zone: "tk1v"}) }},
		{name: "find missing zone", validate: func() error { return ValidateFindRequest(&server.FindRequest{}) }, wantErr: "Zone が必要"},
		{name: "find rejects all zones", validate: func() error { return ValidateFindRequest(&server.FindRequest{Zone: "all"}) }, wantErr: `Zone に "all"`},
		{name: "read", validate: func() error { return ValidateReadRequest(&server.ReadRequest{Zone: "tk1v", ID: 123}) }},
		{name: "read rejects all", validate: func() error { return ValidateReadRequest(&server.ReadRequest{Zone: "all", ID: 123}) }, wantErr: `Zone に "all"`},
		{name: "create", validate: func() error {
			return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v", Name: "server", CPU: 1, MemoryGB: 1})
		}},
		{name: "create with no-wait", validate: func() error {
			return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v", Name: "server", CPU: 1, MemoryGB: 1, NoWait: true})
		}},
		{name: "create with boot-after-create", validate: func() error {
			return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v", Name: "server", CPU: 1, MemoryGB: 1, BootAfterCreate: true})
		}},
		{name: "create rejects no-wait with boot-after-create", validate: func() error {
			return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v", Name: "server", CPU: 1, MemoryGB: 1, NoWait: true, BootAfterCreate: true})
		}, wantErr: "NoWait=true と BootAfterCreate=true は併用できません"},
		{name: "create missing name", validate: func() error { return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v"}) }, wantErr: "Name が必要"},
		{name: "create missing plan", validate: func() error { return ValidateCreateRequest(&server.CreateRequest{Zone: "tk1v", Name: "server"}) }, wantErr: "CPU と MemoryGB"},
		{name: "boot", validate: func() error {
			return ValidateBootRequest(&server.BootRequest{Zone: "tk1v", ID: 123, UserData: "#cloud-config"})
		}},
		{name: "boot missing ID", validate: func() error { return ValidateBootRequest(&server.BootRequest{Zone: "tk1v", UserData: "#cloud-config"}) }, wantErr: "ID が必要"},
		{name: "boot without user-data", validate: func() error { return ValidateBootRequest(&server.BootRequest{Zone: "tk1v", ID: 123}) }},
		{name: "update", validate: func() error {
			return ValidateUpdateRequest(&server.UpdateRequest{Zone: "tk1v", ID: 123, Name: stringPointer("server")})
		}},
		{name: "update empty", validate: func() error { return ValidateUpdateRequest(&server.UpdateRequest{Zone: "tk1v", ID: 123}) }, wantErr: "更新するフィールド"},
		{name: "delete", validate: func() error { return ValidateDeleteRequest(&server.DeleteRequest{Zone: "tk1v", ID: 123}) }},
		{name: "delete missing id", validate: func() error { return ValidateDeleteRequest(&server.DeleteRequest{Zone: "tk1v"}) }, wantErr: "ID が必要"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.validate()
			if test.wantErr == "" && err != nil {
				t.Fatalf("validator() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("validator() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateServerReadRequestRejectsZeroID(t *testing.T) {
	err := ValidateReadRequest(&server.ReadRequest{Zone: "tk1v"})
	if err == nil || !strings.Contains(err.Error(), "ID が必要") {
		t.Fatalf("validator() error = %v, want missing ID error", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
