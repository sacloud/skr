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

// Package tutorials は docs/manual/tutorials の console ブロックを実行検証する harness です。
// 設計は docs/design/tutorial-script-harness.md を参照してください。
package tutorials

import (
	"fmt"
	"strings"
)

// Step は console ブロック内の 1 コマンドと、直後に続く期待出力を表します。
type Step struct {
	// Line はコマンドの 1 行目のチュートリアル内行番号 (1 起点) です。
	Line int
	// Command は行継続と heredoc を連結した完全なコマンドテキストです。
	Command string
	// Expected は直後に続くインライン期待出力の行です。
	Expected []string
}

// Block は console コードブロック 1 個を表します。
type Block struct {
	// Line はブロック開始フェンスの行番号 (1 起点) です。
	Line int
	// Steps はブロック内のコマンドの順序リストです。
	Steps []*Step
}

// JSONExample は console ブロックの直後に置かれた JSON コードブロックです。
type JSONExample struct {
	// Line はブロック開始フェンスの行番号 (1 起点) です。
	Line int
	// Text はコードブロック内のテキストです。
	Text string
}

// tutorialScript はチュートリアル 1 ファイルから抽出した実行単位です。
type tutorialScript struct {
	// Path は docs/manual/tutorials からの相対パスです。
	Path string
	// Blocks は抽出した console ブロックの順序リストです。
	Blocks []*Block
	// JSONExamples は console ブロックの直後に置かれた JSON コードブロックで、
	// キーは対応する Blocks 内のインデックスです。
	JSONExamples map[int]*JSONExample
}

// fence はコードフェンス行の解析結果です。
type fence struct {
	kind    string // "console"、"json"、"other"、空文字列はフェンス行以外
	closing bool   // 閉じフェンスの場合 true
}

// fenceInfo は行を解析してフェンス行かどうかと種類を返します。
func fenceInfo(line string) fence {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "```") {
		return fence{}
	}
	info := strings.TrimSpace(trimmed[3:])
	if info == "" {
		return fence{closing: true}
	}
	return fence{kind: strings.Fields(info)[0]}
}

// extractScript はチュートリアルの Markdown から console ブロックを抽出します。
func extractScript(path, content string) (*tutorialScript, error) {
	lines := strings.Split(content, "\n")
	script := &tutorialScript{Path: path, JSONExamples: map[int]*JSONExample{}}

	for i := 0; i < len(lines); {
		f := fenceInfo(lines[i])
		if f.kind != "console" {
			i++
			continue
		}
		block, next, err := parseConsoleBlock(lines, i)
		if err != nil {
			return nil, fmt.Errorf("%s: line %d: %w", path, i+1, err)
		}
		idx := len(script.Blocks)
		script.Blocks = append(script.Blocks, block)
		// console ブロックの直後 (空行を挟んでもよい) に続く JSON コードブロックは
		// 最後の step への期待出力です。
		for j := next; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				continue
			}
			if jf := fenceInfo(lines[j]); jf.kind == "json" {
				text, end, err := parseFencedBody(lines, j)
				if err != nil {
					return nil, fmt.Errorf("%s: line %d: %w", path, j+1, err)
				}
				script.JSONExamples[idx] = &JSONExample{Line: j + 1, Text: text}
				next = end
			}
			break
		}
		i = next
	}
	return script, nil
}

// parseFencedBody は開始フェンス行のインデックスから本文と、閉じフェンスの次の行インデックスを返します。
func parseFencedBody(lines []string, open int) (string, int, error) {
	for j := open + 1; j < len(lines); j++ {
		if f := fenceInfo(lines[j]); f.closing {
			return strings.Join(lines[open+1:j], "\n"), j + 1, nil
		}
	}
	return "", 0, fmt.Errorf("unterminated code block")
}

// parseConsoleBlock は console ブロック本文を step に分割します。
// next は閉じフェンスの次の行インデックスです。
func parseConsoleBlock(lines []string, open int) (*Block, int, error) {
	block := &Block{Line: open + 1}
	var current *Step
	var heredocTerm string
	var heredocStripTabs bool

	// newStep は新しい step を開始します ($ 行のたびに呼ばれます)。
	newStep := func(lineNo int, text string) {
		current = &Step{Line: lineNo, Command: text}
		block.Steps = append(block.Steps, current)
	}
	// appendToCurrent は行継続や heredoc 本体を現在の step に連結します。
	appendToCurrent := func(lineNo int, text string) {
		if current == nil {
			// heredoc 本体は $ 行の後にしか現れないため、ここには到達しない。
			newStep(lineNo, text)
			return
		}
		current.Command += "\n" + text
	}

	for j := open + 1; j < len(lines); j++ {
		line := lines[j]
		if f := fenceInfo(line); f.closing {
			return block, j + 1, nil
		}

		if heredocTerm != "" {
			body := line
			if heredocStripTabs {
				body = strings.TrimLeft(line, "\t")
			}
			if body == heredocTerm {
				// 終端行もコマンドの一部として連結する。含めないと bash の
				// source 実行時に heredoc が閉じられず構文エラーになる。
				appendToCurrent(j+1, line)
				heredocTerm = ""
				continue
			}
			appendToCurrent(j+1, line)
			continue
		}

		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// 空行は期待出力に含めない。
		case strings.HasPrefix(trimmed, "$ "):
			command := strings.TrimSpace(trimmed[2:])
			newStep(j+1, command)
			// heredoc の開始を確認します。
			if term, strip, ok := heredocStart(command); ok {
				heredocTerm = term
				heredocStripTabs = strip
			}
		case current != nil && strings.HasSuffix(current.Command, "\\"):
			// 行継続の 2 行目以降。先頭のインデントを除去して連結します。
			appendToCurrent(j+1, strings.TrimLeft(line, " \t"))
		case strings.HasPrefix(trimmed, "#"):
			// コメント行は検証対象に含めない。
		default:
			// コマンド行ではないため、直前の step の期待出力とします。
			if current == nil {
				return nil, 0, fmt.Errorf("expected output before any command: %q", line)
			}
			current.Expected = append(current.Expected, line)
		}
	}
	return nil, 0, fmt.Errorf("unterminated console block")
}

// heredocStart はコマンドテキストから heredoc の開始を検出し、終端トークンと
// <<- 形式によるタブ除去の要否を返します。
func heredocStart(command string) (term string, stripTabs bool, ok bool) {
	idx := strings.Index(command, "<<")
	if idx < 0 {
		return "", false, false
	}
	rest := command[idx+2:]
	if strings.HasPrefix(rest, "-") {
		stripTabs = true
		rest = rest[1:]
	}
	rest = strings.TrimLeft(rest, " \t")
	end := strings.IndexAny(rest, " \t\n")
	token := rest
	if end >= 0 {
		token = rest[:end]
	}
	token = strings.Trim(token, `'"`)
	if token == "" {
		return "", false, false
	}
	return token, stripTabs, true
}
