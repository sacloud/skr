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
	"io"
	"os"
	"strings"

	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
)

// DecodeRequest は CommonServiceItem 操作のリクエストを読み込みます。通知先
// (saknoticedestination) のリクエストにはメールアドレスや Webhook URL が含まれ得るため、
// デコードエラーを入力値を含まないメッセージへ変換します。通知先の更新リクエストで
// Settings を含む入力は、@path.json 形式のみ受け付けます。
func DecodeRequest(input string, destination any) error {
	data, err := requestInput(input)
	if err != nil {
		return err
	}
	if isDestinationRequest(data) {
		if err := requireDestinationRequestFile(input, data, destination); err != nil {
			return err
		}
		return protectDestinationError("通知先リクエストを読み込めません。JSON の形式と入力ファイルを確認してください",
			decodeJSON(data, destination))
	}
	return decodeJSON(data, destination)
}

// isDestinationRequest は入力 JSON が通知先のリクエストかどうかを返します。作成
// リクエストは request_input_transform が設定した Provider.Class で、更新リクエストは
// 通知先のみが持つ Settings.Type (email / webhook) で判定します。共通の
// CommonServiceItem を含まないリクエスト (メッセージ送信やルーティング順序の変更など) は
// false になります。
func isDestinationRequest(data []byte) bool {
	var request struct {
		CommonServiceItem struct {
			Provider struct {
				Class string `json:"Class"`
			} `json:"Provider"`
			Settings json.RawMessage `json:"Settings"`
		} `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return false
	}
	if request.CommonServiceItem.Provider.Class == string(v1.CommonServiceItemProviderClassSaknoticedestination) {
		return true
	}
	var settings struct {
		Type string `json:"Type"`
	}
	if err := json.Unmarshal(request.CommonServiceItem.Settings, &settings); err != nil {
		return false
	}
	settingsType := v1.CommonServiceItemDestinationSettingsType(settings.Type)
	return settingsType == v1.CommonServiceItemDestinationSettingsTypeEmail ||
		settingsType == v1.CommonServiceItemDestinationSettingsTypeWebhook
}

// requireDestinationRequestFile は通知先の更新リクエストが Settings を持つ場合に、
// @path.json 形式以外の入力を拒否します。JSON の形式が不正な入力も拒否し、
// Settings の有無で入力規則を回避できないようにします。作成リクエストは
// request_input_transform で @path.json に限定済みです。
func requireDestinationRequestFile(input string, data []byte, destination any) error {
	if _, isUpdate := destination.(*v1.PutCommonServiceItemRequest); !isUpdate {
		return nil
	}
	if strings.HasPrefix(input, "@") {
		return nil
	}
	var request struct {
		CommonServiceItem struct {
			Settings json.RawMessage `json:"Settings"`
		} `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("通知先リクエストを読み込めません。JSON の形式と入力ファイルを確認してください")
	}
	if len(request.CommonServiceItem.Settings) != 0 {
		return fmt.Errorf("通知先のメールアドレスやWebhook URLを保護するため、--request は @path.json で指定してください")
	}
	return nil
}

// SetCreateRequestProviderInput は --provider-class の値を作成リクエスト JSON の
// Provider と ServiceClass に設定します。API の作成リクエストでは Provider と
// ServiceClass が必須のため、デコードの前に入力 JSON に設定します。未指定の
// Description、Icon、Tags は API の必須項目を満たす既定値で補完します。
// 通知先は Settings にメールアドレスや Webhook URL を含むため、@path.json 形式のみ受け付けます。
func SetCreateRequestProviderInput(command *CommonServiceItemCreateCommand, input string) (string, error) {
	class, err := parseProviderClass(command.ProviderClass)
	if err != nil {
		return "", err
	}
	if class == v1.CommonServiceItemProviderClassSaknoticedestination && !strings.HasPrefix(input, "@") {
		return "", fmt.Errorf("通知先のメールアドレスやWebhook URLを保護するため、--request は @path.json で指定してください")
	}
	updated, err := applyCreateRequestInput(input, class)
	if err != nil {
		if class == v1.CommonServiceItemProviderClassSaknoticedestination {
			return "", protectDestinationError("通知先リクエストを読み込めません。JSON の形式と入力ファイルを確認してください", err)
		}
		return "", err
	}
	return updated, nil
}

// applyCreateRequestInput は作成リクエスト JSON に Provider、ServiceClass、
// 未指定項目の既定値を設定します。
func applyCreateRequestInput(input string, class v1.CommonServiceItemProviderClass) (string, error) {
	request, item, err := decodeRequestItem(input)
	if err != nil {
		return "", err
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
	if err := setRequestItemProvider(item, class, serviceClassFor(class)); err != nil {
		return "", err
	}
	return encodeRequestItem(request, item)
}

// decodeRequestItem は --request の入力を読み込み、CommonServiceItem の
// オブジェクトとして展開します。
func decodeRequestItem(input string) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	data, err := requestInput(input)
	if err != nil {
		return nil, nil, err
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, nil, fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request["CommonServiceItem"], &item); err != nil {
		return nil, nil, fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return nil, nil, fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	return request, item, nil
}

// serviceClassFor は種別に対応する ServiceClass を返します。ServiceClass は作成
// リクエストの必須項目で、種別ごとに値が決まっています。
func serviceClassFor(class v1.CommonServiceItemProviderClass) string {
	switch class {
	case v1.CommonServiceItemProviderClassSaknoticedestination:
		return string(v1.PostCommonServiceItemRequestCommonServiceItemServiceClassCloudSaknoticedestination2)
	case v1.CommonServiceItemProviderClassSaknoticegroup:
		return string(v1.PostCommonServiceItemRequestCommonServiceItemServiceClassCloudSaknoticegroup2)
	default:
		return string(v1.PostCommonServiceItemRequestCommonServiceItemServiceClassCloudSaknoticerouting2)
	}
}

// setRequestItemProvider は CommonServiceItem の Provider を指定した種別で上書きし、
// ServiceClass も設定します。リクエスト JSON に種別を複写させる代わりに、
// --provider-class の値を API のリクエストに反映します。
func setRequestItemProvider(item map[string]json.RawMessage, class v1.CommonServiceItemProviderClass, serviceClass string) error {
	provider, err := json.Marshal(struct {
		Class v1.CommonServiceItemProviderClass `json:"Class"`
	}{Class: class})
	if err != nil {
		return fmt.Errorf("encode notification provider: %w", err)
	}
	item["Provider"] = provider
	item["ServiceClass"] = json.RawMessage(marshalString(serviceClass))
	return nil
}

// encodeRequestItem は CommonServiceItem をリクエスト JSON に戻します。
func encodeRequestItem(request map[string]json.RawMessage, item map[string]json.RawMessage) (string, error) {
	encoded, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	request["CommonServiceItem"] = encoded
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode request JSON: %w", err)
	}
	return string(data), nil
}

func marshalString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		// 文字列の JSON への変換で失敗しないため、この分岐は実行されません。
		return `""`
	}
	return string(data)
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
		//nolint:gosec // パスは --request 経由でユーザーが明示的に指定したものです。
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

// decodeJSON はリクエスト JSON を destination へ展開します。
func decodeJSON(data []byte, destination any) error {
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}
