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
	"context"
	"encoding/json"
	"fmt"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// ProcessConfigurationSettings は実行設定の宛先とパラメーターです。
//
// Destination に応じて次の 2 つのフィールドを指定します。他の宛先のフィールドを
// 指定するとエラーになります。
//
//   - simplenotification: GroupID と Message
//   - simplemq:           QueueName と Content
//   - autoscale:          Action と ResourceID
//
// 指定しなかった宛先フィールド (nil) は、作成時は空文字列として送信します。
type ProcessConfigurationSettings struct {
	// Destination はジョブの宛先です。simplenotification、simplemq、autoscale の
	// いずれかです。それ以外の値はエラーになります。
	Destination string
	// GroupID は宛先 simplenotification のグループ ID (API の group_id) です。
	GroupID *string
	// Message は宛先 simplenotification のメッセージ (API の message) です。
	Message *string
	// QueueName は宛先 simplemq のキュー名 (API の queue_name) です。
	QueueName *string
	// Content は宛先 simplemq で配送する本文 (API の content) です。
	Content *string
	// Action は宛先 autoscale のアクション (API の action) です。
	// scale_up または scale_down です。
	Action *string
	// ResourceID は宛先 autoscale の対象リソース ID (API の resource_id) です。
	ResourceID *string
}

// CreateProcessConfigurationRequest は実行設定を作成するリクエストです。
type CreateProcessConfigurationRequest struct {
	// Name は実行設定の名前です。必須で、空の場合はエラーを返します。
	Name string
	// Description は説明です。空文字列は API には null として送信します。
	Description string
	// Settings は宛先とパラメーターです。
	Settings ProcessConfigurationSettings
}

// UpdateProcessConfigurationRequest は実行設定を更新するリクエストです。
//
// ポインター型のフィールドは、nil なら現状維持、non-nil なら指定値への変更です。
// Settings 関連のフィールド (Destination と宛先別フィールド) を 1 つでも指定すると、
// API の Settings 全体を送信します。Name と Description は Settings と独立して
// 更新でき、その場合は Settings を送信しません。
//
// 宛先パラメーターのマージ規則は次のとおりです。
//
//   - 現在と同じ宛先のまま一部のパラメーターを指定した場合、宛先対応表にあるキーは
//     指定値で置き換え、nil のキーは既存値を保持します。対応表にない既存のキーも
//     保持します。
//   - Destination を現在と異なる値に変更した場合、旧宛先のパラメーターは
//     引き継がれません。新宛先のパラメーターを指定してください。
//   - 宛先の変更時や既存値のないキーでは、指定しなかった宛先フィールド (nil) は
//     空文字列として送信します。
//
// 読み取りと更新の間に他の利用者による変更がないことは検証しません。
type UpdateProcessConfigurationRequest struct {
	// ID は更新対象の実行設定 ID です。存在しない場合や種別が一致しない場合は
	// エラーを返します。
	ID string
	// Name は新しい名前です。nil のときは現状維持します。
	Name *string
	// Description は新しい説明です。nil のときは現状維持します。空文字列は
	// API には空文字列として送信します。作成時と異なり null にはなりません。
	Description *string
	// Destination は新しい宛先です。nil のときは現状維持します。
	Destination *string
	// GroupID は宛先 simplenotification のグループ ID (API の group_id) です。
	GroupID *string
	// Message は宛先 simplenotification のメッセージ (API の message) です。
	Message *string
	// QueueName は宛先 simplemq のキュー名 (API の queue_name) です。
	QueueName *string
	// Content は宛先 simplemq で配送する本文 (API の content) です。
	Content *string
	// Action は宛先 autoscale のアクション (API の action) です。
	Action *string
	// ResourceID は宛先 autoscale の対象リソース ID (API の resource_id) です。
	ResourceID *string
}

// SetProcessConfigurationSecretRequest は実行設定へ secret を登録するリクエストです。
//
// Secret は宛先に応じた形式の JSON オブジェクトを SDK の型 (v1.SetSecretRequestSecret)
// で指定します。宛先 simplemq は {"APIKey":"..."}、それ以外の宛先は
// {"AccessToken":"...","AccessTokenSecret":"..."} です。既存の secret は置き換わります。
// この操作は応答を返しません。
type SetProcessConfigurationSecretRequest struct {
	// ID は secret を登録する実行設定 ID です。存在しない場合や種別が一致しない場合は
	// エラーを返します。
	ID string
	// Secret は宛先に応じた secret の内容です。
	Secret v1.SetSecretRequestSecret
}

// parameterKeys は宛先ごとの Parameters のキー (API のフィールド) です。
// 宛先の妥当性検証も兼ねます。
func parameterKeys(destination string) ([]string, error) {
	switch destination {
	case "simplenotification":
		return []string{"group_id", "message"}, nil
	case "simplemq":
		return []string{"queue_name", "content"}, nil
	case "autoscale":
		return []string{"action", "resource_id"}, nil
	default:
		return nil, fmt.Errorf("Destination は simplenotification、simplemq、autoscale のいずれかです")
	}
}

// parameters は宛先別の入力と既存 Parameters を API の Settings へ変換します。
// 既存 Parameters は同じ宛先のキーと未知のキーを保持し、宛先対応表のキーの値が
// 文字列でない場合はエラーになります。
func parameters(input ProcessConfigurationSettings, existing string) (v1.Settings, error) {
	keys, err := parameterKeys(input.Destination)
	if err != nil {
		return v1.Settings{}, err
	}
	fields := map[string]*string{
		"group_id": input.GroupID, "message": input.Message,
		"queue_name": input.QueueName, "content": input.Content,
		"action": input.Action, "resource_id": input.ResourceID,
	}
	for key, value := range fields {
		if value != nil && key != keys[0] && key != keys[1] {
			return v1.Settings{}, fmt.Errorf("Destination と %s の組み合わせが不正です", key)
		}
	}
	values := map[string]json.RawMessage{}
	if existing != "" {
		if err := json.Unmarshal([]byte(existing), &values); err != nil || values == nil {
			return v1.Settings{}, fmt.Errorf("既存 Parameters が JSON オブジェクトではありません")
		}
	}
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || string(raw) == "null" {
				return v1.Settings{}, fmt.Errorf("Parameters の %s は文字列である必要があります", key)
			}
		}
		if fields[key] != nil {
			raw, err := json.Marshal(*fields[key])
			if err != nil {
				return v1.Settings{}, err
			}
			values[key] = raw
		} else if _, ok := values[key]; !ok {
			values[key] = json.RawMessage(`""`)
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return v1.Settings{}, fmt.Errorf("Parameters を変換できません: %w", err)
	}
	return v1.NewProcessConfigurationSettingsSettings(v1.ProcessConfigurationSettings{
		Destination: v1.ProcessConfigurationSettingsDestination(input.Destination), Parameters: string(data),
	}), nil
}

// ListProcessConfigurations は実行設定の一覧を返します。
//
// 応答は生の SDK モデルです。利用者向けの形状には ProjectProcessConfiguration を
// 使ってください。
func (s *Service) ListProcessConfigurations(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.process.List(ctx)
}

// ReadProcessConfiguration は ID を指定して実行設定を返します。
//
// 応答の ID とリソース種別、Settings の種別を検証し、一致しない場合はエラーを返します。
func (s *Service) ReadProcessConfiguration(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.process, req.ID, v1.ProviderClassEventbusprocessconfiguration)
}

// CreateProcessConfiguration は実行設定を作成します。
//
// 宛先と宛先別フィールドの組み合わせを検証します。Description の空文字列は
// API には null として送信します。指定しなかった宛先フィールド (nil) は空文字列と
// して送信します。
func (s *Service) CreateProcessConfiguration(ctx context.Context, req *CreateProcessConfigurationRequest) (*v1.CommonServiceItem, error) {
	settings, err := parameters(req.Settings, "")
	if err != nil {
		return nil, err
	}
	request, err := createRequest(req.Name, req.Description, settings)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.process.Create(ctx, request)
}

// UpdateProcessConfiguration は実行設定を更新します。
//
// 現状を API から読み取り、宛先パラメーターのマージ規則 (UpdateProcessConfigurationRequest
// を参照) に従って API の Settings を組み立てます。Settings 関連のフィールドを 1 つも
// 指定しない場合は Settings を送信せず、Name と Description だけを更新します。
// 読み取りと更新の間に context がキャンセルされた場合は更新 API を呼び出しません。
func (s *Service) UpdateProcessConfiguration(ctx context.Context, req *UpdateProcessConfigurationRequest) (*v1.CommonServiceItem, error) {
	current, err := s.ReadProcessConfiguration(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.Destination != nil || req.GroupID != nil || req.Message != nil || req.QueueName != nil || req.Content != nil || req.Action != nil || req.ResourceID != nil {
		old := current.Settings.ProcessConfigurationSettings
		destination := string(old.Destination)
		existing := old.Parameters
		if req.Destination != nil && *req.Destination != destination {
			destination, existing = *req.Destination, ""
		}
		// 既存の空文字列を、新規設定の空オブジェクトとして扱いません。
		if existing == "" && (req.Destination == nil || *req.Destination == string(old.Destination)) {
			return nil, fmt.Errorf("既存 Parameters が JSON オブジェクトではありません")
		}
		settings, err := parameters(ProcessConfigurationSettings{Destination: destination,
			GroupID: req.GroupID, Message: req.Message, QueueName: req.QueueName,
			Content: req.Content, Action: req.Action, ResourceID: req.ResourceID}, existing)
		if err != nil {
			return nil, err
		}
		request.CommonServiceItem.Settings.SetTo(settings)
	}
	return update(ctx, s.process, req.ID, request)
}

// DeleteProcessConfiguration は ID を指定して実行設定を削除します。
//
// 削除前に API から対象を再取得し、ID とリソース種別、Settings の種別が一致しない
// 場合は削除せずにエラーを返します。context がキャンセルされた場合は削除 API を
// 呼び出しません。
func (s *Service) DeleteProcessConfiguration(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.process, req.ID, v1.ProviderClassEventbusprocessconfiguration)
}

// SetProcessConfigurationSecret は実行設定へ secret を登録または置き換えます。
//
// 登録前に API から対象を再取得し、ID とリソース種別、Settings の種別が一致しない
// 場合は secret を送信せずにエラーを返します。secret の値をエラーメッセージに
// 含めないのは呼び出し側の責務です。
func (s *Service) SetProcessConfigurationSecret(ctx context.Context, req *SetProcessConfigurationSecretRequest) error {
	if _, err := s.ReadProcessConfiguration(ctx, &ReadRequest{ID: req.ID}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.process.UpdateSecret(ctx, req.ID, v1.SetSecretRequest{Secret: req.Secret})
}
