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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

type eventbusAPICommand struct {
	ProcessConfiguration processConfigurationCommands `cmd:"" name:"process-configuration" help:"ジョブの実行先とパラメータを定義します。simple-notification、simplemq、autoscale の実行先サービスを先に作成してから、この実行設定を作成してください。作成後、schedule または trigger からこの実行設定 ID を参照します。"`
	Schedule             scheduleCommands             `cmd:"" help:"指定した時刻から定期的に実行設定を実行します。スケジュール周期は最短 1 分です。先に process-configuration を作成し、その ID を指定してください。StartsAt は Unix epoch のミリ秒を整数で指定します。例: skr eventbus-api schedule create --request='{\"CommonServiceItem\":{\"Name\":\"daily\",\"Settings\":{\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\",\"StartsAt\":1893456000000,\"RecurringStep\":1,\"RecurringUnit\":\"day\"}}}'"`
	Trigger              triggerCommands              `cmd:"" help:"イベントソースの変更を検知して実行設定を実行します。先に process-configuration を作成し、その ID を指定してください。Settings には Source、Types、ProcessConfigurationID を指定します。Source と Types の例示値は実際のイベントソースに置き換えてください。例: skr eventbus-api trigger create --request='{\"CommonServiceItem\":{\"Name\":\"on-change\",\"Settings\":{\"Source\":\"EVENT-SOURCE\",\"Types\":[\"EVENT-TYPE\"],\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\"}}}'"`
}

type itemAPI interface {
	List(context.Context) ([]v1.CommonServiceItem, error)
	Read(context.Context, string) (*v1.CommonServiceItem, error)
	Create(context.Context, v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Update(context.Context, string, v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Delete(context.Context, string) error
}

type processConfigurationAPI interface {
	itemAPI
	UpdateSecret(context.Context, string, v1.SetSecretRequest) error
}

type itemAPIFactory func() (itemAPI, error)

type processConfigurationCommands struct {
	List         itemListCommand                   `cmd:"" help:"実行設定を一覧表示し、JSON 配列で出力します。例: skr eventbus-api process-configuration list"`
	Read         itemReadCommand                   `cmd:"" help:"ID を指定して実行設定を読み取り、JSON で出力します。ID は list の出力で確認できます。"`
	Create       processConfigurationCreateCommand `cmd:"" help:"実行設定を作成します。実行先サービス（simplenotification、simplemq、autoscale）は先に作成してください。例: skr eventbus-api process-configuration create --request='{\"CommonServiceItem\":{\"Name\":\"notify\",\"Settings\":{\"Destination\":\"simplenotification\",\"Parameters\":\"{\\\"group_id\\\":\\\"GROUP-ID\\\",\\\"message\\\":\\\"hello\\\"}\"}}}'"`
	Update       processConfigurationUpdateCommand `cmd:"" help:"実行設定を部分更新します。未指定の項目は変更されません。Settings を指定する場合は Destination と Parameters を含む設定全体を指定してください。"`
	Delete       itemDeleteCommand                 `cmd:"" help:"ID を指定して実行設定を削除します。削除前に対象 ID を確認してください。"`
	UpdateSecret itemUpdateSecretCommand           `cmd:"" help:"実行設定が実行先サービスを呼び出すためのシークレットを登録します。シークレットは --secret-file でファイルまたは標準入力から読み込み、コマンドラインには記述しないでください。SimpleMQ の例: {\"APIKey\":\"SIMPLEMQ-API-KEY\"}"`
}

type scheduleCommands struct {
	List   itemListCommand       `cmd:"" help:"スケジュールを一覧表示し、JSON 配列で出力します。"`
	Read   itemReadCommand       `cmd:"" help:"ID を指定してスケジュールを読み取り、JSON で出力します。"`
	Create scheduleCreateCommand `cmd:"" help:"実行スケジュールを作成します。StartsAt は Unix epoch ミリ秒の整数です。RecurringStep と RecurringUnit（min、hour、day）、または Crontab を指定します。周期は最短 1 分です。例: skr eventbus-api schedule create --request='{\"CommonServiceItem\":{\"Name\":\"daily\",\"Settings\":{\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\",\"StartsAt\":1893456000000,\"RecurringStep\":1,\"RecurringUnit\":\"day\"}}}'"`
	Update scheduleUpdateCommand `cmd:"" help:"スケジュールを部分更新します。未指定項目は変更されません。Settings を更新する場合は ProcessConfigurationID と StartsAt、および周期または Crontab を含めます。"`
	Delete itemDeleteCommand     `cmd:"" help:"ID を指定してスケジュールを削除します。削除前に対象 ID を確認してください。"`
}

type triggerCommands struct {
	List   itemListCommand      `cmd:"" help:"トリガーを一覧表示し、JSON 配列で出力します。"`
	Read   itemReadCommand      `cmd:"" help:"ID を指定してトリガーを読み取り、JSON で出力します。"`
	Create triggerCreateCommand `cmd:"" help:"イベントトリガーを作成します。Settings に Source、Types、ProcessConfigurationID を指定します。Source と Types の例示値は実際のイベントソースに置き換えてください。例: skr eventbus-api trigger create --request='{\"CommonServiceItem\":{\"Name\":\"on-change\",\"Settings\":{\"Source\":\"EVENT-SOURCE\",\"Types\":[\"EVENT-TYPE\"],\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\"}}}'"`
	Update triggerUpdateCommand `cmd:"" help:"トリガーを部分更新します。未指定項目は変更されません。Settings を更新する場合は Source、Types、ProcessConfigurationID を含めます。"`
	Delete itemDeleteCommand    `cmd:"" help:"ID を指定してトリガーを削除します。削除前に対象 ID を確認してください。"`
}

type itemListCommand struct {
	factory itemAPIFactory
}

func (c *itemListCommand) Run(ctx *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	items, err := op.List(context.Background())
	if err != nil {
		return err
	}
	return writeJSON(ctx, items)
}

type itemReadCommand struct {
	ID      string `arg:"" help:"list の出力で確認したリソース ID。"`
	factory itemAPIFactory
}

func (c *itemReadCommand) Run(ctx *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Read(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type processConfigurationCreateCommand struct {
	Request string `name:"request" required:"" help:"CommonServiceItem に Name と Settings を含む JSON。直接指定するか @path.json でファイルを指定します。Provider は自動設定されます。"`
	factory itemAPIFactory
}

func (c *processConfigurationCreateCommand) Run(ctx *kong.Context) error {
	return runCreateItem(ctx, c.Request, c.factory, v1.ProviderClassEventbusprocessconfiguration)
}

type scheduleCreateCommand struct {
	Request string `name:"request" required:"" help:"Settings は ProcessConfigurationID、StartsAt（Unix epoch ミリ秒の整数）、RecurringStep と RecurringUnit（min、hour、day）で指定します。RecurringStep/RecurringUnit の代わりに Crontab も指定できます。直接 JSON または @path.json。"`
	factory itemAPIFactory
}

func (c *scheduleCreateCommand) Run(ctx *kong.Context) error {
	return runCreateItem(ctx, c.Request, c.factory, v1.ProviderClassEventbusschedule)
}

type triggerCreateCommand struct {
	Request string `name:"request" required:"" help:"Settings に Source、Types（文字列配列）、ProcessConfigurationID を含む JSON。直接指定するか @path.json でファイルを指定します。"`
	factory itemAPIFactory
}

func (c *triggerCreateCommand) Run(ctx *kong.Context) error {
	return runCreateItem(ctx, c.Request, c.factory, v1.ProviderClassEventbustrigger)
}

type processConfigurationUpdateCommand struct {
	ID      string `arg:"" help:"更新する実行設定 ID。"`
	Request string `name:"request" required:"" help:"変更する項目を含む UpdateCommonServiceItemRequest JSON。Settings は Destination と Parameters を含む実行設定全体を指定します。直接 JSON または @path.json。Description を null にすると説明を消去します。"`
	factory itemAPIFactory
}

func (c *processConfigurationUpdateCommand) Run(ctx *kong.Context) error {
	return runUpdateItem(ctx, c.ID, c.Request, c.factory)
}

type scheduleUpdateCommand struct {
	ID      string `arg:"" help:"更新するスケジュール ID。"`
	Request string `name:"request" required:"" help:"変更する項目を含む UpdateCommonServiceItemRequest JSON。Settings は schedule 用の JSON オブジェクトです。直接 JSON または @path.json。Description を null にすると説明を消去します。"`
	factory itemAPIFactory
}

func (c *scheduleUpdateCommand) Run(ctx *kong.Context) error {
	return runUpdateItem(ctx, c.ID, c.Request, c.factory)
}

type triggerUpdateCommand struct {
	ID      string `arg:"" help:"更新するトリガー ID。"`
	Request string `name:"request" required:"" help:"変更する項目を含む UpdateCommonServiceItemRequest JSON。Settings は Source、Types、ProcessConfigurationID を含む trigger 用 JSON オブジェクトです。直接 JSON または @path.json。Description を null にすると説明を消去します。"`
	factory itemAPIFactory
}

func (c *triggerUpdateCommand) Run(ctx *kong.Context) error {
	return runUpdateItem(ctx, c.ID, c.Request, c.factory)
}

func runCreateItem(ctx *kong.Context, input string, factory itemAPIFactory, providerClass v1.ProviderClass) error {
	var request v1.CreateCommonServiceItemRequest
	if err := decodeCreateRequest(input, providerClass, &request); err != nil {
		return err
	}
	op, err := factory()
	if err != nil {
		return err
	}
	item, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

func runUpdateItem(ctx *kong.Context, id, input string, factory itemAPIFactory) error {
	var request v1.UpdateCommonServiceItemRequest
	if err := decodeRequest(input, &request); err != nil {
		return err
	}
	op, err := factory()
	if err != nil {
		return err
	}
	item, err := op.Update(context.Background(), id, request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type itemDeleteCommand struct {
	ID      string `arg:"" help:"削除するリソース ID。list の出力で確認できます。"`
	factory itemAPIFactory
}

func (c *itemDeleteCommand) Run(_ *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	return op.Delete(context.Background(), c.ID)
}

type itemUpdateSecretCommand struct {
	ID         string `arg:"" help:"シークレットを更新する実行設定 ID。"`
	SecretFile string `name:"secret-file" required:"" help:"Secret オブジェクトの JSON ファイル。- の場合は標準入力から読み込みます。SimpleMQ: {\"APIKey\":\"...\"}。API キー認証: {\"AccessToken\":\"...\",\"AccessTokenSecret\":\"...\"}。"`
}

func (c *itemUpdateSecretCommand) Run(_ *kong.Context) error {
	var secret v1.SetSecretRequestSecret
	data, err := readSecretFile(c.SecretFile)
	if err != nil {
		return err
	}
	if err := unmarshalRequest(data, &secret); err != nil {
		return err
	}
	op, err := newProcessConfigurationAPI()
	if err != nil {
		return err
	}
	request := v1.SetSecretRequest{Secret: secret}
	return op.UpdateSecret(context.Background(), c.ID, request)
}

func newCLI() cli {
	result := cli{}
	processConfigurationFactory := func() (itemAPI, error) {
		return newProcessConfigurationAPI()
	}
	result.EventbusAPI.ProcessConfiguration.setFactory(processConfigurationFactory)
	result.EventbusAPI.Schedule.setFactory(func() (itemAPI, error) {
		client, err := newEventbusClient()
		if err != nil {
			return nil, err
		}
		return eventbus.NewScheduleOp(client), nil
	})
	result.EventbusAPI.Trigger.setFactory(func() (itemAPI, error) {
		client, err := newEventbusClient()
		if err != nil {
			return nil, err
		}
		return eventbus.NewTriggerOp(client), nil
	})
	return result
}

func (c *scheduleCommands) setFactory(factory itemAPIFactory) {
	c.List.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
}

func (c *triggerCommands) setFactory(factory itemAPIFactory) {
	c.List.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
}

func (c *processConfigurationCommands) setFactory(factory itemAPIFactory) {
	c.List.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
}

func newProcessConfigurationAPI() (processConfigurationAPI, error) {
	client, err := newEventbusClient()
	if err != nil {
		return nil, err
	}
	return eventbus.NewProcessConfigurationOp(client), nil
}

func newEventbusClient() (*v1.Client, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	return eventbus.NewClient(&client)
}

func decodeRequest(input string, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}
	return unmarshalRequest(data, destination)
}

func decodeCreateRequest(input string, providerClass v1.ProviderClass, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}

	var request struct {
		CommonServiceItem json.RawMessage `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request.CommonServiceItem, &item); err != nil {
		return fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	provider, err := json.Marshal(struct {
		Class v1.ProviderClass `json:"Class"`
	}{Class: providerClass})
	if err != nil {
		return fmt.Errorf("encode EventBus provider: %w", err)
	}
	item["Provider"] = provider
	request.CommonServiceItem, err = json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	data, err = json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode create request: %w", err)
	}
	return unmarshalRequest(data, destination)
}

func requestData(input string) ([]byte, error) {
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // Path is explicitly provided by the user via --secret-file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}

func unmarshalRequest(data []byte, destination any) error {
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func writeJSON(ctx *kong.Context, value any) error {
	encoder := json.NewEncoder(ctx.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
