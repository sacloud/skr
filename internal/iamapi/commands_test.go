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

package iamapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeRequestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memberships.json")
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o600); err != nil {
		t.Fatal(err)
	}
	var value []int
	if err := DecodeRequest("@"+path, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 3 || value[0] != 1 || value[2] != 3 {
		t.Fatalf("decoded %#v, want [1 2 3]", value)
	}
	if err := DecodeRequest("@/missing/memberships.json", &value); err == nil {
		t.Error("DecodeRequest(@missing) succeeded, want an error")
	}
	if err := DecodeRequest("{invalid", &value); err == nil {
		t.Error("DecodeRequest(invalid JSON) succeeded, want an error")
	}
}
