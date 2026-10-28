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

package commandcoverage

import (
	"fmt"
	"slices"
	"strings"
)

// 末端コマンドの E2E 状況を表すステータス。
const (
	// StatusLive はライブ E2E で検証済みであることを表す。
	StatusLive = "live"
	// StatusImplemented はシナリオ実装済みでライブ未実施であることを表す。
	StatusImplemented = "implemented"
	// StatusDeferred は延期であることを表す。追跡 Issue の参照が必須。
	StatusDeferred = "deferred"
	// StatusTodo は E2E が未整備であることを表す。
	StatusTodo = "todo"
)

// Entry は baseline に記録した末端コマンド1件分の状況。
type Entry struct {
	Path   string
	Status string
	Issue  string
	Note   string
}

// ParseBaseline は baseline.txt の内容を解析します。
// 1行1コマンドで、"<path> <status> [#<issue> <説明>] [<補足>]" 形式を受け付けます。
// パスは空白を含むため、行内で最初に現れたステータスのトークンでパスと残りを分離します。
// # で始まる行はコメントとして、空行は読み飛ばします。
func ParseBaseline(data []byte) (map[string]Entry, error) {
	entries := make(map[string]Entry)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		statusIndex := slices.Index(fields, statusToken(fields))
		if statusIndex < 1 {
			return nil, fmt.Errorf("baseline line %d: want \"<path> <status> [#<issue> <説明>] [<補足>]\", got %q", i+1, line)
		}
		path, status := strings.Join(fields[:statusIndex], " "), fields[statusIndex]
		entry := Entry{Path: path, Status: status}
		rest := fields[statusIndex+1:]
		if len(rest) > 0 && strings.HasPrefix(rest[0], "#") {
			entry.Issue = rest[0]
			rest = rest[1:]
		}
		if status == StatusDeferred && entry.Issue == "" {
			return nil, fmt.Errorf("baseline line %d: deferred には追跡 Issue の参照が必要です: %q", i+1, line)
		}
		if _, dup := entries[path]; dup {
			return nil, fmt.Errorf("baseline line %d: duplicate path %q", i+1, path)
		}
		entry.Note = strings.Join(rest, " ")
		entries[path] = entry
	}
	return entries, nil
}

// statusToken は行に含まれるステータスのトークンを返します。ステータスがなければ空文字を返します。
func statusToken(fields []string) string {
	for _, field := range fields {
		switch field {
		case StatusLive, StatusImplemented, StatusDeferred, StatusTodo:
			return field
		}
	}
	return ""
}
