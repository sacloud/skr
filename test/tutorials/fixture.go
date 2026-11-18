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
	"os"
	"path/filepath"

	"sort"
	"strings"
)

// harnessSkipPrefixes は harness 固定の skip 対象コマンドです。
// SSH セッションは実行対象とせず、モックに接続できない CLI として扱います。
var harnessSkipPrefixes = []string{"ssh", "exit"}

// MockConfig はチュートリアル実行に使うモックへのシードです。
type MockConfig struct {
	// Zones は IaaS モックに登録するゾーン名のリストです。
	Zones []string `json:"zones,omitempty"`
}

// Fixture はチュートリアル 1 ファイル分の実行補助情報です。
// ファイルは test/tutorials/fixtures/<チュートリアル名>.json に置きます。
type Fixture struct {
	// Values は読者選択値のトークン置換表です。トークンは置換後の値に写像されます。
	Values map[string]string `json:"values,omitempty"`
	// Skip は実行を省略するコマンドの正規化プレフィックスです。
	Skip []string `json:"skip,omitempty"`
	// Stub は正規化コマンドテキストから置換後のコマンドテキストへの写像です。
	Stub map[string]string `json:"stub,omitempty"`
	// Mock はモックへのシードです。
	Mock MockConfig `json:"mock,omitempty"`

	// usedStub / usedSkip / usedValues は実行時に照合された項目の記録です。
	// 未使用の fixture 項目を fail-closed で検出するために使います。
	usedStub   map[string]bool
	usedSkip   map[string]bool
	usedValues map[string]bool
}

// loadFixture は fixture ファイルを読み込みます。ファイルがなければエラーにします。
func loadFixture(fixturesDir, tutorialPath string) (*Fixture, error) {
	name := fixtureName(tutorialPath)
	path := filepath.Join(fixturesDir, name)
	data, err := os.ReadFile(path) //nolint:gosec // fixtures ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return nil, fmt.Errorf("read fixture for %s: %w", tutorialPath, err)
	}
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	if err := fixture.validate(path); err != nil {
		return nil, err
	}
	return &fixture, nil
}

// fixtureName はチュートリアルのパスから fixture のファイル名を返します。
// ディレクトリを除いたベース名を使うため、同一ベース名のチュートリアルが
// 複数ある場合はエラーにする必要があります (detectFixtureNameConflicts)。
func fixtureName(tutorialPath string) string {
	return strings.TrimSuffix(filepath.Base(tutorialPath), ".md") + ".json"
}

// detectFixtureNameConflicts は fixture ファイル名が衝突するチュートリアルを検出します。
func detectFixtureNameConflicts(paths []string) error {
	seen := map[string][]string{}
	for _, p := range paths {
		name := fixtureName(p)
		seen[name] = append(seen[name], p)
	}
	for name, owners := range seen {
		if len(owners) > 1 {
			return fmt.Errorf("fixture name %s conflicts: %s", name, strings.Join(owners, ", "))
		}
	}
	return nil
}

// validate は fixture の内容を検証します。
func (f *Fixture) validate(path string) error {
	f.usedStub = map[string]bool{}
	f.usedSkip = map[string]bool{}
	f.usedValues = map[string]bool{}
	for token, value := range f.Values {
		if token == "" {
			return fmt.Errorf("fixture %s: values must not have empty token", path)
		}
		if strings.ContainsAny(token, " \t\n$") {
			return fmt.Errorf("fixture %s: value token %q must not contain whitespace or $", path, token)
		}
		if value == "" {
			return fmt.Errorf("fixture %s: value for token %s must not be empty", path, token)
		}
	}
	for _, s := range f.Skip {
		if normalizeCommand(s) == "" {
			return fmt.Errorf("fixture %s: skip entries must not be blank", path)
		}
	}
	for key, value := range f.Stub {
		if normalizeCommand(key) == "" {
			return fmt.Errorf("fixture %s: stub keys must not be blank", path)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("fixture %s: stub replacement for %q must not be blank", path, key)
		}
	}
	return nil
}

// normalizeCommand はコマンドテキストを fixture や skip の照合用に正規化します。
// 前後の空白を除去し、連続する空白を 1 個にまとめます。改行は空白に置き換えます。
func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

// substituteValues はコマンドテキスト内のトークンを fixture の値に置き換えます。
// トークンは前後が英数字・ハイフン・アンダースコアでない場合のみ置換します。
// 置換したトークンは使用済みとして記録します。
func (f *Fixture) substituteValues(command string) string {
	tokens := make([]string, 0, len(f.Values))
	for token := range f.Values {
		tokens = append(tokens, token)
	}
	// 長いトークンを先に置換する (例: QUEUE-NAME-2 の前に QUEUE-NAME が
	// 誤って部分一致しないようにする)。
	sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
	for _, token := range tokens {
		replaced := replaceToken(command, token, f.Values[token])
		if replaced != command {
			f.usedValues[token] = true
		}
		command = replaced
	}
	return command
}

// replaceToken は text 内の token を、前後がトークン構成文字 (英数字・ハイフン・
// アンダースコア) でない場合に限り value に置き換えます。
func replaceToken(text, token, value string) string {
	isTokenChar := func(b byte) bool {
		return b == '_' || b == '-' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], token) &&
			(i == 0 || !isTokenChar(text[i-1])) &&
			(i+len(token) == len(text) || !isTokenChar(text[i+len(token)])) {
			out.WriteString(value)
			i += len(token)
			continue
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

// shouldSkip はコマンドが skip 対象かどうかを返します。
// マッチした項目は使用済みとして記録します。
func (f *Fixture) shouldSkip(command string) bool {
	normalized := normalizeCommand(command)
	for _, prefix := range harnessSkipPrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") {
			return true
		}
	}
	for _, prefix := range f.Skip {
		p := normalizeCommand(prefix)
		if normalized == p || strings.HasPrefix(normalized, p+" ") {
			f.usedSkip[p] = true
			return true
		}
	}
	return false
}

// stubReplacement は正規化コマンドテキストが stub のキーと一致する場合、
// 置換後のコマンドテキストを返します。マッチした項目は使用済みとして記録します。
func (f *Fixture) stubReplacement(command string) (string, bool) {
	if len(f.Stub) == 0 {
		return "", false
	}
	normalized := normalizeCommand(command)
	if value, ok := f.Stub[normalized]; ok {
		f.usedStub[normalized] = true
		return value, true
	}
	return "", false
}

// checkUnused は使用されなかった fixture の項目をエラーにします。
// チュートリアルの編集で対応が外れた項目を fail-closed で検出するためのものです。
func (f *Fixture) checkUnused() error {
	var unused []string
	for token := range f.Values {
		if !f.usedValues[token] {
			unused = append(unused, "values: "+token)
		}
	}
	for _, skip := range f.Skip {
		if !f.usedSkip[normalizeCommand(skip)] {
			unused = append(unused, "skip: "+normalizeCommand(skip))
		}
	}
	for key := range f.Stub {
		if !f.usedStub[key] {
			unused = append(unused, "stub: "+key)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		return fmt.Errorf("unused fixture entries: %s", strings.Join(unused, "; "))
	}
	return nil
}
