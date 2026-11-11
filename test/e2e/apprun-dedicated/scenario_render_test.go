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

// このファイルは apprun-dedicated コマンドのうち、API を変更しない render の
// ライブ E2E シナリオです。
package main

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioAppRunDedicatedRender は render コマンドのライブ E2E です。
// render は API に接続せず、定義の検証と正規化、秘密情報の出力からの除外を確認します。
// リソースを作成しないため費用は発生しません。
func TestScenarioAppRunDedicatedRender(t *testing.T) {
	h := helper.New(t, "apprun-dedicated",
		"AppRun 専有型の高レベルコマンドのうち render を実行し、定義の検証と正規化、秘密情報が出力に含まれないことを確認する (API に接続せず、リソースを作成しない)")

	suffix := helper.RandomSuffix(t)
	cluster := "skr-e2e-render-cluster-" + suffix
	name := "skr-e2e-render-app-" + suffix
	// registryPassword と secret=true の環境変数の値は、出力に現れないことを
	// 確認するためのダミー値です。実在する認証情報ではありません。
	registryPassword := "not-a-real-password-" + suffix
	secretValue := "not-a-real-secret-" + suffix
	definition := map[string]any{
		"cluster":          cluster,
		"name":             name,
		"image":            "ghcr.io/sacloud/skr-e2e-apprun-dedicated:render",
		"cpu":              1000,
		"memory":           1024,
		"scalingMode":      "manual",
		"fixedScale":       1,
		"registryUsername": "skr-e2e",
		"registryPassword": registryPassword,
		"env": []map[string]any{
			{"key": "PUBLIC_VALUE", "value": "visible", "secret": false},
			{"key": "SECRET_VALUE", "value": secretValue, "secret": true},
		},
	}
	path := h.WriteRequestJSONFile(t, "render-definition", definition)

	stdout, stderr, err := h.CallAllowingFailure(t, "render で定義を検証して正規化した結果を表示する",
		"apprun-dedicated", "render", "--app", path)
	require.NoError(t, err, "stderr: %s", stderr)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(stdout, &rendered), "stdout: %s", stdout)

	// 正規化された定義の内容を確認する。
	require.Equal(t, cluster, rendered["cluster"])
	require.Equal(t, name, rendered["name"])
	require.Equal(t, definition["image"], rendered["image"])
	require.EqualValues(t, definition["cpu"], rendered["cpu"])
	require.EqualValues(t, definition["memory"], rendered["memory"])
	require.Equal(t, "manual", rendered["scalingMode"])
	require.EqualValues(t, 1, rendered["fixedScale"])
	require.Equal(t, "skr-e2e", rendered["registryUsername"])

	// registryPassword と secret=true の環境変数の値は出力に含めない。
	require.NotContains(t, string(stdout), registryPassword)
	require.NotContains(t, string(stdout), secretValue)

	// secret=false の環境変数の値は出力に含める。
	env, ok := rendered["env"].([]any)
	require.True(t, ok, "env should be an array: %v", rendered["env"])
	require.Len(t, env, 2)
	byName := map[string]any{}
	for i := range env {
		entry, ok := env[i].(map[string]any)
		require.True(t, ok, "env[%d] should be an object: %v", i, env[i])
		byName[entry["key"].(string)] = entry
	}
	public, ok := byName["PUBLIC_VALUE"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "visible", public["value"])
	secret, ok := byName["SECRET_VALUE"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, secret["value"], "secret=true の環境変数の値は出力に含めません")
}
