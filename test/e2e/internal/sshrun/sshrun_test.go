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

package sshrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

// evidenceRecord は証跡 JSON レコードの検証に使う項目です。
type evidenceRecord struct {
	Step        string   `json:"step"`
	Description string   `json:"description"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Stdout      string   `json:"stdout"`
	Error       string   `json:"error"`
	ExitStatus  *int     `json:"exit_status"`
}

func newRecorder(t *testing.T) *evidence.Recorder {
	t.Helper()
	recorder, err := evidence.CreateAt(filepath.Join(t.TempDir(), "tmp", "sshrun-test"), time.Now(), evidence.Scenario{})
	if err != nil {
		t.Fatal(err)
	}
	return recorder
}

func evidenceFilenames(t *testing.T, recorder *evidence.Recorder) []string {
	t.Helper()
	entries, err := os.ReadDir(recorder.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func readEvidence(t *testing.T, recorder *evidence.Recorder, filename string) evidenceRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(recorder.Dir(), filename)) //nolint:gosec // このテストの一時ディレクトリ内の固定された evidence ファイル名。
	if err != nil {
		t.Fatal(err)
	}
	var entry evidenceRecord
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestProbeLoginSucceedsWithoutRecording(t *testing.T) {
	recorder := newRecorder(t)
	runner := Runner{
		Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
		SSH: func(context.Context, []string) (string, string, error) {
			return "test-server\n", "", nil
		},
	}
	done, obs := runner.ProbeLogin(context.Background(), "192.0.2.10")
	if !done {
		t.Fatal("ProbeLogin reported failure for a successful SSH login")
	}
	if obs.Err != nil || obs.Binary != "ssh" || strings.TrimSpace(obs.Stdout) != "test-server" {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	// 待機前の単発確認は証跡に記録しない。
	if names := evidenceFilenames(t, recorder); len(names) != 0 {
		t.Fatalf("ProbeLogin recorded evidence: %v", names)
	}
}

func TestProbeLoginTreatsEmptyHostnameAndFailureAsLoginFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		stdout  string
		runErr  error
		wantErr string
	}{
		{"connection refused", "", errors.New("connection refused"), "connection refused"},
		{"empty hostname", "  \n", nil, "empty hostname"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := Runner{
				PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
				SSH: func(context.Context, []string) (string, string, error) {
					return test.stdout, "", test.runErr
				},
			}
			done, obs := runner.ProbeLogin(context.Background(), "192.0.2.10")
			if done {
				t.Fatal("ProbeLogin reported success for a failed SSH login")
			}
			if obs.Err == nil || !strings.Contains(obs.Err.Error(), test.wantErr) {
				t.Fatalf("observation error = %v, want %q", obs.Err, test.wantErr)
			}
		})
	}
}

func TestProbeLoginUsesFixedArgumentsAndDefaultUser(t *testing.T) {
	var got []string
	runner := Runner{
		PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
		SSH: func(_ context.Context, args []string) (string, string, error) {
			got = args
			return "", "", errors.New("refused")
		},
	}
	runner.ProbeLogin(context.Background(), "192.0.2.10")
	want := []string{
		"-i", "/tmp/ssh-key",
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=/tmp/known-hosts",
		"skr-e2e@192.0.2.10",
		"hostname",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ssh args = %v, want %v", got, want)
	}
	runner.User = "ubuntu"
	runner.ProbeLogin(context.Background(), "192.0.2.10")
	if got[len(got)-2] != "ubuntu@192.0.2.10" {
		t.Fatalf("ssh login user = %q, want ubuntu@192.0.2.10", got[len(got)-2])
	}
}

func TestWaitRecordsSingleSummaryOnSuccess(t *testing.T) {
	recorder := newRecorder(t)
	runner := Runner{
		Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
		SSH: func(context.Context, []string) (string, string, error) {
			return "test-server\n", "", nil
		},
	}
	attempts := 0
	err := runner.Wait(context.Background(), "test-ssh-login", "Server 123 の起動後の SSH ログイン確認",
		Options{Timeout: 5 * time.Second, PollInterval: time.Millisecond},
		func(ctx context.Context) (bool, Observation, error) {
			attempts++
			if attempts < 3 {
				// 最初の2回はサーバー起動前として、SSH 確認の前に再試行する。
				return false, Observation{}, nil
			}
			done, obs := runner.ProbeLogin(ctx, "192.0.2.10")
			return done, obs, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	// 複数回の確認があっても、待機の記録は 1 件だけ。
	names := evidenceFilenames(t, recorder)
	if len(names) != 1 || names[0] != "001-test-ssh-login.json" {
		t.Fatalf("evidence files = %v, want a single test-ssh-login record", names)
	}
	entry := readEvidence(t, recorder, names[0])
	if entry.ExitStatus == nil || *entry.ExitStatus != 0 || entry.Error != "" {
		t.Fatalf("successful wait was recorded as a failure: %+v", entry)
	}
	if !strings.Contains(entry.Description, "3 回の確認で成功") || !strings.Contains(entry.Description, "Server 123 の起動後の SSH ログイン確認") {
		t.Fatalf("summary description = %q, want waited duration and attempt count", entry.Description)
	}
	if !strings.HasPrefix(entry.Command, "ssh ") || !strings.Contains(strings.Join(entry.Args, " "), "skr-e2e@192.0.2.10") {
		t.Fatalf("successful login was not recorded: command=%q args=%v", entry.Command, entry.Args)
	}
	if !strings.Contains(entry.Stdout, "test-server") {
		t.Fatalf("hostname output was not recorded: %q", entry.Stdout)
	}
}

func TestWaitRecordsFailureOnTimeout(t *testing.T) {
	recorder := newRecorder(t)
	runner := Runner{Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts"}
	err := runner.Wait(context.Background(), "test-ssh-login", "Server 123 の起動後の SSH ログイン確認",
		Options{Timeout: 20 * time.Millisecond, PollInterval: time.Millisecond},
		func(context.Context) (bool, Observation, error) {
			// サーバーが起動しないため、状態確認の結果だけを返す。
			return false, Observation{Binary: "./skr", Args: []string{"iaas-api", "server", "read"}, Stdout: "down"}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "SSH login check timed out after 20ms") {
		t.Fatalf("wait error = %v, want timeout", err)
	}
	names := evidenceFilenames(t, recorder)
	if len(names) != 1 || names[0] != "001-test-ssh-login.json" {
		t.Fatalf("evidence files = %v, want a single failure record", names)
	}
	entry := readEvidence(t, recorder, names[0])
	if entry.ExitStatus != nil || !strings.Contains(entry.Error, "timed out") {
		t.Fatalf("timeout was not recorded as a failure: %+v", entry)
	}
	if !strings.Contains(entry.Description, "タイムアウト") {
		t.Fatalf("summary description = %q, want timeout summary", entry.Description)
	}
	// タイムアウト時も、最後の確認内容が失敗の記録に残る。
	if !strings.HasPrefix(entry.Command, "./skr ") || !strings.Contains(entry.Stdout, "down") {
		t.Fatalf("last observation was not recorded: command=%q stdout=%q", entry.Command, entry.Stdout)
	}
}

func TestWaitRecordsFailureAndReturnsProbeError(t *testing.T) {
	probeErr := errors.New("decode Server: invalid character")
	recorder := newRecorder(t)
	runner := Runner{Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts"}
	err := runner.Wait(context.Background(), "test-ssh-login", "Server 123 の起動後の SSH ログイン確認",
		Options{Timeout: time.Second, PollInterval: time.Millisecond},
		func(context.Context) (bool, Observation, error) {
			return false, Observation{Binary: "./skr"}, probeErr
		})
	if !errors.Is(err, probeErr) {
		t.Fatalf("wait error = %v, want %v", err, probeErr)
	}
	names := evidenceFilenames(t, recorder)
	if len(names) != 1 {
		t.Fatalf("evidence files = %v, want a single failure record", names)
	}
	entry := readEvidence(t, recorder, names[0])
	if !strings.Contains(entry.Error, probeErr.Error()) || !strings.Contains(entry.Description, "エラー") {
		t.Fatalf("probe error was not recorded: %+v", entry)
	}
}

func TestWaitReturnsErrorWhenRecordingFails(t *testing.T) {
	recorder := newRecorder(t)
	// 待機ステップの記録先をディレクトリにして、書き込みを必ず失敗させる。
	if err := os.Mkdir(filepath.Join(recorder.Dir(), "001-test-ssh-login.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{
		Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
		SSH: func(context.Context, []string) (string, string, error) {
			return "test-server\n", "", nil
		},
	}
	err := runner.Wait(context.Background(), "test-ssh-login", "Server 123 の起動後の SSH ログイン確認",
		Options{Timeout: time.Second, PollInterval: time.Millisecond},
		func(context.Context) (bool, Observation, error) {
			return true, Observation{}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "write evidence") {
		t.Fatalf("wait error = %v, want evidence write failure", err)
	}
}

func TestWaitUsesDefaultsWhenOptionsAreEmpty(t *testing.T) {
	recorder := newRecorder(t)
	runner := Runner{Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts"}
	attempts := 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := runner.Wait(ctx, "test-ssh-login", "Server 123 の起動後の SSH ログイン確認", Options{},
		func(context.Context) (bool, Observation, error) {
			attempts++
			return false, Observation{}, nil
		})
	// 既定のタイムアウト（20分）と再試行間隔（10秒）では、20ms のキャンセルまでに確認は 1 回だけ行われる。
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("wait error = %v, want parent context cancellation", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want a single attempt before cancellation", attempts)
	}
}

func TestRunRecordsCommandResult(t *testing.T) {
	recorder := newRecorder(t)
	runner := Runner{
		Recorder: recorder, PrivateKey: "/tmp/ssh-key", KnownHosts: "/tmp/known-hosts",
		SSH: func(_ context.Context, args []string) (string, string, error) {
			if !strings.Contains(args[len(args)-1], "poweroff") {
				t.Fatalf("remote command = %q, want poweroff", args[len(args)-1])
			}
			return "", "", nil
		},
	}
	if err := runner.Run(context.Background(), "server-shutdown", "192.0.2.10", "sudo -n systemctl poweroff --no-block"); err != nil {
		t.Fatal(err)
	}
	names := evidenceFilenames(t, recorder)
	if len(names) != 1 || names[0] != "001-server-shutdown.json" {
		t.Fatalf("evidence files = %v, want a single server-shutdown record", names)
	}
	entry := readEvidence(t, recorder, names[0])
	if entry.ExitStatus == nil || *entry.ExitStatus != 0 {
		t.Fatalf("successful command was recorded as a failure: %+v", entry)
	}

	runner.SSH = func(context.Context, []string) (string, string, error) {
		return "", "", errors.New("connection closed")
	}
	err := runner.Run(context.Background(), "server-shutdown", "192.0.2.10", "sudo -n systemctl poweroff --no-block")
	if err == nil || !strings.Contains(err.Error(), "SSH command failed") {
		t.Fatalf("run error = %v, want SSH command failure", err)
	}
	entry = readEvidence(t, recorder, "002-server-shutdown.json")
	if !strings.Contains(entry.Error, "connection closed") {
		t.Fatalf("command failure was not recorded: %+v", entry)
	}
}
