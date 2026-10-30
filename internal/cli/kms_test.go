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
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	kmsmock "github.com/sacloud/sakumock/kms"
)

func TestRunKmsAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"kms-api", "--help"},
			want: []string{"key", "FIPS 140-3", "AES-256", "削除予約制", "manual.sakura.ad.jp/cloud/appliance/kms/index.html"},
		},
		{
			args: []string{"kms-api", "key", "--help"},
			want: []string{"list", "read", "create", "update", "delete", "rotate", "change-status", "schedule-destruction", "encrypt", "decrypt"},
		},
		{
			args: []string{"kms-api", "key", "create", "--help"},
			want: []string{"--name", "--description", "--plain-key-file", "--request", "@path.json", "標準入力", "認証情報"},
		},
		{
			args: []string{"kms-api", "key", "update", "--help"},
			want: []string{"--name", "--description", "--request", "@path.json", "個別フラグと併用できません"},
		},
		{
			args: []string{"kms-api", "key", "list", "--help"},
			want: []string{"--count", "--from"},
		},
		{
			args: []string{"kms-api", "key", "change-status", "--help"},
			want: []string{"--status", "active", "restricted", "suspended"},
		},
		{
			args: []string{"kms-api", "key", "schedule-destruction", "--help"},
			want: []string{"--pending-days", "7〜90"},
		},
		{
			args: []string{"kms-api", "key", "encrypt", "--help"},
			want: []string{"--plain-file", "--algo", "aes-256-gcm", "aes-256-cbc", "aes-256-kw"},
		},
		{
			args: []string{"kms-api", "key", "decrypt", "--help"},
			want: []string{"--cipher-file", "base64"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				require.Contains(t, help, expected, "help does not contain %q:\n%s", expected, help)
			}
		})
	}
}

func TestRunKmsAPIRejectsInlineSensitiveCreateRequest(t *testing.T) {
	const material = "test-only-kms-imported-material"
	args := []string{"kms-api", "key", "create", "--request",
		`{"Name":"my-key","PlainKey":"` + material + `"}`}
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.NotContains(t, stderr.String(), material, "error = %q, want safe file-input guidance without secret", stderr.String())
	require.Contains(t, stderr.String(), "@path.json", "missing expected text %q", "@path.json")
}

func TestRunKmsAPIRejectsMissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"kms-api", "key", "create"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--name が必要です", "error = %q", stderr.String())
}

func TestRunKmsAPIRejectsInvalidChangeStatus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"kms-api", "key", "change-status", "123456789012", "--status", "invalid"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "active、restricted、suspended", "error = %q", stderr.String())
}

func TestRunKmsAPIRejectsInvalidAlgo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"kms-api", "key", "encrypt", "123456789012", "--plain-file", "plain.txt", "--algo", "invalid"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "aes-256-gcm", "error = %q", stderr.String())
}

func TestRunKmsAPIRejectsMissingPlainFile(t *testing.T) {
	// --plain-file にはファイルまたは標準入力を指定するため、存在しないパスは読み込みエラーになる
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"kms-api", "key", "encrypt", "123456789012", "--plain-file", "missing-plain.txt"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "read secret file", "error = %q", stderr.String())
}

func TestKmsAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := kmsmock.NewTestServer(kmsmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_KMS", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	// KMS API の応答フィールドは PascalCase です
	var created struct {
		ID           string `json:"ID"`
		Name         string `json:"Name"`
		Status       string `json:"Status"`
		KeyOrigin    string `json:"KeyOrigin"`
		ServiceClass string `json:"ServiceClass"`
	}
	err := json.Unmarshal(runCommand("kms-api", "key", "create", "--name", "mock-kms", "--description", "test key"), &created)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID, "created key = %+v", created)
	require.Equal(t, "mock-kms", created.Name, "unexpected value")
	require.Equal(t, "active", created.Status, "unexpected value")
	require.Equal(t, "generated", created.KeyOrigin, "unexpected value")
	require.Equal(t, "cloud/kms/key", created.ServiceClass, "unexpected value")
	keyID := created.ID

	createdData := runCommand("kms-api", "key", "read", keyID)
	var readKey struct {
		ID          string   `json:"ID"`
		Name        string   `json:"Name"`
		Description string   `json:"Description"`
		Tags        []string `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal(createdData, &readKey))
	require.Equal(t, keyID, readKey.ID, "unexpected value")
	require.Equal(t, "test key", readKey.Description, "unexpected value")

	// BYOK モード: --plain-key-file から秘密鍵を読み込んで作成する
	dir := t.TempDir()
	plainKeyFile := filepath.Join(dir, "plainkey.txt")
	require.NoError(t, os.WriteFile(plainKeyFile, []byte(base64.StdEncoding.EncodeToString([]byte("test-only-kms-imported-material"))+"\n"), 0o600))
	var imported struct {
		ID        string `json:"ID"`
		KeyOrigin string `json:"KeyOrigin"`
	}
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "create", "--name", "mock-kms-byok", "--plain-key-file", plainKeyFile), &imported))
	require.NotEmpty(t, imported.ID, "imported key = %+v", imported)
	require.Equal(t, "imported", imported.KeyOrigin, "unexpected value")

	var listed []struct {
		ID string `json:"ID"`
	}
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "list", "--count", "10", "--from", "0"), &listed))
	require.NotEmpty(t, listed, "key list = %+v, want created keys", listed)

	var updated struct {
		ID   string `json:"ID"`
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "update", keyID, "--name", "mock-kms-renamed", "--description", "renamed"), &updated))
	require.Equal(t, "mock-kms-renamed", updated.Name, "unexpected value")
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "rotate", keyID), &updated))
	require.Equal(t, keyID, updated.ID, "unexpected value")
	var restricted string
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "change-status", keyID, "--status", "restricted"), &restricted))
	require.Equal(t, "restricted", restricted, "unexpected value")
	var activated string
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "change-status", keyID, "--status", "active"), &activated))
	require.Equal(t, "active", activated, "unexpected value")

	// 暗号化・復号
	plainFile := filepath.Join(dir, "plain.txt")
	plain := "test-plain-text"
	require.NoError(t, os.WriteFile(plainFile, []byte(plain), 0o600))
	cipherJSON := runCommand("kms-api", "key", "encrypt", keyID, "--plain-file", plainFile)
	var cipherText string
	require.NoError(t, json.Unmarshal(cipherJSON, &cipherText))
	require.NotEmpty(t, cipherText, "encrypt result = %q", string(cipherJSON))

	cipherFile := filepath.Join(dir, "cipher.txt")
	require.NoError(t, os.WriteFile(cipherFile, cipherJSON, 0o600))
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"kms-api", "key", "decrypt", keyID, "--cipher-file", cipherFile}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Equal(t, plain, stdout.String(), "decrypt result mismatch")

	// 削除予約と取り消し
	var scheduled struct {
		Status                 string `json:"Status"`
		DeletionScheduledAfter string `json:"DeletionScheduledAfter"`
	}
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "schedule-destruction", keyID, "--pending-days", "7"), &scheduled))
	require.Equal(t, "pending_destruction", scheduled.Status, "schedule-destruction result = %+v", scheduled)
	require.NotEmpty(t, scheduled.DeletionScheduledAfter, "schedule-destruction result = %+v", scheduled)
	var canceled string
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "change-status", keyID, "--status", "active"), &canceled))
	require.Equal(t, "active", canceled, "canceled destruction schedule = %+v", canceled)

	runCommand("kms-api", "key", "delete", keyID)
	runCommand("kms-api", "key", "delete", imported.ID)
	var remaining []struct {
		ID string `json:"ID"`
	}
	require.NoError(t, json.Unmarshal(runCommand("kms-api", "key", "list", "--query", `map(select(.ID == "`+keyID+`" or .ID == "`+imported.ID+`"))`), &remaining))
	require.Len(t, remaining, 0, "deleted keys remain in list: %+v", remaining)
}
