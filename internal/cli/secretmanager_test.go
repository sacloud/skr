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

	"github.com/stretchr/testify/require"

	smmock "github.com/sacloud/sakumock/secretmanager"
)

func TestRunSecretManagerAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"secretmanager-api", "--help"},
			want: []string{"vault", "secret", "50 世代", "KMS", "manual.sakura.ad.jp/cloud/appliance/secretsmanager/index.html"},
		},
		{
			args: []string{"secretmanager-api", "vault", "--help"},
			want: []string{"list", "read", "create", "update", "delete"},
		},
		{
			args: []string{"secretmanager-api", "vault", "create", "--help"},
			want: []string{"--name", "--description", "--kms-key-id", "--request", "@path.json", "課金"},
		},
		{
			args: []string{"secretmanager-api", "vault", "update", "--help"},
			want: []string{"--name", "--description", "--request", "個別フラグと併用できません"},
		},
		{
			args: []string{"secretmanager-api", "vault", "list", "--help"},
			want: []string{"--count", "--from"},
		},
		{
			args: []string{"secretmanager-api", "secret", "--help"},
			want: []string{"list", "create", "delete", "unveil"},
		},
		{
			args: []string{"secretmanager-api", "secret", "create", "--help"},
			want: []string{"--name", "--value-file", "標準入力", "upsert", "50 世代"},
		},
		{
			args: []string{"secretmanager-api", "secret", "unveil", "--help"},
			want: []string{"--secret-version", "--query", "秘密情報"},
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

func TestRunSecretManagerAPIRejectsMissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"secretmanager-api", "vault", "create", "--kms-key-id", "123456789012"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--name が必要です", "error = %q", stderr.String())
}

func TestRunSecretManagerAPIRejectsMissingKmsKeyID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"secretmanager-api", "vault", "create", "--name", "my-vault"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--kms-key-id が必要です", "error = %q", stderr.String())
}

func TestRunSecretManagerAPIRejectsMissingValueFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"secretmanager-api", "secret", "create", "vault-1", "--name", "db-password"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--value-file", "error = %q", stderr.String())
}

func TestRunSecretManagerAPIRejectsMissingValueFileArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"secretmanager-api", "secret", "create", "vault-1", "--name", "db-password", "--value-file", "missing-value.txt"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "read secret file", "error = %q", stderr.String())
}

func TestRunSecretManagerAPIRejectsVaultCreateWithRequestAndFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"secretmanager-api", "vault", "create", "--request", `{"Name":"my-vault","KmsKeyID":"123456789012"}`, "--name", "my-vault"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません", "error = %q", stderr.String())
}

func TestSecretManagerAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := smmock.NewTestServer(smmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_SECRETMANAGER", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	// Vault 作成 (KMS キー ID は必須)
	var created struct {
		ID        string `json:"ID"`
		Name      string `json:"Name"`
		KmsKeyID  string `json:"KmsKeyID"`
		CreatedAt string `json:"CreatedAt"`
	}
	err := json.Unmarshal(runCommand("secretmanager-api", "vault", "create", "--name", "mock-vault", "--kms-key-id", "123456789012"), &created)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID, "created vault = %+v", created)
	require.Equal(t, "mock-vault", created.Name, "unexpected value")
	require.Equal(t, "123456789012", created.KmsKeyID, "unexpected value")
	require.NotEmpty(t, created.CreatedAt, "created vault = %+v", created)
	vaultID := created.ID

	// Read は PascalCase の応答フィールドを返します
	var readVault struct {
		ID          string   `json:"ID"`
		Name        string   `json:"Name"`
		Description string   `json:"Description"`
		KmsKeyID    string   `json:"KmsKeyID"`
		Tags        []string `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "vault", "read", vaultID), &readVault))
	require.Equal(t, vaultID, readVault.ID, "unexpected value")
	require.Equal(t, "mock-vault", readVault.Name, "unexpected value")

	// List に作成した Vault が含まれること
	var listed []struct {
		ID string `json:"ID"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "vault", "list"), &listed))
	var found bool
	for _, vault := range listed {
		if vault.ID == vaultID {
			found = true
		}
	}
	require.True(t, found, "vault list = %+v, want created vault %s", listed, vaultID)

	// Update (Tags は個別フラグで更新するとクリアされる)
	var updated struct {
		ID          string `json:"ID"`
		Name        string `json:"Name"`
		Description string `json:"Description"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "vault", "update", vaultID, "--name", "mock-vault-renamed", "--description", "renamed"), &updated))
	require.Equal(t, "mock-vault-renamed", updated.Name, "unexpected value")
	require.Equal(t, "renamed", updated.Description, "unexpected value")

	// シークレットの作成 (upsert)
	dir := t.TempDir()
	valueFile := filepath.Join(dir, "value.txt")
	require.NoError(t, os.WriteFile(valueFile, []byte("secret-value-1\n"), 0o600))
	var secretCreated struct {
		Name          string `json:"Name"`
		LatestVersion int    `json:"LatestVersion"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "secret", "create", vaultID, "--name", "db-password", "--value-file", valueFile), &secretCreated))
	require.Equal(t, "db-password", secretCreated.Name, "unexpected value")
	require.Equal(t, 1, secretCreated.LatestVersion, "unexpected value")

	// 同名シークレットは新しいバージョンとして更新される (upsert)
	require.NoError(t, os.WriteFile(valueFile, []byte("secret-value-2\n"), 0o600))
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "secret", "create", vaultID, "--name", "db-password", "--value-file", valueFile), &secretCreated))
	require.Equal(t, 2, secretCreated.LatestVersion, "upsert should bump the version: %+v", secretCreated)

	// List にシークレットが含まれること
	var secretListed []struct {
		Name          string `json:"Name"`
		LatestVersion int    `json:"LatestVersion"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "secret", "list", vaultID), &secretListed))
	found = false
	for _, secret := range secretListed {
		if secret.Name == "db-password" {
			found = true
			require.Equal(t, 2, secret.LatestVersion, "secret = %+v", secret)
		}
	}
	require.True(t, found, "secret list = %+v, want created secret", secretListed)

	// unveil: 値をそのまま標準出力へ出力します (--query は適用されない)
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"secretmanager-api", "secret", "unveil", vaultID, "db-password"}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Empty(t, stderr.String(), "unveil stderr = %q", stderr.String())
	require.Equal(t, "secret-value-2", stdout.String(), "unveil result mismatch")

	// --secret-version で過去のバージョンを取得できること
	stdout.Reset()
	require.Equal(t, 0, run([]string{"secretmanager-api", "secret", "unveil", vaultID, "db-password", "--secret-version", "1"}, &stdout, &stderr), "stderr: %s", stderr.String())
	require.Equal(t, "secret-value-1", stdout.String(), "unveil with secret-version result mismatch")

	// シークレット削除と不在確認
	runCommand("secretmanager-api", "secret", "delete", vaultID, "db-password")
	var remaining []struct {
		Name string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "secret", "list", vaultID), &remaining))
	for _, secret := range remaining {
		require.NotEqual(t, "db-password", secret.Name, "deleted secret remains in list: %+v", remaining)
	}

	// Vault 削除と不在確認
	runCommand("secretmanager-api", "vault", "delete", vaultID)
	var remainingVaults []struct {
		ID string `json:"ID"`
	}
	require.NoError(t, json.Unmarshal(runCommand("secretmanager-api", "vault", "list", "--query", `map(select(.ID == "`+vaultID+`"))`), &remainingVaults))
	require.Len(t, remainingVaults, 0, "deleted vault remains in list: %+v", remainingVaults)
}
