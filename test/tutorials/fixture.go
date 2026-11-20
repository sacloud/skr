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

	"regexp"
	"slices"
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
	// Services は起動する Ogen 系モックサービスのリストです。
	// simplemq、eventbus、workflows をサポートします。eventbus は simplemq と
	// 一緒に起動する必要があります (サービスリンクの宛先を実 API に向けないため)。
	Services []string `json:"services,omitempty"`
	// Events はチュートリアル外の副作用として発生するイベントの宣言です。
	// after に一致する step が成功するたびに EventBus モックへ注入されます。
	Events []*MockEvent `json:"events,omitempty"`
}

// MockEvent は harness が EventBus モックへ注入するイベントです。
// 実世界ではリソース操作の副作用としてイベントログに記録され、EventBus が
// それを受け取ります。モックにはその経路がないため、fixture で宣言した
// 位置 (after) で注入してトリガーを発火させます。
type MockEvent struct {
	// After はイベントを注入するタイミングを示す、チュートリアルに書かれた
	// コマンドテキスト (正規化後) です。stub や skip と同じ照合方法を使います。
	After string `json:"after"`
	// Source と Type はイベントの送信元と種別です。
	Source string `json:"source"`
	Type   string `json:"type"`
	// Attributes はトリガー条件の照合に使うイベント属性です。値は
	// fixture の値で置換されます。
	Attributes map[string]string `json:"attributes,omitempty"`
}

// mockEventBody は EventBus モックのイベント注入エンドポイント
// (POST /_sakumock/events) のリクエスト本文です。
type mockEventBody struct {
	Source     string            `json:"Source"`
	Type       string            `json:"Type"`
	Attributes map[string]string `json:"Attributes"`
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

	// usedStub / usedSkip / usedValues / usedEvents は実行時に照合された項目の
	// 記録です。未使用の fixture 項目を fail-closed で検出するために使います。
	usedStub   map[string]bool
	usedSkip   map[string]bool
	usedValues map[string]bool
	usedEvents map[*MockEvent]bool
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
	f.usedEvents = map[*MockEvent]bool{}
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
	hasSimpleMQ := false
	hasEventBus := false
	seenServices := map[string]bool{}
	for _, service := range f.Mock.Services {
		if seenServices[service] {
			return fmt.Errorf("fixture %s: mock.services declares %q more than once", path, service)
		}
		seenServices[service] = true
		switch service {
		case "simplemq":
			hasSimpleMQ = true
		case "eventbus":
			hasEventBus = true
		case "workflows":
		case "monitoringsuite":
		default:
			return fmt.Errorf("fixture %s: unknown mock service %q", path, service)
		}
	}
	// EventBus モックのサービスリンク (トリガー発火時のメッセージ送信) は
	// SimpleMQ モックの URL を必要とします。宣言がない場合はリンク先が
	// 実 API になるため、一緒に宣言することを要求します。
	if hasEventBus && !hasSimpleMQ {
		return fmt.Errorf("fixture %s: the eventbus mock service requires simplemq to be declared too", path)
	}
	if len(f.Mock.Events) > 0 && !hasEventBus {
		return fmt.Errorf("fixture %s: mock.events requires the eventbus mock service", path)
	}
	for _, event := range f.Mock.Events {
		if normalizeCommand(event.After) == "" {
			return fmt.Errorf("fixture %s: mock.events entries must not have blank after", path)
		}
		if event.Source == "" {
			return fmt.Errorf("fixture %s: mock.events entries must not have empty source", path)
		}
		if event.Type == "" {
			return fmt.Errorf("fixture %s: mock.events entries must not have empty type", path)
		}
	}
	return nil
}

// normalizeCommand はコマンドテキストを fixture や skip の照合用に正規化します。
// 前後の空白を除去し、連続する空白を 1 個にまとめます。改行は空白に置き換えます。
func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

// capsWordPattern は「大文字・数字・ハイフンのみで構成される語」のパターンです。
// チュートリアルのプレースホルダー (例: QUEUE-NAME、UNIQUE-SUFFIX) は
// この形で書かれるという規約に基づきます。
var capsWordPattern = regexp.MustCompile(`[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*`)

// substituteValues はコマンドテキスト内のトークンを fixture の値に置き換えます。
// トークンは前後が英数字・アンダースコアでない場合のみ置換します。ハイフンは
// 境界として扱うため、eventbus-switch-UNIQUE-SUFFIX のような名前の一部でも
// 置換されます。置換したトークンは使用済みとして記録します。
// 値を定義したトークンが、それ自身の値を持たないより長いプレースホルダー語の
// 一部としてしか一致しない場合は、置換結果が意図しない名前になるためエラーにします。
func (f *Fixture) substituteValues(command string) (string, error) {
	for _, word := range capsWordPattern.FindAllString(command, -1) {
		if _, ok := f.Values[word]; ok {
			continue
		}
		for token := range f.Values {
			if tokenEmbeddedIn(word, token) {
				return "", fmt.Errorf("value token %s only matches part of placeholder %s: define a value for %s or use distinct tokens", token, word, word)
			}
		}
	}
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
	return command, nil
}

// tokenEmbeddedIn は token が word のハイフン区切り部分列として含まれるかを
// 返します。word と token が同じ語の場合は部分ではないため false です。
func tokenEmbeddedIn(word, token string) bool {
	if word == token {
		return false
	}
	ws := strings.Split(word, "-")
	ts := strings.Split(token, "-")
	for i := 0; i+len(ts) <= len(ws); i++ {
		if slices.Equal(ws[i:i+len(ts)], ts) {
			return true
		}
	}
	return false
}

// replaceToken は text 内の token を、前後がトークン構成文字 (英数字・
// アンダースコア) でない場合に限り value に置き換えます。ハイフンは境界として
// 扱いますが、アンダースコアは構成文字のため ZONE は ZONE_ID の一部としては
// 置換されません。
func replaceToken(text, token, value string) string {
	isTokenChar := func(b byte) bool {
		return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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

// substituteEvent はイベント注入の本文を fixture の値で置換します。
func (f *Fixture) substituteEvent(ev *MockEvent) (mockEventBody, error) {
	body := mockEventBody{Attributes: map[string]string{}}
	var err error
	if body.Source, err = f.substituteValues(ev.Source); err != nil {
		return body, err
	}
	if body.Type, err = f.substituteValues(ev.Type); err != nil {
		return body, err
	}
	for key, value := range ev.Attributes {
		v, err := f.substituteValues(value)
		if err != nil {
			return body, fmt.Errorf("event attribute %s: %w", key, err)
		}
		body.Attributes[key] = v
	}
	return body, nil
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
	for _, event := range f.Mock.Events {
		if !f.usedEvents[event] {
			unused = append(unused, "event: "+normalizeCommand(event.After))
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		return fmt.Errorf("unused fixture entries: %s", strings.Join(unused, "; "))
	}
	return nil
}
