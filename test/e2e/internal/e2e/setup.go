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
	"fmt"
	"os"
	"path/filepath"
)

// ValidateExecutable は path が実行可能なファイルであることを確認し、絶対パスを返します。
func ValidateExecutable(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat skr binary: %w", err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("skr must be an executable built binary")
	}
	return abs, nil
}

// TempDir はプライベートな一時ディレクトリを作成し、後片付け用の関数を返します。
// pattern は os.MkdirTemp の命名パターンです。
func TempDir(pattern string) (string, func() error, error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create private temporary directory: %w", err)
	}
	return dir, func() error { return os.RemoveAll(dir) }, nil
}
