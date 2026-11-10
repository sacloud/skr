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

// コマンド層の単体テストです。service 層の検証 (wire 形式、マージ、検証) は
// service/eventbus-service-go のテスト、コマンド全体の挙動は internal/cli の
// sakumock テストで行うため、ここでは CLI 固有の処理だけを検証します。
package eventbus

import (
	"bytes"
	"errors"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"

	eventbusapi "github.com/sacloud/skr/service/eventbus-service-go"
)

func ptr[T any](value T) *T { return &value }

// commandHarness は高レベルコマンドのテストで、kong の解析と実行、
// 出力の記録を行います。
type commandHarness struct {
	outputs        []any
	validateOutput func(*kong.Context) error
}

func newCommandHarness() *commandHarness {
	return &commandHarness{}
}

func (h *commandHarness) runtime() Runtime {
	validateOutput := h.validateOutput
	if validateOutput == nil {
		validateOutput = func(*kong.Context) error { return nil }
	}
	return Runtime{
		ValidateOutput: validateOutput,
		WriteOutput:    func(_ *kong.Context, value any) error { h.outputs = append(h.outputs, value); return nil },
	}
}

// runError はエラーになることを確認し、エラーを返します。引数は skr CLI 全体と同じ
// 形式 (先頭に "eventbus") で受け取り、ルート分を除いて解析します。
// 正常系の実行は service/eventbus-service-go のテストと internal/cli の sakumock
// テストで検証します。
func (h *commandHarness) runError(t *testing.T, factory ServiceFactory, args ...string) error {
	t.Helper()
	commands := NewCommands(h.runtime(), factory, nil)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	ctx, err := parser.Parse(args[1:])
	require.NoError(t, err)
	return ctx.Run()
}

func TestCommandsWithoutFactory(t *testing.T) {
	h := newCommandHarness()
	err := h.runError(t, nil, "eventbus", "process-configuration", "list")
	require.Error(t, err)
	require.Contains(t, err.Error(), "API クライアントが設定されていません")

	// 出力検証がエラーのときは API を呼び出しません。
	// factory が nil のため API を呼び出せないことも、エラーが出力検証側で
	// あることで確認します。
	h.validateOutput = func(*kong.Context) error { return errors.New("出力検証エラー") }
	err = h.runError(t, nil, "eventbus", "process-configuration", "list")
	require.Error(t, err)
	require.Contains(t, err.Error(), "出力検証エラー")
}

func TestRuntimeWithoutWriteOutput(t *testing.T) {
	var runtime Runtime
	err := runtime.writeOutput(nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API 出力処理が設定されていません")

	err = runtime.validateOutput(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API 出力処理が設定されていません")
}

func TestCommandContextCancels(t *testing.T) {
	ctx, cancel := commandContext()
	require.NoError(t, ctx.Err())
	cancel()
	require.Error(t, ctx.Err())
}

func TestParseStartsAtHelpers(t *testing.T) {
	got, err := parseStartsAt("2030-01-10T09:00:00+09:00")
	require.NoError(t, err)
	require.EqualValues(t, 1894233600000, got.UnixMilli())

	optional, err := parseOptionalStartsAt(nil)
	require.NoError(t, err)
	require.Nil(t, optional)

	_, err = parseOptionalStartsAt(ptr("invalid"))
	require.Error(t, err)

	_, err = parseStartsAt("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "RFC3339")
}

func TestWriteCreateUpdateOutput(t *testing.T) {
	var runtime Runtime

	// item が nil の場合は ID を含めません。
	err := writeCreateUpdateOutput[*eventbusapi.ProcessConfigurationOutput](runtime, nil, nil, nil, bytes.ErrTooLarge)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "ID:")

	// item がある場合は変更の成功と ID を伝えます。
	item := &v1.CommonServiceItem{ID: "123"}
	err = writeCreateUpdateOutput[*eventbusapi.ProcessConfigurationOutput](runtime, nil, item, nil, bytes.ErrTooLarge)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API 操作は成功しましたが")
	require.Contains(t, err.Error(), "ID: 123")
}
