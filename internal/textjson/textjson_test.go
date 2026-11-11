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

package textjson

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	fields, err := Parse("key=subject,op=in,value=a=b,value= c ,flag=x", []Field{
		{Name: "key", Required: true},
		{Name: "op", Required: true},
		{Name: "value", Required: true, Repeated: true},
		{Name: "flag"},
	})
	require.NoError(t, err)

	// 最初の "=" で分割し、後続の "=" と空白は字義どおり保持する
	require.Equal(t, "subject", fields.Get("key"))
	require.Equal(t, "in", fields.Get("op"))
	require.Equal(t, []string{"a=b", " c "}, fields.GetValues("value"))
	require.Equal(t, "x", fields.Get("flag"))

	// 反復したキーの Get は最初の値を返す
	require.Equal(t, "a=b", fields.Get("value"))

	// 未指定のキーは空文字列と nil、Has は false
	require.Equal(t, "", fields.Get("missing"))
	require.Nil(t, fields.GetValues("missing"))
	require.False(t, fields.Has("missing"))
	require.True(t, fields.Has("key"))

	// 零値の Fields も安全に参照できる
	var empty Fields
	require.Equal(t, "", empty.Get("key"))
	require.Nil(t, empty.GetValues("key"))
	require.False(t, empty.Has("key"))
}

func TestParseRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		spec    []Field
		wantErr string
	}{
		{"empty input", "", []Field{{Name: "key"}}, "key=value 形式"},
		{"empty element", "key=x,", []Field{{Name: "key"}}, "key=value 形式"},
		{"missing equal", "key", []Field{{Name: "key"}}, "key=value 形式"},
		{"empty key", "=value", []Field{{Name: "key"}}, "key=value 形式"},
		{"empty value", "key=", []Field{{Name: "key"}}, "key=value 形式"},
		{"unknown key", "unknown=x,key=x", []Field{{Name: "key"}}, "未対応のキーです。指定可能なキー: key"},
		{"duplicate key", "key=x,key=y", []Field{{Name: "key"}}, "key は重複して指定できません"},
		{"missing required", "op=eq", []Field{{Name: "key", Required: true}, {Name: "op", Required: true}},
			"必須キーがありません: key"},
		{"missing multiple required", "flag=x", []Field{{Name: "key", Required: true}, {Name: "op", Required: true}, {Name: "flag"}},
			"必須キーがありません: key, op"},
		{"empty spec", "key=x", nil, "未対応のキーです。指定可能なキー: なし"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input, test.spec)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestParseRejectsDuplicateFieldSpec(t *testing.T) {
	_, err := Parse("key=x", []Field{{Name: "key"}, {Name: "key"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "仕様が重複しています")
}

// TestParseErrorsDoNotEchoInput は、エラーメッセージに入力の値を含まないことを確認します。
// 入力に秘密情報が含まれていても表示されないことを、構文エラー・未対応キー・重複の
// 各経路で検証します。
func TestParseErrorsDoNotEchoInput(t *testing.T) {
	spec := []Field{{Name: "key", Required: true}}
	for _, input := range []string{
		"secret-value", "=secret-value", "key=secret-value,", "key=",
		"password=secret-value", "key=secret-value,key=secret-value",
		"unknown=secret-value", "key=secret-value,unknown=secret-value",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := Parse(input, spec)
			require.Error(t, err)
			require.False(t, strings.Contains(err.Error(), "secret-value"),
				"error %q must not echo the input value", err.Error())
		})
	}
}
