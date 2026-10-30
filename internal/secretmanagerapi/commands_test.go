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

package secretmanagerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"

	secretmanager "github.com/sacloud/sacloud-sdk-go/api/secretmanager"
	v1 "github.com/sacloud/sacloud-sdk-go/api/secretmanager/apis/v1"
)

// fakeSecretAPI は SecretAPI の動作を置き換えるテスト用実装です。
type fakeSecretAPI struct {
	createVaultID string
	createRequest secretmanager.CreateSecretParams
	unveilRequest secretmanager.UnveilParams
	unveilValue   string
	createErr     error
	unveilErr     error
}

func (f *fakeSecretAPI) List(context.Context, string, *int, *int) ([]v1.SecretResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeSecretAPI) Create(_ context.Context, vaultID string, request secretmanager.CreateSecretParams) (*v1.CreateSecretResponse, error) {
	f.createVaultID = vaultID
	f.createRequest = request
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &v1.CreateSecretResponse{Name: request.Name, LatestVersion: 1}, nil
}

func (f *fakeSecretAPI) Delete(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *fakeSecretAPI) Unveil(_ context.Context, vaultID string, name string, version *int) (*v1.UnveilResponse, error) {
	f.unveilRequest = secretmanager.UnveilParams{Name: name, Version: version}
	if f.unveilErr != nil {
		return nil, f.unveilErr
	}
	unveiled := 1
	if version != nil {
		unveiled = *version
	}
	return &v1.UnveilResponse{Name: name, Version: unveiled, Value: f.unveilValue}, nil
}

// testRuntime は出力処理をスタブに差し替えた SecretRuntime です。
func testRuntime(stdout *bytes.Buffer) SecretRuntime {
	return SecretRuntime{
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput: func(ctx *kong.Context, value any) error {
			encoder := json.NewEncoder(ctx.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(value)
		},
	}
}

func parseCommand(t *testing.T, command any, args ...string) *kong.Context {
	t.Helper()
	parser, err := kong.New(command, kong.Exit(func(int) {}))
	require.NoError(t, err)
	ctx, err := parser.Parse(args)
	require.NoError(t, err)
	return ctx
}

func TestSecretValue(t *testing.T) {
	t.Run("trims surrounding whitespace", func(t *testing.T) {
		value, err := secretValue([]byte("  my-value\n"))
		require.NoError(t, err)
		require.Equal(t, "my-value", value)
	})

	t.Run("keeps inner whitespace", func(t *testing.T) {
		value, err := secretValue([]byte("line1\nline2\n"))
		require.NoError(t, err)
		require.Equal(t, "line1\nline2", value)
	})

	t.Run("rejects empty value", func(t *testing.T) {
		_, err := secretValue([]byte("   \n"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "secret value file is empty")
	})
}

func TestReadSecretFile(t *testing.T) {
	t.Run("reads from file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value.txt")
		require.NoError(t, os.WriteFile(path, []byte("from-file"), 0o600))
		data, err := readSecretFile(path)
		require.NoError(t, err)
		require.Equal(t, []byte("from-file"), data)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := readSecretFile(filepath.Join(t.TempDir(), "missing.txt"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read secret file")
	})
}

func TestSecretCreateCommandRun(t *testing.T) {
	t.Run("creates secret from value file", func(t *testing.T) {
		valueFile := filepath.Join(t.TempDir(), "value.txt")
		require.NoError(t, os.WriteFile(valueFile, []byte("  my-value \n"), 0o600))
		fake := &fakeSecretAPI{}
		command := &SecretCreateCommand{
			factory: func() (SecretAPI, error) { return fake, nil },
			runtime: testRuntime(nil),
		}
		kongCtx := parseCommand(t, command, "vault-1", "--name", "db-password", "--value-file", valueFile)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "vault-1", fake.createVaultID)
		require.Equal(t, "db-password", fake.createRequest.Name)
		// 前後の空白は正規化される
		require.Equal(t, "my-value", fake.createRequest.Value)
	})

	t.Run("missing value file fails at parse", func(t *testing.T) {
		command := &SecretCreateCommand{}
		parser, err := kong.New(command, kong.Exit(func(int) {}))
		require.NoError(t, err)
		_, err = parser.Parse([]string{"vault-1", "--name", "db-password"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--value-file")
	})

	t.Run("missing value file argument", func(t *testing.T) {
		fake := &fakeSecretAPI{}
		command := &SecretCreateCommand{
			factory: func() (SecretAPI, error) { return fake, nil },
			runtime: testRuntime(nil),
		}
		kongCtx := parseCommand(t, command, "vault-1", "--name", "db-password", "--value-file", filepath.Join(t.TempDir(), "missing.txt"))
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "read secret file")
	})

	t.Run("empty value file", func(t *testing.T) {
		valueFile := filepath.Join(t.TempDir(), "value.txt")
		require.NoError(t, os.WriteFile(valueFile, []byte("   \n"), 0o600))
		fake := &fakeSecretAPI{}
		command := &SecretCreateCommand{
			factory: func() (SecretAPI, error) { return fake, nil },
			runtime: testRuntime(nil),
		}
		kongCtx := parseCommand(t, command, "vault-1", "--name", "db-password", "--value-file", valueFile)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "secret value file is empty")
	})

	t.Run("factory error", func(t *testing.T) {
		valueFile := filepath.Join(t.TempDir(), "value.txt")
		require.NoError(t, os.WriteFile(valueFile, []byte("my-value"), 0o600))
		command := &SecretCreateCommand{
			factory: func() (SecretAPI, error) { return nil, errors.New("client error") },
			runtime: testRuntime(nil),
		}
		kongCtx := parseCommand(t, command, "vault-1", "--name", "db-password", "--value-file", valueFile)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "client error")
	})
}

func TestSecretUnveilCommandRun(t *testing.T) {
	t.Run("writes raw value to stdout", func(t *testing.T) {
		fake := &fakeSecretAPI{unveilValue: "revealed-value"}
		var stdout, stderr bytes.Buffer
		command := &SecretUnveilCommand{factory: func() (SecretAPI, error) { return fake, nil }}
		parser, err := kong.New(command, kong.Exit(func(int) {}), kong.Writers(&stdout, &stderr))
		require.NoError(t, err)
		kongCtx, err := parser.Parse([]string{"vault-1", "db-password"})
		require.NoError(t, err)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "revealed-value", stdout.String())
		require.Equal(t, "db-password", fake.unveilRequest.Name)
		require.Nil(t, fake.unveilRequest.Version, "secret-version not specified")
	})

	t.Run("passes secret-version", func(t *testing.T) {
		fake := &fakeSecretAPI{}
		command := &SecretUnveilCommand{factory: func() (SecretAPI, error) { return fake, nil }}
		kongCtx := parseCommand(t, command, "vault-1", "db-password", "--secret-version", "3")
		require.NoError(t, command.Run(kongCtx))
		require.NotNil(t, fake.unveilRequest.Version)
		require.Equal(t, 3, *fake.unveilRequest.Version)
	})

	t.Run("unveil error", func(t *testing.T) {
		fake := &fakeSecretAPI{unveilErr: errors.New("secret not found")}
		command := &SecretUnveilCommand{factory: func() (SecretAPI, error) { return fake, nil }}
		kongCtx := parseCommand(t, command, "vault-1", "missing")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "secret not found")
	})
}

func TestSecretOpDelegatesVaultID(t *testing.T) {
	// secretOp は Vault ID をメソッド引数で受け取る SecretAPI 実装であることを確認します。
	var op SecretAPI = &secretOp{}
	require.NotNil(t, op)
}
