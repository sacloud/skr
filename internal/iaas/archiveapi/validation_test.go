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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
)

func TestParseArchiveOSType(t *testing.T) {
	for _, name := range ostype.OSTypeShortNames {
		if got := parseArchiveOSType(name); got == ostype.Custom {
			t.Errorf("parseArchiveOSType(%q) = Custom", name)
		}
	}
	for _, input := range []string{"", "unknown", "custom"} {
		if got := parseArchiveOSType(input); got >= ostype.Custom {
			t.Errorf("parseArchiveOSType(%q) = %d, want invalid sentinel", input, got)
		}
	}
}
