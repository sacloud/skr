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

package apprunapi

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func DecodeRequest(input string, destination any) error {
	var data []byte
	switch {
	case input == "-":
		var err error
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read request from standard input: %w", err)
		}
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		if path == "" {
			return fmt.Errorf("request file path is empty")
		}
		var err error
		data, err = os.ReadFile(path) // #nosec G304 -- リクエストファイルのパスはユーザーが明示的に指定する。
		if err != nil {
			return fmt.Errorf("read request file: %w", err)
		}
	default:
		data = []byte(input)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}
