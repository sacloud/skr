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

package main

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
)

func TestSimpleMQGeneratedCodeMatchesConfig(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{config: "api/commands/simplemq-queue.json", output: "internal/simplemqapi/simplemq_queue_api_generated.go"},
		{config: "api/commands/simplemq-message.json", output: "internal/simplemqapi/simplemq_message_api_generated.go"},
	} {
		configData, err := os.ReadFile(test.config)
		if err != nil {
			t.Fatal(err)
		}
		config, err := apigen.DecodeConfig(configData)
		if err != nil {
			t.Fatal(err)
		}
		want, err := apigen.Generate(config)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(test.output)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("generated SimpleMQ commands are stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
		}
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
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("run(%v) exit code = %d; stderr: %s", test.args, exitCode, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help output does not contain %q", text)
				}
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
		if exitCode := run(args, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("run(%v) exit code = %d; stderr: %s", args, exitCode, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.Bytes()
	}

	var created queue.CommonServiceItem
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "create", "--name", "mock-queue", "--description", "test queue"), &created); err != nil {
		t.Fatal(err)
	}
	if id := simplemq.GetQueueID(&created); id == "" || simplemq.GetQueueName(&created) != "mock-queue" {
		t.Fatalf("create returned %#v, want queue ID and name", created)
	}
	queueID := simplemq.GetQueueID(&created)

	var listed []queue.CommonServiceItem
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "list"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || simplemq.GetQueueID(&listed[0]) != queueID {
		t.Fatalf("list returned %#v, want queue %s", listed, queueID)
	}
	t.Setenv("COLUMNS", "200")
	table := string(runCommand("simplemq-api", "queue", "list", "--output", "table"))
	if !strings.Contains(table, queueID) || !strings.Contains(table, "mock-queue") {
		t.Fatalf("table output = %q, want created queue ID and name", table)
	}

	var jsonCreated queue.CommonServiceItem
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "create", "--request", `{"CommonServiceItem":{"Name":"json-queue"}}`), &jsonCreated); err != nil {
		t.Fatal(err)
	}
	if simplemq.GetQueueName(&jsonCreated) != "json-queue" {
		t.Fatalf("JSON create returned %#v, want json-queue", jsonCreated)
	}
	runCommand("simplemq-api", "queue", "delete", simplemq.GetQueueID(&jsonCreated))

	var configured queue.CommonServiceItem
	configRequest := `{"CommonServiceItem":{"Settings":{"VisibilityTimeoutSeconds":45,"ExpireSeconds":604800}}}`
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "config", queueID, "--request", configRequest), &configured); err != nil {
		t.Fatal(err)
	}
	if configured.Settings.VisibilityTimeoutSeconds != 45 || configured.Settings.ExpireSeconds != 604800 {
		t.Fatalf("config returned settings %+v, want visibility timeout 45 and expiry 604800", configured.Settings)
	}
	var configuredByFlags queue.CommonServiceItem
	if err := json.Unmarshal(runCommand(
		"simplemq-api", "queue", "config", queueID,
		"--visibility-timeout-seconds", "45", "--expire-seconds", "604800",
	), &configuredByFlags); err != nil {
		t.Fatal(err)
	}
	if configuredByFlags.Settings != configured.Settings {
		t.Fatalf("flag config returned settings %+v, JSON config returned %+v", configuredByFlags.Settings, configured.Settings)
	}

	var rotated struct {
		APIKey string
	}
	rotateOutput := runCommand("simplemq-api", "queue", "rotate-api-key", queueID, "--output", "json")
	if err := json.Unmarshal(rotateOutput, &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.APIKey == "" || !bytes.Contains(rotateOutput, []byte(rotated.APIKey)) {
		t.Fatalf("rotate returned %q, want APIKey", rotateOutput)
	}
	apiKeyPath := filepath.Join(t.TempDir(), "simplemq.key")
	if err := os.WriteFile(apiKeyPath, []byte(rotated.APIKey), 0o600); err != nil {
		t.Fatal(err)
	}

	var read queue.CommonServiceItem
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "read", queueID), &read); err != nil {
		t.Fatal(err)
	}
	if simplemq.GetQueueName(&read) != "mock-queue" {
		t.Fatalf("read returned %#v, want mock-queue", read)
	}

	var sent message.NewMessage
	if err := json.Unmarshal(runCommand("simplemq-api", "message", "send", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, "--content", "aGVsbG8="), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Content != "aGVsbG8=" || sent.ID == "" {
		t.Fatalf("send returned %#v, want message content and ID", sent)
	}

	var received []message.Message
	if err := json.Unmarshal(runCommand("simplemq-api", "message", "receive", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath), &received); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].ID != sent.ID || received[0].Content != sent.Content {
		t.Fatalf("receive returned %#v, want sent message %s", received, sent.ID)
	}

	var extended message.Message
	if err := json.Unmarshal(runCommand("simplemq-api", "message", "extend-timeout", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, string(sent.ID)), &extended); err != nil {
		t.Fatal(err)
	}
	if extended.ID != sent.ID {
		t.Fatalf("extend-timeout returned ID %s, want %s", extended.ID, sent.ID)
	}
	runCommand("simplemq-api", "message", "delete", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, string(sent.ID))

	var count map[string]int
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "count-messages", queueID), &count); err != nil {
		t.Fatal(err)
	}
	if count["Count"] != 0 {
		t.Errorf("count-messages = %d, want 0", count["Count"])
	}

	runCommand("simplemq-api", "message", "send", "--queue-name", "mock-queue", "--api-key-file", apiKeyPath, "--content", "dGVzdA==")
	runCommand("simplemq-api", "queue", "clear-messages", queueID)
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "count-messages", queueID), &count); err != nil {
		t.Fatal(err)
	}
	if count["Count"] != 0 {
		t.Errorf("count after clear-messages = %d, want 0", count["Count"])
	}

	runCommand("simplemq-api", "queue", "delete", queueID)
	listed = nil
	if err := json.Unmarshal(runCommand("simplemq-api", "queue", "list"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("list after delete returned %#v, want empty", listed)
	}
	emptyTable := string(runCommand("simplemq-api", "queue", "list", "--output", "table"))
	if want := "+------------+\n| No results |\n+------------+\n"; emptyTable != want {
		t.Errorf("empty table output = %q, want %q", emptyTable, want)
	}
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
		if exitCode := run(args, &stdout, &stderr); exitCode == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("run(%v) = %d stdout %q stderr %q; want an explicit error", args, exitCode, stdout.String(), stderr.String())
		}
	}
}
