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
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// baselineStatusImplemented は実行検証が実装済みであることを表します。
const baselineStatusImplemented = "implemented"

// baselineEntry は baseline.txt の 1 行です。
type baselineEntry struct {
	// Path は docs/manual/tutorials からの相対パスです。
	Path string
	// Status は implemented または deferred です。
	Status string
	// Blocks は抽出された console ブロック数の下限です。
	Blocks int
	// Reason は deferred の場合の理由です。
	Reason string
}

// loadBaseline は baseline.txt を読み込みます。
func loadBaseline(path string) (map[string]*baselineEntry, error) {
	file, err := os.Open(path) //nolint:gosec // test/tutorials 配下の baseline ファイルを読み込む。
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer file.Close() //nolint:errcheck // 読み取り専用の baseline ファイルのクローズエラーは読み込みエラーに含まれません。

	entries := map[string]*baselineEntry{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entry, err := parseBaselineLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("baseline.txt: line %d: %w", lineNo, err)
		}
		if _, ok := entries[entry.Path]; ok {
			return nil, fmt.Errorf("baseline.txt: line %d: duplicate entry for %s", lineNo, entry.Path)
		}
		entries[entry.Path] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan baseline: %w", err)
	}
	return entries, nil
}

// parseBaselineLine は baseline.txt の 1 行を解析します。
// 書式は <path> <status> <blocks> [<理由>] です。
func parseBaselineLine(line string) (*baselineEntry, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("invalid baseline entry: %q", line)
	}
	blocks, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("invalid block count in %q: %w", line, err)
	}
	if blocks < 0 {
		return nil, fmt.Errorf("invalid block count in %q: must not be negative", line)
	}
	entry := &baselineEntry{
		Path:   fields[0],
		Status: fields[1],
		Blocks: blocks,
		Reason: strings.Join(fields[3:], " "),
	}
	if entry.Status != baselineStatusImplemented && entry.Status != "deferred" {
		return nil, fmt.Errorf("invalid status %q in %q: must be implemented or deferred", entry.Status, line)
	}
	if entry.Status == baselineStatusImplemented && entry.Reason != "" {
		return nil, fmt.Errorf("implemented entry must not have a reason: %q", line)
	}
	if entry.Status == "deferred" && entry.Reason == "" {
		return nil, fmt.Errorf("deferred entry must have a reason: %q", line)
	}
	return entry, nil
}
