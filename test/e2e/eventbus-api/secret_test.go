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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeSecretFiles は発行した SimpleMQ API キーを、秘密情報を受け取る CLI フラグ用の一時ファイルに書き込みます。
// キーを引数へ渡さないため、--api-key-file と --secret-file の両方で同じ発行済みキーを使えます。
// ファイルは 0600 で作成され、テスト終了後に一時ディレクトリとともに削除されます。
func writeSecretFiles(t *testing.T, apiKey string) (keyFile, secretFile string) {
	t.Helper()

	dir := t.TempDir()
	keyFile = filepath.Join(dir, "simplemq.key")
	if err := os.WriteFile(keyFile, []byte(apiKey), 0o600); err != nil {
		t.Fatalf("write SimpleMQ API key file: %v", err)
	}
	secretFile = filepath.Join(dir, "eventbus-secret.json")
	//nolint:gosec // 発行済みの API キーは所有者のみ読める 0600 の一時ファイルにのみ書き込まれる。
	data, err := json.Marshal(struct {
		APIKey string `json:"APIKey"`
	}{APIKey: apiKey})
	if err != nil {
		t.Fatalf("encode EventBus secret: %v", err)
	}
	if err := os.WriteFile(secretFile, data, 0o600); err != nil {
		t.Fatalf("write EventBus secret file: %v", err)
	}
	return keyFile, secretFile
}

// parseAPIKey は rotate-api-key の応答から API キーを取り出します。
func parseAPIKey(data []byte) (string, error) {
	var response struct {
		APIKey string `json:"APIKey"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode API key: %w", err)
	}
	if response.APIKey == "" {
		return "", errors.New("SimpleMQ API key is empty")
	}
	return response.APIKey, nil
}

// TestWriteSecretFiles は秘密情報を受け取る CLI フラグ用ファイルの作成を確認する。
func TestWriteSecretFiles(t *testing.T) {
	keyFile, secretFile := writeSecretFiles(t, "test-only-api-key")

	// SimpleMQ API キーは 0600 のファイルに、鍵の値そのまま書かれる
	info, err := os.Stat(keyFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	data, err := os.ReadFile(keyFile) //nolint:gosec // テスト専用ディレクトリ内のファイルを読み込む。
	require.NoError(t, err)
	require.Equal(t, "test-only-api-key", string(data))

	// EventBus のシークレット登録用 JSON も 0600 で作成される
	secretInfo, err := os.Stat(secretFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), secretInfo.Mode().Perm())
	secretData, err := os.ReadFile(secretFile) //nolint:gosec // テスト専用ディレクトリ内のファイルを読み込む。
	require.NoError(t, err)
	require.JSONEq(t, `{"APIKey":"test-only-api-key"}`, string(secretData))
}

// TestParseAPIKey は rotate-api-key の応答から API キーを取り出す処理を確認する。
func TestParseAPIKey(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		apiKey, err := parseAPIKey([]byte(`{"APIKey":"test-only-api-key"}`))
		require.NoError(t, err)
		require.Equal(t, "test-only-api-key", apiKey)
	})
	t.Run("empty key", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`{"APIKey":""}`))
		require.Error(t, err)
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := parseAPIKey([]byte(`not-json`))
		require.Error(t, err)
	})
}
