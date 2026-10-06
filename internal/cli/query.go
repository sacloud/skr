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

func queryExpression(ctx *kong.Context) (string, bool, error) {
	for _, flag := range ctx.Flags() {
		if flag.Name != "query" || !flag.Set {
			continue
		}
		value, ok := ctx.FlagValue(flag).(*string)
		if !ok || value == nil {
			return "", false, fmt.Errorf("read --query value: unexpected value type")
		}
		return *value, true, nil
	}
	return "", false, nil
}

func compileQuery(expression string) (*gojq.Code, error) {
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("parse --query expression: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("compile --query expression: %w", err)
	}
	return code, nil
}

func writeQueryOutput(writer io.Writer, expression string, value any) error {
	code, err := compileQuery(expression)
	if err != nil {
		return err
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode API output for --query: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var input any
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("decode API output for --query: %w", err)
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
			return fmt.Errorf("evaluate --query expression: %w", err)
		}
		resultJSON, err := gojq.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode --query result: %w", err)
		}
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, resultJSON, "", "  "); err != nil {
			return fmt.Errorf("format --query result: %w", err)
		}
		output.Write(formatted.Bytes())
		output.WriteByte('\n')
	}
	if _, err := writer.Write(output.Bytes()); err != nil {
		return fmt.Errorf("write --query output: %w", err)
	}
	return nil
}
