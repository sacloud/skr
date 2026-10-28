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

// Package commandcoverage は末端サブコマンドの E2E 網羅状況を管理するための
// 解析処理を提供します。実行記録から実行されたコマンドを抽出し、
// baseline.txt に記録した末端コマンドごとの状況と突き合わせます。
package commandcoverage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// LeafPathOf は skr コマンドラインを末端コマンドのパスへ正規化します。
// 位置引数やフラグを含む行でも、末端コマンドのパスがトークン境界で前方一致する
// もののうち最長のパスを返します。対応する末端コマンドがない場合は false を返します。
func LeafPathOf(command string, leaves []string) (string, bool) {
	tokens := strings.Fields(command)
	if len(tokens) == 0 || tokens[0] != "skr" {
		return "", false
	}
	best := ""
	for _, leaf := range leaves {
		leafTokens := strings.Fields(leaf)
		// コマンドラインは末端パスに位置引数やフラグが続くため、
		// 末端パスのトークン数はコマンドラインのトークン数以下であること。
		if len(leafTokens) > len(tokens)-1 {
			continue
		}
		if !slices.Equal(tokens[1:1+len(leafTokens)], leafTokens) {
			continue
		}
		if len(leaf) > len(best) {
			best = leaf
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// Evidence は実行記録から抽出した末端コマンドの集合。
// Unknown は末端コマンドに対応しなかった記録行で、古い CLI 構成の実行記録などが該当します。
type Evidence struct {
	Commands map[string]struct{}
	Unknown  []string
}

// EvidenceCommands は実行記録から実行された末端コマンドを抽出します。
// rootDir は test/e2e を指し、<rootDir>/<識別名>/e2e-output/*.md の Markdown を収集します。
// 記録形式は共有 helper が生成するもので、**command**: に続くコードブロックに
// skr CLI の呼び出しが1行ずつ記録されます。旧 runner 形式の記録は対象外です。
func EvidenceCommands(rootDir string, leaves []string) (Evidence, error) {
	files, err := filepath.Glob(filepath.Join(rootDir, "*", "e2e-output", "*.md"))
	if err != nil {
		return Evidence{}, fmt.Errorf("collect evidence files: %w", err)
	}
	commandLines := make([]string, 0, 64)
	for _, file := range files {
		data, err := os.ReadFile(file) //nolint:gosec // パスは test/e2e 配下の実行記録を Glob で収集したものです。
		if err != nil {
			return Evidence{}, fmt.Errorf("read evidence file %s: %w", file, err)
		}
		commandLines = append(commandLines, evidenceCommandLines(data)...)
	}
	evidence := Evidence{Commands: make(map[string]struct{}, len(commandLines))}
	for _, line := range commandLines {
		path, ok := LeafPathOf(line, leaves)
		if !ok {
			evidence.Unknown = append(evidence.Unknown, line)
			continue
		}
		evidence.Commands[path] = struct{}{}
	}
	return evidence, nil
}

// evidenceCommandLines は実行記録の **command**: コードブロックから skr で始まる行を返します。
func evidenceCommandLines(data []byte) []string {
	var lines []string
	scanner := bytes.NewBuffer(data)
	inCommandBlock := false
	inFence := false
	for {
		line, err := scanner.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		reachedEOF := err != nil
		switch {
		case inCommandBlock && strings.TrimSpace(line) == "```":
			inFence = !inFence
			if !inFence {
				// コードブロックの終端で **command**: の解析を終える。
				inCommandBlock = false
			}
		case inCommandBlock && inFence && strings.HasPrefix(strings.TrimSpace(line), "skr "):
			lines = append(lines, strings.TrimSpace(line))
		case strings.TrimSpace(line) == "**command**:":
			inCommandBlock = true
			inFence = false
		}
		if reachedEOF {
			return lines
		}
	}
}
