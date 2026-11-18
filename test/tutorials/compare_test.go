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

package tutorials

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareOutput(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		actual    string
		wantError bool
	}{
		{
			name:     "空配列の厳密一致",
			expected: "[]",
			actual:   "[]",
		},
		{
			name:      "空配列に対する非空配列",
			expected:  "[]",
			actual:    "[{\"ID\": 1}]",
			wantError: true,
		},
		{
			name:      "配列に対するオブジェクト",
			expected:  "[]",
			actual:    "{}",
			wantError: true,
		},
		{
			name:     "オブジェクトのキーと型の一致 (値は比較しない)",
			expected: "{\n  \"ID\": 21001,\n  \"Name\": \"test-zone-a\"\n}",
			actual:   "{\"ID\": 1, \"Name\": \"is1a\"}",
		},
		{
			name:      "キーの欠落",
			expected:  "{\"ID\": 1, \"Name\": \"x\"}",
			actual:    "{\"ID\": 1}",
			wantError: true,
		},
		{
			name:      "余分なキー",
			expected:  "{\"ID\": 1}",
			actual:    "{\"ID\": 1, \"Extra\": 2}",
			wantError: true,
		},
		{
			name:      "型の不一致 (string vs number)",
			expected:  "{\"ID\": \"123\"}",
			actual:    "{\"ID\": 123}",
			wantError: true,
		},
		{
			name:     "ネストした構造",
			expected: "{\"Zone\": {\"ID\": 1, \"Region\": {\"ID\": 210, \"Name\": \"x\"}}}",
			actual:   "{\"Zone\": {\"ID\": 3, \"Region\": {\"ID\": 401, \"Name\": \"石狩\"}}}",
		},
		{
			name:      "null は null のみ許容",
			expected:  "{\"Region\": null}",
			actual:    "{\"Region\": {\"ID\": 1}}",
			wantError: true,
		},
		{
			name:     "期待出力が JSON でない場合は照合しない",
			expected: "some plain text output",
			actual:   "{\"anything\": 1}",
		},
		{
			name:      "期待が JSON で実際が JSON でない",
			expected:  "[]",
			actual:    "command failed with no output",
			wantError: true,
		},
		{
			name:     "配列要素の構造一致",
			expected: "[{\"ID\": 1, \"Name\": \"a\"}]",
			actual:   "[{\"ID\": 999, \"Name\": \"zzz\"}]",
		},
		{
			name:     "複数行の JSON 例を連結して比較",
			expected: "[\n  {\n    \"ID\": 123456789012,\n    \"Name\": \"QUEUE-NAME\"\n  }\n]",
			actual:   "[\n  {\n    \"ID\": 123,\n    \"Name\": \"eventbus-queue-x1y2\"\n  }\n]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := compareOutput(tt.expected, tt.actual)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
