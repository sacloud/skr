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

package apprundedicated

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

// diffContextLines は unified diff の hunk に含める前後の文脈の行数です。
const diffContextLines = 3

// diffHeader は差分の出力の先頭に置くヘッダーです。
// 左がローカルの定義、右がデプロイ済みの現状です。
const diffHeader = "--- 定義\n+++ 現状\n"

// 差分の色付き出力に使う ANSI エスケープシーケンスです。
const (
	colorRed   = "\x1b[31m"
	colorGreen = "\x1b[32m"
	colorReset = "\x1b[0m"
)

// marshalDefinition は定義をインデント付きの JSON へ整形します。
// init の出力ファイルの形式です。
func marshalDefinition(def *apprundedicatedservice.ApplicationDefinition) ([]byte, error) {
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("定義を JSON へ整形できません: %w", err)
	}
	return append(data, '\n'), nil
}

// definitionText は正規化した定義を diff の入力へ整形します。
func definitionText(m map[string]any) string {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		// 正規化した map は JSON に変換可能な型のみで構成されるため、ここには到達しません。
		return ""
	}
	return string(data) + "\n"
}

// colorEnabled は差分の色付き出力をするかどうかを返します。
// 出力先が標準出力かつ端末のときだけ色を付けます (ADR 0030 の例外の範囲)。
func colorEnabled(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	if !ok || file != os.Stdout {
		return false
	}
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// colorizeDiff は unified diff の追加行を緑に、削除行を赤にします。
func colorizeDiff(diffText string) string {
	lines := strings.Split(strings.TrimSuffix(diffText, "\n"), "\n")
	colored := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+"):
			colored = append(colored, colorGreen+line+colorReset)
		case strings.HasPrefix(line, "-"):
			colored = append(colored, colorRed+line+colorReset)
		default:
			colored = append(colored, line)
		}
	}
	return strings.Join(colored, "\n") + "\n"
}

// diffOpKind は差分行の種類です。
type diffOpKind int

const (
	diffEqual diffOpKind = iota
	diffDelete
	diffInsert
)

// diffLine は差分の 1 行です。
type diffLine struct {
	kind diffOpKind
	text string
}

// unifiedDiff は 2 つのテキストの unified diff を生成します。
// 先頭に diffHeader (定義と現状の見出し) を付けます。差分がない場合は空文字列を
// 返します。context は各 hunk に含める前後の文脈の行数です。
func unifiedDiff(a, b string, context int) string {
	text := formatHunks(lcsDiff(splitLines(a), splitLines(b)), context)
	if text == "" {
		return ""
	}
	return diffHeader + text
}

// splitLines はテキストを行へ分割します。末尾の改行は行に含めません。
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// lcsDiff は 2 つの行の並びの最長共通部分列を求め、保持、削除、追加の列として返します。
func lcsDiff(a, b []string) []diffLine {
	n, m := len(a), len(b)
	// lcs[i][j] は a[i:] と b[j:] の最長共通部分列の長さです。
	// 添字は入力の行数そのものなので、確保サイズの計算があふれることはありません。
	lcs := make([][]int, n)
	for i := range lcs {
		lcs[i] = make([]int, m)
	}
	// at は表の外 (入力の末尾より先) を空の接尾辞として扱います。
	at := func(i, j int) int {
		if i >= n || j >= m {
			return 0
		}
		return lcs[i][j]
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = at(i+1, j+1) + 1
			case at(i+1, j) >= at(i, j+1):
				lcs[i][j] = at(i+1, j)
			default:
				lcs[i][j] = at(i, j+1)
			}
		}
	}
	var ops []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffLine{diffEqual, a[i]})
			i++
			j++
		case at(i+1, j) >= at(i, j+1):
			ops = append(ops, diffLine{diffDelete, a[i]})
			i++
		default:
			ops = append(ops, diffLine{diffInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffLine{diffDelete, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffLine{diffInsert, b[j]})
	}
	return ops
}

// formatHunks は差分行を context 行の文脈でグループ化して unified diff 形式へ整形します。
func formatHunks(ops []diffLine, context int) string {
	changed := false
	for _, op := range ops {
		if op.kind != diffEqual {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}

	var out strings.Builder
	// aLine と bLine は次に消費する行番号 (1 起点) です。
	aLine, bLine := 1, 1
	i := 0
	for i < len(ops) {
		if ops[i].kind == diffEqual {
			aLine++
			bLine++
			i++
			continue
		}
		// hunk の開始を context 行分さかのぼります。
		start, startA, startB := i, aLine, bLine
		for start > 0 && i-start < context && ops[start-1].kind == diffEqual {
			start--
			startA--
			startB--
		}
		// hunk の終端を決めます。変更が続く限り、次の変更までの間隔が context 行以内
		// のときは間の行も含めて 1 つの hunk にします。
		end, endA, endB := i, aLine, bLine
		for end < len(ops) {
			if ops[end].kind != diffEqual {
				if ops[end].kind == diffDelete {
					endA++
				} else {
					endB++
				}
				end++
				continue
			}
			next := end
			for next < len(ops) && ops[next].kind == diffEqual {
				next++
			}
			if next < len(ops) && next-end <= context {
				endA += next - end
				endB += next - end
				end = next
				continue
			}
			break
		}
		// hunk の末尾に context 行分の文脈を足します。
		tail := 0
		for end < len(ops) && tail < context && ops[end].kind == diffEqual {
			endA++
			endB++
			end++
			tail++
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", diffRange(startA, endA-startA), diffRange(startB, endB-startB))
		for k := start; k < end; k++ {
			switch ops[k].kind {
			case diffEqual:
				fmt.Fprintf(&out, " %s\n", ops[k].text)
			case diffDelete:
				fmt.Fprintf(&out, "-%s\n", ops[k].text)
			case diffInsert:
				fmt.Fprintf(&out, "+%s\n", ops[k].text)
			}
		}
		i = end
		aLine, bLine = endA, endB
	}
	return out.String()
}

// diffRange は unified diff の hunk ヘッダーの範囲を整形します。
// 行数が 0 のときは、unified diff の慣例に従って直前の行番号を表示します。
func diffRange(start, count int) string {
	switch {
	case count == 1:
		return fmt.Sprintf("%d", start)
	case count == 0:
		return fmt.Sprintf("%d,0", start-1)
	default:
		return fmt.Sprintf("%d,%d", start, count)
	}
}
