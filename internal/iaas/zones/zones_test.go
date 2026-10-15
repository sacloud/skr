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
	"testing"
)

func TestValidateSingleZone(t *testing.T) {
	for _, test := range []struct {
		zone      string
		wantError string
	}{
		{zone: "", wantError: "Zone が必要です (--request JSON も指定できます)"},
		{zone: "tk1v"},
		{zone: "is1b"},
		{zone: "all", wantError: `Zone に "all" は指定できません。検索対象のゾーン名を指定してください`},
	} {
		t.Run(test.zone, func(t *testing.T) {
			err := ValidateSingleZone(test.zone)
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("ValidateSingleZone(%q) error = %v, want %q", test.zone, err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateSingleZone(%q) error = %v", test.zone, err)
			}
		})
	}
}
