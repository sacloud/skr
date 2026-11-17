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
	"encoding/json"
	"fmt"

	"github.com/alecthomas/kong"
)

func validateOutput(ctx *kong.Context) error {
	flag, set, err := queryFlagFromContext(ctx)
	if err != nil {
		return err
	}
	if set {
		_, err := compileQuery(flag)
		return err
	}
	return nil
}

func writeOutput(ctx *kong.Context, value any) error {
	if flag, set, err := queryFlagFromContext(ctx); err != nil {
		return err
	} else if set {
		return writeQueryOutput(ctx.Stdout, flag, value)
	}
	encoder := json.NewEncoder(ctx.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// rejectQuery は JSON 以外の出力を持つコマンド向けに、--query と --query-raw の指定をエラーにします。
// 指定を暗黙に無視すると利用者の意図と異なる出力になるため、対応しないことを明示します。
func rejectQuery(ctx *kong.Context) error {
	if _, set, err := queryFlagFromContext(ctx); err != nil {
		return err
	} else if set {
		return fmt.Errorf("このコマンドの出力は JSON ではないため --query と --query-raw には対応しません")
	}
	return nil
}
