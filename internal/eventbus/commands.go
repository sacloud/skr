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

// Package eventbus は EventBus の高レベルコマンド層です。
// 実行設定 (process-configuration)、スケジュール (schedule)、トリガー (trigger) の
// 操作を宛先別の個別入力と反復可能なオプションで提供します。
//
// service 層は service/eventbus-service-go モジュールに切り出してあり (ADR 0077)、
// このパッケージは kong コマンドとリクエストの組み立て、出力処理だけを持ちます。
// このファイルは手書きで、低レベルの eventbus-api と JSON 入力の経路には
// 変更を加えません (ADR 0075、ADR 0076)。
package eventbus

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"

	eventbusapi "github.com/sacloud/skr/service/eventbus-service-go"
)

// deref は未指定のフラグを空文字列へ変換します。
// create 用のリクエストは Name と Description が string で、未指定は空文字列です。
func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Runtime は CLI 側から注入する出力処理の集合です。
type Runtime struct {
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

// ServiceFactory は API クライアントを構築して service/eventbus-service-go の
// Service を返します。
type ServiceFactory func() (*eventbusapi.Service, error)

// SecretServiceFactory は set-secret 専用の Service を返します。
// secret を扱う操作では実行前にトレース設定を検査するため、
// 通常の操作と別の factory を受け取ります。
type SecretServiceFactory func() (*eventbusapi.Service, error)

func (r Runtime) validateOutput(kongCtx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(kongCtx)
}

func (r Runtime) writeOutput(kongCtx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(kongCtx, value)
}

// newService は factory から Service を構築します。
// newService は factory から service/eventbus-service-go の Service を構築します。
func newService(factory ServiceFactory) (*eventbusapi.Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

// newSecretService は set-secret 用の factory から Service を構築します。
// newSecretService は set-secret 用の factory から Service を構築します。
func newSecretService(factory SecretServiceFactory) (*eventbusapi.Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

// commandContext は EventBus コマンド専用の実行境界を作ります。
// 割り込みシグナルで context がキャンセルされるため、読み取りと変更操作の間に
// 割り込まれた場合も service 層の確認が変更を続行させません。
func commandContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// Commands は skr eventbus 配下のコマンド群です。
type Commands struct {
	ProcessConfiguration ProcessConfigurationCommands `cmd:"" name:"process-configuration" help:"実行設定 (ジョブの宛先) を操作します。実行設定は SimpleMQ、シンプル通知、オートスケールのいずれかの宛先を持ち、スケジュールとトリガーから ID で参照されます。"`
	Schedule             ScheduleCommands             `cmd:"" name:"schedule" help:"スケジュールを操作します。スケジュールは実行設定を開始日時から、分・時間・日の間隔または Crontab で実行します。間隔と Crontab は同時に指定できません。"`
	Trigger              TriggerCommands              `cmd:"" name:"trigger" help:"トリガーを操作します。トリガーは実行設定を指定した送信元のイベントと結び付け、条件に一致するイベントでジョブを実行します。"`
}

// NewCommands は runtime と factory を各コマンドへ一括注入します。
// set-secret だけ secret を扱うための専用 factory を受け取ります。
func NewCommands(runtime Runtime, factory ServiceFactory, secretFactory SecretServiceFactory) Commands {
	return Commands{
		ProcessConfiguration: ProcessConfigurationCommands{
			List:      ProcessListCommand{factory: factory, runtime: runtime},
			Read:      ProcessReadCommand{factory: factory, runtime: runtime},
			Create:    ProcessCreateCommand{factory: factory, runtime: runtime},
			Update:    ProcessUpdateCommand{factory: factory, runtime: runtime},
			Delete:    ProcessDeleteCommand{factory: factory, runtime: runtime},
			SetSecret: ProcessSetSecretCommand{factory: secretFactory, runtime: runtime},
		},
		Schedule: ScheduleCommands{
			List:   ScheduleListCommand{factory: factory, runtime: runtime},
			Read:   ScheduleReadCommand{factory: factory, runtime: runtime},
			Create: ScheduleCreateCommand{factory: factory, runtime: runtime},
			Update: ScheduleUpdateCommand{factory: factory, runtime: runtime},
			Delete: ScheduleDeleteCommand{factory: factory, runtime: runtime},
		},
		Trigger: TriggerCommands{
			List:   TriggerListCommand{factory: factory, runtime: runtime},
			Read:   TriggerReadCommand{factory: factory, runtime: runtime},
			Create: TriggerCreateCommand{factory: factory, runtime: runtime},
			Update: TriggerUpdateCommand{factory: factory, runtime: runtime},
			Delete: TriggerDeleteCommand{factory: factory, runtime: runtime},
		},
	}
}

// ProcessConfigurationCommands は実行設定のコマンド群です。
type ProcessConfigurationCommands struct {
	List      ProcessListCommand      `cmd:"" help:"実行設定を一覧表示します。出力は宛先別の個別フィールドへ射影され、低レベルの list-common-service-items より少ない情報を表示します。完全な応答が必要なときは低レベルコマンドを使ってください。"`
	Read      ProcessReadCommand      `cmd:"" help:"ID を指定して実行設定を表示します。"`
	Create    ProcessCreateCommand    `cmd:"" help:"実行設定を作成します。宛先 (--destination) に応じてパラメーターが変わります: simplenotification は --group-id と --message、simplemq は --queue-name と --content、autoscale は --action と --resource-id を指定します。未指定のパラメーターは空文字列として送信します。"`
	Update    ProcessUpdateCommand    `cmd:"" help:"実行設定を更新します。指定した項目だけを変更し、未指定の項目は現状維持します。--destination を変更した場合、旧宛先のパラメーターは引き継がれず、指定のないパラメーターは空文字列として送信されるため、新宛先のパラメーターを指定してください。読み取りと更新の間に他の変更がないことは検証しません。"`
	Delete    ProcessDeleteCommand    `cmd:"" help:"実行設定を削除します。削除対象は API から再取得し、リソース種別 (Provider.Class) と設定の種類が一致しない場合は削除しません。"`
	SetSecret ProcessSetSecretCommand `cmd:"" name:"set-secret" help:"実行設定の secret を登録または置き換えます。secret は --secret-file で保護したファイル (@path) または標準入力 (-) から読み込みます。値を引数やインライン JSON に含めることはできません。SimpleMQ 宛先は {\"APIKey\":\"...\"}、それ以外の宛先は {\"AccessToken\":\"...\",\"AccessTokenSecret\":\"...\"} を指定します。トレースが有効な場合は実行しません。"`
}

// ProcessListCommand は実行設定の一覧を表示します。
type ProcessListCommand struct {
	factory ServiceFactory
	runtime Runtime
}

func (c *ProcessListCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	items, err := svc.ListProcessConfigurations(ctx)
	if err != nil {
		return err
	}
	projected := make([]*eventbusapi.ProcessConfigurationOutput, 0, len(items))
	for i := range items {
		output, err := eventbusapi.ProjectProcessConfiguration(&items[i])
		if err != nil {
			return err
		}
		projected = append(projected, output)
	}
	return c.runtime.writeOutput(kongCtx, projected)
}

// ProcessReadCommand は ID を指定して実行設定を表示します。
type ProcessReadCommand struct {
	ID      string `arg:"" name:"id" help:"表示する実行設定 ID。skr eventbus process-configuration list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *ProcessReadCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.ReadProcessConfiguration(ctx, &eventbusapi.ReadRequest{ID: c.ID})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectProcessConfiguration(item)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, output)
}

// ProcessCreateCommand は宛先別の個別入力で実行設定を作成します。
type ProcessCreateCommand struct {
	Name        string  `required:"" help:"実行設定の名前。"`
	Description *string `help:"説明。空文字列は null として送信します。"`
	Destination string  `required:"" help:"ジョブの宛先。simplenotification、simplemq、autoscale のいずれか。"`
	GroupID     *string `help:"--destination simplenotification のときのグループ ID。"`
	Message     *string `help:"--destination simplenotification のときのメッセージ。"`
	QueueName   *string `help:"--destination simplemq のときのキュー名。"`
	Content     *string `help:"--destination simplemq のときに配送するメッセージの本文。"`
	Action      *string `help:"--destination autoscale のときのアクション。scale_up または scale_down。"`
	ResourceID  *string `help:"--destination autoscale のときの対象リソース ID。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *ProcessCreateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.CreateProcessConfiguration(ctx, &eventbusapi.CreateProcessConfigurationRequest{
		Name:        c.Name,
		Description: deref(c.Description),
		Settings: eventbusapi.ProcessConfigurationSettings{
			Destination: c.Destination,
			GroupID:     c.GroupID,
			Message:     c.Message,
			QueueName:   c.QueueName,
			Content:     c.Content,
			Action:      c.Action,
			ResourceID:  c.ResourceID,
		},
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectProcessConfiguration(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// ProcessUpdateCommand は指定した項目だけを変更して実行設定を更新します。
type ProcessUpdateCommand struct {
	ID          string  `arg:"" name:"id" help:"更新する実行設定 ID。skr eventbus process-configuration list の出力で確認できます。"`
	Name        *string `help:"新しい名前。未指定のときは現状維持します。"`
	Description *string `help:"新しい説明。未指定のときは現状維持します。空文字列はそのまま送信します (作成時と異なり null にはなりません)。"`
	Destination *string `help:"新しい宛先。simplenotification、simplemq、autoscale のいずれか。変更した場合、旧宛先のパラメーターは引き継がれません。"`
	GroupID     *string `help:"--destination simplenotification のときのグループ ID。"`
	Message     *string `help:"--destination simplenotification のときのメッセージ。"`
	QueueName   *string `help:"--destination simplemq のときのキュー名。"`
	Content     *string `help:"--destination simplemq のときに配送するメッセージの本文。"`
	Action      *string `help:"--destination autoscale のときのアクション。scale_up または scale_down。"`
	ResourceID  *string `help:"--destination autoscale のときの対象リソース ID。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *ProcessUpdateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.UpdateProcessConfiguration(ctx, &eventbusapi.UpdateProcessConfigurationRequest{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Destination: c.Destination,
		GroupID:     c.GroupID,
		Message:     c.Message,
		QueueName:   c.QueueName,
		Content:     c.Content,
		Action:      c.Action,
		ResourceID:  c.ResourceID,
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectProcessConfiguration(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// ProcessDeleteCommand は ID を指定して実行設定を削除します。
type ProcessDeleteCommand struct {
	ID      string `arg:"" name:"id" help:"削除する実行設定 ID。skr eventbus process-configuration list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *ProcessDeleteCommand) Run(_ *kong.Context) error {
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	return svc.DeleteProcessConfiguration(ctx, &eventbusapi.DeleteRequest{ID: c.ID})
}

// ProcessSetSecretCommand は実行設定へ secret を登録します。
// 入力はファイルまたは標準入力のみで、インライン指定は行いません。
type ProcessSetSecretCommand struct {
	ID         string `arg:"" name:"id" help:"secret を登録する実行設定 ID。skr eventbus process-configuration list の出力で確認できます。"`
	SecretFile string `name:"secret-file" required:"" help:"secret の JSON。@path で保護したファイル、- で標準入力から読み込みます。SimpleMQ 宛先は {\"APIKey\":\"...\"}、それ以外は {\"AccessToken\":\"...\",\"AccessTokenSecret\":\"...\"} の形式です。"`
	factory    SecretServiceFactory
	runtime    Runtime
}

func (c *ProcessSetSecretCommand) Run(_ *kong.Context) error {
	svc, err := newSecretService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	secret, err := LoadSecretInput(c.SecretFile)
	if err != nil {
		return err
	}
	if err := svc.SetProcessConfigurationSecret(ctx, &eventbusapi.SetProcessConfigurationSecretRequest{
		ID:     c.ID,
		Secret: secret,
	}); err != nil {
		return SanitizeSecretError(err)
	}
	return nil
}

// writeCreateUpdateOutput は create・update の成功応答を出力へ射影します。
// 射影に失敗した場合は、変更自体が成功していることと確認できた ID を伝え、
// 自動再送は行いません。
func writeCreateUpdateOutput[T any](runtime Runtime, kongCtx *kong.Context, item *v1.CommonServiceItem, output T, err error) error {
	if err != nil {
		if item != nil {
			return fmt.Errorf("API 操作は成功しましたが、応答の処理に失敗しました (ID: %s): %w", item.ID, err)
		}
		return fmt.Errorf("API 操作は成功しましたが、応答の処理に失敗しました: %w", err)
	}
	return runtime.writeOutput(kongCtx, output)
}

// ScheduleCommands はスケジュールのコマンド群です。
type ScheduleCommands struct {
	List   ScheduleListCommand   `cmd:"" help:"スケジュールを一覧表示します。StartsAt はエポックミリ秒から RFC3339 形式 (UTC) へ変換して表示します。完全な応答が必要なときは低レベルの list-common-service-items を使ってください。"`
	Read   ScheduleReadCommand   `cmd:"" help:"ID を指定してスケジュールを表示します。"`
	Create ScheduleCreateCommand `cmd:"" help:"スケジュールを作成します。開始日時 (--starts-at) から、分・時間・日の間隔 (--recurring-step と --recurring-unit) または Crontab (--crontab) のどちらかで繰り返します。両方は指定できません。--recurring-step と --recurring-unit は片方だけの指定ではエラーになります。"`
	Update ScheduleUpdateCommand `cmd:"" help:"スケジュールを更新します。指定した項目だけを変更し、未指定の項目は現状維持します。--recurring-step と --recurring-unit は組で指定し、片方だけの変更はエラーになります。繰り返しモードを切り替えるときは、切り替え先のモードのオプションを指定してください (例: Crontab から間隔指定へは --recurring-step と --recurring-unit を指定します)。読み取りと更新の間に他の変更がないことは検証しません。"`
	Delete ScheduleDeleteCommand `cmd:"" help:"スケジュールを削除します。削除対象は API から再取得し、リソース種別 (Provider.Class) と設定の種類が一致しない場合は削除しません。"`
}

// ScheduleListCommand はスケジュールの一覧を表示します。
type ScheduleListCommand struct {
	factory ServiceFactory
	runtime Runtime
}

func (c *ScheduleListCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	items, err := svc.ListSchedules(ctx)
	if err != nil {
		return err
	}
	projected := make([]*eventbusapi.ScheduleOutput, 0, len(items))
	for i := range items {
		output, err := eventbusapi.ProjectSchedule(&items[i])
		if err != nil {
			return err
		}
		projected = append(projected, output)
	}
	return c.runtime.writeOutput(kongCtx, projected)
}

// ScheduleReadCommand は ID を指定してスケジュールを表示します。
type ScheduleReadCommand struct {
	ID      string `arg:"" name:"id" help:"表示するスケジュール ID。skr eventbus schedule list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *ScheduleReadCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.ReadSchedule(ctx, &eventbusapi.ReadRequest{ID: c.ID})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectSchedule(item)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, output)
}

// ScheduleCreateCommand は実行モード (間隔または Crontab) を指定してスケジュールを作成します。
type ScheduleCreateCommand struct {
	Name                   string  `required:"" help:"スケジュールの名前。"`
	Description            *string `help:"説明。空文字列は null として送信します。"`
	ProcessConfigurationID string  `required:"" help:"実行する実行設定の ID。skr eventbus process-configuration list の出力で確認できます。"`
	StartsAt               string  `required:"" help:"実行を開始する日時。RFC3339 形式 (例: 2030-01-10T09:00:00+09:00、小数秒はミリ秒まで) で指定し、API へはエポックミリ秒で送信します。"`
	RecurringStep          *int    `help:"繰り返しの間隔。--recurring-unit と組み合わせます。"`
	RecurringUnit          *string `help:"繰り返しの単位。min、hour、day のいずれか。--recurring-step と組み合わせます。"`
	Crontab                *string `help:"繰り返しを Crontab 形式で指定します。--recurring-step、--recurring-unit と同時に指定できません。"`
	factory                ServiceFactory
	runtime                Runtime
}

func (c *ScheduleCreateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	startsAt, err := parseStartsAt(c.StartsAt)
	if err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.CreateSchedule(ctx, &eventbusapi.CreateScheduleRequest{
		Name:        c.Name,
		Description: deref(c.Description),
		Settings: eventbusapi.ScheduleSettings{
			ProcessConfigurationID: c.ProcessConfigurationID,
			StartsAt:               startsAt,
			RecurringStep:          c.RecurringStep,
			RecurringUnit:          c.RecurringUnit,
			Crontab:                c.Crontab,
		},
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectSchedule(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// ScheduleUpdateCommand は指定した項目だけを変更してスケジュールを更新します。
type ScheduleUpdateCommand struct {
	ID                     string  `arg:"" name:"id" help:"更新するスケジュール ID。skr eventbus schedule list の出力で確認できます。"`
	Name                   *string `help:"新しい名前。未指定のときは現状維持します。"`
	Description            *string `help:"新しい説明。未指定のときは現状維持します。空文字列はそのまま送信します (作成時と異なり null にはなりません)。"`
	ProcessConfigurationID *string `help:"実行する実行設定の新しい ID。未指定のときは現状維持します。"`
	StartsAt               *string `help:"実行を開始する新しい日時。RFC3339 形式 (例: 2030-01-10T09:00:00+09:00、小数秒はミリ秒まで) で指定し、API へはエポックミリ秒で送信します。"`
	RecurringStep          *int    `help:"繰り返しの新しい間隔。--recurring-unit と組み合わせます。片方だけの変更はエラーになります。"`
	RecurringUnit          *string `help:"繰り返しの新しい単位。min、hour、day のいずれか。--recurring-step と組み合わせます。片方だけの変更はエラーになります。"`
	Crontab                *string `help:"繰り返しを Crontab 形式で指定します。指定すると間隔指定 (step と unit) は解除されます。--recurring-step、--recurring-unit と同時に指定できません。"`
	factory                ServiceFactory
	runtime                Runtime
}

func (c *ScheduleUpdateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	startsAt, err := parseOptionalStartsAt(c.StartsAt)
	if err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.UpdateSchedule(ctx, &eventbusapi.UpdateScheduleRequest{
		ID:                     c.ID,
		Name:                   c.Name,
		Description:            c.Description,
		ProcessConfigurationID: c.ProcessConfigurationID,
		StartsAt:               startsAt,
		RecurringStep:          c.RecurringStep,
		RecurringUnit:          c.RecurringUnit,
		Crontab:                c.Crontab,
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectSchedule(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// ScheduleDeleteCommand は ID を指定してスケジュールを削除します。
type ScheduleDeleteCommand struct {
	ID      string `arg:"" name:"id" help:"削除するスケジュール ID。skr eventbus schedule list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *ScheduleDeleteCommand) Run(_ *kong.Context) error {
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	return svc.DeleteSchedule(ctx, &eventbusapi.DeleteRequest{ID: c.ID})
}

// TriggerCommands はトリガーのコマンド群です。
type TriggerCommands struct {
	List   TriggerListCommand   `cmd:"" help:"トリガーを一覧表示します。Conditions は等号 (eq) と包含 (in) の形へ射影します。完全な応答が必要なときは低レベルの list-common-service-items を使ってください。"`
	Read   TriggerReadCommand   `cmd:"" help:"ID を指定してトリガーを表示します。"`
	Create TriggerCreateCommand `cmd:"" help:"トリガーを作成します。--source で送信元を指定し、--type でイベントタイプを、--condition で条件を指定します (どちらも省略できます)。--type と --condition は反復可能で、指定した値全体で置き換えます。値にカンマを含めても分割しません。"`
	Update TriggerUpdateCommand `cmd:"" help:"トリガーを更新します。指定した項目だけを変更し、未指定の項目は現状維持します。--type と --condition は指定時に既存値の全体を置き換えます。Types や Conditions を空にする (null への変更) は低レベルの eventbus-api で行ってください。読み取りと更新の間に他の変更がないことは検証しません。"`
	Delete TriggerDeleteCommand `cmd:"" help:"トリガーを削除します。削除対象は API から再取得し、リソース種別 (Provider.Class) と設定の種類が一致しない場合は削除しません。"`
}

// TriggerListCommand はトリガーの一覧を表示します。
type TriggerListCommand struct {
	factory ServiceFactory
	runtime Runtime
}

func (c *TriggerListCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	items, err := svc.ListTriggers(ctx)
	if err != nil {
		return err
	}
	projected := make([]*eventbusapi.TriggerOutput, 0, len(items))
	for i := range items {
		output, err := eventbusapi.ProjectTrigger(&items[i])
		if err != nil {
			return err
		}
		projected = append(projected, output)
	}
	return c.runtime.writeOutput(kongCtx, projected)
}

// TriggerReadCommand は ID を指定してトリガーを表示します。
type TriggerReadCommand struct {
	ID      string `arg:"" name:"id" help:"表示するトリガー ID。skr eventbus trigger list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *TriggerReadCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.ReadTrigger(ctx, &eventbusapi.ReadRequest{ID: c.ID})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectTrigger(item)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, output)
}

// TriggerCreateCommand は反復可能な --type と --condition でトリガーを作成します。
type TriggerCreateCommand struct {
	Name                   string   `required:"" help:"トリガーの名前。"`
	Description            *string  `help:"説明。空文字列は null として送信します。"`
	Source                 string   `required:"" help:"送信元イベントの URI。//eventbus.sakura.ad.jp/eventlog または //eventbus.sakura.ad.jp/monitoringsuite/alert のいずれか。"`
	Type                   []string `sep:"none" help:"対象のイベントタイプ。反復可能で、指定した値全体で置き換えます。"`
	Condition              []string `sep:"none" help:"イベントの一致条件。key=名前,op=eq|in,value=値 の形式で指定します。key は小文字英数字 (20 文字以内) で data は使えません。value は反復可能で、op=in のとき複数の値で包含を表します。反復可能で、指定した条件全体で置き換えます。"`
	ProcessConfigurationID string   `required:"" help:"実行する実行設定の ID。skr eventbus process-configuration list の出力で確認できます。"`
	factory                ServiceFactory
	runtime                Runtime
}

func (c *TriggerCreateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	conditions, err := ParseConditions(c.Condition)
	if err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.CreateTrigger(ctx, &eventbusapi.CreateTriggerRequest{
		Name:        c.Name,
		Description: deref(c.Description),
		Settings: eventbusapi.TriggerSettings{
			Source:                 c.Source,
			Types:                  c.Type,
			Conditions:             conditions,
			ProcessConfigurationID: c.ProcessConfigurationID,
		},
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectTrigger(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// TriggerUpdateCommand は指定した項目だけを変更してトリガーを更新します。
type TriggerUpdateCommand struct {
	ID                     string   `arg:"" name:"id" help:"更新するトリガー ID。skr eventbus trigger list の出力で確認できます。"`
	Name                   *string  `help:"新しい名前。未指定のときは現状維持します。"`
	Description            *string  `help:"新しい説明。未指定のときは現状維持します。空文字列はそのまま送信します (作成時と異なり null にはなりません)。"`
	Source                 *string  `help:"送信元イベントの新しい URI。//eventbus.sakura.ad.jp/eventlog または //eventbus.sakura.ad.jp/monitoringsuite/alert のいずれか。未指定のときは現状維持します。"`
	Type                   []string `sep:"none" help:"対象の新しいイベントタイプ。反復可能で、指定時に既存値の全体を置き換えます。未指定のときは現状維持します。"`
	Condition              []string `sep:"none" help:"イベントの新しい一致条件。key=名前,op=eq|in,value=値 の形式で指定します。value は反復可能です。反復可能で、指定時に既存の条件全体を置き換えます。未指定のときは現状維持します。"`
	ProcessConfigurationID *string  `help:"実行する実行設定の新しい ID。未指定のときは現状維持します。"`
	factory                ServiceFactory
	runtime                Runtime
}

func (c *TriggerUpdateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	conditions, err := ParseConditions(c.Condition)
	if err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	item, err := svc.UpdateTrigger(ctx, &eventbusapi.UpdateTriggerRequest{
		ID:                     c.ID,
		Name:                   c.Name,
		Description:            c.Description,
		Source:                 c.Source,
		Types:                  c.Type,
		Conditions:             conditions,
		ProcessConfigurationID: c.ProcessConfigurationID,
	})
	if err != nil {
		return err
	}
	output, err := eventbusapi.ProjectTrigger(item)
	return writeCreateUpdateOutput(c.runtime, kongCtx, item, output, err)
}

// TriggerDeleteCommand は ID を指定してトリガーを削除します。
type TriggerDeleteCommand struct {
	ID      string `arg:"" name:"id" help:"削除するトリガー ID。skr eventbus trigger list の出力で確認できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *TriggerDeleteCommand) Run(_ *kong.Context) error {
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	return svc.DeleteTrigger(ctx, &eventbusapi.DeleteRequest{ID: c.ID})
}

// parseStartsAt は --starts-at の RFC3339 形式を解析します。
func parseStartsAt(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("--starts-at に RFC3339 形式 (例: 2030-01-10T09:00:00+09:00) の日時を指定してください")
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--starts-at に RFC3339 形式 (例: 2030-01-10T09:00:00+09:00) の日時を指定してください: %w", err)
	}
	return t, nil
}

// parseOptionalStartsAt は更新用の --starts-at を未指定のとき nil のまま解析します。
func parseOptionalStartsAt(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	t, err := parseStartsAt(*value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
