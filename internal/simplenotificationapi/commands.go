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

package simplenotificationapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	simplenotification "github.com/sacloud/sacloud-sdk-go/api/simple-notification"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	Destination DestinationCommands `cmd:"" help:"メールアドレスまたは Webhook の通知先を管理します。"`
	Group       GroupCommands       `cmd:"" help:"通知先をまとめるグループと通知送信を管理します。"`
	Routing     RoutingCommands     `cmd:"" help:"通知ルーティング設定を管理します。"`
	History     HistoryCommands     `cmd:"" help:"通知履歴を参照します。"`
}

type Runtime struct {
	DecodeRequest  func(string, any) error
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

func NewCommands(runtime Runtime, trace bool) Commands {
	destinationRuntime := DestinationRuntime{
		DecodeRequest: func(input string, destination any) error {
			return decodeDestinationRequest(input, destination, runtime.DecodeRequest,
				v1.CommonServiceItemProviderClassSaknoticedestination,
				v1.CommonServiceItemServiceClassCloudSaknoticedestination2)
		},
		ValidateOutput: runtime.ValidateOutput,
		WriteOutput:    runtime.WriteOutput,
	}
	groupRuntime := GroupRuntime{
		DecodeRequest: func(input string, destination any) error {
			return decodeResourceRequest(input, destination, runtime.DecodeRequest,
				v1.CommonServiceItemProviderClassSaknoticegroup,
				v1.CommonServiceItemServiceClassCloudSaknoticegroup2)
		},
		ValidateOutput: runtime.ValidateOutput,
		WriteOutput:    runtime.WriteOutput,
	}
	routingRuntime := RoutingRuntime{
		DecodeRequest: func(input string, destination any) error {
			return decodeResourceRequest(input, destination, runtime.DecodeRequest,
				v1.CommonServiceItemProviderClassSaknoticerouting,
				v1.CommonServiceItemServiceClassCloudSaknoticerouting2)
		},
		ValidateOutput: runtime.ValidateOutput,
		WriteOutput:    runtime.WriteOutput,
	}
	historyRuntime := HistoryRuntime{
		DecodeRequest:  runtime.DecodeRequest,
		ValidateOutput: runtime.ValidateOutput,
		WriteOutput:    runtime.WriteOutput,
	}
	return Commands{
		Destination: NewDestinationCommands(destinationRuntime, func() (DestinationAPI, error) {
			client, err := NewClient(trace)
			if err != nil {
				return nil, err
			}
			return simplenotification.NewDestinationOp(client), nil
		}),
		Group: NewGroupCommands(groupRuntime, func() (GroupAPI, error) {
			client, err := NewClient(trace)
			if err != nil {
				return nil, err
			}
			return simplenotification.NewGroupOp(client), nil
		}),
		Routing: NewRoutingCommands(routingRuntime, func() (RoutingAPI, error) {
			client, err := NewClient(trace)
			if err != nil {
				return nil, err
			}
			return simplenotification.NewRoutingOp(client), nil
		}),
		History: NewHistoryCommands(historyRuntime, func() (HistoryAPI, error) {
			client, err := NewClient(trace)
			if err != nil {
				return nil, err
			}
			return simplenotification.NewHistoryOp(client), nil
		}),
	}
}

func decodeDestinationRequest(input string, destination any, decode func(string, any) error, provider v1.CommonServiceItemProviderClass, serviceClass v1.CommonServiceItemServiceClass) error {
	requiresFile := false
	switch destination.(type) {
	case *v1.PostCommonServiceItemRequest:
		requiresFile = true
	case *v1.PutCommonServiceItemRequest:
		if !strings.HasPrefix(input, "@") {
			var request struct {
				CommonServiceItem struct {
					Settings json.RawMessage `json:"Settings"`
				} `json:"CommonServiceItem"`
			}
			if err := json.Unmarshal([]byte(input), &request); err == nil && len(request.CommonServiceItem.Settings) != 0 {
				requiresFile = true
			}
		}
	}
	if requiresFile && !strings.HasPrefix(input, "@") {
		return fmt.Errorf("通知先のメールアドレスやWebhook URLを保護するため、--request は @path.json で指定してください")
	}
	return decodeResourceRequest(input, destination, decode, provider, serviceClass)
}

func decodeResourceRequest(input string, destination any, decode func(string, any) error, provider v1.CommonServiceItemProviderClass, serviceClass v1.CommonServiceItemServiceClass) error {
	if _, isCreateRequest := destination.(*v1.PostCommonServiceItemRequest); !isCreateRequest {
		if decode == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		return decode(input, destination)
	}
	if decode == nil {
		return fmt.Errorf("API リクエスト処理が設定されていません")
	}

	data, err := requestData(input)
	if err != nil {
		return err
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request["CommonServiceItem"], &item); err != nil {
		return fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	providerJSON, err := json.Marshal(map[string]string{"Class": string(provider)})
	if err != nil {
		return fmt.Errorf("encode notification provider: %w", err)
	}
	serviceClassJSON, err := json.Marshal(serviceClass)
	if err != nil {
		return fmt.Errorf("encode notification service class: %w", err)
	}
	for key, value := range map[string]json.RawMessage{
		"Description": json.RawMessage(`""`),
		"Icon":        json.RawMessage("null"),
		"Tags":        json.RawMessage("[]"),
	} {
		if _, exists := item[key]; !exists {
			item[key] = value
		}
	}
	item["Provider"] = providerJSON
	item["ServiceClass"] = serviceClassJSON
	request["CommonServiceItem"], err = json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	data, err = json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode create request: %w", err)
	}
	return decode(string(data), destination)
}

func requestData(input string) ([]byte, error) {
	if !strings.HasPrefix(input, "@") {
		return []byte(input), nil
	}
	data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
	if err != nil {
		return nil, fmt.Errorf("read request file: %w", err)
	}
	return data, nil
}

func SetDestinationProvider(request *v1.PostCommonServiceItemRequest) error {
	request.CommonServiceItem.Provider.Class = v1.CommonServiceItemProviderClassSaknoticedestination
	request.CommonServiceItem.ServiceClass = v1.CommonServiceItemServiceClassCloudSaknoticedestination2
	return nil
}

func SetGroupProvider(request *v1.PostCommonServiceItemRequest) error {
	request.CommonServiceItem.Provider.Class = v1.CommonServiceItemProviderClassSaknoticegroup
	request.CommonServiceItem.ServiceClass = v1.CommonServiceItemServiceClassCloudSaknoticegroup2
	return nil
}

func SetRoutingProvider(request *v1.PostCommonServiceItemRequest) error {
	request.CommonServiceItem.Provider.Class = v1.CommonServiceItemProviderClassSaknoticerouting
	request.CommonServiceItem.ServiceClass = v1.CommonServiceItemServiceClassCloudSaknoticerouting2
	return nil
}

func NewClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return simplenotification.NewClient(client)
}
