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

package packetfilter

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name      string
		validator string
		request   any
		wantError string
	}{
		{name: "find", validator: "validatePacketFilterFindRequest", request: &packetfilter.FindRequest{Zone: "is1b"}},
		{name: "find missing zone", validator: "validatePacketFilterFindRequest", request: &packetfilter.FindRequest{}, wantError: "Zone が必要"},
		{name: "find rejects all", validator: "validatePacketFilterFindRequest", request: &packetfilter.FindRequest{Zone: "all"}, wantError: `Zone に "all"`},
		{name: "read", validator: "validatePacketFilterReadRequest", request: &packetfilter.ReadRequest{Zone: "is1b", ID: 123}},
		{name: "read missing ID", validator: "validatePacketFilterReadRequest", request: &packetfilter.ReadRequest{Zone: "is1b"}, wantError: "ID が必要"},
		{name: "create", validator: "validatePacketFilterCreateRequest", request: &packetfilter.CreateRequest{Zone: "is1b", Name: "filter"}},
		{name: "create missing name", validator: "validatePacketFilterCreateRequest", request: &packetfilter.CreateRequest{Zone: "is1b"}, wantError: "Name が必要"},
		{name: "update", validator: "validatePacketFilterUpdateRequest", request: &packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Name: stringPointer("updated")}},
		{name: "update expression", validator: "validatePacketFilterUpdateRequest", request: &packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Expression: &[]*iaas.PacketFilterExpression{nil}, OriginalExpressionHash: "hash"}},
		{name: "update expression needs hash", validator: "validatePacketFilterUpdateRequest", request: &packetfilter.UpdateRequest{Zone: "is1b", ID: 123, Expression: &[]*iaas.PacketFilterExpression{nil}}, wantError: "OriginalExpressionHash が必要"},
		{name: "update no changes", validator: "validatePacketFilterUpdateRequest", request: &packetfilter.UpdateRequest{Zone: "is1b", ID: 123}, wantError: "更新するフィールド"},
		{name: "delete", validator: "validatePacketFilterDeleteRequest", request: &packetfilter.DeleteRequest{Zone: "is1b", ID: 123}},
		{name: "delete rejects all", validator: "validatePacketFilterDeleteRequest", request: &packetfilter.DeleteRequest{Zone: "all", ID: 123}, wantError: `Zone に "all"`},
		{name: "wrong type", validator: "validatePacketFilterFindRequest", request: &packetfilter.ReadRequest{}, wantError: "型が不正"},
		{name: "unknown validator", validator: "unknown", request: &packetfilter.FindRequest{}, wantError: "未知の PacketFilter"},
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

func stringPointer(value string) *string {
	return &value
}
