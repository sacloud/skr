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

package e2e

import (
	"strings"
	"testing"
)

// isHex は文字列が 16 進数のみで構成されているかを判定します。
func isHex(value string) bool {
	return !strings.ContainsFunc(value, func(char rune) bool {
		return !strings.ContainsRune("0123456789abcdef", char)
	})
}

func TestRandomSuffix(t *testing.T) {
	first, err := RandomSuffix()
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomSuffix()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 16 {
		t.Fatalf("suffix length = %d, want 16", len(first))
	}
	if !isHex(first) {
		t.Fatalf("suffix %q is not hexadecimal", first)
	}
	if first == second {
		t.Fatal("two generated suffixes are identical")
	}
}
