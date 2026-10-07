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

package simplemqapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq"
	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/queue"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	Queue   QueueCommands   `cmd:"" help:"キューを作成・管理します。"`
	Message MessageCommands `cmd:"" help:"キューの API キーを使ってメッセージを送受信します。"`
}

type QueueCreateCommand struct {
	Request     *string `help:"個別フラグと併用不可。CreateQueueRequest JSON を直接または @path.json で指定します。CommonServiceItem.Name は5〜64文字の英数字またはハイフンです。Provider.Class は simplemq に設定します。Tags と Icon は JSON で指定します。例: --request='{\"CommonServiceItem\":{\"Name\":\"sample-queue\"}}'"`
	Name        *string `help:"必須: キュー名 (5〜64文字の英数字またはハイフン)。例: sample-queue"`
	Description *string `help:"任意: キューの説明。"`
	factory     QueueAPIFactory
	runtime     QueueRuntime
}

func (c *QueueCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	flags := c.Name != nil || c.Description != nil
	if c.Request != nil && flags {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	var request queue.CreateQueueRequest
	if c.Request != nil {
		if err := decodeCreateRequest(*c.Request, &request); err != nil {
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

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, item)
}

func decodeCreateRequest(input string, destination *queue.CreateQueueRequest) error {
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
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

type QueueConfigCommand struct {
	ID                       string  `arg:"" help:"設定を変更するキュー ID。"`
	Request                  *string `help:"個別フラグと併用不可。ConfigQueueRequest JSON を直接または @path.json で指定します。Description、Tags、Icon など追加項目は JSON で指定します。例: --request='{\"CommonServiceItem\":{\"Settings\":{\"VisibilityTimeoutSeconds\":30,\"ExpireSeconds\":345600}}}'"`
	VisibilityTimeoutSeconds *int    `name:"visibility-timeout-seconds" help:"必須: 可視性タイムアウト秒数 (5〜900)。API の CommonServiceItem.Settings.VisibilityTimeoutSeconds に対応します。--expire-seconds と併せて指定します。"`
	ExpireSeconds            *int    `name:"expire-seconds" help:"必須: メッセージ保存期間秒数 (60〜1209600)。API の CommonServiceItem.Settings.ExpireSeconds に対応します。--visibility-timeout-seconds と併せて指定します。"`
	factory                  QueueAPIFactory
	runtime                  QueueRuntime
}

func (c *QueueConfigCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request, err := configRequest(c.Request, c.VisibilityTimeoutSeconds, c.ExpireSeconds, c.runtime.DecodeRequest)
	if err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Config(context.Background(), c.ID, request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, item)
}

func configRequest(input *string, visibilityTimeoutSeconds, expireSeconds *int, decode func(string, any) error) (queue.ConfigQueueRequest, error) {
	if input != nil {
		if visibilityTimeoutSeconds != nil || expireSeconds != nil {
			return queue.ConfigQueueRequest{}, fmt.Errorf("--request と個別フラグは併用できません")
		}
		if decode == nil {
			return queue.ConfigQueueRequest{}, fmt.Errorf("API リクエスト処理が設定されていません")
		}
		var request queue.ConfigQueueRequest
		if err := decode(*input, &request); err != nil {
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

type QueueCountMessagesCommand struct {
	ID      string `arg:"" help:"メッセージ数を取得するキュー ID。"`
	factory QueueAPIFactory
	runtime QueueRuntime
}

func (c *QueueCountMessagesCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	count, err := op.CountMessages(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, map[string]int{"Count": count})
}

type QueueRotateAPIKeyCommand struct {
	ID      string `arg:"" help:"キーを発行するキュー ID。"`
	factory QueueAPIFactory
	runtime QueueRuntime
}

func (c *QueueRotateAPIKeyCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	apiKey, err := op.RotateAPIKey(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, map[string]string{"APIKey": apiKey})
}

func (r QueueRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r QueueRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
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

func NewQueueAPI(trace bool) (QueueAPI, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	queueClient, err := simplemq.NewQueueClient(client)
	if err != nil {
		return nil, err
	}
	return simplemq.NewQueueOp(queueClient), nil
}

func NewMessageAPI(queueName, apiKeyFile string, trace bool) (MessageAPI, error) {
	data, err := readSecretFile(apiKeyFile)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(string(data))
	if apiKey == "" {
		return nil, fmt.Errorf("API key file is empty")
	}

	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	messageClient, err := simplemq.NewMessageClient(apiKey, client)
	if err != nil {
		return nil, err
	}
	return simplemq.NewMessageOp(messageClient, queueName), nil
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // Path is explicitly provided by the user via --api-key-file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}
