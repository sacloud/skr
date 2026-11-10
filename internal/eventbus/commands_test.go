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

package eventbus

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/stretchr/testify/require"
)

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

// run は引数を解析して実行します。factory には fake API を包んだ Service を返す関数を渡します。
// 引数は skr CLI 全体と同じ形式 (先頭に "eventbus") で受け取り、ルート分を除いて解析します。
func (h *commandHarness) run(t *testing.T, factory ServiceFactory, args ...string) {
	t.Helper()
	commands := NewCommands(h.runtime(), factory, nil)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	ctx, err := parser.Parse(args[1:])
	require.NoError(t, err)
	require.NoError(t, ctx.Run())
}

// runError はエラーになることを確認し、エラーを返します。
func (h *commandHarness) runError(t *testing.T, factory ServiceFactory, args ...string) error {
	t.Helper()
	commands := NewCommands(h.runtime(), factory, nil)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	ctx, err := parser.Parse(args[1:])
	require.NoError(t, err)
	return ctx.Run()
}

// newFakeService は 1 つの fakeAPI を 3 種別の API として使う Service を返します。
func newFakeService(api *fakeAPI) *Service {
	return &Service{process: api, schedule: api, trigger: api}
}

func TestProcessCreateCommandSendsWireRequest(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "create",
		"--name", "wire-process",
		"--description", "説明",
		"--destination", "simplemq",
		"--queue-name", "queue-1",
		"--content", "content-1",
	)

	data, err := json.Marshal(&api.createRequest)
	require.NoError(t, err)
	wire := string(data)
	require.Contains(t, wire, `"Name":"wire-process"`)
	require.Contains(t, wire, `"Destination":"simplemq"`)
	// Parameters は JSON 文字列として送られます。
	require.Contains(t, wire, `\"queue_name\":\"queue-1\"`)
	require.Contains(t, wire, `\"content\":\"content-1\"`)
	// Provider.Class は SDK の操作層が設定するため、fake を使うこのテストでは検証しません
	// (service_sdk_test.go と CLI レベルの sakumock テストで検証します)。

	require.Len(t, h.outputs, 1)
	output, ok := h.outputs[0].(*ProcessConfigurationOutput)
	require.True(t, ok, "output type = %T", h.outputs[0])
	require.Equal(t, "123", output.ID)
}

func TestProcessCreateCommandRejectsCrossDestinationFields(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	h := newCommandHarness()
	err := h.runError(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "create",
		"--name", "wire-process",
		"--destination", "simplemq",
		"--group-id", "group-1",
	)
	require.Error(t, err)
	require.Zero(t, api.mutations)
}

func TestProcessUpdateCommandNameOnlySkipsSettings(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "update", "123", "--name", "renamed",
	)

	data, err := json.Marshal(&api.updateRequest)
	require.NoError(t, err)
	require.Contains(t, string(data), `"Name":"renamed"`)
	require.NotContains(t, string(data), "Settings")
}

func TestProcessUpdateCommandChangesDestination(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "update", "123",
		"--destination", "autoscale", "--action", "scale_up", "--resource-id", "resource-1",
	)

	data, err := json.Marshal(&api.updateRequest)
	require.NoError(t, err)
	wire := string(data)
	require.Contains(t, wire, `"Destination":"autoscale"`)
	// Parameters は JSON 文字列として送られます。
	require.Contains(t, wire, `\"action\":\"scale_up\"`)
	require.Contains(t, wire, `\"resource_id\":\"resource-1\"`)
	// 旧宛先のパラメーターは引き継ぎません。
	require.NotContains(t, wire, "queue_name")
}

func TestProcessListCommandProjectsItems(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "list",
	)

	require.Len(t, h.outputs, 1)
	projected, ok := h.outputs[0].([]*ProcessConfigurationOutput)
	require.True(t, ok, "output type = %T", h.outputs[0])
	require.Len(t, projected, 1)
	require.Equal(t, "123", projected[0].ID)
}

func TestScheduleCreateCommandConvertsStartsAt(t *testing.T) {
	api := &fakeAPI{item: scheduleItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "schedule", "create",
		"--name", "wire-schedule",
		"--process-configuration-id", "456",
		"--starts-at", "2030-01-10T09:00:00+09:00",
		"--recurring-step", "5",
		"--recurring-unit", "hour",
	)

	data, err := json.Marshal(&api.createRequest)
	require.NoError(t, err)
	require.Contains(t, string(data), `"StartsAt":1894233600000`)

	require.Len(t, h.outputs, 1)
	output, ok := h.outputs[0].(*ScheduleOutput)
	require.True(t, ok, "output type = %T", h.outputs[0])
	// fake API が返す item の StartsAt を射影します。
	require.Equal(t, "2030-03-17T17:46:40.123Z", output.StartsAt)
}

func TestScheduleCreateCommandRejectsInvalidStartsAt(t *testing.T) {
	api := &fakeAPI{item: scheduleItem()}
	h := newCommandHarness()
	err := h.runError(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "schedule", "create",
		"--name", "wire-schedule",
		"--process-configuration-id", "456",
		"--starts-at", "2030/01/10",
		"--recurring-step", "5",
		"--recurring-unit", "hour",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "RFC3339")
	require.Zero(t, api.mutations)
}

func TestScheduleUpdateCommandSwitchesToCrontab(t *testing.T) {
	api := &fakeAPI{item: scheduleItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "schedule", "update", "123", "--crontab", "0 9 * * *",
	)

	settings := api.updateRequest.CommonServiceItem.Settings.Value.ScheduleSettings
	require.Equal(t, "0 9 * * *", settings.Crontab.Value)
	require.False(t, settings.RecurringStep.IsSet())
	require.False(t, settings.RecurringUnit.IsSet())
}

func TestTriggerCreateCommandSendsRepeatedFlags(t *testing.T) {
	api := &fakeAPI{item: triggerItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "trigger", "create",
		"--name", "wire-trigger",
		"--source", "//eventbus.sakura.ad.jp/eventlog",
		"--type", "zone.updated",
		"--type", "billing,updated",
		"--condition", "key=zone,op=eq,value=is1b",
		"--condition", "key=host,op=in,value=www1,value=www2",
		"--process-configuration-id", "456",
	)

	data, err := json.Marshal(&api.createRequest)
	require.NoError(t, err)
	wire := string(data)
	// カンマを含む値は分割されません。
	require.Contains(t, wire, `"Types":["zone.updated","billing,updated"]`)
	require.Contains(t, wire, `"Op":"eq"`)
	require.Contains(t, wire, `"Op":"in"`)
	require.Contains(t, wire, `"Values":["www1","www2"]`)
}

func TestTriggerUpdateCommandNoFlagsSkipsSettings(t *testing.T) {
	api := &fakeAPI{item: triggerItem()}
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "trigger", "update", "123",
	)

	require.False(t, api.updateRequest.CommonServiceItem.Settings.IsSet())
}

func TestTriggerCreateCommandRejectsInvalidCondition(t *testing.T) {
	api := &fakeAPI{item: triggerItem()}
	h := newCommandHarness()
	err := h.runError(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "trigger", "create",
		"--name", "wire-trigger",
		"--source", "//eventbus.sakura.ad.jp/eventlog",
		"--condition", "key=zone,op=eq",
		"--process-configuration-id", "456",
	)
	require.Error(t, err)
	require.Zero(t, api.mutations)
}

func TestDeleteCommands(t *testing.T) {
	h := newCommandHarness()
	h.run(t, func() (*Service, error) { return newFakeService(&fakeAPI{item: processItem()}), nil },
		"eventbus", "process-configuration", "delete", "123",
	)
	h.run(t, func() (*Service, error) { return newFakeService(&fakeAPI{item: scheduleItem()}), nil },
		"eventbus", "schedule", "delete", "123",
	)
	h.run(t, func() (*Service, error) { return newFakeService(&fakeAPI{item: triggerItem()}), nil },
		"eventbus", "trigger", "delete", "123",
	)
	require.Empty(t, h.outputs, "delete は出力しません")
}

func TestCommandsWithoutFactory(t *testing.T) {
	h := newCommandHarness()
	err := h.runError(t, nil, "eventbus", "process-configuration", "list")
	require.Error(t, err)
	require.Contains(t, err.Error(), "API クライアントが設定されていません")

	// 出力検証がエラーのときは API を呼び出しません。
	api := &fakeAPI{item: processItem()}
	h.validateOutput = func(*kong.Context) error { return errors.New("出力検証エラー") }
	err = h.runError(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "list")
	require.Error(t, err)
	require.Zero(t, api.mutations)
}

func TestOutputProjectionFailureReportsID(t *testing.T) {
	// create 成功後の射影失敗を、ID とともに報告します。
	api := &fakeAPI{item: processItem()}
	api.item.Settings = v1.NewScheduleSettingsSettings(v1.ScheduleSettings{})
	h := newCommandHarness()
	err := h.runError(t, func() (*Service, error) { return newFakeService(api), nil },
		"eventbus", "process-configuration", "create",
		"--name", "wire-process",
		"--destination", "simplemq",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API 操作は成功しましたが")
	require.Contains(t, err.Error(), "ID: 123")
	require.Empty(t, h.outputs)
}

func TestRuntimeWithoutWriteOutput(t *testing.T) {
	api := &fakeAPI{item: processItem()}
	commands := NewCommands(Runtime{
		ValidateOutput: func(*kong.Context) error { return nil },
	}, func() (*Service, error) { return newFakeService(api), nil }, nil)
	parser, err := kong.New(&commands, kong.Name("skr"))
	require.NoError(t, err)
	ctx, err := parser.Parse([]string{"process-configuration", "list"})
	require.NoError(t, err)
	err = ctx.Run()
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

func TestWriteCreateUpdateOutputNilItem(t *testing.T) {
	var runtime Runtime
	err := writeCreateUpdateOutput[*ProcessConfigurationOutput](runtime, nil, nil, nil, bytes.ErrTooLarge)
	// item が nil の場合は ID を含めません。
	require.Error(t, err)
	require.NotContains(t, err.Error(), "ID:")
}
