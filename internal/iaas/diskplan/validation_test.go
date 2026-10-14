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

package diskplan

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskplanSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
)

func TestValidateRequest(t *testing.T) {
	for _, test := range []struct {
		testName  string
		validator string
		request   any
		wantErr   string
	}{
		{testName: "find valid", validator: "validateDiskPlanFindRequest", request: &diskplanSDK.FindRequest{Zone: "is1b"}},
		{testName: "read valid", validator: "validateDiskPlanReadRequest", request: &diskplanSDK.ReadRequest{Zone: "is1b", ID: types.ID(4)}},
		{testName: "unknown validator", validator: "unknown", request: &diskplanSDK.FindRequest{}, wantErr: "未知"},
		{testName: "find wrong request type", validator: "validateDiskPlanFindRequest", request: &diskplanSDK.ReadRequest{}, wantErr: "型が不正"},
		{testName: "read wrong request type", validator: "validateDiskPlanReadRequest", request: &diskplanSDK.FindRequest{}, wantErr: "型が不正"},
		{testName: "find zone required", validator: "validateDiskPlanFindRequest", request: &diskplanSDK.FindRequest{}, wantErr: "Zone が必要"},
		{testName: "find all zone", validator: "validateDiskPlanFindRequest", request: &diskplanSDK.FindRequest{Zone: "all"}, wantErr: "指定できません"},
		{testName: "read zone required", validator: "validateDiskPlanReadRequest", request: &diskplanSDK.ReadRequest{ID: 4}, wantErr: "Zone が必要"},
		{testName: "read all zone", validator: "validateDiskPlanReadRequest", request: &diskplanSDK.ReadRequest{Zone: "all", ID: 4}, wantErr: "指定できません"},
		{testName: "read id required", validator: "validateDiskPlanReadRequest", request: &diskplanSDK.ReadRequest{Zone: "is1b"}, wantErr: "ID が必要"},
	} {
		t.Run(test.testName, func(t *testing.T) {
			err := ValidateRequest(test.validator, test.request)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateRequest() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
