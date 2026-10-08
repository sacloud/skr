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

package zones

import (
	"strings"
	"testing"
)

func TestValidateSingleZone(t *testing.T) {
	for _, test := range []struct {
		zone      string
		wantError bool
	}{
		{zone: "tk1v"},
		{zone: "is1b"},
		{zone: "all", wantError: true},
	} {
		t.Run(test.zone, func(t *testing.T) {
			err := ValidateSingleZone(test.zone)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), `Zone に "all"`) {
					t.Fatalf("ValidateSingleZone(%q) error = %v, want retired all-zones error", test.zone, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateSingleZone(%q) error = %v", test.zone, err)
			}
		})
	}
}
