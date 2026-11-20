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

// コマンド層の単体テストです。kong の解析、Runtime 注入、service 層の結果を
// 出力処理へ渡す経路を検証します。service 層の検証は plans_test.go、CLI 全体の
// 接続と出力は internal/cli のテストで行います。
package nfs

import (
	"errors"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// commandHarness はコマンド層のテストで、kong の解析と実行、出力の記録を行います。
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

// runError はエラーになることを確認し、エラーを返します。引数は skr CLI 全体と
// 同じ形式 (先頭に "nfs") で受け取り、ルート分を除いて解析します。
func (h *commandHarness) runError(t *testing.T, factory ServiceFactory, args ...string) error {
	t.Helper()
	commands := NewCommands(h.runtime(), factory)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	ctx, err := parser.Parse(args[1:])
	require.NoError(t, err)
	return ctx.Run()
}

func testService(t *testing.T) *Service {
	t.Helper()
	return NewService(&fakeNoteFinder{notes: []*iaas.Note{
		planNote(113100243383, "tk1a",
			planContent(`[{"size":20,"availability":"available","planId":20,"cloneable":true}]`, `[]`),
			types.Availabilities.Available),
	}})
}

func TestPlansCommand(t *testing.T) {
	h := newCommandHarness()
	svc := testService(t)
	err := h.runError(t, func() (*Service, error) { return svc, nil }, "nfs", "plans", "--zone", "tk1a")
	require.NoError(t, err)
	require.Len(t, h.outputs, 1)
	plans, ok := h.outputs[0].([]PlanOutput)
	require.True(t, ok)
	require.Len(t, plans, 1)
	require.Equal(t, "SSD", plans[0].Storage)
}

func TestPlansCommandWithoutFactory(t *testing.T) {
	h := newCommandHarness()
	err := h.runError(t, nil, "nfs", "plans", "--zone", "tk1a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "API クライアントが設定されていません")

	// 出力検証がエラーのときは API を呼び出しません。
	// factory が nil のため API を呼び出せないことも、エラーが出力検証側で
	// あることで確認します。
	h.validateOutput = func(*kong.Context) error { return errors.New("出力検証エラー") }
	err = h.runError(t, nil, "nfs", "plans", "--zone", "tk1a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "出力検証エラー")
}

func TestPlansCommandZoneRequired(t *testing.T) {
	commands := NewCommands(newCommandHarness().runtime(), nil)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"plans"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--zone")
}

func TestPlansCommandServiceError(t *testing.T) {
	h := newCommandHarness()
	svc := NewService(&fakeNoteFinder{notes: nil})
	err := h.runError(t, func() (*Service, error) { return svc, nil }, "nfs", "plans", "--zone", "tk9x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NFS プラン情報が見つかりません")
	require.Empty(t, h.outputs)
}
