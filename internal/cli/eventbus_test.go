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

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventbusmock "github.com/sacloud/sakumock/eventbus"
	"github.com/sacloud/skr/internal/eventbus"
	"github.com/stretchr/testify/require"
)

func TestRunEventbusCommandHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"eventbus", "--help"},
			want: []string{
				"process-configuration", "schedule", "trigger", "set-secret",
				"--secret-file", "eventbus-api",
			},
		},
		{
			args: []string{"eventbus", "process-configuration", "create", "--help"},
			want: []string{
				"--name", "--description", "--destination",
				"--group-id", "--message", "--queue-name", "--content", "--action", "--resource-id",
				"simplenotification", "simplemq", "autoscale",
			},
		},
		{
			args: []string{"eventbus", "process-configuration", "update", "--help"},
			want: []string{"実行設定 ID", "--destination", "引き継がれ", "現状維持"},
		},
		{
			args: []string{"eventbus", "process-configuration", "set-secret", "--help"},
			want: []string{"実行設定 ID", "--secret-file", "@path", "-", "インライン", "トレース"},
		},
		{
			args: []string{"eventbus", "schedule", "create", "--help"},
			want: []string{
				"--name", "--process-configuration-id", "--starts-at",
				"--recurring-step", "--recurring-unit", "--crontab", "min", "hour", "day", "RFC3339", "どちらか",
			},
		},
		{
			args: []string{"eventbus", "trigger", "create", "--help"},
			want: []string{"--source", "--type", "--condition", "--process-configuration-id", "反復可能", "key=名前,op=eq|in,value=値"},
		},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
			}
		})
	}
}

func TestEventbusCommandsWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := eventbusmock.NewTestServer(eventbusmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_EVENTBUS", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}

	// 実行設定を作成します。
	var process eventbus.ProcessConfigurationOutput
	err := json.Unmarshal(runCommand("eventbus", "process-configuration", "create",
		"--name", "mock-process", "--description", "test process",
		"--destination", "simplemq",
		"--queue-name", "mock-queue", "--content", "hello",
	), &process)
	require.NoError(t, err)
	require.Equal(t, "mock-process", process.Name, "create returned %#v, want name", process)
	require.Equal(t, "simplemq", process.Destination, "unexpected value")
	require.Equal(t, map[string]string{"queue_name": "mock-queue", "content": "hello"}, process.Parameters, "unexpected value")
	require.Equal(t, []string{}, process.Tags, "Tags の null は空配列で出力します")
	require.NotNil(t, process.Description, "unexpected value")
	require.Equal(t, "test process", *process.Description, "unexpected value")
	processID := process.ID
	require.NotEmpty(t, processID, "create should return ID")

	// 一覧と絞り込み出力を確認します。
	var listed []eventbus.ProcessConfigurationOutput
	err2 := json.Unmarshal(runCommand("eventbus", "process-configuration", "list"), &listed)
	require.NoError(t, err2)
	require.Len(t, listed, 1, "list returned %#v, want created process", listed)
	require.Equal(t, processID, listed[0].ID, "unexpected value")

	var projected []struct {
		ID   string
		Name string
	}
	err3 := json.Unmarshal(runCommand("eventbus", "process-configuration", "list", "--query", "map({ID,Name})"), &projected)
	require.NoError(t, err3)
	require.Len(t, projected, 1, "projected output = %#v, want created process", projected)
	require.Equal(t, processID, projected[0].ID, "unexpected value")
	require.Equal(t, "mock-process", projected[0].Name, "unexpected value")

	var read eventbus.ProcessConfigurationOutput
	err4 := json.Unmarshal(runCommand("eventbus", "process-configuration", "read", processID), &read)
	require.NoError(t, err4)
	require.Equal(t, "mock-process", read.Name, "unexpected value")

	// 名前だけ更新しても宛先設定を保持します。
	var renamed eventbus.ProcessConfigurationOutput
	err5 := json.Unmarshal(runCommand("eventbus", "process-configuration", "update", processID,
		"--name", "renamed-process",
	), &renamed)
	require.NoError(t, err5)
	require.Equal(t, "renamed-process", renamed.Name, "unexpected value")
	require.Equal(t, map[string]string{"queue_name": "mock-queue", "content": "hello"}, renamed.Parameters, "unexpected value")

	// 宛先を切り替えると旧宛先のパラメーターは引き継ぎません。
	var autoscaled eventbus.ProcessConfigurationOutput
	err6 := json.Unmarshal(runCommand("eventbus", "process-configuration", "update", processID,
		"--destination", "autoscale", "--action", "scale_up", "--resource-id", "res-1",
	), &autoscaled)
	require.NoError(t, err6)
	require.Equal(t, "autoscale", autoscaled.Destination, "unexpected value")
	require.Equal(t, map[string]string{"action": "scale_up", "resource_id": "res-1"}, autoscaled.Parameters, "unexpected value")

	// simplemq に戻して secret を設定します。
	runCommand("eventbus", "process-configuration", "update", processID,
		"--destination", "simplemq", "--queue-name", "mock-queue", "--content", "hello")

	secretPath := filepath.Join(t.TempDir(), "secret-input.json")
	err7 := os.WriteFile(secretPath, []byte(`{"APIKey":"SECRET-FILE-VALUE"}`), 0o600)
	require.NoError(t, err7)
	runCommand("eventbus", "process-configuration", "set-secret", processID, "--secret-file", "@"+secretPath)
	stored, ok := server.Secret(processID)
	require.True(t, ok, "secret should be stored on the mock server")
	var storedSecret struct {
		APIKey string
	}
	require.NoError(t, json.Unmarshal(stored, &storedSecret))
	require.Equal(t, "SECRET-FILE-VALUE", storedSecret.APIKey, "stored secret = %s", stored)

	// 標準入力からも設定できます。
	reader, writer, err8 := os.Pipe()
	require.NoError(t, err8)
	originalStdin := os.Stdin
	os.Stdin = reader
	_, err9 := writer.WriteString(`{"APIKey":"SECRET-STDIN-VALUE"}` + "\n")
	require.NoError(t, err9)
	require.NoError(t, writer.Close())
	runCommand("eventbus", "process-configuration", "set-secret", processID, "--secret-file", "-")
	os.Stdin = originalStdin
	require.NoError(t, reader.Close())
	stored, ok = server.Secret(processID)
	require.True(t, ok, "secret should be stored on the mock server")
	storedSecret = struct {
		APIKey string
	}{}
	require.NoError(t, json.Unmarshal(stored, &storedSecret))
	require.Equal(t, "SECRET-STDIN-VALUE", storedSecret.APIKey, "stored secret = %s", stored)

	// スケジュールを作成します。
	var schedule eventbus.ScheduleOutput
	err10 := json.Unmarshal(runCommand("eventbus", "schedule", "create",
		"--name", "mock-schedule", "--description", "test schedule",
		"--process-configuration-id", processID,
		"--starts-at", "2030-01-10T09:00:00+09:00",
		"--recurring-step", "5", "--recurring-unit", "hour",
	), &schedule)
	require.NoError(t, err10)
	require.Equal(t, "mock-schedule", schedule.Name, "create returned %#v, want name", schedule)
	require.Equal(t, processID, schedule.ProcessConfigurationID, "unexpected value")
	require.Equal(t, "2030-01-10T00:00:00Z", schedule.StartsAt, "starts-at は UTC の RFC3339 で出力します")
	require.NotNil(t, schedule.RecurringStep, "unexpected value")
	require.Equal(t, 5, *schedule.RecurringStep, "unexpected value")
	require.NotNil(t, schedule.RecurringUnit, "unexpected value")
	require.Equal(t, "hour", *schedule.RecurringUnit, "unexpected value")
	require.Nil(t, schedule.Crontab, "crontab 未指定のときは null で出力します")
	scheduleID := schedule.ID
	require.NotEmpty(t, scheduleID, "create should return ID")

	// crontab へ切り替えると繰り返し設定を解除します。
	var crontabSchedule eventbus.ScheduleOutput
	err11 := json.Unmarshal(runCommand("eventbus", "schedule", "update", scheduleID,
		"--name", "mock-schedule", "--crontab", "0 9 * * *",
	), &crontabSchedule)
	require.NoError(t, err11)
	require.NotNil(t, crontabSchedule.Crontab, "unexpected value")
	require.Equal(t, "0 9 * * *", *crontabSchedule.Crontab, "unexpected value")
	require.Nil(t, crontabSchedule.RecurringStep, "crontab への切り替えで RecurringStep を解除します")
	require.Nil(t, crontabSchedule.RecurringUnit, "crontab への切り替えで RecurringUnit を解除します")

	// トリガーを作成します。
	var trigger eventbus.TriggerOutput
	err12 := json.Unmarshal(runCommand("eventbus", "trigger", "create",
		"--name", "mock-trigger", "--description", "test trigger",
		"--source", "//eventbus.sakura.ad.jp/eventlog",
		"--type", "zone.updated",
		"--type", "billing,updated",
		"--condition", "key=zone,op=eq,value=is1b",
		"--condition", "key=host,op=in,value=www1,value=www2",
		"--process-configuration-id", processID,
	), &trigger)
	require.NoError(t, err12)
	require.Equal(t, "mock-trigger", trigger.Name, "create returned %#v, want name", trigger)
	require.Equal(t, processID, trigger.ProcessConfigurationID, "unexpected value")
	require.Equal(t, []string{"zone.updated", "billing,updated"}, trigger.Types, "カンマを含む値は分割しません")
	require.Equal(t, []eventbus.TriggerConditionOutput{
		{Key: "zone", Op: "eq", Values: []string{"is1b"}},
		{Key: "host", Op: "in", Values: []string{"www1", "www2"}},
	}, trigger.Conditions, "unexpected value")
	triggerID := trigger.ID
	require.NotEmpty(t, triggerID, "create should return ID")

	// Types だけを差し替えます。
	var typedTrigger eventbus.TriggerOutput
	err13 := json.Unmarshal(runCommand("eventbus", "trigger", "update", triggerID,
		"--name", "mock-trigger", "--type", "disk.inserted",
	), &typedTrigger)
	require.NoError(t, err13)
	require.Equal(t, []string{"disk.inserted"}, typedTrigger.Types, "unexpected value")

	// 削除します。
	runCommand("eventbus", "schedule", "delete", scheduleID)
	runCommand("eventbus", "trigger", "delete", triggerID)
	runCommand("eventbus", "process-configuration", "delete", processID)

	// process-configuration の一覧が空になります。
	var empty []eventbus.ProcessConfigurationOutput
	err14 := json.Unmarshal(runCommand("eventbus", "process-configuration", "list"), &empty)
	require.NoError(t, err14)
	require.Len(t, empty, 0, "list after delete returned %#v, want empty", empty)
	var projectedEmpty []struct {
		ID   string
		Name string
	}
	err15 := json.Unmarshal(runCommand("eventbus", "process-configuration", "list", "--query", "map({ID,Name})"), &projectedEmpty)
	require.NoError(t, err15)
	require.Len(t, projectedEmpty, 0, "projected output = %#v, want empty", projectedEmpty)
}

func TestEventbusCommandInputErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := eventbusmock.NewTestServer(eventbusmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_EVENTBUS", server.TestURL())

	secretPath := filepath.Join(t.TempDir(), "secret-input.json")
	err := os.WriteFile(secretPath, []byte(`{"APIKey":"SECRET-REFUSED-VALUE"}`), 0o600)
	require.NoError(t, err)

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid condition syntax",
			args: []string{"eventbus", "trigger", "create", "--name", "t",
				"--source", "//eventbus.sakura.ad.jp/eventlog",
				"--condition", "key=zone,op=eq",
				"--process-configuration-id", "123"},
			want: "--condition は key・op・value が必要です",
		},
		{
			name: "invalid condition op",
			args: []string{"eventbus", "trigger", "create", "--name", "t",
				"--source", "//eventbus.sakura.ad.jp/eventlog",
				"--condition", "key=zone,op=gt,value=1",
				"--process-configuration-id", "123"},
			want: "eq または in",
		},
		{
			name: "invalid starts-at",
			args: []string{"eventbus", "schedule", "create", "--name", "s",
				"--process-configuration-id", "123",
				"--starts-at", "2030/01/10",
				"--recurring-step", "5", "--recurring-unit", "hour"},
			want: "RFC3339",
		},
		{
			name: "recurring and crontab are exclusive",
			args: []string{"eventbus", "schedule", "create", "--name", "s",
				"--process-configuration-id", "123",
				"--starts-at", "2030-01-10T00:00:00Z",
				"--recurring-step", "5", "--recurring-unit", "hour",
				"--crontab", "0 9 * * *"},
			want: "どちらか一方",
		},
		{
			name: "cross destination fields",
			args: []string{"eventbus", "process-configuration", "create", "--name", "p",
				"--destination", "simplemq", "--group-id", "g"},
			want: "Destination と group_id の組み合わせが不正です",
		},
		{
			name: "missing secret file",
			args: []string{"eventbus", "process-configuration", "set-secret", "123",
				"--secret-file", "@/missing/secret-input.json"},
			want: "開けません",
		},
		{
			name: "trace is refused for secret operations",
			args: []string{"--trace", "eventbus", "process-configuration", "set-secret", "123",
				"--secret-file", "@" + secretPath},
			want: "トレースを無効にしてください",
		},
		{
			name: "invalid query is rejected before the API call",
			args: []string{"eventbus", "process-configuration", "list", "--query", ".["},
			want: "query",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run(test.args, &stdout, &stderr)
			require.NotEqual(t, 0, exitCode, "run(%v) = %d stdout %q stderr %q; want an explicit error",
				test.args, exitCode, stdout.String(), stderr.String())
			require.EqualValues(t, 0, stdout.Len(), "unexpected stdout: %q", stdout.String())
			require.Contains(t, stderr.String(), test.want, "stderr: %s", stderr.String())
			// secret の値は出力しません。
			require.NotContains(t, stderr.String(), "SECRET-REFUSED-VALUE", "stderr: %s", stderr.String())
			require.NotContains(t, stdout.String(), "SECRET-REFUSED-VALUE")
		})
	}
}
