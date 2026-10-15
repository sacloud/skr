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

package archiveapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	archiveSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
)

func TestTypedRequestValidators(t *testing.T) {
	for _, test := range []struct {
		testName string
		validate func() error
		wantErr  string
	}{
		{testName: "find valid", validate: func() error { return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b"}) }},
		{testName: "find os type", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b", OSType: ostype.Ubuntu2404})
		}},
		{testName: "read valid", validate: func() error {
			return ValidateReadRequest(&archiveSDK.ReadRequest{Zone: "is1b", ID: types.ID(123)})
		}},
		{testName: "find zone required", validate: func() error { return ValidateFindRequest(&archiveSDK.FindRequest{}) }, wantErr: "Zone が必要"},
		{testName: "find all zone", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "all"})
		}, wantErr: "指定できません"},
		{testName: "read zone required", validate: func() error { return ValidateReadRequest(&archiveSDK.ReadRequest{ID: 123}) }, wantErr: "Zone が必要"},
		{testName: "read all zone", validate: func() error {
			return ValidateReadRequest(&archiveSDK.ReadRequest{Zone: "all", ID: 123})
		}, wantErr: "指定できません"},
		{testName: "read id required", validate: func() error { return ValidateReadRequest(&archiveSDK.ReadRequest{Zone: "is1b"}) }, wantErr: "ID が必要"},
		{testName: "invalid os type", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b", OSType: -1})
		}, wantErr: "OSType が不正"},
		{testName: "unknown os type number", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b", OSType: 1000})
		}, wantErr: "OSType が不正"},
		{testName: "os type and names", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b", OSType: ostype.Ubuntu2404, Names: []string{"image"}})
		}, wantErr: "併用できません"},
		{testName: "os type and tags", validate: func() error {
			return ValidateFindRequest(&archiveSDK.FindRequest{Zone: "is1b", OSType: ostype.Ubuntu2404, Tags: []string{"stable"}})
		}, wantErr: "併用できません"},
	} {
		t.Run(test.testName, func(t *testing.T) {
			err := test.validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validator() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
