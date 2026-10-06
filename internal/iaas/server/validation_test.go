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

package serverresource

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name      string
		validator string
		request   any
		wantErr   string
	}{
		{name: "find", validator: "validateServerFindRequest", request: &server.FindRequest{Zone: "tk1v"}},
		{name: "find missing zone", validator: "validateServerFindRequest", request: &server.FindRequest{}, wantErr: "Zone が必要"},
		{name: "find all zones", validator: "validateServerFindRequest", request: &server.FindRequest{Zone: "all"}},
		{name: "read", validator: "validateServerReadRequest", request: &server.ReadRequest{Zone: "tk1v", ID: 123}},
		{name: "read rejects all", validator: "validateServerReadRequest", request: &server.ReadRequest{Zone: "all", ID: 123}, wantErr: "find のみ"},
		{name: "create", validator: "validateServerCreateRequest", request: &server.CreateRequest{Zone: "tk1v", Name: "server", CPU: 1, MemoryGB: 1}},
		{name: "create missing name", validator: "validateServerCreateRequest", request: &server.CreateRequest{Zone: "tk1v"}, wantErr: "Name が必要"},
		{name: "create missing plan", validator: "validateServerCreateRequest", request: &server.CreateRequest{Zone: "tk1v", Name: "server"}, wantErr: "CPU と MemoryGB"},
		{name: "update", validator: "validateServerUpdateRequest", request: &server.UpdateRequest{Zone: "tk1v", ID: 123, Name: stringPointer("server")}},
		{name: "update empty", validator: "validateServerUpdateRequest", request: &server.UpdateRequest{Zone: "tk1v", ID: 123}, wantErr: "更新するフィールド"},
		{name: "delete", validator: "validateServerDeleteRequest", request: &server.DeleteRequest{Zone: "tk1v", ID: 123}},
		{name: "delete missing id", validator: "validateServerDeleteRequest", request: &server.DeleteRequest{Zone: "tk1v"}, wantErr: "ID が必要"},
		{name: "unknown", validator: "unknown", request: &server.FindRequest{Zone: "tk1v"}, wantErr: "未知の Server"},
		{name: "wrong type", validator: "validateServerFindRequest", request: &server.ReadRequest{Zone: "tk1v", ID: 123}, wantErr: "型が不正"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.validator, test.request)
			if test.wantErr == "" && err != nil {
				t.Fatalf("ValidateRequest() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("ValidateRequest() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateServerFindRequestAllowsZoneAll(t *testing.T) {
	if err := ValidateRequest("validateServerFindRequest", &server.FindRequest{Zone: "all"}); err != nil {
		t.Fatalf("ValidateRequest() error = %v, want nil", err)
	}
}

func TestValidateServerReadRequestRejectsZeroID(t *testing.T) {
	err := ValidateRequest("validateServerReadRequest", &server.ReadRequest{Zone: "tk1v"})
	if err == nil || !strings.Contains(err.Error(), "ID が必要") {
		t.Fatalf("ValidateRequest() error = %v, want missing ID error", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
