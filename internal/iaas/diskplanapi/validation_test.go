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

package diskplanapi

import (
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskplanSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
)

func TestTypedRequestValidators(t *testing.T) {
	for _, test := range []struct {
		testName string
		validate func() error
		wantErr  string
	}{
		{testName: "read valid", validate: func() error { return ValidateReadRequest(&diskplanSDK.ReadRequest{Zone: "is1b", ID: types.ID(4)}) }},
		{testName: "read id required", validate: func() error { return ValidateReadRequest(&diskplanSDK.ReadRequest{Zone: "is1b"}) }, wantErr: "ID が必要"},
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
