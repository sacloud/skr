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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	saclient "github.com/sacloud/sacloud-sdk-go/common/saclient"
)

// LoadSecretInput は set-secret コマンドの secret 入力を読み込みます。
// 入力は @path 形式 (保護したファイル) または - (標準入力) のみを許可し、
// インライン指定や環境変数からの読み込みは行いません。
// JSON は SimpleMQ 宛先の {"APIKey":"..."} または API キー認証の
// {"AccessToken":"...","AccessTokenSecret":"..."} のみを受け付け、
// 未知のフィールド、両形式の混在、値の欠落・空文字列、文字列以外の型、
// 末尾の余分なデータを拒否します。
// 読み込んだ値や入力本文をエラーメッセージへ含めません。
func LoadSecretInput(input string) (v1.SetSecretRequestSecret, error) {
	data, err := readProtectedInput(input)
	if err != nil {
		return v1.SetSecretRequestSecret{}, err
	}
	return parseSecretInput(data)
}

// readProtectedInput は @path または - から secret の本文を読み込みます。
// それ以外の指定は受け付けません。
func readProtectedInput(input string) ([]byte, error) {
	if input == "" {
		return nil, fmt.Errorf("--secret-file を指定してください。@path で保護したファイル、- で標準入力から読み込みます")
	}
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("--secret-file - の標準入力を読み込めません: %w", err)
		}
		return data, nil
	}
	if !strings.HasPrefix(input, "@") {
		return nil, fmt.Errorf("--secret-file には @path で保護したファイル、または - (標準入力) を指定してください。値を引数に含めることはできません")
	}
	path := strings.TrimPrefix(input, "@")
	if path == "" {
		return nil, fmt.Errorf("--secret-file @ の後ろにファイルのパスを指定してください")
	}
	file, err := os.Open(path) // #nosec G304 -- パスは --secret-file で利用者が明示的に指定したものです。
	if err != nil {
		return nil, fmt.Errorf("--secret-file のファイル %q を開けません: %w", path, err)
	}
	defer file.Close() //nolint:errcheck // 読み取り専用のファイルのクローズエラーは読み込みエラーに含まれません。
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("--secret-file のファイル %q を読み込めません: %w", path, err)
	}
	return data, nil
}

// parseSecretInput は secret 入力の JSON を厳密に解釈して SetSecretRequestSecret を構築します。
func parseSecretInput(data []byte) (v1.SetSecretRequestSecret, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return v1.SetSecretRequestSecret{}, fmt.Errorf("--secret-file の JSON を解釈できません: %w", err)
	}
	// JSON オブジェクトの後ろに余分なデータがあれば拒否します。
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return v1.SetSecretRequestSecret{}, fmt.Errorf("--secret-file には JSON オブジェクトだけを指定してください")
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	switch {
	case equalStrings(keys, []string{"APIKey"}):
		value, err := secretStringField(fields["APIKey"])
		if err != nil {
			return v1.SetSecretRequestSecret{}, err
		}
		return v1.NewSimpleMQSecretSetSecretRequestSecret(v1.SimpleMQSecret{APIKey: value}), nil
	case equalStrings(keys, []string{"AccessToken", "AccessTokenSecret"}):
		accessToken, err := secretStringField(fields["AccessToken"])
		if err != nil {
			return v1.SetSecretRequestSecret{}, err
		}
		accessTokenSecret, err := secretStringField(fields["AccessTokenSecret"])
		if err != nil {
			return v1.SetSecretRequestSecret{}, err
		}
		return v1.NewSacloudAPISecretSetSecretRequestSecret(v1.SacloudAPISecret{
			AccessToken:       accessToken,
			AccessTokenSecret: accessTokenSecret,
		}), nil
	default:
		return v1.SetSecretRequestSecret{}, fmt.Errorf("--secret-file には SimpleMQ 宛先の場合は APIKey、API キー認証の場合は AccessToken と AccessTokenSecret を指定してください")
	}
}

// secretStringField は secret 入力の値を、文字列かつ空でない場合だけ返します。
func secretStringField(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("--secret-file の値は文字列で指定してください")
	}
	if value == "" {
		return "", fmt.Errorf("--secret-file の値は空にできません")
	}
	return value, nil
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// CheckTraceSettings は secret を扱う操作の前に、SDK の実効トレース設定を検査します。
// 設定解決のエラーを見逃さないよう解決を先に行い、TraceMode が有効な場合は
// 秘密情報がログへ出力されるおそれがあるため、操作を実行させずにエラーを返します。
// 認証情報を含み得る設定全体は返さず、エラーメッセージにも含めません。
func CheckTraceSettings(client *saclient.Client) error {
	if _, err := client.Profile(); err != nil {
		return fmt.Errorf("SDK の設定を解決できません: %w", err)
	}
	if mode, ok := client.JSON()["TraceMode"]; ok {
		return fmt.Errorf("secret を扱う操作ではトレースを無効にしてください (有効な TraceMode: %v)", mode)
	}
	return nil
}

// SanitizeSecretError は set-secret のエラーを安全な診断情報へ変換します。
// API エラーの原因や応答本文には secret や認証情報が含まれるおそのため、
// 分類だけを伝え、詳細は展開しません。それ以外のエラーはそのまま返します。
func SanitizeSecretError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *saclient.Error
	if errors.As(err, &apiErr) {
		switch {
		case saclient.IsNotFoundError(err):
			return fmt.Errorf("secret の登録先の実行設定が見つかりません (404)")
		case saclient.IsConflictError(err):
			return fmt.Errorf("secret の登録が競合しています (409)")
		default:
			return fmt.Errorf("secret の登録が失敗しました。入力内容と操作の権限を確認してください")
		}
	}
	return err
}
