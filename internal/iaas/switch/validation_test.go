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

package switchresource

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name      string
		validator string
		request   any
		wantError string
	}{
		{name: "find", validator: "validateSwitchFindRequest", request: &swytch.FindRequest{Zone: "tk1v"}},
		{name: "find missing zone", validator: "validateSwitchFindRequest", request: &swytch.FindRequest{}, wantError: "--zone が必要"},
		{name: "find rejects all zone", validator: "validateSwitchFindRequest", request: &swytch.FindRequest{Zone: "all"}, wantError: `Zone に "all"`},
		{name: "read", validator: "validateSwitchReadRequest", request: &swytch.ReadRequest{Zone: "tk1v", ID: 123}},
		{name: "read missing zone", validator: "validateSwitchReadRequest", request: &swytch.ReadRequest{ID: 123}, wantError: "--zone が必要"},
		{name: "read missing id", validator: "validateSwitchReadRequest", request: &swytch.ReadRequest{Zone: "tk1v"}, wantError: "--id が必要"},
		{name: "read rejects all zone", validator: "validateSwitchReadRequest", request: &swytch.ReadRequest{Zone: "all", ID: 123}, wantError: `Zone に "all"`},
		{name: "create", validator: "validateSwitchCreateRequest", request: &swytch.CreateRequest{Zone: "tk1v", Name: "switch"}},
		{name: "create missing zone", validator: "validateSwitchCreateRequest", request: &swytch.CreateRequest{Name: "switch"}, wantError: "--zone が必要"},
		{name: "create missing name", validator: "validateSwitchCreateRequest", request: &swytch.CreateRequest{Zone: "tk1v"}, wantError: "--name が必要"},
		{name: "create rejects all zone", validator: "validateSwitchCreateRequest", request: &swytch.CreateRequest{Zone: "all", Name: "switch"}, wantError: `Zone に "all"`},
		{name: "update", validator: "validateSwitchUpdateRequest", request: &swytch.UpdateRequest{Zone: "tk1v", ID: 123}},
		{name: "update missing zone", validator: "validateSwitchUpdateRequest", request: &swytch.UpdateRequest{ID: 123}, wantError: "--zone が必要"},
		{name: "update missing id", validator: "validateSwitchUpdateRequest", request: &swytch.UpdateRequest{Zone: "tk1v"}, wantError: "--id が必要"},
		{name: "update rejects all zone", validator: "validateSwitchUpdateRequest", request: &swytch.UpdateRequest{Zone: "all", ID: 123}, wantError: `Zone に "all"`},
		{name: "delete", validator: "validateSwitchDeleteRequest", request: &swytch.DeleteRequest{Zone: "tk1v", ID: 123}},
		{name: "delete missing zone", validator: "validateSwitchDeleteRequest", request: &swytch.DeleteRequest{ID: 123}, wantError: "--zone が必要"},
		{name: "delete missing id", validator: "validateSwitchDeleteRequest", request: &swytch.DeleteRequest{Zone: "tk1v"}, wantError: "--id が必要"},
		{name: "delete rejects all zone", validator: "validateSwitchDeleteRequest", request: &swytch.DeleteRequest{Zone: "all", ID: 123}, wantError: `Zone に "all"`},
		{name: "find wrong request type", validator: "validateSwitchFindRequest", request: &swytch.ReadRequest{}, wantError: "型が不正"},
		{name: "read wrong request type", validator: "validateSwitchReadRequest", request: &swytch.FindRequest{}, wantError: "型が不正"},
		{name: "create wrong request type", validator: "validateSwitchCreateRequest", request: &swytch.FindRequest{}, wantError: "型が不正"},
		{name: "update wrong request type", validator: "validateSwitchUpdateRequest", request: &swytch.FindRequest{}, wantError: "型が不正"},
		{name: "delete wrong request type", validator: "validateSwitchDeleteRequest", request: &swytch.FindRequest{}, wantError: "型が不正"},
		{name: "unknown validator", validator: "unknown", request: &swytch.FindRequest{}, wantError: "未知の Switch"},
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
