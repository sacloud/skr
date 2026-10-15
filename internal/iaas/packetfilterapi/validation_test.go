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

package packetfilterapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
)

func TestTypedRequestValidators(t *testing.T) {
	tests := []struct {
		name      string
		validate  func() error
		wantError string
	}{
		{name: "find", validate: func() error { return ValidateFindRequest(&packetfilter.FindRequest{Zone: "is1b"}) }},
		{name: "find missing zone", validate: func() error { return ValidateFindRequest(&packetfilter.FindRequest{}) }, wantError: "Zone が必要"},
		{name: "find rejects all", validate: func() error { return ValidateFindRequest(&packetfilter.FindRequest{Zone: "all"}) }, wantError: `Zone に "all"`},
		{name: "read", validate: func() error { return ValidateReadRequest(&packetfilter.ReadRequest{Zone: "is1b", ID: 123}) }},
		{name: "read missing ID", validate: func() error { return ValidateReadRequest(&packetfilter.ReadRequest{Zone: "is1b"}) }, wantError: "ID が必要"},
		{name: "create", validate: func() error { return ValidateCreateRequest(&packetfilter.CreateRequest{Zone: "is1b", Name: "filter"}) }},
		{name: "create missing name", validate: func() error { return ValidateCreateRequest(&packetfilter.CreateRequest{Zone: "is1b"}) }, wantError: "Name が必要"},
		{name: "update", validate: func() error {
			return ValidateUpdateRequest(&packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Name: stringPointer("updated")})
		}},
		{name: "update expression", validate: func() error {
			return ValidateUpdateRequest(&packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Expression: &[]*iaas.PacketFilterExpression{nil}, OriginalExpressionHash: "hash"})
		}},
		{name: "update expression needs hash", validate: func() error {
			return ValidateUpdateRequest(&packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Expression: &[]*iaas.PacketFilterExpression{nil}})
		}, wantError: "OriginalExpressionHash が必要"},
		{name: "update no changes", validate: func() error { return ValidateUpdateRequest(&packetfilter.UpdateRequest{Zone: "is1b", ID: 123}) }, wantError: "更新するフィールド"},
		{name: "delete", validate: func() error { return ValidateDeleteRequest(&packetfilter.DeleteRequest{Zone: "is1b", ID: 123}) }},
		{name: "delete rejects all", validate: func() error { return ValidateDeleteRequest(&packetfilter.DeleteRequest{Zone: "all", ID: 123}) }, wantError: `Zone に "all"`},
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

func stringPointer(value string) *string {
	return &value
}
