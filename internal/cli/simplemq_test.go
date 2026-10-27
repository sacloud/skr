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

	"github.com/sacloud/sacloud-sdk-go/api/simplemq"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/message"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/queue"
	simplemqmock "github.com/sacloud/sakumock/simplemq"
	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestSimpleMQGeneratedCodeMatchesConfig(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{config: "api/commands/simplemq-queue.json", output: "internal/simplemqapi/simplemq_queue_api_generated.go"},
		{config: "api/commands/simplemq-message.json", output: "internal/simplemqapi/simplemq_message_api_generated.go"},
		{config: "api/commands/eventbus-process-c0nfiguration.json", output: "internal/eventbusapi/eventbus_process_c0nfiguration_api_generated.go"},
		{config: "api/commands/eventbus-schedule.json", output: "internal/eventbusapi/eventbus_schedule_api_generated.go"},
		{config: "api/commands/eventbus-trigger.json", output: "internal/eventbusapi/eventbus_trigger_api_generated.go"},
		{config: "api/commands/iam-user.json", output: "internal/iamapi/iam_user_api_generated.go"},
		{config: "api/commands/iam-group.json", output: "internal/iamapi/iam_group_api_generated.go"},
		{config: "api/commands/iam-policy.json", output: "internal/iamapi/iam_policy_api_generated.go"},
	} {
		configData, err := os.ReadFile(repositoryPath(test.config))
		require.NoError(t, err)
		config, err := apigen.DecodeConfig(configData)
		require.NoError(t, err)
		want, err := apigen.Generate(config)
		require.NoError(t, err)
		got, err := os.ReadFile(repositoryPath(test.output))
		require.NoError(t, err)
		require.Equal(t, want, got, "generated API commands are stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
	}
}

func TestRunSimpleMQAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"simplemq-api", "--help"},
			want: []string{"queue", "message", "SAKURA_ACCESS_TOKEN", "--api-key-file", "JSON"},
		},
		{
			args: []string{"simplemq-api", "queue", "--help"},
			want: []string{"list", "read", "create", "config", "delete", "count-messages", "rotate-api-key", "clear-messages"},
		},
		{
			args: []string{"simplemq-api", "queue", "create", "--help"},
			want: []string{"--name", "--description", "--request", "5〜64", "@path.json", "併用不可"},
		},
		{
			args: []string{"simplemq-api", "queue", "config", "--help"},
			want: []string{"--visibility-timeout-seconds", "--expire-seconds", "--request", "併用不可", "API の", "CommonServiceItem.Settings", "5〜900", "60〜1209600", "345600"},
		},
		{
			args: []string{"simplemq-api", "queue", "rotate-api-key", "--help"},
			want: []string{"キュー ID"},
		},
		{
			args: []string{"simplemq-api", "message", "send", "--help"},
			want: []string{"--queue-name", "--api-key-file", "--content", "256000"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
			}
		})
	}
}

func TestSimpleMQAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := simplemqmock.NewTestServer(simplemqmock.Config{Strict: true})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_SIMPLE_MQ_QUEUE", server.TestURL())
	t.Setenv("SAKURA_ENDPOINTS_SIMPLE_MQ_MESSAGE", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}

	var created queue.CommonServiceItem
	err := json.Unmarshal(runCommand("simplemq-api", "queue", "create", "--name", "mock-queue", "--description", "test queue"), &created)
	require.NoError(t, err)
	if id := simplemq.GetQueueID(&created); id == "" || simplemq.GetQueueName(&created) != "mock-queue" {
		t.Fatalf("create returned %#v, want queue ID and name", created)
	}
	queueID := simplemq.GetQueueID(&created)

	var listed []queue.CommonServiceItem
	err2 := json.Unmarshal(runCommand("simplemq-api", "queue", "list"), &listed)
	require.NoError(t, err2)
	if len(listed) != 1 || simplemq.GetQueueID(&listed[0]) != queueID {
		t.Fatalf("list returned %#v, want queue %s", listed, queueID)
	}
	var projected []struct {
		ID   string
		Name string
	}
	err3 := json.Unmarshal(runCommand("simplemq-api", "queue", "list", "--query", "map({ID,Name})"), &projected)
	require.NoError(t, err3)
	require.Len(t, projected, 1, "projected output = %+v, want created queue ID and name", projected)
	require.Equal(t, queueID, projected[0].ID, "unexpected value")
	require.Equal(t, "mock-queue", projected[0].Name, "unexpected value")

	var jsonCreated queue.CommonServiceItem
	err4 := json.Unmarshal(runCommand("simplemq-api", "queue", "create", "--request", `{"CommonServiceItem":{"Name":"json-queue"}}`), &jsonCreated)
	require.NoError(t, err4)
	if simplemq.GetQueueName(&jsonCreated) != "json-queue" {
		t.Fatalf("JSON create returned %#v, want json-queue", jsonCreated)
	}
	runCommand("simplemq-api", "queue", "delete", simplemq.GetQueueID(&jsonCreated))

	var configured queue.CommonServiceItem
	configRequest := `{"CommonServiceItem":{"Settings":{"VisibilityTimeoutSeconds":45,"ExpireSeconds":604800}}}`
	err5 := json.Unmarshal(runCommand("simplemq-api", "queue", "config", queueID, "--request", configRequest), &configured)
	require.NoError(t, err5)
	require.EqualValues(t, 45, configured.Settings.VisibilityTimeoutSeconds, "config returned settings %+v, want visibility timeout 45 and expiry 604800", configured.Settings)
	require.EqualValues(t, 604800, configured.Settings.ExpireSeconds, "unexpected value")
	var configuredByFlags queue.CommonServiceItem
	require.NoError(t, json.Unmarshal(runCommand(
		"simplemq-api", "queue", "config", queueID,
		"--visibility-timeout-seconds", "45", "--expire-seconds", "604800",
	), &configuredByFlags))
	require.Equal(t, configured.Settings, configuredByFlags.Settings, "flag config returned settings %+v, JSON config returned %+v", configuredByFlags.Settings, configured.Settings)

	var rotated struct {
		APIKey string
	}
	rotateOutput := runCommand("simplemq-api", "queue", "rotate-api-key", queueID)
	err6 := json.Unmarshal(rotateOutput, &rotated)
	require.NoError(t, err6)
	if rotated.APIKey == "" || !bytes.Contains(rotateOutput, []byte(rotated.APIKey)) {
		t.Fatalf("rotate returned %q, want APIKey", rotateOutput)
	}
	apiKeyPath := filepath.Join(t.TempDir(), "simplemq.key")
	err7 := os.WriteFile(apiKeyPath, []byte(rotated.APIKey), 0o600)
	require.NoError(t, err7)

	var read queue.CommonServiceItem
	err8 := json.Unmarshal(runCommand("simplemq-api", "queue", "read", queueID), &read)
	require.NoError(t, err8)
	if simplemq.GetQueueName(&read) != "mock-queue" {
		t.Fatalf("read returned %#v, want mock-queue", read)
	}

	var sent message.NewMessage
	err9 := json.Unmarshal(runCommand("simplemq-api", "message", "send", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, "--content", "aGVsbG8="), &sent)
	require.NoError(t, err9)
	require.Equal(t, "aGVsbG8=", string(sent.Content), "send returned %#v, want message content and ID", sent)
	require.NotEmpty(t, sent.ID, "should not be empty")

	var received []message.Message
	err10 := json.Unmarshal(runCommand("simplemq-api", "message", "receive", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath), &received)
	require.NoError(t, err10)
	require.Len(t, received, 1, "receive returned %#v, want sent message %s", received, sent.ID)
	require.Equal(t, sent.ID, received[0].ID, "unexpected value")
	require.Equal(t, sent.Content, received[0].Content, "unexpected value")

	var extended message.Message
	err11 := json.Unmarshal(runCommand("simplemq-api", "message", "extend-timeout", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, string(sent.ID)), &extended)
	require.NoError(t, err11)
	require.Equal(t, sent.ID, extended.ID, "extend-timeout returned ID %s, want %s", extended.ID, sent.ID)
	runCommand("simplemq-api", "message", "delete", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, string(sent.ID))

	var count map[string]int
	err12 := json.Unmarshal(runCommand("simplemq-api", "queue", "count-messages", queueID), &count)
	require.NoError(t, err12)
	require.Zero(t, count["Count"], "count-messages = %d, want 0", count["Count"])

	runCommand("simplemq-api", "message", "send", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, "--content", "dGVzdA==")
	runCommand("simplemq-api", "queue", "clear-messages", queueID)
	err13 := json.Unmarshal(runCommand("simplemq-api", "queue", "count-messages", queueID), &count)
	require.NoError(t, err13)
	require.Zero(t, count["Count"], "count after clear-messages = %d, want 0", count["Count"])

	runCommand("simplemq-api", "queue", "delete", queueID)
	listed = nil
	err14 := json.Unmarshal(runCommand("simplemq-api", "queue", "list"), &listed)
	require.NoError(t, err14)
	require.Len(t, listed, 0, "list after delete returned %#v, want empty", listed)
	emptyJSON := string(runCommand("simplemq-api", "queue", "list", "--query", "map({ID,Name})"))
	want := "[]\n"
	require.Equal(t, want, emptyJSON, "empty projected output = %q, want %q", emptyJSON, want)
}

func TestSimpleMQCommandInputErrors(t *testing.T) {
	for _, args := range [][]string{
		{"simplemq-api", "queue", "create"},
		{"simplemq-api", "queue", "create", "--request", `{"CommonServiceItem":{"Name":"mock-queue"}}`, "--name", "mock-queue"},
		{"simplemq-api", "queue", "config", "queue-id"},
		{"simplemq-api", "queue", "config", "queue-id", "--visibility-timeout-seconds", "30"},
		{"simplemq-api", "queue", "config", "queue-id", "--expire-seconds", "345600"},
		{"simplemq-api", "queue", "config", "queue-id", "--request", `{"CommonServiceItem":{"Settings":{"VisibilityTimeoutSeconds":30,"ExpireSeconds":345600}}}`, "--expire-seconds", "345600"},
		{"simplemq-api", "queue", "config", "queue-id", "--request", `{"CommonServiceItem":{"Settings":{"VisibilityTimeoutSeconds":30,"ExpireSeconds":345600}}}`, "--visibility-timeout-seconds", "0"},
		{"simplemq-api", "message", "receive", "--queue-name", "mock-queue", "--api-key-file", "/missing/simplemq.key"},
	} {
		var stdout, stderr bytes.Buffer
		exitCode := run(args, &stdout, &stderr)
		require.NotEqual(t, 0, exitCode, "run(%v) = %d stdout %q stderr %q; want an explicit error", args, exitCode, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
}
