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
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// userLimitResponse は CLI の JSON 出力を AppRun ユーザー情報として解釈するための項目です。
type userLimitResponse struct {
	Limit struct {
		ApplicationCount int `json:"application_count"`
	} `json:"limit"`
}

// runUserScenario は AppRun ユーザーの参照と、未サインアップの場合のサインアップを実行します。
// サインアップはアカウント単位で取り消せないため、既にサインアップ済みの場合は
// create の失敗を許容し、read で実際の状態を確認します。
func runUserScenario(t *testing.T, h *helper.Helper) {
	t.Helper()

	stdout, _, createErr := h.CallAllowingFailure(t, "AppRun ユーザーへサインアップする",
		"apprun-api", "user", "create")
	signedUpNow := createErr == nil
	var signedUp userLimitResponse
	if signedUpNow {
		// サインアップした場合は、応答の上限を確認する
		err := json.Unmarshal(stdout, &signedUp)
		require.NoError(t, err, "decode AppRun signup: %v", err)
	} else {
		// アカウントは既にサインアップ済みの可能性があります。次の参照で実際の状態を確認します。
		t.Logf("AppRun user create returned an error; assuming the account already signed up: %v", createErr)
	}

	// サインアップ後のユーザー情報を参照する
	read := parseUserLimit(t, h.Call(t, "AppRun ユーザーを参照する", "apprun-api", "user", "read"))
	require.Positive(t, read.Limit.ApplicationCount, "AppRun user limit application_count should be positive")
	if signedUpNow {
		require.Equal(t, signedUp.Limit.ApplicationCount, read.Limit.ApplicationCount,
			"AppRun signup limit application_count should match the read value")
	}
}

// parseUserLimit は CLI の JSON 出力を AppRun ユーザー情報として解釈します。
func parseUserLimit(t *testing.T, data []byte) userLimitResponse {
	t.Helper()

	var value userLimitResponse
	err := json.Unmarshal(data, &value)
	require.NoError(t, err, "decode AppRun user: %v", err)
	return value
}
