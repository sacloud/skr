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

// このファイルは IaaS 製品プラン E2E の共有ロジックの単体テストです。
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// intPtr はテストで *int を作るためのヘルパーです。
func intPtr(v int) *int {
	return &v
}

// TestSliceLength は結果配列の要素数の計算を確認します。
func TestSliceLength(t *testing.T) {
	require.Equal(t, -1, sliceLength[int](nil), "nil slice should return -1")
	require.Equal(t, 0, sliceLength(&[]int{}), "empty slice should return 0")
	require.Equal(t, 2, sliceLength(&[]int{1, 2}), "slice length should match items")
}

// TestValidateFindResult は Find 応答のページング情報の検証を確認します。
func TestValidateFindResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		total     *int
		from      *int
		count     *int
		itemCount int
		want      string
	}{
		{name: "nil pagination", total: nil, from: nil, count: nil, itemCount: -1, want: "required pagination fields"},
		{name: "missing total", total: nil, from: intPtr(0), count: intPtr(0), itemCount: 0, want: "required pagination fields"},
		{name: "missing from", total: intPtr(0), from: nil, count: intPtr(0), itemCount: 0, want: "required pagination fields"},
		{name: "missing count", total: intPtr(0), from: intPtr(0), count: nil, itemCount: 0, want: "required pagination fields"},
		{name: "null result array", total: intPtr(0), from: intPtr(0), count: intPtr(0), itemCount: -1, want: "required pagination fields"},
		{name: "negative total", total: intPtr(-1), from: intPtr(0), count: intPtr(0), itemCount: 0, want: "pagination metadata"},
		{name: "mismatched from", total: intPtr(1), from: intPtr(5), count: intPtr(1), itemCount: 1, want: "pagination metadata"},
		{name: "negative count", total: intPtr(1), from: intPtr(0), count: intPtr(-1), itemCount: -1, want: "required pagination fields"},
		{name: "count above requested", total: intPtr(2), from: intPtr(0), count: intPtr(2), itemCount: 2, want: "pagination metadata"},
		{name: "count above remaining", total: intPtr(2), from: intPtr(1), count: intPtr(2), itemCount: 2, want: "pagination metadata"},
		{name: "count above total", total: intPtr(1), from: intPtr(0), count: intPtr(2), itemCount: 2, want: "pagination metadata"},
		{name: "from above total", total: intPtr(1), from: intPtr(2), count: intPtr(0), itemCount: 0, want: "pagination metadata"},
		{name: "count differs from items", total: intPtr(1), from: intPtr(0), count: intPtr(1), itemCount: 0, want: "pagination metadata"},
		{name: "zero count with item", total: intPtr(1), from: intPtr(0), count: intPtr(0), itemCount: 1, want: "pagination metadata"},
		{name: "empty page with remaining", total: intPtr(1), from: intPtr(0), count: intPtr(0), itemCount: 0, want: "pagination metadata"},
		{name: "valid single page", total: intPtr(1), from: intPtr(0), count: intPtr(1), itemCount: 1},
		{name: "valid empty result", total: intPtr(0), from: intPtr(0), count: intPtr(0), itemCount: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateFindResult(test.total, test.from, test.count, test.itemCount, 0, 1)
			if test.want == "" {
				require.NoError(t, err, "validateFindResult error: %v", err)
				return
			}
			require.ErrorContains(t, err, test.want)
		})
	}
}
