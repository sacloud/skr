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

package tutorials

import (
	"encoding/json"
	"fmt"
	"strings"
)

// compareOutput は期待出力と実際の出力を比較します。
//
// 期待出力と実際の出力の両方が JSON として解釈できる場合は構造比較を行い、
// それ以外の場合はコマンドの終了コードによる検証のみを意味します (nil を返す)。
// チュートリアルの出力例はマスクされた値を含むため、スカラー値は型のみを比較します。
func compareOutput(expected, actual string) error {
	expectedJSON, ok := decodeJSON(expected)
	if !ok {
		// 期待出力が JSON でない (プレーンテキストの例) 場合は照合しない。
		return nil
	}
	actualJSON, ok := decodeJSON(actual)
	if !ok {
		return fmt.Errorf("actual output is not JSON:\n%s", actual)
	}
	if err := compareValue(expectedJSON, actualJSON, "$"); err != nil {
		return fmt.Errorf("output mismatch: %w\nactual:\n%s", err, actual)
	}
	return nil
}

// decodeJSON はテキストを JSON 値として解釈します。
func decodeJSON(text string) (any, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, false
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, false
	}
	return v, true
}

// compareValue は JSON 値を再帰的に比較します。パスはエラー報告用です。
func compareValue(expected, actual any, path string) error {
	switch exp := expected.(type) {
	case map[string]any:
		act, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expect object, got %s", path, jsonTypeName(actual))
		}
		// キーの集合が一致することを要求します。期待にないキーも不一致とします。
		if len(exp) != len(act) {
			return fmt.Errorf("%s: object key count: expect %d (%v), got %d (%v)",
				path, len(exp), sortedKeys(exp), len(act), sortedKeys(act))
		}
		for k, ev := range exp {
			av, ok := act[k]
			if !ok {
				return fmt.Errorf("%s: missing key %q", path, k)
			}
			if err := compareValue(ev, av, path+"."+k); err != nil {
				return err
			}
		}
		return nil
	case []any:
		act, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("%s: expect array, got %s", path, jsonTypeName(actual))
		}
		if len(exp) != len(act) {
			return fmt.Errorf("%s: array length: expect %d, got %d", path, len(exp), len(act))
		}
		for i := range exp {
			if err := compareValue(exp[i], act[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case string:
		if _, ok := actual.(string); !ok {
			return fmt.Errorf("%s: expect string, got %s", path, jsonTypeName(actual))
		}
		return nil
	case float64:
		if _, ok := actual.(float64); !ok {
			return fmt.Errorf("%s: expect number, got %s", path, jsonTypeName(actual))
		}
		return nil
	case bool:
		if _, ok := actual.(bool); !ok {
			return fmt.Errorf("%s: expect bool, got %s", path, jsonTypeName(actual))
		}
		return nil
	case nil:
		if actual != nil {
			return fmt.Errorf("%s: expect null, got %s", path, jsonTypeName(actual))
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported expected type %T", path, expected)
	}
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// 報告の見やすさのためソートする。
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
