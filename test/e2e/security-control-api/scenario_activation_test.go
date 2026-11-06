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

package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioActivation は security-control-api activation コマンドのライブ E2E です。
// 現在の有効化状態を読み取り、読み取った値をそのまま送り返す同一値の往復で update を
// 確認します。create は未アクティベートのプロジェクトでのみ成功するため、既にアクティベート
// 済みのこのプロジェクトでは 409 で拒否されることを確認します (正常系の検証は未アクティベートの
// プロジェクトを要するため追跡事項)。状態は変更しないため、課金対象リソースは作成しません。
func TestScenarioActivation(t *testing.T) {
	h := helper.New(t, "security-control-api-activation",
		"セキュリティコントロールの有効化状態の読み取り・更新が正しく動作し、既にアクティベート済みのプロジェクトでの create が 409 で拒否されることを確認する")

	// 現在の有効化状態を読み取る
	current := readActivation(t, h, "現在の有効化状態を読み取る")
	require.NotEmpty(t, current.ServicePrincipalId, "activation should have a service principal ID: %+v", current)
	require.True(t, current.IsActive, "this scenario expects an activated project and refuses to change the activation state: %+v", current)
	sp := current.ServicePrincipalId

	// 読み取った値をそのまま送り返す同一値の往復で update を確認する
	updated := updateActivation(t, h, sp, true, "読み取った値をそのまま送り返す同一値の往復で update を確認する")
	require.Equal(t, sp, updated.ServicePrincipalId, "same-value update should keep the service principal: %+v", updated)
	require.True(t, updated.IsActive, "same-value update should keep the activation active: %+v", updated)
	require.Equal(t, current.AutomatedActionLimit, updated.AutomatedActionLimit, "same-value update should keep the automated action limit: %+v", updated)

	// create は未アクティベートのプロジェクトでのみ成功するため、既にアクティベート済みの
	// このプロジェクトでは 409 で拒否されることを確認する
	_, stderr, err := h.CallAllowingFailure(t, "アクティベート済みのプロジェクトでの create が拒否されることを確認する",
		"security-control-api", "activation", "create",
		"--service-principal-id", sp,
	)
	require.Error(t, err, "create on an activated project should fail: %s", stderr)
	require.True(t, activationConflict(stderr), "create on an activated project should be rejected as a conflict: %s", stderr)

	// create の拒否後も状態が変わっていないことを確認する
	after := readActivation(t, h, "create の拒否後も状態が変わっていないことを確認する")
	require.True(t, after.IsActive, "activation should remain active after the rejected create: %+v", after)
	require.Equal(t, sp, after.ServicePrincipalId, "service principal should not change: %+v", after)
}

// activationOutput は security-control-api activation コマンドの JSON 出力です。
type activationOutput struct {
	ServicePrincipalId   string `json:"servicePrincipalId"`
	IsActive             bool   `json:"isActive"`
	AutomatedActionLimit int    `json:"automatedActionLimit"`
}

// readActivation は skr CLI で現在の有効化状態を読み取ります。
func readActivation(t *testing.T, h *helper.Helper, description string) activationOutput {
	t.Helper()

	data := h.Call(t, description, "security-control-api", "activation", "read")
	var out activationOutput
	require.NoError(t, json.Unmarshal(data, &out), "decode activation read response: %s", string(data))
	return out
}

// updateActivation は skr CLI でサービスプリンシパルと有効状態を更新します。
// API の更新リクエストは両方のフィールドを送信するため、--service-principal-id と
// --is-active の両方を指定します。
func updateActivation(t *testing.T, h *helper.Helper, sp string, active bool, description string) activationOutput {
	t.Helper()

	data := h.Call(t, description,
		"security-control-api", "activation", "update",
		"--service-principal-id", sp,
		"--is-active="+strconv.FormatBool(active),
	)
	var out activationOutput
	require.NoError(t, json.Unmarshal(data, &out), "decode activation update response: %s", string(data))
	return out
}

// activationConflict はコマンドの失敗がアクティベート済みプロジェクトへの create 拒否の
// API 409 かどうかを判定します。
func activationConflict(stderr string) bool {
	return strings.HasPrefix(strings.TrimSpace(stderr), "security-control: Activation.Create: API Error 409:")
}
