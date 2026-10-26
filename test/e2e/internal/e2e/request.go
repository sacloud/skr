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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteRequest は value を JSON エンコードして dir/name.json に書き込み、
// ファイルパスを返します。作成されるファイルのパーミッションは 0o600 です。
func WriteRequest(dir, name string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", name, err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write %s request: %w", name, err)
	}
	return path, nil
}
