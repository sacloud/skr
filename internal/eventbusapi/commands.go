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
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

// Commands は EventBus API のコマンド群です。CommonServiceItem 操作は
// Kong の embed で展開して、1 階層のフラットなコマンド構造にしています。
type Commands struct {
	CommonServiceItem CommonServiceItemCommands `embed:""`
}

// CommonServiceItemUpdateSecretCommand は実行設定のシークレットを更新します。
// シークレット更新は実行設定専用の操作のため --provider-class を付けず、
// 種別を eventbusprocessconfiguration に固定します。
type CommonServiceItemUpdateSecretCommand struct {
	ID         string `arg:"" help:"シークレットを更新する実行設定 ID。"`
	SecretFile string `name:"secret-file" required:"" help:"Secret オブジェクトの JSON ファイル。- の場合は標準入力から読み込みます。SimpleMQ: {\"APIKey\":\"...\"}。API キー認証: {\"AccessToken\":\"...\",\"AccessTokenSecret\":\"...\"}。"`
	factory    CommonServiceItemAPIFactory
	runtime    CommonServiceItemRuntime
}

// commonServiceItemSecretUpdater はシークレット更新をサポートする API 操作が実装するメソッドです。
// シークレット更新は実行設定の操作のみに存在するため、共通の API interface には含まれません。
type commonServiceItemSecretUpdater interface {
	UpdateSecret(context.Context, string, v1.SetSecretRequest) error
}

func (c *CommonServiceItemUpdateSecretCommand) Run(_ *kong.Context) error {
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
	op, err := c.factory(string(v1.ProviderClassEventbusprocessconfiguration))
	if err != nil {
		return err
	}
	secretOp, ok := op.(commonServiceItemSecretUpdater)
	if !ok {
		return fmt.Errorf("シークレットの更新は実行設定 (eventbusprocessconfiguration) に対してのみ実行できます")
	}
	request := v1.SetSecretRequest{Secret: secret}
	return secretOp.UpdateSecret(context.Background(), c.ID, request)
}

// NewCommonServiceItemAPI は --provider-class で指定された種別に対応する API 操作を返します。
// 種別ごとの SDK 操作は、一覧操作の Provider.Class 絞り込みと、作成・更新時の
// リクエストへの Provider.Class 設定を行います。読み取りと削除は ID で直接操作するため
// 種別に依存しません。
func NewCommonServiceItemAPI(trace bool, providerClass string) (CommonServiceItemAPI, error) {
	class, err := parseProviderClass(providerClass)
	if err != nil {
		return nil, err
	}
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return selectCommonServiceItemOp(class, client), nil
}

// SetCreateRequestProviderInput は --provider-class の値を作成リクエスト JSON の Provider に設定します。
// API の作成リクエストでは Provider が必須のため、デコードの前に入力 JSON に設定します。
// 入力は --request の値 (@path、-、インライン JSON) です。
func SetCreateRequestProviderInput(command *CommonServiceItemCreateCommand, input string) (string, error) {
	return setRequestProviderInput(input, v1.ProviderClass(command.ProviderClass))
}

// SetUpdateRequestProviderInput は --provider-class の値を更新リクエスト JSON の Provider に設定します。
func SetUpdateRequestProviderInput(command *CommonServiceItemUpdateCommand, input string) (string, error) {
	return setRequestProviderInput(input, v1.ProviderClass(command.ProviderClass))
}

// setRequestProviderInput は入力 JSON の CommonServiceItem.Provider を指定した種別で上書きします。
// リクエスト JSON に種別を複写させる代わりに、--provider-class の値を API のリクエストに反映します。
func setRequestProviderInput(input string, providerClass v1.ProviderClass) (string, error) {
	data, err := requestInput(input)
	if err != nil {
		return "", err
	}
	var request struct {
		CommonServiceItem json.RawMessage `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return "", fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request.CommonServiceItem, &item); err != nil {
		return "", fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return "", fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	provider, err := json.Marshal(struct {
		Class v1.ProviderClass `json:"Class"`
	}{Class: providerClass})
	if err != nil {
		return "", fmt.Errorf("encode EventBus provider: %w", err)
	}
	item["Provider"] = provider
	request.CommonServiceItem, err = json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	data, err = json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode request JSON: %w", err)
	}
	return string(data), nil
}

// requestInput は --request の入力を読み込みます。@path はファイル、- は標準入力から読み込みます。
func requestInput(input string) ([]byte, error) {
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read request from standard input: %w", err)
		}
		return data, nil
	}
	if strings.HasPrefix(input, "@") {

		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

// selectCommonServiceItemOp は種別に対応する SDK 操作を選択します。クライアントは
// 操作の生成にのみ使われるため、選択の検証にはダミーのクライアントを渡せます。
func selectCommonServiceItemOp(class v1.ProviderClass, client *v1.Client) CommonServiceItemAPI {
	switch class {
	case v1.ProviderClassEventbusprocessconfiguration:
		return eventbus.NewProcessConfigurationOp(client)
	case v1.ProviderClassEventbusschedule:
		return eventbus.NewScheduleOp(client)
	default:
		return eventbus.NewTriggerOp(client)
	}
}

// parseProviderClass は --provider-class の値を ProviderClass に変換し、
// 未対応の値をエラーにします。
func parseProviderClass(value string) (v1.ProviderClass, error) {
	switch v1.ProviderClass(value) {
	case v1.ProviderClassEventbusprocessconfiguration, v1.ProviderClassEventbusschedule, v1.ProviderClassEventbustrigger:
		return v1.ProviderClass(value), nil
	default:
		return "", fmt.Errorf("--provider-class には eventbusprocessconfiguration、eventbusschedule、eventbustrigger のいずれかを指定してください")
	}
}

func newClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return eventbus.NewClient(client)
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // パスは --secret-file 経由でユーザーが明示的に指定したものです。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}
