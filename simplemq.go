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
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/queue"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

type simpleMQAPICommand struct {
	Queue   simpleMQQueueCommands   `cmd:"" help:"キューを作成・管理します。"`
	Message simpleMQMessageCommands `cmd:"" help:"キューの API キーを使ってメッセージを送受信します。"`
}

type simpleMQQueueCommands struct {
	List          simpleMQQueueListCommand          `cmd:"" help:"キューを一覧表示します。"`
	Read          simpleMQQueueReadCommand          `cmd:"" help:"ID を指定してキューを読み取ります。"`
	Create        simpleMQQueueCreateCommand        `cmd:"" help:"キューを作成します。作成後、rotate-api-key でメッセージ API 用のキーを発行してください。"`
	Config        simpleMQQueueConfigCommand        `cmd:"" help:"キューの設定を更新します。--visibility-timeout-seconds と --expire-seconds を指定するか、--request JSON を指定してください。"`
	Delete        simpleMQQueueDeleteCommand        `cmd:"" help:"ID を指定してキューを削除します。"`
	CountMessages simpleMQQueueCountMessagesCommand `cmd:"" name:"count-messages" help:"キュー内のメッセージ数を取得します。"`
	RotateAPIKey  simpleMQQueueRotateAPIKeyCommand  `cmd:"" name:"rotate-api-key" help:"メッセージ API キーを発行または再発行し、結果を出力します。APIKey は認証情報として安全に扱ってください。"`
	ClearMessages simpleMQQueueClearMessagesCommand `cmd:"" name:"clear-messages" help:"キュー内のメッセージをすべて削除します。"`
}

type simpleMQMessageCommands struct {
	Send          simpleMQMessageSendCommand          `cmd:"" help:"キューへメッセージを送信します。"`
	Receive       simpleMQMessageReceiveCommand       `cmd:"" help:"キューからメッセージを受信します。"`
	ExtendTimeout simpleMQMessageExtendTimeoutCommand `cmd:"" name:"extend-timeout" help:"受信したメッセージの可視性タイムアウトを延長します。"`
	Delete        simpleMQMessageDeleteCommand        `cmd:"" help:"受信したメッセージを削除します。"`
}

type simpleMQQueueListCommand struct{}

func (simpleMQQueueListCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	items, err := op.List(context.Background())
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, items)
}

type simpleMQQueueReadCommand struct {
	ID string `arg:"" help:"list の出力にあるキュー ID。"`
}

func (c simpleMQQueueReadCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	item, err := op.Read(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, item)
}

type simpleMQQueueCreateCommand struct {
	Request     *string `help:"個別フラグと併用不可。CreateQueueRequest JSON を直接または @path.json で指定します。CommonServiceItem.Name は5〜64文字の英数字またはハイフンです。Provider.Class は simplemq に設定します。Tags と Icon は JSON で指定します。例: --request='{\"CommonServiceItem\":{\"Name\":\"sample-queue\"}}'"`
	Name        *string `help:"必須: キュー名 (5〜64文字の英数字またはハイフン)。例: sample-queue"`
	Description *string `help:"任意: キューの説明。"`
}

func (c simpleMQQueueCreateCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	flags := c.Name != nil || c.Description != nil
	if c.Request != nil && flags {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	var request queue.CreateQueueRequest
	if c.Request != nil {
		if err := decodeSimpleMQCreateRequest(*c.Request, &request); err != nil {
			return err
		}
	} else {
		if c.Name == nil || *c.Name == "" {
			return fmt.Errorf("--name が必要です (--request JSON も指定できます)")
		}
		item := queue.CreateQueueRequestCommonServiceItem{Name: queue.QueueName(*c.Name)}
		if c.Description != nil {
			item.Description = queue.NewOptString(*c.Description)
		}
		request.CommonServiceItem = item
	}

	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	item, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, item)
}

func decodeSimpleMQCreateRequest(input string, destination *queue.CreateQueueRequest) error {
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
		Class queue.CreateQueueRequestCommonServiceItemProviderClass `json:"Class"`
	}{Class: queue.CreateQueueRequestCommonServiceItemProviderClassSimplemq})
	if err != nil {
		return fmt.Errorf("encode SimpleMQ provider: %w", err)
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

type simpleMQQueueConfigCommand struct {
	ID                       string  `arg:"" help:"設定を変更するキュー ID。"`
	Request                  *string `help:"個別フラグと併用不可。ConfigQueueRequest JSON を直接または @path.json で指定します。Description、Tags、Icon など追加項目は JSON で指定します。例: --request='{\"CommonServiceItem\":{\"Settings\":{\"VisibilityTimeoutSeconds\":30,\"ExpireSeconds\":345600}}}'"`
	VisibilityTimeoutSeconds *int    `name:"visibility-timeout-seconds" help:"必須: 可視性タイムアウト秒数 (5〜900)。API の CommonServiceItem.Settings.VisibilityTimeoutSeconds に対応します。--expire-seconds と併せて指定します。"`
	ExpireSeconds            *int    `name:"expire-seconds" help:"必須: メッセージ保存期間秒数 (60〜1209600)。API の CommonServiceItem.Settings.ExpireSeconds に対応します。--visibility-timeout-seconds と併せて指定します。"`
}

func (c simpleMQQueueConfigCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	request, err := simpleMQConfigRequest(c.Request, c.VisibilityTimeoutSeconds, c.ExpireSeconds)
	if err != nil {
		return err
	}
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	item, err := op.Config(context.Background(), c.ID, request)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, item)
}

func simpleMQConfigRequest(input *string, visibilityTimeoutSeconds, expireSeconds *int) (queue.ConfigQueueRequest, error) {
	if input != nil {
		if visibilityTimeoutSeconds != nil || expireSeconds != nil {
			return queue.ConfigQueueRequest{}, fmt.Errorf("--request と個別フラグは併用できません")
		}
		var request queue.ConfigQueueRequest
		if err := decodeRequest(*input, &request); err != nil {
			return queue.ConfigQueueRequest{}, err
		}
		return request, nil
	}

	if visibilityTimeoutSeconds == nil || expireSeconds == nil {
		var missing []string
		if visibilityTimeoutSeconds == nil {
			missing = append(missing, "--visibility-timeout-seconds")
		}
		if expireSeconds == nil {
			missing = append(missing, "--expire-seconds")
		}
		return queue.ConfigQueueRequest{}, fmt.Errorf("%s が必要です (--request JSON も指定できます)", strings.Join(missing, " と "))
	}

	return queue.ConfigQueueRequest{
		CommonServiceItem: queue.ConfigQueueRequestCommonServiceItem{
			Settings: queue.Settings{
				VisibilityTimeoutSeconds: queue.VisibilityTimeoutSeconds(*visibilityTimeoutSeconds),
				ExpireSeconds:            queue.ExpireSeconds(*expireSeconds),
			},
		},
	}, nil
}

type simpleMQQueueDeleteCommand struct {
	ID string `arg:"" help:"削除するキュー ID。"`
}

func (c simpleMQQueueDeleteCommand) Run(_ *kong.Context) error {
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	return op.Delete(context.Background(), c.ID)
}

type simpleMQQueueCountMessagesCommand struct {
	ID string `arg:"" help:"メッセージ数を取得するキュー ID。"`
}

func (c simpleMQQueueCountMessagesCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	count, err := op.CountMessages(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, map[string]int{"Count": count})
}

type simpleMQQueueRotateAPIKeyCommand struct {
	ID string `arg:"" help:"キーを発行するキュー ID。"`
}

func (c simpleMQQueueRotateAPIKeyCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	apiKey, err := op.RotateAPIKey(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, map[string]string{"APIKey": apiKey})
}

type simpleMQQueueClearMessagesCommand struct {
	ID string `arg:"" help:"メッセージをすべて削除するキュー ID。"`
}

func (c simpleMQQueueClearMessagesCommand) Run(_ *kong.Context) error {
	op, err := newSimpleMQQueueAPI()
	if err != nil {
		return err
	}
	return op.ClearMessages(context.Background(), c.ID)
}

type simpleMQMessageSendCommand struct {
	QueueName  string `name:"queue-name" required:"" help:"メッセージ送信先のキュー名 (5〜64文字の英数字またはハイフン)。"`
	APIKeyFile string `name:"api-key-file" required:"" help:"キュー API キーのファイル。キーを直接引数に指定しないでください。- の場合は標準入力から読み込みます。"`
	Content    string `name:"content" required:"" help:"メッセージ本文。API 定義では英数字と + / = の文字で最大256000文字です。"`
}

func (c simpleMQMessageSendCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQMessageAPI(c.QueueName, c.APIKeyFile)
	if err != nil {
		return err
	}
	item, err := op.Send(context.Background(), c.Content)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, item)
}

type simpleMQMessageReceiveCommand struct {
	QueueName  string `name:"queue-name" required:"" help:"受信元のキュー名 (5〜64文字の英数字またはハイフン)。"`
	APIKeyFile string `name:"api-key-file" required:"" help:"キュー API キーのファイル。キーを直接引数に指定しないでください。- の場合は標準入力から読み込みます。"`
}

func (c simpleMQMessageReceiveCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQMessageAPI(c.QueueName, c.APIKeyFile)
	if err != nil {
		return err
	}
	items, err := op.Receive(context.Background())
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, items)
}

type simpleMQMessageExtendTimeoutCommand struct {
	QueueName  string `name:"queue-name" required:"" help:"対象キュー名。"`
	APIKeyFile string `name:"api-key-file" required:"" help:"キュー API キーのファイル。キーを直接引数に指定しないでください。- の場合は標準入力から読み込みます。"`
	MessageID  string `arg:"" name:"message-id" help:"receive の出力にあるメッセージ ID。"`
}

func (c simpleMQMessageExtendTimeoutCommand) Run(ctx *kong.Context) error {
	format, err := outputType(ctx)
	if err != nil {
		return err
	}
	op, err := newSimpleMQMessageAPI(c.QueueName, c.APIKeyFile)
	if err != nil {
		return err
	}
	item, err := op.ExtendTimeout(context.Background(), c.MessageID)
	if err != nil {
		return err
	}
	return writeOutputWithFormat(ctx, format, item)
}

type simpleMQMessageDeleteCommand struct {
	QueueName  string `name:"queue-name" required:"" help:"対象キュー名。"`
	APIKeyFile string `name:"api-key-file" required:"" help:"キュー API キーのファイル。キーを直接引数に指定しないでください。- の場合は標準入力から読み込みます。"`
	MessageID  string `arg:"" name:"message-id" help:"削除するメッセージ ID。"`
}

func (c simpleMQMessageDeleteCommand) Run(_ *kong.Context) error {
	op, err := newSimpleMQMessageAPI(c.QueueName, c.APIKeyFile)
	if err != nil {
		return err
	}
	return op.Delete(context.Background(), c.MessageID)
}

func newSimpleMQQueueAPI() (simplemq.QueueAPI, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	queueClient, err := simplemq.NewQueueClient(&client)
	if err != nil {
		return nil, err
	}
	return simplemq.NewQueueOp(queueClient), nil
}

func newSimpleMQMessageAPI(queueName, apiKeyFile string) (simplemq.MessageAPI, error) {
	data, err := readSecretFile(apiKeyFile)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(string(data))
	if apiKey == "" {
		return nil, fmt.Errorf("API key file is empty")
	}

	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	messageClient, err := simplemq.NewMessageClient(apiKey, &client)
	if err != nil {
		return nil, err
	}
	return simplemq.NewMessageOp(messageClient, queueName), nil
}
