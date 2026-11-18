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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// stepTimeout は 1 step の実行タイムアウトです。
	stepTimeout = 60 * time.Second
	// profileName は harness が作成する skr プロファイルの名前です。
	profileName = "tutorials"
	// zonesEnv は harness 用 IaaS モックにゾーンを伝える環境変数です。
	zonesEnv = "SKR_TUTORIAL_HARNESS_ZONES"
)

// runner はチュートリアル 1 ファイルの実行を担います。
type runner struct {
	// binDir はビルド済み skr バイナリを PATH の先頭に加えるディレクトリです。
	binDir string
	// fixturesDir は fixture ディレクトリです。
	fixturesDir string
	// workDir はチュートリアル実行に使う一時ディレクトリで、初期 cwd でもあります。
	workDir string
	// fixture はこのチュートリアルの fixture です。
	fixture *Fixture

	// stateDir は step 間の状態の受け渡しに使うディレクトリです。
	stateDir string
	// cwd は直前 step 終了時のカレントディレクトリです。
	cwd string
	// umask は直前 step 終了時の umask です。
	umask string
	// env は直前 step 終了時の環境変数の写像です。nil の場合は
	// まだ 1 step も実行しておらず、テストプロセスの環境変数を基準にします。
	env map[string]string
}

// newRunner は runner を初期化し、ラッパースクリプトを書き込みます。
func newRunner(binDir, fixturesDir, workDir string, fixture *Fixture) (*runner, error) {
	r := &runner{
		binDir:      binDir,
		fixturesDir: fixturesDir,
		workDir:     workDir,
		fixture:     fixture,
		stateDir:    filepath.Join(workDir, ".harness-state"),
		cwd:         workDir,
		umask:       "022",
		// env は nil のままにし、最初の step はテストプロセスの環境変数を基準にする。
		env: nil,
	}
	if err := os.MkdirAll(r.stateDir, 0o750); err != nil {
		return nil, err
	}
	if len(fixture.Mock.Zones) == 0 {
		return nil, fmt.Errorf("fixture must seed at least one zone for the IaaS mock")
	}
	// チュートリアルは順次実行されるため、テストプロセスの環境変数を上書きしてよい。
	if err := os.Setenv(zonesEnv, strings.Join(fixture.Mock.Zones, ",")); err != nil {
		return nil, err
	}
	if err := r.writeWrapper(); err != nil {
		return nil, err
	}
	return r, nil
}

// setup は実行環境を作成します。テスト用の RSA 秘密鍵と skr プロファイルを作成し、
// current プロファイルに設定します。
func (r *runner) setup() error {
	// プロファイル作成に必要なダミーの秘密鍵を書き出します。
	// 実環境の認証情報は使わない。
	keyPath := filepath.Join(r.workDir, ".harness-key.pem")
	if err := writeHarnessPrivateKey(keyPath); err != nil {
		return err
	}
	setupCommand := fmt.Sprintf(
		`skr config create %s --service-principal-id dummy --service-principal-key-id dummy --private-key-file %s --use`,
		profileName, keyPath,
	)
	return r.execStep(&Step{Command: setupCommand}, "setup: skr config create")
}

// run はチュートリアルの各 step を実行します。エラーはチュートリアル内の
// 行番号付きで返します。stub と skip はチュートリアルに書かれたコマンドテキスト
// (正規化後) をキーに判定し、値の置換は実行するコマンドと stub の置換後テキストに
// 適用します。
func (r *runner) run(script *tutorialScript) error {
	for blockIdx, block := range script.Blocks {
		for _, step := range block.Steps {
			if replacement, stubbed := r.fixture.stubReplacement(step.Command); stubbed {
				// stub 化したコマンドは実行せず、値の置換後に構文検証だけを行います。
				if err := validateStubCommand(r.fixture.substituteValues(replacement)); err != nil {
					return fmt.Errorf("%s: line %d: stub: %w", script.Path, step.Line, err)
				}
				continue
			}

			if r.fixture.shouldSkip(step.Command) {
				continue
			}

			command := r.fixture.substituteValues(step.Command)
			description := fmt.Sprintf("%s: line %d: %s", script.Path, step.Line, strings.Join(strings.Fields(step.Command), " "))
			if err := r.execStep(&Step{Line: step.Line, Command: command}, description); err != nil {
				return err
			}
			if err := r.compareInlineExpected(script.Path, step); err != nil {
				return err
			}
		}
		// ブロックの直後に置かれた JSON 例は、ブロック内で最後に実行した
		// step の出力と照合します。
		if example, ok := script.JSONExamples[blockIdx]; ok {
			actual, err := os.ReadFile(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
			if err != nil {
				return fmt.Errorf("%s: line %d: read last output: %w", script.Path, example.Line, err)
			}
			if err := compareOutput(example.Text, string(actual)); err != nil {
				return fmt.Errorf("%s: line %d: %w", script.Path, example.Line, err)
			}
		}
	}
	// チュートリアルの編集で対応が外れた fixture の項目を検出します。
	return r.fixture.checkUnused()
}

// compareInlineExpected は step 直後にチュートリアルに書かれたインライン期待出力を
// 実行結果と照合します。期待出力が JSON として解釈できない場合 (プレーンテキストの
// 説明行など) は照合しません。
func (r *runner) compareInlineExpected(path string, step *Step) error {
	if len(step.Expected) == 0 {
		return nil
	}
	actual, err := os.ReadFile(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("%s: line %d: read last output: %w", path, step.Line, err)
	}
	if err := compareOutput(strings.Join(step.Expected, "\n"), string(actual)); err != nil {
		return fmt.Errorf("%s: line %d: %w", path, step.Line, err)
	}
	return nil
}

// execStep は 1 step を bash で実行し、状態を回収します。
func (r *runner) execStep(step *Step, description string) error {
	stepPath := filepath.Join(r.stateDir, "step.sh")
	if err := os.WriteFile(stepPath, []byte(step.Command+"\n"), 0o600); err != nil { //nolint:gosec // runner が作成した state ディレクトリ配下に step スクリプトを書き込む。
		return err
	}

	// 出力を受け取るファイルを用意します。os.Create は既存ファイルを切り詰めます。
	outputFile, err := os.Create(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下に出力ファイルを作成する。
	if err != nil {
		return err
	}
	defer func() { _ = outputFile.Close() }() // step の出力はファイルに書き終えているためクローズエラーは無視する。

	stderrFile, err := os.Create(filepath.Join(r.stateDir, "last-stderr")) //nolint:gosec // runner が作成した state ディレクトリ配下に stderr ファイルを作成する。
	if err != nil {
		return err
	}
	defer func() { _ = stderrFile.Close() }() // step の stderr はファイルに書き終えているためクローズエラーは無視する。

	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(r.stateDir, "wrapper.sh")) //nolint:gosec // runner が書き込んだ固定のラッパースクリプトを、シェル経由で実行する。
	cmd.Stdout = outputFile
	cmd.Stderr = stderrFile
	cmd.Env = r.processEnv()
	if err := cmd.Run(); err != nil {
		stderr, _ := os.ReadFile(filepath.Join(r.stateDir, "last-stderr")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s: timeout after %s", description, stepTimeout)
		}
		return fmt.Errorf("%s: %w\nstderr:\n%s", description, err, stderr)
	}

	return r.harvestState()
}

// processEnv は bash プロセスに渡す環境変数を組み立てます。
// 最初の step はテストプロセスの環境変数を基にし、2 step 目以降は直前 step
// 終了時の完全な環境 (unset の反映を含む) を基にします。
func (r *runner) processEnv() []string {
	merged := map[string]string{}
	if r.env == nil {
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			merged[key] = value
		}
	} else {
		for key, value := range r.env {
			merged[key] = value
		}
	}
	// PATH はビルド済みバイナリを優先します。2 step 目以降の PATH には
	// すでに binDir が含まれるため、重複して先頭に加えない。
	prefix := r.binDir + string(os.PathListSeparator)
	if !strings.HasPrefix(merged["PATH"], prefix) {
		merged["PATH"] = prefix + merged["PATH"]
	}
	// harness 固有の変数は明示的に組み立て直すため、引き継ぎ分を除去します。
	for _, key := range []string{"SKR_HARNESS_CWD", "SKR_HARNESS_UMASK", "SKR_HARNESS_STATE", "SKR_HARNESS_STEP"} {
		delete(merged, key)
	}
	env := make([]string, 0, len(merged)+4)
	for key, value := range merged {
		env = append(env, key+"="+value)
	}
	return append(env,
		"SKR_HARNESS_CWD="+r.cwd,
		"SKR_HARNESS_UMASK="+r.umask,
		"SKR_HARNESS_STATE="+r.stateDir,
		"SKR_HARNESS_STEP="+filepath.Join(r.stateDir, "step.sh"),
	)
}

// harvestState は step 終了後の状態ファイルを読み込みます。
func (r *runner) harvestState() error {
	cwd, err := os.ReadFile(filepath.Join(r.stateDir, "cwd")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read cwd state: %w", err)
	}
	r.cwd = strings.TrimSpace(string(cwd))

	umask, err := os.ReadFile(filepath.Join(r.stateDir, "umask")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read umask state: %w", err)
	}
	r.umask = strings.TrimSpace(string(umask))

	raw, err := os.ReadFile(filepath.Join(r.stateDir, "env")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read env state: %w", err)
	}
	env := map[string]string{}
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte("="), 2)
		key := string(parts[0])
		// SHLVL と _ はシェルが管理するため引き継がない。
		if key == "SHLVL" || key == "_" {
			continue
		}
		env[key] = string(parts[1])
	}
	r.env = env
	return nil
}

// writeWrapper は step 実行に使うラッパースクリプトを書き込みます。
// set -a により step 内の変数代入も環境変数として env ダンプに現れ、
// 次の step に引き継がれます。
func (r *runner) writeWrapper() error {
	wrapper := `set -o errexit -o nounset -o pipefail
set -a
cd "$SKR_HARNESS_CWD"
umask "$SKR_HARNESS_UMASK"
source "$SKR_HARNESS_STEP"
printf '%s' "$(pwd)" > "$SKR_HARNESS_STATE/cwd"
umask > "$SKR_HARNESS_STATE/umask"
env -0 > "$SKR_HARNESS_STATE/env"
`
	return os.WriteFile(filepath.Join(r.stateDir, "wrapper.sh"), []byte(wrapper), 0o600)
}

// validateStubCommand は stub 化したコマンドの構文を検証します。
// skr コマンドは kong のパースで検証し、--query / --query-raw の式は
// gojq でコンパイルできることを確認します。
func validateStubCommand(command string) error {
	fields := shellFields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty command")
	}
	if fields[0] == "skr" {
		if err := skrParseOnly(fields[1:]); err != nil {
			return err
		}
	}
	for i := 0; i < len(fields); i++ {
		if (fields[i] == "--query" || fields[i] == "--query-raw") && i+1 < len(fields) {
			if err := compileGojq(fields[i+1]); err != nil {
				return fmt.Errorf("%s: %w", fields[i], err)
			}
		}
	}
	return nil
}

// shellFields はコマンドテキストをシェル風にトークン化します。
// シングルクォートとダブルクォートをトークンの区切りとして扱い、
// クォート文字自体はトークンに含めません。エスケープは解釈しません。
func shellFields(command string) []string {
	var fields []string
	var current strings.Builder
	var inSingle, inDouble bool
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, r := range command {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return fields
}
