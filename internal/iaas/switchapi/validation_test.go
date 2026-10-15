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

package switchapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
)

func TestTypedRequestValidators(t *testing.T) {
	tests := []struct {
		name      string
		validate  func() error
		wantError string
	}{
		{name: "find", validate: func() error { return ValidateFindRequest(&swytch.FindRequest{Zone: "tk1v"}) }},
		{name: "find missing zone", validate: func() error { return ValidateFindRequest(&swytch.FindRequest{}) }, wantError: "--zone が必要"},
		{name: "find rejects all zone", validate: func() error { return ValidateFindRequest(&swytch.FindRequest{Zone: "all"}) }, wantError: `Zone に "all"`},
		{name: "read", validate: func() error { return ValidateReadRequest(&swytch.ReadRequest{Zone: "tk1v", ID: 123}) }},
		{name: "read missing zone", validate: func() error { return ValidateReadRequest(&swytch.ReadRequest{ID: 123}) }, wantError: "--zone が必要"},
		{name: "read missing id", validate: func() error { return ValidateReadRequest(&swytch.ReadRequest{Zone: "tk1v"}) }, wantError: "--id が必要"},
		{name: "read rejects all zone", validate: func() error { return ValidateReadRequest(&swytch.ReadRequest{Zone: "all", ID: 123}) }, wantError: `Zone に "all"`},
		{name: "create", validate: func() error { return ValidateCreateRequest(&swytch.CreateRequest{Zone: "tk1v", Name: "switch"}) }},
		{name: "create missing zone", validate: func() error { return ValidateCreateRequest(&swytch.CreateRequest{Name: "switch"}) }, wantError: "--zone が必要"},
		{name: "create missing name", validate: func() error { return ValidateCreateRequest(&swytch.CreateRequest{Zone: "tk1v"}) }, wantError: "--name が必要"},
		{name: "create rejects all zone", validate: func() error { return ValidateCreateRequest(&swytch.CreateRequest{Zone: "all", Name: "switch"}) }, wantError: `Zone に "all"`},
		{name: "update", validate: func() error { return ValidateUpdateRequest(&swytch.UpdateRequest{Zone: "tk1v", ID: 123}) }},
		{name: "update missing zone", validate: func() error { return ValidateUpdateRequest(&swytch.UpdateRequest{ID: 123}) }, wantError: "--zone が必要"},
		{name: "update missing id", validate: func() error { return ValidateUpdateRequest(&swytch.UpdateRequest{Zone: "tk1v"}) }, wantError: "--id が必要"},
		{name: "update rejects all zone", validate: func() error { return ValidateUpdateRequest(&swytch.UpdateRequest{Zone: "all", ID: 123}) }, wantError: `Zone に "all"`},
		{name: "delete", validate: func() error { return ValidateDeleteRequest(&swytch.DeleteRequest{Zone: "tk1v", ID: 123}) }},
		{name: "delete missing zone", validate: func() error { return ValidateDeleteRequest(&swytch.DeleteRequest{ID: 123}) }, wantError: "--zone が必要"},
		{name: "delete missing id", validate: func() error { return ValidateDeleteRequest(&swytch.DeleteRequest{Zone: "tk1v"}) }, wantError: "--id が必要"},
		{name: "delete rejects all zone", validate: func() error { return ValidateDeleteRequest(&swytch.DeleteRequest{Zone: "all", ID: 123}) }, wantError: `Zone に "all"`},
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
