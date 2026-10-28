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

// このファイルは http コマンドの E2E シナリオです。
// 選択中のプロファイルで認証付き HTTP GET リクエストを送信し、生の JSON 応答を確認します。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// zoneListURL は http コマンドで呼び出す設備関連 API の zone 一覧 URL です。
const zoneListURL = "https://secure.sakura.ad.jp/cloud/zone/is1b/api/cloud/1.1/zone"

// TestScenarioHTTP は http コマンドのライブ E2E です。
// zone API へ読み取り専用の GET リクエストを送信し、生の応答が空でなく有効な JSON であることを確認します。
// テストはリソースを作成・変更・削除しませんが、認証情報を secure.sakura.ad.jp へ送信します。
func TestScenarioHTTP(t *testing.T) {
	h := helper.New(t, "http-api",
		"skr http コマンドで HTTP リクエストを送信し、応答の取得と終了コードが正しく動作することを確認する")

	// 選択中の SDK プロファイルを確認する
	profile, _, err := h.CallAllowingFailure(t, "現在選択されている SDK プロファイルを確認する",
		"config", "current")
	require.NoError(t, err, "read the selected SDK profile")
	require.NotEmpty(t, strings.TrimSpace(string(profile)),
		"a selected SDK profile is required")

	// zone API へ読み取り専用の GET リクエストを送信する
	body := h.Call(t, "さくらのクラウド zone API への GET リクエストが JSON 応答を返すことを確認する",
		"http", zoneListURL, "--method", "GET")
	require.NoError(t, validateJSONResponse(body),
		"the zone API response should be non-empty valid JSON")
}

// validateJSONResponse は生の HTTP 応答が空でなく有効な JSON であることを確認します。
// API のリクエスト／レスポンス形状を独自に再実装せず、応答形式だけを検証します。
func validateJSONResponse(body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return errors.New("HTTP response is empty")
	}
	if !json.Valid(trimmed) {
		return errors.New("HTTP response is not valid JSON")
	}
	return nil
}

// TestValidateJSONResponse は HTTP 応答の JSON 検証を確認する。
func TestValidateJSONResponse(t *testing.T) {
	require.NoError(t, validateJSONResponse([]byte(`{"response":{"zones":[]}}`)))
	require.NoError(t, validateJSONResponse([]byte("  {\"response\":{}}  \n")))

	for name, body := range map[string][]byte{
		"empty":     nil,
		"blank":     []byte("   \n"),
		"not json":  []byte("not-json"),
		"truncated": []byte(`{"response":`),
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, validateJSONResponse(body),
				"invalid HTTP responses should be rejected")
		})
	}
}
