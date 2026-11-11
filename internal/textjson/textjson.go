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

// Package textjson は、skr 全体で統一された text → JSON 簡易表記
// (カンマ区切りの key=value リスト) の解析を提供します (ADR 0078)。
//
// 反復可能なオプションの 1 回の指定を 1 つの構造 (配列の 1 要素など) に対応させ、
// カンマ区切りの key=value 要素として解析します。各オプションは Field でキーの
// スキーマ (必須、反復可否) を定義し、解析結果の Fields を API リクエストの構造へ
// 変換します。値は字義どおり扱い、空白の補正や引用符・エスケープの解釈は行いません。
// エラーメッセージには入力の値を含めません。
package textjson

import (
	"fmt"
	"strings"
)

// Field は簡易表記の 1 キー (key=value の key) の仕様です。
type Field struct {
	// Name は要素の左辺に書くキー名です。
	Name string
	// Required は 1 回の指定の中で 1 回以上の指定を要求するかどうかです。
	Required bool
	// Repeated は同一指定内での反復を許可するかどうかです。許可しないキーの重複はエラーになります。
	Repeated bool
}

// Fields は 1 回の指定を解析した結果です。キー名から値を参照できます。
type Fields struct {
	values map[string][]string
}

// Get はキーの最初の値を返します。指定されていない場合は空文字列を返します。
func (f Fields) Get(key string) string {
	values := f.values[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// GetValues はキーの値を出現順に返します。指定されていない場合は nil を返します。
func (f Fields) GetValues(key string) []string {
	return f.values[key]
}

// Has はキーが指定されているかどうかを返します。
func (f Fields) Has(key string) bool {
	return len(f.values[key]) > 0
}

// Parse は text をカンマ区切りの key=value 要素に分解し、spec に照らして検証します。
// 各要素は最初の "=" でキーと値に分け、後続の "=" は値の一部として保持します。
// 未対応のキー、必須キーの欠落、反復を許可しないキーの重複、空の要素や値は
// エラーになります。値の前後の空白は補正せず、キーの重複は上書きしません。
func Parse(text string, spec []Field) (Fields, error) {
	allowed := make(map[string]Field, len(spec))
	for _, field := range spec {
		if _, exists := allowed[field.Name]; exists {
			return Fields{}, fmt.Errorf("キー %q の仕様が重複しています", field.Name)
		}
		allowed[field.Name] = field
	}
	values := make(map[string][]string, len(spec))
	for _, element := range strings.Split(text, ",") {
		key, value, ok := strings.Cut(element, "=")
		if !ok || key == "" || value == "" {
			return Fields{}, fmt.Errorf("各要素は空でない key=value 形式で指定してください")
		}
		field, known := allowed[key]
		if !known {
			return Fields{}, fmt.Errorf("未対応のキーです。指定可能なキー: %s", specNames(spec))
		}
		if !field.Repeated {
			if _, exists := values[key]; exists {
				return Fields{}, fmt.Errorf("%s は重複して指定できません", key)
			}
		}
		values[key] = append(values[key], value)
	}
	var missing []string
	for _, field := range spec {
		if field.Required && len(values[field.Name]) == 0 {
			missing = append(missing, field.Name)
		}
	}
	if len(missing) > 0 {
		return Fields{}, fmt.Errorf("必須キーがありません: %s", strings.Join(missing, ", "))
	}
	return Fields{values: values}, nil
}

// specNames はスキーマのキー名をカンマ区切りで返します。スキーマが空の場合は「なし」を返します。
func specNames(spec []Field) string {
	if len(spec) == 0 {
		return "なし"
	}
	names := make([]string, 0, len(spec))
	for _, field := range spec {
		names = append(names, field.Name)
	}
	return strings.Join(names, ", ")
}
