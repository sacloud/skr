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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// serviceClassZone はサービスクラスを検索するゾーンです。
const serviceClassZone = "is1b"

// serviceClassQuery は検索結果に含める項目を選択する jq 式です。
const serviceClassQuery = "map({ID,ServiceClassName,ServiceClassPath,DisplayName,IsPublic})"

// serviceClassSummary はサービスクラスの検索結果で確認する項目です。
type serviceClassSummary struct {
	ID               int64
	ServiceClassName string
	ServiceClassPath string
	DisplayName      string
	IsPublic         bool
}

// TestScenarioServiceClass は service-class コマンドのライブ E2E です。
// 選択中のプロファイルの確認と、指定ゾーンのサービスクラス検索を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioServiceClass(t *testing.T) {
	h := helper.New(t, "serviceclass-api",
		"IaaS API の service-class コマンドで選択中のプロファイルの確認と、指定ゾーンのサービスクラス検索が正しく動作することを確認する")

	// 選択中のプロファイルを確認する
	profile := strings.TrimSpace(string(h.Call(t, "選択中のプロファイルを確認する", "config", "current")))
	require.NotEmpty(t, profile, "selected profile is empty")

	// 指定ゾーンのサービスクラスを検索して内容を確認する
	classes := findServiceClasses(t, h, "指定ゾーンのサービスクラスを検索する")
	require.NotEmpty(t, classes, "service class list is empty in zone %s", serviceClassZone)
	for i, class := range classes {
		require.Positive(t, class.ID, "service class %d has no positive ID", i)
		require.NotEmpty(t, class.ServiceClassName, "service class %d has an empty name", i)
		require.NotEmpty(t, class.ServiceClassPath, "service class %d has an empty path", i)
	}
}

// findServiceClasses は指定ゾーンのサービスクラスを検索して応答を返します。
func findServiceClasses(t *testing.T, h *helper.Helper, description string) []serviceClassSummary {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, description, "iaas-api", "service-class", "find",
		"--zone", serviceClassZone, "--query", serviceClassQuery)
	require.NoError(t, err, "%s: service-class find: %v", description, err)
	var classes []serviceClassSummary
	require.NoError(t, json.Unmarshal(data, &classes), "%s: decode service class list: %v", description, err)
	return classes
}
