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

package eventbusapi

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

type Commands struct {
	ProcessConfiguration ProcessConfigurationCommands `cmd:"" name:"process-configuration" help:"ジョブの実行先とパラメータを定義します。simple-notification、simplemq、autoscale の実行先サービスを先に作成してから、この実行設定を作成してください。作成後、schedule または trigger からこの実行設定 ID を参照します。"`
	Schedule             ScheduleCommands             `cmd:"" help:"指定した時刻から定期的に実行設定を実行します。スケジュール周期は最短 1 分です。先に process-configuration を作成し、その ID を指定してください。StartsAt は Unix epoch のミリ秒を整数で指定します。例: skr eventbus-api schedule create --request='{\"CommonServiceItem\":{\"Name\":\"daily\",\"Settings\":{\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\",\"StartsAt\":1893456000000,\"RecurringStep\":1,\"RecurringUnit\":\"day\"}}}'"`
	Trigger              TriggerCommands              `cmd:"" help:"イベントソースの変更を検知して実行設定を実行します。先に process-configuration を作成し、その ID を指定してください。Settings には Source、Types、ProcessConfigurationID を指定します。Source と Types の例示値は実際のイベントソースに置き換えてください。例: skr eventbus-api trigger create --request='{\"CommonServiceItem\":{\"Name\":\"on-change\",\"Settings\":{\"Source\":\"EVENT-SOURCE\",\"Types\":[\"EVENT-TYPE\"],\"ProcessConfigurationID\":\"PROCESS-CONFIGURATION-ID\"}}}'"`
}

type ProcessConfigurationUpdateSecretCommand struct {
	ID         string `arg:"" help:"シークレットを更新する実行設定 ID。"`
	SecretFile string `name:"secret-file" required:"" help:"Secret オブジェクトの JSON ファイル。- の場合は標準入力から読み込みます。SimpleMQ: {\"APIKey\":\"...\"}。API キー認証: {\"AccessToken\":\"...\",\"AccessTokenSecret\":\"...\"}。"`
	factory    ProcessConfigurationAPIFactory
	runtime    ProcessConfigurationRuntime
}

func (c *ProcessConfigurationUpdateSecretCommand) Run(_ *kong.Context) error {
	var secret v1.SetSecretRequestSecret
	data, err := readSecretFile(c.SecretFile)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &secret); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	request := v1.SetSecretRequest{Secret: secret}
	return op.UpdateSecret(context.Background(), c.ID, request)
}

func NewProcessConfigurationAPI() (ProcessConfigurationAPI, error) {
	client, err := newClient()
	if err != nil {
		return nil, err
	}
	return eventbus.NewProcessConfigurationOp(client), nil
}

func NewScheduleAPI() (ScheduleAPI, error) {
	client, err := newClient()
	if err != nil {
		return nil, err
	}
	return eventbus.NewScheduleOp(client), nil
}

func NewTriggerAPI() (TriggerAPI, error) {
	client, err := newClient()
	if err != nil {
		return nil, err
	}
	return eventbus.NewTriggerOp(client), nil
}

func ValidateRequest(name string, request any) error {
	switch name {
	case "setProcessConfigurationProvider":
		return setProvider(request, v1.ProviderClassEventbusprocessconfiguration)
	case "setScheduleProvider":
		return setProvider(request, v1.ProviderClassEventbusschedule)
	case "setTriggerProvider":
		return setProvider(request, v1.ProviderClassEventbustrigger)
	default:
		return fmt.Errorf("unknown EventBus request validator %q", name)
	}
}

func DecodeRequest(input string, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}
	if _, ok := destination.(*v1.CreateCommonServiceItemRequest); ok {
		data, err = withProvider(data, v1.ProviderClassEventbusprocessconfiguration)
		if err != nil {
			return err
		}
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func setProvider(request any, providerClass v1.ProviderClass) error {
	createRequest, ok := request.(*v1.CreateCommonServiceItemRequest)
	if !ok {
		return fmt.Errorf("EventBus provider can only be set on create requests")
	}
	createRequest.CommonServiceItem.Provider = v1.Provider{Class: providerClass}
	return nil
}

func withProvider(data []byte, providerClass v1.ProviderClass) ([]byte, error) {
	var request struct {
		CommonServiceItem json.RawMessage `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request.CommonServiceItem, &item); err != nil {
		return nil, fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return nil, fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	provider, err := json.Marshal(struct {
		Class v1.ProviderClass `json:"Class"`
	}{Class: providerClass})
	if err != nil {
		return nil, fmt.Errorf("encode EventBus provider: %w", err)
	}
	item["Provider"] = provider
	request.CommonServiceItem, err = json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	data, err = json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode create request: %w", err)
	}
	return data, nil
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

func newClient() (*v1.Client, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	return eventbus.NewClient(&client)
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
