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

package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Recorder は E2E 実行の証跡をメモリに保持し、タイムスタンプ付きの非公開ディレクトリへ書き込みます。
// JSON レコードと ORDER.txt はそのままの証跡として残し、REPORT.md はメモリ上のレコードと
// シナリオ・ステップの説明から生成するため、テストの意図と各ステップで確認した内容を読み取れます。
type Recorder struct {
	dir                 string
	sequence            int
	records             []record
	scenarioTitle       string
	scenarioDescription string
}

// Scenario は E2E シナリオのタイトルと目的を人間が読める形で保持します。
// New または CreateAt に渡すと、個々のステップの前にテストの意図が REPORT.md で説明されます。
type Scenario struct {
	Title       string
	Description string
}

type record struct {
	Sequence      int      `json:"sequence"`
	Step          string   `json:"step"`
	Description   string   `json:"description,omitempty"`
	Command       string   `json:"command"`
	Args          []string `json:"args,omitempty"`
	Request       any      `json:"request,omitempty"`
	Stdout        string   `json:"stdout"`
	Stderr        string   `json:"stderr"`
	ExitStatus    *int     `json:"exit_status"`
	Error         string   `json:"error,omitempty"`
	ExpectedError bool     `json:"expected_error,omitempty"`
}

// RequestFileInput は @path 形式で CLI に渡したリクエストファイルのパスと内容を記録します。
type RequestFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// New は <repo-root>/tmp/<service>/ の下に、現在時刻をディレクトリ名として Recorder を作成します。
func New(service string, scenario Scenario) (*Recorder, error) {
	if filepath.Base(service) != service || service == "" {
		return nil, fmt.Errorf("invalid E2E evidence service %q", service)
	}
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("find repository root for E2E evidence: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return nil, errors.New("find repository root for E2E evidence: empty path")
	}
	return CreateAt(filepath.Join(root, "tmp", service), time.Now(), scenario)
}

// CreateAt は baseDir の下に、now をディレクトリ名の接頭辞として Recorder を作成します。
func CreateAt(baseDir string, now time.Time, scenario Scenario) (*Recorder, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create E2E evidence root: %w", err)
	}
	prefix := now.Local().Format("200601021504")
	for suffix := 1; ; suffix++ {
		name := prefix
		if suffix > 1 {
			name = fmt.Sprintf("%s-%02d", prefix, suffix)
		}
		dir := filepath.Join(baseDir, name)
		if err := os.Mkdir(dir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, fmt.Errorf("create E2E evidence directory %s: %w", dir, err)
		}
		order := "Sequence\tStep\tResult\tFile\n"
		if err := os.WriteFile(filepath.Join(dir, "ORDER.txt"), []byte(order), 0o600); err != nil {
			return nil, fmt.Errorf("initialize E2E evidence order in %s: %w", dir, err)
		}
		return &Recorder{dir: dir, scenarioTitle: scenario.Title, scenarioDescription: scenario.Description}, nil
	}
}

// Dir はこの Recorder の非公開 evidence ディレクトリを返します。
func (r *Recorder) Dir() string {
	return r.dir
}

// ValidateStep は step が空でなく、ファイル名として安全な識別子であることを検証します。
func ValidateStep(step string) error {
	if filepath.Base(step) != step || step == "" {
		return fmt.Errorf("invalid evidence step %q", step)
	}
	return nil
}

// Record はコマンド実行の成功・失敗を記録します。互換性のため元のシグネチャを維持しており、
// ステップの意図を呼び出し元で説明として添えるには RecordWithDescription を使います。
func (r *Recorder) Record(step, binary string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool) error {
	return r.record(step, "", binary, args, request, stdout, stderr, runErr, redactStdout, false)
}

// RecordWithDescription は Record に加えて、REPORT.md に表示されるステップ単位の説明を記録します。
func (r *Recorder) RecordWithDescription(step, description, binary string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool) error {
	return r.record(step, description, binary, args, request, stdout, stderr, runErr, redactStdout, false)
}

// RecordExpectedError は、存在しないリソース ID の not-found 確認のように、CLI の失敗が期待どおりの結果であるステップを記録します。
// ORDER.txt とレポートではそのステップを期待どおりの失敗として扱うため、失敗が想定外には見えません。
func (r *Recorder) RecordExpectedError(step, binary string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool) error {
	return r.record(step, "", binary, args, request, stdout, stderr, runErr, redactStdout, true)
}

// RecordExpectedErrorWithDescription は RecordExpectedError に加えて、REPORT.md に表示されるステップ単位の説明を記録します。
func (r *Recorder) RecordExpectedErrorWithDescription(step, description, binary string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool) error {
	return r.record(step, description, binary, args, request, stdout, stderr, runErr, redactStdout, true)
}

func (r *Recorder) record(step, description, binary string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool, expectedError bool) error {
	if err := ValidateStep(step); err != nil {
		return err
	}
	r.sequence++
	sequence := r.sequence
	filename := fmt.Sprintf("%03d-%s.json", sequence, step)
	entry := record{
		Sequence:      sequence,
		Step:          step,
		Description:   description,
		Command:       commandLine(binary, args),
		Args:          args,
		Request:       request,
		Stdout:        stdout,
		Stderr:        stderr,
		ExpectedError: expectedError,
	}
	if redactStdout {
		entry.Stdout = "[REDACTED: APIKey]"
	}
	if runErr != nil {
		entry.Error = runErr.Error()
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			entry.ExitStatus = exitStatus(exitErr.ExitCode())
		}
	} else {
		status := 0
		entry.ExitStatus = &status
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode evidence: %w", step, err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, filename), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("%s: write evidence: %w", step, err)
	}
	result := "ok"
	if runErr != nil {
		result = "failed"
		if expectedError {
			result = "expected-error"
		}
	}
	if err := r.appendOrder(sequence, step, result, filename); err != nil {
		return err
	}
	r.records = append(r.records, entry)
	return nil
}

func exitStatus(status int) *int {
	if status < 0 {
		return nil
	}
	return &status
}

// SetResult は RESULT.txt と REPORT.md を書き込みます。レポートはメモリ上のレコードと
// シナリオ・ステップの説明から生成されます。
func (r *Recorder) SetResult(result string, completed time.Time) error {
	if result != "passed" && result != "failed" {
		return fmt.Errorf("invalid E2E result %q", result)
	}
	content := fmt.Sprintf("Result: %s\nCompleted: %s\n", result, completed.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(r.dir, "RESULT.txt"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write E2E result in %s: %w", r.dir, err)
	}
	report := r.renderReport(result, completed)
	if err := os.WriteFile(filepath.Join(r.dir, "REPORT.md"), []byte(report), 0o600); err != nil {
		return fmt.Errorf("write E2E report in %s: %w", r.dir, err)
	}
	return nil
}

func (r *Recorder) renderReport(result string, completed time.Time) string {
	var report strings.Builder
	report.WriteString("# E2E 実行レポート\n\n")

	if r.scenarioTitle != "" || r.scenarioDescription != "" {
		report.WriteString("## シナリオ\n\n")
		if r.scenarioTitle != "" {
			fmt.Fprintf(&report, "**タイトル**: %s\n\n", html.EscapeString(r.scenarioTitle))
		}
		if r.scenarioDescription != "" {
			report.WriteString("**目的**: ")
			report.WriteString(html.EscapeString(r.scenarioDescription))
			report.WriteString("\n\n")
		}
	}

	fmt.Fprintf(&report, "- 結果: **%s**\n", result)
	fmt.Fprintf(&report, "- 完了日時: `%s`\n", completed.Format(time.RFC3339))
	fmt.Fprintf(&report, "- 実行コマンド数: %d\n\n", len(r.records))

	report.WriteString("## ステップ一覧\n\n")
	report.WriteString("| # | ステップ | 確認内容 | 結果 | Exit status |\n|---:|---|---|---|---:|\n")
	for _, entry := range r.records {
		stepResult, status := "成功", "取得できません"
		if entry.Error != "" {
			stepResult = "失敗"
			if entry.ExpectedError {
				stepResult = "期待どおりの失敗"
			}
		}
		if entry.ExitStatus != nil {
			status = strconv.Itoa(*entry.ExitStatus)
		}
		desc := entry.Description
		if desc == "" {
			desc = "(説明未登録)"
		}
		fmt.Fprintf(&report, "| %03d | `%s` | %s | %s | %s |\n", entry.Sequence, entry.Step, desc, stepResult, status)
	}

	report.WriteString("\n## 詳細\n")
	for _, entry := range r.records {
		stepResult, status := "成功", "取得できません"
		if entry.Error != "" {
			stepResult = "失敗"
			if entry.ExpectedError {
				stepResult = "期待どおりの失敗"
			}
		}
		if entry.ExitStatus != nil {
			status = strconv.Itoa(*entry.ExitStatus)
		}
		report.WriteString("\n")
		fmt.Fprintf(&report, "### %03d `%s` — %s (exit status %s)\n\n", entry.Sequence, html.EscapeString(entry.Step), stepResult, status)
		if entry.Description != "" {
			fmt.Fprintf(&report, "**確認内容**: %s\n\n", html.EscapeString(entry.Description))
		}
		report.WriteString("**実行コマンド**\n\n")
		writeCodeBlock(&report, entry.Command)
		if requestFile, ok := entry.Request.(RequestFileInput); ok {
			fmt.Fprintf(&report, "**request file** (`%s`)\n\n", html.EscapeString(requestFile.Path))
			writeCodeBlock(&report, requestFile.Content)
		}
		report.WriteString("**stdout**\n\n")
		writeCodeBlock(&report, entry.Stdout)
		report.WriteString("**stderr**\n\n")
		writeCodeBlock(&report, entry.Stderr)
		if entry.Error != "" {
			report.WriteString("**実行エラー**\n\n")
			writeCodeBlock(&report, entry.Error)
		}
	}
	return report.String()
}

func writeCodeBlock(builder *strings.Builder, content string) {
	maxRun := 0
	for _, line := range strings.Split(content, "\n") {
		run := 0
		for _, char := range line {
			if char == '`' {
				run++
				if run > maxRun {
					maxRun = run
				}
			} else {
				run = 0
			}
		}
	}
	fence := strings.Repeat("`", max(3, maxRun+1))
	fmt.Fprintf(builder, "%stext\n%s", fence, content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteByte('\n')
	}
	fmt.Fprintf(builder, "%s\n\n", fence)
}

func commandLine(binary string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(binary))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("_@%+=:,./-", char))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (r *Recorder) appendOrder(sequence int, step, result, filename string) error {
	path := filepath.Join(r.dir, "ORDER.txt")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // ファイル名は非公開の evidence ディレクトリ内で固定されます。
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(file, "%03d\t%s\t%s\t%s\n", sequence, step, result, filename); err != nil {
		return errors.Join(fmt.Errorf("append %s: %w", path, err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
