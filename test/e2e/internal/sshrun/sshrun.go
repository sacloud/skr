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

// Package sshrun は E2E runner 向けに、一時サーバーへの SSH 到達確認と SSH コマンドの実行を共通化します。
// 待機中の個々の確認コマンドは証跡に記録せず、待機の結果を 1 件だけ記録します。
// これにより REPORT.md には待機時間と確認回数が残り、途中の確認成否が成功と失敗のどちらだったかで
// 読みにくくなることを防ぎます。
package sshrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	// DefaultTimeout は SSH 到達確認の既定のタイムアウトです。
	DefaultTimeout = 20 * time.Minute
	// DefaultPollInterval は到達確認の既定の再試行間隔です。
	DefaultPollInterval = 10 * time.Second
	// DefaultUser は一時サーバーへ SSH ログインする既定のユーザー名です。
	DefaultUser = "skr-e2e"
	// DefaultCommandTimeout は 1 回の SSH コマンド実行のタイムアウトです。
	DefaultCommandTimeout = 15 * time.Second
)

// SSHFunc は ssh コマンドの実装です。単体テストで差し替えます。
type SSHFunc func(ctx context.Context, args []string) (stdout, stderr string, err error)

// Observation は待機中に実行した最後の確認コマンドの内容です。
// 待機の要約を証跡へ記録するとき、最後の確認内容として一緒に記録します。
type Observation struct {
	Binary  string
	Args    []string
	Request any
	Stdout  string
	Stderr  string
	Err     error
}

// Probe は待機条件を 1 回評価します。
// done が true のとき待機を完了します。err が nil でないときは想定外のエラーとして待機を中断し、
// 最後の Observation とともに失敗として証跡へ記録します。
type Probe func(ctx context.Context) (done bool, obs Observation, err error)

// Options は待機のタイムアウトと再試行間隔です。ゼロ値の項目は既定値を使います。
type Options struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

// Runner は SSH コマンドの実行と、到達確認の証跡記録を担います。
type Runner struct {
	// Recorder は待機の要約と SSH コマンドの結果を記録する証跡です。
	Recorder *evidence.Recorder
	// User は SSH ログインのユーザー名です。空の場合は DefaultUser を使います。
	User string
	// PrivateKey は秘密鍵ファイルのパスです。
	PrivateKey string
	// KnownHosts は既知ホストファイルのパスです。
	KnownHosts string
	// SSH は ssh コマンドの実装です。nil の場合は ssh コマンドを実行します。単体テストで差し替えます。
	SSH SSHFunc
}

// Wait は probe を PollInterval ごとに再試行し、待機の結果を 1 件だけ証跡に記録します。
// 待機中の個々の確認コマンドは記録せず、待機時間、確認回数、最後の確認内容を step の説明として記録します。
// 期限以内に待機できなかった場合と想定外のエラーが発生した場合も、最後の確認内容を失敗として
// 記録してからエラーを返します。記録に失敗した場合は待機の結果とともにエラーを返します。
func (r Runner) Wait(ctx context.Context, step, description string, opts Options, probe Probe) error {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	interval := opts.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	started := time.Now()
	attempts := 0
	var last Observation
	var waitErr error
	var summary string
loop:
	for {
		done, obs, err := probe(ctx)
		attempts++
		if obs.Binary != "" {
			last = obs
		}
		switch {
		case err != nil:
			waitErr = err
			summary = fmt.Sprintf("%s（%d 回の確認後にエラー）", description, attempts)
			break loop
		case done:
			summary = fmt.Sprintf("%s（%s 待機、%d 回の確認で成功）", description, waited(started), attempts)
			break loop
		}
		select {
		case <-ctx.Done():
			waitErr = fmt.Errorf("SSH login check was cancelled: %w", ctx.Err())
			summary = fmt.Sprintf("%s（%s 待機後に中断、%d 回確認）", description, waited(started), attempts)
			break loop
		case <-deadline.C:
			if last.Err != nil {
				waitErr = fmt.Errorf("SSH login check timed out after %s (%d checks, last error: %v)", timeout, attempts, last.Err)
			} else {
				waitErr = fmt.Errorf("SSH login check timed out after %s (%d checks)", timeout, attempts)
			}
			summary = fmt.Sprintf("%s（%s でタイムアウト、%d 回確認）", description, timeout, attempts)
			break loop
		case <-ticker.C:
		}
	}
	recErr := r.recordWait(step, summary, last, waitErr)
	return errors.Join(waitErr, recErr)
}

// ProbeLogin は address へ 1 回 SSH ログインを試みます。証跡には記録しません。
// リモートで `hostname` を実行し、出力が空の場合はログイン失敗として扱います。
func (r Runner) ProbeLogin(ctx context.Context, address string) (bool, Observation) {
	args := r.loginArgs(address, "hostname")
	timeout, cancel := context.WithTimeout(ctx, DefaultCommandTimeout)
	defer cancel()
	stdout, stderr, err := r.command(timeout, args)
	if err == nil && strings.TrimSpace(stdout) == "" {
		err = errors.New("SSH login returned an empty hostname")
	}
	return err == nil, Observation{Binary: "ssh", Args: args, Stdout: stdout, Stderr: stderr, Err: err}
}

// Run は address に対して 1 回の SSH コマンドを実行し、結果を証跡に記録します。
// 待機を伴わない poweroff などの操作向けです。
func (r Runner) Run(ctx context.Context, step, address, remoteCommand string) error {
	args := r.loginArgs(address, remoteCommand)
	timeout, cancel := context.WithTimeout(ctx, DefaultCommandTimeout)
	defer cancel()
	stdout, stderr, runErr := r.command(timeout, args)
	recErr := r.recordCommand(step, args, stdout, stderr, runErr)
	if runErr != nil {
		return errors.Join(fmt.Errorf("%s: SSH command failed (see evidence): %w", step, runErr), recErr)
	}
	return recErr
}

func (r Runner) recordWait(step, description string, last Observation, waitErr error) error {
	if r.Recorder == nil {
		return errors.New("SSH readiness check evidence recorder is not configured")
	}
	return r.Recorder.RecordWithDescription(step, description, last.Binary, last.Args, last.Request, last.Stdout, last.Stderr, waitErr, false)
}

func (r Runner) recordCommand(step string, args []string, stdout, stderr string, runErr error) error {
	if r.Recorder == nil {
		return errors.New("SSH command evidence recorder is not configured")
	}
	return r.Recorder.Record(step, "ssh", args, nil, stdout, stderr, runErr, false)
}

func (r Runner) loginArgs(address, remoteCommand string) []string {
	return []string{
		"-i", r.PrivateKey,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + r.KnownHosts,
		r.user() + "@" + address,
		remoteCommand,
	}
}

func (r Runner) user() string {
	if r.User != "" {
		return r.User
	}
	return DefaultUser
}

func (r Runner) command(ctx context.Context, args []string) (string, string, error) {
	if r.SSH != nil {
		return r.SSH(ctx, args)
	}
	return runSSH(ctx, args)
}

func runSSH(ctx context.Context, args []string) (string, string, error) {
	//nolint:gosec // 引数は runner が組み立てた固定のリモートコマンドで、シェルを介さずに渡します。
	command := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func waited(started time.Time) string {
	return time.Since(started).Round(time.Second).String()
}
