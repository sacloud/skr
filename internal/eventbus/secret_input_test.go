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

package eventbus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/stretchr/testify/require"
)

// writeSecretFile は secret 入力のテストファイルを作成します。
func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret-input.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadSecretInputSimpleMQ(t *testing.T) {
	path := writeSecretFile(t, `{"APIKey":"SECRET-VALUE-123"}`)
	secret, err := LoadSecretInput("@" + path)
	require.NoError(t, err)
	require.True(t, secret.IsSimpleMQSecret(), "secret type = %v, want SimpleMQSecret", secret.Type)
	require.Equal(t, "SECRET-VALUE-123", secret.SimpleMQSecret.APIKey)
}

func TestLoadSecretInputAPIKeyAuth(t *testing.T) {
	path := writeSecretFile(t, `{"AccessToken":"SECRET-TOKEN-123","AccessTokenSecret":"SECRET-TOKEN-SECRET-123"}`)
	secret, err := LoadSecretInput("@" + path)
	require.NoError(t, err)
	require.True(t, secret.IsSacloudAPISecret(), "secret type = %v, want SacloudAPISecret", secret.Type)
	require.Equal(t, "SECRET-TOKEN-123", secret.SacloudAPISecret.AccessToken)
	require.Equal(t, "SECRET-TOKEN-SECRET-123", secret.SacloudAPISecret.AccessTokenSecret)
}

func TestLoadSecretInputStdin(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdin
	os.Stdin = reader
	defer func() {
		os.Stdin = original
		require.NoError(t, reader.Close())
	}()
	_, err = writer.WriteString(`{"APIKey":"SECRET-PIPE-123"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	secret, err := LoadSecretInput("-")
	require.NoError(t, err)
	require.True(t, secret.IsSimpleMQSecret(), "secret type = %v, want SimpleMQSecret", secret.Type)
	require.Equal(t, "SECRET-PIPE-123", secret.SimpleMQSecret.APIKey)
}

func TestLoadSecretInputRejectsInputs(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		want    string
		notWant string
	}{
		{name: "inline value", input: "SECRET-VALUE-123", want: "@path", notWant: "SECRET-VALUE-123"},
		{name: "inline JSON", input: `{"APIKey":"SECRET-VALUE-123"}`, want: "@path", notWant: "SECRET-VALUE-123"},
		{name: "env reference", input: "$SECRET_ENV", want: "@path", notWant: "SECRET_ENV"},
		{name: "at sign only", input: "@", want: "パス"},
		{name: "missing file", input: "@/does/not/exist.json", want: "開けません"},
		{name: "empty", input: "", want: "--secret-file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadSecretInput(test.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
			if test.notWant != "" {
				require.NotContains(t, err.Error(), test.notWant)
			}
		})
	}
}

func TestParseSecretInputRejectsInvalidJSON(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{name: "not JSON", data: "not-json", want: "解釈できません"},
		{name: "array", data: `["APIKey"]`, want: "解釈できません"},
		{name: "mixed forms", data: `{"APIKey":"SECRET","AccessToken":"SECRET"}`, want: "APIKey"},
		{name: "mixed secret form", data: `{"AccessToken":"SECRET"}`, want: "AccessTokenSecret"},
		{name: "unknown field", data: `{"APIKey":"SECRET","extra":"x"}`, want: "APIKey"},
		{name: "unknown field only", data: `{"extra":"x"}`, want: "APIKey"},
		{name: "empty object", data: `{}`, want: "APIKey"},
		{name: "empty value", data: `{"APIKey":""}`, want: "空にできません"},
		{name: "empty access token secret", data: `{"AccessToken":"SECRET","AccessTokenSecret":""}`, want: "空にできません"},
		{name: "non string value", data: `{"APIKey":123}`, want: "文字列"},
		{name: "null value", data: `{"APIKey":null}`, want: "空にできません"},
		{name: "trailing data", data: `{"APIKey":"SECRET"} trailing`, want: "JSON オブジェクトだけ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSecretInput([]byte(test.data))
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
			require.NotContains(t, err.Error(), "SECRET", "エラーメッセージに値を含めてはいけません")
		})
	}
}

func TestCheckTraceSettings(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")

	var plain saclient.Client
	require.NoError(t, plain.SetEnviron(os.Environ()))
	require.NoError(t, CheckTraceSettings(&plain))

	var traced saclient.Client
	require.NoError(t, traced.SetEnviron(os.Environ()))
	require.NoError(t, traced.SetWith(saclient.WithTraceMode("all")))
	err := CheckTraceSettings(&traced)
	require.Error(t, err)
	require.Contains(t, err.Error(), "トレースを無効にしてください")
}

func TestSanitizeSecretError(t *testing.T) {
	require.NoError(t, SanitizeSecretError(nil))

	notFound := saclient.NewError(404, "NOT-FOUND-DETAIL", nil)
	got := SanitizeSecretError(notFound)
	require.Error(t, got)
	require.Contains(t, got.Error(), "見つかりません (404)")
	require.NotContains(t, got.Error(), "NOT-FOUND-DETAIL")

	conflict := saclient.NewError(409, "CONFLICT-DETAIL", nil)
	got = SanitizeSecretError(conflict)
	require.Error(t, got)
	require.Contains(t, got.Error(), "競合しています (409)")
	require.NotContains(t, got.Error(), "CONFLICT-DETAIL")

	serverError := saclient.NewError(500, "INTERNAL-DETAIL", nil)
	got = SanitizeSecretError(serverError)
	require.Error(t, got)
	require.Contains(t, got.Error(), "secret の登録が失敗しました")
	require.NotContains(t, got.Error(), "INTERNAL-DETAIL")

	// API エラーが別のエラーで包まれていても分類します。
	wrapped := fmt.Errorf("update secret: %w", saclient.NewError(404, "NOT-FOUND-DETAIL", nil))
	got = SanitizeSecretError(wrapped)
	require.Error(t, got)
	require.Contains(t, got.Error(), "見つかりません (404)")

	// API エラー以外はそのまま返します。
	local := errors.New("ID が必要です")
	got = SanitizeSecretError(local)
	require.Equal(t, local, got)
}
