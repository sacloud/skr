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

// Package e2e は E2E runner 間で共有可能な CLI 実行と証跡記録の機構を提供します。
package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

// Runner は skr バイナリを実行し、evidence.Recorder に記録する E2E 用 CLI ランナーです。
type Runner struct {
	binary   string
	recorder *evidence.Recorder
	prefix   []string
	env      []string
	timeout  time.Duration
}

// RunnerOption は Runner のオプションを設定する関数です。
type RunnerOption func(*Runner)

// WithPrefix は、各コマンドの先頭に挿入するサブコマンド接頭辞を指定します。
// たとえば container-registry-api の場合は []string{"container-registry-api"} を指定します。
func WithPrefix(prefix ...string) RunnerOption {
	return func(r *Runner) { r.prefix = prefix }
}

// WithEnv はコマンド実行時に追加する環境変数を指定します。
func WithEnv(env ...string) RunnerOption {
	return func(r *Runner) { r.env = env }
}

// WithTimeout はコマンド実行のタイムアウトを指定します。既定値は 60 秒です。
func WithTimeout(timeout time.Duration) RunnerOption {
	return func(r *Runner) { r.timeout = timeout }
}

// NewRunner は Runner を作成します。
func NewRunner(binary string, recorder *evidence.Recorder, opts ...RunnerOption) *Runner {
	r := &Runner{
		binary:   binary,
		recorder: recorder,
		timeout:  60 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Caller は E2E シナリオから CLI 呼び出しを行うための抽象インターフェースです。
// テストで Runner をモックするために使用します。
type Caller interface {
	Call(ctx context.Context, step, description string, args ...string) ([]byte, error)
}

// Call は接頭辞なしで skr を実行します。config current など、トップレベルコマンド向けです。
func (r *Runner) Call(ctx context.Context, step, description string, args ...string) ([]byte, error) {
	return r.run(ctx, step, description, args...)
}

// CallSubcommand は接頭辞を付与してから skr を実行します。
// たとえば接頭辞が container-registry-api の場合、CallSubcommand(ctx, step, desc, "registry", "list")
// は skr container-registry-api registry list を実行します。
func (r *Runner) CallSubcommand(ctx context.Context, step, description string, args ...string) ([]byte, error) {
	return r.run(ctx, step, description, append(r.prefix, args...)...)
}

func (r *Runner) run(ctx context.Context, step, description string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	requestFile, err := requestFileForEvidence(args)
	if err != nil {
		return nil, fmt.Errorf("%s: read request file for evidence: %w", step, err)
	}
	timeout, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // 選択されたローカルの skr 実行ファイルと構造化された引数を、シェルなしで直接実行する。
	command.Env = append(os.Environ(), r.env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	if err := r.recorder.RecordWithDescription(step, description, r.binary, args, requestFile, stdout.String(), stderr.String(), runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

func requestFileForEvidence(args []string) (any, error) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--request" || !strings.HasPrefix(args[i+1], "@") {
			continue
		}
		path := strings.TrimPrefix(args[i+1], "@")
		content, err := os.ReadFile(path) // #nosec G304 -- リクエストパスは E2E runner がプライベートな一時ディレクトリ内に作成する。
		if err != nil {
			return nil, err
		}
		return evidence.RequestFileInput{Path: path, Content: string(content)}, nil
	}
	return nil, nil
}
