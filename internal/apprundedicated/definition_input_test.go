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

package apprundedicated

import (
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/stretchr/testify/require"
)

func TestLoadDefinitionFromFile(t *testing.T) {
	path := t.TempDir() + "/application.json"
	content := `{"cluster":"c1","name":"app1","image":"ghcr.io/example/app:v1"}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	def, err := loadDefinition(strings.NewReader(""), path)
	require.NoError(t, err)
	require.Equal(t, "c1", def.Cluster)
	require.Equal(t, "app1", def.Name)
	require.Equal(t, "ghcr.io/example/app:v1", def.Image)
}

func TestLoadDefinitionFromStdin(t *testing.T) {
	def, err := loadDefinition(strings.NewReader(`{"cluster":"c1","name":"app1"}`), "-")
	require.NoError(t, err)
	require.Equal(t, "c1", def.Cluster)
	require.Equal(t, "app1", def.Name)
}

func TestLoadDefinitionErrors(t *testing.T) {
	t.Run("空の指定", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(""), "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--app")
	})

	t.Run("ファイル不在", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(""), t.TempDir()+"/missing.json")
		require.Error(t, err)
		require.Contains(t, err.Error(), "存在しません")
	})

	// --app は値のインライン指定を受け付けないため、利用者が誤って秘密情報を含む
	// 値を指定した場合も、その値がエラーメッセージへ漏れてはいけません。
	t.Run("読み込みエラーに入力値を含めない", func(t *testing.T) {
		inline := `{"registryPassword":"SECRET-VALUE"}`
		_, err := loadDefinition(strings.NewReader(""), inline)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--app")
		require.NotContains(t, err.Error(), "SECRET-VALUE")
		require.NotContains(t, err.Error(), inline)
	})

	t.Run("未知のフィールド", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(`{"cluster":"c1","name":"app1","unknown":1}`), "-")
		require.Error(t, err)
		require.Contains(t, err.Error(), "解釈できません")
	})

	t.Run("末尾の余分なデータ", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(`{"cluster":"c1","name":"app1"}{}`), "-")
		require.Error(t, err)
		require.Contains(t, err.Error(), "末尾に余分なデータ")
	})

	t.Run("cluster の欠落", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(`{"name":"app1"}`), "-")
		require.Error(t, err)
		require.Contains(t, err.Error(), "cluster は必須です")
	})

	t.Run("name の欠落", func(t *testing.T) {
		_, err := loadDefinition(strings.NewReader(`{"cluster":"c1"}`), "-")
		require.Error(t, err)
		require.Contains(t, err.Error(), "name は必須です")
	})
}

func TestReadConfirm(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      bool
		wantError bool
	}{
		{name: "y で続行", input: "y\n", want: true},
		{name: "Y で続行", input: "Y\n", want: true},
		{name: "n で中止", input: "n\n", want: false},
		{name: "N で中止", input: "N\n", want: false},
		{name: "空行で中止", input: "\n", want: false},
		{name: "即座に EOF で中止", input: "", want: false},
		{name: "EOF までの応答でも中止", input: "no\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var promptOut strings.Builder
			got, err := readConfirm(strings.NewReader(test.input), &promptOut)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}

	t.Run("y 以外の入力は再度応答を求めます", func(t *testing.T) {
		var promptOut strings.Builder
		got, err := readConfirm(strings.NewReader("yes\nn\n"), &promptOut)
		require.NoError(t, err)
		require.False(t, got)
		require.Contains(t, promptOut.String(), "y または n を入力してください")
	})
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
	require.Contains(t, err.Error(), "deploy")
	require.Contains(t, err.Error(), "トレースを無効にしてください")
}

func TestVersionNumberFlag(t *testing.T) {
	t.Run("未指定は nil のまま", func(t *testing.T) {
		got, err := versionNumberFlag(nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("指定した値へ変換する", func(t *testing.T) {
		target := 3
		got, err := versionNumberFlag(&target)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.EqualValues(t, 3, *got)
	})

	t.Run("負の値は拒否", func(t *testing.T) {
		target := -1
		_, err := versionNumberFlag(&target)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--target")
	})

	t.Run("int32 を超える値は拒否", func(t *testing.T) {
		target := 1 << 31
		_, err := versionNumberFlag(&target)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--target")
	})
}
