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

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/itchyny/gojq"
)

// queryFlag はセットされたクエリ系フラグ (--query / --query-raw) の値を表します。
// name はエラーメッセージに使うフラグ名で、raw は --query-raw の指定で
// 文字列の結果を引用符なしで出力することを表します。
type queryFlag struct {
	expression string
	raw        bool
	name       string
}

// queryFlagFromContext はセットされた --query / --query-raw の値を集めます。
// どちらもセットされていなければ set は false です。同時指定は kong の xor タグで
// 拒否しますが、タグの変更に備えてここでも拒否します。
func queryFlagFromContext(ctx *kong.Context) (*queryFlag, bool, error) {
	var query, queryRaw *string
	for _, flag := range ctx.Flags() {
		if !flag.Set {
			continue
		}
		var value *string
		if v, ok := ctx.FlagValue(flag).(*string); ok {
			value = v
		}
		switch flag.Name {
		case "query", "query-raw":
			if value == nil {
				return nil, false, fmt.Errorf("read --%s value: unexpected value type", flag.Name)
			}
			if flag.Name == "query" {
				query = value
			} else {
				queryRaw = value
			}
		}
	}
	switch {
	case query != nil && queryRaw != nil:
		return nil, false, fmt.Errorf("--query と --query-raw は同時に指定できません")
	case query != nil:
		return &queryFlag{expression: *query, name: "query"}, true, nil
	case queryRaw != nil:
		return &queryFlag{expression: *queryRaw, raw: true, name: "query-raw"}, true, nil
	default:
		return nil, false, nil
	}
}

func compileQuery(flag *queryFlag) (*gojq.Code, error) {
	query, err := gojq.Parse(flag.expression)
	if err != nil {
		return nil, fmt.Errorf("parse --%s expression: %w", flag.name, err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("compile --%s expression: %w", flag.name, err)
	}
	return code, nil
}

func writeQueryOutput(writer io.Writer, flag *queryFlag, value any) error {
	code, err := compileQuery(flag)
	if err != nil {
		return err
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode API output for --%s: %w", flag.name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var input any
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("decode API output for --%s: %w", flag.name, err)
	}

	var output bytes.Buffer
	iter := code.Run(input)
	for {
		result, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := result.(error); ok {
			if halt, ok := err.(*gojq.HaltError); ok && halt.Value() == nil {
				break
			}
			return fmt.Errorf("evaluate --%s expression: %w", flag.name, err)
		}
		if flag.raw {
			// --query-raw は jq -r に準拠し、文字列の結果だけ引用符なしで出力します。
			// 文字列以外は --query と同じ JSON 出力になります。
			if s, ok := result.(string); ok {
				output.WriteString(s)
				output.WriteByte('\n')
				continue
			}
		}
		resultJSON, err := gojq.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode --%s result: %w", flag.name, err)
		}
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, resultJSON, "", "  "); err != nil {
			return fmt.Errorf("format --%s result: %w", flag.name, err)
		}
		output.Write(formatted.Bytes())
		output.WriteByte('\n')
	}
	if _, err := writer.Write(output.Bytes()); err != nil {
		return fmt.Errorf("write --%s output: %w", flag.name, err)
	}
	return nil
}
