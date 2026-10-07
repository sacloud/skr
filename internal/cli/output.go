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

	"github.com/alecthomas/kong"
)

func validateOutput(ctx *kong.Context) error {
	query, set, err := queryExpression(ctx)
	if err != nil {
		return err
	}
	if set {
		_, err := compileQuery(query)
		return err
	}
	return nil
}

func writeOutput(ctx *kong.Context, value any) error {
	if query, set, err := queryExpression(ctx); err != nil {
		return err
	} else if set {
		return writeQueryOutput(ctx.Stdout, query, value)
	}
	encoder := json.NewEncoder(ctx.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
