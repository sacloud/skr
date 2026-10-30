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

package kmsapi

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/sacloud/sacloud-sdk-go/api/kms/apis/v1"
)

func TestCreateRequestParams(t *testing.T) {
	t.Run("flags only", func(t *testing.T) {
		name := "my-key"
		description := "description"
		plainKeyPath := filepath.Join(t.TempDir(), "plainkey.txt")
		require.NoError(t, os.WriteFile(plainKeyPath, []byte("c2VjcmV0\n"), 0o600))
		params, err := createRequestParams(nil, &name, &description, &plainKeyPath, nil)
		require.NoError(t, err)
		require.Equal(t, "my-key", params.Name)
		require.NotNil(t, params.Description)
		require.Equal(t, "description", *params.Description)
		require.NotNil(t, params.PlainKey)
		// 前後の空白は正規化される
		require.Equal(t, "c2VjcmV0", *params.PlainKey)
	})

	t.Run("name only", func(t *testing.T) {
		name := "my-key"
		params, err := createRequestParams(nil, &name, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "my-key", params.Name)
		require.Nil(t, params.Description)
		require.Nil(t, params.PlainKey)
	})

	t.Run("missing name", func(t *testing.T) {
		_, err := createRequestParams(nil, nil, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--name が必要です")
	})

	t.Run("request json file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "create.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"Name":"from-json","Description":"d","Tags":["env:test"],"PlainKey":"c2VjcmV0"}`), 0o600))
		decode := func(input string, dest any) error {
			data, err := os.ReadFile(input[1:])
			if err != nil {
				return err
			}
			return json.Unmarshal(data, dest)
		}
		input := "@" + path
		params2, err2 := createRequestParams(&input, nil, nil, nil, decode)
		require.NoError(t, err2)
		require.Equal(t, "from-json", params2.Name)
		require.NotNil(t, params2.PlainKey)
		require.Equal(t, "c2VjcmV0", *params2.PlainKey)
		require.Equal(t, []string{"env:test"}, params2.Tags)
	})

	t.Run("rejects inline json", func(t *testing.T) {
		inline := `{"Name":"my-key","PlainKey":"c2VjcmV0"}`
		_, err := createRequestParams(&inline, nil, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "@path.json")
		require.NotContains(t, err.Error(), "c2VjcmV0")
	})

	t.Run("rejects flags with request", func(t *testing.T) {
		input := "-"
		name := "my-key"
		_, err := createRequestParams(&input, &name, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "併用できません")
	})
}

func TestChangeKeyStateStatus(t *testing.T) {
	for _, test := range []struct {
		input string
		want  v1.ChangeKeyStateRequestStatus
	}{
		{input: "active", want: v1.ChangeKeyStateRequestStatusActive},
		{input: "restricted", want: v1.ChangeKeyStateRequestStatusRestricted},
		{input: "suspended", want: v1.ChangeKeyStateRequestStatusSuspended},
	} {
		t.Run(test.input, func(t *testing.T) {
			status, err := changeKeyStateStatus(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, status)
		})
	}
	t.Run("invalid", func(t *testing.T) {
		_, err := changeKeyStateStatus("invalid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "active、restricted、suspended")
	})
}

func TestEncryptionRequestAlgo(t *testing.T) {
	for _, test := range []struct {
		input string
		want  v1.EncryptionRequestAlgo
	}{
		{input: "", want: v1.EncryptionRequestAlgoAes256Gcm},
		{input: "aes-256-gcm", want: v1.EncryptionRequestAlgoAes256Gcm},
		{input: "aes-256-cbc", want: v1.EncryptionRequestAlgoAes256Cbc},
		{input: "aes-256-kw", want: v1.EncryptionRequestAlgoAes256Kw},
	} {
		t.Run(test.input, func(t *testing.T) {
			algo, err := encryptionRequestAlgo(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, algo)
		})
	}
	t.Run("invalid", func(t *testing.T) {
		_, err := encryptionRequestAlgo("invalid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "aes-256-gcm")
	})
}

func TestDecodeCipherText(t *testing.T) {
	t.Run("raw base64", func(t *testing.T) {
		cipher, err := decodeCipherText([]byte("c2VjcmV0\n"))
		require.NoError(t, err)
		require.Equal(t, "c2VjcmV0", cipher)
	})
	t.Run("quoted json output", func(t *testing.T) {
		cipher, err := decodeCipherText([]byte("\"c2VjcmV0==\"\n"))
		require.NoError(t, err)
		require.Equal(t, "c2VjcmV0==", cipher)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := decodeCipherText([]byte("  \n"))
		require.Error(t, err)
	})
	t.Run("roundtrip with base64", func(t *testing.T) {
		plain := "test-plain"
		encoded := base64.StdEncoding.EncodeToString([]byte(plain))
		cipher, err := decodeCipherText([]byte("\"" + encoded + "\""))
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(cipher)
		require.NoError(t, err)
		require.Equal(t, plain, string(decoded))
	})
}

func TestReadSecretFile(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(path, []byte("value\n"), 0o600))
		data, err := readSecretFile(path)
		require.NoError(t, err)
		require.Equal(t, "value\n", string(data))
	})
	t.Run("missing file", func(t *testing.T) {
		_, err := readSecretFile(filepath.Join(t.TempDir(), "missing.txt"))
		require.Error(t, err)
	})
}
